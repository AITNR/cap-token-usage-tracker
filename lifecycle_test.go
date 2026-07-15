package main

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestDecodeLifecycleAcceptsConfigYAMLRepresentations(t *testing.T) {
	dataDir := t.TempDir()
	yamlConfig := []byte("data_dir: " + filepath.ToSlash(dataDir) + "\nretention_days: 45\n")
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
	for index, value := range yamlConfig {
		byteValues[index] = int(value)
	}
	array, err := json.Marshal(map[string]any{
		"config_yaml":    byteValues,
		"schema_version": 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, raw := range map[string][]byte{"base64": standard, "plain": plain, "array": array} {
		t.Run(name, func(t *testing.T) {
			request, config, decodeErr := decodeLifecycle(raw)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if string(request.ConfigYAML) != string(yamlConfig) {
				t.Fatalf("config yaml = %q", request.ConfigYAML)
			}
			if config.RetentionDays != 45 || config.DataPath != filepath.Join(dataDir, defaultDatabaseFile) {
				t.Fatalf("unexpected config: %+v", config)
			}
		})
	}
}

func TestDecodeLifecycleEmptyUsesReferenceDefaults(t *testing.T) {
	t.Setenv("CAP_TOKEN_USAGE_TRACKER_DIR", "")
	t.Setenv("USAGE_STATISTICS_DIR", "")
	_, config, err := decodeLifecycle(nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.RetentionDays != 0 {
		t.Fatalf("retention days = %d, want 0", config.RetentionDays)
	}
	if filepath.Base(config.DataPath) != defaultDatabaseFile || !strings.Contains(filepath.ToSlash(config.DataPath), "/.cli-proxy-api/plugins/cap-token-usage-tracker/") {
		t.Fatalf("unexpected default data path: %q", config.DataPath)
	}
}

func TestParseConfigSupportsAliasesAndRejectsAmbiguousPaths(t *testing.T) {
	dir := t.TempDir()
	config, err := parseConfig([]byte("数据目录: " + filepath.ToSlash(dir) + "\n用量保留天数: 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.DataPath != filepath.Join(dir, defaultDatabaseFile) || config.RetentionDays != 7 {
		t.Fatalf("alias config = %+v", config)
	}

	legacyPath := filepath.Join(t.TempDir(), "legacy-name.db")
	config, err = parseConfig([]byte("data_path: " + filepath.ToSlash(legacyPath) + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	absoluteLegacyPath, _ := filepath.Abs(legacyPath)
	if config.DataPath != absoluteLegacyPath {
		t.Fatalf("legacy data_path = %q, want %q", config.DataPath, absoluteLegacyPath)
	}
	if _, err := parseConfig([]byte("data_dir: one\ndata_path: two\n")); err == nil {
		t.Fatal("data_dir and data_path were accepted together")
	}
	if _, err := parseConfig([]byte("retention_days: -1\n")); err == nil {
		t.Fatal("negative retention was accepted")
	}
}

func TestPluginRegistrationExposesReferenceStorageConfig(t *testing.T) {
	registration := pluginRegistration()
	if !registration.Capabilities.UsagePlugin || !registration.Capabilities.ManagementAPI {
		t.Fatalf("capabilities = %+v", registration.Capabilities)
	}
	fields := registration.Metadata.ConfigFields
	if len(fields) != 2 || fields[0].Name != "data_dir" || fields[1].Name != "retention_days" {
		t.Fatalf("config fields = %+v", fields)
	}
	for _, field := range fields {
		if strings.Contains(field.Name, "flush") || strings.Contains(field.Name, "sync") {
			t.Fatalf("legacy batching config field remains: %+v", field)
		}
	}
}

func TestRegisterRejectsNewerSchema(t *testing.T) {
	runtime := &pluginRuntime{}
	raw, err := json.Marshal(lifecycleRequest{SchemaVersion: pluginabi.SchemaVersion + 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.register(raw); err == nil {
		t.Fatal("newer schema version was accepted")
	}
}

func TestHandleUsageSynchronouslyPersistsAndIgnoresMalformedCallbacks(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()

	requestedAt := time.Now().UTC().Add(-time.Second)
	raw, err := json.Marshal(pluginapi.UsageRecord{
		Provider:    "test-provider",
		Model:       "committed-before-return",
		APIKey:      "client-key",
		AuthID:      "credential-id",
		RequestedAt: requestedAt,
		Latency:     2 * time.Second,
		TTFT:        200 * time.Millisecond,
		Detail: pluginapi.UsageDetail{
			InputTokens:  2,
			OutputTokens: 3,
			TotalTokens:  5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.handleUsage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if stored, _ := result["stored"].(bool); !stored {
		t.Fatalf("usage result = %+v", result)
	}

	// The query immediately after handleUsage must see the committed row; there
	// is no mailbox or deferred flush stage.
	usage, err := store.QueryUsage(t.Context(), QueryRange{})
	if err != nil {
		t.Fatal(err)
	}
	details := usage["client-key"]["committed-before-return"]
	if len(details) != 1 || details[0].Tokens.TotalTokens != 5 || !details[0].Timestamp.Equal(requestedAt) {
		t.Fatalf("persisted usage = %+v", usage)
	}

	for _, invalid := range [][]byte{nil, []byte(`{"broken"`)} {
		result, err := runtime.handleUsage(invalid)
		if err != nil {
			t.Fatal(err)
		}
		if ignored, _ := result["ignored"].(bool); !ignored {
			t.Fatalf("invalid callback result = %+v", result)
		}
	}
	diagnostics := runtime.usageDiagnostics(store)
	if diagnostics.CallbacksReceived != 3 || diagnostics.Decoded != 1 || diagnostics.Enqueued != 1 || diagnostics.DecodeErrors != 1 || diagnostics.EnqueueErrors != 0 {
		t.Fatalf("callback diagnostics = %+v", diagnostics)
	}
	if diagnostics.Processed != 1 || diagnostics.PersistedSinceOpen != 1 || diagnostics.MailboxDepth != 0 || diagnostics.PendingFlush != 0 {
		t.Fatalf("store diagnostics = %+v", diagnostics)
	}
}

func TestHandleUsageWithoutStoreIsIgnored(t *testing.T) {
	runtime := &pluginRuntime{}
	raw, err := json.Marshal(pluginapi.UsageRecord{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.handleUsage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ignored, _ := result["ignored"].(bool); !ignored {
		t.Fatalf("result = %+v", result)
	}
	diagnostics := runtime.usageDiagnostics(nil)
	if diagnostics.CallbacksReceived != 1 || diagnostics.Decoded != 1 || diagnostics.Enqueued != 0 {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestRuntimeRegisterAndReconfigureDataDirectory(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	runtime := &pluginRuntime{}
	defer runtime.shutdown()

	registerRaw, err := json.Marshal(map[string]any{
		"config_yaml":    base64.StdEncoding.EncodeToString([]byte("data_dir: " + filepath.ToSlash(firstDir) + "\nretention_days: 0\n")),
		"schema_version": pluginabi.SchemaVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.register(registerRaw); err != nil {
		t.Fatal(err)
	}
	firstStore := runtime.store
	if firstStore == nil || runtime.config.DataPath != filepath.Join(firstDir, defaultDatabaseFile) {
		t.Fatalf("runtime after register = %+v", runtime.config)
	}

	sameRaw, _ := json.Marshal(map[string]any{
		"config_yaml":    "data_dir: " + filepath.ToSlash(firstDir) + "\nretention_days: 3\n",
		"schema_version": pluginabi.SchemaVersion,
	})
	if _, err := runtime.reconfigure(sameRaw); err != nil {
		t.Fatal(err)
	}
	if runtime.store != firstStore || runtime.config.RetentionDays != 3 {
		t.Fatalf("same-path reconfigure replaced store or missed retention: %+v", runtime.config)
	}

	otherRaw, _ := json.Marshal(map[string]any{
		"config_yaml":    "data_dir: " + filepath.ToSlash(secondDir) + "\n",
		"schema_version": pluginabi.SchemaVersion,
	})
	if _, err := runtime.reconfigure(otherRaw); err != nil {
		t.Fatal(err)
	}
	if runtime.store == firstStore || runtime.config.DataPath != filepath.Join(secondDir, defaultDatabaseFile) {
		t.Fatalf("different-path reconfigure did not replace store: %+v", runtime.config)
	}
	if err := firstStore.db.Ping(); err == nil {
		t.Fatal("previous store remained open after data_dir changed")
	}
}
