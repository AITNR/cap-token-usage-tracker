package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestDecodeLifecycleAcceptsConfigYAMLRepresentations(t *testing.T) {
	yamlConfig := []byte("retention_days: 45\nsync_on_record: true\n")
	standard, err := json.Marshal(lifecycleRequest{ConfigYAML: yamlConfig, SchemaVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := json.Marshal(map[string]any{
		"config_yaml":    string(yamlConfig),
		"schema_version": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	byteValues := make([]int, len(yamlConfig))
	for i, value := range yamlConfig {
		byteValues[i] = int(value)
	}
	array, err := json.Marshal(map[string]any{
		"config_yaml":    byteValues,
		"schema_version": 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{
		"base64": standard,
		"plain":  plain,
		"array":  array,
	} {
		t.Run(name, func(t *testing.T) {
			request, config, decodeErr := decodeLifecycle(raw)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if string(request.ConfigYAML) != string(yamlConfig) {
				t.Fatalf("config yaml = %q", request.ConfigYAML)
			}
			if config.RetentionDays != 45 || !config.SyncOnRecord {
				t.Fatalf("unexpected config: %+v", config)
			}
		})
	}
}

func TestDecodeLifecycleEmptyUsesReliableDefaults(t *testing.T) {
	_, config, err := decodeLifecycle(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !config.SyncOnRecord {
		t.Fatal("sync_on_record should default to true")
	}
}

func TestHandleUsageReliableModeWaitsForStoreCommit(t *testing.T) {
	store := newStoreMailbox()
	runtime := &pluginRuntime{store: store, config: Config{SyncOnRecord: true}}
	raw, err := json.Marshal(pluginapi.UsageRecord{
		Provider:    "test",
		Model:       "reliable",
		RequestedAt: time.Now().UTC(),
		Detail: pluginapi.UsageDetail{
			InputTokens:  2,
			OutputTokens: 3,
			TotalTokens:  5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, handleErr := runtime.handleUsage(raw)
		done <- handleErr
	}()

	var command recordCommand
	deadline := time.Now().Add(time.Second)
	for {
		store.queueMu.Lock()
		if len(store.queue)-store.queueHead == 1 {
			var ok bool
			command, ok = store.queue[store.queueHead].(recordCommand)
			store.queueMu.Unlock()
			if !ok {
				t.Fatalf("unexpected queued command: %#v", store.queue[store.queueHead])
			}
			break
		}
		store.queueMu.Unlock()
		if time.Now().After(deadline) {
			t.Fatal("usage command was not queued")
		}
		time.Sleep(time.Millisecond)
	}
	if command.resp == nil || !command.forceSync || command.usage.Counters.TotalTokens != 5 {
		t.Fatalf("unexpected reliable command: %#v", command)
	}

	select {
	case err := <-done:
		t.Fatalf("usage callback returned before persistence acknowledgement: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	command.resp <- nil
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("usage callback did not return after persistence acknowledgement")
	}
}

func TestHandleUsageBatchModeOnlyWaitsForMailboxAcceptance(t *testing.T) {
	store := newStoreMailbox()
	runtime := &pluginRuntime{store: store, config: Config{SyncOnRecord: false}}
	raw, err := json.Marshal(pluginapi.UsageRecord{
		Provider:    "test",
		Model:       "batch",
		RequestedAt: time.Now().UTC(),
		Detail: pluginapi.UsageDetail{
			InputTokens:  2,
			OutputTokens: 3,
			TotalTokens:  5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, handleErr := runtime.handleUsage(raw)
		done <- handleErr
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("batch-mode callback waited for the storage actor")
	}

	store.queueMu.Lock()
	defer store.queueMu.Unlock()
	if len(store.queue)-store.queueHead != 1 {
		t.Fatalf("queued commands = %d, want 1", len(store.queue)-store.queueHead)
	}
	command, ok := store.queue[store.queueHead].(recordCommand)
	if !ok || command.resp != nil || command.forceSync || command.usage.Counters.TotalTokens != 5 {
		t.Fatalf("unexpected batch command: %#v", store.queue[store.queueHead])
	}
}

func TestHandleUsageReliableModeReturnsAfterPersistence(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	raw, err := json.Marshal(pluginapi.UsageRecord{
		Provider:    "test",
		Model:       "committed-before-return",
		RequestedAt: time.Now().UTC(),
		Detail: pluginapi.UsageDetail{
			InputTokens:  2,
			OutputTokens: 3,
			TotalTokens:  5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.handleUsage(raw); err != nil {
		t.Fatal(err)
	}

	diagnostics := runtime.usageDiagnostics(store)
	if diagnostics.CallbacksReceived != 1 || diagnostics.Decoded != 1 || diagnostics.Enqueued != 1 || diagnostics.Processed != 1 || diagnostics.PersistedSinceOpen != 1 {
		t.Fatalf("unexpected reliable-mode diagnostics: %+v", diagnostics)
	}
	if diagnostics.MailboxDepth != 0 || diagnostics.PendingFlush != 0 || diagnostics.DecodeErrors != 0 || diagnostics.EnqueueErrors != 0 {
		t.Fatalf("reliable callback returned with backlog/errors: %+v", diagnostics)
	}
}

func TestUsageDiagnosticsTrackCallbackToPersistence(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = false
	config.FlushMaxRecords = 100_000
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	raw, err := json.Marshal(pluginapi.UsageRecord{
		Provider:    "test",
		Model:       "diagnostics",
		RequestedAt: time.Now().UTC(),
		Detail: pluginapi.UsageDetail{
			InputTokens:  2,
			OutputTokens: 3,
			TotalTokens:  5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	const records = 256
	for range records {
		if _, err := runtime.handleUsage(raw); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := runtime.handleUsage([]byte(`{"broken"`)); err == nil {
		t.Fatal("invalid usage JSON should fail")
	}

	// A request query is ordered after every usage command and forces all
	// processed request details to disk before returning.
	page, err := store.QueryRequests("24h", 0, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != records {
		t.Fatalf("persisted request total = %d, want %d", page.Total, records)
	}

	diagnostics := runtime.usageDiagnostics(store)
	if diagnostics.CallbacksReceived != records+1 || diagnostics.Decoded != records || diagnostics.Enqueued != records {
		t.Fatalf("unexpected callback diagnostics: %+v", diagnostics)
	}
	if diagnostics.Processed != records || diagnostics.PersistedSinceOpen != records {
		t.Fatalf("unexpected store diagnostics: %+v", diagnostics)
	}
	if diagnostics.MailboxDepth != 0 || diagnostics.PendingFlush != 0 || diagnostics.DecodeErrors != 1 || diagnostics.EnqueueErrors != 0 {
		t.Fatalf("unexpected diagnostic backlog/errors: %+v", diagnostics)
	}
}
