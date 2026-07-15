package main

import (
	"fmt"
	"math"
	"time"
)

const (
	defaultRequestPageSize = 100
	maxRequestPageSize     = 500
)

// RequestDetail is the sanitized dashboard view of one persisted request. The
// protected /usage API exposes the complete reference-compatible record,
// including credential identifiers and failure body.
type RequestDetail struct {
	ID        string    `json:"id"`
	Time      time.Time `json:"time"`
	Timestamp time.Time `json:"timestamp"`
	Dimensions
	Counters
	Result       string  `json:"result"`
	LatencyNS    uint64  `json:"latency_ns"`
	TTFTNS       uint64  `json:"ttft_ns"`
	GenerationNS uint64  `json:"generation_ns"`
	TPS          float64 `json:"tps"`
	CacheHit     bool    `json:"cache_hit"`
}

type RequestPage struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Range       string          `json:"range"`
	Total       int             `json:"total"`
	Offset      int             `json:"offset"`
	Limit       int             `json:"limit"`
	Items       []RequestDetail `json:"items"`
}

func requestDetailForRecord(record Record) RequestDetail {
	latencyNS := millisecondsToNanoseconds(record.LatencyMs)
	ttftNS := millisecondsToNanoseconds(record.TTFTMs)
	generationNS := latencyNS
	if ttftNS > 0 && latencyNS >= ttftNS {
		generationNS = latencyNS - ttftNS
	}
	var tps float64
	if generationNS > 0 {
		tps = float64(nonNegativeInt64(record.Tokens.OutputTokens)) / (float64(generationNS) / float64(time.Second))
	}
	result := "成功"
	if record.Failed {
		result = "失败"
		if record.FailureStatusCode > 0 {
			result = fmt.Sprintf("失败 (HTTP %d)", record.FailureStatusCode)
		}
	}
	counters := countersForRecord(record)
	return RequestDetail{
		ID:           record.ID,
		Time:         record.Timestamp.UTC(),
		Timestamp:    record.Timestamp.UTC(),
		Dimensions:   dimensionsForRecord(record),
		Counters:     counters,
		Result:       result,
		LatencyNS:    latencyNS,
		TTFTNS:       ttftNS,
		GenerationNS: generationNS,
		TPS:          tps,
		CacheHit:     record.Tokens.CacheReadTokens > 0,
	}
}

func millisecondsToNanoseconds(milliseconds int64) uint64 {
	if milliseconds <= 0 {
		return 0
	}
	if milliseconds > int64(math.MaxUint64/uint64(time.Millisecond)) {
		return math.MaxUint64
	}
	return uint64(milliseconds) * uint64(time.Millisecond)
}
