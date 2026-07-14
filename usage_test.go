package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestDecodeUsageSDKJSON(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		Provider:        " anthropic ",
		ExecutorType:    "claude",
		Model:           "claude-opus-4-8",
		Alias:           "opus",
		APIKey:          "must-not-survive",
		AuthID:          "secret-auth",
		AuthIndex:       "2",
		AuthType:        "oauth",
		Source:          "anthropic",
		ReasoningEffort: "high",
		ServiceTier:     "priority",
		RequestedAt:     now.Add(-time.Minute),
		Latency:         2 * time.Second,
		TTFT:            250 * time.Millisecond,
		Failed:          true,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       "private failure body",
		},
		Detail: pluginapi.UsageDetail{
			InputTokens:         10,
			OutputTokens:        20,
			ReasoningTokens:     4,
			CachedTokens:        5,
			CacheReadTokens:     3,
			CacheCreationTokens: 2,
			TotalTokens:         30,
		},
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Dimensions.Provider != "anthropic" || usage.Dimensions.FailureStatus != 429 {
		t.Fatalf("unexpected dimensions: %+v", usage.Dimensions)
	}
	if usage.Counters.TotalTokens != 30 || usage.LatencyNS != uint64(2*time.Second) || usage.TTFTNS != uint64(250*time.Millisecond) {
		t.Fatalf("unexpected counters: %+v", usage)
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"must-not-survive", "secret-auth", "private failure body"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("sensitive value leaked: %s", secret)
		}
	}
}

func TestDecodeUsageSnakeCaseFallbackAndClamp(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	raw := []byte(`{
		"provider":"test","model":"model","requested_at":"2030-01-01T00:00:00Z",
		"latency":"15ms","ttft_ns":-1,"failed":false,
		"detail":{"input_tokens":12,"output_tokens":8,"reasoning_tokens":-3}
	}`)
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if !usage.RequestedAt.Equal(now) {
		t.Fatalf("future timestamp was not normalized: %v", usage.RequestedAt)
	}
	if usage.Counters.TotalTokens != 20 || usage.Counters.ReasoningTokens != 0 {
		t.Fatalf("unexpected token normalization: %+v", usage.Counters)
	}
	if usage.LatencyNS != uint64(15*time.Millisecond) || usage.TTFTNS != 0 {
		t.Fatalf("unexpected duration normalization: %+v", usage)
	}
}

func TestNormalizeDimensionCapsLength(t *testing.T) {
	value := normalizeDimension(strings.Repeat("界", maxDimensionRunes+20))
	if len([]rune(value)) != maxDimensionRunes {
		t.Fatalf("dimension length = %d", len([]rune(value)))
	}
}
