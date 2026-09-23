package windowkeeper

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func Parse(body []byte, now time.Time) (Snapshot, error) {
	root, err := decodeRoot(body)
	if err != nil {
		return Snapshot{}, err
	}
	snap := Snapshot{
		PlanType: strings.ToLower(text(root, "plan_type", "planType", "account_plan_type")),
	}
	if info := limitObject(root, "rate_limit", "rateLimit", "rate_limits", "rateLimits"); info != nil {
		snap.Windows = append(snap.Windows, windowsFromLimit("codex", info, "", now)...)
	}
	if info := limitObject(root, "code_review_rate_limit", "codeReviewRateLimit", "code_review_rate_limits", "codeReviewRateLimits"); info != nil {
		snap.Windows = append(snap.Windows, windowsFromLimit("code_review", info, "", now)...)
	}
	snap.Windows = append(snap.Windows, additionalWindows(root, now)...)
	return snap, nil
}

func Select(windows []Window, opt Options) []Window {
	out := append([]Window(nil), windows...)
	mainKinds := map[string]bool{}
	for _, window := range out {
		if window.LimitID == "codex" {
			mainKinds[window.Kind] = true
		}
	}
	filter := map[string]bool{}
	for _, kind := range opt.Kinds {
		kind = strings.TrimSpace(kind)
		if kind != "" {
			filter[kind] = true
		}
	}
	mode := opt.IncludeAdditional
	if mode == "" {
		mode = "fill_gaps"
	}
	for i := range out {
		window := &out[i]
		gate := false
		switch {
		case window.LimitID == "codex":
			gate = true
		case window.LimitID == "code_review":
			gate = opt.IncludeCodeReview
		case strings.HasPrefix(window.LimitID, "additional:"):
			switch mode {
			case "all":
				gate = !window.OtherModel
			case "none":
				gate = false
			default:
				gate = !window.OtherModel && !mainKinds[window.Kind]
			}
		}
		if len(filter) > 0 && !filter[window.Kind] {
			gate = false
		}
		window.Gating = gate
	}
	return out
}

func Group(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "free", "guest", "free_workspace":
		return GroupFree
	case "plus", "team", "edu_plus", "self_serve_business_prolite":
		return GroupPlusTeam
	case "pro", "prolite", "business", "edu_pro", "ent26", "enterprise", "enterprise_cbp_automation", "enterprise_cbp_usage_based":
		return GroupProAndAbove
	default:
		if strings.Contains(strings.ToLower(plan), "enterprise") {
			return GroupProAndAbove
		}
		return GroupObserveOnly
	}
}

func ExpectedKinds(group string) []string {
	switch group {
	case GroupFree:
		return []string{KindMonthly}
	case GroupPlusTeam:
		return []string{KindFiveHour, KindWeekly, KindMonthly}
	case GroupProAndAbove:
		return []string{KindWeekly, KindMonthly}
	default:
		return nil
	}
}

func Mismatch(group string, windows []Window) string {
	want := ExpectedKinds(group)
	if len(want) == 0 {
		return ""
	}
	got := map[string]bool{}
	for _, window := range windows {
		if window.LimitID == "code_review" {
			continue
		}
		got[window.Kind] = true
	}
	var extra, missing []string
	seen := map[string]bool{}
	for kind := range got {
		if !has(want, kind) && !seen[kind] {
			extra = append(extra, kind)
			seen[kind] = true
		}
	}
	for _, kind := range want {
		if !got[kind] {
			missing = append(missing, kind)
		}
	}
	if len(extra) == 0 && len(missing) == 0 {
		return ""
	}
	return "extra=" + strings.Join(extra, ",") + ";missing=" + strings.Join(missing, ",")
}

func MarkOtherModels(windows []Window, model string) []Window {
	for i := range windows {
		windows[i].OtherModel = otherModel(windows[i].RawName, model)
	}
	return windows
}

func decodeRoot(body []byte) (map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if nested, ok := root["body"].(string); ok && strings.TrimSpace(nested) != "" {
		var inner map[string]any
		if err := json.Unmarshal([]byte(nested), &inner); err == nil {
			return inner, nil
		}
	}
	if nested, ok := root["body"].(map[string]any); ok {
		return nested, nil
	}
	return root, nil
}

func additionalWindows(root map[string]any, now time.Time) []Window {
	raw, ok := root["additional_rate_limits"]
	if !ok {
		raw = root["additionalRateLimits"]
	}
	var out []Window
	switch typed := raw.(type) {
	case []any:
		for i, item := range typed {
			obj, _ := item.(map[string]any)
			if obj == nil {
				continue
			}
			name := text(obj, "limit_name", "limitName", "name", "normal_model_slug", "normalModelSlug")
			info := limitObject(obj, "rate_limit", "rateLimit")
			if info == nil {
				info = obj
			}
			id := "additional:" + slug(name)
			if strings.TrimSpace(name) == "" {
				id = "additional:" + strconv.Itoa(i)
			}
			out = append(out, windowsFromLimit(id, info, name, now)...)
		}
	case map[string]any:
		for name, item := range typed {
			obj, _ := item.(map[string]any)
			if obj == nil {
				continue
			}
			info := limitObject(obj, "rate_limit", "rateLimit")
			if info == nil {
				info = obj
			}
			label := text(obj, "limit_name", "limitName", "normal_model_slug", "normalModelSlug")
			if label == "" {
				label = name
			}
			out = append(out, windowsFromLimit("additional:"+slug(name), info, label, now)...)
		}
	}
	return out
}

