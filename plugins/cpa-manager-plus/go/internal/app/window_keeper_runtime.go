package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func (r *Runtime) SetModelExecuteStream(fn func(pluginapi.HostModelExecutionRequest) (pluginapi.HostModelStreamResponse, error)) {
	r.mu.Lock()
	r.modelExecuteStream = fn
	r.mu.Unlock()
}

func (r *Runtime) SetModelStreamRead(fn func(pluginapi.HostModelStreamReadRequest) (pluginapi.HostModelStreamReadResponse, error)) {
	r.mu.Lock()
	r.modelStreamRead = fn
	r.mu.Unlock()
}

func (r *Runtime) SetModelStreamClose(fn func(pluginapi.HostModelStreamCloseRequest) error) {
	r.mu.Lock()
	r.modelStreamClose = fn
	r.mu.Unlock()
}

func (r *Runtime) loadWindowKeeperSettings(ctx context.Context) error {
	settings, ok, err := r.store.LoadWindowKeeperSettings(ctx)
	if err != nil {
		return err
	}
	if !ok {
		settings = windowkeeper.DefaultSettings()
	}
	r.windowKeeperMu.Lock()
	r.windowKeeperSettings = settings
	r.windowKeeperMu.Unlock()
	return nil
}

func (r *Runtime) initWindowKeeper() {
	r.windowKeeper = &windowkeeper.Keeper{
		Store:   r.store.WindowKeeper(),
		Catalog: runtimeCatalog{r: r},
		Prober:  runtimeProber{r: r},
		Sender:  runtimeSender{r: r},
		Owner:   "cpa-manager-plus",
		Wake:    r.windowKeeperWake,
	}
}

func (r *Runtime) scheduleWindowKeeper(ctx context.Context) {
	if r.windowKeeper != nil {
		r.windowKeeper.Run(ctx)
	}
}

func (r *Runtime) WindowKeeperSettings() windowkeeper.Settings {
	r.windowKeeperMu.Lock()
	defer r.windowKeeperMu.Unlock()
	return r.windowKeeperSettings
}

func (r *Runtime) SaveWindowKeeperSettings(ctx context.Context, settings windowkeeper.Settings) error {
	normalized, err := windowkeeper.NormalizeSettings(settings)
	if err != nil {
		return err
	}
	if err := r.store.SaveWindowKeeperSettings(ctx, normalized); err != nil {
		return err
	}
	r.windowKeeperMu.Lock()
	r.windowKeeperSettings = normalized
	r.windowKeeperMu.Unlock()
	if r.windowKeeper != nil {
		r.windowKeeper.WakeUp()
	}
	return nil
}

func (r *Runtime) WindowKeeperAccounts(ctx context.Context) ([]windowkeeper.Account, error) {
	accounts, err := r.store.ListWindowKeeperAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		windows, err := r.store.LoadWindowKeeperWindows(ctx, accounts[i].AuthID)
		if err == nil {
			accounts[i].Windows = windows
		}
	}
	return accounts, nil
}

func (r *Runtime) SetWindowKeeperOverride(ctx context.Context, authID, raw string) error {
	if err := r.store.SetWindowKeeperOverride(ctx, authID, raw); err != nil {
		return err
	}
	if r.windowKeeper != nil {
		r.windowKeeper.WakeUp()
	}
	return nil
}

func (r *Runtime) ProbeWindowKeeperAccount(ctx context.Context, authID string) error {
	if r.windowKeeper == nil {
		return fmt.Errorf("window keeper is not initialized")
	}
	return r.windowKeeper.Probe(ctx, authID)
}

func (r *Runtime) ActivateWindowKeeperAccount(ctx context.Context, authID string) error {
	if r.windowKeeper == nil {
		return fmt.Errorf("window keeper is not initialized")
	}
	return r.windowKeeper.Activate(ctx, authID)
}

func (r *Runtime) ResumeWindowKeeperAccount(ctx context.Context, authID string) error {
	if err := r.store.SetWindowKeeperPause(ctx, authID, ""); err != nil {
		return err
	}
	if err := r.store.SetWindowKeeperNotBefore(ctx, authID, time.Now().UTC()); err != nil {
		return err
	}
	if r.windowKeeper != nil {
		r.windowKeeper.WakeUp()
	}
	return nil
}

func (r *Runtime) WindowKeeperAttempts(ctx context.Context, limit int) ([]windowkeeper.Attempt, error) {
	return r.store.ListWindowKeeperAttempts(ctx, limit)
}

// Catalog adapter
type runtimeCatalog struct{ r *Runtime }

