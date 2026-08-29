package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	pluginID                  string
	statsPath                 string
	resetPath                 string
	backupPath                string
	restorePath               string
	dashboardPath             string
	fullDashboardPath         string
	fullModeSessionPath       string
	fullModeSessionRevokePath string
	fullModeDataPath          string
	fullModeAPIKeyLabelsPath  string
	fullModePricesPath        string
	fullModePricesSavePath    string
	fullModePriceSyncPath     string
	fullModeBackupPath        string
	fullModeRestorePath       string
	fullModeResetPath         string
	resourceStatsPath         string
	resourceStatsInitialPath  string
	resourceStatsTrendPath    string
	resourceStatsGroupsPath   string
	resourceRequestsPath      string
	resourceCostsPath         string
	resourceExchangeRatePath  string
	pricesPath                string
	priceSyncPath             string
	resourcePricesPath        string
	resourcePreferencesPath   string
	resourceUsagePath         string
	quotaPath                 string
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
		pluginID:                  pluginID,
		statsPath:                 "/v0/management/plugins/" + pluginID + "/stats",
		resetPath:                 "/v0/management/plugins/" + pluginID + "/reset",
		backupPath:                "/v0/management/plugins/" + pluginID + "/backup",
		restorePath:               "/v0/management/plugins/" + pluginID + "/restore",
		dashboardPath:             "/v0/resource/plugins/" + pluginID + "/dashboard",
		fullDashboardPath:         "/v0/resource/plugins/" + pluginID + "/full-dashboard",
		fullModeSessionPath:       "/v0/management/plugins/" + pluginID + "/full-mode/session",
		fullModeSessionRevokePath: "/v0/resource/plugins/" + pluginID + "/full-mode/session/revoke",
		fullModeDataPath:          "/v0/resource/plugins/" + pluginID + "/full-mode/data",
		fullModeAPIKeyLabelsPath:  "/v0/resource/plugins/" + pluginID + "/full-mode/api-key-labels",
		fullModePricesPath:        "/v0/resource/plugins/" + pluginID + "/full-mode/prices",
		fullModePricesSavePath:    "/v0/resource/plugins/" + pluginID + "/full-mode/prices/save",
		fullModePriceSyncPath:     "/v0/resource/plugins/" + pluginID + "/full-mode/prices/sync",
		fullModeBackupPath:        "/v0/resource/plugins/" + pluginID + "/full-mode/backup",
		fullModeRestorePath:       "/v0/resource/plugins/" + pluginID + "/full-mode/restore",
		fullModeResetPath:         "/v0/resource/plugins/" + pluginID + "/full-mode/reset",
		resourceStatsPath:         "/v0/resource/plugins/" + pluginID + "/stats",
		resourceStatsInitialPath:  "/v0/resource/plugins/" + pluginID + "/stats/initial",
		resourceStatsTrendPath:    "/v0/resource/plugins/" + pluginID + "/stats/trends",
		resourceStatsGroupsPath:   "/v0/resource/plugins/" + pluginID + "/stats/groups",
		resourceRequestsPath:      "/v0/resource/plugins/" + pluginID + "/requests",
		resourceCostsPath:         "/v0/resource/plugins/" + pluginID + "/costs",
		resourceExchangeRatePath:  "/v0/resource/plugins/" + pluginID + "/exchange-rate",
		pricesPath:                "/v0/management/plugins/" + pluginID + "/prices",
		priceSyncPath:             "/v0/management/plugins/" + pluginID + "/prices/sync",
		resourcePricesPath:        "/v0/resource/plugins/" + pluginID + "/prices",
		resourcePreferencesPath:   "/v0/resource/plugins/" + pluginID + "/preferences",
		resourceUsagePath:         "/v0/resource/plugins/" + pluginID + "/v1/usage",
		quotaPath:                 "/v0/resource/plugins/" + pluginID + "/quota",
	}
	r.mu.Lock()
	r.routes = routes
	r.mu.Unlock()

	return managementRegistrationResponse{
		Routes: []pluginapi.ManagementRoute{
			{
				Method:      http.MethodPost,
				Path:        "/plugins/" + pluginID + "/full-mode/session",
				Description: "Issue a short-lived capability for the separate full-mode dashboard.",
			},
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
			{Path: "/full-dashboard", Description: "Full-mode dashboard shell without protected data."},
			{Path: "/full-mode/data", Description: "Capability-protected full-mode data."},
			{Path: "/full-mode/api-key-labels", Description: "Capability-protected API key label management. Send JSON in the X-API-Key-Label header with GET requests."},
			{Path: "/full-mode/session/revoke", Description: "Revoke a full-mode capability."},
			{Path: "/full-mode/prices", Description: "Capability-protected model prices."},
			{Path: "/full-mode/prices/save", Description: "Capability-protected model price save."},
			{Path: "/full-mode/prices/sync", Description: "Capability-protected model price synchronization."},
			{Path: "/full-mode/backup", Description: "Capability-protected database backup download."},
			{Path: "/full-mode/restore", Description: "Capability-protected database backup restore."},
			{Path: "/full-mode/reset", Description: "Capability-protected statistics reset."},
			{Path: "/stats", Description: "Read full token usage statistics for compatible clients."},
			{Path: "/stats/initial", Description: "Read compact first-screen token usage statistics."},
			{Path: "/stats/trends", Description: "Read downsampled per-model token trends."},
			{Path: "/stats/groups", Description: "Read detailed dimension statistics."},
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
			{
				Path:        "/v1/usage",
				Description: "sub2api-compatible user usage: tracked usage amount (USD) versus a 999999 total quota.",
			},
			{
				Path:        "/quota",
				Description: "Read or set the /v1/usage quota limit in USD (GET ?set=<value>).",
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
	config := r.config
	r.mu.RUnlock()
	response, err := r.dispatchManagement(request, routes)
	if err != nil {
		return pluginapi.ManagementResponse{}, err
	}
	return maybeCompressResponse(request, response, config), nil
}

func (r *pluginRuntime) dispatchManagement(request pluginapi.ManagementRequest, routes registeredRoutes) (pluginapi.ManagementResponse, error) {
	if routes.pluginID == "" {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "management routes are not registered"}), nil
	}

	switch request.Path {
	case routes.fullModeSessionPath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		return r.fullModeSessionResponse()
	case routes.dashboardPath:
		if request.Method != "" && !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return dashboardResponse(), nil
	case routes.fullDashboardPath:
		if request.Method != "" && !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return fullDashboardResponse(), nil
	case routes.fullModeDataPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.fullModeDataResponse(request)
	case routes.fullModeAPIKeyLabelsPath:
		// Resource routes are dispatched by the host as GET. Keep PUT support
		// for direct/plugin-level callers and newer hosts.
		if !strings.EqualFold(request.Method, http.MethodGet) && !strings.EqualFold(request.Method, http.MethodPut) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.setAPIKeyLabelResponse(request)
	case routes.fullModeSessionRevokePath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.revokeFullModeSessionResponse(request)
	case routes.fullModePricesPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		if !r.validFullModeSession(fullModeSessionFromRequest(request)) {
			return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
		}
		return r.pricesResponse()
	case routes.fullModePricesSavePath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.fullModeStagedPayloadResponse(request, 2<<20, "application/json", r.savePricesResponse)
	case routes.fullModePriceSyncPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.fullModeStagedPayloadResponse(request, 2<<20, "application/json", r.syncPricesResponse)
	case routes.fullModeBackupPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		if !r.validFullModeSession(fullModeSessionFromRequest(request)) {
			return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
		}
		return r.backupResponse()
	case routes.fullModeRestorePath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.fullModeRestoreResponse(request)
	case routes.fullModeResetPath:
		if !strings.EqualFold(request.Method, http.MethodPost) {
			return methodNotAllowed(http.MethodPost), nil
		}
		if !r.validFullModeSession(fullModeSessionFromRequest(request)) {
			return jsonResponse(http.StatusUnauthorized, map[string]string{"error": "full-mode session is missing or expired"}), nil
		}
		return r.resetResponse(request)
	case routes.statsPath, routes.resourceStatsPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.statsResponse(request)
	case routes.resourceStatsInitialPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.initialStatsResponse(request)
	case routes.resourceStatsTrendPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.statsTrendResponse(request)
	case routes.resourceStatsGroupsPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.groupsStatsResponse(request)
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
	case routes.resourceUsagePath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.sub2apiUsageResponse(request)
	case routes.quotaPath:
		if !strings.EqualFold(request.Method, http.MethodGet) {
			return methodNotAllowed(http.MethodGet), nil
		}
		return r.quotaResponse(request)
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
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	apiKeyIdentities, err := apiKeyIdentitiesFromRequest(request, fullMode, r.store)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	stats, err := r.store.queryStatsByFilter(queryRange, newUsageFilterFromIdentities(request.Query.Get("source"), apiKeyIdentities))
	if err != nil {
		status := errorHTTPStatus(err)
		return jsonResponse(status, map[string]any{"error": err.Error()}), nil
	}
	_, generations := r.store.APIKeyCryptoState()
	return r.sensitiveJSONResponse(http.StatusOK, &stats, fullMode, r.crypto, generations), nil
}

func (r *pluginRuntime) statsFilter(request pluginapi.ManagementRequest, fullMode bool) (usageRange, usageFilter, error) {
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return usageRange{}, usageFilter{}, err
	}
	if r.store == nil {
		return usageRange{}, usageFilter{}, withStatus(http.StatusServiceUnavailable, "storage is not initialized")
	}
	apiKeyIdentities, err := apiKeyIdentitiesFromRequest(request, fullMode, r.store)
	if err != nil {
		return usageRange{}, usageFilter{}, err
	}
	return queryRange, newUsageFilterFromIdentities(request.Query.Get("source"), apiKeyIdentities), nil
}

