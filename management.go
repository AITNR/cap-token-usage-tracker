package main

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type managementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

type registeredRoutes struct {
	pluginID             string
	statsPath            string
	resetPath            string
	dashboardPath        string
	resourceStatsPath    string
	resourceRequestsPath string
	pricesPath           string
	resourcePricesPath   string
}

func (r *pluginRuntime) registerManagement(raw []byte) (managementRegistrationResponse, error) {
	var request pluginapi.ManagementRegistrationRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return managementRegistrationResponse{}, withStatus(400, "decode management registration: %v", err)
	}
	pluginID, err := pluginIDFromResourceBase(request.ResourceBasePath)
	if err != nil {
		return managementRegistrationResponse{}, err
	}

	routes := registeredRoutes{
		pluginID:             pluginID,
		statsPath:            "/v0/management/plugins/" + pluginID + "/stats",
		resetPath:            "/v0/management/plugins/" + pluginID + "/reset",
		dashboardPath:        "/v0/resource/plugins/" + pluginID + "/dashboard",
		resourceStatsPath:    "/v0/resource/plugins/" + pluginID + "/stats",
		resourceRequestsPath: "/v0/resource/plugins/" + pluginID + "/requests",
		pricesPath:           "/v0/management/plugins/" + pluginID + "/prices",
		resourcePricesPath:   "/v0/resource/plugins/" + pluginID + "/prices",
	}
	r.mu.Lock()
	r.routes = routes
	r.mu.Unlock()

	return managementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{
				Method:      http.MethodGet,
				Path:        "/plugins/" + pluginID + "/stats",
				Description: "Read aggregated token usage statistics.",
			},
			{
				Method:      http.MethodPost,
				Path:        "/plugins/" + pluginID + "/reset",
				Description: "Reset all persisted token usage statistics.",
			},
			{
				Method:      http.MethodPut,
				Path:        "/plugins/" + pluginID + "/prices",
				Description: "Persist per-model input and output token prices.",
			},
		},
		Resources: []pluginapi.ResourceRoute{
			{
				Path:        "/dashboard",
				Menu:        "Token 用量",
				Description: "查看持久化的 Token 用量、请求和延迟统计。",
			},
			{
				Path:        "/stats",
				Description: "Read-only token usage statistics for the plugin dashboard.",
			},
			{
				Path:        "/requests",
				Description: "Read paginated per-request token usage details.",
			},
			{
				Path:        "/prices",
				Description: "Read persisted model token prices for the plugin dashboard.",
			},
		},
	}, nil
}

func (r *pluginRuntime) handleManagement(raw []byte) (pluginapi.ManagementResponse, error) {
	var request pluginapi.ManagementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return pluginapi.ManagementResponse{}, withStatus(400, "decode management request: %v", err)
	}

	r.mu.RLock()
	routes := r.routes
	r.mu.RUnlock()
	if routes.pluginID == "" {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "management routes are not registered"}), nil
	}

	switch request.Path {
	case routes.dashboardPath:
		if request.Method != "" && !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return dashboardResponse(), nil
	case routes.statsPath, routes.resourceStatsPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.statsResponse(request)
	case routes.resourceRequestsPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.requestsResponse(request)
	case routes.resourcePricesPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.pricesResponse()
	case routes.pricesPath:
		if !strings.EqualFold(request.Method, http.MethodPut) {
			return methodNotAllowed(http.MethodPut), nil
		}
		return r.savePricesResponse(request)
	case routes.resetPath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		return r.resetResponse(request)
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "route not found"}), nil
	}
}

func (r *pluginRuntime) statsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	stats, err := r.store.Query(request.Query.Get("range"))
	if err != nil {
		status := errorHTTPStatus(err)
		return jsonResponse(status, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, stats), nil
}

func (r *pluginRuntime) requestsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	offset, err := parseNonNegativeQueryInt(request.Query.Get("offset"), 0, "offset")
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	limit, err := parseNonNegativeQueryInt(request.Query.Get("limit"), defaultRequestPageSize, "limit")
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	page, err := r.store.QueryRequests(request.Query.Get("range"), offset, limit, request.Query.Get("model"))
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, page), nil
}

func (r *pluginRuntime) pricesResponse() (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	prices, err := r.store.QueryModelPrices()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, ModelPricesResponse{Prices: prices}), nil
}

func (r *pluginRuntime) savePricesResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType, _, err := mime.ParseMediaType(request.Headers.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"}), nil
	}
	var input ModelPricesResponse
	if err := json.Unmarshal(request.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid model prices JSON"}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	prices, err := r.store.SaveModelPrices(input.Prices)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, ModelPricesResponse{Prices: prices}), nil
}

func parseNonNegativeQueryInt(raw string, fallback int, name string) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, withStatus(http.StatusBadRequest, "%s must be a non-negative integer", name)
	}
	return value, nil
}

func (r *pluginRuntime) resetResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType, _, err := mime.ParseMediaType(request.Headers.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"}), nil
	}
	var confirmation struct {
		Confirm string `json:"confirm"`
	}
	if err := json.Unmarshal(request.Body, &confirmation); err != nil || confirmation.Confirm != "reset" {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": `body must be {"confirm":"reset"}`}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	if err := r.store.Reset(); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"reset":    true,
		"reset_at": nowUTC(),
	}), nil
}

func pluginIDFromResourceBase(base string) (string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	const prefix = "/v0/resource/plugins/"
	if !strings.HasPrefix(base, prefix) {
		return "", withStatus(400, "invalid resource base path %q", base)
	}
	pluginID := strings.TrimPrefix(base, prefix)
	if strings.Contains(pluginID, "/") || !pluginIDPattern.MatchString(pluginID) {
		return "", withStatus(400, "invalid plugin ID in resource base path")
	}
	return pluginID, nil
}

func methodNotAllowed(allowed string) pluginapi.ManagementResponse {
	response := jsonResponse(http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	response.Headers.Set("Allow", allowed)
	return response
}

func jsonResponse(status int, value any) pluginapi.ManagementResponse {
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))
	}
	return pluginapi.ManagementResponse{
		StatusCode: status,
		Headers: http.Header{
			"Content-Type":           []string{"application/json; charset=utf-8"},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
		},
		Body: body,
	}
}
