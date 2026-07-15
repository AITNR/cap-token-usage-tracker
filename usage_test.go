package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestDecodeUsageCapturesReferenceFields(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	requestedAt := time.Date(2026, 7, 15, 11, 59, 58, 123456789, time.FixedZone("UTC+8", 8*60*60))
	input := pluginapi.UsageRecord{
		Provider:        " anthropic ",
		ExecutorType:    " claude ",
		Model:           " claude-opus-4-8 ",
		Alias:           " opus ",
		APIKey:          " sk-client ",
		AuthID:          " auth-secret ",
		AuthIndex:       " 2 ",
		AuthType:        " oauth ",
		Source:          " anthropic ",
		ReasoningEffort: " high ",
		ServiceTier:     " priority ",
		RequestedAt:     requestedAt,
		Latency:         2*time.Second + 999*time.Microsecond,
		TTFT:            250*time.Millisecond + 500*time.Microsecond,
		Failed:          false,
		Failure: pluginapi.UsageFailure{
			StatusCode: 429,
			Body:       " private failure body ",
		},
		Detail: pluginapi.UsageDetail{
			InputTokens:         10,
			OutputTokens:        20,
			ReasoningTokens:     4,
			CachedTokens:        5,
			CacheReadTokens:     3,
			CacheCreationTokens: 2,
			TotalTokens:         34,
		},
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(record.ID); err != nil {
		t.Fatalf("record id is not a UUID: %q: %v", record.ID, err)
	}
	if !record.Timestamp.Equal(requestedAt.UTC()) {
		t.Fatalf("timestamp = %v, want %v", record.Timestamp, requestedAt.UTC())
	}
	if record.APIKey != "sk-client" || record.Provider != "anthropic" || record.Model != "claude-opus-4-8" || record.Alias != "opus" {
		t.Fatalf("unexpected routing metadata: %+v", record)
	}
	if record.AuthID != "auth-secret" || record.AuthIndex != "2" || record.AuthType != "oauth" || record.ExecutorType != "claude" {
		t.Fatalf("unexpected auth/executor metadata: %+v", record)
	}
	if record.Source != "anthropic" || record.ReasoningEffort != "high" || record.ServiceTier != "priority" {
		t.Fatalf("unexpected request metadata: %+v", record)
	}
	if record.LatencyMs != 2000 || record.TTFTMs != 250 {
		t.Fatalf("durations = latency %d ms, ttft %d ms", record.LatencyMs, record.TTFTMs)
	}
	if record.Tokens != (TokenStats{InputTokens: 10, OutputTokens: 20, ReasoningTokens: 4, CachedTokens: 5, CacheReadTokens: 3, CacheCreationTokens: 2, TotalTokens: 34}) {
		t.Fatalf("unexpected tokens: %+v", record.Tokens)
	}
	if !record.Failed || record.FailureStatusCode != 429 || record.FailureBody != "private failure body" {
		t.Fatalf("unexpected failure metadata: %+v", record)
	}
}

func TestDecodeUsageUsesCallbackTimeWhenRequestedAtMissing(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 987654321, time.FixedZone("UTC+8", 8*60*60))
	raw, err := json.Marshal(pluginapi.UsageRecord{Model: " ", Detail: pluginapi.UsageDetail{InputTokens: 17, OutputTokens: 9}})
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeUsage(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	if !record.Timestamp.Equal(now.UTC()) {
		t.Fatalf("timestamp = %v, want %v", record.Timestamp, now.UTC())
	}
	if record.Model != "unknown" {
		t.Fatalf("model = %q, want unknown", record.Model)
	}
	if got := normalizeTotalTokens(record.Tokens); got != 26 {
		t.Fatalf("derived total tokens = %d, want 26", got)
	}
}

func TestDecodeUsageRejectsMalformedJSON(t *testing.T) {
	if _, err := decodeUsage([]byte(`{"broken"`), time.Now()); err == nil {
		t.Fatal("malformed usage JSON was accepted")
	}
}

func TestTokenNormalizationMatchesReferenceStore(t *testing.T) {
	tokens := nonNegativeTokenStats(TokenStats{
		InputTokens:         -1,
		OutputTokens:        5,
		ReasoningTokens:     -2,
		CachedTokens:        7,
		CacheReadTokens:     -3,
		CacheCreationTokens: 4,
		TotalTokens:         0,
	})
	tokens.TotalTokens = normalizeTotalTokens(tokens)
	if tokens != (TokenStats{OutputTokens: 5, CachedTokens: 7, CacheCreationTokens: 4, TotalTokens: 5}) {
		t.Fatalf("unexpected normalized tokens: %+v", tokens)
	}
}
