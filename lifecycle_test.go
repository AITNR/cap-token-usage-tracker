package main

import (
	"encoding/json"
	"testing"
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
