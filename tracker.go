package main

import (
	"sort"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// ModelKey uniquely identifies a model+provider combination.
type ModelKey struct {
	Model    string
	Provider string
}

// ModelStats holds aggregated token usage for one model+provider pair.
type ModelStats struct {
	Model               string        `json:"model"`
	Provider            string        `json:"provider"`
	Requests            int64         `json:"requests"`
	FailedRequests      int64         `json:"failed_requests"`
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	ReasoningTokens     int64         `json:"reasoning_tokens"`
	CachedTokens        int64         `json:"cached_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	TotalTokens         int64         `json:"total_tokens"`
	TotalLatency        time.Duration `json:"total_latency_ns"`
	TotalTTFT           time.Duration `json:"total_ttft_ns"`
}

// AvgLatency returns the average request latency.
func (s ModelStats) AvgLatency() time.Duration {
	if s.Requests == 0 {
		return 0
	}
	return s.TotalLatency / time.Duration(s.Requests)
}

// AvgTTFT returns the average time to first token.
func (s ModelStats) AvgTTFT() time.Duration {
	if s.Requests == 0 {
		return 0
	}
	return s.TotalTTFT / time.Duration(s.Requests)
}

// Summary is the aggregate across all models.
type Summary struct {
	Requests            int64         `json:"requests"`
	FailedRequests      int64         `json:"failed_requests"`
	InputTokens         int64         `json:"input_tokens"`
	OutputTokens        int64         `json:"output_tokens"`
	ReasoningTokens     int64         `json:"reasoning_tokens"`
	CachedTokens        int64         `json:"cached_tokens"`
	CacheReadTokens     int64         `json:"cache_read_tokens"`
	CacheCreationTokens int64         `json:"cache_creation_tokens"`
	TotalTokens         int64         `json:"total_tokens"`
	TotalLatency        time.Duration `json:"total_latency_ns"`
}

// StatsResponse is the JSON response for the /stats API.
type StatsResponse struct {
	Summary  Summary      `json:"summary"`
	Models   []ModelStats `json:"models"`
	Since    time.Time    `json:"since"`
	LastUsed time.Time    `json:"last_used"`
}

// Tracker is a thread-safe usage data store.
type Tracker struct {
	mu       sync.Mutex
	models   map[ModelKey]*ModelStats
	since    time.Time
	lastUsed time.Time
}

// NewTracker creates a new Tracker.
func NewTracker() *Tracker {
	return &Tracker{
		models: make(map[ModelKey]*ModelStats),
		since:  time.Now().UTC(),
	}
}

// RecordUsage ingests a usage record and updates aggregated counters.
func (t *Tracker) RecordUsage(record pluginapi.UsageRecord) {
	key := ModelKey{
		Model:    record.Model,
		Provider: record.Provider,
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	stats, ok := t.models[key]
	if !ok {
		stats = &ModelStats{
			Model:    record.Model,
			Provider: record.Provider,
		}
		t.models[key] = stats
	}

	stats.Requests++
	if record.Failed {
		stats.FailedRequests++
	}
	stats.InputTokens += record.Detail.InputTokens
	stats.OutputTokens += record.Detail.OutputTokens
	stats.ReasoningTokens += record.Detail.ReasoningTokens
	stats.CachedTokens += record.Detail.CachedTokens
	stats.CacheReadTokens += record.Detail.CacheReadTokens
	stats.CacheCreationTokens += record.Detail.CacheCreationTokens
	stats.TotalTokens += record.Detail.TotalTokens
	stats.TotalLatency += record.Latency
	stats.TotalTTFT += record.TTFT

	now := record.RequestedAt.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if t.lastUsed.IsZero() || now.After(t.lastUsed) {
		t.lastUsed = now
	}
}

// GetStats returns a snapshot of all usage statistics.
func (t *Tracker) GetStats() StatsResponse {
	t.mu.Lock()
	defer t.mu.Unlock()

	models := make([]ModelStats, 0, len(t.models))
	for _, s := range t.models {
		models = append(models, *s)
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].TotalTokens != models[j].TotalTokens {
			return models[i].TotalTokens > models[j].TotalTokens
		}
		return models[i].Model < models[j].Model
	})

	summary := Summary{}
	for _, s := range models {
		summary.Requests += s.Requests
		summary.FailedRequests += s.FailedRequests
		summary.InputTokens += s.InputTokens
		summary.OutputTokens += s.OutputTokens
		summary.ReasoningTokens += s.ReasoningTokens
		summary.CachedTokens += s.CachedTokens
		summary.CacheReadTokens += s.CacheReadTokens
		summary.CacheCreationTokens += s.CacheCreationTokens
		summary.TotalTokens += s.TotalTokens
		summary.TotalLatency += s.TotalLatency
	}

	return StatsResponse{
		Summary:  summary,
		Models:   models,
		Since:    t.since,
		LastUsed: t.lastUsed,
	}
}

// Reset clears all collected usage data.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.models = make(map[ModelKey]*ModelStats)
	t.since = time.Now().UTC()
	t.lastUsed = time.Time{}
}
