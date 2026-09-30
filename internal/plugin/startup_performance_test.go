package plugin

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func startupTestActor(t *testing.T) *storeActor {
	t.Helper()
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	actor := &storeActor{db: db, config: config}
	if err := actor.initialize(); err != nil {
		t.Fatal(err)
	}
	return actor
}

func TestStartupMarksRetentionAlreadyPruned(t *testing.T) {
	actor := startupTestActor(t)
	if actor.lastPruneAt.IsZero() {
		t.Fatal("startup would immediately repeat retention pruning")
	}
	lastPrune := actor.lastPruneAt
	if err := actor.flush(time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	if actor.lastPruneAt != lastPrune {
		t.Fatal("first flush repeated startup pruning")
	}
}

func TestUnchangedConfigurationDoesNotFlushPendingRecords(t *testing.T) {
	actor := startupTestActor(t)
	if err := actor.record(normalizedUsage{
		RequestedAt: time.Now().UTC(),
		Dimensions:  Dimensions{Model: "test-model"},
		Counters:    Counters{Requests: 1, TotalTokens: 7},
	}); err != nil {
		t.Fatal(err)
	}
	lastPrune := actor.lastPruneAt
	var before int
	if err := actor.db.View(func(tx *bolt.Tx) error { before = tx.ID(); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := actor.reconfigure(actor.config, actor.crypto); err != nil {
		t.Fatal(err)
	}
	if len(actor.pendingRequests) != 1 || actor.lastPruneAt != lastPrune {
		t.Fatal("unchanged configuration flushed records or invalidated retention state")
	}
	if err := actor.db.View(func(tx *bolt.Tx) error {
		if tx.ID() != before {
			t.Fatal("unchanged configuration wrote a database transaction")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := actor.flush(time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	if err := actor.reload(); err != nil {
		t.Fatal(err)
	}
	if actor.nextRequestSeq != 1 {
		t.Fatal("pending request was not persisted by the subsequent flush")
	}
}

func TestConfigurationChangeOnlyInvalidatesPruningForRetention(t *testing.T) {
	actor := startupTestActor(t)
	lastPrune := actor.lastPruneAt
	changed := actor.config
	changed.FlushInterval = 2 * time.Hour
	if err := actor.reconfigure(changed, actor.crypto); err != nil {
		t.Fatal(err)
	}
	if actor.lastPruneAt != lastPrune {
		t.Fatal("flush interval change invalidated retention state")
	}
	changed.RetentionDays = 1
	if err := actor.reconfigure(changed, actor.crypto); err != nil {
		t.Fatal(err)
	}
	if !actor.lastPruneAt.IsZero() {
		t.Fatal("retention change did not schedule pruning")
	}
}

func TestStartupStillRejectsUnknownCryptoReferences(t *testing.T) {
	for _, bucket := range []string{"hours", "requests"} {
		t.Run(bucket, func(t *testing.T) {
			actor := startupTestActor(t)
			bad := Dimensions{APIKeyHash: strings.Repeat("a", 32), APIKeyGeneration: 999}
			if err := actor.db.Update(func(tx *bolt.Tx) error {
				if bucket == "requests" {
					payload, err := json.Marshal(RequestDetail{Dimensions: bad})
					if err != nil {
						return err
					}
					return tx.Bucket(requestsBucket).Put(encodeRequestKey(time.Now().UnixNano(), 1), payload)
				}
				hour, err := tx.Bucket(hoursBucket).CreateBucketIfNotExists(encodeInt64(time.Now().UTC().Truncate(time.Minute).Unix()))
				if err != nil {
					return err
				}
				key, err := json.Marshal(bad)
				if err != nil {
					return err
				}
				return hour.Put(key, []byte(`{"requests":1}`))
			}); err != nil {
				t.Fatal(err)
			}
			if err := actor.initialize(); err == nil || !strings.Contains(err.Error(), "unknown crypto generation") {
				t.Fatalf("invalid %s reference was accepted: %v", bucket, err)
			}
		})
	}
}

func TestRetentionWithoutExpiredRecordsKeepsKeyState(t *testing.T) {
	actor := startupTestActor(t)
	config := actor.config
	config.APIKeySecret = strings.Repeat("k", 32)
	crypto, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.reconfigure(config, crypto); err != nil {
		t.Fatal(err)
	}
	usage := encryptedUsageForGeneration(t, crypto, actor.activeGeneration, "test-key", "model", 7)
	if err := actor.record(usage); err != nil {
		t.Fatal(err)
	}
	if err := actor.flush(time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	ref := apiKeyRef(usage.Dimensions.APIKeyGeneration, usage.Dimensions.APIKeyHash)
	if err := actor.setAPIKeyLabel(ref, "label"); err != nil {
		t.Fatal(err)
	}
	actor.lastPruneAt = time.Time{}
	if err := actor.flush(time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	if actor.apiKeyCiphertexts[ref] != usage.Dimensions.APIKey || actor.apiKeyLabels[ref] != "label" {
		t.Fatal("an empty retention pass unnecessarily rebuilt key state")
	}
}

func BenchmarkStoreReconfigureUnchanged(b *testing.B) {
	config := Config{DataPath: filepath.Join(b.TempDir(), "usage.db"), RetentionDays: 30, FlushInterval: time.Hour, FlushMaxRecords: 20_000}
	store, err := openStore(config)
	if err != nil {
		b.Fatal(err)
	}
	for range 10_000 {
		if err := store.Record(normalizedUsage{RequestedAt: time.Now().UTC(), Dimensions: Dimensions{Model: "benchmark"}, Counters: Counters{Requests: 1}}); err != nil {
			b.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		b.Fatal(err)
	}
	defer func() { b.StopTimer(); _ = store.Close() }()
	if err := store.Reconfigure(config); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if err := store.Reconfigure(config); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStoreStartup(b *testing.B) {
	config := Config{DataPath: filepath.Join(b.TempDir(), "usage.db"), RetentionDays: 30, FlushInterval: time.Hour, FlushMaxRecords: 20_000}
	store, err := openStore(config)
	if err != nil {
		b.Fatal(err)
	}
	for range 10_000 {
		if err := store.Record(normalizedUsage{RequestedAt: time.Now().UTC(), Dimensions: Dimensions{Model: "benchmark"}, Counters: Counters{Requests: 1}}); err != nil {
			b.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		store, err := openStore(config)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
