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

	key := aggregateKey{Hour: now.Truncate(time.Hour).Unix(), Dimensions: usage.Dimensions}
	if got := actor.data[key].Requests; got != 2 {
		t.Fatalf("accepted requests = %d, want 2", got)
	}
	if actor.pending != 2 || len(actor.dirty) != 1 {
		t.Fatalf("unexpected pending state: pending=%d dirty=%d", actor.pending, len(actor.dirty))
	}
}
