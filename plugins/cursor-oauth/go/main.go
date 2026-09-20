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

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}

static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) {
		return 1;
	}
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}

static void free_host_buffer(void* ptr, size_t len) {
	if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) {
		stored_host->free_buffer(ptr, len);
	}
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unsafe"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cursor-oauth/go/internal/plugin"
)

const abiVersion uint32 = 1

// pluginVersion is the released plugin version. The build sets it with
// -ldflags "-X main.pluginVersion=<version>" so a tag and the shipped library
// can never disagree; the literal here is the development default.
var pluginVersion = "0.6.1"

var handler = plugin.NewHandler(plugin.Dependencies{Emitter: cStreamEmitter{}, Host: cHostCaller{}})

func init() {
	plugin.PluginVersion = pluginVersion
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, api *C.cliproxy_plugin_api) C.int {
	if api == nil {
		return 1
	}
	C.store_host_api(host)
	api.abi_version = C.uint32_t(abiVersion)
	api.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	api.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	api.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, []byte(`{"ok":false,"error":{"code":"invalid_method","message":"method is required"}}`))
		return 1
	}
	var rawRequest []byte
	if request != nil && requestLen > 0 {
		rawRequest = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	rawResponse, ok := handler.CallWithStatus(context.Background(), C.GoString(method), rawRequest)
	writeResponse(response, rawResponse)
	if !ok {
		return 1
	}
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(pointer unsafe.Pointer, length C.size_t) {
	if pointer != nil {
		C.free(pointer)
	}
	_ = length
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

type cStreamEmitter struct{}

type cHostCaller struct{}

func (cHostCaller) Call(ctx context.Context, method string, requestValue any) (json.RawMessage, error) {
	return callHostRaw(ctx, method, requestValue)
}

func (cStreamEmitter) Emit(ctx context.Context, streamID string, payload []byte) error {
	request := struct {
		StreamID string `json:"stream_id"`
		Payload  []byte `json:"payload"`
	}{StreamID: streamID, Payload: payload}
	return callHost(ctx, "host.stream.emit", request)
}

func (cStreamEmitter) Close(streamID string, streamErr error) error {
	request := struct {
		StreamID string `json:"stream_id"`
		Error    string `json:"error,omitempty"`
	}{StreamID: streamID}
	if streamErr != nil {
		request.Error = streamErr.Error()
	}
	return callHost(context.Background(), "host.stream.close", request)
}

func callHost(ctx context.Context, method string, requestValue any) error {
	_, err := callHostRaw(ctx, method, requestValue)
	return err
}

func callHostRaw(ctx context.Context, method string, requestValue any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(requestValue)
	if err != nil {
		return nil, fmt.Errorf("encode host callback: %w", err)
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	request := C.CBytes(raw)
	defer C.free(request)
	var response C.cliproxy_buffer
	if code := C.call_host_api(cMethod, (*C.uint8_t)(request), C.size_t(len(raw)), &response); code != 0 {
		return nil, fmt.Errorf("host callback %s returned %d", method, code)
	}
	if response.ptr == nil {
		return nil, errors.New("host callback returned no response")
	}
	defer C.free_host_buffer(response.ptr, response.len)
	responseRaw := C.GoBytes(response.ptr, C.int(response.len))
	var envelope struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseRaw, &envelope); err != nil {
		return nil, fmt.Errorf("decode host callback response: %w", err)
	}
	if !envelope.OK {
		if envelope.Error != nil && envelope.Error.Message != "" {
			return nil, errors.New(envelope.Error.Message)
		}
		return nil, errors.New("host callback failed")
	}
	return append(json.RawMessage(nil), envelope.Result...), nil
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	pointer := C.CBytes(raw)
	if pointer == nil {
		return
	}
	response.ptr = pointer
	response.len = C.size_t(len(raw))
}