func (r *pluginRuntime) initialStatsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
	r.mu.RLock()
	defer r.mu.RUnlock()
	queryRange, filter, err := r.statsFilter(request, fullMode)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	stats, err := r.store.queryInitialStatsByFilter(queryRange, filter)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	_, generations := r.store.APIKeyCryptoState()
	return r.sensitiveJSONResponse(http.StatusOK, &stats, fullMode, r.crypto, generations), nil
}

func (r *pluginRuntime) statsTrendResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
	r.mu.RLock()
	defer r.mu.RUnlock()
	queryRange, filter, err := r.statsFilter(request, fullMode)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	stats, err := r.store.queryStatsTrendByFilter(queryRange, filter)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, stats), nil
}

func (r *pluginRuntime) groupsStatsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
	r.mu.RLock()
	defer r.mu.RUnlock()
	queryRange, filter, err := r.statsFilter(request, fullMode)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	filter.Model = normalizeDimension(request.Query.Get("model"))
	excludedModels := make(map[string]struct{}, len(request.Query["exclude_model"]))
	for _, model := range request.Query["exclude_model"] {
		if model = normalizeDimension(model); model != "" {
			excludedModels[model] = struct{}{}
		}
	}
	stats, err := r.store.queryGroupsByFilter(queryRange, filter)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	if len(excludedModels) > 0 {
		items := stats.Items[:0]
		for _, item := range stats.Items {
			if _, excluded := excludedModels[compactModelName(item.Model)]; !excluded {
				items = append(items, item)
			}
		}
		stats.Items = items
		stats.Total = len(items)
	}
	if err := sortGroupStats(stats.Items, request.Query.Get("sort"), request.Query.Get("direction")); err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	offset, err := parseNonNegativeQueryInt(request.Query.Get("offset"), 0, "offset")
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	limit, err := parseNonNegativeQueryInt(request.Query.Get("limit"), defaultRequestPageSize, "limit")
	if err != nil || limit < 1 || limit > maxDashboardPageSize {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "limit must be an integer between 1 and 500"}), nil
	}
	if offset > stats.Total {
		offset = stats.Total
	}
	end := offset + limit
	if end > stats.Total {
		end = stats.Total
	}
	stats.Items = stats.Items[offset:end]
	_, generations := r.store.APIKeyCryptoState()
	return r.sensitiveJSONResponse(http.StatusOK, &stats, fullMode, r.crypto, generations), nil
}

