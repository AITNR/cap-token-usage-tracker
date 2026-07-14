package main

import (
	"encoding/json"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

type rpcEnvelope struct {
	OK     bool             `json:"ok"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *pluginabi.Error `json:"error,omitempty"`
}

func dispatchRPC(method string, request []byte) []byte {
	var result any
	var err error

	switch method {
	case pluginabi.MethodPluginRegister:
		result, err = runtimeState.register(request)
	case pluginabi.MethodPluginReconfigure:
		result, err = runtimeState.reconfigure(request)
	case pluginabi.MethodUsageHandle:
		result, err = runtimeState.handleUsage(request)
	case pluginabi.MethodManagementRegister:
		result, err = runtimeState.registerManagement(request)
	case pluginabi.MethodManagementHandle:
		result, err = runtimeState.handleManagement(request)
	case pluginabi.MethodPluginShutdown:
		err = runtimeState.shutdown()
		result = map[string]any{}
	default:
		return marshalError("unknown_method", "unknown method: "+method, false, 404)
	}

	if err != nil {
		return marshalError("plugin_error", err.Error(), false, errorHTTPStatus(err))
	}
	return marshalOK(result)
}

func marshalOK(value any) []byte {
	result, err := json.Marshal(value)
	if err != nil {
		return marshalError("marshal_error", err.Error(), false, 500)
	}
	raw, err := json.Marshal(rpcEnvelope{OK: true, Result: result})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"marshal_error","message":"failed to encode response"}}`)
	}
	return raw
}

func marshalError(code, message string, retryable bool, status int) []byte {
	raw, err := json.Marshal(rpcEnvelope{
		OK: false,
		Error: &pluginabi.Error{
			Code:       code,
			Message:    message,
			Retryable:  retryable,
			HTTPStatus: status,
		},
	})
	if err != nil {
		return []byte(`{"ok":false,"error":{"code":"marshal_error","message":"failed to encode error"}}`)
	}
	return raw
}

type statusError struct {
	status int
	err    error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

func withStatus(status int, format string, args ...any) error {
	return &statusError{status: status, err: fmt.Errorf(format, args...)}
}

func errorHTTPStatus(err error) int {
	var target *statusError
	if err != nil && asStatusError(err, &target) {
		return target.status
	}
	return 500
}

func asStatusError(err error, target **statusError) bool {
	for err != nil {
		if current, ok := err.(*statusError); ok {
			*target = current
			return true
		}
		type unwrapper interface{ Unwrap() error }
		unwrapped, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = unwrapped.Unwrap()
	}
	return false
}
