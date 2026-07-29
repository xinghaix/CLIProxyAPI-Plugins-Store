package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);
typedef struct { uint32_t abi_version; void* host_ctx; cliproxy_host_call_fn call; cliproxy_host_free_fn free_buffer; } cliproxy_host_api;
typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);
typedef struct { uint32_t abi_version; cliproxy_plugin_call_fn call; cliproxy_plugin_free_fn free_buffer; cliproxy_plugin_shutdown_fn shutdown; } cliproxy_plugin_api;
static const cliproxy_host_api* stored_host;
static void store_host_api(const cliproxy_host_api* host) { stored_host = host; }
static int call_host_api(const char* method, const uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (stored_host == NULL || stored_host->call == NULL) return 1;
	return stored_host->call(stored_host->host_ctx, method, request, request_len, response);
}
static void free_host_buffer(void* ptr, size_t len) { if (stored_host != NULL && stored_host->free_buffer != NULL && ptr != NULL) stored_host->free_buffer(ptr, len); }
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/api"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
)

var pluginVersion = "0.5.0"

const (
	// supportedPluginSchemaVersion 保持为 1，确保插件可加载于 schema 1 和 schema 2 host。
	supportedPluginSchemaVersion uint32 = 1

	managementHealthPathRel = "/cpa-manager-plus/health"
	managementAPIPathRel    = "/cpa-manager-plus/api"
	managementHealthPathAbs = "/v0/management/cpa-manager-plus/health"
	managementAPIPathAbs    = "/v0/management/cpa-manager-plus/api"
	resourceAppPath         = "/v0/resource/plugins/cpa-manager-plus/app"
	contentTypeJSON         = "application/json; charset=utf-8"
	contentTypeHTML         = "text/html; charset=utf-8"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}
type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type lifecycleRequest struct {
	ConfigYAML []byte `json:"config_yaml"`
}
type registration struct {
	SchemaVersion uint32                   `json:"schema_version"`
	Metadata      pluginapi.Metadata       `json:"metadata"`
	Capabilities  registrationCapabilities `json:"capabilities"`
}
type registrationCapabilities struct {
	ManagementAPI bool `json:"management_api"`
	UsagePlugin   bool `json:"usage_plugin"`
}
type managementRegistrationResponse struct {
	Routes    []pluginapi.ManagementRoute `json:"routes,omitempty"`
	Resources []pluginapi.ResourceRoute   `json:"resources,omitempty"`
}
type managementRequest struct {
	pluginapi.ManagementRequest
	HostCallbackID string `json:"host_callback_id,omitempty"`
}
type managementResponse struct {
	StatusCode int         `json:"StatusCode"`
	Headers    http.Header `json:"Headers"`
	Body       []byte      `json:"Body"`
}

var runtimeState struct {
	sync.Mutex
	runtime *app.Runtime
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var raw []byte
	if request != nil && requestLen > 0 {
		raw = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	result, err := handleMethod(C.GoString(method), raw)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, result)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {
	runtimeState.Lock()
	runtime := runtimeState.runtime
	runtimeState.runtime = nil
	runtimeState.Unlock()
	if runtime != nil {
		_ = runtime.Close()
	}
}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if err := configure(request); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodUsageHandle:
		var record pluginapi.UsageRecord
		if err := json.Unmarshal(request, &record); err != nil {
			return nil, err
		}
		if runtime := currentRuntime(); runtime != nil {
			runtime.HandleUsage(record)
		}
		return okEnvelope(map[string]any{})
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) error {
	var request lifecycleRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &request); err != nil {
			return err
		}
	}
	runtimeState.Lock()
	defer runtimeState.Unlock()
	if runtimeState.runtime == nil {
		runtime, err := app.New(request.ConfigYAML)
		if err != nil {
			return err
		}
		runtime.SetAuthList(listHostAuth)
		runtime.SetHTTPDo(hostHTTPDo)
		runtimeState.runtime = runtime
		return nil
	}
	return runtimeState.runtime.Reconfigure(request.ConfigYAML)
}

func currentRuntime() *app.Runtime {
	runtimeState.Lock()
	defer runtimeState.Unlock()
	return runtimeState.runtime
}

func pluginRegistration() registration {
	return registration{SchemaVersion: supportedPluginSchemaVersion, Metadata: pluginapi.Metadata{Name: "CPA Manager Plus", Version: pluginVersion, Author: "xinghaix", GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store", ConfigFields: []pluginapi.ConfigField{
		{Name: "data_dir", Type: pluginapi.ConfigFieldTypeString, Description: "本地 SQLite 数据目录；为空时使用 data/cpa-manager-plus"},
		{Name: "queue_capacity", Type: pluginapi.ConfigFieldTypeInteger, Description: "异步用量写入队列容量（1-65536）"},
		{Name: "batch_size", Type: pluginapi.ConfigFieldTypeInteger, Description: "SQLite 批量写入大小（1-1024）"},
	}}, Capabilities: registrationCapabilities{ManagementAPI: true, UsagePlugin: true}}
}

func managementRegistration() managementRegistrationResponse {
	return managementRegistrationResponse{Routes: []pluginapi.ManagementRoute{{Method: http.MethodGet, Path: managementHealthPathRel}, {Method: http.MethodPost, Path: managementAPIPathRel}}, Resources: []pluginapi.ResourceRoute{{Path: "/app", Menu: "CPA Manager Plus", Description: "本地 SQLite 用量监控、价格与账号巡检"}}}
}

