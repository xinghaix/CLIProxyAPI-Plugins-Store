package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
)

// CredentialUsageMetrics matches the metrics shape needed by the frontend.
type CredentialUsageMetrics struct {
	Requests      int64    `json:"requests"`
	Tokens        int64    `json:"tokens"`
	Cost          float64  `json:"cost"`
	SuccessCalls  int64    `json:"successCalls"`
	FailureCalls  int64    `json:"failureCalls"`
	SuccessRate   *float64 `json:"successRate"`
	CostComplete  bool     `json:"costComplete"`
	UnpricedCalls int64    `json:"unpricedCalls"`
	LastSeenMS    *int64   `json:"lastSeenMs"`
}

// CredentialIdentity is used to match credentials against usage_events.
type CredentialIdentity struct {
	Key       string
	AuthIndex string
	AuthID    string
	FileName  string
	Source    string
	Provider  string
}

// CredentialEnrichment holds aggregated usage and inspection cache.
type CredentialEnrichment struct {
	History        *CredentialUsageMetrics `json:"history,omitempty"`
	RecentStatuses []string                `json:"recentStatuses,omitempty"`
	PlanLabel      string                  `json:"planLabel,omitempty"`
	QuotaWindows   any                     `json:"quotaWindows,omitempty"`
}

// EnrichCredentials computes real historical usage and recent status slots for
// the given list of credentials in a single pass over SQLite usage_events.
func (s *Store) EnrichCredentials(ctx context.Context, creds []CredentialIdentity, lookbackMS int64) (map[string]*CredentialEnrichment, error) {
	if lookbackMS <= 0 {
		lookbackMS = 90 * 24 * 3600 * 1000
	}
	fromMS := time.Now().UnixMilli() - lookbackMS
	prices, err := s.Prices(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*CredentialEnrichment, len(creds))
	lookup := make(map[string]string) // token -> key

	for _, c := range creds {
		result[c.Key] = &CredentialEnrichment{
			History: &CredentialUsageMetrics{
				CostComplete: true,
			},
			RecentStatuses: make([]string, 8),
		}
		if idx := strings.TrimSpace(c.AuthIndex); idx != "" {
			lookup["idx:"+idx] = c.Key
		}
		if id := strings.TrimSpace(c.AuthID); id != "" {
			lookup["id:"+id] = c.Key
			lookup["fn:"+filepath.Base(id)] = c.Key
		}
		if fn := strings.TrimSpace(c.FileName); fn != "" {
			lookup["fn:"+fn] = c.Key
			lookup["fn:"+filepath.Base(fn)] = c.Key
		}
		if src := strings.TrimSpace(c.Source); src != "" {
			lookup["src:"+src] = c.Key
			lookup["fn:"+filepath.Base(src)] = c.Key
		}
	}

	// 1. Optional: populate latest inspection cache if table exists
	inspRows, err := s.db.QueryContext(ctx, `
		select coalesce(auth_index,''), coalesce(auth_id,''), coalesce(file_name,''), coalesce(plan_type,''), coalesce(quota_windows_json,'')
		from codex_inspection_results
		order by id desc limit 200
	`)
	if err == nil {
		defer inspRows.Close()
		seen := make(map[string]bool)
		for inspRows.Next() {
			var authIndex, authID, fileName, planType, windowsJSON string
			if err := inspRows.Scan(&authIndex, &authID, &fileName, &planType, &windowsJSON); err == nil {
				key := ""
				if k, ok := lookup["fn:"+strings.TrimSpace(fileName)]; ok {
					key = k
				} else if k, ok := lookup["id:"+strings.TrimSpace(authID)]; ok {
					key = k
				} else if k, ok := lookup["idx:"+strings.TrimSpace(authIndex)]; ok {
					key = k
				}
				if key != "" && result[key] != nil && !seen[key] {
					seen[key] = true
					if planType != "" {
						result[key].PlanLabel = planType
					}
					if strings.TrimSpace(windowsJSON) != "" {
						var windows any
						if err := json.Unmarshal([]byte(windowsJSON), &windows); err == nil {
							result[key].QuotaWindows = windows
						}
					}
				}
			}
		}
	}

	// 2. Query usage_events in one pass
	// ponytail: limit 50000 events, stream or chunk if larger lookback needed
	query := `select id, timestamp_ms, coalesce(provider,''), coalesce(executor_type,''), model, coalesce(auth_id,''), coalesce(auth_index,''), coalesce(source,''), input_tokens, output_tokens, cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens, failed from usage_events where timestamp_ms >= ? order by timestamp_ms desc limit 50000`
	rows, err := s.db.QueryContext(ctx, query, fromMS)
	if err != nil {
		return result, nil // non-fatal fallback
	}
	defer rows.Close()

	// Track recent statuses per key (max 8)
	recentTrack := make(map[string][]string, len(creds))

	for rows.Next() {
		var id, ts, input, output, cached, read, create, total int64
		var failed int
		var provider, execType, model, authID, authIndex, source string
		if err := rows.Scan(&id, &ts, &provider, &execType, &model, &authID, &authIndex, &source, &input, &output, &cached, &read, &create, &total, &failed); err != nil {
			return nil, err
		}

		key := ""
		if authIndex != "" {
			if k, ok := lookup["idx:"+authIndex]; ok {
				key = k
			}
		}
		if key == "" && authID != "" {
			if k, ok := lookup["id:"+authID]; ok {
				key = k
			} else if k, ok := lookup["fn:"+authID]; ok {
				key = k
			} else if k, ok := lookup["fn:"+filepath.Base(authID)]; ok {
				key = k
			}
		}
		if key == "" && source != "" {
			if k, ok := lookup["src:"+source]; ok {
				key = k
			} else if k, ok := lookup["fn:"+source]; ok {
				key = k
			} else if k, ok := lookup["fn:"+filepath.Base(source)]; ok {
				key = k
			}
		}
		if key == "" {
			continue
		}

		enr := result[key]
		if enr == nil || enr.History == nil {
			continue
		}
		h := enr.History
		h.Requests++
		h.Tokens += total
		if failed != 0 {
			h.FailureCalls++
		} else {
			h.SuccessCalls++
		}
		if h.LastSeenMS == nil || ts > *h.LastSeenMS {
			last := ts
			h.LastSeenMS = &last
		}

		statuses := recentTrack[key]
		if len(statuses) < 8 {
			if failed != 0 {
				recentTrack[key] = append(statuses, "fail")
			} else {
				recentTrack[key] = append(recentTrack[key], "ok")
			}
		}

		row := eventRow{
			TimestampMS:         ts,
			Model:               model,
			Provider:            provider,
			ExecutorType:        execType,
			InputTokens:         input,
			OutputTokens:        output,
			CachedTokens:        cached,
			CacheReadTokens:     read,
			CacheCreationTokens: create,
			TotalTokens:         total,
		}
		est := estimateEventCost(row, prices[model])
		if est.Status == pricing.StatusEstimated {
			h.Cost += est.Amount
		} else {
			h.UnpricedCalls++
		}
	}

	for _, enr := range result {
		if enr.History.Requests > 0 {
			rate := float64(enr.History.SuccessCalls) / float64(enr.History.Requests)
			enr.History.SuccessRate = &rate
		}
		enr.History.CostComplete = enr.History.UnpricedCalls == 0
	}

	for k, statuses := range recentTrack {
		enr := result[k]
		if enr == nil {
			continue
		}
		padded := make([]string, 8)
		n := len(statuses)
		for i := 0; i < n; i++ {
			padded[7-i] = statuses[i] // newest at index 7
		}
		enr.RecentStatuses = padded
	}

	return result, nil
}