func sortGroupStats(items []GroupStats, sortKey, direction string) error {
	if sortKey == "" {
		sortKey = "total_tokens"
	}
	if direction == "" {
		direction = "desc"
	}
	if direction != "asc" && direction != "desc" {
		return withStatus(http.StatusBadRequest, "direction must be asc or desc")
	}
	numeric := map[string]bool{"requests": true, "failed_requests": true, "input_tokens": true, "output_tokens": true, "reasoning_tokens": true, "cache_read_tokens": true, "cache_creation_tokens": true, "total_tokens": true, "average_latency_ns": true, "average_ttft_ns": true}
	text := map[string]bool{"model": true, "provider": true, "api_key": true, "alias": true, "source": true, "executor_type": true, "auth_type": true, "service_tier": true, "reasoning_effort": true}
	if !numeric[sortKey] && !text[sortKey] {
		return withStatus(http.StatusBadRequest, "unsupported group sort %q", sortKey)
	}
	value := func(item GroupStats) string {
		switch sortKey {
		case "model":
			return compactModelName(item.Model)
		case "provider":
			return item.Provider
		case "api_key":
			return item.APIKeyHash
		case "alias":
			return item.Alias
		case "source":
			return item.Source
		case "executor_type":
			return item.ExecutorType
		case "auth_type":
			return item.AuthType
		case "service_tier":
			return item.ServiceTier
		case "reasoning_effort":
			return item.ReasoningEffort
		default:
			return ""
		}
	}
	number := func(item GroupStats) uint64 {
		switch sortKey {
		case "requests":
			return item.Requests
		case "failed_requests":
			return item.FailedRequests
		case "input_tokens":
			return item.InputTokens
		case "output_tokens":
			return item.OutputTokens
		case "reasoning_tokens":
			return item.ReasoningTokens
		case "cache_read_tokens":
			return item.CacheReadTokens
		case "cache_creation_tokens":
			return item.CacheCreationTokens
		case "total_tokens":
			return item.TotalTokens
		case "average_latency_ns":
			return item.AverageLatencyNS
		case "average_ttft_ns":
			return item.AverageTTFTNS
		default:
			return 0
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		if numeric[sortKey] {
			less = number(items[i]) < number(items[j])
		} else {
			less = value(items[i]) < value(items[j])
		}
		if numeric[sortKey] && number(items[i]) == number(items[j]) || !numeric[sortKey] && value(items[i]) == value(items[j]) {
			return compareDimensions(items[i].Dimensions, items[j].Dimensions) < 0
		}
		if direction == "desc" {
			return !less
		}
		return less
	})
	return nil
}

