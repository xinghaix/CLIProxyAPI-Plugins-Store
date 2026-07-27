package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const (
	priceSyncSettingsKey = "model_price_sync_settings_v1"
	priceSyncStatusKey   = "model_price_sync_status_v1"
)

type PriceSyncSettings struct {
	Enabled       bool `json:"enabled"`
	IntervalHours int  `json:"intervalHours"`
	ProtectManual bool `json:"protectManual"`
}

type PriceSyncStatus struct {
	Running         bool              `json:"running"`
	LastSyncAtMS    int64             `json:"lastSyncAtMs"`
	LastSuccessAtMS int64             `json:"lastSuccessAtMs"`
	LastError       string            `json:"lastError,omitempty"`
	LastResult      *pricesync.Result `json:"lastResult,omitempty"`
}

func defaultPriceSyncSettings() PriceSyncSettings {
	return PriceSyncSettings{IntervalHours: 12, ProtectManual: true}
}

func (r *Runtime) loadPriceSync(ctx context.Context) error {
	r.priceSettings = defaultPriceSyncSettings()
	if raw, ok, err := r.store.Setting(ctx, priceSyncSettingsKey); err != nil {
		return err
	} else if ok {
		if err := json.Unmarshal(raw, &r.priceSettings); err != nil {
			return fmt.Errorf("decode price sync settings: %w", err)
		}
	}
	if r.priceSettings.IntervalHours < 6 || r.priceSettings.IntervalHours > 168 {
		r.priceSettings.IntervalHours = 12
	}
	if raw, ok, err := r.store.Setting(ctx, priceSyncStatusKey); err != nil {
		return err
	} else if ok {
		_ = json.Unmarshal(raw, &r.priceStatus)
	}
	return nil
}

func (r *Runtime) PriceSyncSettings() PriceSyncSettings {
	r.priceMu.Lock()
	defer r.priceMu.Unlock()
	return r.priceSettings
}
func (r *Runtime) PriceSyncStatus() PriceSyncStatus {
	r.priceMu.Lock()
	defer r.priceMu.Unlock()
	return r.priceStatus
}

func (r *Runtime) UpdatePriceSyncSettings(ctx context.Context, settings PriceSyncSettings) error {
	if settings.IntervalHours < 6 || settings.IntervalHours > 168 {
		return fmt.Errorf("intervalHours must be between 6 and 168")
	}
	settings.ProtectManual = true
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if err := r.store.PutSetting(ctx, priceSyncSettingsKey, raw); err != nil {
		return err
	}
	r.priceMu.Lock()
	r.priceSettings = settings
	r.priceMu.Unlock()
	r.wakePriceSync()
	return nil
}

func (r *Runtime) SyncPrices(ctx context.Context) (pricesync.Result, error) {
	if !r.syncMu.TryLock() {
		return pricesync.Result{}, fmt.Errorf("price sync is already running")
	}
	defer r.syncMu.Unlock()
	r.priceMu.Lock()
	r.priceStatus.Running = true
	r.priceStatus.LastSyncAtMS = time.Now().UnixMilli()
	r.priceMu.Unlock()
	defer func() { r.priceMu.Lock(); r.priceStatus.Running = false; r.priceMu.Unlock() }()
	targets, err := r.store.PriceSyncTargets(ctx)
	if err != nil {
		return r.finishPriceSync(ctx, pricesync.Result{}, err)
	}
	r.mu.Lock()
	do := r.httpDo
	r.mu.Unlock()
	if do == nil {
		return r.finishPriceSync(ctx, pricesync.Result{}, fmt.Errorf("host HTTP callback is unavailable"))
	}
	result, err := pricesync.Run(ctx, targets, func(ctx context.Context, target string, headers http.Header) (pricesync.HTTPResponse, error) {
		return do(ctx, http.MethodGet, target, headers, nil)
	})
	if err == nil {
		upsert, upsertErr := r.store.UpsertSyncedPrices(ctx, result.Matched)
		if upsertErr != nil {
			err = upsertErr
		} else {
			result.Imported = upsert.Imported
			result.Skipped += upsert.Skipped
			result.ProtectedManual = upsert.ProtectedManual
			result.Prices, _ = r.store.Prices(ctx)
		}
	}
	return r.finishPriceSync(ctx, result, err)
}

func (r *Runtime) ConfirmPriceSyncCandidate(ctx context.Context, model string, price store.Price) error {
	if model == "" || price.Source == "" || price.SourceModelID == "" {
		return fmt.Errorf("model, source, and sourceModelId are required")
	}
	_, err := r.store.UpsertSyncedPrices(ctx, map[string]store.Price{model: price})
	return err
}

func (r *Runtime) finishPriceSync(ctx context.Context, result pricesync.Result, runErr error) (pricesync.Result, error) {
	r.priceMu.Lock()
	status := r.priceStatus
	status.Running = false
	status.LastSyncAtMS = time.Now().UnixMilli()
	status.LastResult = &result
	if runErr != nil {
		status.LastError = runErr.Error()
	} else {
		status.LastError = ""
		status.LastSuccessAtMS = status.LastSyncAtMS
	}
	r.priceStatus = status
	r.priceMu.Unlock()
	raw, _ := json.Marshal(status)
	if err := r.store.PutSetting(ctx, priceSyncStatusKey, raw); err != nil && runErr == nil {
		runErr = err
	}
	return result, runErr
}

func (r *Runtime) priceSyncLoop(ctx context.Context) {
	for {
		settings := r.PriceSyncSettings()
		status := r.PriceSyncStatus()
		delay := 24 * time.Hour
		if settings.Enabled {
			next := time.UnixMilli(status.LastSyncAtMS).Add(time.Duration(settings.IntervalHours) * time.Hour)
			if status.LastSyncAtMS == 0 || next.Before(time.Now()) {
				delay = 0
			} else {
				delay = time.Until(next)
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-r.priceWake:
			timer.Stop()
			continue
		case <-timer.C:
			if settings.Enabled {
				_, _ = r.SyncPrices(ctx)
			}
		}
	}
}
func (r *Runtime) wakePriceSync() {
	select {
	case r.priceWake <- struct{}{}:
	default:
	}
}
