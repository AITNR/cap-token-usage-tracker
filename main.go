package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const abiVersion uint32 = 1

const (
	pluginName       = "Token Usage Tracker"
	pluginVersion    = "1.0.0"
	pluginAuthor     = "cap"
	pluginGitHub     = "https://github.com/router-for-me/cap-token-usage-tracker"
	pluginConfigJSON = `{"Name":"` + pluginName + `","Version":"` + pluginVersion + `","Author":"` + pluginAuthor + `","GitHubRepository":"` + pluginGitHub + `","Logo":"","ConfigFields":[]}`
	registerCaps     = `{"usage_plugin":true,"management_api":true}`
)

// ---------------------------------------------------------------------------
// JSON envelope types (matching pluginabi.Envelope)
// ---------------------------------------------------------------------------

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable,omitempty"`
	HTTPStatus int    `json:"http_status,omitempty"`
}

// ---------------------------------------------------------------------------
// Usage record types (matching pluginapi.UsageRecord / UsageDetail)
// ---------------------------------------------------------------------------

type usageDetail struct {
	InputTokens         int64 `json:"InputTokens"`
	OutputTokens        int64 `json:"OutputTokens"`
	ReasoningTokens     int64 `json:"ReasoningTokens"`
	CachedTokens        int64 `json:"CachedTokens"`
	CacheReadTokens     int64 `json:"CacheReadTokens"`
	CacheCreationTokens int64 `json:"CacheCreationTokens"`
	TotalTokens         int64 `json:"TotalTokens"`
}

type usageFailure struct {
	StatusCode int    `json:"StatusCode"`
	Body       string `json:"Body"`
}

type usageRecord struct {
	Provider        string        `json:"Provider"`
	ExecutorType    string        `json:"ExecutorType"`
	Model           string        `json:"Model"`
	Alias           string        `json:"Alias"`
	APIKey          string        `json:"APIKey"`
	AuthID          string        `json:"AuthID"`
	AuthIndex       string        `json:"AuthIndex"`
	AuthType        string        `json:"AuthType"`
	Source          string        `json:"Source"`
	ReasoningEffort string        `json:"ReasoningEffort"`
	ServiceTier     string        `json:"ServiceTier"`
	RequestedAt     time.Time     `json:"RequestedAt"`
	Latency         time.Duration `json:"Latency"`
	TTFT            time.Duration `json:"TTFT"`
	Failed          bool          `json:"Failed"`
	Failure         usageFailure  `json:"Failure"`
	Detail          usageDetail   `json:"Detail"`
}

// ---------------------------------------------------------------------------
// Management request/response types (matching pluginapi)
// ---------------------------------------------------------------------------

type managementRegistrationRequest struct {
	Plugin           json.RawMessage `json:"Plugin"`
	BasePath         string          `json:"BasePath"`
	ResourceBasePath string          `json:"ResourceBasePath"`
}

type managementRequest struct {
	Method  string              `json:"Method"`
	Path    string              `json:"Path"`
	Headers map[string][]string `json:"Headers"`
	Query   map[string][]string `json:"Query"`
	Body    []byte              `json:"Body"`
}

// ---------------------------------------------------------------------------
// Stats tracker
// ---------------------------------------------------------------------------

type modelStats struct {
	Model               string  `json:"model"`
	Provider            string  `json:"provider"`
	Requests            int64   `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	ReasoningTokens     int64   `json:"reasoning_tokens"`
	CachedTokens        int64   `json:"cached_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheCreationTokens int64   `json:"cache_creation_tokens"`
	TotalTokens         int64   `json:"total_tokens"`
	TotalLatencyNS      int64   `json:"total_latency_ns"`
	AverageLatencyMS    float64 `json:"avg_latency_ms"`
	FailedRequests      int64   `json:"failed_requests"`
}

type statsResponse struct {
	Summary   *modelStats   `json:"summary"`
	Models    []*modelStats `json:"models"`
	Providers []*modelStats `json:"providers"`
}

type tracker struct {
	mu    sync.RWMutex
	items map[string]*modelStats
}

func newTracker() *tracker {
	return &tracker{items: make(map[string]*modelStats)}
}

