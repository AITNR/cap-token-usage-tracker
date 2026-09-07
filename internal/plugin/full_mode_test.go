package plugin

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestFullModeSessionProtectsFullModeResources(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	crypto, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, crypto: crypto}
	defer runtime.shutdown()
	raw, err := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}

	dashboardRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullDashboardPath})
	response, err := runtime.handleManagement(dashboardRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full dashboard shell: %+v, %v", response, err)
	}
	if string(response.Body) == "" || containsSensitiveFullModePayload(response.Body) {
		t.Fatal("full dashboard shell must not include protected data")
	}

	dataRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeDataPath})
	response, err = runtime.handleManagement(dataRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing full-mode session response: %+v, %v", response, err)
	}
	pricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModePricesPath})
	response, err = runtime.handleManagement(pricesRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing full-mode session for prices response: %+v, %v", response, err)
	}

	sessionRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeSessionPath})
	response, err = runtime.handleManagement(sessionRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode session response: %+v, %v", response, err)
	}
	var payload struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil || len(payload.Session) < 40 {
		t.Fatal("full-mode session response must contain an opaque token")
	}

	validRequest, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    runtime.routes.fullModeDataPath,
		Headers: http.Header{"X-Full-Mode-Session": []string{payload.Session}},
	})
	response, err = runtime.handleManagement(validRequest)
	if err != nil || response.StatusCode != http.StatusOK || !containsSensitiveFullModePayload(response.Body) {
		t.Fatalf("valid full-mode session response: %+v, %v", response, err)
	}
	revokeRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeSessionRevokePath, Headers: http.Header{"X-Full-Mode-Session": []string{payload.Session}}})
	response, err = runtime.handleManagement(revokeRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode session revoke response: %+v, %v", response, err)
	}
	response, err = runtime.handleManagement(validRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked full-mode session response: %+v, %v", response, err)
	}

	response, err = runtime.handleManagement(sessionRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("replacement full-mode session response: %+v, %v", response, err)
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		t.Fatal(err)
	}
	validRequest, _ = json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    runtime.routes.fullModeDataPath,
		Headers: http.Header{"X-Full-Mode-Session": []string{payload.Session}},
	})

	token, err := base64.RawURLEncoding.DecodeString(payload.Session)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(token)
	runtime.fullModeMu.Lock()
	runtime.fullModeSessions[hash] = fullModeSession{expiresAt: time.Now().UTC().Add(-time.Second)}
	runtime.fullModeMu.Unlock()
	response, err = runtime.handleManagement(validRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired full-mode session response: %+v, %v", response, err)
	}
}

func TestFullModeSessionUsesConfiguredTTL(t *testing.T) {
	runtime := &pluginRuntime{config: Config{FullModeSessionTTLMinutes: 2}}
	first := nowUTC()
	oldNow := nowUTC
	nowUTC = func() time.Time { return first }
	defer func() { nowUTC = oldNow }()

	token, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(decoded)
	runtime.fullModeMu.Lock()
	session := runtime.fullModeSessions[hash]
	runtime.fullModeMu.Unlock()
	if got := session.expiresAt.Sub(first); got != 2*time.Minute {
		t.Fatalf("configured session TTL = %s, want 2m", got)
	}
}

