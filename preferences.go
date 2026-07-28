package main

import "fmt"

const (
	defaultDashboardPageSize = 100
	maxDashboardPageSize     = 500
)

type DashboardPreferences struct {
	RequestPageSize        int      `json:"request_page_size"`
	DimensionPageSize      int      `json:"dimension_page_size"`
	HiddenRequestColumns   []string `json:"hidden_request_columns"`
	HiddenDimensionColumns []string `json:"hidden_dimension_columns"`
}

var requestColumnKeys = []string{
	"time", "model", "source", "service_tier", "result", "ttft_ns", "generation_ns", "tps",
	"reasoning_effort", "input_tokens", "output_tokens", "reasoning_tokens", "cache_read_tokens",
	"cache_creation_tokens", "total_tokens", "cache_hit", "estimated_cost", "price_source",
}

var dimensionColumnKeys = []string{
	"model", "provider", "alias", "source", "executor_type", "auth_type", "service_tier",
	"reasoning_effort", "requests", "failed_requests", "input_tokens", "output_tokens",
	"reasoning_tokens", "cache_read_tokens", "cache_creation_tokens", "total_tokens",
	"average_latency_ns", "average_ttft_ns",
}

func defaultDashboardPreferences() DashboardPreferences {
	return DashboardPreferences{
		RequestPageSize:        defaultDashboardPageSize,
		DimensionPageSize:      defaultDashboardPageSize,
		HiddenRequestColumns:   []string{},
		HiddenDimensionColumns: []string{},
	}
}

func normalizeDashboardPreferences(input DashboardPreferences) (DashboardPreferences, error) {
	if input.RequestPageSize < 1 || input.RequestPageSize > maxDashboardPageSize {
		return DashboardPreferences{}, fmt.Errorf("request_page_size must be between 1 and %d", maxDashboardPageSize)
	}
	if input.DimensionPageSize < 1 || input.DimensionPageSize > maxDashboardPageSize {
		return DashboardPreferences{}, fmt.Errorf("dimension_page_size must be between 1 and %d", maxDashboardPageSize)
	}
	hiddenRequests, err := normalizeHiddenColumns(input.HiddenRequestColumns, requestColumnKeys, "hidden_request_columns")
	if err != nil {
		return DashboardPreferences{}, err
	}
	hiddenDimensions, err := normalizeHiddenColumns(input.HiddenDimensionColumns, dimensionColumnKeys, "hidden_dimension_columns")
	if err != nil {
		return DashboardPreferences{}, err
	}
	return DashboardPreferences{
		RequestPageSize:        input.RequestPageSize,
		DimensionPageSize:      input.DimensionPageSize,
		HiddenRequestColumns:   hiddenRequests,
		HiddenDimensionColumns: hiddenDimensions,
	}, nil
}

func normalizeHiddenColumns(input, allowed []string, field string) ([]string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	hiddenSet := make(map[string]struct{}, len(input))
	for _, key := range input {
		if _, ok := allowedSet[key]; !ok {
			return nil, fmt.Errorf("%s contains unsupported column %q", field, key)
		}
		hiddenSet[key] = struct{}{}
	}
	if len(hiddenSet) >= len(allowed) {
		return nil, fmt.Errorf("%s must leave at least one column visible", field)
	}
	normalized := make([]string, 0, len(hiddenSet))
	for _, key := range allowed {
		if _, ok := hiddenSet[key]; ok {
			normalized = append(normalized, key)
		}
	}
	return normalized, nil
}

func cloneDashboardPreferences(value DashboardPreferences) DashboardPreferences {
	value.HiddenRequestColumns = append([]string{}, value.HiddenRequestColumns...)
	value.HiddenDimensionColumns = append([]string{}, value.HiddenDimensionColumns...)
	return value
}
