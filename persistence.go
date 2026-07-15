package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

const (
	sqliteTimestampLayout = "2006-01-02T15:04:05.000000000Z07:00"
	storeOperationTimeout = 5 * time.Second
	modelPricesSettingKey = "model_prices"
)

const recordSelectColumns = `id, timestamp, api_key, provider, model, alias, source, auth_id, auth_index, auth_type, executor_type,
       reasoning_effort, service_tier, latency_ms, ttft_ms,
       input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens,
       failed, failure_status_code, failure_body`

type Store struct {
	db *sql.DB

	configMu sync.RWMutex
	config   Config

	cleanupMu   sync.Mutex
	lastCleanup time.Time

	closeOnce sync.Once
	closeErr  error

	processed atomic.Uint64
	persisted atomic.Uint64
}

func openStore(config Config) (*Store, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := prepareSQLitePath(normalized.DataPath); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", normalized.DataPath)
	if err != nil {
		return nil, fmt.Errorf("open usage sqlite database: %w", err)
	}
	// The reference implementation serializes access through one SQLite
	// connection. This also keeps writes deterministic for usage.handle.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db, config: normalized}
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	if err := store.initSchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	store.maybeCleanup(time.Now().UTC())
	return store, nil
}

func prepareSQLitePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("usage sqlite path is empty")
	}
	dir := filepath.Clean(filepath.Dir(path))
	if dir != "." && filepath.Dir(dir) != dir {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create usage sqlite directory: %w", err)
		}
	}

	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		file, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Errorf("inspect usage sqlite database: %w", openErr)
		}
		header := make([]byte, 16)
		_, readErr := io.ReadFull(file, header)
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("close inspected usage database: %w", closeErr)
		}
		if readErr != nil || string(header) != "SQLite format 3\x00" {
			return fmt.Errorf("database %q is not SQLite; choose a new data_dir/data_path (legacy bbolt files are not overwritten)", path)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect usage sqlite path: %w", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("create usage sqlite database: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close created usage sqlite database: %w", err)
	}
	return nil
}