func TestFullModeDefaultSecretWarningTracksSuccessfulReconfigure(t *testing.T) {
	config := testConfig(t)
	runtime := &pluginRuntime{}
	defer runtime.shutdown()
	lifecyclePayload := func(secret string) []byte {
		configYAML := []byte("data_path: " + filepath.ToSlash(config.DataPath) + "\napi_key_secret: " + strconv.Quote(secret) + "\n")
		payload, err := json.Marshal(lifecycleRequest{ConfigYAML: configYAML, SchemaVersion: 3})
		if err != nil {
			t.Fatal(err)
		}
		return payload
	}
	if _, err := runtime.register(lifecyclePayload(defaultAPIKeySecret)); err != nil {
		t.Fatal(err)
	}
	registration, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(registration); err != nil {
		t.Fatal(err)
	}
	session, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	request, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    runtime.routes.fullModeDataPath,
		Headers: http.Header{"X-Full-Mode-Session": []string{session}},
	})

	response, err := runtime.handleManagement(request)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"api_key_uses_default_secret":true`) {
		t.Fatalf("default-secret full-mode data = %+v, %v", response, err)
	}

	customSecret := strings.Repeat("custom-secret-", 3)
	if _, err := runtime.reconfigure(lifecyclePayload(customSecret)); err != nil {
		t.Fatal(err)
	}
	response, err = runtime.handleManagement(request)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"api_key_uses_default_secret":false`) {
		t.Fatalf("custom-secret full-mode data = %+v, %v", response, err)
	}
}

func TestFullModeStagedPriceSaveUsesGETResourceRequests(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}
	sessionRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeSessionPath})
	response, err := runtime.handleManagement(sessionRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode session response: %+v, %v", response, err)
	}
	var session struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(response.Body, &session); err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"prices":{"full-mode-test":{"input":1.5,"output":6}}}`))
	baseHeaders := http.Header{"X-Full-Mode-Session": []string{session.Session}}

	beginRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModePricesSavePath, Headers: baseHeaders, Query: map[string][]string{"stage": {"begin"}, "chunks": {"1"}}})
	response, err = runtime.handleManagement(beginRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode upload begin response: %+v, %v", response, err)
	}
	var upload struct {
		Upload string `json:"upload"`
	}
	if err := json.Unmarshal(response.Body, &upload); err != nil || upload.Upload == "" {
		t.Fatalf("full-mode upload begin payload: %s, %v", response.Body, err)
	}

	chunkHeaders := baseHeaders.Clone()
	chunkHeaders.Set("X-Full-Mode-Payload", payload)
	chunkRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModePricesSavePath, Headers: chunkHeaders, Query: map[string][]string{"stage": {"chunk"}, "upload": {upload.Upload}, "index": {"0"}}})
	response, err = runtime.handleManagement(chunkRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode upload chunk response: %+v, %v", response, err)
	}
	commitRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModePricesSavePath, Headers: baseHeaders, Query: map[string][]string{"stage": {"commit"}, "upload": {upload.Upload}}})
	response, err = runtime.handleManagement(commitRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"full-mode-test"`) {
		t.Fatalf("full-mode upload commit response: %+v, %v", response, err)
	}
}

