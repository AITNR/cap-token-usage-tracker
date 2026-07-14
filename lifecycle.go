package main

import (
	"encoding/json"
	"fmt"
	"sync"
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

func (r *pluginRuntime) handleUsage(raw []byte) (map[string]any, error) {
	usage, err := decodeUsage(raw, nowUTC())
	if err != nil {
		return nil, withStatus(400, "%v", err)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.store == nil {
		return nil, withStatus(503, "plugin storage is not initialized")
	}
	if err := r.store.Record(usage); err != nil {
		return nil, err
	}
	return map[string]any{}, nil
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
	var request lifecycleRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return lifecycleRequest{}, Config{}, fmt.Errorf("decode lifecycle request: %w", err)
	}
	config, err := parseConfig(request.ConfigYAML)
	if err != nil {
		return lifecycleRequest{}, Config{}, err
	}
	return request, config, nil
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
				{Name: "data_path", Type: pluginapi.ConfigFieldTypeString, Description: "bbolt database path; relative paths use the CLIProxyAPI working directory."},
				{Name: "retention_days", Type: pluginapi.ConfigFieldTypeInteger, Description: "Number of UTC days of hourly statistics to retain (1-3650)."},
				{Name: "flush_interval", Type: pluginapi.ConfigFieldTypeString, Description: "Maximum delay before batched statistics are flushed, for example 5s."},
				{Name: "flush_max_records", Type: pluginapi.ConfigFieldTypeInteger, Description: "Flush after this many accepted usage records."},
				{Name: "sync_on_record", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Commit every usage record before acknowledging it."},
			},
		},
		Capabilities: registrationCapabilities{UsagePlugin: true, ManagementAPI: true},
	}
}

var nowUTC = func() time.Time { return time.Now().UTC() }