func (r *pluginRuntime) requestsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
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
	apiKeyIdentities, err := apiKeyIdentitiesFromRequest(request, fullMode, r.store)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	page, err := r.store.queryRequestPageByFilter(queryRange, offset, limit, request.Query.Get("model"), newUsageFilterFromIdentities(request.Query.Get("source"), apiKeyIdentities), request.Query.Get("result"))
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	_, generations := r.store.APIKeyCryptoState()
	return r.sensitiveJSONResponse(http.StatusOK, &page, fullMode, r.crypto, generations), nil
}

func (r *pluginRuntime) costsResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	fullMode := r.validFullModeSession(fullModeSessionFromRequest(request))
	queryRange, err := usageRangeFromQuery(request.Query.Get("range"), request.Query.Get("start"), request.Query.Get("end"), time.Now().UTC())
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	apiKeyIdentities, err := apiKeyIdentitiesFromRequest(request, fullMode, r.store)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	costs, err := r.store.queryCostsByFilter(queryRange, newUsageFilterFromIdentities(request.Query.Get("source"), apiKeyIdentities))
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	_, generations := r.store.APIKeyCryptoState()
	return r.sensitiveJSONResponse(http.StatusOK, &costs, fullMode, r.crypto, generations), nil
}

// sub2apiUsageResponse mimics the sub2api GET /v1/usage protocol (unrestricted
// wallet mode) using the plugin's own tracked usage statistics. The plugin has
// no real quota, so it reports a fixed total quota of 999999 and the current
// tracked usage amount (USD) as "used": used/999999.
func (r *pluginRuntime) sub2apiUsageResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	rangeValue := request.Query.Get("range")
	if rangeValue == "" {
		rangeValue = "retention"
	}
	now := time.Now().UTC()
	queryRange, err := usageRangeFromQuery(rangeValue, request.Query.Get("start"), request.Query.Get("end"), now)
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	todayRange := usageRange{Name: "custom", Start: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), End: now}

	r.mu.RLock()
	if r.store == nil {
		r.mu.RUnlock()
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	stats, errStats := r.store.queryInitialStatsByFilter(queryRange, usageFilter{})
	costs, errCosts := r.store.queryCostsByFilter(queryRange, usageFilter{})
	todayStats, errTodayStats := r.store.queryInitialStatsByFilter(todayRange, usageFilter{})
	todayCosts, errTodayCosts := r.store.queryCostsByFilter(todayRange, usageFilter{})
	r.mu.RUnlock()
	if errStats != nil {
		return jsonResponse(errorHTTPStatus(errStats), map[string]any{"error": errStats.Error()}), nil
	}
	if errCosts != nil {
		return jsonResponse(errorHTTPStatus(errCosts), map[string]any{"error": errCosts.Error()}), nil
	}
	if errTodayStats != nil {
		todayStats = InitialStatsResponse{}
	}
	if errTodayCosts != nil {
		todayCosts = CostResponse{}
	}

	used := costs.Summary.TotalUSD
	r.mu.RLock()
	totalQuota := r.effectiveQuotaLocked()
	r.mu.RUnlock()
	remaining := totalQuota - used
	if remaining < 0 {
		remaining = 0
	}

	// usage.total / usage.today（sub2api 字段名）
	aggFrom := func(s InitialStatsResponse, c CostResponse) map[string]any {
		return map[string]any{
			"requests":             s.Summary.Requests,
			"input_tokens":         s.Summary.InputTokens,
			"output_tokens":        s.Summary.OutputTokens,
			"cache_read_tokens":    s.Summary.CacheReadTokens,
			"cache_creation_tokens": s.Summary.CacheCreationTokens,
			"total_tokens":         s.Summary.TotalTokens,
			"cost":                 c.Summary.TotalUSD,
			"actual_cost":          c.Summary.TotalUSD,
		}
	}
	usage := map[string]any{
		"total":              aggFrom(stats, costs),
		"today":              aggFrom(todayStats, todayCosts),
		"average_duration_ms": avgDurationMS(stats.Summary),
		"rpm":                0,
		"tpm":                0,
	}

	// daily_usage：按天聚合 Series（token）+ costs Series（金额）
	type dailyAgg struct {
		Counters
		cost float64
	}
	dayMap := make(map[string]*dailyAgg)
	addCounters := func(day string, c Counters) {
		e, ok := dayMap[day]
		if !ok {
			e = &dailyAgg{}
			dayMap[day] = e
		}
		e.Counters.add(c)
	}
	for _, sp := range stats.Series {
		if t, errParse := time.Parse(time.RFC3339, sp.Hour); errParse == nil {
			addCounters(t.Format("2006-01-02"), sp.Counters)
		}
	}
	for _, cp := range costs.Series {
		if t, errParse := time.Parse(time.RFC3339, cp.Hour); errParse == nil {
			day := t.Format("2006-01-02")
			if e, ok := dayMap[day]; ok {
				e.cost += cp.TotalUSD
			}
		}
	}
	type dailyEntry struct {
		Date             string  `json:"date"`
		Requests         uint64  `json:"requests"`
		InputTokens      uint64  `json:"input_tokens"`
		OutputTokens     uint64  `json:"output_tokens"`
		CacheReadTokens  uint64  `json:"cache_read_tokens"`
		CacheWriteTokens uint64  `json:"cache_write_tokens"`
		TotalTokens      uint64  `json:"total_tokens"`
		Cost             float64 `json:"cost"`
		ActualCost       float64 `json:"actual_cost"`
	}
	dailyUsage := make([]dailyEntry, 0, len(dayMap))
	for day, e := range dayMap {
		dailyUsage = append(dailyUsage, dailyEntry{
			Date:             day,
			Requests:         e.Requests,
			InputTokens:      e.InputTokens,
			OutputTokens:     e.OutputTokens,
			CacheReadTokens:  e.CacheReadTokens,
			CacheWriteTokens: e.CacheCreationTokens,
			TotalTokens:      e.TotalTokens,
			Cost:             e.cost,
			ActualCost:       e.cost,
		})
	}
	sort.Slice(dailyUsage, func(i, j int) bool { return dailyUsage[i].Date < dailyUsage[j].Date })

	// model_stats：per-model token（stats.Models）+ cost（costs.Models）
	modelCost := make(map[string]float64, len(costs.Models))
	for _, m := range costs.Models {
		modelCost[m.Model] = m.TotalUSD
	}
	type modelStatEntry struct {
		Model               string  `json:"model"`
		Requests            uint64  `json:"requests"`
		InputTokens         uint64  `json:"input_tokens"`
		OutputTokens        uint64  `json:"output_tokens"`
		CacheCreationTokens uint64  `json:"cache_creation_tokens"`
		CacheReadTokens     uint64  `json:"cache_read_tokens"`
		TotalTokens         uint64  `json:"total_tokens"`
		Cost                float64 `json:"cost"`
		ActualCost          float64 `json:"actual_cost"`
		AccountCost         float64 `json:"account_cost"`
	}
	modelStats := make([]modelStatEntry, 0, len(stats.Models))
	for _, m := range stats.Models {
		cost := modelCost[m.Model]
		modelStats = append(modelStats, modelStatEntry{
			Model:               m.Model,
			Requests:            m.Requests,
			InputTokens:         m.InputTokens,
			OutputTokens:        m.OutputTokens,
			CacheCreationTokens: m.CacheCreationTokens,
			CacheReadTokens:     m.CacheReadTokens,
			TotalTokens:         m.TotalTokens,
			Cost:                cost,
			ActualCost:          cost,
			AccountCost:         cost,
		})
	}
	sort.Slice(modelStats, func(i, j int) bool { return modelStats[i].Cost > modelStats[j].Cost })

	return jsonResponse(http.StatusOK, map[string]any{
		"mode":        "unrestricted",
		"isValid":     true,
		"planName":    "CAP Token Usage Tracker",
		"unit":        "USD",
		"quota":       totalQuota,
		"used":        used,
		"remaining":   remaining,
		"balance":     remaining,
		"range":       queryRange.Name,
		"usage":       usage,
		"daily_usage": dailyUsage,
		"model_stats": modelStats,
	}), nil
}

