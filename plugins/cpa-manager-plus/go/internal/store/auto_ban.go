package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Auto-Ban account states.
const (
	AutoBanStateIdle          = "idle"
	AutoBanStateFlagged       = "flagged"
	AutoBanStatePendingAction = "pending_action"
	AutoBanStateDisabled      = "disabled"
	AutoBanStateCooling       = "cooling"
	AutoBanStateEnabling      = "enabling"
	AutoBanStateHeld          = "held"
	AutoBanStateDeleted       = "deleted"
)

// Auto-Ban rule actions.
const (
	AutoBanActionNone           = "none"
	AutoBanActionReview         = "review"
	AutoBanActionDisable        = "disable"
	AutoBanActionDelete         = "delete"
	AutoBanActionCooldownEnable = "cooldown_enable"
)

// Capability bit flags.
const (
	AutoBanCapDisable = 1 << 0
	AutoBanCapEnable  = 1 << 1
	AutoBanCapDelete  = 1 << 2
)

// Source bit flags on rules.
const (
	AutoBanSourceUsage      = 1 << 0
	AutoBanSourceInspection = 1 << 1
)

// BanSignal is a normalized failure/success signal for Auto-Ban evaluation.
type BanSignal struct {
	AccountKey   string            `json:"accountKey"`
	Provider     string            `json:"provider"`
	AccountKind  string            `json:"accountKind"`
	FileName     string            `json:"fileName,omitempty"`
	AuthIndex    string            `json:"authIndex,omitempty"`
	AuthID       string            `json:"authId,omitempty"`
	APIKeyHash   string            `json:"apiKeyHash,omitempty"`
	DisplayName  string            `json:"displayName,omitempty"`
	StatusCode   int               `json:"statusCode,omitempty"`
	ErrorKind    string            `json:"errorKind,omitempty"`
	FailSummary  string            `json:"failSummary,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Source       string            `json:"source"` // usage | inspection | manual | scheduler
	AtMS         int64             `json:"atMs,omitempty"`
	Success      bool              `json:"success,omitempty"`
	Detail       any               `json:"detail,omitempty"`
	Capabilities int               `json:"capabilities,omitempty"`
}

// AutoBanRule is a configurable match + action rule.
type AutoBanRule struct {
	ID                       int64    `json:"id"`
	Enabled                  bool     `json:"enabled"`
	Priority                 int      `json:"priority"`
	Name                     string   `json:"name"`
	ProviderScope            string   `json:"providerScope"`
	AccountKind              string   `json:"accountKind"`
	MatchStatusCodes         []int    `json:"matchStatusCodes"`
	MatchErrorKinds          []string `json:"matchErrorKinds"`
	MatchBodySubstrings      []string `json:"matchBodySubstrings"`
	SourceMask               int      `json:"sourceMask"`
	ThresholdMode            string   `json:"thresholdMode"` // consecutive | total
	ThresholdCount           int      `json:"thresholdCount"`
	WindowMS                 *int64   `json:"windowMs,omitempty"`
	SuccessResetsConsecutive bool     `json:"successResetsConsecutive"`
	Action                   string   `json:"action"`
	CooldownMS               *int64   `json:"cooldownMs,omitempty"`
	CooldownSource           string   `json:"cooldownSource"`
	RespectHostCooldown      bool     `json:"respectHostCooldown"`
	MaxActionsPerDay         *int     `json:"maxActionsPerDay,omitempty"`
	CreatedAtMS              int64    `json:"createdAtMs"`
	UpdatedAtMS              int64    `json:"updatedAtMs"`
}

// AutoBanAccountState is the materialized FSM + counters for one account.
type AutoBanAccountState struct {
	ID                int64  `json:"id"`
	AccountKey        string `json:"accountKey"`
	Provider          string `json:"provider"`
	AccountKind       string `json:"accountKind"`
	FileName          string `json:"fileName,omitempty"`
	AuthIndex         string `json:"authIndex,omitempty"`
	AuthID            string `json:"authId,omitempty"`
	APIKeyHash        string `json:"apiKeyHash,omitempty"`
	DisplayName       string `json:"displayName,omitempty"`
	State             string `json:"state"`
	ActiveRuleID      *int64 `json:"activeRuleId,omitempty"`
	LastStatusCode    *int   `json:"lastStatusCode,omitempty"`
	LastErrorKind     string `json:"lastErrorKind,omitempty"`
	LastSignalAtMS    *int64 `json:"lastSignalAtMs,omitempty"`
	ConsecutiveHits   int    `json:"consecutiveHits"`
	TotalHits         int    `json:"totalHits"`
	WindowStartedAtMS *int64 `json:"windowStartedAtMs,omitempty"`
	CooldownUntilMS   *int64 `json:"cooldownUntilMs,omitempty"`
	CooldownReason    string `json:"cooldownReason,omitempty"`
	ManualHold        bool   `json:"manualHold"`
	ManualHoldReason  string `json:"manualHoldReason,omitempty"`
	LastAction        string `json:"lastAction,omitempty"`
	LastActionAtMS    *int64 `json:"lastActionAtMs,omitempty"`
	LastActionError   string `json:"lastActionError,omitempty"`
	CapabilityFlags   int    `json:"capabilityFlags"`
	DetailJSON        string `json:"detailJson,omitempty"`
	CreatedAtMS       int64  `json:"createdAtMs"`
	UpdatedAtMS       int64  `json:"updatedAtMs"`
}

// AutoBanHistory is an append-only audit event.
type AutoBanHistory struct {
	ID          int64  `json:"id"`
	AccountKey  string `json:"accountKey"`
	Provider    string `json:"provider,omitempty"`
	RuleID      *int64 `json:"ruleId,omitempty"`
	EventType   string `json:"eventType"`
	FromState   string `json:"fromState,omitempty"`
	ToState     string `json:"toState,omitempty"`
	StatusCode  *int   `json:"statusCode,omitempty"`
	ErrorKind   string `json:"errorKind,omitempty"`
	Source      string `json:"source,omitempty"`
	Action      string `json:"action,omitempty"`
	Message     string `json:"message,omitempty"`
	DetailJSON  string `json:"detailJson,omitempty"`
	Actor       string `json:"actor"`
	CreatedAtMS int64  `json:"createdAtMs"`
}

// AutoBanApplyResult is the outcome of applying one signal.
type AutoBanApplyResult struct {
	State           AutoBanAccountState `json:"state"`
	MatchedRule     *AutoBanRule        `json:"matchedRule,omitempty"`
	ThresholdMet    bool                `json:"thresholdMet"`
	ShouldExecute   bool                `json:"shouldExecute"`
	ExecuteAction   string              `json:"executeAction,omitempty"`
	Suppressed      string              `json:"suppressed,omitempty"`
	CooldownUntilMS *int64              `json:"cooldownUntilMs,omitempty"`
}

func (s *Store) ensureAutoBanSchema(ctx context.Context) error {
	statements := []string{
		`create table if not exists auto_ban_rules (
			id integer primary key autoincrement,
			enabled integer not null default 1,
			priority integer not null default 100,
			name text not null,
			provider_scope text not null,
			account_kind text not null default 'any',
			match_status_codes text not null default '[]',
			match_error_kinds text not null default '[]',
			match_body_substrings text not null default '[]',
			source_mask integer not null default 3,
			threshold_mode text not null,
			threshold_count integer not null,
			window_ms integer,
			success_resets_consecutive integer not null default 1,
			action text not null,
			cooldown_ms integer,
			cooldown_source text not null default 'header_or_default',
			respect_host_cooldown integer not null default 1,
			max_actions_per_day integer,
			created_at_ms integer not null,
			updated_at_ms integer not null
		)`,
		`create index if not exists idx_auto_ban_rules_lookup on auto_ban_rules(enabled, provider_scope, priority)`,
		`create table if not exists auto_ban_account_state (
			id integer primary key autoincrement,
			account_key text not null unique,
			provider text not null,
			account_kind text not null,
			file_name text,
			auth_index text,
			auth_id text,
			api_key_hash text,
			display_name text,
			state text not null,
			active_rule_id integer,
			last_status_code integer,
			last_error_kind text,
			last_signal_at_ms integer,
			consecutive_hits integer not null default 0,
			total_hits integer not null default 0,
			window_started_at_ms integer,
			cooldown_until_ms integer,
			cooldown_reason text,
			manual_hold integer not null default 0,
			manual_hold_reason text,
			last_action text,
			last_action_at_ms integer,
			last_action_error text,
			capability_flags integer not null default 0,
			detail_json text,
			created_at_ms integer not null,
			updated_at_ms integer not null
		)`,
		`create index if not exists idx_auto_ban_state_cooldown on auto_ban_account_state(state, cooldown_until_ms)`,
		`create index if not exists idx_auto_ban_state_provider on auto_ban_account_state(provider, state)`,
		`create table if not exists auto_ban_history (
			id integer primary key autoincrement,
			account_key text not null,
			provider text,
			rule_id integer,
			event_type text not null,
			from_state text,
			to_state text,
			status_code integer,
			error_kind text,
			source text,
			action text,
			message text,
			detail_json text,
			actor text not null default 'system',
			created_at_ms integer not null
		)`,
		`create index if not exists idx_auto_ban_history_account on auto_ban_history(account_key, created_at_ms desc)`,
		`create index if not exists idx_auto_ban_history_time on auto_ban_history(created_at_ms desc)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("auto ban schema: %w", err)
		}
	}
	return s.seedDefaultAutoBanRules(ctx)
}

