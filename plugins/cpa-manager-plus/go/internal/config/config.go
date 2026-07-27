package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultDataDirectory = "data/cpa-manager-plus"
	defaultQueueCapacity = 1_024
	defaultBatchSize     = 64
)

// Config contains only settings owned by the local plugin runtime.
type Config struct {
	DataDir       string          `yaml:"data_dir"`
	QueueCapacity int             `yaml:"queue_capacity"`
	BatchSize     int             `yaml:"batch_size"`
	Collector     CollectorConfig `yaml:"collector"`
	Codex         CodexConfig     `yaml:"codex_inspection"`
}

type CollectorConfig struct {
	Enabled bool `yaml:"enabled"`
}

type CodexConfig struct {
	Enabled         bool   `yaml:"enabled"`
	ScheduleMode    string `yaml:"schedule_mode"`
	IntervalMinutes int    `yaml:"interval_minutes"`
	AutoActionMode  string `yaml:"auto_action_mode"`
}

func Default() Config {
	return Config{
		QueueCapacity: defaultQueueCapacity,
		BatchSize:     defaultBatchSize,
		Collector:     CollectorConfig{Enabled: true},
		Codex: CodexConfig{
			ScheduleMode:    "interval",
			IntervalMinutes: 60,
			AutoActionMode:  "none",
		},
	}
}

func Parse(raw []byte) (Config, error) {
	cfg := Default()
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse plugin config: %w", err)
		}
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = strings.TrimSpace(os.Getenv("CPA_MANAGER_PLUS_DATA_DIR"))
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = defaultDataDirectory
	}
	resolved, err := filepath.Abs(filepath.Clean(cfg.DataDir))
	if err != nil {
		return Config{}, fmt.Errorf("resolve data_dir: %w", err)
	}
	cfg.DataDir = resolved
	if cfg.QueueCapacity < 1 || cfg.QueueCapacity > 65_536 {
		return Config{}, fmt.Errorf("queue_capacity must be between 1 and 65536")
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > 1_024 {
		return Config{}, fmt.Errorf("batch_size must be between 1 and 1024")
	}
	switch cfg.Codex.ScheduleMode {
	case "", "interval":
		cfg.Codex.ScheduleMode = "interval"
	case "time_points":
	default:
		return Config{}, fmt.Errorf("unsupported codex_inspection.schedule_mode")
	}
	if cfg.Codex.IntervalMinutes < 1 || cfg.Codex.IntervalMinutes > 24*60 {
		return Config{}, fmt.Errorf("codex_inspection.interval_minutes must be between 1 and 1440")
	}
	switch cfg.Codex.AutoActionMode {
	case "", "none":
		cfg.Codex.AutoActionMode = "none"
	case "enable", "disable", "delete":
	default:
		return Config{}, fmt.Errorf("unsupported codex_inspection.auto_action_mode")
	}
	return cfg, nil
}