func windowsFromLimit(limitID string, info map[string]any, modelName string, now time.Time) []Window {
	parentReached := truth(info, "limit_reached", "limitReached") || !boolDefault(info, true, "allowed")
	var out []Window
	for _, slot := range []string{"primary", "secondary"} {
		raw := object(info, slot+"_window", slot+"Window", slot)
		if raw == nil {
			continue
		}
		out = append(out, oneWindow(limitID, slot, raw, parentReached, modelName, now))
	}
	return out
}

func oneWindow(limitID, slot string, raw map[string]any, parentReached bool, modelName string, now time.Time) Window {
	seconds := periodSeconds(raw)
	reached := parentReached || truth(raw, "limit_reached", "limitReached")
	used, _ := number(raw, "used_percent", "usedPercent", "usage_percent", "usagePercent", "utilization")
	if reached && used == 0 {
		used = 100
	}
	end, endOK := instant(raw, "reset_at", "resetAt", "resets_at", "resetsAt", "reset_time", "resetTime")
	start, startOK := instant(raw, "start_at", "startAt", "started_at", "startedAt", "window_start", "windowStart", "window_started_at", "windowStartedAt")
	source := ""
	if startOK {
		source = SourceExplicit
	}
	if !endOK {
		if after, ok := number(raw, "reset_after_seconds", "resetAfterSeconds", "reset_in", "resetIn"); ok && after > 0 {
			end = now.Add(time.Duration(after * float64(time.Second)))
			endOK = true
			if source == "" {
				source = SourceProbeRelative
			}
		}
	}
	if endOK && !startOK && seconds > 0 {
		start = end.Add(-time.Duration(seconds) * time.Second)
		if source == "" {
			source = SourceDerived
		}
	}
	if source == "" && endOK {
		source = SourceExplicit
	}
	name := modelName
	if name == "" {
		name = text(raw, "limit_name", "limitName", "normal_model_slug", "normalModelSlug")
	}
	return Window{
		LimitID: limitID, Slot: slot, Kind: kindFor(seconds), PeriodSeconds: seconds,
		UsedPercent: used, LimitReached: reached, StartsAt: start.UTC(), EndsAt: end.UTC(),
		TimeSource: source, RawName: name,
	}
}

func otherModel(name, model string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	model = strings.ToLower(strings.TrimSpace(model))
	if name == "" || model == "" || name == "codex" {
		return false
	}
	if strings.Contains(name, model) {
		return false
	}
	return strings.Contains(name, "gpt") || strings.Contains(name, "sol")
}

func kindFor(seconds int64) string {
	if seconds <= 0 {
		return KindCustom
	}
	if near(float64(seconds), 18000) {
		return KindFiveHour
	}
	if near(float64(seconds), 604800) {
		return KindWeekly
	}
	if seconds >= 28*24*3600 && seconds <= 31*24*3600 {
		return KindMonthly
	}
	return KindCustom
}

func near(value, target float64) bool {
	return math.Abs(value-target)/target <= 0.02
}

func periodSeconds(raw map[string]any) int64 {
	if value, ok := number(raw, "limit_window_seconds", "limitWindowSeconds"); ok && value > 0 {
		return int64(math.Round(value))
	}
	if value, ok := number(raw, "window_minutes", "windowMinutes"); ok && value > 0 {
		return int64(math.Round(value * 60))
	}
	return 0
}

func limitObject(root map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		obj := object(root, key)
		if obj == nil {
			continue
		}
		if inner := object(obj, "rate_limit", "rateLimit"); inner != nil {
			return inner
		}
		return obj
	}
	return nil
}

func object(root map[string]any, keys ...string) map[string]any {
	if root == nil {
		return nil
	}
	for _, key := range keys {
		if value, ok := root[key].(map[string]any); ok {
			return value
		}
	}
	return nil
}

func text(root map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := root[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		}
	}
	return ""
}

func number(root map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := root[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case json.Number:
			parsed, err := typed.Float64()
			return parsed, err == nil
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			return parsed, err == nil
		}
	}
	return 0, false
}

func truth(root map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := root[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return strings.EqualFold(typed, "true") || typed == "1"
		}
	}
	return false
}

func boolDefault(root map[string]any, fallback bool, keys ...string) bool {
	for _, key := range keys {
		value, ok := root[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return strings.EqualFold(typed, "true") || typed == "1"
		}
	}
	return fallback
}

func instant(root map[string]any, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		value, ok := root[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return fromEpoch(int64(typed))
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return fromEpoch(parsed)
			}
		case string:
			typed = strings.TrimSpace(typed)
			if parsed, err := strconv.ParseInt(typed, 10, 64); err == nil {
				return fromEpoch(parsed)
			}
			if t, err := time.Parse(time.RFC3339, typed); err == nil {
				return t, true
			}
		}
	}
	return time.Time{}, false
}

func fromEpoch(val int64) (time.Time, bool) {
	if val <= 0 {
		return time.Time{}, false
	}
	if val > 1_000_000_000_000 {
		return time.UnixMilli(val), true
	}
	return time.Unix(val, 0), true
}

func slug(val string) string {
	val = strings.ToLower(strings.TrimSpace(val))
	var out []rune
	for _, r := range val {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		} else if len(out) > 0 && out[len(out)-1] != '-' {
			out = append(out, '-')
		}
	}
	return strings.Trim(string(out), "-")
}

func has(list []string, item string) bool {
	for _, entry := range list {
		if entry == item {
			return true
		}
	}
	return false
}