func (t *tracker) record(r usageRecord) {
	key := r.Provider + "|" + r.Model
	t.mu.Lock()
	s, ok := t.items[key]
	if !ok {
		s = &modelStats{Model: r.Model, Provider: r.Provider}
		t.items[key] = s
	}
	s.Requests++
	s.InputTokens += r.Detail.InputTokens
	s.OutputTokens += r.Detail.OutputTokens
	s.ReasoningTokens += r.Detail.ReasoningTokens
	s.CachedTokens += r.Detail.CachedTokens
	s.CacheReadTokens += r.Detail.CacheReadTokens
	s.CacheCreationTokens += r.Detail.CacheCreationTokens
	s.TotalTokens += r.Detail.TotalTokens
	s.TotalLatencyNS += int64(r.Latency)
	if s.Requests > 0 {
		s.AverageLatencyMS = float64(s.TotalLatencyNS) / float64(s.Requests) / 1e6
	}
	if r.Failed {
		s.FailedRequests++
	}
	t.mu.Unlock()
}

func (t *tracker) get() statsResponse {
	t.mu.RLock()
	defer t.mu.RUnlock()

	summary := &modelStats{Model: "Total", Provider: "All"}
	models := make([]*modelStats, 0, len(t.items))
	for _, s := range t.items {
		models = append(models, s)
		summary.Requests += s.Requests
		summary.InputTokens += s.InputTokens
		summary.OutputTokens += s.OutputTokens
		summary.ReasoningTokens += s.ReasoningTokens
		summary.CachedTokens += s.CachedTokens
		summary.CacheReadTokens += s.CacheReadTokens
		summary.CacheCreationTokens += s.CacheCreationTokens
		summary.TotalTokens += s.TotalTokens
		summary.TotalLatencyNS += s.TotalLatencyNS
		summary.FailedRequests += s.FailedRequests
	}
	if summary.Requests > 0 {
		summary.AverageLatencyMS = float64(summary.TotalLatencyNS) / float64(summary.Requests) / 1e6
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].Model < models[j].Model
	})

	providerMap := make(map[string]*modelStats)
	for _, s := range t.items {
		p, ok := providerMap[s.Provider]
		if !ok {
			p = &modelStats{Model: s.Provider, Provider: s.Provider}
			providerMap[s.Provider] = p
		}
		p.Requests += s.Requests
		p.InputTokens += s.InputTokens
		p.OutputTokens += s.OutputTokens
		p.ReasoningTokens += s.ReasoningTokens
		p.CachedTokens += s.CachedTokens
		p.CacheReadTokens += s.CacheReadTokens
		p.CacheCreationTokens += s.CacheCreationTokens
		p.TotalTokens += s.TotalTokens
		p.TotalLatencyNS += s.TotalLatencyNS
		p.FailedRequests += s.FailedRequests
	}
	providers := make([]*modelStats, 0, len(providerMap))
	for _, p := range providerMap {
		if p.Requests > 0 {
			p.AverageLatencyMS = float64(p.TotalLatencyNS) / float64(p.Requests) / 1e6
		}
		providers = append(providers, p)
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Provider < providers[j].Provider
	})

	return statsResponse{Summary: summary, Models: models, Providers: providers}
}

func (t *tracker) reset() {
	t.mu.Lock()
	t.items = make(map[string]*modelStats)
	t.mu.Unlock()
}

var globalTracker = newTracker()

// ---------------------------------------------------------------------------
// C ABI exports
// ---------------------------------------------------------------------------

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}

	var rawBody []byte
	if request != nil && requestLen > 0 {
		rawBody = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}

	raw, errHandle := dispatchMethod(C.GoString(method), rawBody)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	globalTracker.reset()
}

// ---------------------------------------------------------------------------
// Method dispatch
// ---------------------------------------------------------------------------

