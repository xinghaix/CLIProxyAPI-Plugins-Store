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
	if len(creds) == 0 {
		return map[string]*CredentialEnrichment{}, nil
	}
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

	snapshotAt := make(map[string]int64, len(result))
	applyQuotaSnapshot := func(key, provider, planType, windowsJSON string, atMS int64) {
		if key == "" {
			return
		}
		if prev, ok := snapshotAt[key]; ok && prev > atMS {
			return
		}
		effProvider := strings.TrimSpace(provider)
		if effProvider == "" {
			effProvider = identities[key].Provider
		}
		if strings.TrimSpace(planType) != "" || snapshotAt[key] == 0 {
			result[key].PlanLabel = planType
		}
		if strings.TrimSpace(windowsJSON) != "" {
			var windows any
			if err := json.Unmarshal([]byte(windowsJSON), &windows); err == nil {
				result[key].QuotaWindows = NormalizeQuotaWindows(effProvider, windows)
			}
		}
		snapshotAt[key] = atMS
	}

	// 1a. Populate latest inspection cache if table exists.
	// Pre-deduplicate by identity tuple in SQL so uninspected credentials do not
	// force a full table scan over all historical inspection runs.
	inspRows, err := s.db.QueryContext(ctx, `
		select coalesce(auth_index,''), coalesce(auth_id,''), coalesce(file_name,''), coalesce(provider,''), coalesce(auth_type,''), coalesce(plan_type,''), coalesce(quota_windows_json,''), coalesce(created_at_ms, 0)
		from codex_inspection_results
		where id in (
			select max(id) from codex_inspection_results
			group by account_key, coalesce(auth_index,''), coalesce(auth_id,''), coalesce(file_name,''), coalesce(provider,''), coalesce(auth_type,'')
		)
		order by id desc
	`)
	if err == nil {
		defer inspRows.Close()
		seen := make(map[string]bool)
		// Stream until every credential has a cache entry; a global row limit
		// would let repeated inspections of a busy account evict quiet accounts.
		for len(seen) < len(result) && inspRows.Next() {
			var authIndex, authID, fileName, provider, authType, planType, windowsJSON string
			var createdAtMS int64
			if err := inspRows.Scan(&authIndex, &authID, &fileName, &provider, &authType, &planType, &windowsJSON, &createdAtMS); err != nil {
				return nil, err
			}
			key := match(authIndex, authID, fileName, provider, authType)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			applyQuotaSnapshot(key, provider, planType, windowsJSON, createdAtMS)
		}
		if err := inspRows.Err(); err != nil {
			return nil, err
		}
		// Store has one DB connection; release it even after the early exit.
		if err := inspRows.Close(); err != nil {
			return nil, err
		}
	}

	// 1b. Merge standalone credential_quota_snapshots (written by on-demand quota
	// probes even when scheduled inspection is disabled).
	snapRows, err := s.db.QueryContext(ctx, `
		select coalesce(auth_index,''), coalesce(auth_id,''), coalesce(file_name,''), coalesce(provider,''), coalesce(auth_type,''), coalesce(plan_type,''), coalesce(quota_windows_json,''), coalesce(updated_at_ms, 0)
		from credential_quota_snapshots
		order by updated_at_ms desc
	`)
	if err == nil {
		defer snapRows.Close()
		for snapRows.Next() {
			var authIndex, authID, fileName, provider, authType, planType, windowsJSON string
			var updatedAtMS int64
			if err := snapRows.Scan(&authIndex, &authID, &fileName, &provider, &authType, &planType, &windowsJSON, &updatedAtMS); err != nil {
				return nil, err
			}
			key := match(authIndex, authID, fileName, provider, authType)
			if key == "" {
				continue
			}
			applyQuotaSnapshot(key, provider, planType, windowsJSON, updatedAtMS)
		}
		if err := snapRows.Close(); err != nil {
			return nil, err
		}
	}

	// 1c. Cooperate with window_keeper: if a Codex credential has no cached quota
	// windows yet (or window_keeper has a fresher observation), synthesize standard
	// 5h / weekly windows from window_keeper_windows.
	s.enrichFromWindowKeeper(ctx, result, snapshotAt, match)

	// 2. Share complete pricing context with analytics and account windows.
	rows, err := s.enrichedPricingEvents(ctx, `timestamp_ms >= ?`, fromMS)
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

