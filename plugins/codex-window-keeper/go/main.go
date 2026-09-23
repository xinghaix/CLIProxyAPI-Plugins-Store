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
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/api"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/keeper"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/policy"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/store"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/upstream"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/usage"
	"gopkg.in/yaml.v3"
)

//go:embed web/index.html
var indexHTML []byte

var pluginVersion = "0.1.0"

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

type managementRequest struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   []byte `json:"body"`
}

type managementResponse struct {
	StatusCode int         `json:"status_code"`
	Headers    http.Header `json:"headers"`
	Body       []byte      `json:"body"`
}

var runtime struct {
	mu      sync.Mutex
	keeper  *keeper.Keeper
	cancel  context.CancelFunc
	done    chan struct{}
	dataDir string
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	if host != nil {
		C.store_host_api(host)
	}
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
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() { stopRuntime() }

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		if err := configure(request); err != nil {
			return nil, err
		}
		return okEnvelope(pluginRegistration())
	case pluginabi.MethodManagementRegister:
		return okEnvelope(managementRegistration())
	case pluginabi.MethodManagementHandle:
		return handleManagement(request)
	case pluginabi.MethodPluginQuiesce:
		stopRuntime()
		return okEnvelope(map[string]any{})
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func configure(raw []byte) error {
	var request lifecycleRequest
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &request); err != nil {
			return err
		}
	}
	var file config.File
	if len(request.ConfigYAML) > 0 {
		if err := yaml.Unmarshal(request.ConfigYAML, &file); err != nil {
			return err
		}
	}
	dataDir := strings.TrimSpace(file.DataDir)
	if dataDir == "" {
		dataDir = "data/codex-window-keeper"
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.keeper != nil {
		if runtime.dataDir != dataDir {
			return fmt.Errorf("data_dir is already open at %s", runtime.dataDir)
		}
		runtime.keeper.WakeUp()
		return nil
	}
	db, err := store.Open(context.Background(), dataDir)
	if err != nil {
		return err
	}
	if _, ok, err := db.LoadSettings(context.Background()); err != nil {
		_ = db.Close()
		return err
	} else if !ok {
		if err := db.SaveSettings(context.Background(), config.FromFile(file)); err != nil {
			_ = db.Close()
			return err
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	k := &keeper.Keeper{Store: db, Catalog: hostCatalog{}, Prober: hostProber{}, Sender: hostSender{}, Owner: "codex-window-keeper", Wake: make(chan struct{}, 1)}
	done := make(chan struct{})
	runtime.keeper = k
	runtime.cancel = cancel
	runtime.done = done
	runtime.dataDir = dataDir
	go func() {
		defer close(done)
		k.Run(ctx)
	}()
	return nil
}

func stopRuntime() {
	runtime.mu.Lock()
	k, cancel, done := runtime.keeper, runtime.cancel, runtime.done
	runtime.mu.Unlock()
	if k == nil {
		return
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	if k.Store != nil {
		_ = k.Store.Close()
	}
	runtime.mu.Lock()
	if runtime.keeper == k {
		runtime.keeper = nil
		runtime.cancel = nil
		runtime.done = nil
	}
	runtime.mu.Unlock()
}

func currentKeeper() *keeper.Keeper {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.keeper
}

func pluginRegistration() map[string]any {
	return map[string]any{
		"schema_version": uint32(1),
		"metadata": pluginapi.Metadata{
			Name: "Codex Window Keeper", Version: pluginVersion, Author: "xinghaix",
			GitHubRepository: "https://github.com/xinghaix/CLIProxyAPI-Plugins-Store",
			Logo:             "https://cdn.jsdelivr.net/gh/xinghaix/CLIProxyAPI-Plugins-Store@main/plugins/codex-window-keeper/assets/logo.svg",
			ConfigFields: []pluginapi.ConfigField{
				{Name: "data_dir", Type: pluginapi.ConfigFieldTypeString, Description: "唯一 SQLite 所在目录，默认 data/codex-window-keeper"},
				{Name: "enabled", Type: pluginapi.ConfigFieldTypeBoolean, Description: "首次创建数据库时是否启用。之后以数据库里的设置为准"},
				{Name: "model", Type: pluginapi.ConfigFieldTypeString, Description: "额度窗口恢复后发送消息所用的模型"},
				{Name: "reasoning_effort", Type: pluginapi.ConfigFieldTypeString, Description: "发送消息的思考级别"},
				{Name: "prompt", Type: pluginapi.ConfigFieldTypeString, Description: "用于确认窗口可用的短消息"},
				{Name: "window_kinds", Type: pluginapi.ConfigFieldTypeArray, Description: "纳入门控的窗口种类；省略时动态采用账号返回的窗口"},
			},
		},
		"capabilities": map[string]any{"management_api": true},
	}
}

func managementRegistration() pluginapi.ManagementRegistrationResponse {
	specs := []struct{ method, path string }{
		{http.MethodGet, "/codex-window-keeper/health"},
		{http.MethodGet, "/codex-window-keeper/settings"},
		{http.MethodPut, "/codex-window-keeper/settings"},
		{http.MethodPut, "/codex-window-keeper/connection"},
		{http.MethodGet, "/codex-window-keeper/accounts"},
		{http.MethodPut, "/codex-window-keeper/accounts/override"},
		{http.MethodPost, "/codex-window-keeper/accounts/probe"},
		{http.MethodPost, "/codex-window-keeper/accounts/activate"},
		{http.MethodPost, "/codex-window-keeper/accounts/resume"},
		{http.MethodGet, "/codex-window-keeper/attempts"},
	}
	routes := make([]pluginapi.ManagementRoute, 0, len(specs))
	for _, spec := range specs {
		routes = append(routes, pluginapi.ManagementRoute{Method: spec.method, Path: spec.path})
	}
	return pluginapi.ManagementRegistrationResponse{Routes: routes, Resources: []pluginapi.ResourceRoute{{Path: "/app", Menu: "额度窗口", Description: "Codex OAuth 额度窗口恢复后自动发送一条消息"}}}
}

func handleManagement(raw []byte) ([]byte, error) {
	var request managementRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	path := strings.TrimRight(strings.TrimSpace(request.Path), "/")
	if path == "/app" || path == "/v0/resource/plugins/codex-window-keeper/app" {
		return okEnvelope(managementResponse{StatusCode: http.StatusOK, Headers: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: append([]byte(nil), indexHTML...)})
	}
	k := currentKeeper()
	if k == nil {
		return okEnvelope(jsonResponse(http.StatusServiceUnavailable, []byte(`{"error":"runtime is not initialized"}`)))
	}
	status, body := api.Service{Keeper: k}.Handle(context.Background(), strings.ToUpper(request.Method), path, request.Body)
	return okEnvelope(jsonResponse(status, body))
}

func jsonResponse(status int, body []byte) managementResponse {
	return managementResponse{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: body}
}

type hostCatalog struct{}

func (hostCatalog) List(ctx context.Context) ([]keeper.AccountRef, error) {
	raw, err := callHost(pluginabi.MethodHostAuthList, map[string]any{})
	if err != nil {
		return nil, err
	}
	entries, err := decodeAuths(raw)
	if err != nil {
		return nil, err
	}
	var out []keeper.AccountRef
	for _, entry := range entries {
		if !codexOAuth(entry) || strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.AuthIndex) == "" {
			continue
		}
		accountID, plan := strings.TrimSpace(entry.Account), ""
		if rawAuth, err := callHost(pluginabi.MethodHostAuthGet, pluginapi.HostAuthGetRequest{AuthIndex: entry.AuthIndex}); err == nil {
			var auth pluginapi.HostAuthGetResponse
			if json.Unmarshal(rawAuth, &auth) == nil {
				parsedPlan, parsedID := upstream.AuthMetadata(auth.JSON)
				if parsedPlan != "" {
					plan = parsedPlan
				}
				if parsedID != "" {
					accountID = parsedID
				}
			}
		}
		out = append(out, keeper.AccountRef{AuthID: entry.ID, AuthIndex: entry.AuthIndex, AccountID: accountID, Email: entry.Email, Name: entry.Name, Plan: plan, Disabled: entry.Disabled, Unavailable: entry.Unavailable, NextRetryAfter: entry.NextRetryAfter})
	}
	return out, nil
}

type hostProber struct{}

func (hostProber) Probe(ctx context.Context, ref keeper.AccountRef) (usage.Snapshot, error) {
	k := currentKeeper()
	if k == nil {
		return usage.Snapshot{}, fmt.Errorf("runtime is not initialized")
	}
	settings, ok, err := k.Store.LoadSettings(ctx)
	if err != nil {
		return usage.Snapshot{}, err
	}
	if !ok {
		settings = config.Default()
	}
	key, _, err := k.Store.LoadManagementKey(ctx)
	if err != nil {
		return usage.Snapshot{}, err
	}
	return upstream.ProbeUsage(ctx, hostDo, settings.BaseURL, key, ref.AuthIndex, ref.AccountID, time.Now().UTC())
}

type hostSender struct{}

func (hostSender) Send(ctx context.Context, ref keeper.AccountRef, settings config.Settings) (keeper.SendResult, error) {
	body, _ := json.Marshal(map[string]any{
		"model": settings.Model, "store": false,
		"input":     []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": settings.Prompt}}}},
		"reasoning": map[string]any{"effort": settings.Effort},
	})
	raw, err := callHost(pluginabi.MethodHostModelExecuteStream, pluginapi.HostModelExecutionRequest{
		EntryProtocol: "openai-response", ExitProtocol: "codex", Model: settings.Model, Stream: true,
		Body: body, ForcedProvider: "codex", AuthID: ref.AuthID,
	})
	if err != nil {
		return keeper.SendResult{Kind: policy.KindRetry}, err
	}
	var opened pluginapi.HostModelStreamResponse
	if err := json.Unmarshal(raw, &opened); err != nil {
		return keeper.SendResult{Kind: policy.KindRetry}, err
	}
	if opened.StreamID != "" {
		defer callHost(pluginabi.MethodHostModelStreamClose, pluginapi.HostModelStreamCloseRequest{StreamID: opened.StreamID})
	}
	if opened.StatusCode >= 400 {
		return keeper.SendResult{Status: opened.StatusCode, Kind: kindForStatus(opened.StatusCode)}, nil
	}
	var chunks []byte
	for {
		if err := ctx.Err(); err != nil {
			return keeper.SendResult{Kind: policy.KindRetry}, err
		}
		piece, err := callHost(pluginabi.MethodHostModelStreamRead, pluginapi.HostModelStreamReadRequest{StreamID: opened.StreamID})
		if err != nil {
			return keeper.SendResult{Kind: policy.KindRetry}, err
		}
		var read pluginapi.HostModelStreamReadResponse
		if err := json.Unmarshal(piece, &read); err != nil {
			return keeper.SendResult{Kind: policy.KindRetry}, err
		}
		chunks = append(chunks, read.Payload...)
		if read.Error != "" {
			return keeper.SendResult{Status: opened.StatusCode, Kind: policy.KindRetry, Excerpt: read.Error}, nil
		}
		if read.Done {
			break
		}
	}
	ok, text := upstream.Completion(chunks)
	result := keeper.SendResult{OK: ok, Status: opened.StatusCode, Excerpt: text}
	if !ok {
		result.Kind = policy.KindRetry
	}
	return result, nil
}

func kindForStatus(status int) string {
	switch status {
	case 401, 403:
		return policy.KindAuth
	case 429:
		return policy.KindQuota
	case 400:
		return policy.KindConfig
	default:
		return policy.KindRetry
	}
}

func hostDo(method, target string, header map[string]string, body []byte) (int, []byte, error) {
	headers := make(http.Header, len(header))
	for key, value := range header {
		headers.Set(key, value)
	}
	raw, err := callHost(pluginabi.MethodHostHTTPDo, pluginapi.HTTPRequest{Method: method, URL: target, Headers: headers, Body: body})
	if err != nil {
		return 0, nil, err
	}
	var response pluginapi.HTTPResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return 0, nil, err
	}
	return response.StatusCode, response.Body, nil
}

func codexOAuth(entry pluginapi.HostAuthFileEntry) bool {
	if !strings.Contains(strings.ToLower(entry.Provider+" "+entry.Type), "codex") {
		return false
	}
	switch strings.ToLower(entry.AccountType) {
	case "", "oauth", "oauth2":
		return true
	default:
		return false
	}
}

func decodeAuths(raw json.RawMessage) ([]pluginapi.HostAuthFileEntry, error) {
	var entries []pluginapi.HostAuthFileEntry
	if err := json.Unmarshal(raw, &entries); err == nil {
		return entries, nil
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, err
	}
	for _, key := range []string{"auths", "items", "files"} {
		if value, ok := wrapped[key]; ok && json.Unmarshal(value, &entries) == nil {
			return entries, nil
		}
	}
	return nil, fmt.Errorf("decode host.auth.list")
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
	response.ptr = C.CBytes(data)
	response.len = C.size_t(len(data))
}

func okEnvelope(result any) ([]byte, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}
