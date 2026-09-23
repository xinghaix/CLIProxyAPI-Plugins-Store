package app

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/responsemodel"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

// Only evidence is queued: never response bodies, request payloads, or credentials.
// Hooks must not wait on SQLite or modify the response delivered to the client.
type responseObserver struct {
	mu        sync.Mutex
	tracker   *responsemodel.Tracker
	queue     chan responsemodel.Observation
	disabled  atomic.Bool
	seen      atomic.Int64
	written   atomic.Int64
	dropped   atomic.Int64
	lastError atomic.Value
}

func (r *Runtime) startResponseObserver(ctx context.Context) {
	r.responseObserver = &responseObserver{tracker: responsemodel.New(), queue: make(chan responsemodel.Observation, 1024)}
	r.wait.Add(1)
	go func() { defer r.wait.Done(); r.runResponseObserver(ctx) }()
}

// DisableResponseObserver handles an unscoped telemetry gap without blocking the
// model request. Host-reported metadata remains available; only derived evidence
// is suppressed immediately and quarantined by the background worker.
func (r *Runtime) DisableResponseObserver(reason string) {
	if r == nil || r.responseObserver == nil || r.closed.Load() || !r.Config().Collector.Enabled {
		return
	}
	o := r.responseObserver
	o.dropped.Add(1)
	o.lastError.Store(reason)
	o.disabled.Store(true)
	r.store.SuppressResponseObservations()
}

func (r *Runtime) ObserveRawResponse(req pluginapi.ResponseTransformRequest) {
	r.observeResponse(func(t *responsemodel.Tracker) []responsemodel.Observation { return t.ObserveRaw(req) })
}

func (r *Runtime) ObserveResponse(req pluginapi.ResponseInterceptRequest) {
	r.observeResponse(func(t *responsemodel.Tracker) []responsemodel.Observation { return t.ObserveResponse(req) })
}

func (r *Runtime) ObserveStreamResponse(req pluginapi.StreamChunkInterceptRequest) {
	r.observeResponse(func(t *responsemodel.Tracker) []responsemodel.Observation { return t.ObserveStream(req) })
}

func (r *Runtime) observeResponse(observe func(*responsemodel.Tracker) []responsemodel.Observation) {
	if r == nil || r.responseObserver == nil || r.closed.Load() {
		return
	}
	o := r.responseObserver
	o.mu.Lock()
	defer o.mu.Unlock()
	if r.closed.Load() || o.disabled.Load() || !r.Config().Collector.Enabled {
		return
	}
	o.seen.Add(1)
	for _, evidence := range observe(o.tracker) {
		select {
		case o.queue <- evidence:
		default:
			// Losing a late conflict could leave an earlier attribution looking valid.
			// Stop observing and quarantine persisted observer evidence instead.
			o.dropped.Add(1)
			o.lastError.Store("response observation queue full; observer disabled until restart")
			o.disabled.Store(true)
			r.store.SuppressResponseObservations()
			return
		}
	}
}

func (r *Runtime) runResponseObserver(ctx context.Context) {
	o := r.responseObserver
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	quarantined := false
	quarantine := func(writeCtx context.Context) {
		if !o.disabled.Load() || quarantined {
			return
		}
		if err := r.store.QuarantineResponseObservations(writeCtx); err != nil {
			o.lastError.Store(err.Error())
			return
		}
		quarantined = true
	}
	persist := func(writeCtx context.Context, evidence responsemodel.Observation) {
		if o.disabled.Load() {
			quarantine(writeCtx)
			return
		}
		if err := r.store.RecordResponseMetadata(writeCtx, store.ResponseMetadata{
			Key: evidence.Key, Model: evidence.Model, ServiceTier: evidence.ServiceTier, EvidenceID: evidence.EvidenceID,
			TierAmbiguous: evidence.ServiceTierAmbiguous, Ambiguous: evidence.Ambiguous,
		}); err != nil {
			o.lastError.Store(err.Error())
			o.disabled.Store(true)
			r.store.SuppressResponseObservations()
			quarantine(writeCtx)
			return
		}
		o.written.Add(1)
	}
	for {
		select {
		case evidence := <-o.queue:
			writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			persist(writeCtx, evidence)
			cancel()
		case <-ticker.C:
			writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			quarantine(writeCtx)
			cancel()
		case <-ctx.Done():
			// Close has fenced producers before cancellation. Bound total drain time.
			writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for {
				select {
				case evidence := <-o.queue:
					persist(writeCtx, evidence)
				default:
					quarantine(writeCtx)
					return
				}
			}
		}
	}
}

func (r *Runtime) responseObserverHealth() map[string]any {
	o := r.responseObserver
	if o == nil {
		return map[string]any{"enabled": false}
	}
	message, _ := o.lastError.Load().(string)
	return map[string]any{
		"enabled":                r.Config().Collector.Enabled && !r.closed.Load() && !o.disabled.Load(),
		"disabled_until_restart": o.disabled.Load(),
		"callbacks":              o.seen.Load(), "persisted": o.written.Load(),
		"queue_depth": len(o.queue), "dropped": o.dropped.Load(), "last_error": message,
	}
}
