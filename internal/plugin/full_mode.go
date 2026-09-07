package plugin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	fullModeUploadTTL                = 15 * time.Minute
	fullModeUploadChunkSize          = 6000
	fullModeUploadsPerSession        = 2
	fullModeUploadsTotal             = 8
	fullModeUploadReservedBytesLimit = 96 << 20
)

type fullModeSession struct {
	expiresAt time.Time
}

type fullModeUpload struct {
	sessionHash [32]byte
	expiresAt   time.Time
	chunkCount  int
	chunks      map[int]string
}

// fullModeUploadChunkLimit converts a decoded payload limit into the maximum
// number of 6,000-character raw-url-base64 chunks that can encode it.
func fullModeUploadChunkLimit(maxPayloadBytes int) int {
	if maxPayloadBytes <= 0 {
		return 0
	}
	encodedBytes := (maxPayloadBytes / 3) * 4
	if remainder := maxPayloadBytes % 3; remainder > 0 {
		encodedBytes += remainder + 1
	}
	return (encodedBytes + fullModeUploadChunkSize - 1) / fullModeUploadChunkSize
}

func fullModeUploadReservedBytes(chunkCount int) int {
	if chunkCount <= 0 {
		return 0
	}
	return chunkCount * fullModeUploadChunkSize
}

