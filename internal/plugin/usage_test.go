package plugin

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
		AuthIndex:       "stable-auth-index",
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
	if usage.Dimensions.APIKey != record.APIKey {
		t.Fatalf("transient API key = %q", usage.Dimensions.APIKey)
	}
	for _, secret := range []string{"secret-auth", "private failure body", record.AuthIndex} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("sensitive value leaked: %s", secret)
		}
	}
}

func TestDecodeUsageKeepsAuthIndexTransient(t *testing.T) {
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	raw := []byte("{\"provider\":\"codex\",\"source\":\"cli\",\"auth_index\":\"stable-auth-index\"}")
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.authIndex != "stable-auth-index" {
		t.Fatalf("transient auth index = %q", usage.authIndex)
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "stable-auth-index") || strings.Contains(string(encoded), "auth_index") {
		t.Fatalf("auth index leaked in normalized usage JSON: %s", encoded)
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
	if usage.Dimensions.APIKey != apiKey {
		t.Fatalf("transient API key = %q", usage.Dimensions.APIKey)
	}
}

func TestDecodeUsagePrefersConfiguredBaseURL(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	apiKey := "sk-user-secret-1234567890"
	record := pluginapi.UsageRecord{
		Provider:     "openai",
		ExecutorType: "OpenAICompatExecutor",
		APIKey:       apiKey,
		AuthType:     "apikey",
		Source:       apiKey,
		BaseURL:      "https://user:secret@relay.example.com/v1/?token=leaky#frag",
		RequestedAt:  now,
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"BaseURL"`) {
		t.Fatalf("SDK record should serialize the field as %q: %s", "BaseURL", raw)
	}
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Dimensions.Source != "https://relay.example.com/v1" {
		t.Fatalf("source = %q, want sanitized configured base URL", usage.Dimensions.Source)
	}
	if usage.baseURL != "https://relay.example.com/v1" {
		t.Fatalf("transient base URL = %q", usage.baseURL)
	}
	encoded, err := json.Marshal(usage)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"user:secret", "token=leaky"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("base URL credential leaked: %s", encoded)
		}
	}
}

func TestDecodeUsageAcceptsSnakeCaseBaseURL(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	raw := []byte(`{"provider":"xai","api_key":"xai-secret-1234567890","auth_type":"apikey","source":"xai-secret-1234567890","base_url":"https://relay.example.com/xai"}`)
	usage, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Dimensions.Source != "https://relay.example.com/xai" {
		t.Fatalf("source = %q, want snake_case base_url honored", usage.Dimensions.Source)
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
		baseURL      string
		want         string
	}{
		{name: "url removes credentials and request metadata", source: "https://user:secret@example.com/v1/?api_key=secret#fragment", want: "https://example.com/v1"},
		{name: "api key auth replaces arbitrary key", source: "plain-secret-without-known-prefix", provider: "anthropic", authType: "api_key", want: "https://api.anthropic.com"},
		{name: "record api key match", source: "opaque", apiKey: "opaque", provider: "gemini", want: "https://generativelanguage.googleapis.com"},
		{name: "known credential prefix", source: "gsk_secret01234567890123456789", provider: "groq", want: "https://api.groq.com/openai/v1"},
		{name: "safe integration source", source: "cli", provider: "openai", authType: "oauth", want: "cli"},
		{name: "unknown provider fallback", source: "secret", provider: "custom-provider", authType: "apikey", want: "custom-provider"},
		{name: "base url preferred over provider service address", source: "plain-secret-without-known-prefix", provider: "xai", authType: "api_key", baseURL: "https://relay.example.com/v1", want: "https://relay.example.com/v1"},
		{name: "base url used when source matches api key without auth type", source: "opaque", apiKey: "opaque", provider: "openai", baseURL: "https://relay.example.com/openai", want: "https://relay.example.com/openai"},
		{name: "base url used for credential-looking source", source: "gsk_secret01234567890123456789", provider: "groq", baseURL: "https://relay.example.com/groq", want: "https://relay.example.com/groq"},
		{name: "base url ignored for non-credential source", source: "cli", provider: "openai", authType: "oauth", baseURL: "https://relay.example.com/v1", want: "cli"},
		{name: "explicit source url still wins over base url", source: "https://direct.example.com/v1", provider: "openai", authType: "api_key", baseURL: "https://relay.example.com/v1", want: "https://direct.example.com/v1"},
		{name: "invalid base url falls back to provider service address", source: "opaque", apiKey: "opaque", provider: "openai", authType: "apikey", baseURL: "not a url", want: "https://api.openai.com/v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := safeUsageSource(test.source, test.apiKey, test.provider, test.executorType, test.authType, test.baseURL); got != test.want {
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
