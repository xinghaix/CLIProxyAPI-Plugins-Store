package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

type Writer struct {
	queue       chan store.Event
	store       *store.Store
	batchSize   int
	lastWriteMS atomic.Int64
	dropped     atomic.Int64
	failed      atomic.Int64
}

func NewWriter(database *store.Store, capacity, batchSize int) *Writer {
	return &Writer{queue: make(chan store.Event, capacity), store: database, batchSize: batchSize}
}

func (w *Writer) Enqueue(record pluginapi.UsageRecord) {
	event := ToEvent(record)
	select {
	case w.queue <- event:
	default:
		w.dropped.Add(1)
	}
}

func (w *Writer) Run(ctx context.Context) {
	flushEvery := time.NewTicker(500 * time.Millisecond)
	defer flushEvery.Stop()
	batch := make([]store.Event, 0, w.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if _, err := w.store.InsertEvents(ctx, batch); err != nil {
			w.failed.Add(int64(len(batch)))
		} else {
			w.lastWriteMS.Store(time.Now().UnixMilli())
		}
		batch = batch[:0]
	}
	for {
		select {
		case event := <-w.queue:
			batch = append(batch, event)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-flushEvery.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (w *Writer) Depth() int         { return len(w.queue) }
func (w *Writer) Dropped() int64     { return w.dropped.Load() }
func (w *Writer) Failed() int64      { return w.failed.Load() }
func (w *Writer) LastWriteMS() int64 { return w.lastWriteMS.Load() }

func ToEvent(record pluginapi.UsageRecord) store.Event {
	at := record.RequestedAt
	if at.IsZero() {
		at = time.Now()
	}
	failure := sanitize(record.Failure.Body, 1_024)
	headers, _ := json.Marshal(selectHeaders(record.ResponseHeaders))
	model := strings.TrimSpace(record.Model)
	if model == "" {
		model = "unknown"
	}
	event := store.Event{
		TimestampMS:         at.UnixMilli(),
		Provider:            strings.TrimSpace(record.Provider),
		ExecutorType:        strings.TrimSpace(record.ExecutorType),
		Model:               model,
		APIKeyHash:          digest(record.APIKey),
		AuthID:              strings.TrimSpace(record.AuthID),
		AuthIndex:           strings.TrimSpace(record.AuthIndex),
		AuthType:            strings.TrimSpace(record.AuthType),
		Source:              strings.TrimSpace(record.Source),
		ReasoningEffort:     strings.TrimSpace(record.ReasoningEffort),
		ServiceTier:         strings.TrimSpace(record.ServiceTier),
		InputTokens:         record.Detail.InputTokens,
		OutputTokens:        record.Detail.OutputTokens,
		ReasoningTokens:     record.Detail.ReasoningTokens,
		CachedTokens:        record.Detail.CachedTokens,
		CacheReadTokens:     record.Detail.CacheReadTokens,
		CacheCreationTokens: record.Detail.CacheCreationTokens,
		TotalTokens:         record.Detail.TotalTokens,
		LatencyMS:           record.Latency.Milliseconds(),
		TTFTMS:              record.TTFT.Milliseconds(),
		Failed:              record.Failed,
		FailStatusCode:      record.Failure.StatusCode,
		FailSummary:         failure,
		ResponseHeadersJSON: string(headers),
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.InputTokens + event.OutputTokens + event.ReasoningTokens
	}
	event.Hash = eventHash(event)
	return event
}

func eventHash(event store.Event) string {
	value := strings.Join([]string{
		strconvInt(event.TimestampMS), event.Provider, event.Model, event.APIKeyHash, event.AuthIndex,
		strconvInt(event.TotalTokens), strconvInt(event.LatencyMS), strconvInt(int64(event.FailStatusCode)), event.FailSummary,
	}, "\x00")
	return digest(value)
}

func digest(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sanitize(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > limit {
		return value[:limit] + "…"
	}
	return value
}

func selectHeaders(headers map[string][]string) map[string][]string {
	result := map[string][]string{}
	for _, key := range []string{"X-Request-ID", "X-Trace-ID", "X-Ratelimit-Reset", "Retry-After"} {
		if values := headers[key]; len(values) > 0 {
			result[key] = append([]string(nil), values...)
		}
	}
	return result
}

func strconvInt(value int64) string { return strconv.FormatInt(value, 10) }
