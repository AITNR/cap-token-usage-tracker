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
*/
import "C"

import (
	"encoding/json"
	"net/http"
	"time"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// usageRecordJSON is a host-format-agnostic mirror of pluginapi.UsageRecord.
// The host may serialize UsageRecord with PascalCase keys (Go default, no JSON tags)
// or snake_case keys, so we use a raw map to accept both formats.
type usageRecordJSON struct {
	Provider        string
	ExecutorType    string
	Model           string
	Alias           string
	APIKey          string
	AuthID          string
	AuthIndex       string
	AuthType        string
	Source          string
	ReasoningEffort string
	ServiceTier     string
	RequestedAt     time.Time
	Latency         time.Duration
	TTFT            time.Duration
	Failed          bool
	FailureCode     int
	FailureBody     string
	Detail          struct {
		InputTokens         int64
		OutputTokens        int64
		ReasoningTokens     int64
		CachedTokens        int64
		CacheReadTokens     int64
		CacheCreationTokens int64
		TotalTokens         int64
	}
}

// UnmarshalJSON accepts both PascalCase (Go default) and snake_case JSON keys.
func (r *usageRecordJSON) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.Provider = firstString(raw, "Provider", "provider")
	r.ExecutorType = firstString(raw, "ExecutorType", "executor_type")
	r.Model = firstString(raw, "Model", "model")
	r.Alias = firstString(raw, "Alias", "alias")
	r.APIKey = firstString(raw, "APIKey", "ApiKey", "api_key")
	r.AuthID = firstString(raw, "AuthID", "AuthId", "auth_id")
	r.AuthIndex = firstString(raw, "AuthIndex", "auth_index")
	r.AuthType = firstString(raw, "AuthType", "auth_type")
	r.Source = firstString(raw, "Source", "source")
	r.ReasoningEffort = firstString(raw, "ReasoningEffort", "reasoning_effort")
	r.ServiceTier = firstString(raw, "ServiceTier", "service_tier")
	r.Failed = firstBool(raw, "Failed", "failed")

	// RequestedAt
	for _, key := range []string{"RequestedAt", "requested_at"} {
		if v, ok := raw[key]; ok {
			var t time.Time
			if err := json.Unmarshal(v, &t); err == nil {
				r.RequestedAt = t
				break
			}
		}
	}

	// Duration fields: try both int64 (nanoseconds) and string formats
	for _, key := range []string{"Latency", "latency", "latency_ns"} {
		if v, ok := raw[key]; ok {
			var n int64
			if err := json.Unmarshal(v, &n); err == nil {
				r.Latency = time.Duration(n)
				break
			}
		}
	}
	for _, key := range []string{"TTFT", "ttft", "ttft_ns"} {
		if v, ok := raw[key]; ok {
			var n int64
			if err := json.Unmarshal(v, &n); err == nil {
				r.TTFT = time.Duration(n)
				break
			}
		}
	}

	// Failure sub-object
	for _, key := range []string{"Failure", "failure"} {
		if v, ok := raw[key]; ok {
			var f map[string]json.RawMessage
			if err := json.Unmarshal(v, &f); err == nil {
				r.FailureCode = int(firstInt(f, "StatusCode", "status_code"))
				r.FailureBody = firstString(f, "Body", "body")
				break
			}
		}
	}

	// Detail sub-object
	for _, key := range []string{"Detail", "detail"} {
		if v, ok := raw[key]; ok {
			var d map[string]json.RawMessage
			if err := json.Unmarshal(v, &d); err == nil {
				r.Detail.InputTokens = firstInt(d, "InputTokens", "input_tokens")
				r.Detail.OutputTokens = firstInt(d, "OutputTokens", "output_tokens")
				r.Detail.ReasoningTokens = firstInt(d, "ReasoningTokens", "reasoning_tokens")
				r.Detail.CachedTokens = firstInt(d, "CachedTokens", "cached_tokens")
				r.Detail.CacheReadTokens = firstInt(d, "CacheReadTokens", "cache_read_tokens")
				r.Detail.CacheCreationTokens = firstInt(d, "CacheCreationTokens", "cache_creation_tokens")
				r.Detail.TotalTokens = firstInt(d, "TotalTokens", "total_tokens")
				break
			}
		}
	}

	return nil
}

