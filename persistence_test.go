package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		DataPath:      filepath.Join(t.TempDir(), defaultDatabaseFile),
		RetentionDays: 0,
	}
}

func testRecord(id string, timestamp time.Time, model string, total int64) Record {
	return Record{
		ID:        id,
		Timestamp: timestamp,
		Provider:  "provider",
		Model:     model,
		Tokens: TokenStats{
			InputTokens:  total / 2,
			OutputTokens: total - total/2,
			TotalTokens:  total,
		},
	}
}

func TestStorePersistsCompleteRequestAcrossRestart(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := time.Date(2026, 7, 15, 8, 9, 10, 123456789, time.FixedZone("UTC+8", 8*60*60))
	record := Record{
		ID:              "request-1",
		Timestamp:       timestamp,
		APIKey:          " api-key ",
		Provider:        " provider ",
		Model:           " model ",
		Alias:           " alias ",
		Source:          " source ",
		AuthID:          " auth-id ",
		AuthIndex:       " 3 ",
		AuthType:        " oauth ",
		ExecutorType:    " executor ",
		ReasoningEffort: " high ",
		ServiceTier:     " priority ",
		LatencyMs:       2500,
		TTFTMs:          300,
		Tokens: TokenStats{
			InputTokens:         10,
			OutputTokens:        20,
			ReasoningTokens:     4,
			CachedTokens:        5,
			CacheReadTokens:     3,
			CacheCreationTokens: 2,
			TotalTokens:         34,
		},
		Failed:            true,
		FailureStatusCode: 429,
		FailureBody:       " failure body ",
	}
	if err := store.Record(record); err != nil {
		t.Fatal(err)
	}

	// Record is synchronous: a separate connection can observe the committed row
	// before Record returns.
	observer, err := sql.Open("sqlite", config.DataPath)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := observer.QueryRow("SELECT COUNT(*) FROM usage_records WHERE id = ?", record.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := observer.Close(); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("committed row count = %d, want 1", count)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	usage, err := store.QueryUsage(context.Background(), QueryRange{})
	if err != nil {
		t.Fatal(err)
	}
	details := usage["api-key"]["model"]
	if len(details) != 1 {
		t.Fatalf("usage grouping = %+v", usage)
	}
	detail := details[0]
	if detail.ID != record.ID || !detail.Timestamp.Equal(timestamp.UTC()) {
		t.Fatalf("unexpected identity/time: %+v", detail)
	}
	if detail.Provider != "provider" || detail.Model != "model" || detail.Alias != "alias" || detail.Source != "source" {
		t.Fatalf("unexpected routing metadata: %+v", detail)
	}
	if detail.AuthID != "auth-id" || detail.AuthIndex != "3" || detail.AuthType != "oauth" || detail.ExecutorType != "executor" {
		t.Fatalf("unexpected auth metadata: %+v", detail)
	}
	if detail.ReasoningEffort != "high" || detail.ServiceTier != "priority" || detail.LatencyMs != 2500 || detail.TTFTMs != 300 {
		t.Fatalf("unexpected request metadata: %+v", detail)
	}
	if detail.Tokens != record.Tokens || !detail.Failed || detail.FailureStatusCode != 429 || detail.FailureBody != "failure body" {
		t.Fatalf("unexpected usage/failure metadata: %+v", detail)
	}

	var storedTimestamp string
	if err := store.db.QueryRow("SELECT timestamp FROM usage_records WHERE id = ?", record.ID).Scan(&storedTimestamp); err != nil {
		t.Fatal(err)
	}
	if storedTimestamp != formatTimestamp(timestamp) {
		t.Fatalf("stored timestamp = %q, want %q", storedTimestamp, formatTimestamp(timestamp))
	}
}

func TestStoreNormalizesTokensDurationsAndTotal(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record := Record{
		ID:                "normalized",
		Timestamp:         time.Now().UTC(),
		Model:             "",
		LatencyMs:         -1,
		TTFTMs:            -2,
		FailureStatusCode: -500,
		Tokens: TokenStats{
			InputTokens:         8,
			OutputTokens:        3,
			ReasoningTokens:     -4,
			CachedTokens:        9,
			CacheReadTokens:     -1,
			CacheCreationTokens: 2,
		},
	}
	if err := store.Record(record); err != nil {
		t.Fatal(err)
	}
	usage, err := store.QueryUsage(context.Background(), QueryRange{})
	if err != nil {
		t.Fatal(err)
	}
	detail := usage["unknown"]["unknown"][0]
	wantTokens := TokenStats{InputTokens: 8, OutputTokens: 3, CachedTokens: 9, CacheCreationTokens: 2, TotalTokens: 11}
	if detail.Tokens != wantTokens {
		t.Fatalf("tokens = %+v, want %+v", detail.Tokens, wantTokens)
	}
	if detail.LatencyMs != 0 || detail.TTFTMs != 0 || detail.FailureStatusCode != 0 {
		t.Fatalf("negative values were not clamped: %+v", detail)
	}
}

func TestStoreAggregatesRequestsAtMinuteGranularity(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	for index, requestedAt := range []time.Time{base.Add(10 * time.Second), base.Add(50 * time.Second), base.Add(time.Minute + 5*time.Second)} {
		record := testRecord("minute-"+string(rune('a'+index)), requestedAt, "minute-model", 1)
		if err := store.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := store.Query("24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Series) != 2 || stats.Series[0].Requests != 2 || stats.Series[1].Requests != 1 {
		t.Fatalf("minute series = %+v", stats.Series)
	}
	first, err := time.Parse(time.RFC3339, stats.Series[0].Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := time.Parse(time.RFC3339, stats.Series[1].Hour)
	if err != nil {
		t.Fatal(err)
	}
	if second.Sub(first) != time.Minute {
		t.Fatalf("bucket spacing = %v, want 1m", second.Sub(first))
	}
}

func TestStoreQueryRequestsIsPaginatedFilteredAndSanitized(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Now().UTC().Add(-time.Minute)
	records := []Record{
		{ID: "one", Timestamp: base, APIKey: "secret-key", AuthID: "secret-auth", Provider: "p", Model: "a", LatencyMs: 2000, TTFTMs: 500, Tokens: TokenStats{OutputTokens: 30, TotalTokens: 30}, FailureBody: "secret-body"},
		{ID: "two", Timestamp: base.Add(time.Second), Provider: "p", Model: "b", Failed: true, FailureStatusCode: 500, Tokens: TokenStats{TotalTokens: 2}},
		{ID: "three", Timestamp: base.Add(2 * time.Second), Provider: "p", Model: "a", Tokens: TokenStats{TotalTokens: 3}},
	}
	for _, record := range records {
		if err := store.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.QueryRequests("24h", 0, 1, "a")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != "three" {
		t.Fatalf("unexpected filtered page: %+v", page)
	}
	page, err = store.QueryRequests("24h", 1, 1, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "one" {
		t.Fatalf("unexpected second page: %+v", page)
	}
	item := page.Items[0]
	if item.GenerationNS != uint64(1500*time.Millisecond) || item.TPS != 20 || item.Result != "成功" {
		t.Fatalf("unexpected derived request fields: %+v", item)
	}
	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-key", "secret-auth", "secret-body"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("dashboard request response leaked %q: %s", secret, encoded)
		}
	}
}

func TestStoreQueryUsageRangeAndDelete(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)
	for index, timestamp := range []time.Time{base, base.Add(time.Hour), base.Add(2 * time.Hour)} {
		if err := store.Record(testRecord([]string{"first", "second", "third"}[index], timestamp, "m", int64(index+1))); err != nil {
			t.Fatal(err)
		}
	}
	start := base.Add(time.Hour)
	end := base.Add(2 * time.Hour)
	usage, err := store.QueryUsage(context.Background(), QueryRange{Start: &start, End: &end})
	if err != nil {
		t.Fatal(err)
	}
	details := usage["provider"]["m"]
	if len(details) != 1 || details[0].ID != "second" {
		t.Fatalf("range result = %+v", usage)
	}
	result, err := store.Delete(context.Background(), []string{"second", "missing", "second", ""})
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 1 || len(result.Missing) != 2 || result.Missing[0] != "missing" || result.Missing[1] != "second" {
		t.Fatalf("delete result = %+v", result)
	}
}

func TestStoreRetentionAndReconfigure(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	if err := store.Record(testRecord("old", now.Add(-48*time.Hour), "m", 1)); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(testRecord("recent", now.Add(-time.Hour), "m", 1)); err != nil {
		t.Fatal(err)
	}
	config.RetentionDays = 1
	if err := store.Reconfigure(config); err != nil {
		t.Fatal(err)
	}
	usage, err := store.QueryUsage(context.Background(), QueryRange{})
	if err != nil {
		t.Fatal(err)
	}
	details := usage["provider"]["m"]
	if len(details) != 1 || details[0].ID != "recent" {
		t.Fatalf("retention result = %+v", usage)
	}
	other := config
	other.DataPath = filepath.Join(t.TempDir(), "other.db")
	if err := store.Reconfigure(other); err == nil {
		t.Fatal("reconfigure accepted a database path change")
	}
}

func TestStoreResetKeepsModelPrices(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	prices, err := store.SaveModelPrices(map[string]ModelPrice{
		"gpt-test": {Input: 2.5, Output: 10},
		"zero":     {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices["gpt-test"].Output != 10 {
		t.Fatalf("saved prices = %+v", prices)
	}
	if err := store.Record(testRecord("priced", time.Now().UTC(), "gpt-test", 7)); err != nil {
		t.Fatal(err)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 0 {
		t.Fatalf("reset stats = %+v", stats.Summary)
	}
	prices, err = store.QueryModelPrices()
	if err != nil || len(prices) != 1 || prices["gpt-test"].Input != 2.5 {
		t.Fatalf("prices after reset/restart = %+v, %v", prices, err)
	}
}

func TestModelPriceValidation(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, prices := range []map[string]ModelPrice{
		{"": {Input: 1}},
		{"bad": {Input: -1}},
		{"bad": {Output: maxTokenPricePerM + 1}},
		{"gpt": {Input: 1}, " gpt ": {Output: 2}},
		{strings.Repeat("界", maxDimensionRunes+1): {Input: 1}},
	} {
		if _, err := store.SaveModelPrices(prices); err == nil || errorHTTPStatus(err) != 400 {
			t.Fatalf("invalid prices accepted: %+v, %v", prices, err)
		}
	}
}

func TestSQLiteSchemaSelfHealing(t *testing.T) {
	config := testConfig(t)
	if err := os.MkdirAll(filepath.Dir(config.DataPath), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", config.DataPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE usage_records (id TEXT PRIMARY KEY, timestamp TEXT NOT NULL, provider TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage_records (id, timestamp, provider) VALUES ('legacy', '2026-07-15T00:00:00.000000000Z', 'legacy-provider')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	columns, err := store.existingColumns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"api_key", "model", "auth_id", "reasoning_effort", "service_tier", "cache_creation_tokens", "failure_body"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("self-healed schema missing %q", column)
		}
	}
	if err := store.Record(testRecord("new", time.Now().UTC(), "new-model", 9)); err != nil {
		t.Fatal(err)
	}
	usage, err := store.QueryUsage(context.Background(), QueryRange{})
	if err != nil {
		t.Fatal(err)
	}
	if len(usage["legacy-provider"]["unknown"]) != 1 || len(usage["provider"]["new-model"]) != 1 {
		t.Fatalf("migrated usage = %+v", usage)
	}
}

func TestOpenStoreRejectsLegacyOrCorruptDatabase(t *testing.T) {
	for name, contents := range map[string][]byte{
		"legacy-bbolt": []byte("not-a-sqlite-bbolt-database"),
		"short":        []byte("bad"),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.db")
			if err := os.WriteFile(path, contents, 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := openStore(Config{DataPath: path})
			if err == nil || !strings.Contains(err.Error(), "not SQLite") {
				t.Fatalf("open error = %v", err)
			}
		})
	}
}
