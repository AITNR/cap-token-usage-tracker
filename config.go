package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultDatabaseFile  = "usage.db"
	defaultRetentionDays = 0
	defaultPluginDirName = "cap-token-usage-tracker"
)

type Config struct {
	DataPath      string
	RetentionDays int
}

// parseConfig follows the reference plugin's configuration model: data_dir
// selects the directory containing usage.db and retention_days=0 disables
// automatic cleanup. data_path remains accepted as a compatibility alias for
// deployments of older versions of this plugin.
func parseConfig(raw []byte) (Config, error) {
	values := map[string]any{}
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &values); err != nil {
			return Config{}, fmt.Errorf("parse config YAML: %w", err)
		}
	}

	dataDir := strings.TrimSpace(configString(values, "data_dir", "数据目录"))
	dataPath := strings.TrimSpace(configString(values, "data_path"))
	if dataDir != "" && dataPath != "" {
		return Config{}, fmt.Errorf("data_dir and data_path cannot be configured together")
	}

	retentionDays := defaultRetentionDays
	if value, ok := lookupConfig(values, "retention_days", "用量保留天数"); ok {
		parsed, err := configInt(value)
		if err != nil {
			return Config{}, fmt.Errorf("parse retention_days: %w", err)
		}
		retentionDays = parsed
	}

	if dataPath == "" {
		if dataDir == "" {
			dataDir = strings.TrimSpace(os.Getenv("CAP_TOKEN_USAGE_TRACKER_DIR"))
		}
		if dataDir == "" {
			// Keep the reference plugin's environment variable as a fallback for
			// users moving an existing usage-statistics deployment to this plugin.
			dataDir = strings.TrimSpace(os.Getenv("USAGE_STATISTICS_DIR"))
		}
		if dataDir == "" {
			home, err := os.UserHomeDir()
			if err == nil && strings.TrimSpace(home) != "" {
				dataDir = filepath.Join(home, ".cli-proxy-api", "plugins", defaultPluginDirName)
			} else {
				dataDir = filepath.Join(".cli-proxy-api", "plugins", defaultPluginDirName)
			}
		}
		dataPath = filepath.Join(dataDir, defaultDatabaseFile)
	}

	return normalizeConfig(Config{DataPath: dataPath, RetentionDays: retentionDays})
}

func normalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.DataPath) == "" {
		return Config{}, fmt.Errorf("database path must not be empty")
	}
	if cfg.RetentionDays < 0 || cfg.RetentionDays > 3650 {
		return Config{}, fmt.Errorf("retention_days must be between 0 and 3650")
	}
	absolute, err := filepath.Abs(filepath.Clean(cfg.DataPath))
	if err != nil {
		return Config{}, fmt.Errorf("resolve database path: %w", err)
	}
	cfg.DataPath = absolute
	return cfg, nil
}

func lookupConfig(values map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func configString(values map[string]any, keys ...string) string {
	value, ok := lookupConfig(values, keys...)
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func configInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case uint64:
		if typed > uint64(^uint(0)>>1) {
			return 0, fmt.Errorf("value is too large")
		}
		return int(typed), nil
	case float64:
		if typed != float64(int(typed)) {
			return 0, fmt.Errorf("value must be an integer")
		}
		return int(typed), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, fmt.Errorf("value must be an integer")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("value must be an integer")
	}
}
