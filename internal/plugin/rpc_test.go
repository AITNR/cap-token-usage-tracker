package plugin

import (
	"encoding/json"
	"math"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestSupportedRPCSchemaFitsSDK(t *testing.T) {
	if pluginabi.SchemaVersion < maxSupportedRPCSchema {
		t.Fatalf("CLIProxyAPI SDK schema version = %d, below supported plugin schema %d", pluginabi.SchemaVersion, maxSupportedRPCSchema)
	}
	if pluginabi.ABIVersion != 1 {
		t.Fatalf("CLIProxyAPI SDK ABI version = %d, want 1", pluginabi.ABIVersion)
	}
}

func TestRPCNegotiatesHostSchemaAndShutdown(t *testing.T) {
	tests := []struct {
		name       string
		hostSchema uint32
		wantSchema uint32
	}{
		{name: "missing legacy schema", hostSchema: 0, wantSchema: 1},
		{name: "schema 1", hostSchema: 1, wantSchema: 1},
		{name: "schema 2", hostSchema: 2, wantSchema: 2},
		{name: "schema 3", hostSchema: 3, wantSchema: 3},
		{name: "future schema 4", hostSchema: 4, wantSchema: 3},
		{name: "future schema 255", hostSchema: 255, wantSchema: 3},
		{name: "future max schema", hostSchema: math.MaxUint32, wantSchema: 3},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_ = runtimeState.shutdown()
			tempDir := t.TempDir()
			t.Cleanup(func() { _ = runtimeState.shutdown() })

			config := []byte("data_path: " + filepath.ToSlash(filepath.Join(tempDir, "rpc.db")) + "\n")
			request, err := json.Marshal(map[string]any{
				"config_yaml":           config,
				"schema_version":        test.hostSchema,
				"future_optional_field": map[string]any{"enabled": true},
			})
			if err != nil {
				t.Fatal(err)
			}

			for _, method := range []string{pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure} {
				var response rpcEnvelope
				if err := json.Unmarshal(dispatchRPC(method, request), &response); err != nil {
					t.Fatal(err)
				}
				if !response.OK || response.Error != nil {
					t.Fatalf("%s failed: %+v", method, response)
				}
				var registered registration
				if err := json.Unmarshal(response.Result, &registered); err != nil {
					t.Fatal(err)
				}
				if registered.SchemaVersion != test.wantSchema || !registered.Capabilities.UsagePlugin || !registered.Capabilities.ManagementAPI {
					t.Fatalf("unexpected %s registration: %+v", method, registered)
				}
				if registered.Metadata.GitHubRepository != "https://github.com/AITNR/cap-token-usage-tracker" {
					t.Fatalf("unexpected metadata: %+v", registered.Metadata)
				}
			}
		})
	}

	var response rpcEnvelope
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginShutdown, nil), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Error != nil {
		t.Fatalf("shutdown failed: %+v", response)
	}
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginShutdown, nil), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Error != nil {
		t.Fatalf("second shutdown failed: %+v", response)
	}
}

func TestRPCErrorEnvelopes(t *testing.T) {
	var response rpcEnvelope
	if err := json.Unmarshal(dispatchRPC("missing.method", nil), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil || response.Error.Code != "unknown_method" || response.Error.HTTPStatus != 404 {
		t.Fatalf("unexpected unknown method response: %+v", response)
	}

	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginRegister, []byte("not-json")), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil || response.Error.Code != "plugin_error" {
		t.Fatalf("unexpected malformed request response: %+v", response)
	}
}
