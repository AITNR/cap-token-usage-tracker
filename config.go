package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultDataPath        = "./data/token-usage-tracker.db"
	defaultRetentionDays   = 30
	defaultFlushInterval   = 5 * time.Second
	defaultFlushMaxRecords = 100
)

type Config struct {
	DataPath        string
	RetentionDays   int
	FlushInterval   time.Duration
	FlushMaxRecords int
	SyncOnRecord    bool
	Language        Language
}

type configYAML struct {
	DataPath        string `yaml:"data_path"`
	RetentionDays   *int   `yaml:"retention_days"`
	FlushInterval   string `yaml:"flush_interval"`
	FlushMaxRecords *int   `yaml:"flush_max_records"`
	SyncOnRecord    *bool  `yaml:"sync_on_record"`
	Language        string `yaml:"language"`
}

func defaultConfig() Config {
	return Config{
		DataPath:        defaultDataPath,
		RetentionDays:   defaultRetentionDays,
		FlushInterval:   defaultFlushInterval,
		FlushMaxRecords: defaultFlushMaxRecords,
		SyncOnRecord:    true,
		Language:        LanguageAuto,
	}
}

func parseConfig(raw []byte) (Config, error) {
	cfg := defaultConfig()
	if len(raw) == 0 {
		return normalizeConfig(cfg)
	}

	var input configYAML
	if err := yaml.Unmarshal(raw, &input); err != nil {
		return Config{}, fmt.Errorf("parse config YAML: %w", err)
	}
	if strings.TrimSpace(input.DataPath) != "" {
		cfg.DataPath = strings.TrimSpace(input.DataPath)
	}
	if input.RetentionDays != nil {
		cfg.RetentionDays = *input.RetentionDays
	}
	if strings.TrimSpace(input.FlushInterval) != "" {
		interval, err := time.ParseDuration(strings.TrimSpace(input.FlushInterval))
		if err != nil {
			return Config{}, fmt.Errorf("parse flush_interval: %w", err)
		}
		cfg.FlushInterval = interval
	}
	if input.FlushMaxRecords != nil {
		cfg.FlushMaxRecords = *input.FlushMaxRecords
	}
	if input.SyncOnRecord != nil {
		cfg.SyncOnRecord = *input.SyncOnRecord
	}
	if strings.TrimSpace(input.Language) != "" {
		cfg.Language = Language(strings.TrimSpace(input.Language))
	}
	return normalizeConfig(cfg)
}

func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.DataPath) == "" {
		return Config{}, fmt.Errorf("data_path must not be empty")
	}
	if cfg.RetentionDays < 1 || cfg.RetentionDays > 3650 {
		return Config{}, fmt.Errorf("retention_days must be between 1 and 3650")
	}
	if cfg.FlushInterval < time.Second || cfg.FlushInterval > time.Hour {
		return Config{}, fmt.Errorf("flush_interval must be between 1s and 1h")
	}
	if cfg.FlushMaxRecords < 1 || cfg.FlushMaxRecords > 1_000_000 {
		return Config{}, fmt.Errorf("flush_max_records must be between 1 and 1000000")
	}
	if strings.TrimSpace(string(cfg.Language)) == "" {
		cfg.Language = LanguageAuto
	} else {
		normalized := normalizeLanguage(string(cfg.Language))
		if normalized == "" {
			return Config{}, fmt.Errorf("language must be one of %s", strings.Join(supportedLanguageValues(true), ", "))
		}
		cfg.Language = normalized
	}
	absolute, err := filepath.Abs(filepath.Clean(cfg.DataPath))
	if err != nil {
		return Config{}, fmt.Errorf("resolve data_path: %w", err)
	}
	cfg.DataPath = absolute
	return cfg, nil
}
