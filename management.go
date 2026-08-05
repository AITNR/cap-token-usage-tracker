package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type managementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}

type registeredRoutes struct {
	pluginID                 string
	statsPath                string
	resetPath                string
	backupPath               string
	restorePath              string
	dashboardPath            string
	resourceStatsPath        string
	resourceRequestsPath     string
	resourceCostsPath        string
	resourceExchangeRatePath string
	pricesPath               string
	priceSyncPath            string
	resourcePricesPath       string
	resourcePreferencesPath  string
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
		pluginID:                 pluginID,
		statsPath:                "/v0/management/plugins/" + pluginID + "/stats",
		resetPath:                "/v0/management/plugins/" + pluginID + "/reset",
		backupPath:               "/v0/management/plugins/" + pluginID + "/backup",
		restorePath:              "/v0/management/plugins/" + pluginID + "/restore",
		dashboardPath:            "/v0/resource/plugins/" + pluginID + "/dashboard",
		resourceStatsPath:        "/v0/resource/plugins/" + pluginID + "/stats",
		resourceRequestsPath:     "/v0/resource/plugins/" + pluginID + "/requests",
		resourceCostsPath:        "/v0/resource/plugins/" + pluginID + "/costs",
		resourceExchangeRatePath: "/v0/resource/plugins/" + pluginID + "/exchange-rate",
		pricesPath:               "/v0/management/plugins/" + pluginID + "/prices",
		priceSyncPath:            "/v0/management/plugins/" + pluginID + "/prices/sync",
		resourcePricesPath:       "/v0/resource/plugins/" + pluginID + "/prices",
		resourcePreferencesPath:  "/v0/resource/plugins/" + pluginID + "/preferences",
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
				Description: "Persist per-model input, output, cache, and context-tier token prices.",
			},
			{
				Method:      http.MethodPost,
				Path:        "/plugins/" + pluginID + "/prices/sync",
				Description: "Synchronize CLIProxyAPI model prices from models.dev.",
			},
			{
				Method:      http.MethodGet,
				Path:        "/plugins/" + pluginID + "/backup",
				Description: "Download a full bbolt database backup of persisted token usage data.",
			},
			{
				Method:      http.MethodPost,
				Path:        "/plugins/" + pluginID + "/restore",
				Description: "Restore the persisted token usage database from a previous backup.",
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
				Path:        "/costs",
				Description: "Read exact per-request-derived estimated cost statistics.",
			},
			{
				Path:        "/exchange-rate",
				Description: "Read the cached latest USD to CNY exchange rate for dashboard display.",
			},
			{
				Path:        "/prices",
				Description: "Read persisted model token prices for the plugin dashboard.",
			},
			{
				Path:        "/preferences",
				Description: "Read and persist dashboard table preferences.",
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
	case routes.resourceCostsPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.costsResponse(request)
	case routes.resourceExchangeRatePath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.exchangeRateResponse()
	case routes.resourcePricesPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.pricesResponse()
	case routes.resourcePreferencesPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.preferencesResponse(request)
	case routes.pricesPath:
		if !strings.EqualFold(request.Method, http.MethodPut) {
			return methodNotAllowed(http.MethodPut), nil
		}
		return r.savePricesResponse(request)
	case routes.priceSyncPath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		return r.syncPricesResponse(request)
	case routes.resetPath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		return r.resetResponse(request)
	case routes.backupPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.backupResponse()
	case routes.restorePath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		return r.restoreResponse(request)
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
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	stats, err := r.store.queryStatsBySource(queryRange, request.Query.Get("source"))
	if err != nil {
		status := errorHTTPStatus(err)
		return jsonResponse(status, map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, stats), nil
}

func (r *pluginRuntime) requestsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
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
	page, err := r.store.queryRequestPageBySource(queryRange, offset, limit, request.Query.Get("model"), request.Query.Get("source"), request.Query.Get("result"))
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, page), nil
}

func (r *pluginRuntime) costsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	costs, err := store.queryCostsBySource(queryRange, request.Query.Get("source"))
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, costs), nil
}

func (r *pluginRuntime) exchangeRateResponse() (pluginapi.ManagementResponse, error) {
	r.mu.Lock()
	service := r.exchangeRates
	if service == nil {
		service = newExchangeRateService()
		r.exchangeRates = service
	}
	r.mu.Unlock()
	rate, err := service.latest()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, rate), nil
}

func (r *pluginRuntime) pricesResponse() (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	priceBook, err := store.QueryPriceBook()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, priceBook), nil
}

// Plugin resource routes are dispatched by the host as GET-only. The save=1
// query form persists this small, non-sensitive dashboard preference payload.
func (r *pluginRuntime) preferencesResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	if request.Query.Get("save") == "" {
		if len(request.Query) != 0 {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "save must be 1 when preference values are supplied"}), nil
		}
		preferences, err := store.QueryDashboardPreferences()
		if err != nil {
			return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
		}
		return jsonResponse(http.StatusOK, preferences), nil
	}
	preferences, err := dashboardPreferencesFromQuery(request.Query)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	preferences, err = store.SaveDashboardPreferences(preferences)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, preferences), nil
}