func TestFullModeBackupAndRestoreRequireSession(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}
	backupRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeBackupPath})
	response, err := runtime.handleManagement(backupRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized full-mode backup response: %+v, %v", response, err)
	}
	restoreRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeRestorePath, Query: map[string][]string{"stage": {"begin"}, "chunks": {"1"}}})
	response, err = runtime.handleManagement(restoreRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized full-mode restore response: %+v, %v", response, err)
	}

	sessionRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeSessionPath})
	response, err = runtime.handleManagement(sessionRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode session response: %+v, %v", response, err)
	}
	var session struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(response.Body, &session); err != nil || session.Session == "" {
		t.Fatalf("full-mode session payload: %s, %v", response.Body, err)
	}

	headers := http.Header{"X-Full-Mode-Session": []string{session.Session}}
	backupRequest, _ = json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeBackupPath, Headers: headers})
	response, err = runtime.handleManagement(backupRequest)
	if err != nil || response.StatusCode != http.StatusOK || response.Headers.Get("Content-Type") != "application/octet-stream" || len(response.Body) == 0 {
		t.Fatalf("authorized full-mode backup response: %+v, %v", response, err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(response.Body)
	chunkCount := (len(encoded) + fullModeUploadChunkSize - 1) / fullModeUploadChunkSize
	restoreRequest, _ = json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeRestorePath, Headers: headers, Query: map[string][]string{"stage": {"begin"}, "chunks": {strconv.Itoa(chunkCount)}}})
	response, err = runtime.handleManagement(restoreRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode restore begin response: %+v, %v", response, err)
	}
	var upload struct {
		Upload string `json:"upload"`
	}
	if err := json.Unmarshal(response.Body, &upload); err != nil || upload.Upload == "" {
		t.Fatalf("full-mode restore begin payload: %s, %v", response.Body, err)
	}
	for index := 0; index < chunkCount; index++ {
		chunkHeaders := headers.Clone()
		start := index * fullModeUploadChunkSize
		end := start + fullModeUploadChunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunkHeaders.Set("X-Full-Mode-Payload", encoded[start:end])
		chunkRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeRestorePath, Headers: chunkHeaders, Query: map[string][]string{"stage": {"chunk"}, "upload": {upload.Upload}, "index": {strconv.Itoa(index)}}})
		response, err = runtime.handleManagement(chunkRequest)
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("full-mode restore chunk %d response: %+v, %v", index, response, err)
		}
	}
	commitHeaders := headers.Clone()
	commitHeaders.Set("X-Confirm-Restore", "replace")
	commitRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeRestorePath, Headers: commitHeaders, Query: map[string][]string{"stage": {"commit"}, "upload": {upload.Upload}}})
	response, err = runtime.handleManagement(commitRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"restored":true`) {
		t.Fatalf("full-mode restore commit response: %+v, %v", response, err)
	}
}

func TestFullModeResetRequiresSession(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "reset-model"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 5}}); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"confirm":"reset"}`)
	headers := http.Header{"Content-Type": []string{"application/json"}}

	unauthorized, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeResetPath, Headers: headers.Clone(), Body: body})
	response, err := runtime.handleManagement(unauthorized)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing session reset response: %+v, %v", response, err)
	}

	badSessionHeaders := headers.Clone()
	badSessionHeaders.Set("X-Full-Mode-Session", "bogus-session-token")
	badSessionRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeResetPath, Headers: badSessionHeaders, Body: body})
	response, err = runtime.handleManagement(badSessionRequest)
	if err != nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid session reset response: %+v, %v", response, err)
	}

	getRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeResetPath})
	response, err = runtime.handleManagement(getRequest)
	if err != nil || response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method reset response: %+v, %v", response, err)
	}

	session, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	authorizedHeaders := headers.Clone()
	authorizedHeaders.Set("X-Full-Mode-Session", session)
	authorized, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeResetPath, Headers: authorizedHeaders, Body: body})
	response, err = runtime.handleManagement(authorized)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"reset":true`) {
		t.Fatalf("authorized reset response: %+v, %v", response, err)
	}

	statsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: map[string][]string{"range": {"24h"}}})
	response, err = runtime.handleManagement(statsRequest)
	if err != nil || response.StatusCode != http.StatusOK || strings.Contains(string(response.Body), "reset-model") {
		t.Fatalf("stats after reset: %+v, %v", response, err)
	}

	managementReset, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: headers.Clone(), Body: body})
	response, err = runtime.handleManagement(managementReset)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("management reset route must remain available: %+v, %v", response, err)
	}
}

func containsSensitiveFullModePayload(body []byte) bool {
	var payload struct {
		FullMode bool `json:"full_mode"`
	}
	return json.Unmarshal(body, &payload) == nil && payload.FullMode
}

func TestFullModeUploadChunkLimitMatchesPayloadLimit(t *testing.T) {
	const twoMiB = 2 << 20
	const sixtyFourMiB = 64 << 20
	if got := fullModeUploadChunkLimit(twoMiB); got != 467 {
		t.Fatalf("2 MiB upload chunk limit = %d, want 467", got)
	}
	if got := fullModeUploadChunkLimit(sixtyFourMiB); got != 14914 {
		t.Fatalf("64 MiB upload chunk limit = %d, want 14914", got)
	}

	runtime := &pluginRuntime{config: Config{}}
	session, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	headers := http.Header{"X-Full-Mode-Session": []string{session}}
	for _, test := range []struct {
		name            string
		maxPayloadBytes int
		chunks          int
		wantStatus      int
	}{
		{name: "price boundary", maxPayloadBytes: twoMiB, chunks: 467, wantStatus: http.StatusOK},
		{name: "price above boundary", maxPayloadBytes: twoMiB, chunks: 468, wantStatus: http.StatusBadRequest},
		{name: "restore boundary", maxPayloadBytes: sixtyFourMiB, chunks: 14914, wantStatus: http.StatusOK},
		{name: "restore above boundary", maxPayloadBytes: sixtyFourMiB, chunks: 14915, wantStatus: http.StatusBadRequest},
	} {
		request := pluginapi.ManagementRequest{Headers: headers, Query: map[string][]string{"stage": {"begin"}, "chunks": {strconv.Itoa(test.chunks)}}}
		response, err := runtime.fullModeStagedPayloadResponse(request, test.maxPayloadBytes, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			return jsonResponse(http.StatusOK, map[string]bool{"handled": true}), nil
		})
		if err != nil || response.StatusCode != test.wantStatus {
			t.Fatalf("%s response = %+v, err = %v, want status %d", test.name, response, err, test.wantStatus)
		}
	}
}

func TestFullModeUploadSessionAndRuntimeLimits(t *testing.T) {
	runtime := &pluginRuntime{config: Config{}}
	session, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	begin := func(chunks int) pluginapi.ManagementResponse {
		request := pluginapi.ManagementRequest{
			Headers: http.Header{"X-Full-Mode-Session": []string{session}},
			Query:   map[string][]string{"stage": {"begin"}, "chunks": {strconv.Itoa(chunks)}},
		}
		response, err := runtime.fullModeStagedPayloadResponse(request, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			return jsonResponse(http.StatusOK, nil), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	for i := 0; i < fullModeUploadsPerSession; i++ {
		if response := begin(1); response.StatusCode != http.StatusOK {
			t.Fatalf("session upload %d response = %+v, want 200", i, response)
		}
	}
	if response := begin(1); response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("session upload limit response = %+v, want 429", response)
	}

	runtime = &pluginRuntime{config: Config{}}
	for sessionIndex := 0; sessionIndex < fullModeUploadsTotal; sessionIndex++ {
		session, err := runtime.createFullModeSession()
		if err != nil {
			t.Fatal(err)
		}
		request := pluginapi.ManagementRequest{
			Headers: http.Header{"X-Full-Mode-Session": []string{session}},
			Query:   map[string][]string{"stage": {"begin"}, "chunks": {"1"}},
		}
		response, err := runtime.fullModeStagedPayloadResponse(request, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			return jsonResponse(http.StatusOK, nil), nil
		})
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("runtime upload %d response = %+v, err = %v, want 200", sessionIndex, response, err)
		}
	}
	session, err = runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	request := pluginapi.ManagementRequest{
		Headers: http.Header{"X-Full-Mode-Session": []string{session}},
		Query:   map[string][]string{"stage": {"begin"}, "chunks": {"1"}},
	}
	response, err := runtime.fullModeStagedPayloadResponse(request, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
		return jsonResponse(http.StatusOK, nil), nil
	})
	if err != nil || response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("runtime upload limit response = %+v, err = %v, want 429", response, err)
	}
}

func TestFullModeUploadReservedByteLimit(t *testing.T) {
	runtime := &pluginRuntime{config: Config{}}
	for i := 0; i < 2; i++ {
		session, err := runtime.createFullModeSession()
		if err != nil {
			t.Fatal(err)
		}
		request := pluginapi.ManagementRequest{
			Headers: http.Header{"X-Full-Mode-Session": []string{session}},
			Query:   map[string][]string{"stage": {"begin"}, "chunks": {strconv.Itoa(fullModeUploadChunkLimit(maxDatabaseBackupBytes))}},
		}
		response, err := runtime.fullModeStagedPayloadResponse(request, maxDatabaseBackupBytes, "application/octet-stream", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
			return jsonResponse(http.StatusOK, nil), nil
		})
		wantStatus := http.StatusOK
		if i == 1 {
			wantStatus = http.StatusTooManyRequests
		}
		if err != nil || response.StatusCode != wantStatus {
			t.Fatalf("reserved-byte upload %d response = %+v, err = %v, want %d", i, response, err, wantStatus)
		}
	}
}

func TestFullModeUploadExpiryAndRevokeCleanup(t *testing.T) {
	runtime := &pluginRuntime{config: Config{}}
	session, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	request := pluginapi.ManagementRequest{
		Headers: http.Header{"X-Full-Mode-Session": []string{session}},
		Query:   map[string][]string{"stage": {"begin"}, "chunks": {"1"}},
	}
	response, err := runtime.fullModeStagedPayloadResponse(request, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
		return jsonResponse(http.StatusOK, nil), nil
	})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial upload response = %+v, err = %v", response, err)
	}
	runtime.fullModeMu.Lock()
	for id := range runtime.fullModeUploads {
		upload := runtime.fullModeUploads[id]
		upload.expiresAt = nowUTC().Add(-time.Second)
		runtime.fullModeUploads[id] = upload
	}
	runtime.fullModeMu.Unlock()

	request.Query = map[string][]string{"stage": {"begin"}, "chunks": {"1"}}
	response, err = runtime.fullModeStagedPayloadResponse(request, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
		return jsonResponse(http.StatusOK, nil), nil
	})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("upload after expiry purge response = %+v, err = %v", response, err)
	}
	runtime.fullModeMu.Lock()
	activeAfterExpiry := len(runtime.fullModeUploads)
	runtime.fullModeMu.Unlock()
	if activeAfterExpiry != 1 {
		t.Fatalf("active uploads after expiry purge = %d, want 1", activeAfterExpiry)
	}

	runtime.revokeFullModeSession(session)
	runtime.fullModeMu.Lock()
	activeAfterRevoke := len(runtime.fullModeUploads)
	runtime.fullModeMu.Unlock()
	if activeAfterRevoke != 0 {
		t.Fatalf("active uploads after revoke = %d, want 0", activeAfterRevoke)
	}
}

func TestFullModeUploadRejectsCrossSessionAccess(t *testing.T) {
	runtime := &pluginRuntime{config: Config{}}
	owner, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	other, err := runtime.createFullModeSession()
	if err != nil {
		t.Fatal(err)
	}
	beginRequest := pluginapi.ManagementRequest{
		Headers: http.Header{"X-Full-Mode-Session": []string{owner}},
		Query:   map[string][]string{"stage": {"begin"}, "chunks": {"1"}},
	}
	response, err := runtime.fullModeStagedPayloadResponse(beginRequest, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
		return jsonResponse(http.StatusOK, nil), nil
	})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("owner upload begin response = %+v, err = %v", response, err)
	}
	var payload struct {
		Upload string `json:"upload"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil || payload.Upload == "" {
		t.Fatalf("owner upload begin payload = %s, err = %v", response.Body, err)
	}
	chunkRequest := pluginapi.ManagementRequest{
		Headers: http.Header{"X-Full-Mode-Session": []string{other}, "X-Full-Mode-Payload": []string{"QQ"}},
		Query:   map[string][]string{"stage": {"chunk"}, "upload": {payload.Upload}, "index": {"0"}},
	}
	response, err = runtime.fullModeStagedPayloadResponse(chunkRequest, 2<<20, "application/json", func(pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
		return jsonResponse(http.StatusOK, nil), nil
	})
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-session chunk response = %+v, err = %v, want 400", response, err)
	}
}
