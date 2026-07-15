package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// TokenStats mirrors upstream usage.Detail field semantics for one request.
type TokenStats struct {
	InputTokens         int64 `json:"input_tokens"`
	OutputTokens        int64 `json:"output_tokens"`
	ReasoningTokens     int64 `json:"reasoning_tokens"`
	CachedTokens        int64 `json:"cached_tokens"`
	CacheReadTokens     int64 `json:"cache_read_tokens"`
	CacheCreationTokens int64 `json:"cache_creation_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
}

// Record is the complete persisted shape for one upstream request. Its fields
// intentionally match the reference plugin's SQLite row model.
type Record struct {
	ID                string
	Timestamp         time.Time
	APIKey            string
	Provider          string
	Model             string
	Alias             string
	Source            string
	AuthID            string
	AuthIndex         string
	AuthType          string
	ExecutorType      string
	ReasoningEffort   string
	ServiceTier       string
	LatencyMs         int64
	TTFTMs            int64
	Tokens            TokenStats
	Failed            bool
	FailureStatusCode int
	FailureBody       string
}

// UsageRequestDetail is the protected raw-usage API shape. APIKey and Model are
// represented by the surrounding APIUsage map keys, matching the reference.
type UsageRequestDetail struct {
	ID                string     `json:"id"`
	Timestamp         time.Time  `json:"timestamp"`
	Provider          string     `json:"provider,omitempty"`
	Model             string     `json:"model,omitempty"`
	Alias             string     `json:"alias,omitempty"`
	Source            string     `json:"source"`
	AuthID            string     `json:"auth_id,omitempty"`
	AuthIndex         string     `json:"auth_index"`
	AuthType          string     `json:"auth_type,omitempty"`
	ExecutorType      string     `json:"executor_type,omitempty"`
	ReasoningEffort   string     `json:"reasoning_effort"`
	ServiceTier       string     `json:"service_tier"`
	LatencyMs         int64      `json:"latency_ms"`
	TTFTMs            int64      `json:"ttft_ms"`
	Tokens            TokenStats `json:"tokens"`
	Failed            bool       `json:"failed"`
	FailureStatusCode int        `json:"failure_status_code,omitempty"`
	FailureBody       string     `json:"failure_body,omitempty"`
}

type APIUsage map[string]map[string][]UsageRequestDetail

type QueryRange struct {
	Start *time.Time
	End   *time.Time
}

type DeleteResult struct {
	Deleted int64    `json:"deleted"`
	Missing []string `json:"missing"`
}

// decodeUsage uses the SDK's UsageRecord type directly so field acquisition is
// kept aligned with the host ABI instead of maintaining a partial JSON parser.
func decodeUsage(raw []byte, now time.Time) (Record, error) {
	var usage pluginapi.UsageRecord
	if err := json.Unmarshal(raw, &usage); err != nil {
		return Record{}, fmt.Errorf("decode usage record: %w", err)
	}
	return toRecord(usage, now), nil
}

func toRecord(usage pluginapi.UsageRecord, now time.Time) Record {
	timestamp := usage.RequestedAt
	if timestamp.IsZero() {
		timestamp = now
	}
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	return Record{
		ID:              uuid.NewString(),
		Timestamp:       timestamp.UTC(),
		APIKey:          strings.TrimSpace(usage.APIKey),
		Provider:        strings.TrimSpace(usage.Provider),
		Model:           normalizeModel(usage.Model),
		Alias:           strings.TrimSpace(usage.Alias),
		Source:          strings.TrimSpace(usage.Source),
		AuthID:          strings.TrimSpace(usage.AuthID),
		AuthIndex:       strings.TrimSpace(usage.AuthIndex),
		AuthType:        strings.TrimSpace(usage.AuthType),
		ExecutorType:    strings.TrimSpace(usage.ExecutorType),
		ReasoningEffort: strings.TrimSpace(usage.ReasoningEffort),
		ServiceTier:     strings.TrimSpace(usage.ServiceTier),
		LatencyMs:       durationToMilliseconds(usage.Latency),
		TTFTMs:          durationToMilliseconds(usage.TTFT),
		Tokens: TokenStats{
			InputTokens:         usage.Detail.InputTokens,
			OutputTokens:        usage.Detail.OutputTokens,
			ReasoningTokens:     usage.Detail.ReasoningTokens,
			CachedTokens:        usage.Detail.CachedTokens,
			CacheReadTokens:     usage.Detail.CacheReadTokens,
			CacheCreationTokens: usage.Detail.CacheCreationTokens,
			TotalTokens:         usage.Detail.TotalTokens,
		},
		Failed:            usage.Failed || usage.Failure.StatusCode >= 400,
		FailureStatusCode: usage.Failure.StatusCode,
		FailureBody:       strings.TrimSpace(usage.Failure.Body),
	}
}

func durationToMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64(duration / time.Millisecond)
}

func normalizeModel(model string) string {
	if trimmed := strings.TrimSpace(model); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

func groupingKey(apiKey, provider string) string {
	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(provider); trimmed != "" {
		return trimmed
	}
	return "unknown"
}

func normalizeTotalTokens(tokens TokenStats) int64 {
	if tokens.TotalTokens != 0 {
		return tokens.TotalTokens
	}
	total := saturatingTokenSum(tokens.InputTokens, tokens.OutputTokens, tokens.ReasoningTokens)
	if total != 0 {
		return total
	}
	return saturatingTokenSum(tokens.InputTokens, tokens.OutputTokens, tokens.ReasoningTokens, tokens.CachedTokens)
}

func saturatingTokenSum(values ...int64) int64 {
	var total int64
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if total > math.MaxInt64-value {
			return math.MaxInt64
		}
		total += value
	}
	return total
}

func nonNegativeTokenStats(tokens TokenStats) TokenStats {
	tokens.InputTokens = nonNegativeInt64(tokens.InputTokens)
	tokens.OutputTokens = nonNegativeInt64(tokens.OutputTokens)
	tokens.ReasoningTokens = nonNegativeInt64(tokens.ReasoningTokens)
	tokens.CachedTokens = nonNegativeInt64(tokens.CachedTokens)
	tokens.CacheReadTokens = nonNegativeInt64(tokens.CacheReadTokens)
	tokens.CacheCreationTokens = nonNegativeInt64(tokens.CacheCreationTokens)
	tokens.TotalTokens = nonNegativeInt64(tokens.TotalTokens)
	return tokens
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