func dispatchMethod(method string, rawBody []byte) ([]byte, error) {
	// Unwrap host JSON envelope to get the inner result payload.
	var payload json.RawMessage
	if len(rawBody) > 0 {
		var env envelope
		if err := json.Unmarshal(rawBody, &env); err == nil && len(env.Result) > 0 {
			payload = env.Result
		} else {
			payload = rawBody
		}
	}

	switch method {
	case "plugin.register", "plugin.reconfigure":
		return handleRegister()
	case "usage.handle":
		return handleUsage(payload)
	case "management.register":
		return handleManagementRegister(payload)
	case "management.handle":
		return handleManagementHandle(payload)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

// ---------------------------------------------------------------------------
// Method handlers
// ---------------------------------------------------------------------------

func handleRegister() ([]byte, error) {
	result := fmt.Sprintf(
		`{"schema_version":1,"metadata":%s,"capabilities":%s}`,
		pluginConfigJSON, registerCaps,
	)
	return okEnvelopeJSON(result)
}

func handleUsage(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return okEnvelopeJSON("{}")
	}
	var rec usageRecord
	if err := json.Unmarshal(payload, &rec); err != nil {
		return okEnvelopeJSON("{}")
	}
	globalTracker.record(rec)
	return okEnvelopeJSON("{}")
}

func handleManagementRegister(payload []byte) ([]byte, error) {
	var req managementRegistrationRequest
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	}

	mgmtBase := strings.TrimRight(req.BasePath, "/")
	if mgmtBase == "" {
		mgmtBase = "/v0/management"
	}
	resBase := strings.TrimRight(req.ResourceBasePath, "/")
	if resBase == "" {
		resBase = "/v0/resource/plugins/token-usage-tracker"
	}

	statsPath := mgmtBase + "/plugins/token-usage-tracker/stats"
	resetPath := mgmtBase + "/plugins/token-usage-tracker/reset"

	result := fmt.Sprintf(
		`{"routes":[`+
			`{"Method":"GET","Path":%q},`+
			`{"Method":"POST","Path":%q}`+
			`],"resources":[`+
			`{"Path":"/dashboard","Menu":"Token Usage","Description":"Token usage dashboard with real-time statistics"}`+
			`]}`,
		statsPath, resetPath,
	)
	return okEnvelopeJSON(result)
}

func handleManagementHandle(payload []byte) ([]byte, error) {
	var req managementRequest
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &req)
	}

	// The host passes the full URL path. Strip known prefixes to get
	// the relative path for routing.
	relPath := extractRelativePath(req.Path)

	switch {
	case relPath == "/dashboard" || relPath == "" || relPath == "/":
		return serveDashboard()
	case relPath == "/stats" && strings.EqualFold(req.Method, "GET"):
		return serveStats()
	case relPath == "/reset" && strings.EqualFold(req.Method, "POST"):
		return serveReset()
	default:
		return managementResponse(http.StatusNotFound, "application/json",
			[]byte(`{"error":"not found"}`))
	}
}

func extractRelativePath(fullPath string) string {
	p := fullPath
	// Strip resource prefix: /v0/resource/plugins/token-usage-tracker
	for _, prefix := range []string{
		"/v0/resource/plugins/token-usage-tracker",
		"/v0/management/plugins/token-usage-tracker",
		"/plugins/token-usage-tracker",
	} {
		if strings.HasPrefix(p, prefix) {
			p = strings.TrimPrefix(p, prefix)
			break
		}
	}
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

func serveStats() ([]byte, error) {
	stats := globalTracker.get()
	body, err := json.Marshal(stats)
	if err != nil {
		return managementResponse(http.StatusInternalServerError, "application/json",
			[]byte(`{"error":"marshal failed"}`))
	}
	return managementResponse(http.StatusOK, "application/json", body)
}

func serveReset() ([]byte, error) {
	globalTracker.reset()
	return managementResponse(http.StatusOK, "application/json",
		[]byte(`{"ok":true,"message":"stats reset"}`))
}

func serveDashboard() ([]byte, error) {
	stats := globalTracker.get()
	statsJSON, _ := json.Marshal(stats)
	html := buildDashboardHTML(string(statsJSON))
	return managementResponse(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

func okEnvelopeJSON(result string) ([]byte, error) {
	return json.Marshal(envelope{OK: true, Result: json.RawMessage(result)})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func managementResponse(statusCode int, contentType string, body []byte) ([]byte, error) {
	result := struct {
		StatusCode int                 `json:"StatusCode"`
		Headers    map[string][]string `json:"Headers"`
		Body       []byte              `json:"Body"`
	}{
		StatusCode: statusCode,
		Headers:    map[string][]string{"Content-Type": {contentType}},
		Body:       body,
	}
	raw, err := json.Marshal(struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result,omitempty"`
	}{OK: true, Result: mustMarshalJSON(result)})
	return raw, err
}

func mustMarshalJSON(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

// callHost invokes a host callback. Reserved for future host callback usage.
func callHost(method string, payload []byte) {
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var req *C.uint8_t
	if len(payload) > 0 {
		req = (*C.uint8_t)(C.CBytes(payload))
		defer C.free(unsafe.Pointer(req))
	}
	if C.call_host_api(cMethod, req, C.size_t(len(payload)), &response) == 0 && response.ptr != nil {
		C.free_host_buffer(response.ptr, response.len)
	}
}