// firstString returns the first matching key's string value from a raw JSON map.
func firstString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				return s
			}
		}
	}
	return ""
}

// firstInt returns the first matching key's int64 value from a raw JSON map.
func firstInt(m map[string]json.RawMessage, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var n int64
			if err := json.Unmarshal(v, &n); err == nil {
				return n
			}
		}
	}
	return 0
}

// firstBool returns the first matching key's bool value from a raw JSON map.
func firstBool(m map[string]json.RawMessage, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var b bool
			if err := json.Unmarshal(v, &b); err == nil {
				return b
			}
		}
	}
	return false
}

// toSDK converts the parsed record to the SDK's UsageRecord type.
func (r *usageRecordJSON) toSDK() pluginapi.UsageRecord {
	return pluginapi.UsageRecord{
		Provider:        r.Provider,
		ExecutorType:    r.ExecutorType,
		Model:           r.Model,
		Alias:           r.Alias,
		APIKey:          r.APIKey,
		AuthID:          r.AuthID,
		AuthIndex:       r.AuthIndex,
		AuthType:        r.AuthType,
		Source:          r.Source,
		ReasoningEffort: r.ReasoningEffort,
		ServiceTier:     r.ServiceTier,
		RequestedAt:     r.RequestedAt,
		Latency:         r.Latency,
		TTFT:            r.TTFT,
		Failed:          r.Failed,
		Failure: pluginapi.UsageFailure{
			StatusCode: r.FailureCode,
			Body:       r.FailureBody,
		},
		Detail: pluginapi.UsageDetail{
			InputTokens:         r.Detail.InputTokens,
			OutputTokens:        r.Detail.OutputTokens,
			ReasoningTokens:     r.Detail.ReasoningTokens,
			CachedTokens:        r.Detail.CachedTokens,
			CacheReadTokens:     r.Detail.CacheReadTokens,
			CacheCreationTokens: r.Detail.CacheCreationTokens,
			TotalTokens:         r.Detail.TotalTokens,
		},
	}
}

// tracker is the global usage data store.
var tracker = NewTracker()

// envelope is the JSON envelope for plugin RPC responses.
type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// registration is the plugin registration payload returned for plugin.register.
type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	UsagePlugin   bool `json:"usage_plugin"`
	ManagementAPI bool `json:"management_api"`
}

// managementRegistrationResponse is returned for management.register.
type managementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
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
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, errHandle := handleMethod(C.GoString(method), requestBytes)
	if errHandle != nil {
		writeResponse(response, errorEnvelope("plugin_error", errHandle.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

// handleMethod dispatches incoming plugin RPC methods.
func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodUsageHandle:
		return handleUsage(request)
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

// pluginRegistration returns the registration payload with usage_plugin and management_api capabilities.
func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "token-usage-tracker",
			Version:          "1.0.0",
			Author:           "router-for-me",
			GitHubRepository: "https://github.com/router-for-me/cap-token-usage-tracker",
		},
		Capabilities: registrationCapability{
			UsagePlugin:   true,
			ManagementAPI: true,
		},
	}
}

// managementRegistration returns the management routes and resources owned by this plugin.
func managementRegistration() managementRegistrationResponse {
	return managementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{
				Method:      http.MethodGet,
				Path:        "/stats",
				Menu:        "Token 统计",
				Description: "以 JSON 格式查看聚合的 Token 用量统计。",
			},
			{
				Method:      http.MethodPost,
				Path:        "/reset",
				Description: "重置所有 Token 用量计数器为零。",
			},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/dashboard",
				Menu:        "Token 用量追踪",
				Description: "实时仪表盘，展示所有模型和提供商的 Token 用量。",
			},
		},
	}
}

// handleUsage processes a usage.handle call by storing the usage record.
func handleUsage(request []byte) ([]byte, error) {
	var raw usageRecordJSON
	if err := json.Unmarshal(request, &raw); err != nil {
		return nil, err
	}
	record := raw.toSDK()
	tracker.RecordUsage(record)
	return okEnvelope(map[string]any{})
}

// okEnvelope wraps a value in a successful JSON envelope.
func okEnvelope(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

// errorEnvelope creates an error JSON envelope.
func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

// writeResponse writes raw bytes into a C cliproxy_buffer.
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