func (s *Store) initSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("usage sqlite store is nil")
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS usage_records (
	id TEXT PRIMARY KEY,
	timestamp TEXT NOT NULL,
	api_key TEXT NOT NULL DEFAULT '',
	provider TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	alias TEXT NOT NULL DEFAULT '',
	source TEXT NOT NULL DEFAULT '',
	auth_id TEXT NOT NULL DEFAULT '',
	auth_index TEXT NOT NULL DEFAULT '',
	auth_type TEXT NOT NULL DEFAULT '',
	executor_type TEXT NOT NULL DEFAULT '',
	reasoning_effort TEXT NOT NULL DEFAULT '',
	service_tier TEXT NOT NULL DEFAULT '',
	latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
	ttft_ms INTEGER NOT NULL DEFAULT 0 CHECK (ttft_ms >= 0),
	input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
	output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
	reasoning_tokens INTEGER NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
	cached_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cached_tokens >= 0),
	cache_read_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_read_tokens >= 0),
	cache_creation_tokens INTEGER NOT NULL DEFAULT 0 CHECK (cache_creation_tokens >= 0),
	total_tokens INTEGER NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
	failed INTEGER NOT NULL DEFAULT 0 CHECK (failed IN (0, 1)),
	failure_status_code INTEGER NOT NULL DEFAULT 0 CHECK (failure_status_code >= 0),
	failure_body TEXT NOT NULL DEFAULT ''
)`); err != nil {
		return fmt.Errorf("initialize usage sqlite schema: %w", err)
	}
	if err := s.migrateSchema(ctx); err != nil {
		return err
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_usage_records_timestamp ON usage_records(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_records_api_model ON usage_records(api_key, provider, model)`,
		`CREATE TABLE IF NOT EXISTS plugin_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize usage sqlite indexes/settings: %w", err)
		}
	}
	return nil
}

// migrateSchema mirrors the reference repository's schema self-healing: any
// columns absent from an older SQLite usage_records table are added in place.
func (s *Store) migrateSchema(ctx context.Context) error {
	existing, err := s.existingColumns(ctx)
	if err != nil {
		return err
	}
	additions := []struct {
		name string
		ddl  string
	}{
		{"api_key", `ALTER TABLE usage_records ADD COLUMN api_key TEXT NOT NULL DEFAULT ''`},
		{"provider", `ALTER TABLE usage_records ADD COLUMN provider TEXT NOT NULL DEFAULT ''`},
		{"model", `ALTER TABLE usage_records ADD COLUMN model TEXT NOT NULL DEFAULT ''`},
		{"alias", `ALTER TABLE usage_records ADD COLUMN alias TEXT NOT NULL DEFAULT ''`},
		{"source", `ALTER TABLE usage_records ADD COLUMN source TEXT NOT NULL DEFAULT ''`},
		{"auth_id", `ALTER TABLE usage_records ADD COLUMN auth_id TEXT NOT NULL DEFAULT ''`},
		{"auth_index", `ALTER TABLE usage_records ADD COLUMN auth_index TEXT NOT NULL DEFAULT ''`},
		{"auth_type", `ALTER TABLE usage_records ADD COLUMN auth_type TEXT NOT NULL DEFAULT ''`},
		{"executor_type", `ALTER TABLE usage_records ADD COLUMN executor_type TEXT NOT NULL DEFAULT ''`},
		{"reasoning_effort", `ALTER TABLE usage_records ADD COLUMN reasoning_effort TEXT NOT NULL DEFAULT ''`},
		{"service_tier", `ALTER TABLE usage_records ADD COLUMN service_tier TEXT NOT NULL DEFAULT ''`},
		{"latency_ms", `ALTER TABLE usage_records ADD COLUMN latency_ms INTEGER NOT NULL DEFAULT 0`},
		{"ttft_ms", `ALTER TABLE usage_records ADD COLUMN ttft_ms INTEGER NOT NULL DEFAULT 0`},
		{"input_tokens", `ALTER TABLE usage_records ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0`},
		{"output_tokens", `ALTER TABLE usage_records ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0`},
		{"reasoning_tokens", `ALTER TABLE usage_records ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0`},
		{"cached_tokens", `ALTER TABLE usage_records ADD COLUMN cached_tokens INTEGER NOT NULL DEFAULT 0`},
		{"cache_read_tokens", `ALTER TABLE usage_records ADD COLUMN cache_read_tokens INTEGER NOT NULL DEFAULT 0`},
		{"cache_creation_tokens", `ALTER TABLE usage_records ADD COLUMN cache_creation_tokens INTEGER NOT NULL DEFAULT 0`},
		{"total_tokens", `ALTER TABLE usage_records ADD COLUMN total_tokens INTEGER NOT NULL DEFAULT 0`},
		{"failed", `ALTER TABLE usage_records ADD COLUMN failed INTEGER NOT NULL DEFAULT 0`},
		{"failure_status_code", `ALTER TABLE usage_records ADD COLUMN failure_status_code INTEGER NOT NULL DEFAULT 0`},
		{"failure_body", `ALTER TABLE usage_records ADD COLUMN failure_body TEXT NOT NULL DEFAULT ''`},
	}
	for _, addition := range additions {
		if _, ok := existing[addition.name]; ok {
			continue
		}
		if _, err := s.db.ExecContext(ctx, addition.ddl); err != nil {
			return fmt.Errorf("migrate usage sqlite add %s: %w", addition.name, err)
		}
	}
	return nil
}

func (s *Store) existingColumns(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info(usage_records)")
	if err != nil {
		return nil, fmt.Errorf("read usage sqlite schema: %w", err)
	}
	defer rows.Close()
	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notNull    int
			defaultV   sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultV, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan usage sqlite schema: %w", err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate usage sqlite schema: %w", err)
	}
	return columns, nil
}

// Record synchronously inserts one request. usage.handle does not return until
// this method has completed, matching the reference plugin's durability model.
func (s *Store) Record(record Record) error {
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	err := s.Insert(ctx, record)
	s.processed.Add(1)
	if err != nil {
		return err
	}
	s.persisted.Add(1)
	s.maybeCleanup(time.Now().UTC())
	return nil
}

func (s *Store) Insert(ctx context.Context, record Record) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("usage sqlite store is nil")
	}
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Errorf("usage record id is empty")
	}
	tokens := nonNegativeTokenStats(record.Tokens)
	tokens.TotalTokens = normalizeTotalTokens(tokens)
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage_records (
	id, timestamp, api_key, provider, model, alias, source, auth_id, auth_index, auth_type, executor_type,
	reasoning_effort, service_tier, latency_ms, ttft_ms,
	input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens,
	failed, failure_status_code, failure_body
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(record.ID),
		formatRecordTimestamp(record.Timestamp),
		strings.TrimSpace(record.APIKey),
		strings.TrimSpace(record.Provider),
		normalizeModel(record.Model),
		strings.TrimSpace(record.Alias),
		strings.TrimSpace(record.Source),
		strings.TrimSpace(record.AuthID),
		strings.TrimSpace(record.AuthIndex),
		strings.TrimSpace(record.AuthType),
		strings.TrimSpace(record.ExecutorType),
		strings.TrimSpace(record.ReasoningEffort),
		strings.TrimSpace(record.ServiceTier),
		nonNegativeInt64(record.LatencyMs),
		nonNegativeInt64(record.TTFTMs),
		tokens.InputTokens,
		tokens.OutputTokens,
		tokens.ReasoningTokens,
		tokens.CachedTokens,
		tokens.CacheReadTokens,
		tokens.CacheCreationTokens,
		tokens.TotalTokens,
		boolToInt(record.Failed),
		nonNegativeInt(record.FailureStatusCode),
		strings.TrimSpace(record.FailureBody),
	)
	if err != nil {
		return fmt.Errorf("insert usage sqlite record: %w", err)
	}
	return nil
}

func (s *Store) Query(rangeName string) (StatsResponse, error) {
	now := time.Now().UTC()
	canonicalRange, cutoff, err := queryCutoff(rangeName, now)
	if err != nil {
		return StatsResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()

	query := `SELECT ` + recordSelectColumns + ` FROM usage_records`
	args := []any{}
	if !cutoff.IsZero() {
		query += ` WHERE timestamp >= ?`
		args = append(args, formatTimestamp(cutoff))
	}
	query += ` ORDER BY timestamp ASC, id ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return StatsResponse{}, fmt.Errorf("query usage statistics rows: %w", err)
	}
	defer rows.Close()

	data := make(map[aggregateKey]Counters)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return StatsResponse{}, err
		}
		key := aggregateKey{
			Hour:       record.Timestamp.UTC().Truncate(time.Minute).Unix(),
			Dimensions: dimensionsForRecord(record),
		}
		counters := data[key]
		counters.add(countersForRecord(record))
		data[key] = counters
	}
	if err := rows.Err(); err != nil {
		return StatsResponse{}, fmt.Errorf("iterate usage statistics rows: %w", err)
	}

	since, lastUsed, err := s.usageBounds(ctx, now)
	if err != nil {
		return StatsResponse{}, err
	}
	return buildStats(data, since, lastUsed, canonicalRange, now)
}

