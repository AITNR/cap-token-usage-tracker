package main

import (
	"encoding/json"
	"fmt"
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
	if len(saved) != 1 || saved["gpt-test"].Output != 10 {
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
	if err != nil || len(prices) != 1 || prices["gpt-test"].Input != 2.5 {
		t.Fatalf("prices after restart = %+v, %v", prices, err)
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	prices, err = store.QueryModelPrices()
	if err != nil || len(prices) != 1 || prices["gpt-test"].Output != 10 {
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

	lockedConfig := config
	lockedConfig.DataPath = config.DataPath
	if _, err := openStore(lockedConfig); err == nil {
		t.Fatal("expected locked database open to fail")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
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

func TestStoreEnqueueDoesNotWaitForConsumer(t *testing.T) {
	store := newStoreMailbox()
	usage := normalizedUsage{
		Dimensions:  Dimensions{Provider: "test", Model: "queued"},
		RequestedAt: time.Now().UTC(),
		Counters:    Counters{Requests: 1, TotalTokens: 1},
	}

	const records = 4096
	done := make(chan error, 1)
	go func() {
		for range records {
			if err := store.Enqueue(usage); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("enqueue blocked on persistence consumer")
	}

	store.queueMu.Lock()
	queued := len(store.queue) - store.queueHead
	store.queueMu.Unlock()
	if queued != records {
		t.Fatalf("queued records = %d, want %d", queued, records)
	}
}

func TestStoreCloseDrainsEnqueuedUsage(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = false
	config.FlushMaxRecords = 100_000
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	const records = 1024
	for range records {
		if err := store.Enqueue(normalizedUsage{
			Dimensions:  Dimensions{Provider: "test", Model: "shutdown-drain"},
			RequestedAt: now,
			Counters:    Counters{Requests: 1, TotalTokens: 3},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	stats, err := store.Query("24h")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != records || stats.Summary.TotalTokens != records*3 {
		t.Fatalf("shutdown did not drain queued usage: %+v", stats.Summary)
	}
}

func TestStoreEnqueuedUsagePreservesFIFOWithQueries(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = false
	config.FlushMaxRecords = 100_000
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now().UTC()
	const records = 2048
	for i := range records {
		usage := normalizedUsage{
			Dimensions:  Dimensions{Provider: "test", Model: "fifo"},
			RequestedAt: now.Add(time.Duration(i) * time.Nanosecond),
			Counters:    Counters{Requests: 1, InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
		}
		if err := store.Enqueue(usage); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := store.Query("24h")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != records || stats.Summary.TotalTokens != records*2 {
		t.Fatalf("unexpected queued summary: %+v", stats.Summary)
	}

	page, err := store.QueryRequests("24h", 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != records {
		t.Fatalf("persisted request total = %d, want %d", page.Total, records)
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
	store := newStoreMailbox()
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
