package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const maxDimensionRunes = 160

type normalizedUsage struct {
	Dimensions  Dimensions
	RequestedAt time.Time
	LatencyNS   uint64
	TTFTNS      uint64
	Counters    Counters
}

func decodeUsage(raw []byte, now time.Time) (normalizedUsage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return normalizedUsage{}, fmt.Errorf("decode usage record: %w", err)
	}

	requestedAt := firstTime(root, "RequestedAt", "requested_at")
	now = now.UTC()
	if requestedAt.IsZero() || requestedAt.After(now.Add(24*time.Hour)) {
		requestedAt = now
	} else {
		requestedAt = requestedAt.UTC()
	}

	failure := firstObject(root, "Failure", "failure")
	detail := firstObject(root, "Detail", "detail")
	total, totalPresent := firstInt64Present(detail, "TotalTokens", "total_tokens")
	if !totalPresent {
		total = saturatingInt64Sum(
			firstInt64(detail, "InputTokens", "input_tokens"),
			firstInt64(detail, "OutputTokens", "output_tokens"),
		)
	}

	failed := firstBool(root, "Failed", "failed")
	return normalizedUsage{
		Dimensions: Dimensions{
			Provider:        normalizeDimension(firstString(root, "Provider", "provider")),
			ExecutorType:    normalizeDimension(firstString(root, "ExecutorType", "executor_type")),
			Model:           normalizeDimension(firstString(root, "Model", "model")),
			Alias:           normalizeDimension(firstString(root, "Alias", "alias")),
			Source:          normalizeDimension(firstString(root, "Source", "source")),
			AuthType:        normalizeDimension(firstString(root, "AuthType", "auth_type")),
			ServiceTier:     normalizeDimension(firstString(root, "ServiceTier", "service_tier")),
			ReasoningEffort: normalizeDimension(firstString(root, "ReasoningEffort", "reasoning_effort")),
			Failed:          failed,
			FailureStatus:   clampStatus(firstInt64(failure, "StatusCode", "status_code")),
		},
		RequestedAt: requestedAt,
		LatencyNS:   positiveDurationNS(root, "Latency", "latency", "latency_ns"),
		TTFTNS:      positiveDurationNS(root, "TTFT", "ttft", "ttft_ns"),
		Counters: Counters{
			Requests:            1,
			FailedRequests:      boolCount(failed),
			InputTokens:         positiveUint(firstInt64(detail, "InputTokens", "input_tokens")),
			OutputTokens:        positiveUint(firstInt64(detail, "OutputTokens", "output_tokens")),
			ReasoningTokens:     positiveUint(firstInt64(detail, "ReasoningTokens", "reasoning_tokens")),
			CachedTokens:        positiveUint(firstInt64(detail, "CachedTokens", "cached_tokens")),
			CacheReadTokens:     positiveUint(firstInt64(detail, "CacheReadTokens", "cache_read_tokens")),
			CacheCreationTokens: positiveUint(firstInt64(detail, "CacheCreationTokens", "cache_creation_tokens")),
			TotalTokens:         positiveUint(total),
		},
	}, nil
}

func firstObject(root map[string]json.RawMessage, keys ...string) map[string]json.RawMessage {
	for _, key := range keys {
		if value, ok := root[key]; ok {
			var result map[string]json.RawMessage
			if json.Unmarshal(value, &result) == nil {
				return result
			}
		}
	}
	return map[string]json.RawMessage{}
}

func firstString(root map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if value, ok := root[key]; ok {
			var result string
			if json.Unmarshal(value, &result) == nil {
				return result
			}
		}
	}
	return ""
}

func firstBool(root map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if value, ok := root[key]; ok {
			var result bool
			if json.Unmarshal(value, &result) == nil {
				return result
			}
		}
	}
	return false
}

func firstInt64(root map[string]json.RawMessage, keys ...string) int64 {
	value, _ := firstInt64Present(root, keys...)
	return value
}

func firstInt64Present(root map[string]json.RawMessage, keys ...string) (int64, bool) {
	for _, key := range keys {
		value, ok := root[key]
		if !ok {
			continue
		}
		var result int64
		if json.Unmarshal(value, &result) == nil {
			return result, true
		}
	}
	return 0, false
}

func firstTime(root map[string]json.RawMessage, keys ...string) time.Time {
	for _, key := range keys {
		if value, ok := root[key]; ok {
			var result time.Time
			if json.Unmarshal(value, &result) == nil {
				return result
			}
		}
	}
	return time.Time{}
}

func positiveDurationNS(root map[string]json.RawMessage, keys ...string) uint64 {
	for _, key := range keys {
		value, ok := root[key]
		if !ok {
			continue
		}
		var numeric int64
		if json.Unmarshal(value, &numeric) == nil {
			return positiveUint(numeric)
		}
		var text string
		if json.Unmarshal(value, &text) == nil {
			if duration, err := time.ParseDuration(text); err == nil && duration > 0 {
				return uint64(duration)
			}
		}
	}
	return 0
}

func normalizeDimension(value string) string {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "�")
	}
	runes := []rune(value)
	if len(runes) > maxDimensionRunes {
		value = string(runes[:maxDimensionRunes])
	}
	return value
}

func positiveUint(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func boolCount(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func clampStatus(value int64) int {
	if value < 0 || value > 999 {
		return 0
	}
	return int(value)
}

func saturatingInt64Sum(left, right int64) int64 {
	if left <= 0 {
		left = 0
	}
	if right <= 0 {
		right = 0
	}
	if left > int64(^uint64(0)>>1)-right {
		return int64(^uint64(0) >> 1)
	}
	return left + right
}