func (s *Store) seedDefaultAutoBanRules(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `select count(*) from auto_ban_rules`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	seeds := []struct {
		priority       int
		name, provider string
		kind           string
		codes, kinds   string
		mode           string
		threshold      int
		windowMS       any
		action         string
		cooldownMS     any
		cooldownSource string
		respectHost    int
	}{
		{10, "codex-429-cooldown", "codex", "oauth_auth_file", `[429]`, `["rate_limited"]`, "consecutive", 1, nil, AutoBanActionCooldownEnable, nil, "header_or_default", 0},
		{20, "codex-401-disable", "codex", "oauth_auth_file", `[401]`, `[]`, "consecutive", 1, nil, AutoBanActionDisable, nil, "header_or_default", 0},
		{30, "xai-quota-disable", "xai", "oauth_auth_file", `[]`, `["quota_exhausted"]`, "consecutive", 1, nil, AutoBanActionDisable, nil, "header_or_default", 0},
		{40, "xai-429-observe", "xai", "oauth_auth_file", `[429]`, `["rate_limited"]`, "consecutive", 3, nil, AutoBanActionReview, nil, "header_or_default", 1},
		{50, "xai-auth-observe", "xai", "oauth_auth_file", `[401,402,403]`, `[]`, "consecutive", 1, nil, AutoBanActionReview, nil, "header_or_default", 1},
		{60, "custom-429-review", "custom", "custom_provider", `[429]`, `[]`, "total", 5, int64(15 * 60 * 1000), AutoBanActionReview, nil, "header_or_default", 0},
	}
	for _, seed := range seeds {
		_, err := s.db.ExecContext(ctx, `insert into auto_ban_rules(
			enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,
			source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,
			respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms
		) values(1,?,?,?,?,?,?,'[]',3,?,?,?,1,?,?,?,?,null,?,?)`,
			seed.priority, seed.name, seed.provider, seed.kind, seed.codes, seed.kinds,
			seed.mode, seed.threshold, seed.windowMS, seed.action, seed.cooldownMS, seed.cooldownSource,
			seed.respectHost, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// ListAutoBanRules returns all rules ordered by priority.
func (s *Store) ListAutoBanRules(ctx context.Context) ([]AutoBanRule, error) {
	rows, err := s.db.QueryContext(ctx, `select id,enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms from auto_ban_rules order by priority asc, id asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoBanRule{}
	for rows.Next() {
		rule, err := scanAutoBanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// GetAutoBanRule loads one rule by id.
func (s *Store) GetAutoBanRule(ctx context.Context, id int64) (AutoBanRule, error) {
	row := s.db.QueryRowContext(ctx, `select id,enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms from auto_ban_rules where id=?`, id)
	return scanAutoBanRule(row)
}

// UpsertAutoBanRule inserts or updates a rule.
func (s *Store) UpsertAutoBanRule(ctx context.Context, rule AutoBanRule) (AutoBanRule, error) {
	if err := validateAutoBanRule(rule); err != nil {
		return AutoBanRule{}, err
	}
	now := time.Now().UnixMilli()
	codes, _ := json.Marshal(rule.MatchStatusCodes)
	if rule.MatchStatusCodes == nil {
		codes = []byte("[]")
	}
	kinds, _ := json.Marshal(rule.MatchErrorKinds)
	if rule.MatchErrorKinds == nil {
		kinds = []byte("[]")
	}
	bodies, _ := json.Marshal(rule.MatchBodySubstrings)
	if rule.MatchBodySubstrings == nil {
		bodies = []byte("[]")
	}
	if rule.SourceMask == 0 {
		rule.SourceMask = AutoBanSourceUsage | AutoBanSourceInspection
	}
	if rule.CooldownSource == "" {
		rule.CooldownSource = "header_or_default"
	}
	if rule.AccountKind == "" {
		rule.AccountKind = "any"
	}
	if rule.ID > 0 {
		_, err := s.db.ExecContext(ctx, `update auto_ban_rules set enabled=?,priority=?,name=?,provider_scope=?,account_kind=?,match_status_codes=?,match_error_kinds=?,match_body_substrings=?,source_mask=?,threshold_mode=?,threshold_count=?,window_ms=?,success_resets_consecutive=?,action=?,cooldown_ms=?,cooldown_source=?,respect_host_cooldown=?,max_actions_per_day=?,updated_at_ms=? where id=?`,
			boolInt(rule.Enabled), rule.Priority, rule.Name, rule.ProviderScope, rule.AccountKind, string(codes), string(kinds), string(bodies),
			rule.SourceMask, rule.ThresholdMode, rule.ThresholdCount, nullInt64(rule.WindowMS), boolInt(rule.SuccessResetsConsecutive),
			rule.Action, nullInt64(rule.CooldownMS), rule.CooldownSource, boolInt(rule.RespectHostCooldown), nullIntValue(rule.MaxActionsPerDay), now, rule.ID)
		if err != nil {
			return AutoBanRule{}, err
		}
		return s.GetAutoBanRule(ctx, rule.ID)
	}
	result, err := s.db.ExecContext(ctx, `insert into auto_ban_rules(enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		boolInt(rule.Enabled), rule.Priority, rule.Name, rule.ProviderScope, rule.AccountKind, string(codes), string(kinds), string(bodies),
		rule.SourceMask, rule.ThresholdMode, rule.ThresholdCount, nullInt64(rule.WindowMS), boolInt(rule.SuccessResetsConsecutive),
		rule.Action, nullInt64(rule.CooldownMS), rule.CooldownSource, boolInt(rule.RespectHostCooldown), nullIntValue(rule.MaxActionsPerDay), now, now)
	if err != nil {
		return AutoBanRule{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return AutoBanRule{}, err
	}
	return s.GetAutoBanRule(ctx, id)
}

// DeleteAutoBanRule removes a rule.
func (s *Store) DeleteAutoBanRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `delete from auto_ban_rules where id=?`, id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ReplaceAutoBanRules replaces all rules atomically.
func (s *Store) ReplaceAutoBanRules(ctx context.Context, rules []AutoBanRule) ([]AutoBanRule, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `delete from auto_ban_rules`); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for _, rule := range rules {
		if err := validateAutoBanRule(rule); err != nil {
			return nil, err
		}
		codes, _ := json.Marshal(rule.MatchStatusCodes)
		if rule.MatchStatusCodes == nil {
			codes = []byte("[]")
		}
		kinds, _ := json.Marshal(rule.MatchErrorKinds)
		if rule.MatchErrorKinds == nil {
			kinds = []byte("[]")
		}
		bodies, _ := json.Marshal(rule.MatchBodySubstrings)
		if rule.MatchBodySubstrings == nil {
			bodies = []byte("[]")
		}
		sourceMask := rule.SourceMask
		if sourceMask == 0 {
			sourceMask = AutoBanSourceUsage | AutoBanSourceInspection
		}
		cooldownSource := rule.CooldownSource
		if cooldownSource == "" {
			cooldownSource = "header_or_default"
		}
		accountKind := rule.AccountKind
		if accountKind == "" {
			accountKind = "any"
		}
		if _, err := tx.ExecContext(ctx, `insert into auto_ban_rules(enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			boolInt(rule.Enabled), rule.Priority, rule.Name, rule.ProviderScope, accountKind, string(codes), string(kinds), string(bodies),
			sourceMask, rule.ThresholdMode, rule.ThresholdCount, nullInt64(rule.WindowMS), boolInt(rule.SuccessResetsConsecutive),
			rule.Action, nullInt64(rule.CooldownMS), cooldownSource, boolInt(rule.RespectHostCooldown), nullIntValue(rule.MaxActionsPerDay), now, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListAutoBanRules(ctx)
}

// ListAutoBanAccounts lists account states with optional filters.
func (s *Store) ListAutoBanAccounts(ctx context.Context, state, provider, q string, limit int) ([]AutoBanAccountState, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	query := `select id,account_key,provider,account_kind,coalesce(file_name,''),coalesce(auth_index,''),coalesce(auth_id,''),coalesce(api_key_hash,''),coalesce(display_name,''),state,active_rule_id,last_status_code,coalesce(last_error_kind,''),last_signal_at_ms,consecutive_hits,total_hits,window_started_at_ms,cooldown_until_ms,coalesce(cooldown_reason,''),manual_hold,coalesce(manual_hold_reason,''),coalesce(last_action,''),last_action_at_ms,coalesce(last_action_error,''),capability_flags,coalesce(detail_json,''),created_at_ms,updated_at_ms from auto_ban_account_state where 1=1`
	args := []any{}
	if state != "" {
		query += ` and state=?`
		args = append(args, state)
	}
	if provider != "" {
		query += ` and provider=?`
		args = append(args, strings.ToLower(provider))
	}
	if q != "" {
		query += ` and (account_key like ? or coalesce(file_name,'') like ? or coalesce(display_name,'') like ? or coalesce(auth_index,'') like ?)`
		like := "%" + q + "%"
		args = append(args, like, like, like, like)
	}
	query += ` order by updated_at_ms desc limit ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoBanAccountState{}
	for rows.Next() {
		item, err := scanAutoBanAccountState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// GetAutoBanAccount loads one account state by key.
func (s *Store) GetAutoBanAccount(ctx context.Context, accountKey string) (AutoBanAccountState, error) {
	row := s.db.QueryRowContext(ctx, `select id,account_key,provider,account_kind,coalesce(file_name,''),coalesce(auth_index,''),coalesce(auth_id,''),coalesce(api_key_hash,''),coalesce(display_name,''),state,active_rule_id,last_status_code,coalesce(last_error_kind,''),last_signal_at_ms,consecutive_hits,total_hits,window_started_at_ms,cooldown_until_ms,coalesce(cooldown_reason,''),manual_hold,coalesce(manual_hold_reason,''),coalesce(last_action,''),last_action_at_ms,coalesce(last_action_error,''),capability_flags,coalesce(detail_json,''),created_at_ms,updated_at_ms from auto_ban_account_state where account_key=?`, accountKey)
	return scanAutoBanAccountState(row)
}

// GetAutoBanAccountByID loads one account state by id.
func (s *Store) GetAutoBanAccountByID(ctx context.Context, id int64) (AutoBanAccountState, error) {
	row := s.db.QueryRowContext(ctx, `select id,account_key,provider,account_kind,coalesce(file_name,''),coalesce(auth_index,''),coalesce(auth_id,''),coalesce(api_key_hash,''),coalesce(display_name,''),state,active_rule_id,last_status_code,coalesce(last_error_kind,''),last_signal_at_ms,consecutive_hits,total_hits,window_started_at_ms,cooldown_until_ms,coalesce(cooldown_reason,''),manual_hold,coalesce(manual_hold_reason,''),coalesce(last_action,''),last_action_at_ms,coalesce(last_action_error,''),capability_flags,coalesce(detail_json,''),created_at_ms,updated_at_ms from auto_ban_account_state where id=?`, id)
	return scanAutoBanAccountState(row)
}

// ListDueAutoBanCooldowns returns cooling accounts whose cooldown has expired.
func (s *Store) ListDueAutoBanCooldowns(ctx context.Context, nowMS int64, limit int) ([]AutoBanAccountState, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `select id,account_key,provider,account_kind,coalesce(file_name,''),coalesce(auth_index,''),coalesce(auth_id,''),coalesce(api_key_hash,''),coalesce(display_name,''),state,active_rule_id,last_status_code,coalesce(last_error_kind,''),last_signal_at_ms,consecutive_hits,total_hits,window_started_at_ms,cooldown_until_ms,coalesce(cooldown_reason,''),manual_hold,coalesce(manual_hold_reason,''),coalesce(last_action,''),last_action_at_ms,coalesce(last_action_error,''),capability_flags,coalesce(detail_json,''),created_at_ms,updated_at_ms from auto_ban_account_state where state=? and cooldown_until_ms is not null and cooldown_until_ms<=? and manual_hold=0 order by cooldown_until_ms asc limit ?`, AutoBanStateCooling, nowMS, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoBanAccountState{}
	for rows.Next() {
		item, err := scanAutoBanAccountState(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListAutoBanHistory returns history for an account.
func (s *Store) ListAutoBanHistory(ctx context.Context, accountKey string, limit int) ([]AutoBanHistory, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `select id,account_key,coalesce(provider,''),rule_id,event_type,coalesce(from_state,''),coalesce(to_state,''),status_code,coalesce(error_kind,''),coalesce(source,''),coalesce(action,''),coalesce(message,''),coalesce(detail_json,''),actor,created_at_ms from auto_ban_history where account_key=? order by created_at_ms desc, id desc limit ?`, accountKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoBanHistory{}
	for rows.Next() {
		item, err := scanAutoBanHistory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// AppendAutoBanHistory appends one history event.
func (s *Store) AppendAutoBanHistory(ctx context.Context, entry AutoBanHistory) (AutoBanHistory, error) {
	if entry.AccountKey == "" || entry.EventType == "" {
		return AutoBanHistory{}, fmt.Errorf("history requires account_key and event_type")
	}
	if entry.Actor == "" {
		entry.Actor = "system"
	}
	if entry.CreatedAtMS == 0 {
		entry.CreatedAtMS = time.Now().UnixMilli()
	}
	result, err := s.db.ExecContext(ctx, `insert into auto_ban_history(account_key,provider,rule_id,event_type,from_state,to_state,status_code,error_kind,source,action,message,detail_json,actor,created_at_ms) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.AccountKey, nullText(entry.Provider), nullInt64(entry.RuleID), entry.EventType, nullText(entry.FromState), nullText(entry.ToState),
		nullInt(entry.StatusCode), nullText(entry.ErrorKind), nullText(entry.Source), nullText(entry.Action), nullText(entry.Message), nullText(entry.DetailJSON), entry.Actor, entry.CreatedAtMS)
	if err != nil {
		return AutoBanHistory{}, err
	}
	entry.ID, _ = result.LastInsertId()
	return entry, nil
}

// ApplyAutoBanSignal evaluates a signal against rules and updates counters/state.
// When a threshold is met and an executable action is required, state becomes pending_action
// and ShouldExecute is true — the caller performs the CPA side effect then calls TransitionAutoBanAction.
func (s *Store) ApplyAutoBanSignal(ctx context.Context, signal BanSignal, dryRun bool) (AutoBanApplyResult, error) {
	if signal.AccountKey == "" {
		return AutoBanApplyResult{}, fmt.Errorf("signal requires account_key")
	}
	if signal.AtMS == 0 {
		signal.AtMS = time.Now().UnixMilli()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoBanApplyResult{}, err
	}
	defer tx.Rollback()

	state, err := getOrCreateAutoBanStateTx(ctx, tx, signal)
	if err != nil {
		return AutoBanApplyResult{}, err
	}
	if signal.Capabilities != 0 {
		state.CapabilityFlags = signal.Capabilities
	}
	fromState := state.State

	if state.ManualHold || state.State == AutoBanStateHeld || state.State == AutoBanStateDeleted {
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, EventType: "suppressed_manual_hold",
			FromState: fromState, ToState: state.State, Source: signal.Source, Message: "manual hold or deleted blocks auto-ban",
			StatusCode: intPtrValue(signal.StatusCode), ErrorKind: signal.ErrorKind, Actor: "system", CreatedAtMS: signal.AtMS,
		})
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		return AutoBanApplyResult{State: state, Suppressed: "manual_hold"}, nil
	}

	// Success path: reset only the active consecutive rule when it opted in.
	if signal.Success {
		rules, err := listEnabledAutoBanRulesTx(ctx, tx)
		if err != nil {
			return AutoBanApplyResult{}, err
		}
		reset := false
		if state.ActiveRuleID != nil {
			for _, rule := range rules {
				if rule.ID == *state.ActiveRuleID {
					reset = rule.ThresholdMode == "consecutive" && rule.SuccessResetsConsecutive
					break
				}
			}
		}
		if reset && state.ConsecutiveHits > 0 {
			state.ConsecutiveHits = 0
			if err := appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
				AccountKey: state.AccountKey, Provider: state.Provider, RuleID: state.ActiveRuleID, EventType: "counter_reset",
				FromState: fromState, ToState: state.State, Source: signal.Source, Message: "success signal reset consecutive hits",
				Actor: "system", CreatedAtMS: signal.AtMS,
			}); err != nil {
				return AutoBanApplyResult{}, err
			}
		}
		state.LastSignalAtMS = &signal.AtMS
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		return AutoBanApplyResult{State: state}, nil
	}

	rules, err := listEnabledAutoBanRulesTx(ctx, tx)
	if err != nil {
		return AutoBanApplyResult{}, err
	}
	rule, ok := matchAutoBanRule(rules, signal)
	if !ok {
		state.LastSignalAtMS = &signal.AtMS
		if signal.StatusCode > 0 {
			code := signal.StatusCode
			state.LastStatusCode = &code
		}
		state.LastErrorKind = signal.ErrorKind
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		return AutoBanApplyResult{State: state}, nil
	}

	// Counters are scoped to the currently matched rule; do not carry a streak into a new rule.
	if state.ActiveRuleID != nil && *state.ActiveRuleID != rule.ID {
		state.ConsecutiveHits = 0
		state.TotalHits = 0
		state.WindowStartedAtMS = nil
		if err := appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{AccountKey: state.AccountKey, Provider: state.Provider, RuleID: state.ActiveRuleID, EventType: "counter_reset", FromState: fromState, ToState: state.State, Source: signal.Source, Message: "matched rule changed", Actor: "system", CreatedAtMS: signal.AtMS}); err != nil {
			return AutoBanApplyResult{}, err
		}
	}
	state.ActiveRuleID = &rule.ID

	// Update counters.
	state.LastSignalAtMS = &signal.AtMS
	if signal.StatusCode > 0 {
		code := signal.StatusCode
		state.LastStatusCode = &code
	}
	state.LastErrorKind = signal.ErrorKind
	if signal.FileName != "" {
		state.FileName = signal.FileName
	}
	if signal.AuthIndex != "" {
		state.AuthIndex = signal.AuthIndex
	}
	if signal.DisplayName != "" {
		state.DisplayName = signal.DisplayName
	}

	if rule.ThresholdMode == "total" {
		windowMS := int64(0)
		if rule.WindowMS != nil {
			windowMS = *rule.WindowMS
		}
		if windowMS > 0 {
			if state.WindowStartedAtMS == nil || signal.AtMS-*state.WindowStartedAtMS > windowMS {
				state.WindowStartedAtMS = &signal.AtMS
				state.TotalHits = 1
			} else {
				state.TotalHits++
			}
		} else {
			state.TotalHits++
		}
	} else {
		state.ConsecutiveHits++
	}

	thresholdMet := false
	if rule.ThresholdMode == "total" {
		thresholdMet = state.TotalHits >= rule.ThresholdCount
	} else {
		thresholdMet = state.ConsecutiveHits >= rule.ThresholdCount
	}

	detail, _ := json.Marshal(map[string]any{
		"statusCode": signal.StatusCode, "errorKind": signal.ErrorKind, "source": signal.Source,
		"failSummary": truncateAutoBan(signal.FailSummary, 512), "headers": signal.Headers, "rule": rule.Name,
	})
	state.DetailJSON = string(detail)

	_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
		AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "signal_matched",
		FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
		StatusCode: intPtrValue(signal.StatusCode), ErrorKind: signal.ErrorKind,
		Message:    fmt.Sprintf("matched rule %s hits consecutive=%d total=%d", rule.Name, state.ConsecutiveHits, state.TotalHits),
		DetailJSON: state.DetailJSON, Actor: "system", CreatedAtMS: signal.AtMS,
	})

	result := AutoBanApplyResult{State: state, MatchedRule: &rule, ThresholdMet: thresholdMet}

	if !thresholdMet {
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		result.State = state
		return result, nil
	}

	_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
		AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "threshold_reached",
		FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
		StatusCode: intPtrValue(signal.StatusCode), ErrorKind: signal.ErrorKind,
		Message: fmt.Sprintf("threshold met for rule %s", rule.Name), Actor: "system", CreatedAtMS: signal.AtMS,
	})

	// Already in an active ban/cool state: do not re-fire CPA spam.
	if state.State == AutoBanStateCooling || state.State == AutoBanStateDisabled || state.State == AutoBanStatePendingAction || state.State == AutoBanStateEnabling {
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		result.State = state
		result.Suppressed = "already_active"
		return result, nil
	}

	if rule.RespectHostCooldown {
		state.State = AutoBanStateFlagged
		state.ActiveRuleID = &rule.ID
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "suppressed_host_cooldown",
			FromState: fromState, ToState: state.State, Source: signal.Source, Action: AutoBanActionReview,
			Message: "respect_host_cooldown: observe only", Actor: "system", CreatedAtMS: signal.AtMS,
		})
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		result.State = state
		result.Suppressed = "host_cooldown"
		return result, nil
	}

	if rule.MaxActionsPerDay != nil && *rule.MaxActionsPerDay > 0 && (rule.Action == AutoBanActionDisable || rule.Action == AutoBanActionDelete || rule.Action == AutoBanActionCooldownEnable) {
		reached, err := autoBanDailyCapReachedTx(ctx, tx, state.AccountKey, rule.ID, *rule.MaxActionsPerDay, signal.AtMS)
		if err != nil {
			return AutoBanApplyResult{}, err
		}
		if reached {
			state.State = AutoBanStateFlagged
			state.ActiveRuleID = &rule.ID
			if err := appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "suppressed_daily_cap", FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action, StatusCode: intPtrValue(signal.StatusCode), ErrorKind: signal.ErrorKind, Message: "daily action cap reached", Actor: "system", CreatedAtMS: signal.AtMS}); err != nil {
				return AutoBanApplyResult{}, err
			}
			if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
				return AutoBanApplyResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return AutoBanApplyResult{}, err
			}
			result.State = state
			result.Suppressed = "daily_cap"
			return result, nil
		}
	}

	// Capability gate for destructive actions.
	if rule.Action == AutoBanActionDisable || rule.Action == AutoBanActionCooldownEnable {
		if state.CapabilityFlags&AutoBanCapDisable == 0 {
			state.State = AutoBanStateFlagged
			state.ActiveRuleID = &rule.ID
			_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
				AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "suppressed_capability",
				FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
				Message: "account lacks disable capability", Actor: "system", CreatedAtMS: signal.AtMS,
			})
			if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
				return AutoBanApplyResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return AutoBanApplyResult{}, err
			}
			result.State = state
			result.Suppressed = "capability"
			return result, nil
		}
	}
	if rule.Action == AutoBanActionDelete && state.CapabilityFlags&AutoBanCapDelete == 0 {
		state.State = AutoBanStateFlagged
		state.ActiveRuleID = &rule.ID
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "suppressed_capability",
			FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
			Message: "account lacks delete capability", Actor: "system", CreatedAtMS: signal.AtMS,
		})
		if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
			return AutoBanApplyResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutoBanApplyResult{}, err
		}
		result.State = state
		result.Suppressed = "capability"
		return result, nil
	}

	if rule.Action == AutoBanActionCooldownEnable && rule.CooldownSource == "header_only" {
		if _, ok := parseCooldownHeader(signal.Headers, signal.AtMS); !ok {
			state.State = AutoBanStateFlagged
			state.ActiveRuleID = &rule.ID
			if err := appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "suppressed_missing_reset", FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action, StatusCode: intPtrValue(signal.StatusCode), ErrorKind: signal.ErrorKind, Message: "cooldown response header is required", Actor: "system", CreatedAtMS: signal.AtMS}); err != nil {
				return AutoBanApplyResult{}, err
			}
			if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
				return AutoBanApplyResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return AutoBanApplyResult{}, err
			}
			result.State = state
			result.Suppressed = "missing_reset"
			return result, nil
		}
	}

	state.ActiveRuleID = &rule.ID

	switch rule.Action {
	case AutoBanActionNone:
		// no state change beyond counters
	case AutoBanActionReview:
		state.State = AutoBanStateFlagged
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "state_transition",
			FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
			Message: "flagged for review", Actor: "system", CreatedAtMS: signal.AtMS,
		})
	case AutoBanActionDisable, AutoBanActionDelete, AutoBanActionCooldownEnable:
		if dryRun {
			_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
				AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "action_attempt",
				FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
				Message: "dry-run: would execute action", Actor: "system", CreatedAtMS: signal.AtMS,
			})
		} else {
			state.State = AutoBanStatePendingAction
			result.ShouldExecute = true
			result.ExecuteAction = rule.Action
			if rule.Action == AutoBanActionCooldownEnable {
				until := resolveCooldownUntilMS(rule, signal, signal.AtMS)
				result.CooldownUntilMS = &until
			}
			_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
				AccountKey: state.AccountKey, Provider: state.Provider, RuleID: &rule.ID, EventType: "state_transition",
				FromState: fromState, ToState: state.State, Source: signal.Source, Action: rule.Action,
				Message: "pending automatic action", Actor: "system", CreatedAtMS: signal.AtMS,
			})
		}
	default:
		state.State = AutoBanStateFlagged
	}

	if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
		return AutoBanApplyResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutoBanApplyResult{}, err
	}
	result.State = state
	result.MatchedRule = &rule
	return result, nil
}

// TransitionAutoBanAction records the result of a CPA action attempt.
func (s *Store) TransitionAutoBanAction(ctx context.Context, accountKey, action string, success bool, actionErr string, cooldownUntilMS *int64, actor, source string) (AutoBanAccountState, error) {
	if actor == "" {
		actor = "system"
	}
	if source == "" {
		source = "system"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	defer tx.Rollback()
	state, err := getAutoBanStateTx(ctx, tx, accountKey)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	from := state.State
	now := time.Now().UnixMilli()
	state.LastAction = action
	state.LastActionAtMS = &now
	if success {
		state.LastActionError = ""
		switch action {
		case AutoBanActionDisable, "manual_disable":
			state.State = AutoBanStateDisabled
			state.CooldownUntilMS = nil
			state.CooldownReason = ""
			if action == "manual_disable" {
				state.ManualHold = true
				state.ManualHoldReason = "manual disable"
			}
		case AutoBanActionDelete, "manual_delete":
			state.State = AutoBanStateDeleted
			state.CooldownUntilMS = nil
		case AutoBanActionCooldownEnable:
			state.State = AutoBanStateCooling
			state.CooldownUntilMS = cooldownUntilMS
			state.CooldownReason = "cooldown_enable"
		case "enable", "manual_enable", "manual_unban", "cooldown_expire":
			state.State = AutoBanStateIdle
			state.CooldownUntilMS = nil
			state.CooldownReason = ""
			state.ConsecutiveHits = 0
			state.ActiveRuleID = nil
			if action == "manual_unban" || action == "manual_enable" {
				state.ManualHold = false
				state.ManualHoldReason = ""
			}
		case "hold":
			state.State = AutoBanStateHeld
			state.ManualHold = true
		case "release":
			state.ManualHold = false
			state.ManualHoldReason = ""
			if state.State == AutoBanStateHeld {
				if state.CooldownUntilMS != nil && *state.CooldownUntilMS > now {
					state.State = AutoBanStateCooling
				} else {
					state.State = AutoBanStateIdle
				}
			}
		case "enabling":
			state.State = AutoBanStateEnabling
		default:
			// keep state
		}
		eventType := "action_success"
		if strings.HasPrefix(action, "manual_") {
			eventType = action
		} else if action == "cooldown_expire" {
			eventType = "cooldown_expired"
		} else if action == AutoBanActionCooldownEnable {
			eventType = "cooldown_started"
		}
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, RuleID: state.ActiveRuleID, EventType: eventType,
			FromState: from, ToState: state.State, Source: source, Action: action, Actor: actor, CreatedAtMS: now,
			Message: "action succeeded",
		})
	} else {
		state.LastActionError = actionErr
		// Return pending/enabling to previous logical state.
		if state.State == AutoBanStatePendingAction || state.State == AutoBanStateEnabling {
			if action == AutoBanActionCooldownEnable || action == "cooldown_expire" || action == "enable" {
				state.State = AutoBanStateCooling
				if cooldownUntilMS != nil {
					state.CooldownUntilMS = cooldownUntilMS
				} else {
					retry := now + 5*60*1000
					state.CooldownUntilMS = &retry
				}
			} else if action == AutoBanActionDisable {
				// leave pending failed as flagged so operator can see it
				state.State = AutoBanStateFlagged
			} else {
				state.State = AutoBanStateFlagged
			}
		}
		_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
			AccountKey: state.AccountKey, Provider: state.Provider, RuleID: state.ActiveRuleID, EventType: "action_failed",
			FromState: from, ToState: state.State, Source: source, Action: action, Actor: actor, CreatedAtMS: now,
			Message: actionErr,
		})
	}
	if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
		return AutoBanAccountState{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutoBanAccountState{}, err
	}
	return state, nil
}

// ResetAutoBanCounters clears consecutive/total counters for an account.
func (s *Store) ResetAutoBanCounters(ctx context.Context, accountKey string) (AutoBanAccountState, error) {
	state, err := s.GetAutoBanAccount(ctx, accountKey)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	state.ConsecutiveHits = 0
	state.TotalHits = 0
	state.WindowStartedAtMS = nil
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	defer tx.Rollback()
	if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
		return AutoBanAccountState{}, err
	}
	_ = appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{
		AccountKey: accountKey, Provider: state.Provider, EventType: "counter_reset",
		FromState: state.State, ToState: state.State, Source: "manual", Actor: "user", CreatedAtMS: now,
		Message: "counters reset by user",
	})
	if err := tx.Commit(); err != nil {
		return AutoBanAccountState{}, err
	}
	return state, nil
}

// SetAutoBanManualHold enables or releases an operator hold without invoking CPA.
func (s *Store) SetAutoBanManualHold(ctx context.Context, accountKey string, hold bool, reason string) (AutoBanAccountState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	defer tx.Rollback()
	state, err := getAutoBanStateTx(ctx, tx, accountKey)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	from := state.State
	state.ManualHold = hold
	if hold {
		state.ManualHoldReason = strings.TrimSpace(reason)
		if state.ManualHoldReason == "" {
			state.ManualHoldReason = "manual hold"
		}
		if state.State == AutoBanStateIdle || state.State == AutoBanStateFlagged {
			state.State = AutoBanStateHeld
		}
	} else {
		state.ManualHoldReason = ""
		if state.State == AutoBanStateHeld {
			state.State = AutoBanStateIdle
		}
	}
	if err := saveAutoBanStateTx(ctx, tx, state); err != nil {
		return AutoBanAccountState{}, err
	}
	eventType := "manual_release"
	if hold {
		eventType = "manual_hold"
	}
	if err := appendAutoBanHistoryTx(ctx, tx, AutoBanHistory{AccountKey: state.AccountKey, Provider: state.Provider, RuleID: state.ActiveRuleID, EventType: eventType, FromState: from, ToState: state.State, Source: "manual", Message: state.ManualHoldReason, Actor: "user"}); err != nil {
		return AutoBanAccountState{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutoBanAccountState{}, err
	}
	return state, nil
}

// AutoBanAccountKey builds a stable account key.
func AutoBanAccountKey(provider, accountKind, authIndex, fileName, authID, apiKeyHash string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	accountKind = strings.TrimSpace(accountKind)
	if accountKind == "" {
		if apiKeyHash != "" && authIndex == "" && fileName == "" {
			accountKind = "custom_provider"
		} else {
			accountKind = "oauth_auth_file"
		}
	}
	if accountKind == "custom_provider" || provider == "custom" {
		id := firstNonEmptyAutoBan(apiKeyHash, authID, fileName, authIndex)
		return "custom:" + provider + ":" + id
	}
	id := firstNonEmptyAutoBan(authIndex, fileName, authID)
	return "oauth:" + provider + ":" + id
}

func resolveCooldownUntilMS(rule AutoBanRule, signal BanSignal, nowMS int64) int64 {
	switch rule.CooldownSource {
	case "fixed":
		if rule.CooldownMS != nil && *rule.CooldownMS > 0 {
			return nowMS + *rule.CooldownMS
		}
	case "header_only":
		if until, ok := parseCooldownHeader(signal.Headers, nowMS); ok {
			return until
		}
		// missing header: short fallback so caller can suppress
		return nowMS
	default: // header_or_default
		if until, ok := parseCooldownHeader(signal.Headers, nowMS); ok {
			return until
		}
		if rule.CooldownMS != nil && *rule.CooldownMS > 0 {
			return nowMS + *rule.CooldownMS
		}
		// Codex default 5h
		return nowMS + 5*60*60*1000
	}
	return nowMS + 5*60*60*1000
}

func parseCooldownHeader(headers map[string]string, nowMS int64) (int64, bool) {
	if len(headers) == 0 {
		return 0, false
	}
	// Prefer X-Ratelimit-Reset (unix sec or ms)
	for _, key := range []string{"X-Ratelimit-Reset", "x-ratelimit-reset", "X-RateLimit-Reset"} {
		if raw, ok := headers[key]; ok && strings.TrimSpace(raw) != "" {
			var n int64
			if _, err := fmt.Sscan(strings.TrimSpace(raw), &n); err == nil && n > 0 {
				if n < 1_000_000_000_000 { // seconds
					return n * 1000, true
				}
				return n, true
			}
		}
	}
	for _, key := range []string{"Retry-After", "retry-after"} {
		if raw, ok := headers[key]; ok && strings.TrimSpace(raw) != "" {
			raw = strings.TrimSpace(raw)
			var seconds int64
			if _, err := fmt.Sscan(raw, &seconds); err == nil && seconds >= 0 {
				return nowMS + seconds*1000, true
			}
		}
	}
	return 0, false
}

func matchAutoBanRule(rules []AutoBanRule, signal BanSignal) (AutoBanRule, bool) {
	sourceBit := AutoBanSourceUsage
	if signal.Source == "inspection" {
		sourceBit = AutoBanSourceInspection
	}
	provider := strings.ToLower(strings.TrimSpace(signal.Provider))
	kind := strings.TrimSpace(signal.AccountKind)
	if kind == "" {
		kind = "oauth_auth_file"
	}
	body := strings.ToLower(signal.FailSummary)

	var best *AutoBanRule
	bestScore := -1
	for i := range rules {
		rule := rules[i]
		if !rule.Enabled {
			continue
		}
		if rule.SourceMask&sourceBit == 0 {
			continue
		}
		scope := strings.ToLower(strings.TrimSpace(rule.ProviderScope))
		score := 0
		switch {
		case scope == provider:
			score = 300
		case scope == "custom" && (kind == "custom_provider" || provider == "custom"):
			score = 200
		case scope == "*":
			score = 100
		default:
			continue
		}
		if rule.AccountKind != "" && rule.AccountKind != "any" && rule.AccountKind != kind {
			continue
		}
		if !ruleMatchesSignal(rule, signal.StatusCode, signal.ErrorKind, body) {
			continue
		}
		// Lower priority number wins; among equal priority, earlier id (already sorted).
		// Convert to score with priority inversion.
		score = score*10000 - rule.Priority
		if best == nil || score > bestScore {
			copy := rule
			best = &copy
			bestScore = score
		}
	}
	if best == nil {
		return AutoBanRule{}, false
	}
	return *best, true
}

func ruleMatchesSignal(rule AutoBanRule, statusCode int, errorKind, body string) bool {
	hasCodes := len(rule.MatchStatusCodes) > 0
	hasKinds := len(rule.MatchErrorKinds) > 0
	hasBodies := len(rule.MatchBodySubstrings) > 0
	if !hasCodes && !hasKinds && !hasBodies {
		return false
	}
	codeOK, kindOK, bodyOK := !hasCodes, !hasKinds, !hasBodies
	if hasCodes {
		for _, code := range rule.MatchStatusCodes {
			if code == statusCode {
				codeOK = true
				break
			}
		}
	}
	if hasKinds {
		ek := strings.ToLower(strings.TrimSpace(errorKind))
		for _, kind := range rule.MatchErrorKinds {
			if strings.ToLower(strings.TrimSpace(kind)) == ek {
				kindOK = true
				break
			}
		}
	}
	if hasBodies {
		for _, sub := range rule.MatchBodySubstrings {
			if sub != "" && strings.Contains(body, strings.ToLower(sub)) {
				bodyOK = true
				break
			}
		}
	}
	// OR across configured dimensions
	return (hasCodes && codeOK) || (hasKinds && kindOK) || (hasBodies && bodyOK)
}

func validateAutoBanRule(rule AutoBanRule) error {
	if strings.TrimSpace(rule.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	if strings.TrimSpace(rule.ProviderScope) == "" {
		return fmt.Errorf("providerScope is required")
	}
	switch rule.ThresholdMode {
	case "consecutive", "total":
	default:
		return fmt.Errorf("thresholdMode must be consecutive or total")
	}
	if rule.ThresholdCount < 1 {
		return fmt.Errorf("thresholdCount must be >= 1")
	}
	switch rule.Action {
	case AutoBanActionNone, AutoBanActionReview, AutoBanActionDisable, AutoBanActionDelete, AutoBanActionCooldownEnable:
	default:
		return fmt.Errorf("unsupported action")
	}
	if rule.Action == AutoBanActionDelete && (rule.MaxActionsPerDay == nil || *rule.MaxActionsPerDay < 1) {
		return fmt.Errorf("delete rules require maxActionsPerDay >= 1")
	}
	if len(rule.MatchStatusCodes) == 0 && len(rule.MatchErrorKinds) == 0 && len(rule.MatchBodySubstrings) == 0 {
		return fmt.Errorf("rule must match at least one status code, error kind, or body substring")
	}
	return nil
}

func getOrCreateAutoBanStateTx(ctx context.Context, tx *sql.Tx, signal BanSignal) (AutoBanAccountState, error) {
	state, err := getAutoBanStateTx(ctx, tx, signal.AccountKey)
	if err == nil {
		return state, nil
	}
	if err != sql.ErrNoRows {
		return AutoBanAccountState{}, err
	}
	now := time.Now().UnixMilli()
	kind := signal.AccountKind
	if kind == "" {
		kind = "oauth_auth_file"
	}
	provider := strings.ToLower(strings.TrimSpace(signal.Provider))
	_, err = tx.ExecContext(ctx, `insert into auto_ban_account_state(account_key,provider,account_kind,file_name,auth_index,auth_id,api_key_hash,display_name,state,consecutive_hits,total_hits,manual_hold,capability_flags,created_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?,?,0,0,0,?,?,?)`,
		signal.AccountKey, provider, kind, nullText(signal.FileName), nullText(signal.AuthIndex), nullText(signal.AuthID), nullText(signal.APIKeyHash), nullText(signal.DisplayName),
		AutoBanStateIdle, signal.Capabilities, now, now)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	return getAutoBanStateTx(ctx, tx, signal.AccountKey)
}

func getAutoBanStateTx(ctx context.Context, tx *sql.Tx, accountKey string) (AutoBanAccountState, error) {
	row := tx.QueryRowContext(ctx, `select id,account_key,provider,account_kind,coalesce(file_name,''),coalesce(auth_index,''),coalesce(auth_id,''),coalesce(api_key_hash,''),coalesce(display_name,''),state,active_rule_id,last_status_code,coalesce(last_error_kind,''),last_signal_at_ms,consecutive_hits,total_hits,window_started_at_ms,cooldown_until_ms,coalesce(cooldown_reason,''),manual_hold,coalesce(manual_hold_reason,''),coalesce(last_action,''),last_action_at_ms,coalesce(last_action_error,''),capability_flags,coalesce(detail_json,''),created_at_ms,updated_at_ms from auto_ban_account_state where account_key=?`, accountKey)
	return scanAutoBanAccountState(row)
}

func saveAutoBanStateTx(ctx context.Context, tx *sql.Tx, state AutoBanAccountState) error {
	now := time.Now().UnixMilli()
	state.UpdatedAtMS = now
	_, err := tx.ExecContext(ctx, `update auto_ban_account_state set provider=?,account_kind=?,file_name=?,auth_index=?,auth_id=?,api_key_hash=?,display_name=?,state=?,active_rule_id=?,last_status_code=?,last_error_kind=?,last_signal_at_ms=?,consecutive_hits=?,total_hits=?,window_started_at_ms=?,cooldown_until_ms=?,cooldown_reason=?,manual_hold=?,manual_hold_reason=?,last_action=?,last_action_at_ms=?,last_action_error=?,capability_flags=?,detail_json=?,updated_at_ms=? where account_key=?`,
		state.Provider, state.AccountKind, nullText(state.FileName), nullText(state.AuthIndex), nullText(state.AuthID), nullText(state.APIKeyHash), nullText(state.DisplayName),
		state.State, nullInt64(state.ActiveRuleID), nullInt(state.LastStatusCode), nullText(state.LastErrorKind), nullInt64(state.LastSignalAtMS),
		state.ConsecutiveHits, state.TotalHits, nullInt64(state.WindowStartedAtMS), nullInt64(state.CooldownUntilMS), nullText(state.CooldownReason),
		boolInt(state.ManualHold), nullText(state.ManualHoldReason), nullText(state.LastAction), nullInt64(state.LastActionAtMS), nullText(state.LastActionError),
		state.CapabilityFlags, nullText(state.DetailJSON), now, state.AccountKey)
	return err
}

func listEnabledAutoBanRulesTx(ctx context.Context, tx *sql.Tx) ([]AutoBanRule, error) {
	rows, err := tx.QueryContext(ctx, `select id,enabled,priority,name,provider_scope,account_kind,match_status_codes,match_error_kinds,match_body_substrings,source_mask,threshold_mode,threshold_count,window_ms,success_resets_consecutive,action,cooldown_ms,cooldown_source,respect_host_cooldown,max_actions_per_day,created_at_ms,updated_at_ms from auto_ban_rules where enabled=1 order by priority asc, id asc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoBanRule{}
	for rows.Next() {
		rule, err := scanAutoBanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func appendAutoBanHistoryTx(ctx context.Context, tx *sql.Tx, entry AutoBanHistory) error {
	if entry.Actor == "" {
		entry.Actor = "system"
	}
	if entry.CreatedAtMS == 0 {
		entry.CreatedAtMS = time.Now().UnixMilli()
	}
	_, err := tx.ExecContext(ctx, `insert into auto_ban_history(account_key,provider,rule_id,event_type,from_state,to_state,status_code,error_kind,source,action,message,detail_json,actor,created_at_ms) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.AccountKey, nullText(entry.Provider), nullInt64(entry.RuleID), entry.EventType, nullText(entry.FromState), nullText(entry.ToState),
		nullInt(entry.StatusCode), nullText(entry.ErrorKind), nullText(entry.Source), nullText(entry.Action), nullText(entry.Message), nullText(entry.DetailJSON), entry.Actor, entry.CreatedAtMS)
	return err
}

func autoBanDailyCapReachedTx(ctx context.Context, tx *sql.Tx, accountKey string, ruleID int64, limit int, nowMS int64) (bool, error) {
	if limit < 1 {
		return false, nil
	}
	at := time.UnixMilli(nowMS).UTC()
	start := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
	var count int
	err := tx.QueryRowContext(ctx, `select count(*) from auto_ban_history where account_key=? and rule_id=? and created_at_ms>=? and event_type in ('action_success','cooldown_started','manual_delete')`, accountKey, ruleID, start).Scan(&count)
	if err != nil {
		return false, err
	}
	return count >= limit, nil
}

func scanAutoBanRule(row scanner) (AutoBanRule, error) {
	var rule AutoBanRule
	var enabled, successReset, respect int
	var codes, kinds, bodies string
	var window, cooldown sql.NullInt64
	var maxDay sql.NullInt64
	err := row.Scan(&rule.ID, &enabled, &rule.Priority, &rule.Name, &rule.ProviderScope, &rule.AccountKind, &codes, &kinds, &bodies,
		&rule.SourceMask, &rule.ThresholdMode, &rule.ThresholdCount, &window, &successReset, &rule.Action, &cooldown, &rule.CooldownSource,
		&respect, &maxDay, &rule.CreatedAtMS, &rule.UpdatedAtMS)
	if err != nil {
		return AutoBanRule{}, err
	}
	rule.Enabled = enabled != 0
	rule.SuccessResetsConsecutive = successReset != 0
	rule.RespectHostCooldown = respect != 0
	_ = json.Unmarshal([]byte(codes), &rule.MatchStatusCodes)
	_ = json.Unmarshal([]byte(kinds), &rule.MatchErrorKinds)
	_ = json.Unmarshal([]byte(bodies), &rule.MatchBodySubstrings)
	if rule.MatchStatusCodes == nil {
		rule.MatchStatusCodes = []int{}
	}
	if rule.MatchErrorKinds == nil {
		rule.MatchErrorKinds = []string{}
	}
	if rule.MatchBodySubstrings == nil {
		rule.MatchBodySubstrings = []string{}
	}
	if window.Valid {
		v := window.Int64
		rule.WindowMS = &v
	}
	if cooldown.Valid {
		v := cooldown.Int64
		rule.CooldownMS = &v
	}
	if maxDay.Valid {
		v := int(maxDay.Int64)
		rule.MaxActionsPerDay = &v
	}
	return rule, nil
}

func scanAutoBanAccountState(row scanner) (AutoBanAccountState, error) {
	var state AutoBanAccountState
	var activeRule, lastSignal, windowStart, cooldownUntil, lastActionAt sql.NullInt64
	var lastStatus sql.NullInt64
	var hold int
	err := row.Scan(&state.ID, &state.AccountKey, &state.Provider, &state.AccountKind, &state.FileName, &state.AuthIndex, &state.AuthID, &state.APIKeyHash, &state.DisplayName,
		&state.State, &activeRule, &lastStatus, &state.LastErrorKind, &lastSignal, &state.ConsecutiveHits, &state.TotalHits, &windowStart, &cooldownUntil, &state.CooldownReason,
		&hold, &state.ManualHoldReason, &state.LastAction, &lastActionAt, &state.LastActionError, &state.CapabilityFlags, &state.DetailJSON, &state.CreatedAtMS, &state.UpdatedAtMS)
	if err != nil {
		return AutoBanAccountState{}, err
	}
	state.ManualHold = hold != 0
	if activeRule.Valid {
		v := activeRule.Int64
		state.ActiveRuleID = &v
	}
	if lastStatus.Valid {
		v := int(lastStatus.Int64)
		state.LastStatusCode = &v
	}
	if lastSignal.Valid {
		v := lastSignal.Int64
		state.LastSignalAtMS = &v
	}
	if windowStart.Valid {
		v := windowStart.Int64
		state.WindowStartedAtMS = &v
	}
	if cooldownUntil.Valid {
		v := cooldownUntil.Int64
		state.CooldownUntilMS = &v
	}
	if lastActionAt.Valid {
		v := lastActionAt.Int64
		state.LastActionAtMS = &v
	}
	return state, nil
}

func scanAutoBanHistory(row scanner) (AutoBanHistory, error) {
	var entry AutoBanHistory
	var ruleID, status sql.NullInt64
	err := row.Scan(&entry.ID, &entry.AccountKey, &entry.Provider, &ruleID, &entry.EventType, &entry.FromState, &entry.ToState, &status, &entry.ErrorKind, &entry.Source, &entry.Action, &entry.Message, &entry.DetailJSON, &entry.Actor, &entry.CreatedAtMS)
	if err != nil {
		return AutoBanHistory{}, err
	}
	if ruleID.Valid {
		v := ruleID.Int64
		entry.RuleID = &v
	}
	if status.Valid {
		v := int(status.Int64)
		entry.StatusCode = &v
	}
	return entry, nil
}

func nullInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullIntValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func intPtrValue(code int) *int {
	if code <= 0 {
		return nil
	}
	v := code
	return &v
}

func firstNonEmptyAutoBan(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "unknown"
}

func truncateAutoBan(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
