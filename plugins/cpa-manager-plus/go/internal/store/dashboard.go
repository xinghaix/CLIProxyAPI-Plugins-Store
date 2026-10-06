package store

import (
	"context"
	"sort"
	"time"
)

func (s *Store) Dashboard(ctx context.Context, todayStartMS, nowMS int64) (map[string]any, error) {
	if nowMS <= 0 {
		nowMS = time.Now().UnixMilli()
	}
	if todayStartMS < 0 || todayStartMS > nowMS {
		todayStartMS = nowMS - 24*60*60*1000
	}
	analytics, err := s.Analytics(ctx, AnalyticsRequest{FromMS: todayStartMS, ToMS: nowMS, IncludeFailed: true, Limit: 5, Granularity: "hour"})
	if err != nil {
		return nil, err
	}
	rolling30m, err := s.rolling(ctx, nowMS)
	if err != nil {
		return nil, err
	}
	today := analytics["summary"]
	models, _ := analytics["model_stats"].([]map[string]any)
	sort.Slice(models, func(i, j int) bool { return models[i]["cost"].(float64) > models[j]["cost"].(float64) })
	if len(models) > 5 {
		models = models[:5]
	}
	failures, _ := analytics["recent_failures"].([]map[string]any)
	return map[string]any{
		"today":            today,
		"rolling_30m":      rolling30m,
		"traffic_timeline": analytics["timeline"],
		"top_models_today": models,
		"model_cost_rank":  models,
		"recent_failures":  failures,
		"channel_health":   []any{},
		"token_mix":        []any{},
		"config_summary":   map[string]any{},
	}, nil
}

func (s *Store) rolling(ctx context.Context, nowMS int64) (map[string]any, error) {
	var calls, tokens int64
	err := s.db.QueryRowContext(ctx, `select count(*), coalesce(sum(total_tokens),0) from usage_events where timestamp_ms >= ? and timestamp_ms <= ?`, nowMS-30*60*1000, nowMS).Scan(&calls, &tokens)
	if err != nil {
		return nil, err
	}
	return map[string]any{"rpm": float64(calls) / 30, "tpm": float64(tokens) / 30, "total_calls": calls, "total_tokens": tokens}, nil
}