// SaveCredentialQuotaSnapshot persists the latest quota windows and plan label
// from an on-demand probe so OAuth Credentials works independently even when
// scheduled inspection and window keeper are disabled.
func (s *Store) SaveCredentialQuotaSnapshot(ctx context.Context, result InspectionResult) error {
	if s == nil || s.db == nil {
		return nil
	}
	accountKey := strings.TrimSpace(result.AccountKey)
	if accountKey == "" {
		accountKey = strings.TrimSpace(result.AuthIndex)
	}
	if accountKey == "" {
		accountKey = strings.TrimSpace(result.AuthID)
	}
	if accountKey == "" {
		accountKey = strings.TrimSpace(result.FileName)
	}
	if accountKey == "" {
		return nil
	}
	windowsJSON := ""
	if result.QuotaWindows != nil {
		normalized := NormalizeQuotaWindows(result.Provider, result.QuotaWindows)
		if raw, err := json.Marshal(normalized); err == nil {
			windowsJSON = string(raw)
		}
	}
	if strings.TrimSpace(result.PlanType) == "" && strings.TrimSpace(windowsJSON) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		insert into credential_quota_snapshots(account_key, auth_index, auth_id, file_name, provider, auth_type, plan_type, quota_windows_json, updated_at_ms)
		values(?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(account_key) do update set
			auth_index = excluded.auth_index,
			auth_id = excluded.auth_id,
			file_name = excluded.file_name,
			provider = excluded.provider,
			auth_type = excluded.auth_type,
			plan_type = case when excluded.plan_type != '' then excluded.plan_type else credential_quota_snapshots.plan_type end,
			quota_windows_json = case when excluded.quota_windows_json != '' then excluded.quota_windows_json else credential_quota_snapshots.quota_windows_json end,
			updated_at_ms = excluded.updated_at_ms
	`,
		accountKey,
		strings.TrimSpace(result.AuthIndex),
		strings.TrimSpace(result.AuthID),
		strings.TrimSpace(result.FileName),
		strings.TrimSpace(result.Provider),
		strings.TrimSpace(result.AuthType),
		strings.TrimSpace(result.PlanType),
		windowsJSON,
		time.Now().UnixMilli(),
	)
	return err
}

func (s *Store) enrichFromWindowKeeper(
	ctx context.Context,
	result map[string]*CredentialEnrichment,
	snapshotAt map[string]int64,
	match func(authIndex, authID, source, provider, authType string) string,
) {
	rows, err := s.db.QueryContext(ctx, `
		select
			coalesce(a.auth_index, ''),
			coalesce(a.auth_id, ''),
			coalesce(a.display_name, ''),
			coalesce(a.plan_type, ''),
			coalesce(a.updated_at_ms, 0),
			coalesce(w.limit_id, ''),
			coalesce(w.slot, ''),
			coalesce(w.period_seconds, 0),
			coalesce(w.ends_at_ms, 0),
			coalesce(w.used_percent, 0),
			coalesce(w.absent, 0)
		from window_keeper_accounts a
		left join window_keeper_windows w on w.account_id = a.auth_id
		order by a.auth_id, w.period_seconds asc
	`)
	if err != nil {
		return
	}
	defer rows.Close()

	type wkRow struct {
		limitID       string
		slot          string
		periodSeconds int64
		endsAtMS      int64
		usedPercent   float64
		absent        int
	}
	byKey := make(map[string][]wkRow)
	planByKey := make(map[string]string)
	updatedByKey := make(map[string]int64)

	for rows.Next() {
		var authIndex, authID, name, planType, limitID, slot string
		var updatedAtMS, periodSeconds, endsAtMS int64
		var usedPercent float64
		var absent int
		if err := rows.Scan(&authIndex, &authID, &name, &planType, &updatedAtMS, &limitID, &slot, &periodSeconds, &endsAtMS, &usedPercent, &absent); err != nil {
			return
		}
		key := match(authIndex, authID, name, "codex", "oauth")
		if key == "" {
			continue
		}
		if updatedAtMS > updatedByKey[key] {
			updatedByKey[key] = updatedAtMS
		}
		if strings.TrimSpace(planType) != "" && planByKey[key] == "" {
			planByKey[key] = strings.TrimSpace(planType)
		}
		if absent != 0 || periodSeconds <= 0 || limitID != "codex" {
			continue
		}
		byKey[key] = append(byKey[key], wkRow{
			limitID:       limitID,
			slot:          slot,
			periodSeconds: periodSeconds,
			endsAtMS:      endsAtMS,
			usedPercent:   usedPercent,
			absent:        absent,
		})
	}
	_ = rows.Close()

	for key, enr := range result {
		if enr == nil {
			continue
		}
		if enr.PlanLabel == "" && planByKey[key] != "" {
			enr.PlanLabel = planByKey[key]
		}
		if enr.QuotaWindows != nil && updatedByKey[key] <= snapshotAt[key] {
			continue
		}
		wkWindows := byKey[key]
		if len(wkWindows) == 0 {
			continue
		}
		synthesized := make([]map[string]any, 0, len(wkWindows))
		for _, w := range wkWindows {
			id := "five-hour"
			kind := "five_hour"
			label := "5 小时限额"
			periodHours := float64(w.periodSeconds) / 3600.0
			if w.periodSeconds >= 6*24*3600 {
				id = "weekly"
				kind = "weekly"
				label = "周限额"
			} else if w.periodSeconds >= 25*24*3600 {
				id = "monthly"
				kind = "monthly"
				label = "月限额"
			}
			used := w.usedPercent
			if used < 0 {
				used = 0
			} else if used > 100 {
				used = 100
			}
			rem := 100 - used
			winMap := map[string]any{
				"id":                 id,
				"kind":               kind,
				"label":              label,
				"usedPercent":        used,
				"remainingPercent":   rem,
				"periodHours":        periodHours,
				"limitWindowSeconds": w.periodSeconds,
				"modelScope":         "account",
			}
			if w.endsAtMS > 0 {
				winMap["resetAt"] = time.UnixMilli(w.endsAtMS).UTC().Format(time.RFC3339)
			}
			synthesized = append(synthesized, winMap)
		}
		if len(synthesized) > 0 {
			enr.QuotaWindows = synthesized
		}
	}
}
