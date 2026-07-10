package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// handleManagement processes a management.handle call by routing the request
// to the appropriate handler based on the request path.
func handleManagement(request []byte) ([]byte, error) {
	var req pluginapi.ManagementRequest
	if err := json.Unmarshal(request, &req); err != nil {
		return errorEnvelope("invalid_request", "failed to parse management request: "+err.Error()), nil
	}

	relPath := stripPluginPrefix(req.Path)

	var resp pluginapi.ManagementResponse
	switch {
	case relPath == "/dashboard" || relPath == "/" || relPath == "":
		resp = handleDashboard(req)
	case relPath == "/stats":
		resp = handleStats(req)
	case relPath == "/reset":
		resp = handleReset(req)
	default:
		resp = pluginapi.ManagementResponse{
			StatusCode: http.StatusNotFound,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       errorEnvelope("not_found", "unknown route: "+relPath),
		}
	}

	return okEnvelope(resp)
}

// stripPluginPrefix removes the standard CLIProxyAPI route prefix and plugin ID
// from a full path, returning the relative route path.
//
// Full paths look like:
//   - /v0/resource/plugins/<pluginID>/dashboard
//   - /v0/management/plugins/<pluginID>/stats
//
// The result is the relative path after the plugin ID segment.
func stripPluginPrefix(path string) string {
	parts := strings.Split(path, "/")
	// ["", "v0", "resource"|"management", "plugins", "<pluginID>", ...relative...]
	if len(parts) >= 6 {
		return "/" + strings.Join(parts[5:], "/")
	}
	if len(parts) == 5 {
		return "/"
	}
	return path
}

// handleStats returns aggregated token usage statistics as JSON.
func handleStats(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	stats := tracker.GetStats()
	body, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return pluginapi.ManagementResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       errorEnvelope("marshal_error", err.Error()),
		}
	}
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       body,
	}
}

// handleReset clears all usage counters and returns a confirmation response.
func handleReset(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	tracker.Reset()
	confirmation := map[string]any{
		"reset":   true,
		"message": "All token usage counters have been reset.",
	}
	body, _ := json.MarshalIndent(confirmation, "", "  ")
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       body,
	}
}