func (s *Store) QueryRequests(rangeName string, offset, limit int, model string) (RequestPage, error) {
	now := time.Now().UTC()
	canonicalRange, cutoff, err := queryCutoff(rangeName, now)
	if err != nil {
		return RequestPage{}, err
	}
	if offset < 0 {
		return RequestPage{}, withStatus(400, "offset must not be negative")
	}
	if limit == 0 {
		limit = defaultRequestPageSize
	}
	if limit < 1 || limit > maxRequestPageSize {
		return RequestPage{}, withStatus(400, "limit must be between 1 and %d", maxRequestPageSize)
	}

	where, args := requestWhere(cutoff, model)
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_records`+where, args...).Scan(&total); err != nil {
		return RequestPage{}, fmt.Errorf("count usage request rows: %w", err)
	}
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordSelectColumns+` FROM usage_records`+where+` ORDER BY timestamp DESC, id DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return RequestPage{}, fmt.Errorf("query usage request rows: %w", err)
	}
	defer rows.Close()

	page := RequestPage{
		GeneratedAt: now,
		Range:       canonicalRange,
		Total:       total,
		Offset:      offset,
		Limit:       limit,
		Items:       make([]RequestDetail, 0, limit),
	}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return RequestPage{}, err
		}
		page.Items = append(page.Items, requestDetailForRecord(record))
	}
	if err := rows.Err(); err != nil {
		return RequestPage{}, fmt.Errorf("iterate usage request rows: %w", err)
	}
	return page, nil
}