func (r *pluginRuntime) fullModeSessionTTL() time.Duration {
	r.mu.RLock()
	minutes := r.config.FullModeSessionTTLMinutes
	r.mu.RUnlock()
	if minutes == 0 {
		minutes = defaultFullModeSessionTTLMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func (r *pluginRuntime) createFullModeSession() (string, error) {
	ttl := r.fullModeSessionTTL()
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", err
	}
	now := nowUTC()
	hash := sha256.Sum256(tokenBytes[:])
	r.fullModeMu.Lock()
	if r.fullModeSessions == nil {
		r.fullModeSessions = make(map[[32]byte]fullModeSession)
	}
	for key, session := range r.fullModeSessions {
		if !now.Before(session.expiresAt) {
			delete(r.fullModeSessions, key)
		}
	}
	r.fullModeSessions[hash] = fullModeSession{expiresAt: now.Add(ttl)}
	r.fullModeMu.Unlock()
	return base64.RawURLEncoding.EncodeToString(tokenBytes[:]), nil
}

func (r *pluginRuntime) validFullModeSession(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 128 {
		return false
	}
	tokenBytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(tokenBytes) != 32 {
		return false
	}
	want := sha256.Sum256(tokenBytes)
	now := nowUTC()
	r.fullModeMu.Lock()
	defer r.fullModeMu.Unlock()
	for key, session := range r.fullModeSessions {
		if !now.Before(session.expiresAt) {
			delete(r.fullModeSessions, key)
		}
	}
	for key, session := range r.fullModeSessions {
		if subtle.ConstantTimeCompare(key[:], want[:]) == 1 {
			return now.Before(session.expiresAt)
		}
	}
	return false
}

func (r *pluginRuntime) revokeFullModeSession(raw string) {
	raw = strings.TrimSpace(raw)
	tokenBytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(tokenBytes) != 32 {
		return
	}
	hash := sha256.Sum256(tokenBytes)
	r.fullModeMu.Lock()
	delete(r.fullModeSessions, hash)
	for id, upload := range r.fullModeUploads {
		if subtle.ConstantTimeCompare(upload.sessionHash[:], hash[:]) == 1 {
			delete(r.fullModeUploads, id)
		}
	}
	r.fullModeMu.Unlock()
}

func fullModeSessionFromRequest(request pluginapi.ManagementRequest) string {
	return request.Headers.Get("X-Full-Mode-Session")
}

func fullModeSessionHash(raw string) ([32]byte, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 128 {
		return [32]byte{}, false
	}
	tokenBytes, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(tokenBytes) != 32 {
		return [32]byte{}, false
	}
	return sha256.Sum256(tokenBytes), true
}

func (r *pluginRuntime) purgeExpiredFullModeUploads(now time.Time) {
	for id, upload := range r.fullModeUploads {
		if !now.Before(upload.expiresAt) {
			delete(r.fullModeUploads, id)
		}
	}
}

func (r *pluginRuntime) fullModeStagedPayloadResponse(request pluginapi.ManagementRequest, maxPayloadBytes int, contentType string, handler func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error)) (pluginapi.ManagementResponse, error) {
	session := fullModeSessionFromRequest(request)
	if !r.validFullModeSession(session) {
		return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
	}
	sessionHash, ok := fullModeSessionHash(session)
	if !ok {
		return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
	}

	now := nowUTC()
	switch request.Query.Get("stage") {
	case "begin":
		chunkCount, err := strconv.Atoi(request.Query.Get("chunks"))
		if err != nil || chunkCount < 1 || chunkCount > fullModeUploadChunkLimit(maxPayloadBytes) {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid full-mode upload chunk count"}), nil
		}
		var idBytes [16]byte
		if _, err := rand.Read(idBytes[:]); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]string{"error": "could not create full-mode upload"}), nil
		}
		id := base64.RawURLEncoding.EncodeToString(idBytes[:])
		r.fullModeMu.Lock()
		if r.fullModeUploads == nil {
			r.fullModeUploads = make(map[string]fullModeUpload)
		}
		r.purgeExpiredFullModeUploads(now)
		sessionUploads, totalUploads, reservedBytes := 0, 0, 0
		for _, upload := range r.fullModeUploads {
			totalUploads++
			reservedBytes += fullModeUploadReservedBytes(upload.chunkCount)
			if subtle.ConstantTimeCompare(upload.sessionHash[:], sessionHash[:]) == 1 {
				sessionUploads++
			}
		}
		if sessionUploads >= fullModeUploadsPerSession || totalUploads >= fullModeUploadsTotal || reservedBytes+fullModeUploadReservedBytes(chunkCount) > fullModeUploadReservedBytesLimit {
			r.fullModeMu.Unlock()
			return jsonResponse(http.StatusTooManyRequests, map[string]string{"error": "too many active full-mode uploads"}), nil
		}
		r.fullModeUploads[id] = fullModeUpload{
			sessionHash: sessionHash,
			expiresAt:   now.Add(fullModeUploadTTL),
			chunkCount:  chunkCount,
			chunks:      make(map[int]string, chunkCount),
		}
		r.fullModeMu.Unlock()
		return jsonResponse(http.StatusOK, map[string]string{"upload": id}), nil
	case "chunk":
		id := request.Query.Get("upload")
		index, err := strconv.Atoi(request.Query.Get("index"))
		chunk := request.Headers.Get("X-Full-Mode-Payload")
		if id == "" || err != nil || index < 0 || len(chunk) == 0 || len(chunk) > fullModeUploadChunkSize || strings.ContainsAny(chunk, "=+/ \t\r\n") {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid full-mode upload chunk"}), nil
		}
		r.fullModeMu.Lock()
		r.purgeExpiredFullModeUploads(now)
		upload, exists := r.fullModeUploads[id]
		if !exists || subtle.ConstantTimeCompare(upload.sessionHash[:], sessionHash[:]) != 1 || index >= upload.chunkCount {
			r.fullModeMu.Unlock()
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "unknown full-mode upload"}), nil
		}
		upload.chunks[index] = chunk
		r.fullModeUploads[id] = upload
		r.fullModeMu.Unlock()
		return jsonResponse(http.StatusOK, map[string]bool{"uploaded": true}), nil
	case "commit":
		id := request.Query.Get("upload")
		if id == "" {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "missing full-mode upload"}), nil
		}
		r.fullModeMu.Lock()
		r.purgeExpiredFullModeUploads(now)
		upload, exists := r.fullModeUploads[id]
		if exists {
			delete(r.fullModeUploads, id)
		}
		r.fullModeMu.Unlock()
		if !exists || subtle.ConstantTimeCompare(upload.sessionHash[:], sessionHash[:]) != 1 {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "unknown full-mode upload"}), nil
		}
		var encoded strings.Builder
		for index := 0; index < upload.chunkCount; index++ {
			chunk, present := upload.chunks[index]
			if !present {
				return jsonResponse(http.StatusBadRequest, map[string]string{"error": "full-mode upload is incomplete"}), nil
			}
			encoded.WriteString(chunk)
		}
		body, err := base64.RawURLEncoding.DecodeString(encoded.String())
		if err != nil || len(body) > maxPayloadBytes {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid full-mode upload payload"}), nil
		}
		request.Body = body
		request.Headers = request.Headers.Clone()
		request.Headers.Set("Content-Type", contentType)
		return handler(request)
	default:
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid full-mode upload stage"}), nil
	}
}

