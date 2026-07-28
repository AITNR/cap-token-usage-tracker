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

func TestDecodeUsageReplacesAPIKeySourceWithProviderServiceAddress(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 34, 56, 0, time.UTC)
	apiKey := "sk-user-secret-1234567890"
	record := pluginapi.UsageRecord{
		Provider:     "openai",
		ExecutorType: "OpenAICompatExecutor",
		APIKey:       apiKey,
		AuthType:     "apikey",
		Source:       apiKey,
		RequestedAt:  now,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Dimensions.Source != "https://api.openai.com/v1" {
		t.Fatalf("source = %q, want OpenAI service address", usage.Dimensions.Source)
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), apiKey) {
		t.Fatalf("API key leaked in normalized usage: %s", encoded)
	}
}

func TestSafeUsageSourceSanitizesURLAndCredentials(t *testing.T) {
	tests := []struct {
		name         string
		source       string
		apiKey       string
		provider     string
		executorType string
		authType     string
		want         string
	}{
		{name: "url removes credentials and request metadata", source: "https://user:secret@example.com/v1/?api_key=secret#fragment", want: "https://example.com/v1"},
		{name: "api key auth replaces arbitrary key", source: "plain-secret-without-known-prefix", provider: "anthropic", authType: "api_key", want: "https://api.anthropic.com"},
		{name: "record api key match", source: "opaque", apiKey: "opaque", provider: "gemini", want: "https://generativelanguage.googleapis.com"},
		{name: "known credential prefix", source: "gsk_secret01234567890123456789", provider: "groq", want: "https://api.groq.com/openai/v1"},
		{name: "safe integration source", source: "cli", provider: "openai", authType: "oauth", want: "cli"},
		{name: "unknown provider fallback", source: "secret", provider: "custom-provider", authType: "apikey", want: "custom-provider"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := safeUsageSource(test.source, test.apiKey, test.provider, test.executorType, test.authType); got != test.want {
				t.Fatalf("safeUsageSource() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDecodeUsageDerivesExplicitZeroTotal(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		RequestedAt: now,
		Detail: pluginapi.UsageDetail{
			InputTokens:     17,
			OutputTokens:    9,
			ReasoningTokens: 4,
			CachedTokens:    50,
			TotalTokens:     0,
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
	if usage.Counters.TotalTokens != 30 {
		t.Fatalf("total tokens = %d, want 30", usage.Counters.TotalTokens)
	}
}

func TestDecodeUsageFallsBackToCachedTokens(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		RequestedAt: now,
		Detail: pluginapi.UsageDetail{
			InputTokens:         -1,
			OutputTokens:        0,
			ReasoningTokens:     -2,
			CachedTokens:        41,
			CacheReadTokens:     99,
			CacheCreationTokens: 100,
			TotalTokens:         0,
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
	if usage.Counters.TotalTokens != 41 {
		t.Fatalf("total tokens = %d, want cached fallback 41", usage.Counters.TotalTokens)
	}
}

func TestDecodeUsagePreservesExplicitPositiveTotal(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	record := pluginapi.UsageRecord{
		RequestedAt: now,
		Detail: pluginapi.UsageDetail{
			InputTokens:     17,
			OutputTokens:    9,
			ReasoningTokens: 4,
			CachedTokens:    50,
			TotalTokens:     7,
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
	if usage.Counters.TotalTokens != 7 {
		t.Fatalf("total tokens = %d, want explicit total 7", usage.Counters.TotalTokens)
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