func requestWhere(cutoff time.Time, model string) (string, []any) {
	conditions := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if !cutoff.IsZero() {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, formatTimestamp(cutoff))
	}
	if model = strings.TrimSpace(model); model != "" {
		conditions = append(conditions, "model = ?")
		args = append(args, model)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

// QueryUsage is the protected, reference-compatible raw usage query.
func (s *Store) QueryUsage(ctx context.Context, rng QueryRange) (APIUsage, error) {
	if s == nil || s.db == nil {
		return APIUsage{}, nil
	}
	query := `SELECT ` + recordSelectColumns + ` FROM usage_records`
	args := make([]any, 0, 2)
	conditions := make([]string, 0, 2)
	if rng.Start != nil && !rng.Start.IsZero() {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, formatTimestamp(*rng.Start))
	}
	if rng.End != nil && !rng.End.IsZero() {
		conditions = append(conditions, "timestamp < ?")
		args = append(args, formatTimestamp(*rng.End))
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp ASC, id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query raw usage records: %w", err)
	}
	defer rows.Close()
	result := APIUsage{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		key := groupingKey(record.APIKey, record.Provider)
		model := normalizeModel(record.Model)
		if result[key] == nil {
			result[key] = map[string][]UsageRequestDetail{}
		}
		result[key][model] = append(result[key][model], usageRequestDetail(record))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate raw usage records: %w", err)
	}
	return result, nil
}

func usageRequestDetail(record Record) UsageRequestDetail {
	tokens := nonNegativeTokenStats(record.Tokens)
	tokens.TotalTokens = normalizeTotalTokens(tokens)
	return UsageRequestDetail{
		ID:                record.ID,
		Timestamp:         record.Timestamp.UTC(),
		Provider:          record.Provider,
		Model:             record.Model,
		Alias:             record.Alias,
		Source:            record.Source,
		AuthID:            record.AuthID,
		AuthIndex:         record.AuthIndex,
		AuthType:          record.AuthType,
		ExecutorType:      record.ExecutorType,
		ReasoningEffort:   record.ReasoningEffort,
		ServiceTier:       record.ServiceTier,
		LatencyMs:         nonNegativeInt64(record.LatencyMs),
		TTFTMs:            nonNegativeInt64(record.TTFTMs),
		Tokens:            tokens,
		Failed:            record.Failed,
		FailureStatusCode: nonNegativeInt(record.FailureStatusCode),
		FailureBody:       record.FailureBody,
	}
}

func (s *Store) Delete(ctx context.Context, ids []string) (DeleteResult, error) {
	result := DeleteResult{Missing: []string{}}
	if s == nil || s.db == nil {
		result.Missing = append(result.Missing, ids...)
		return result, nil
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		res, err := s.db.ExecContext(ctx, "DELETE FROM usage_records WHERE id = ?", id)
		if err != nil {
			return result, fmt.Errorf("delete usage record %s: %w", id, err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return result, fmt.Errorf("read deleted usage row count: %w", err)
		}
		if rows == 0 {
			result.Missing = append(result.Missing, id)
			continue
		}
		result.Deleted += rows
	}
	return result, nil
}

func (s *Store) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM usage_records WHERE timestamp < ?", formatTimestamp(cutoff))
	if err != nil {
		return 0, fmt.Errorf("delete expired usage records: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read expired usage row count: %w", err)
	}
	return rows, nil
}

func (s *Store) QueryModelPrices() (map[string]ModelPrice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	var encoded string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM plugin_settings WHERE key = ?", modelPricesSettingKey).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]ModelPrice{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query model prices: %w", err)
	}
	var prices map[string]ModelPrice
	if err := json.Unmarshal([]byte(encoded), &prices); err != nil {
		return nil, fmt.Errorf("decode model prices: %w", err)
	}
	normalized, err := normalizeModelPrices(prices)
	if err != nil {
		return nil, fmt.Errorf("validate stored model prices: %w", err)
	}
	return normalized, nil
}

func (s *Store) SaveModelPrices(prices map[string]ModelPrice) (map[string]ModelPrice, error) {
	normalized, err := normalizeModelPrices(prices)
	if err != nil {
		return nil, withStatus(400, "%v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	if len(normalized) == 0 {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM plugin_settings WHERE key = ?", modelPricesSettingKey); err != nil {
			return nil, fmt.Errorf("delete model prices: %w", err)
		}
		return map[string]ModelPrice{}, nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode model prices: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO plugin_settings (key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, modelPricesSettingKey, string(encoded)); err != nil {
		return nil, fmt.Errorf("save model prices: %w", err)
	}
	return cloneModelPrices(normalized), nil
}

func (s *Store) Reset() error {
	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	if _, err := s.db.ExecContext(ctx, "DELETE FROM usage_records"); err != nil {
		return fmt.Errorf("reset usage records: %w", err)
	}
	return nil
}

func (s *Store) Reconfigure(config Config) error {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return err
	}
	s.configMu.Lock()
	if normalized.DataPath != s.config.DataPath {
		s.configMu.Unlock()
		return errors.New("database path changes require opening a new store")
	}
	s.config = normalized
	s.configMu.Unlock()

	s.cleanupMu.Lock()
	s.lastCleanup = time.Time{}
	s.cleanupMu.Unlock()
	s.maybeCleanup(time.Now().UTC())
	return nil
}