func (r *pluginRuntime) fullModeRestoreResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	return r.fullModeStagedPayloadResponse(request, maxDatabaseBackupBytes, "application/octet-stream", r.restoreResponse)
}

func (r *pluginRuntime) fullModeSessionResponse() (pluginapi.ManagementResponse, error) {
	token, err := r.createFullModeSession()
	if err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": "could not create full-mode session"}), nil
	}
	return jsonResponse(http.StatusOK, map[string]string{"session": token}), nil
}

func (r *pluginRuntime) fullModeDataResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if !r.validFullModeSession(fullModeSessionFromRequest(request)) {
		return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]string{"error": "storage is not initialized"}), nil
	}
	labels, err := r.store.APIKeyLabels()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]string{"error": err.Error()}), nil
	}
	crypto := r.crypto
	return jsonResponse(http.StatusOK, map[string]any{
		"full_mode":                   true,
		"sensitive_data":              []any{},
		"api_key_tracking_enabled":    crypto.enabled,
		"api_key_uses_default_secret": crypto.enabled && crypto.usesDefaultSecret,
		"api_key_labels":              labels,
	}), nil
}

func (r *pluginRuntime) setAPIKeyLabelResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	if !r.validFullModeSession(fullModeSessionFromRequest(request)) {
		return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
	}
	var input struct {
		Ref   string `json:"ref"`
		Label string `json:"label"`
	}
	if strings.EqualFold(request.Method, http.MethodGet) {
		refs, labels := request.Query["ref"], request.Query["label"]
		if len(refs) == 1 && len(labels) == 1 {
			input.Ref, input.Label = refs[0], labels[0]
		} else {
			body := []byte(request.Headers.Get("X-API-Key-Label"))
			if len(body) == 0 {
				return jsonResponse(http.StatusBadRequest, map[string]string{"error": "API key label query requires exactly one ref and label"}), nil
			}
			if len(body) > 16<<10 || !utf8.Valid(body) || decodeStrictJSON(body, &input) != nil {
				return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid API key label JSON"}), nil
			}
		}
	} else {
		if len(request.Body) > 16<<10 {
			return jsonResponse(http.StatusRequestEntityTooLarge, map[string]string{"error": "API key label JSON is too large"}), nil
		}
		if !utf8.Valid(request.Body) {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "API key label JSON must be valid UTF-8"}), nil
		}
		if err := decodeStrictJSON(request.Body, &input); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]string{"error": "invalid API key label JSON"}), nil
		}
	}
	if err := validateAPIKeyLabel(input.Ref, input.Label); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": err.Error()}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]string{"error": "storage is not initialized"}), nil
	}
	if err := r.store.SetAPIKeyLabel(input.Ref, input.Label); err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]string{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{"saved": true, "ref": input.Ref, "label": input.Label}), nil
}

func (r *pluginRuntime) revokeFullModeSessionResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	token := fullModeSessionFromRequest(request)
	if !r.validFullModeSession(token) {
		return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
	}
	r.revokeFullModeSession(token)
	return jsonResponse(http.StatusOK, map[string]bool{"revoked": true}), nil
}