func handleManagement(raw []byte) ([]byte, error) {
	var request managementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	requestPath := strings.TrimRight(strings.TrimSpace(request.Path), "/")
	if requestPath == "" {
		requestPath = "/"
	}
	if strings.EqualFold(request.Method, http.MethodGet) && strings.HasPrefix(requestPath, "/v0/resource/plugins/cpa-manager-plus") {
		return okEnvelope(handleResource(requestPath))
	}
	runtime := currentRuntime()
	if runtime == nil {
		return okEnvelope(jsonResponse(http.StatusServiceUnavailable, map[string]any{"error": "local runtime is not initialized"}))
	}
	if strings.EqualFold(request.Method, http.MethodGet) && (requestPath == managementHealthPathRel || requestPath == managementHealthPathAbs) {
		return okEnvelope(jsonResponse(http.StatusOK, runtime.Health(context.Background())))
	}
	if strings.EqualFold(request.Method, http.MethodPost) && (requestPath == managementAPIPathRel || requestPath == managementAPIPathAbs) {
		response := api.Handle(context.Background(), runtime, request.Body)
		return okEnvelope(managementResponse{StatusCode: response.StatusCode, Headers: response.Headers, Body: response.Body})
	}
	return okEnvelope(jsonResponse(http.StatusNotFound, map[string]any{"error": "plugin route not found", "path": requestPath}))
}

func handleResource(requestPath string) managementResponse {
	filePath, ok := resourceFileForPath(requestPath)
	if !ok {
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "resource not found"})
	}
	body, err := embeddedWebFS.ReadFile(filePath)
	if err != nil {
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "resource not found"})
	}
	return managementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{contentTypeHTML}}, Body: append([]byte(nil), body...)}
}
func resourceFileForPath(requestPath string) (string, bool) {
	cleaned := path.Clean("/" + strings.TrimSpace(requestPath))
	return "web-dist/index.html", cleaned == resourceAppPath || cleaned == resourceAppPath+"/"
}
func jsonResponse(status int, payload any) managementResponse {
	raw, err := json.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"error":"marshal failed"}`)
	}
	return managementResponse{StatusCode: status, Headers: http.Header{"Content-Type": []string{contentTypeJSON}}, Body: raw}
}
func okEnvelope(result any) ([]byte, error) {
	raw, err := json.Marshal(envelope{OK: true, Result: mustRaw(result)})
	return raw, err
}
func mustRaw(value any) json.RawMessage {
	if raw, ok := value.(json.RawMessage); ok {
		return raw
	}
	raw, _ := json.Marshal(value)
	return raw
}
func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}
func hostHTTPDo(_ context.Context, method, target string, headers http.Header, body []byte) (pricesync.HTTPResponse, error) {
	raw, err := callHost(pluginabi.MethodHostHTTPDo, map[string]any{"method": method, "url": target, "headers": headers, "body": body})
	if err != nil {
		return pricesync.HTTPResponse{}, err
	}
	var response struct {
		StatusCode int         `json:"StatusCode"`
		Headers    http.Header `json:"Headers"`
		Body       []byte      `json:"Body"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return pricesync.HTTPResponse{}, fmt.Errorf("decode host.http.do: %w", err)
	}
	return pricesync.HTTPResponse{StatusCode: response.StatusCode, Headers: response.Headers, Body: response.Body}, nil
}

func listHostAuth() ([]pluginapi.HostAuthFileEntry, error) {
	raw, err := callHost(pluginabi.MethodHostAuthList, map[string]any{})
	if err != nil {
		return nil, err
	}
	var auths []pluginapi.HostAuthFileEntry
	if err := json.Unmarshal(raw, &auths); err == nil {
		return auths, nil
	}
	var wrapped struct {
		Items []pluginapi.HostAuthFileEntry `json:"items"`
		Auths []pluginapi.HostAuthFileEntry `json:"auths"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	if len(wrapped.Items) > 0 {
		return wrapped.Items, nil
	}
	return wrapped.Auths, nil
}

func callHost(method string, payload any) (json.RawMessage, error) {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cMethod := C.CString(method)
	defer C.free(unsafe.Pointer(cMethod))
	var response C.cliproxy_buffer
	var request *C.uint8_t
	if len(rawPayload) > 0 {
		allocated := C.CBytes(rawPayload)
		if allocated == nil {
			return nil, fmt.Errorf("allocate host callback request")
		}
		defer C.free(allocated)
		request = (*C.uint8_t)(allocated)
	}
	code := C.call_host_api(cMethod, request, C.size_t(len(rawPayload)), &response)
	var rawResponse []byte
	if response.ptr != nil && response.len > 0 {
		rawResponse = C.GoBytes(response.ptr, C.int(response.len))
		C.free_host_buffer(response.ptr, response.len)
	}
	if len(rawResponse) == 0 {
		return nil, fmt.Errorf("host callback %s returned no response, code=%d", method, int(code))
	}
	var result envelope
	if err := json.Unmarshal(rawResponse, &result); err != nil {
		return nil, err
	}
	if code != 0 || !result.OK {
		if result.Error != nil {
			return nil, fmt.Errorf("%s: %s", result.Error.Code, result.Error.Message)
		}
		return nil, fmt.Errorf("host callback %s failed", method)
	}
	return append(json.RawMessage(nil), result.Result...), nil
}

func writeResponse(response *C.cliproxy_buffer, data []byte) {
	if response == nil || len(data) == 0 {
		return
	}
	ptr := C.CBytes(data)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(data))
}
