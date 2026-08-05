package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		DataPath:        filepath.Join(t.TempDir(), "usage.db"),
		RetentionDays:   30,
		FlushInterval:   time.Hour,
		FlushMaxRecords: 100,
	}
}

func TestStorePersistsAcrossRestartAndReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	usage := normalizedUsage{
		Dimensions:  Dimensions{Provider: "p", Model: "m"},
		RequestedAt: time.Now().UTC(),
		LatencyNS:   uint64(time.Second),
		Counters:    Counters{Requests: 1, InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	if err := store.Record(usage); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 15 || stats.Summary.Requests != 1 {
		t.Fatalf("persisted stats = %+v", stats.Summary)
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
	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 0 || len(stats.Groups) != 0 {
		t.Fatalf("reset did not persist: %+v", stats)
	}
}

func TestDashboardPreferencesPersistAcrossRestartAndStatsReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := store.QueryDashboardPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.RequestPageSize != 100 || defaults.DimensionPageSize != 100 || len(defaults.HiddenRequestColumns) != 0 || len(defaults.HiddenDimensionColumns) != 0 {
		t.Fatalf("default preferences = %+v", defaults)
	}
	want := DashboardPreferences{
		RequestPageSize:        50,
		DimensionPageSize:      200,
		HiddenRequestColumns:   []string{"source", "model", "source"},
		HiddenDimensionColumns: []string{"provider"},
	}
	saved, err := store.SaveDashboardPreferences(want)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RequestPageSize != 50 || saved.DimensionPageSize != 200 || len(saved.HiddenRequestColumns) != 2 || saved.HiddenRequestColumns[0] != "model" || saved.HiddenRequestColumns[1] != "source" {
		t.Fatalf("normalized preferences = %+v", saved)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	loaded, err := store.QueryDashboardPreferences()
	if err != nil || loaded.RequestPageSize != 50 || loaded.DimensionPageSize != 200 || len(loaded.HiddenDimensionColumns) != 1 || loaded.HiddenDimensionColumns[0] != "provider" {
		t.Fatalf("preferences after restart = %+v, %v", loaded, err)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.QueryDashboardPreferences()
	if err != nil || loaded.RequestPageSize != 50 || len(loaded.HiddenRequestColumns) != 2 {
		t.Fatalf("stats reset removed dashboard preferences: %+v, %v", loaded, err)
	}
}

func TestDashboardPreferencesValidation(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	valid := defaultDashboardPreferences()
	for _, mutate := range []func(*DashboardPreferences){
		func(value *DashboardPreferences) { value.RequestPageSize = 0 },
		func(value *DashboardPreferences) { value.DimensionPageSize = 501 },
		func(value *DashboardPreferences) { value.HiddenRequestColumns = []string{"api_key"} },
		func(value *DashboardPreferences) {
			value.HiddenDimensionColumns = append([]string(nil), dimensionColumnKeys...)
		},
	} {
		value := cloneDashboardPreferences(valid)
		mutate(&value)
		if _, err := store.SaveDashboardPreferences(value); err == nil || errorHTTPStatus(err) != 400 {
			t.Fatalf("invalid preferences accepted: %+v, %v", value, err)
		}
	}
}

func TestModelPricesPersistAcrossRestartAndStatsReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := store.SaveModelPrices(map[string]ModelPrice{
		"gpt-test": {Input: 2.5, Output: 10},
		"zero":     {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved["gpt-test"].Output != 10 || saved["zero"].Source != priceSourceManual {
		t.Fatalf("saved prices = %+v", saved)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prices, err := store.QueryModelPrices()
	if err != nil || len(prices) != 2 || prices["gpt-test"].Input != 2.5 || prices["zero"].Source != priceSourceManual {
		t.Fatalf("prices after restart = %+v, %v", prices, err)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	prices, err = store.QueryModelPrices()
	if err != nil || len(prices) != 2 || prices["gpt-test"].Output != 10 || prices["zero"].Source != priceSourceManual {
		t.Fatalf("stats reset removed model prices: %+v, %v", prices, err)
	}
}

func TestModelPriceValidation(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, prices := range []map[string]ModelPrice{
		{"": {Input: 1}},
		{"bad": {Input: -1}},
		{"bad": {Output: maxTokenPricePerM + 1}},
		{"gpt": {Input: 1}, " gpt ": {Output: 2}},
	} {
		if _, err := store.SaveModelPrices(prices); err == nil || errorHTTPStatus(err) != 400 {
			t.Fatalf("invalid prices accepted: %+v, %v", prices, err)
		}
	}
}

func TestStoreAggregatesAtMinuteGranularity(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Minute)
	for _, requestedAt := range []time.Time{base.Add(10 * time.Second), base.Add(50 * time.Second), base.Add(time.Minute + 5*time.Second)} {
		if err := store.Record(normalizedUsage{
			Dimensions:  Dimensions{Model: "minute-model"},
			RequestedAt: requestedAt,
			Counters:    Counters{Requests: 1, TotalTokens: 1},
		}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := store.Query("24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Series) != 2 || stats.Series[0].Requests != 2 || stats.Series[1].Requests != 1 {
		t.Fatalf("minute series = %+v, want two minute buckets", stats.Series)
	}
	first, err := time.Parse(time.RFC3339, stats.Series[0].Hour)
	if err != nil {
		t.Fatal(err)
	}
	second, err := time.Parse(time.RFC3339, stats.Series[1].Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Sub(first); got != time.Minute {
		t.Fatalf("minute bucket spacing = %v, want %v", got, time.Minute)
	}
}

func TestStoreSyncOnRecord(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	usage := normalizedUsage{Dimensions: Dimensions{Model: "sync"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1, TotalTokens: 7}}
	if err := store.Record(usage); err != nil {
		t.Fatal(err)
	}

	replacement, err := openStore(config)
	if err != nil {
		t.Fatalf("open replacement store: %v", err)
	}
	defer replacement.Close()
	if _, err := store.Query("retention"); err == nil || err.Error() != "store is closed" {
		t.Fatalf("retired store query error = %v, want store is closed", err)
	}
	stats, err := replacement.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 1 || stats.Summary.TotalTokens != 7 {
		t.Fatalf("synchronous record was not preserved across handover: %+v", stats.Summary)
	}
}

func TestStoreReconfigureSamePath(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	config.RetentionDays = 7
	config.FlushInterval = 2 * time.Second
	if err := store.Reconfigure(config); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionAdvancesRetainedSinceAndSurvivesRestart(t *testing.T) {
	config := testConfig(t)
	config.RetentionDays = 1
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	old := normalizedUsage{
		Dimensions:  Dimensions{Model: "expired"},
		RequestedAt: time.Now().UTC().Add(-48 * time.Hour),
		Counters:    Counters{Requests: 1, TotalTokens: 9},
	}
	if err := store.Record(old); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 0 {
		t.Fatalf("expired usage was retained: %+v", stats)
	}
	minimum := time.Now().UTC().Add(-25 * time.Hour)
	if stats.RetainedSince.Before(minimum) {
		t.Fatalf("retained_since did not advance: %v", stats.RetainedSince)
	}
	retainedSince := stats.RetainedSince
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if !stats.RetainedSince.Equal(retainedSince) {
		t.Fatalf("retained_since changed after restart: %v != %v", stats.RetainedSince, retainedSince)
	}
}

func TestStoreConcurrentCloseIsIdempotent(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	const callers = 8
	var wg sync.WaitGroup
	wg.Add(callers)
	errorsFound := make(chan error, callers)
	for range callers {
		go func() {
			defer wg.Done()
			errorsFound <- store.Close()
		}()
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatalf("close failed: %v", err)
		}
	}
}

func TestRuntimeSerializesConcurrentReconfigure(t *testing.T) {
	base := testConfig(t)
	runtime := &pluginRuntime{}
	request := func(retention int) []byte {
		config := []byte("data_path: " + filepath.ToSlash(base.DataPath) + "\nretention_days: " + fmt.Sprint(retention) + "\n")
		raw, _ := json.Marshal(lifecycleRequest{ConfigYAML: config, SchemaVersion: 1})
		return raw
	}
	if _, err := runtime.register(request(30)); err != nil {
		t.Fatal(err)
	}
	defer runtime.shutdown()

	var wg sync.WaitGroup
	for _, retention := range []int{7, 14, 21, 30} {
		wg.Add(1)
		go func(value int) {
			defer wg.Done()
			if _, err := runtime.reconfigure(request(value)); err != nil {
				t.Errorf("reconfigure %d: %v", value, err)
			}
		}(retention)
	}
	wg.Wait()
	runtime.mu.RLock()
	active := runtime.config.RetentionDays
	runtime.mu.RUnlock()
	if active != 7 && active != 14 && active != 21 && active != 30 {
		t.Fatalf("unexpected active retention: %d", active)
	}
}

func TestStoreDoesNotDropRecordsAfterFlushFailure(t *testing.T) {
	db, err := bolt.Open(filepath.Join(t.TempDir(), "failed-flush.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	actor := &storeActor{
		db: db,
		config: Config{
			RetentionDays:   30,
			FlushInterval:   time.Hour,
			FlushMaxRecords: 100,
			SyncOnRecord:    true,
		},
		data:  make(map[aggregateKey]Counters),
		dirty: make(map[aggregateKey]struct{}),
		since: now,
	}
	store := &Store{commands: make(chan any, 8), done: make(chan struct{})}
	go store.run(actor)

	// Closing the database forces every synchronous flush to fail. Both usage
	// calls should report that failure, but neither record may be discarded.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	usage := normalizedUsage{
		Dimensions:  Dimensions{Provider: "test", Model: "recovery"},
		RequestedAt: now,
		Counters:    Counters{Requests: 1, TotalTokens: 3},
	}
	if err := store.Record(usage); err == nil {
		t.Fatal("first record should report the forced flush failure")
	}
	if err := store.Record(usage); err == nil {
		t.Fatal("second record should report the forced flush failure")
	}
	_ = store.Close()

	key := aggregateKey{Hour: now.Truncate(time.Minute).Unix(), Dimensions: usage.Dimensions}
	if got := actor.data[key].Requests; got != 2 {
		t.Fatalf("accepted requests = %d, want 2", got)
	}
	if actor.pending != 2 || len(actor.dirty) != 1 {
		t.Fatalf("unexpected pending state: pending=%d dirty=%d", actor.pending, len(actor.dirty))
	}
}

func TestStorePersistsAndQueriesPerRequestDetails(t *testing.T) {
	config := testConfig(t)
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	usages := []normalizedUsage{
		{
			Dimensions:  Dimensions{Provider: "openai", Model: "alpha", Source: "cli", ServiceTier: "priority", ReasoningEffort: "high"},
			RequestedAt: base, LatencyNS: uint64(3 * time.Second), TTFTNS: uint64(time.Second),
			Counters: Counters{Requests: 1, InputTokens: 100, OutputTokens: 40, ReasoningTokens: 8, CacheReadTokens: 12, CacheCreationTokens: 3, TotalTokens: 148},
		},
		{
			Dimensions:  Dimensions{Provider: "anthropic", Model: "beta", Source: "web", ServiceTier: "standard", Failed: true, FailureStatus: 500},
			RequestedAt: base.Add(time.Second), LatencyNS: uint64(2 * time.Second), TTFTNS: uint64(500 * time.Millisecond),
			Counters: Counters{Requests: 1, FailedRequests: 1, InputTokens: 20, OutputTokens: 3, TotalTokens: 23},
		},
		{
			Dimensions: Dimensions{Model: "beta", Source: "batch"}, RequestedAt: base.Add(time.Second),
			Counters: Counters{Requests: 1, InputTokens: 7, OutputTokens: 2, TotalTokens: 9},
		},
	}
	for _, usage := range usages {
		if err := store.Record(usage); err != nil {
			t.Fatal(err)
		}
	}

	page, err := store.QueryRequests("24h", 0, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 2 {
		t.Fatalf("unexpected first page: %+v", page)
	}
	if page.Items[0].Sequence <= page.Items[1].Sequence || page.Items[0].Model != "beta" {
		t.Fatalf("requests are not newest-first: %+v", page.Items)
	}
	if page.Items[1].Result != "失败 (HTTP 500)" || page.Items[1].GenerationNS != uint64(1500*time.Millisecond) || page.Items[1].TPS != 2 {
		t.Fatalf("unexpected failed request detail: %+v", page.Items[1])
	}
	filtered, err := store.QueryRequests("24h", 0, 100, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 1 || len(filtered.Items) != 1 {
		t.Fatalf("unexpected filtered page: %+v", filtered)
	}
	item := filtered.Items[0]
	if item.Result != "成功" || item.GenerationNS != uint64(2*time.Second) || item.TTFTNS != uint64(time.Second) || !item.CacheHit {
		t.Fatalf("unexpected request timings/status: %+v", item)
	}
	if item.TPS != 20 || item.InputTokens != 100 || item.OutputTokens != 40 || item.ReasoningTokens != 8 || item.CacheCreationTokens != 3 {
		t.Fatalf("unexpected request counters: %+v", item)
	}
	queryRange, err := presetUsageRange("24h", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.queryRequestPageBySource(queryRange, 0, 100, "", "", "failed")
	if err != nil || failed.Total != 1 || len(failed.Items) != 1 || !failed.Items[0].Failed {
		t.Fatalf("unexpected failed-result filter: %+v, %v", failed, err)
	}
	success, err := store.queryRequestPageBySource(queryRange, 0, 100, "alpha", "cli", "success")
	if err != nil || success.Total != 1 || len(success.Items) != 1 || success.Items[0].Failed {
		t.Fatalf("unexpected combined request filters: %+v, %v", success, err)
	}
	if _, err := store.queryRequestPageBySource(queryRange, 0, 100, "", "", "unknown"); err == nil {
		t.Fatal("invalid result filter was accepted")
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err = store.QueryRequests("24h", 2, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 1 || page.Items[0].Model != "alpha" {
		t.Fatalf("request details did not survive restart/pagination: %+v", page)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	page, err = store.QueryRequests("retention", 0, 100, "")
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("reset did not clear request details: %+v, %v", page, err)
	}
}

func TestRequestDetailsRespectRetention(t *testing.T) {
	config := testConfig(t)
	config.RetentionDays = 1
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Record(normalizedUsage{
		Dimensions: Dimensions{Model: "expired"}, RequestedAt: time.Now().UTC().Add(-48 * time.Hour),
		Counters: Counters{Requests: 1, TotalTokens: 5},
	}); err != nil {
		t.Fatal(err)
	}
	page, err := store.QueryRequests("retention", 0, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("expired request detail was retained: %+v", page)
	}
}

func TestObservedModelsRespectRetention(t *testing.T) {
	config := testConfig(t)
	config.RetentionDays = 1
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	for model, requestedAt := range map[string]time.Time{
		"expired": now.Add(-48 * time.Hour),
		"current": now.Add(-time.Hour),
	} {
		if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: model}, RequestedAt: requestedAt, Counters: Counters{Requests: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	models, err := store.ObservedModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "current" {
		t.Fatalf("observed models = %+v", models)
	}
}

func TestSchemaOneDatabaseMigratesRequestBucket(t *testing.T) {
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(1)); err != nil {
			return err
		}
		if err := meta.Put(sinceKey, encodeInt64(time.Now().UTC().UnixNano())); err != nil {
			return err
		}
		_, err = tx.CreateBucket(hoursBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryRequests("24h", 0, 100, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		if tx.Bucket(requestsBucket) == nil {
			return fmt.Errorf("requests bucket is missing after migration")
		}
		if version := decodeUint64(tx.Bucket(metaBucket).Get(schemaKey)); version != persistenceSchemaVersion {
			return fmt.Errorf("schema version = %d", version)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaFourUsageSourcesMigrateWithoutAPIKeys(t *testing.T) {
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	apiKey := "sk-persisted-secret-1234567890"
	dimensions := Dimensions{Provider: "openai", Model: "gpt-test", Source: apiKey, AuthType: "apikey"}
	counters := Counters{Requests: 1, InputTokens: 3, TotalTokens: 3}
	dimensionKey, _ := json.Marshal(dimensions)
	counterValue, _ := json.Marshal(counters)
	requestValue, _ := json.Marshal(RequestDetail{Sequence: 1, Time: now, Dimensions: dimensions, Counters: counters})
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(4)); err != nil {
			return err
		}
		if err := meta.Put(sinceKey, encodeInt64(now.Add(-time.Hour).UnixNano())); err != nil {
			return err
		}
		if err := meta.Put(requestSequenceKey, encodeUint64(1)); err != nil {
			return err
		}
		hours, err := tx.CreateBucket(hoursBucket)
		if err != nil {
			return err
		}
		hour, err := hours.CreateBucket(encodeInt64(now.Truncate(time.Minute).Unix()))
		if err != nil {
			return err
		}
		if err := hour.Put(dimensionKey, counterValue); err != nil {
			return err
		}
		requests, err := tx.CreateBucket(requestsBucket)
		if err != nil {
			return err
		}
		return requests.Put(encodeRequestKey(now.UnixNano(), 1), requestValue)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.Groups) != 1 || stats.Groups[0].Source != "https://api.openai.com/v1" {
		t.Fatalf("migrated groups = %+v", stats.Groups)
	}
	page, err := store.QueryRequests("retention", 0, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Source != "https://api.openai.com/v1" {
		t.Fatalf("migrated requests = %+v", page.Items)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		if version := decodeUint64(tx.Bucket(metaBucket).Get(schemaKey)); version != persistenceSchemaVersion {
			return fmt.Errorf("schema version = %d", version)
		}
		for _, bucketName := range [][]byte{hoursBucket, requestsBucket} {
			bucket := tx.Bucket(bucketName)
			if bucket == nil {
				continue
			}
			if err := bucket.ForEach(func(key, value []byte) error {
				if bytes.Contains(key, []byte(apiKey)) || bytes.Contains(value, []byte(apiKey)) {
					return fmt.Errorf("API key remains in %s bucket", bucketName)
				}
				if value == nil {
					nested := bucket.Bucket(key)
					if nested != nil {
						return nested.ForEach(func(nestedKey, nestedValue []byte) error {
							if bytes.Contains(nestedKey, []byte(apiKey)) || bytes.Contains(nestedValue, []byte(apiKey)) {
								return fmt.Errorf("API key remains in nested %s bucket", bucketName)
							}
							return nil
						})
					}
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaThreePricesMigrateAtomically(t *testing.T) {
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	legacyPrices, _ := json.Marshal(map[string]any{
		"paid": map[string]any{"input": 2.5, "output": 10.0},
		"free": map[string]any{"input": 0.0, "output": 0.0},
	})
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(3)); err != nil {
			return err
		}
		if err := meta.Put(modelPricesKey, legacyPrices); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(requestsBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.QueryPriceBook()
	if err != nil {
		t.Fatal(err)
	}
	if book.Revision != 1 || book.Prices["paid"].Source != priceSourceManual || book.Prices["free"].Source != priceSourceManual {
		t.Fatalf("migrated price book = %+v", book)
	}
	if len(book.SyncSettings.ProviderPriority) == 0 {
		t.Fatalf("migrated sync settings = %+v", book.SyncSettings)
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
	book, err = store.QueryPriceBook()
	if err != nil || len(book.Prices) != 2 || book.Prices["free"].Source != priceSourceManual {
		t.Fatalf("migrated prices after reset/restart = %+v, %v", book, err)
	}
}

func TestFailedSchemaThreePriceMigrationKeepsSchema(t *testing.T) {
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(3)); err != nil {
			return err
		}
		if err := meta.Put(modelPricesKey, []byte(`{"bad":{"input":-1}}`)); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(requestsBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := openStore(config); err == nil {
		_ = store.Close()
		t.Fatal("invalid schema-three prices migrated successfully")
	}
	db, err = bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		version := decodeUint64(tx.Bucket(metaBucket).Get(schemaKey))
		if version != 3 {
			return fmt.Errorf("schema version changed to %d after failed migration", version)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStoreBackupAndRestore(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC().Truncate(time.Minute)
	if err := store.Record(normalizedUsage{
		RequestedAt: now,
		Dimensions:  Dimensions{Provider: "p", Model: "backup-model", Source: "cli"},
		Counters:    Counters{Requests: 1, InputTokens: 11, OutputTokens: 7, TotalTokens: 18},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDashboardPreferences(DashboardPreferences{RequestPageSize: 25, DimensionPageSize: 50}); err != nil {
		t.Fatal(err)
	}

	before, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if before.Summary.TotalTokens != 18 {
		t.Fatalf("pre-backup tokens = %d, want 18", before.Summary.TotalTokens)
	}

	backup, err := store.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if len(backup) == 0 {
		t.Fatal("expected non-empty backup")
	}

	// Mutate live state so restore has something to reverse.
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 0 {
		t.Fatalf("expected empty stats after reset, got %+v", stats.Summary)
	}

	leasePath := config.DataPath + ".handover"
	leaseBefore, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read handover lease: %v", err)
	}

	if err := store.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}

	leaseAfter, err := os.ReadFile(leasePath)
	if err != nil {
		t.Fatalf("read handover lease after restore: %v", err)
	}
	if string(leaseBefore) != string(leaseAfter) {
		t.Fatalf("handover lease changed during restore: before=%q after=%q", leaseBefore, leaseAfter)
	}

	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 18 {
		t.Fatalf("restored total tokens = %d, want 18", stats.Summary.TotalTokens)
	}
	preferences, err := store.QueryDashboardPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if preferences.RequestPageSize != 25 || preferences.DimensionPageSize != 50 {
		t.Fatalf("restored preferences = %+v", preferences)
	}

	// Invalid payload must leave the live database usable.
	if err := store.RestoreBackup([]byte("not-a-database")); err == nil {
		t.Fatal("expected invalid backup to fail")
	}
	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 18 {
		t.Fatalf("tokens after failed restore = %d, want 18", stats.Summary.TotalTokens)
	}
}

func TestValidateRestoreDatabaseRejectsWrongSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-schema.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(requestsBucket); err != nil {
			return err
		}
		return meta.Put(schemaKey, encodeUint64(4))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDatabase(path); err == nil {
		t.Fatal("expected schema mismatch")
	}
}

func TestValidateRestoreDatabaseRejectsMalformedRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-requests.db")
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		requests, err := tx.CreateBucket(requestsBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(persistenceSchemaVersion)); err != nil {
			return err
		}
		return requests.Put(encodeRequestKey(time.Now().UTC().UnixNano(), 1), []byte("not-json"))
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDatabase(path); err == nil {
		t.Fatal("expected malformed request payload to fail validation")
	}
}

func TestRecoverInterruptedRestoreUsesRollbackWhenLiveMissing(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{
		RequestedAt: time.Now().UTC(),
		Dimensions:  Dimensions{Model: "rollback-model", Source: "cli"},
		Counters:    Counters{Requests: 1, TotalTokens: 12},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	rollbackPath := config.DataPath + ".rollback"
	if err := os.Rename(config.DataPath, rollbackPath); err != nil {
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
	if stats.Summary.TotalTokens != 12 {
		t.Fatalf("recovered tokens = %d, want 12", stats.Summary.TotalTokens)
	}
	if _, err := os.Stat(rollbackPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback file still present after recovery: %v", err)
	}
}

func TestRecoverInterruptedRestoreDropsRollbackWhenLiveExists(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{
		RequestedAt: time.Now().UTC(),
		Dimensions:  Dimensions{Model: "live-model", Source: "cli"},
		Counters:    Counters{Requests: 1, TotalTokens: 8},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	rollbackPath := config.DataPath + ".rollback"
	if err := os.WriteFile(rollbackPath, []byte("stale-rollback"), 0o600); err != nil {
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
	if stats.Summary.TotalTokens != 8 {
		t.Fatalf("live tokens = %d, want 8", stats.Summary.TotalTokens)
	}
	if _, err := os.Stat(rollbackPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale rollback file still present: %v", err)
	}
}