// avgDurationMS returns the average request duration in milliseconds from a
// Counters aggregate (TotalLatencyNS / LatencySamples), or 0 when unknown.
func avgDurationMS(c Counters) float64 {
	if c.LatencySamples == 0 {
		return 0
	}
	return float64(c.TotalLatencyNS) / float64(c.LatencySamples) / 1e6
}

const defaultQuotaLimitUSD = 999999.0

// quotaResponse reads or sets the /v1/usage quota limit (USD). GET returns the
// current limit; GET ?set=<value> persists a new limit to a small JSON file in
// the plugin data directory (survives restarts) and updates the runtime value.
func (r *pluginRuntime) quotaResponse(request pluginapi.ManagementRequest) (pluginapi.ManagementResponse, error) {
	setValue := strings.TrimSpace(request.Query.Get("set"))
	if setValue == "" {
		r.mu.RLock()
		quota := r.effectiveQuotaLocked()
		r.mu.RUnlock()
		return jsonResponse(http.StatusOK, map[string]any{"quota": quota}), nil
	}
	value, err := strconv.ParseFloat(setValue, 64)
	if err != nil || value < 0 || value > 1e12 {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "set must be a non-negative number (USD)"}), nil
	}
	r.mu.Lock()
	r.quotaLimitUSD = value
	path := r.quotaFilePathLocked()
	r.mu.Unlock()
	if path != "" {
		if err := saveQuotaToFile(path, value); err != nil {
			return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
		}
	}
	return jsonResponse(http.StatusOK, map[string]any{"quota": value}), nil
}