func (s *Store) Diagnostics() UsageDiagnostics {
	if s == nil {
		return UsageDiagnostics{}
	}
	return UsageDiagnostics{
		Processed:          s.processed.Load(),
		PersistedSinceOpen: s.persisted.Load(),
	}
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	s.closeOnce.Do(func() { s.closeErr = s.db.Close() })
	return s.closeErr
}

func (s *Store) maybeCleanup(now time.Time) {
	s.configMu.RLock()
	days := s.config.RetentionDays
	s.configMu.RUnlock()
	if days <= 0 {
		return
	}

	s.cleanupMu.Lock()
	if !s.lastCleanup.IsZero() && now.Sub(s.lastCleanup) < time.Hour {
		s.cleanupMu.Unlock()
		return
	}
	s.lastCleanup = now
	s.cleanupMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), storeOperationTimeout)
	defer cancel()
	_, _ = s.DeleteBefore(ctx, now.Add(-time.Duration(days)*24*time.Hour))
}

func (s *Store) usageBounds(ctx context.Context, now time.Time) (time.Time, time.Time, error) {
	var minimum sql.NullString
	var maximum sql.NullString
	if err := s.db.QueryRowContext(ctx, "SELECT MIN(timestamp), MAX(timestamp) FROM usage_records").Scan(&minimum, &maximum); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("query usage time bounds: %w", err)
	}
	var since time.Time
	var lastUsed time.Time
	var err error
	if minimum.Valid && strings.TrimSpace(minimum.String) != "" {
		since, err = time.Parse(time.RFC3339Nano, minimum.String)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse first usage timestamp: %w", err)
		}
		since = since.UTC()
	} else {
		s.configMu.RLock()
		days := s.config.RetentionDays
		s.configMu.RUnlock()
		if days > 0 {
			since = now.Add(-time.Duration(days) * 24 * time.Hour)
		} else {
			since = now
		}
	}
	if maximum.Valid && strings.TrimSpace(maximum.String) != "" {
		lastUsed, err = time.Parse(time.RFC3339Nano, maximum.String)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("parse last usage timestamp: %w", err)
		}
		lastUsed = lastUsed.UTC()
	}
	return since, lastUsed, nil
}

func scanRecord(scanner interface{ Scan(dest ...any) error }) (Record, error) {
	var (
		record        Record
		timestampText string
		failedInt     int
	)
	if err := scanner.Scan(
		&record.ID,
		&timestampText,
		&record.APIKey,
		&record.Provider,
		&record.Model,
		&record.Alias,
		&record.Source,
		&record.AuthID,
		&record.AuthIndex,
		&record.AuthType,
		&record.ExecutorType,
		&record.ReasoningEffort,
		&record.ServiceTier,
		&record.LatencyMs,
		&record.TTFTMs,
		&record.Tokens.InputTokens,
		&record.Tokens.OutputTokens,
		&record.Tokens.ReasoningTokens,
		&record.Tokens.CachedTokens,
		&record.Tokens.CacheReadTokens,
		&record.Tokens.CacheCreationTokens,
		&record.Tokens.TotalTokens,
		&failedInt,
		&record.FailureStatusCode,
		&record.FailureBody,
	); err != nil {
		return Record{}, fmt.Errorf("scan usage sqlite record: %w", err)
	}
	timestamp, err := time.Parse(time.RFC3339Nano, timestampText)
	if err != nil {
		return Record{}, fmt.Errorf("parse usage sqlite timestamp: %w", err)
	}
	record.Timestamp = timestamp.UTC()
	record.LatencyMs = nonNegativeInt64(record.LatencyMs)
	record.TTFTMs = nonNegativeInt64(record.TTFTMs)
	record.Tokens = nonNegativeTokenStats(record.Tokens)
	record.Tokens.TotalTokens = normalizeTotalTokens(record.Tokens)
	record.Failed = failedInt != 0
	record.FailureStatusCode = nonNegativeInt(record.FailureStatusCode)
	record.Model = normalizeModel(record.Model)
	return record, nil
}

func formatTimestamp(timestamp time.Time) string {
	return timestamp.UTC().Format(sqliteTimestampLayout)
}

func formatRecordTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		timestamp = time.Now()
	}
	return formatTimestamp(timestamp)
}
