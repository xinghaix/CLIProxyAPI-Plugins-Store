package app

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/datapath"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/ingest"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

type connection struct {
	BaseURL       string `json:"cpaBaseUrl"`
	ManagementKey string `json:"managementKey"`
}

// Runtime owns all local resources that must stop before the plugin is unloaded.
type Runtime struct {
	mu            sync.Mutex
	config        config.Config
	store         *store.Store
	writer        *ingest.Writer
	cancel        context.CancelFunc
	wait          sync.WaitGroup
	closed        atomic.Bool
	started       time.Time
	masterKey     []byte
	connection    connection
	authList      func() ([]pluginapi.HostAuthFileEntry, error)
	httpDo        func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error)
	syncMu        sync.Mutex
	priceMu       sync.Mutex
	priceSettings PriceSyncSettings
	priceStatus   PriceSyncStatus
	priceWake     chan struct{}
}

func New(rawConfig []byte) (*Runtime, error) {
	cfg, err := config.Parse(rawConfig)
	if err != nil {
		return nil, err
	}
	masterKey, err := datapath.Prepare(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	database, err := store.Open(context.Background(), cfg.DataDir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &Runtime{config: cfg, store: database, cancel: cancel, started: time.Now(), masterKey: masterKey, priceWake: make(chan struct{}, 1)}
	if err := runtime.loadConnection(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := runtime.loadPriceSync(context.Background()); err != nil {
		_ = database.Close()
		return nil, err
	}
	runtime.writer = ingest.NewWriter(database, cfg.QueueCapacity, cfg.BatchSize)
	runtime.wait.Add(1)
	go func() { defer runtime.wait.Done(); runtime.writer.Run(ctx) }()
	runtime.wait.Add(1)
	go func() { defer runtime.wait.Done(); runtime.priceSyncLoop(ctx) }()
	if cfg.Codex.Enabled {
		runtime.wait.Add(1)
		go func() { defer runtime.wait.Done(); runtime.scheduleInspections(ctx) }()
	}
	return runtime, nil
}

func (r *Runtime) Reconfigure(rawConfig []byte) error {
	cfg, err := config.Parse(rawConfig)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if cfg.DataDir != r.config.DataDir {
		return fmt.Errorf("data_dir cannot change while the plugin is running; stop CPA, move the complete data directory, then restart")
	}
	if cfg.QueueCapacity != r.config.QueueCapacity || cfg.BatchSize != r.config.BatchSize {
		return fmt.Errorf("queue_capacity and batch_size require a restart")
	}
	r.config = cfg
	return nil
}

func (r *Runtime) HandleUsage(record pluginapi.UsageRecord) {
	if r == nil || r.closed.Load() {
		return
	}
	r.mu.Lock()
	enabled := r.config.Collector.Enabled
	r.mu.Unlock()
	if enabled {
		r.writer.Enqueue(record)
	}
}

func (r *Runtime) Store() *store.Store   { return r.store }
func (r *Runtime) Config() config.Config { r.mu.Lock(); defer r.mu.Unlock(); return r.config }

func (r *Runtime) UpdateCollector(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.config.Collector.Enabled = enabled
}

func (r *Runtime) Connection() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.connection.BaseURL, r.connection.ManagementKey != ""
}

func (r *Runtime) UpdateConnection(ctx context.Context, baseURL, managementKey string) error {
	r.mu.Lock()
	next := r.connection
	if baseURL != "" {
		next.BaseURL = baseURL
	}
	if managementKey != "" {
		next.ManagementKey = managementKey
	}
	r.mu.Unlock()
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	sealed, err := seal(r.masterKey, raw)
	if err != nil {
		return err
	}
	if err := r.store.PutSetting(ctx, "cpa_connection_v1", sealed); err != nil {
		return err
	}
	r.mu.Lock()
	r.connection = next
	r.mu.Unlock()
	return nil
}

func (r *Runtime) loadConnection(ctx context.Context) error {
	raw, ok, err := r.store.Setting(ctx, "cpa_connection_v1")
	if err != nil || !ok {
		return err
	}
	plain, err := open(r.masterKey, raw)
	if err != nil {
		return fmt.Errorf("decrypt saved CPA connection: %w", err)
	}
	return json.Unmarshal(plain, &r.connection)
}

func seal(key, value []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return []byte(base64.RawStdEncoding.EncodeToString(nonce) + "." + base64.RawStdEncoding.EncodeToString(gcm.Seal(nil, nonce, value, nil))), nil
}
func open(key, value []byte) ([]byte, error) {
	parts := strings.Split(string(value), ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid encrypted setting")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (r *Runtime) Health(ctx context.Context) map[string]any {
	count, countErr := r.store.EventCount(ctx)
	last, lastErr := r.store.LastEventAt(ctx)
	cfg := r.Config()
	result := map[string]any{
		"ok":               countErr == nil && lastErr == nil && !r.closed.Load(),
		"runtime":          "local",
		"version":          "0.4.1",
		"data_dir":         cfg.DataDir,
		"started_at_ms":    r.started.UnixMilli(),
		"event_count":      count,
		"last_event_at_ms": last,
		"queue_depth":      r.writer.Depth(),
		"dropped_events":   r.writer.Dropped(),
		"write_failures":   r.writer.Failed(),
		"last_write_at_ms": r.writer.LastWriteMS(),
	}
	if countErr != nil {
		result["error"] = countErr.Error()
	} else if lastErr != nil {
		result["error"] = lastErr.Error()
	}
	return result
}

// SetAuthList supplies the least-privileged host callback used by local inspection.
func (r *Runtime) SetAuthList(list func() ([]pluginapi.HostAuthFileEntry, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.authList = list
}

func (r *Runtime) SetHTTPDo(do func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpDo = do
}

func (r *Runtime) ExecuteCandidate(ctx context.Context, id int64, action string) error {
	if action == "ignore" || action == "resolve" {
		return r.store.ResolveCandidate(ctx, id, action)
	}
	candidate, err := r.store.Candidate(ctx, id)
	if err != nil {
		return err
	}
	r.mu.Lock()
	connection, do := r.connection, r.httpDo
	r.mu.Unlock()
	if do == nil || connection.BaseURL == "" || connection.ManagementKey == "" {
		return fmt.Errorf("CPA connection is not configured")
	}
	base, err := url.Parse(connection.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return fmt.Errorf("invalid CPA Base URL")
	}
	headers := http.Header{"Authorization": []string{"Bearer " + connection.ManagementKey}, "Accept": []string{"application/json"}}
	var method, target string
	var body []byte
	switch action {
	case "enable":
		method = http.MethodPatch
		target = base.ResolveReference(&url.URL{Path: "/v0/management/auth-files/status"}).String()
		headers.Set("Content-Type", "application/json")
		body, _ = json.Marshal(map[string]any{"name": candidate.AuthFileName, "auth_index": candidate.AuthIndex, "disabled": false})
	case "delete":
		method = http.MethodDelete
		endpoint := base.ResolveReference(&url.URL{Path: "/v0/management/auth-files"})
		q := endpoint.Query()
		q.Set("name", candidate.AuthFileName)
		endpoint.RawQuery = q.Encode()
		target = endpoint.String()
	default:
		return fmt.Errorf("unsupported candidate action")
	}
	response, err := do(ctx, method, target, headers, body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("CPA action returned HTTP %d", response.StatusCode)
	}
	return r.store.ResolveCandidate(ctx, id, action)
}

func (r *Runtime) RunInspection(ctx context.Context) (map[string]any, error) {
	r.mu.Lock()
	list := r.authList
	r.mu.Unlock()
	if list == nil {
		return nil, fmt.Errorf("host auth callback is unavailable")
	}
	auths, err := list()
	if err != nil {
		return nil, err
	}
	accounts := make([]store.InspectionAccount, 0, len(auths))
	for _, auth := range auths {
		if auth.Provider != "codex" {
			continue
		}
		key := auth.AuthIndex
		if key == "" {
			key = auth.ID
		}
		if key == "" {
			key = auth.Name
		}
		accounts = append(accounts, store.InspectionAccount{Key: key, FileName: auth.Name, DisplayName: firstNonEmpty(auth.Email, auth.Label, auth.Name), Provider: auth.Provider, Status: auth.Status, Disabled: auth.Disabled})
	}
	run, err := r.store.StartInspectionWithAccounts(ctx, "manual", accounts)
	if err != nil {
		return nil, err
	}
	return r.store.InspectionDetail(ctx, run.ID)
}

func (r *Runtime) scheduleInspections(ctx context.Context) {
	for {
		cfg := r.Config()
		interval := time.Duration(cfg.Codex.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = time.Hour
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			if cfg.Codex.Enabled {
				_, _ = r.RunInspection(ctx)
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func (r *Runtime) Close() error {
	if r == nil || !r.closed.CompareAndSwap(false, true) {
		return nil
	}
	r.cancel()
	done := make(chan struct{})
	go func() { r.wait.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("local runtime did not stop within 10 seconds")
	}
	return r.store.Close()
}