func (c runtimeCatalog) List(ctx context.Context) ([]windowkeeper.AccountRef, error) {
	if c.r.authList == nil {
		return nil, fmt.Errorf("authList callback not available")
	}
	entries, err := c.r.authList()
	if err != nil {
		return nil, err
	}
	var out []windowkeeper.AccountRef
	for _, entry := range entries {
		if !codexOAuthEntry(entry) || strings.TrimSpace(entry.ID) == "" || strings.TrimSpace(entry.AuthIndex) == "" {
			continue
		}
		accountID := strings.TrimSpace(entry.Account)
		plan := ""
		if c.r.authGet != nil {
			if rawAuth, err := c.r.authGet(entry.AuthIndex); err == nil {
				parsedPlan, parsedID := windowkeeper.AuthMetadata(rawAuth.JSON)
				if parsedPlan != "" {
					plan = parsedPlan
				}
				if parsedID != "" {
					accountID = parsedID
				}
			}
		}
		out = append(out, windowkeeper.AccountRef{
			AuthID: entry.ID, AuthIndex: entry.AuthIndex, AccountID: accountID,
			Email: entry.Email, Name: entry.Name, Plan: plan,
			Disabled: entry.Disabled, Unavailable: entry.Unavailable,
			NextRetryAfter: entry.NextRetryAfter,
		})
	}
	return out, nil
}

func codexOAuthEntry(entry pluginapi.HostAuthFileEntry) bool {
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

// Prober adapter
type runtimeProber struct{ r *Runtime }

func (p runtimeProber) Probe(ctx context.Context, ref windowkeeper.AccountRef) (windowkeeper.Snapshot, error) {
	p.r.mu.Lock()
	connection := p.r.connection
	do := p.r.httpDo
	p.r.mu.Unlock()

	settings := p.r.WindowKeeperSettings()
	baseURL := connection.BaseURL
	if baseURL == "" {
		baseURL = settings.BaseURL
	}
	key := connection.ManagementKey

	httpDoer := func(method, target string, header map[string]string, body []byte) (int, []byte, error) {
		headers := make(http.Header, len(header))
		for k, v := range header {
			headers.Set(k, v)
		}
		if do == nil {
			return 0, nil, fmt.Errorf("host httpDo not available")
		}
		resp, err := do(ctx, method, target, headers, body)
		if err != nil {
			return 0, nil, err
		}
		return resp.StatusCode, resp.Body, nil
	}

	return windowkeeper.ProbeUsage(ctx, httpDoer, baseURL, key, ref.AuthIndex, ref.AccountID, settings.UserAgent, time.Now().UTC())
}

// Sender adapter
type runtimeSender struct{ r *Runtime }

func (s runtimeSender) Send(ctx context.Context, ref windowkeeper.AccountRef, settings windowkeeper.Settings) (windowkeeper.SendResult, error) {
	s.r.mu.Lock()
	execStream := s.r.modelExecuteStream
	readStream := s.r.modelStreamRead
	closeStream := s.r.modelStreamClose
	s.r.mu.Unlock()

	if execStream == nil || readStream == nil || closeStream == nil {
		return windowkeeper.SendResult{Kind: windowkeeper.ErrKindRetry}, fmt.Errorf("host model streaming callbacks not available")
	}

	requestBody := map[string]any{
		"model": settings.Model, "store": false,
		"input":     []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": settings.Prompt}}}},
		"reasoning": map[string]any{"effort": settings.Effort},
	}
	if strings.TrimSpace(settings.ServiceTier) != "" {
		requestBody["service_tier"] = strings.TrimSpace(settings.ServiceTier)
	}
	body, _ := json.Marshal(requestBody)

	opened, err := execStream(pluginapi.HostModelExecutionRequest{
		EntryProtocol: "openai-response", ExitProtocol: "codex", Model: settings.Model, Stream: true,
		Body: body, ForcedProvider: "codex", AuthID: ref.AuthID,
	})
	if err != nil {
		return windowkeeper.SendResult{Kind: windowkeeper.ErrKindRetry}, err
	}
	if opened.StreamID != "" {
		defer closeStream(pluginapi.HostModelStreamCloseRequest{StreamID: opened.StreamID})
	}
	if opened.StatusCode >= 400 {
		return windowkeeper.SendResult{
			Status: opened.StatusCode,
			Kind:   windowkeeper.Classify(opened.StatusCode, ""),
		}, nil
	}
	var chunks []byte
	for {
		if err := ctx.Err(); err != nil {
			return windowkeeper.SendResult{Kind: windowkeeper.ErrKindRetry}, err
		}
		read, err := readStream(pluginapi.HostModelStreamReadRequest{StreamID: opened.StreamID})
		if err != nil {
			return windowkeeper.SendResult{Kind: windowkeeper.ErrKindRetry}, err
		}
		chunks = append(chunks, read.Payload...)
		if read.Error != "" {
			return windowkeeper.SendResult{
				Status: opened.StatusCode, Kind: windowkeeper.ErrKindRetry, Excerpt: read.Error,
			}, nil
		}
		if read.Done {
			break
		}
	}
	ok, text := windowkeeper.Completion(chunks)
	result := windowkeeper.SendResult{OK: ok, Status: opened.StatusCode, Excerpt: text}
	if !ok {
		result.Kind = windowkeeper.ErrKindRetry
	}
	return result, nil
}
