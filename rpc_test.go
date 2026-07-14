package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

func TestRPCRegistrationAndShutdown(t *testing.T) {
	_ = runtimeState.shutdown()
	t.Cleanup(func() { _ = runtimeState.shutdown() })

	config := []byte("data_path: " + filepath.ToSlash(filepath.Join(t.TempDir(), "rpc.db")) + "\n")
	request, err := json.Marshal(lifecycleRequest{ConfigYAML: config, SchemaVersion: pluginabi.SchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	var response rpcEnvelope
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginRegister, request), &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.Error != nil {
		t.Fatalf("registration failed: %+v", response)
	}
	var registered registration
	if err := json.Unmarshal(response.Result, &registered); err != nil {
		t.Fatal(err)
	}
	if registered.SchemaVersion != pluginabi.SchemaVersion || !registered.Capabilities.UsagePlugin || !registered.Capabilities.ManagementAPI {
		t.Fatalf("unexpected registration: %+v", registered)
	}
	if registered.Metadata.GitHubRepository != "https://github.com/AITNR/cap-token-usage-tracker" {
		t.Fatalf("unexpected metadata: %+v", registered.Metadata)
	}

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

func TestRPCRejectsNewerSchema(t *testing.T) {
	request, _ := json.Marshal(lifecycleRequest{SchemaVersion: pluginabi.SchemaVersion + 1})
	var response rpcEnvelope
	if err := json.Unmarshal(dispatchRPC(pluginabi.MethodPluginRegister, request), &response); err != nil {
		t.Fatal(err)
	}
	if response.OK || response.Error == nil {
		t.Fatalf("newer schema was accepted: %+v", response)
	}
}