func dashboardPreferencesFromQuery(query map[string][]string) (DashboardPreferences, error) {
	allowed := map[string]struct{}{
		"save": {}, "request_page_size": {}, "dimension_page_size": {},
		"hidden_request_column": {}, "hidden_dimension_column": {}, "time_range_mode": {},
		"time_range_start": {}, "time_range_end": {},
	}
	for key := range query {
		if _, ok := allowed[key]; !ok {
			return DashboardPreferences{}, withStatus(http.StatusBadRequest, "unsupported dashboard preference query parameter %q", key)
		}
	}
	if values := query["save"]; len(values) != 1 || values[0] != "1" {
		return DashboardPreferences{}, withStatus(http.StatusBadRequest, "save must be 1")
	}
	requestPageSize, err := parseDashboardPageSize(query, "request_page_size")
	if err != nil {
		return DashboardPreferences{}, err
	}
	dimensionPageSize, err := parseDashboardPageSize(query, "dimension_page_size")
	if err != nil {
		return DashboardPreferences{}, err
	}
	timeRangeMode, err := optionalDashboardPreference(query, "time_range_mode")
	if err != nil {
		return DashboardPreferences{}, err
	}
	timeRangeStart, err := optionalDashboardPreference(query, "time_range_start")
	if err != nil {
		return DashboardPreferences{}, err
	}
	timeRangeEnd, err := optionalDashboardPreference(query, "time_range_end")
	if err != nil {
		return DashboardPreferences{}, err
	}
	return DashboardPreferences{
		RequestPageSize:        requestPageSize,
		DimensionPageSize:      dimensionPageSize,
		HiddenRequestColumns:   append([]string{}, query["hidden_request_column"]...),
		HiddenDimensionColumns: append([]string{}, query["hidden_dimension_column"]...),
		TimeRangeMode:          timeRangeMode,
		TimeRangeStart:         timeRangeStart,
		TimeRangeEnd:           timeRangeEnd,
	}, nil
}

func optionalDashboardPreference(query map[string][]string, name string) (string, error) {
	values := query[name]
	if len(values) > 1 {
		return "", withStatus(http.StatusBadRequest, "%s must be supplied at most once", name)
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

func parseDashboardPageSize(query map[string][]string, name string) (int, error) {
	values := query[name]
	if len(values) != 1 {
		return 0, withStatus(http.StatusBadRequest, "%s must be supplied exactly once", name)
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || value < 1 || value > maxDashboardPageSize {
		return 0, withStatus(http.StatusBadRequest, "%s must be an integer between 1 and %d", name, maxDashboardPageSize)
	}
	return value, nil
}

func (r *pluginRuntime) savePricesResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType, _, err := mime.ParseMediaType(request.Headers.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"}), nil
	}
	if len(request.Body) > 2<<20 {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]any{"error": "model prices JSON is too large"}), nil
	}
	var input struct {
		Prices       map[string]ModelPrice `json:"prices"`
		SyncSettings *PriceSyncSettings    `json:"sync_settings,omitempty"`
	}
	if err := decodeStrictJSON(request.Body, &input); err != nil || input.Prices == nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid model prices JSON"}), nil
	}
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	priceBook, err := store.SavePriceBook(input.Prices, input.SyncSettings)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, priceBook), nil
}

func (r *pluginRuntime) syncPricesResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType, _, err := mime.ParseMediaType(request.Headers.Get("Content-Type"))
	if err != nil || !strings.EqualFold(contentType, "application/json") {
		return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/json"}), nil
	}
	if len(request.Body) > 2<<20 {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]any{"error": "model price synchronization JSON is too large"}), nil
	}
	var input struct {
		Source       string             `json:"source"`
		Models       []string           `json:"models"`
		SyncSettings *PriceSyncSettings `json:"sync_settings,omitempty"`
	}
	if err := decodeStrictJSON(request.Body, &input); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid model price synchronization JSON"}), nil
	}
	if input.Source != "" && input.Source != priceSourceModelsDev {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": `source must be "models.dev"`}), nil
	}
	priceBook, err := r.syncModelsDev(input.SyncSettings, input.Models)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, priceBook), nil
}

func decodeStrictJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
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

func (r *pluginRuntime) backupResponse() (pluginapi.ManagementResponse, error) {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	data, err := store.Backup()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	filename := "cap-token-usage-tracker-" + nowUTC().UTC().Format("20060102-150405") + ".db"
	return pluginapi.ManagementResponse{
		StatusCode: http.StatusOK,
		Headers: http.Header{
			"Content-Type":           []string{"application/octet-stream"},
			"Content-Disposition":    []string{`attachment; filename="` + filename + `"`},
			"Cache-Control":          []string{"no-store"},
			"X-Content-Type-Options": []string{"nosniff"},
		},
		Body: data,
	}, nil
}

func (r *pluginRuntime) restoreResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	contentType := strings.TrimSpace(request.Headers.Get("Content-Type"))
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.EqualFold(mediaType, "application/octet-stream") {
			return jsonResponse(http.StatusUnsupportedMediaType, map[string]any{"error": "Content-Type must be application/octet-stream"}), nil
		}
	}
	if request.Headers.Get("X-Confirm-Restore") != "replace" {
		return jsonResponse(http.StatusBadRequest, map[string]string{"error": "missing X-Confirm-Restore: replace header"}), nil
	}
	if len(request.Body) == 0 {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "backup body must not be empty"}), nil
	}
	if len(request.Body) > maxDatabaseBackupBytes {
		return jsonResponse(http.StatusRequestEntityTooLarge, map[string]any{"error": "backup body is too large"}), nil
	}
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	if err := store.RestoreBackup(request.Body); err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, map[string]any{
		"restored":    true,
		"restored_at": nowUTC(),
	}), nil
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
