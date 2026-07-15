package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var version = "dev"

type lifecycleRequest struct {
	ConfigYAML    []byte `json:"config_yaml"`
	SchemaVersion uint32 `json:"schema_version"`
}

type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}

type registrationCapabilities struct {
	UsagePlugin   bool `json:"usage_plugin"`
	ManagementAPI bool `json:"management_api"`
}

type pluginRuntime struct {
	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	store       *Store
	config      Config
	routes      registeredRoutes

	usageCallbacks atomic.Uint64
	usageDecoded   atomic.Uint64
	usageEnqueued  atomic.Uint64
	decodeErrors   atomic.Uint64
	enqueueErrors  atomic.Uint64
}

var runtimeState = &pluginRuntime{}

func (r *pluginRuntime) register(raw []byte) (registration, error) {
	request, config, err := decodeLifecycle(raw)
	if err != nil {
		return registration{}, err
	}
	if request.SchemaVersion > pluginabi.SchemaVersion {
		return registration{}, fmt.Errorf("unsupported schema version %d", request.SchemaVersion)
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if err := r.applyConfig(config); err != nil {
		return registration{}, err
	}
	return pluginRegistration(), nil
}

func (r *pluginRuntime) reconfigure(raw []byte) (registration, error) {
	request, config, err := decodeLifecycle(raw)
	if err != nil {
		return registration{}, err
	}
	if request.SchemaVersion > pluginabi.SchemaVersion {
		return registration{}, fmt.Errorf("unsupported schema version %d", request.SchemaVersion)
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()
	if err := r.applyConfig(config); err != nil {
		return registration{}, err
	}
	return pluginRegistration(), nil
}

func (r *pluginRuntime) applyConfig(config Config) error {
	r.mu.RLock()
	current := r.store
	currentConfig := r.config
	r.mu.RUnlock()

	if current != nil && currentConfig.DataPath == config.DataPath {
		if err := current.Reconfigure(config); err != nil {
			return err
		}
		r.mu.Lock()
		r.config = config
		r.mu.Unlock()
		return nil
	}

	next, err := openStore(config)
	if err != nil {
		return err
	}
	r.mu.Lock()
	old := r.store
	r.store = next
	r.config = config
	r.mu.Unlock()
	if old != nil {
		if err := old.Close(); err != nil {
			return fmt.Errorf("close previous store: %w", err)
		}
	}
	return nil
}

// handleUsage follows the reference plugin: malformed/empty callbacks are
// ignored, while a valid callback is acknowledged only after its SQLite INSERT
// has completed.
func (r *pluginRuntime) handleUsage(raw []byte) (map[string]any, error) {
	r.usageCallbacks.Add(1)
	if len(raw) == 0 {
		return map[string]any{"ignored": true}, nil
	}
	record, err := decodeUsage(raw, nowUTC())
	if err != nil {
		r.decodeErrors.Add(1)
		return map[string]any{"ignored": true}, nil
	}
	r.usageDecoded.Add(1)

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return map[string]any{"ignored": true}, nil
	}
	if err := r.store.Record(record); err != nil {
		r.enqueueErrors.Add(1)
		return nil, err
	}
	r.usageEnqueued.Add(1)
	return map[string]any{"stored": true}, nil
}

func (r *pluginRuntime) usageDiagnostics(store *Store) UsageDiagnostics {
	diagnostics := UsageDiagnostics{
		CallbacksReceived: r.usageCallbacks.Load(),
		Decoded:           r.usageDecoded.Load(),
		Enqueued:          r.usageEnqueued.Load(),
		DecodeErrors:      r.decodeErrors.Load(),
		EnqueueErrors:     r.enqueueErrors.Load(),
	}
	if store != nil {
		storeDiagnostics := store.Diagnostics()
		diagnostics.Processed = storeDiagnostics.Processed
		diagnostics.PersistedSinceOpen = storeDiagnostics.PersistedSinceOpen
	}
	return diagnostics
}

func (r *pluginRuntime) shutdown() error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.Lock()
	store := r.store
	r.store = nil
	r.config = Config{}
	r.routes = registeredRoutes{}
	r.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.Close()
}

func decodeLifecycle(raw []byte) (lifecycleRequest, Config, error) {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	var envelope struct {
		ConfigYAML    json.RawMessage `json:"config_yaml"`
		SchemaVersion uint32          `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return lifecycleRequest{}, Config{}, fmt.Errorf("decode lifecycle request: %w", err)
	}
	configYAML, err := decodeLifecycleConfigYAML(envelope.ConfigYAML)
	if err != nil {
		return lifecycleRequest{}, Config{}, err
	}
	request := lifecycleRequest{ConfigYAML: configYAML, SchemaVersion: envelope.SchemaVersion}
	config, err := parseConfig(request.ConfigYAML)
	if err != nil {
		return lifecycleRequest{}, Config{}, err
	}
	return request, config, nil
}

// decodeLifecycleConfigYAML accepts the host's standard base64 encoding of a
// []byte field, while remaining compatible with plain YAML and JSON byte arrays.
func decodeLifecycleConfigYAML(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if decoded, decodeErr := base64.StdEncoding.DecodeString(text); decodeErr == nil && strings.Contains(string(decoded), ":") {
			return decoded, nil
		}
		return []byte(text), nil
	}
	var bytes []byte
	if err := json.Unmarshal(raw, &bytes); err == nil {
		return bytes, nil
	}
	return nil, fmt.Errorf("config_yaml must be a base64/plain string or byte array")
}

func pluginRegistration() registration {
	return registration{
		SchemaVersion: pluginabi.SchemaVersion,
		Metadata: pluginapi.Metadata{
			Name:             "CAP Token Usage Tracker",
			Version:          version,
			Author:           "AITNR",
			GitHubRepository: "https://github.com/AITNR/cap-token-usage-tracker",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "data_dir", Type: pluginapi.ConfigFieldTypeString, Description: "Directory containing usage.db. Defaults to ~/.cli-proxy-api/plugins/cap-token-usage-tracker."},
				{Name: "retention_days", Type: pluginapi.ConfigFieldTypeInteger, Description: "Delete request rows older than this many days; 0 disables cleanup."},
			},
		},
		Capabilities: registrationCapabilities{UsagePlugin: true, ManagementAPI: true},
	}
}

var nowUTC = func() time.Time { return time.Now().UTC() }