// quotaFilePathLocked returns the quota.json path next to the plugin database.
// Callers must hold r.mu.
func (r *pluginRuntime) quotaFilePathLocked() string {
	if r.config.DataPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.config.DataPath), "quota.json")
}

// effectiveQuotaLocked returns the current quota limit, defaulting to 999999.
// Callers must hold r.mu.
func (r *pluginRuntime) effectiveQuotaLocked() float64 {
	if r.quotaLimitUSD > 0 {
		return r.quotaLimitUSD
	}
	return defaultQuotaLimitUSD
}

// loadQuotaLimitLocked loads the persisted quota limit from quota.json when
// present. Callers must hold r.mu (write lock).
func (r *pluginRuntime) loadQuotaLimitLocked() {
	path := r.quotaFilePathLocked()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if value, errParse := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); errParse == nil && value > 0 && value <= 1e12 {
		r.quotaLimitUSD = value
	}
}

func saveQuotaToFile(path string, value float64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.FormatFloat(value, 'f', 2, 64)), 0o600)
}

func apiKeyIdentitiesFromRequest(request pluginapi.ManagementRequest, fullMode bool, store *Store) ([]string, error) {
	refs := nonEmptyQueryValues(request.Query["api_key_ref"])
	hashes := nonEmptyQueryValues(request.Query["api_key_hash"])
	if len(refs) == 0 && len(hashes) == 0 {
		return nil, nil
	}
	if !fullMode {
		return nil, withStatus(http.StatusForbidden, "API key filtering requires a full-mode session")
	}
	if len(refs) > 0 && len(hashes) > 0 {
		return nil, withStatus(http.StatusBadRequest, "api_key_ref and api_key_hash cannot be used together")
	}
	if len(refs) > 0 {
		seen := make(map[string]struct{}, len(refs))
		identities := make([]string, 0, len(refs))
		for _, ref := range refs {
			if _, _, ok := parseAPIKeyRef(ref); !ok {
				return nil, withStatus(http.StatusBadRequest, "api_key_ref is invalid")
			}
			if _, exists := seen[ref]; exists {
				continue
			}
			seen[ref] = struct{}{}
			identities = append(identities, ref)
		}
		return identities, nil
	}
	if len(hashes) > 1 {
		return nil, withStatus(http.StatusBadRequest, "api_key_hash cannot be repeated; use api_key_ref")
	}
	hash := hashes[0]
	if !validAPIKeyHash(hash) {
		return nil, withStatus(http.StatusBadRequest, "api_key_hash must be 32 lowercase hexadecimal characters")
	}
	if store == nil {
		return nil, withStatus(http.StatusServiceUnavailable, "storage is not initialized")
	}
	resolved, err := store.ResolveAPIKeyHash(hash)
	if err != nil {
		return nil, err
	}
	return []string{resolved}, nil
}

func nonEmptyQueryValues(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}
	return filtered
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
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	store := r.store
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	if err := store.RestoreBackup(request.Body); err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	generation, generations := store.APIKeyCryptoState()
	if r.store == store {
		r.apiKeyGeneration = generation
		r.apiKeyGenerations = generations
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
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	store := r.store
	if store == nil {
		return jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "storage is not initialized"}), nil
	}
	if err := store.Reset(); err != nil {
		return jsonResponse(http.StatusInternalServerError, map[string]any{"error": err.Error()}), nil
	}
	generation, generations := store.APIKeyCryptoState()
	if r.store == store {
		r.apiKeyGeneration = generation
		r.apiKeyGenerations = generations
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
