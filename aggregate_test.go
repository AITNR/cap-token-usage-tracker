package main

import (
	"math"
	"testing"
	"time"
)

func TestBuildStatsGroupsRangesAndAverages(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 30, 0, 0, time.UTC)
	dim := Dimensions{Provider: "p", Model: "m"}
	data := map[aggregateKey]Counters{
		{Hour: now.Add(-time.Hour).Truncate(time.Hour).Unix(), Dimensions: dim}: {
			Requests: 2, InputTokens: 10, OutputTokens: 5, TotalTokens: 15,
			TotalLatencyNS: uint64(3 * time.Second), LatencySamples: 2,
		},
		{Hour: now.Add(-48 * time.Hour).Truncate(time.Hour).Unix(), Dimensions: dim}: {
			Requests: 1, TotalTokens: 99,
		},
	}
	stats, err := buildStats(data, now.Add(-7*24*time.Hour), now, "24h", now)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 15 || len(stats.Groups) != 1 || len(stats.Series) != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.Groups[0].AverageLatencyNS != uint64(1500*time.Millisecond) {
		t.Fatalf("average latency = %d", stats.Groups[0].AverageLatencyNS)
	}
}

func TestBuildStatsIncludesModelSeries(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 30, 0, 0, time.UTC)
	hour := now.Add(-time.Hour).Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Provider: "openai", Model: "gpt-test", Alias: "primary"}}: {
			Requests: 2, InputTokens: 120, OutputTokens: 30, TotalTokens: 150,
			TotalLatencyNS: uint64(4 * time.Second), LatencySamples: 2,
		},
		{Hour: hour, Dimensions: Dimensions{Provider: "openai", Model: "gpt-test", Alias: "backup"}}: {
			Requests: 1, InputTokens: 50, OutputTokens: 20, TotalTokens: 70,
		},
	}

	stats, err := buildStats(data, now.Add(-24*time.Hour), now, "24h", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.ModelSeries) != 1 {
		t.Fatalf("model series length = %d, want 1: %+v", len(stats.ModelSeries), stats.ModelSeries)
	}
	point := stats.ModelSeries[0]
	if point.Model != "gpt-test" || point.Requests != 3 || point.InputTokens != 170 || point.OutputTokens != 50 || point.TotalTokens != 220 {
		t.Fatalf("unexpected model point: %+v", point)
	}
}

func TestSaturatingAdd(t *testing.T) {
	if got := saturatingAdd(math.MaxUint64-2, 5); got != math.MaxUint64 {
		t.Fatalf("saturatingAdd = %d", got)
	}
}

func TestQueryCutoffRejectsUnknownRange(t *testing.T) {
	if _, _, err := queryCutoff("year", time.Now()); err == nil || errorHTTPStatus(err) != 400 {
		t.Fatalf("expected status 400, got %v", err)
	}
}

func TestUsageRangeFromQueryParsesCustomOffsets(t *testing.T) {
	queryRange, err := usageRangeFromQuery("custom", "2026-08-20T00:00:00+08:00", "2026-09-06T00:00:00+08:00", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if queryRange.Name != "custom" || !queryRange.Start.Equal(time.Date(2026, 8, 19, 16, 0, 0, 0, time.UTC)) || !queryRange.End.Equal(time.Date(2026, 9, 5, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected custom range: %+v", queryRange)
	}
	for _, values := range [][3]string{
		{"custom", "2026-08-20T00:00:00+08:00", ""},
		{"24h", "2026-08-20T00:00:00+08:00", "2026-08-21T00:00:00+08:00"},
		{"custom", "2026-08-21T00:00:00+08:00", "2026-08-20T00:00:00+08:00"},
		{"custom", "not-a-time", "2026-08-21T00:00:00+08:00"},
	} {
		if _, err := usageRangeFromQuery(values[0], values[1], values[2], time.Now()); err == nil || errorHTTPStatus(err) != 400 {
			t.Fatalf("invalid range accepted: %q %q %q, %v", values[0], values[1], values[2], err)
		}
	}
}

func TestBuildStatsForRangeUsesExclusiveEnd(t *testing.T) {
	start := time.Date(2026, 8, 19, 16, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 5, 16, 0, 0, 0, time.UTC)
	dim := Dimensions{Model: "m"}
	data := map[aggregateKey]Counters{
		{Hour: start.Add(-time.Minute).Unix(), Dimensions: dim}: {Requests: 1, TotalTokens: 1},
		{Hour: start.Unix(), Dimensions: dim}:                   {Requests: 1, TotalTokens: 2},
		{Hour: end.Add(-time.Minute).Unix(), Dimensions: dim}:   {Requests: 1, TotalTokens: 4},
		{Hour: end.Unix(), Dimensions: dim}:                     {Requests: 1, TotalTokens: 8},
	}
	stats := buildStatsForRange(data, start, end, usageRange{Name: "custom", Start: start, End: end}, end)
	if stats.Range != "custom" || stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 6 || len(stats.Series) != 2 {
		t.Fatalf("custom range stats = %+v", stats)
	}
}

func TestBuildStatsStableDimensionOrdering(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 30, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	dimensions := []Dimensions{
		{Provider: "p", Model: "m", FailureStatus: 500},
		{Provider: "p", Model: "m", Failed: true},
		{Provider: "p", Model: "m", ReasoningEffort: "high"},
		{Provider: "p", Model: "m", ServiceTier: "priority"},
		{Provider: "p", Model: "m", AuthType: "oauth"},
		{Provider: "p", ExecutorType: "x", Model: "m"},
	}
	data := make(map[aggregateKey]Counters, len(dimensions))
	for _, dimension := range dimensions {
		data[aggregateKey{Hour: hour, Dimensions: dimension}] = Counters{Requests: 1, TotalTokens: 10}
	}
	stats, err := buildStats(data, now, now, "retention", now)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(stats.Groups); index++ {
		if compareDimensions(stats.Groups[index-1].Dimensions, stats.Groups[index].Dimensions) >= 0 {
			t.Fatalf("groups are not strictly ordered at %d: %+v", index, stats.Groups)
		}
	}
}

func TestQueryCutoffUsesUTCPartialBoundary(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 30, 0, 0, time.FixedZone("offset", 8*60*60))
	_, cutoff, err := queryCutoff("24h", now)
	if err != nil {
		t.Fatal(err)
	}
	expected := now.UTC().Add(-24 * time.Hour).Truncate(time.Minute)
	if !cutoff.Equal(expected) {
		t.Fatalf("cutoff = %v, expected %v", cutoff, expected)
	}
}
