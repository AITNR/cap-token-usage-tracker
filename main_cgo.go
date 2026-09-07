//go:build cgo

package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

extern int cliproxy_plugin_call_bridge(const char*, const uint8_t*, size_t, cliproxy_buffer*);
extern int cliproxy_host_call_bridge(cliproxy_host_api*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxy_host_free_bridge(cliproxy_host_api*, void*, size_t);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"unsafe"

	pluginabi "github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	pluginapi "github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"

	"github.com/AITNR/cap-token-usage-tracker/internal/plugin"
)

var hostAPICallbackState struct {
	sync.Mutex
	cond     *sync.Cond
	host     *C.cliproxy_host_api
	inFlight int
}

func init() {
	hostAPICallbackState.cond = sync.NewCond(&hostAPICallbackState.Mutex)
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, pluginAPI *C.cliproxy_plugin_api) (result C.int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: plugin initialization panic")
			result = 3
		}
	}()
	if pluginAPI == nil {
		return 1
	}
	if host != nil && uint32(host.abi_version) != pluginabi.ABIVersion {
		return 2
	}
	setHostAPI(host)
	plugin.RuntimeState.SetAuthRuntimeLookup(hostRuntimeAuthLookup)
	pluginAPI.abi_version = C.uint32_t(pluginabi.ABIVersion)
	pluginAPI.call = C.cliproxy_plugin_call_fn(C.cliproxy_plugin_call_bridge)
	pluginAPI.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	pluginAPI.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) (result C.int) {
	if response == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0

	defer func() {
		if recovered := recover(); recovered != nil {
			if !writeCResponse(response, plugin.MarshalError("plugin_panic", "plugin call failed", false, 500)) {
				result = 2
				return
			}
			result = 0
		}
	}()

	if method == nil {
		if !writeCResponse(response, plugin.MarshalError("invalid_method", "method is required", false, 400)) {
			return 2
		}
		return 0
	}
	if uint64(requestLen) > uint64(1<<31-1) {
		if !writeCResponse(response, plugin.MarshalError("request_too_large", "request is too large", false, 413)) {
			return 2
		}
		return 0
	}

	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	if !writeCResponse(response, plugin.DispatchRPC(C.GoString(method), requestBytes)) {
		return 2
	}
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: buffer release panic")
		}
	}()
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: shutdown panic")
		}
	}()
	clearHostAPI()
	if err := plugin.RuntimeState.Shutdown(); err != nil {
		fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: shutdown persistence error:", err)
	}
}

func setHostAPI(host *C.cliproxy_host_api) {
	hostAPICallbackState.Lock()
	hostAPICallbackState.host = host
	hostAPICallbackState.Unlock()
}

func clearHostAPI() {
	hostAPICallbackState.Lock()
	hostAPICallbackState.host = nil
	for hostAPICallbackState.inFlight != 0 {
		hostAPICallbackState.cond.Wait()
	}
	hostAPICallbackState.Unlock()
}

func hostRuntimeAuthLookup(authIndex string) (plugin.AuthRuntimeMetadata, error) {
	request, err := json.Marshal(pluginapi.HostAuthGetRequest{AuthIndex: authIndex})
	if err != nil {
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("encode runtime auth request: %w", err)
	}

	hostAPICallbackState.Lock()
	host := hostAPICallbackState.host
	if host == nil {
		hostAPICallbackState.Unlock()
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("host runtime auth API is unavailable")
	}
	hostAPICallbackState.inFlight++
	hostAPICallbackState.Unlock()
	defer func() {
		hostAPICallbackState.Lock()
		hostAPICallbackState.inFlight--
		if hostAPICallbackState.inFlight == 0 {
			hostAPICallbackState.cond.Broadcast()
		}
		hostAPICallbackState.Unlock()
	}()

	method := C.CString(pluginabi.MethodHostAuthGetRuntime)
	defer C.free(unsafe.Pointer(method))
	var requestPtr *C.uint8_t
	if len(request) > 0 {
		requestPtr = (*C.uint8_t)(C.CBytes(request))
		defer C.free(unsafe.Pointer(requestPtr))
	}
	var response C.cliproxy_buffer
	if result := C.cliproxy_host_call_bridge(host, method, requestPtr, C.size_t(len(request)), &response); result != 0 {
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("host runtime auth call failed: %d", int(result))
	}
	if response.ptr == nil || response.len == 0 || uint64(response.len) > uint64(1<<31-1) {
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("host runtime auth response is invalid")
	}
	defer C.cliproxy_host_free_bridge(host, response.ptr, response.len)

	var envelope plugin.RPCEnvelope
	if err := json.Unmarshal(C.GoBytes(response.ptr, C.int(response.len)), &envelope); err != nil {
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("decode runtime auth response: %w", err)
	}
	if !envelope.OK {
		if envelope.Error != nil && envelope.Error.Message != "" {
			return plugin.AuthRuntimeMetadata{}, fmt.Errorf("runtime auth lookup failed: %s", envelope.Error.Message)
		}
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("runtime auth lookup failed")
	}
	var responseData pluginapi.HostAuthGetRuntimeResponse
	if err := json.Unmarshal(envelope.Result, &responseData); err != nil {
		return plugin.AuthRuntimeMetadata{}, fmt.Errorf("decode runtime auth metadata: %w", err)
	}
	return plugin.AuthRuntimeMetadata{
		Provider:    responseData.Auth.Provider,
		Type:        responseData.Auth.Type,
		Email:       responseData.Auth.Email,
		AccountType: responseData.Auth.AccountType,
		Account:     responseData.Auth.Account,
		Label:       responseData.Auth.Label,
	}, nil
}

func writeCResponse(response *C.cliproxy_buffer, raw []byte) bool {
	if response == nil || len(raw) == 0 {
		return false
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return false
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
	return true
}
