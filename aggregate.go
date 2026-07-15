package main

import (
	"cmp"
	"math"
	"sort"
	"time"
)

type Dimensions struct {
	Provider        string `json:"provider"`
	ExecutorType    string `json:"executor_type"`
	Model           string `json:"model"`
	Alias           string `json:"alias"`
	Source          string `json:"source"`
	AuthType        string `json:"auth_type"`
	ServiceTier     string `json:"service_tier"`
	ReasoningEffort string `json:"reasoning_effort"`
	Failed          bool   `json:"failed"`
	FailureStatus   int    `json:"failure_status"`
}

type Counters struct {
	Requests            uint64 `json:"requests"`
	FailedRequests      uint64 `json:"failed_requests"`
	InputTokens         uint64 `json:"input_tokens"`
	OutputTokens        uint64 `json:"output_tokens"`
	ReasoningTokens     uint64 `json:"reasoning_tokens"`
	CachedTokens        uint64 `json:"cached_tokens"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
	TotalTokens         uint64 `json:"total_tokens"`
	TotalLatencyNS      uint64 `json:"total_latency_ns"`
	TotalTTFTNS         uint64 `json:"total_ttft_ns"`
	LatencySamples      uint64 `json:"latency_samples"`
	TTFTSamples         uint64 `json:"ttft_samples"`
}

func (c *Counters) add(other Counters) {
	c.Requests = saturatingAdd(c.Requests, other.Requests)
	c.FailedRequests = saturatingAdd(c.FailedRequests, other.FailedRequests)
	c.InputTokens = saturatingAdd(c.InputTokens, other.InputTokens)
	c.OutputTokens = saturatingAdd(c.OutputTokens, other.OutputTokens)
	c.ReasoningTokens = saturatingAdd(c.ReasoningTokens, other.ReasoningTokens)
	c.CachedTokens = saturatingAdd(c.CachedTokens, other.CachedTokens)
	c.CacheReadTokens = saturatingAdd(c.CacheReadTokens, other.CacheReadTokens)
	c.CacheCreationTokens = saturatingAdd(c.CacheCreationTokens, other.CacheCreationTokens)
	c.TotalTokens = saturatingAdd(c.TotalTokens, other.TotalTokens)
	c.TotalLatencyNS = saturatingAdd(c.TotalLatencyNS, other.TotalLatencyNS)
	c.TotalTTFTNS = saturatingAdd(c.TotalTTFTNS, other.TotalTTFTNS)
	c.LatencySamples = saturatingAdd(c.LatencySamples, other.LatencySamples)
	c.TTFTSamples = saturatingAdd(c.TTFTSamples, other.TTFTSamples)
}

func (c Counters) averageLatencyNS() uint64 {
	if c.LatencySamples == 0 {
		return 0
	}
	return c.TotalLatencyNS / c.LatencySamples
}

func (c Counters) averageTTFTNS() uint64 {
	if c.TTFTSamples == 0 {
		return 0
	}
	return c.TotalTTFTNS / c.TTFTSamples
}

func dimensionsForRecord(record Record) Dimensions {
	return Dimensions{
		Provider:        record.Provider,
		ExecutorType:    record.ExecutorType,
		Model:           record.Model,
		Alias:           record.Alias,
		Source:          record.Source,
		AuthType:        record.AuthType,
		ServiceTier:     record.ServiceTier,
		ReasoningEffort: record.ReasoningEffort,
		Failed:          record.Failed,
		FailureStatus:   nonNegativeInt(record.FailureStatusCode),
	}
}

func countersForRecord(record Record) Counters {
	tokens := nonNegativeTokenStats(record.Tokens)
	tokens.TotalTokens = normalizeTotalTokens(tokens)
	result := Counters{
		Requests:            1,
		FailedRequests:      boolCount(record.Failed),
		InputTokens:         positiveTokenUint(tokens.InputTokens),
		OutputTokens:        positiveTokenUint(tokens.OutputTokens),
		ReasoningTokens:     positiveTokenUint(tokens.ReasoningTokens),
		CachedTokens:        positiveTokenUint(tokens.CachedTokens),
		CacheReadTokens:     positiveTokenUint(tokens.CacheReadTokens),
		CacheCreationTokens: positiveTokenUint(tokens.CacheCreationTokens),
		TotalTokens:         positiveTokenUint(tokens.TotalTokens),
	}
	if record.LatencyMs > 0 {
		result.TotalLatencyNS = millisecondsToNanoseconds(record.LatencyMs)
		result.LatencySamples = 1
	}
	if record.TTFTMs > 0 {
		result.TotalTTFTNS = millisecondsToNanoseconds(record.TTFTMs)
		result.TTFTSamples = 1
	}
	return result
}

func positiveTokenUint(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func boolCount(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}

func saturatingAdd(left, right uint64) uint64 {
	if math.MaxUint64-left < right {
		return math.MaxUint64
	}
	return left + right
}

type aggregateKey struct {
	Hour       int64
	Dimensions Dimensions
}

type GroupStats struct {
	Dimensions
	Counters
	AverageLatencyNS uint64 `json:"average_latency_ns"`
	AverageTTFTNS    uint64 `json:"average_ttft_ns"`
}

type SeriesPoint struct {
	Hour string `json:"hour"`
	Counters
}

// ModelSeriesPoint preserves the time-bucketed model split needed by the dashboard for
// stacked trends, model drill-down, and cost calculations without retaining
// individual prompt contents.
type ModelSeriesPoint struct {
	Hour  string `json:"hour"`
	Model string `json:"model"`
	Counters
}

type UsageDiagnostics struct {
	CallbacksReceived  uint64 `json:"callbacks_received"`
	Decoded            uint64 `json:"decoded"`
	Enqueued           uint64 `json:"enqueued"`
	Processed          uint64 `json:"processed"`
	PersistedSinceOpen uint64 `json:"persisted_since_open"`
	MailboxDepth       int    `json:"mailbox_depth"`
	PendingFlush       int64  `json:"pending_flush"`
	DecodeErrors       uint64 `json:"decode_errors"`
	EnqueueErrors      uint64 `json:"enqueue_errors"`
}

type StatsResponse struct {
	SchemaVersion uint32             `json:"schema_version"`
	GeneratedAt   time.Time          `json:"generated_at"`
	Range         string             `json:"range"`
	RetainedSince time.Time          `json:"retained_since"`
	LastUsed      time.Time          `json:"last_used"`
	Summary       Counters           `json:"summary"`
	Groups        []GroupStats       `json:"groups"`
	Series        []SeriesPoint      `json:"series"`
	ModelSeries   []ModelSeriesPoint `json:"model_series"`
	Diagnostics   UsageDiagnostics   `json:"diagnostics"`
}

func buildStats(data map[aggregateKey]Counters, since, lastUsed time.Time, requestedRange string, now time.Time) (StatsResponse, error) {
	rangeName, cutoff, err := queryCutoff(requestedRange, now)
	if err != nil {
		return StatsResponse{}, err
	}

	groups := make(map[Dimensions]Counters)
	series := make(map[int64]Counters)
	modelSeries := make(map[struct {
		Hour  int64
		Model string
	}]Counters)
	summary := Counters{}
	for key, counters := range data {
		if !cutoff.IsZero() && key.Hour < cutoff.Unix() {
			continue
		}
		group := groups[key.Dimensions]
		group.add(counters)
		groups[key.Dimensions] = group

		point := series[key.Hour]
		point.add(counters)
		series[key.Hour] = point

		model := key.Dimensions.Model
		if model == "" {
			model = "未标记模型"
		}
		modelKey := struct {
			Hour  int64
			Model string
		}{Hour: key.Hour, Model: model}
		modelPoint := modelSeries[modelKey]
		modelPoint.add(counters)
		modelSeries[modelKey] = modelPoint

		summary.add(counters)
	}

	groupRows := make([]GroupStats, 0, len(groups))
	for dimensions, counters := range groups {
		groupRows = append(groupRows, GroupStats{
			Dimensions:       dimensions,
			Counters:         counters,
			AverageLatencyNS: counters.averageLatencyNS(),
			AverageTTFTNS:    counters.averageTTFTNS(),
		})
	}
	sort.Slice(groupRows, func(i, j int) bool {
		if groupRows[i].TotalTokens != groupRows[j].TotalTokens {
			return groupRows[i].TotalTokens > groupRows[j].TotalTokens
		}
		return compareDimensions(groupRows[i].Dimensions, groupRows[j].Dimensions) < 0
	})

	hours := make([]int64, 0, len(series))
	for hour := range series {
		hours = append(hours, hour)
	}
	sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })
	points := make([]SeriesPoint, 0, len(hours))
	for _, hour := range hours {
		points = append(points, SeriesPoint{
			Hour:     time.Unix(hour, 0).UTC().Format(time.RFC3339),
			Counters: series[hour],
		})
	}

	modelKeys := make([]struct {
		Hour  int64
		Model string
	}, 0, len(modelSeries))
	for key := range modelSeries {
		modelKeys = append(modelKeys, key)
	}
	sort.Slice(modelKeys, func(i, j int) bool {
		if modelKeys[i].Hour != modelKeys[j].Hour {
			return modelKeys[i].Hour < modelKeys[j].Hour
		}
		return modelKeys[i].Model < modelKeys[j].Model
	})
	modelPoints := make([]ModelSeriesPoint, 0, len(modelKeys))
	for _, key := range modelKeys {
		modelPoints = append(modelPoints, ModelSeriesPoint{
			Hour:     time.Unix(key.Hour, 0).UTC().Format(time.RFC3339),
			Model:    key.Model,
			Counters: modelSeries[key],
		})
	}

	return StatsResponse{
		SchemaVersion: 1,
		GeneratedAt:   now.UTC(),
		Range:         rangeName,
		RetainedSince: since.UTC(),
		LastUsed:      lastUsed.UTC(),
		Summary:       summary,
		Groups:        groupRows,
		Series:        points,
		ModelSeries:   modelPoints,
	}, nil
}

func queryCutoff(value string, now time.Time) (string, time.Time, error) {
	now = now.UTC()
	switch value {
	case "", "24h":
		return "24h", now.Add(-24 * time.Hour).Truncate(time.Minute), nil
	case "7d":
		return "7d", now.Add(-7 * 24 * time.Hour).Truncate(time.Minute), nil
	case "30d":
		return "30d", now.Add(-30 * 24 * time.Hour).Truncate(time.Minute), nil
	case "retention":
		return "retention", time.Time{}, nil
	default:
		return "", time.Time{}, withStatus(400, "unsupported range %q", value)
	}
}

func compareDimensions(left, right Dimensions) int {
	for _, comparison := range []int{
		cmp.Compare(left.Provider, right.Provider),
		cmp.Compare(left.ExecutorType, right.ExecutorType),
		cmp.Compare(left.Model, right.Model),
		cmp.Compare(left.Alias, right.Alias),
		cmp.Compare(left.Source, right.Source),
		cmp.Compare(left.AuthType, right.AuthType),
		cmp.Compare(left.ServiceTier, right.ServiceTier),
		cmp.Compare(left.ReasoningEffort, right.ReasoningEffort),
		cmp.Compare(boolInt(left.Failed), boolInt(right.Failed)),
		cmp.Compare(left.FailureStatus, right.FailureStatus),
	} {
		if comparison != 0 {
			return comparison
		}
	}
	return 0
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
