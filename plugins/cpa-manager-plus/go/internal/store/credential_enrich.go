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
	AuthType  string
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
	lookup := make(map[string][]string) // token -> distinct credential keys
	identities := make(map[string]CredentialIdentity, len(creds))
	add := func(token, key string) {
		for _, existing := range lookup[token] {
			if existing == key {
				return
			}
		}
		lookup[token] = append(lookup[token], key)
	}

	for _, c := range creds {
		identities[c.Key] = c
		result[c.Key] = &CredentialEnrichment{
			History: &CredentialUsageMetrics{
				CostComplete: true,
			},
			RecentStatuses: make([]string, 8),
		}
		if idx := strings.TrimSpace(c.AuthIndex); idx != "" {
			add("idx:"+idx, c.Key)
		}
		if id := strings.TrimSpace(c.AuthID); id != "" {
			add("id:"+id, c.Key)
			add("fn:"+filepath.Base(id), c.Key)
		}
		if fn := strings.TrimSpace(c.FileName); fn != "" {
			add("fn:"+fn, c.Key)
			add("fn:"+filepath.Base(fn), c.Key)
		}
		if src := strings.TrimSpace(c.Source); src != "" {
			add("src:"+src, c.Key)
			add("fn:"+filepath.Base(src), c.Key)
		}
	}

	// Both history and cache use the same identity precedence. Only visit indexed
	// candidates, and count owners rather than aliases when rejecting ambiguity.
	match := func(authIndex, authID, source, provider, authType string) string {
		authIndex, authID, source = strings.TrimSpace(authIndex), strings.TrimSpace(authID), strings.TrimSpace(source)
		provider, authType = normalizeProvider(provider), normalizeAuthTypeSnapshot(authType)
		unique := func(tokens ...string) string {
			key := ""
			for _, token := range tokens {
				for _, candidate := range lookup[token] {
					c := identities[candidate]
					// Match the account-window contract: contradictory strong indices
					// or providers never fall through to a shared display filename.
					if idx := strings.TrimSpace(c.AuthIndex); idx != "" && authIndex != "" && idx != authIndex {
						continue
					}
					if p := normalizeProvider(c.Provider); p != "" && provider != "" && !strings.EqualFold(p, provider) {
						continue
					}
					if kind := normalizeAuthTypeSnapshot(c.AuthType); kind != "" && authType != "" && kind != authType {
						continue
					}
					if key != "" && key != candidate {
						return ""
					}
					key = candidate
				}
			}
			return key
		}
		if authIndex != "" && len(lookup["idx:"+authIndex]) > 0 {
			return unique("idx:" + authIndex)
		}
		if authID != "" && len(lookup["id:"+authID]) > 0 {
			return unique("id:" + authID)
		}
		var tokens []string
		if authID != "" {
			tokens = append(tokens, "fn:"+authID, "fn:"+filepath.Base(authID))
		}
		if source != "" {
			tokens = append(tokens, "src:"+source, "fn:"+source, "fn:"+filepath.Base(source))
		}
		return unique(tokens...)
	}

	// 1. Optional: populate latest inspection cache if table exists.
	inspRows, err := s.db.QueryContext(ctx, `
		select coalesce(auth_index,''), coalesce(auth_id,''), coalesce(file_name,''), coalesce(provider,''), coalesce(auth_type,''), coalesce(plan_type,''), coalesce(quota_windows_json,'')
		from codex_inspection_results
		order by id desc
	`)
	if err == nil {
		defer inspRows.Close()
		seen := make(map[string]bool)
		// Stream until every credential has a cache entry; a global row limit
		// would let repeated inspections of a busy account evict quiet accounts.
		for len(seen) < len(result) && inspRows.Next() {
			var authIndex, authID, fileName, provider, authType, planType, windowsJSON string
			if err := inspRows.Scan(&authIndex, &authID, &fileName, &provider, &authType, &planType, &windowsJSON); err != nil {
				return nil, err
			}
			key := match(authIndex, authID, fileName, provider, authType)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			result[key].PlanLabel = planType
			if strings.TrimSpace(windowsJSON) != "" {
				var windows any
				if err := json.Unmarshal([]byte(windowsJSON), &windows); err == nil {
					result[key].QuotaWindows = windows
				}
			}
		}
		if err := inspRows.Err(); err != nil {
			return nil, err
		}
		// Store has one DB connection; release it even after the early exit.
		if err := inspRows.Close(); err != nil {
			return nil, err
		}
	}

	// 2. Share complete pricing context with analytics and account windows.
	rows, err := s.enrichedEvents(ctx, `timestamp_ms >= ?`, fromMS)
	if err != nil {
		return nil, err
	}

	// Track recent statuses per key (max 8)
	recentTrack := make(map[string][]string, len(creds))

	providerLookup := providerSnapshots(rows)
	for _, row := range rows {
		row.Provider = resolvedProvider(row, providerLookup)
		key := match(row.AuthIndex, row.AuthID, row.Source, row.Provider, row.AuthType)
		if key == "" {
			continue
		}

		enr := result[key]
		if enr == nil || enr.History == nil {
			continue
		}
		h := enr.History
		h.Requests++
		h.Tokens += row.TotalTokens
		if row.Failed != 0 {
			h.FailureCalls++
		} else {
			h.SuccessCalls++
		}
		if h.LastSeenMS == nil || row.TimestampMS > *h.LastSeenMS {
			last := row.TimestampMS
			h.LastSeenMS = &last
		}

		statuses := recentTrack[key]
		if len(statuses) < 8 {
			if row.Failed != 0 {
				recentTrack[key] = append(statuses, "fail")
			} else {
				recentTrack[key] = append(recentTrack[key], "ok")
			}
		}

		est := estimateEventCost(row, prices[row.Model])
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
