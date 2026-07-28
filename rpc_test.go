package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestRPCAcceptsSchemaV2AndShutdown(t *testing.T) {
	const schemaVersion uint32 = 2
	if pluginabi.SchemaVersion != schemaVersion {
		t.Fatalf("CLIProxyAPI SDK schema version = %d, want %d", pluginabi.SchemaVersion, schemaVersion)
	}

	_ = runtimeState.shutdown()
	t.Cleanup(func() { _ = runtimeState.shutdown() })

	config := []byte("data_path: " + filepath.ToSlash(filepath.Join(t.TempDir(), "rpc.db")) + "\n")
	request, err := json.Marshal(lifecycleRequest{ConfigYAML: config, SchemaVersion: schemaVersion})
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
		if registered.SchemaVersion != schemaVersion || !registered.Capabilities.UsagePlugin || !registered.Capabilities.ManagementAPI {
			t.Fatalf("unexpected %s registration: %+v", method, registered)
		}
		if registered.Metadata.GitHubRepository != "https://github.com/AITNR/cap-token-usage-tracker" {
			t.Fatalf("unexpected metadata: %+v", registered.Metadata)
		}
	}

	var response rpcEnvelope
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginShutdown, nil), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("shutdown failed: %+v", response)
	}
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginShutdown, nil), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
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

func TestRPCRejectsSchemaV3(t *testing.T) {
	request, err := json.Marshal(lifecycleRequest{SchemaVersion: 3})
	if err != nil {
		t.Fatal(err)
	}

	for _, method := range []string{pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure} {
		var response rpcEnvelope
		if err := json.Unmarshal(dispatchRPC(method, request), &response); err != nil {
			t.Fatal(err)
		}
		if response.OK || response.Error == nil || response.Error.Code != "plugin_error" || response.Error.Message != "unsupported schema version 3" {
			t.Fatalf("%s accepted schema version 3: %+v", method, response)
		}
	}
}
