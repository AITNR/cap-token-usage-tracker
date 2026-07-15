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

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
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

*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) (result C.int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: plugin initialization panic")
			result = 3
		}
	}()
	if plugin == nil {
		return 1
	}
	if host != nil && uint32(host.abi_version) != pluginabi.ABIVersion {
		return 2
	}
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
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
			if !writeCResponse(response, marshalError("plugin_panic", "plugin call failed", false, 500)) {
				result = 2
				return
			}
			result = 0
		}
	}()

	if method == nil {
		if !writeCResponse(response, marshalError("invalid_method", "method is required", false, 400)) {
			return 2
		}
		return 0
	}
	if uint64(requestLen) > uint64(1<<31-1) {
		if !writeCResponse(response, marshalError("request_too_large", "request is too large", false, 413)) {
			return 2
		}
		return 0
	}

	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	if !writeCResponse(response, dispatchRPC(C.GoString(method), requestBytes)) {
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
	if err := runtimeState.shutdown(); err != nil {
		fmt.Fprintln(os.Stderr, "cap-token-usage-tracker: shutdown persistence error:", err)
	}
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
