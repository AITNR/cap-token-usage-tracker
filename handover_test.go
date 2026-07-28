package main

import (
	"testing"
	"time"
)

func TestOpenStoreHandsDatabaseToNewPluginInstance(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	first, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })

	usage := normalizedUsage{
		Dimensions:  Dimensions{Provider: "provider", Model: "model"},
		RequestedAt: time.Now().UTC(),
		Counters:    Counters{Requests: 1, TotalTokens: 17},
	}
	if err := first.Record(usage); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	second, err := openStore(config)
	if err != nil {
		t.Fatalf("open replacement store: %v", err)
	}
	defer second.Close()
	if elapsed := time.Since(started); elapsed >= storeOpenHandoverTimeout {
		t.Fatalf("database handover took %v", elapsed)
	}

	if _, err := first.Query("retention"); err == nil || err.Error() != "store is closed" {
		t.Fatalf("retired store query error = %v, want store is closed", err)
	}
	stats, err := second.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 1 || stats.Summary.TotalTokens != 17 {
		t.Fatalf("replacement store lost persisted usage: %+v", stats.Summary)
	}
	if second.lease == nil || !second.lease.current() {
		t.Fatal("replacement store does not own the handover lease")
	}
}
