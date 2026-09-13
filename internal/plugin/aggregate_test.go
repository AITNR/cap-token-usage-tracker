package plugin

import (
	"encoding/json"
	"math"
	"strings"
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
	stats := buildStatsForRange(data, start, end, usageRange{Name: "custom", Start: start, End: end}, "", end)
	if stats.Range != "custom" || stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 6 || len(stats.Series) != 2 {
		t.Fatalf("custom range stats = %+v", stats)
	}
}

func TestBuildStatsForRangeFiltersSourceAndRetainsSourceOptions(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", Source: "cli"}}: {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Model: "beta", Source: "web"}}:  {Requests: 1, TotalTokens: 10},
	}

	stats := buildStatsForRange(data, now.Add(-time.Hour), now, usageRange{Name: "retention"}, "cli", now)
	if stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 20 || len(stats.Groups) != 1 || stats.Groups[0].Source != "cli" {
		t.Fatalf("source-filtered stats = %+v", stats)
	}
	if len(stats.Sources) != 2 || stats.Sources[0] != "cli" || stats.Sources[1] != "web" {
		t.Fatalf("source options = %+v", stats.Sources)
	}
}

func TestBuildStatsForRangeSeparatesAndFiltersSources(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", Source: "Codex-user@example.com"}}:       {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", Source: "Antigravity-user@example.com"}}: {Requests: 1, TotalTokens: 10},
	}

	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, usageRange{Name: "retention"}, newUsageFilter("Codex-user@example.com", ""), now, nil)
	if stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 20 || len(stats.Groups) != 1 || stats.Groups[0].Source != "Codex-user@example.com" {
		t.Fatalf("source-filtered stats = %+v", stats)
	}
}

func TestBuildStatsMergesFailureDimensionsIntoSingleGroup(t *testing.T) {
	now := time.Date(2026, 9, 8, 12, 30, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	base := Dimensions{Provider: "p", Model: "m"}
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: base}: {Requests: 2, InputTokens: 8, OutputTokens: 4, TotalTokens: 12},
		{Hour: hour, Dimensions: func() Dimensions { d := base; d.Failed, d.FailureStatus = true, 500; return d }()}: {Requests: 1, FailedRequests: 1},
		{Hour: hour, Dimensions: func() Dimensions { d := base; d.Failed, d.FailureStatus = true, 429; return d }()}: {Requests: 1, FailedRequests: 1},
		{Hour: hour, Dimensions: func() Dimensions { d := base; d.Failed = true; return d }()}:                       {Requests: 1, FailedRequests: 1},
	}
	queryRange := usageRange{Name: "retention"}

	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, queryRange, usageFilter{}, now, nil)
	if len(stats.Groups) != 1 {
		t.Fatalf("stats group count = %d, groups = %+v", len(stats.Groups), stats.Groups)
	}
	group := stats.Groups[0]
	if group.Requests != 5 || group.FailedRequests != 3 || group.Failed || group.FailureStatus != 0 ||
		group.InputTokens != 8 || group.OutputTokens != 4 || group.TotalTokens != 12 {
		t.Fatalf("merged stats group = %+v", group)
	}

	page := buildGroupsForRange(data, queryRange, usageFilter{}, now, nil)
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("groups page = %+v", page)
	}
	item := page.Items[0]
	if item.Requests != 5 || item.FailedRequests != 3 || item.Failed || item.FailureStatus != 0 || item.TotalTokens != 12 {
		t.Fatalf("merged groups page item = %+v", item)
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

func TestCompactStatsDownsamplesAndPreservesCounters(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	model := Dimensions{Model: "compact-model", Source: "cli"}
	data := map[aggregateKey]Counters{
		{Hour: now.Add(-4*time.Minute - 30*time.Second).Unix(), Dimensions: model}: {Requests: 2, TotalTokens: 20, TotalLatencyNS: uint64(4 * time.Second), LatencySamples: 2, TotalTTFTNS: uint64(time.Second), TTFTSamples: 1},
		{Hour: now.Add(-2*time.Minute - 30*time.Second).Unix(), Dimensions: model}: {Requests: 3, TotalTokens: 30, TotalLatencyNS: uint64(9 * time.Second), LatencySamples: 3, TotalTTFTNS: uint64(2 * time.Second), TTFTSamples: 2},
	}
	queryRange := usageRange{Name: "24h", Start: now.Add(-24 * time.Hour)}
	initial := buildInitialStatsForRange(data, now.Add(-30*24*time.Hour), now, queryRange, usageFilter{}, now, nil)
	if initial.SchemaVersion != 2 || initial.BucketSeconds != 300 || len(initial.Series) != 1 || len(initial.Models) != 1 {
		t.Fatalf("compact initial shape = %+v", initial)
	}
	if initial.Summary.Requests != 5 || initial.Summary.TotalTokens != 50 || initial.Models[0].AverageLatencyNS != uint64(2600*time.Millisecond) || initial.Models[0].AverageTTFTNS != uint64(time.Second) {
		t.Fatalf("compact initial counters = %+v", initial)
	}
	trend := buildStatsTrendForRange(data, now.Add(-30*24*time.Hour), queryRange, usageFilter{}, now)
	if trend.SchemaVersion != 2 || trend.BucketSeconds != 300 || len(trend.ModelSeries) != 1 || trend.ModelSeries[0].Requests != 5 || trend.ModelSeries[0].TotalTTFTNS != uint64(3*time.Second) || trend.ModelSeries[0].TTFTSamples != 3 {
		t.Fatalf("compact trend = %+v", trend)
	}
	sevenDays := usageRange{Name: "7d", Start: now.Add(-7 * 24 * time.Hour)}
	if got := compactBucketSeconds(sevenDays, now.Add(-30*24*time.Hour), now); got != uint64(time.Hour/time.Second) {
		t.Fatalf("7d bucket seconds = %d", got)
	}
}

func TestInitialStatsJSONExcludesDetails(t *testing.T) {
	response := InitialStatsResponse{SchemaVersion: 2, Models: []ModelStats{{Model: "m"}}, Series: []SeriesPoint{}}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"groups"`, `"model_series"`, `"provider"`, `"api_key"`} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("initial response contains %s: %s", forbidden, body)
		}
	}
}

func TestUsageFilterUnionsRepeatedAPIKeyRefs(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	hashA := strings.Repeat("a", 32)
	hashB := strings.Repeat("b", 32)
	hashC := strings.Repeat("c", 32)
	refA := apiKeyRef(1, hashA)
	refB := apiKeyRef(1, hashB)
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", Source: "cli", APIKeyHash: hashA, APIKeyGeneration: 1}}: {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Model: "beta", Source: "cli", APIKeyHash: hashB, APIKeyGeneration: 1}}:  {Requests: 3, TotalTokens: 30},
		{Hour: hour, Dimensions: Dimensions{Model: "gamma", Source: "web", APIKeyHash: hashA, APIKeyGeneration: 1}}: {Requests: 5, TotalTokens: 50},
		{Hour: hour, Dimensions: Dimensions{Model: "delta", Source: "cli", APIKeyHash: hashC, APIKeyGeneration: 1}}: {Requests: 7, TotalTokens: 70},
	}

	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, usageRange{Name: "retention"}, newUsageFilterFromIdentities("cli", []string{refA, "", refB, refA}), now, nil)
	if stats.Summary.Requests != 5 || stats.Summary.TotalTokens != 50 || len(stats.Groups) != 2 {
		t.Fatalf("union-filtered stats = %+v", stats)
	}
}

func TestUsageFilterANDsModelWithAPIKeyRefUnion(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	hashA := strings.Repeat("a", 32)
	hashB := strings.Repeat("b", 32)
	filter := newUsageFilterFromIdentities("", []string{apiKeyRef(1, hashA), apiKeyRef(1, hashB)})
	filter.Model = "alpha"
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", APIKeyHash: hashA, APIKeyGeneration: 1}}: {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Model: "beta", APIKeyHash: hashB, APIKeyGeneration: 1}}:  {Requests: 3, TotalTokens: 30},
		{Hour: hour, Dimensions: Dimensions{Model: "alpha", APIKeyHash: hashB, APIKeyGeneration: 1}}: {Requests: 5, TotalTokens: 50},
	}
	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, usageRange{Name: "retention"}, filter, now, nil)
	if stats.Summary.Requests != 7 || stats.Summary.TotalTokens != 70 || len(stats.Groups) != 2 {
		t.Fatalf("model-and-union stats = %+v", stats)
	}
}

func TestUntitledModelAggregatesStayBackendNeutral(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Provider: "p"}}:                 {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Provider: "p", Model: "named"}}: {Requests: 1, TotalTokens: 10},
	}
	queryRange := usageRange{Name: "retention"}

	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, queryRange, usageFilter{}, now, nil)
	if len(stats.ModelSeries) != 2 || stats.ModelSeries[0].Model != "" || stats.ModelSeries[1].Model != "named" {
		t.Fatalf("model series = %+v", stats.ModelSeries)
	}
	initial := buildInitialStatsForRange(data, now.Add(-time.Hour), now, queryRange, usageFilter{}, now, nil)
	if len(initial.Models) != 2 || initial.Models[0].Model != "" || initial.Models[1].Model != "named" {
		t.Fatalf("initial models = %+v", initial.Models)
	}
	trend := buildStatsTrendForRange(data, now.Add(-time.Hour), queryRange, usageFilter{}, now)
	if len(trend.ModelSeries) != 2 || trend.ModelSeries[0].Model != "" || trend.ModelSeries[1].Model != "named" {
		t.Fatalf("trend models = %+v", trend.ModelSeries)
	}
	for name, payload := range map[string]any{"stats": stats, "initial": initial, "trend": trend} {
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), legacyUntitledModelName) {
			t.Fatalf("%s response leaked backend placeholder: %s", name, body)
		}
	}
}

func TestLegacyUntitledModelFilterCompatibility(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	data := map[aggregateKey]Counters{
		{Hour: hour, Dimensions: Dimensions{Model: ""}}:      {Requests: 2, TotalTokens: 20},
		{Hour: hour, Dimensions: Dimensions{Model: "named"}}: {Requests: 1, TotalTokens: 10},
	}
	filter := usageFilter{Model: legacyUntitledModelName}
	stats := buildStatsForRangeWithFilter(data, now.Add(-time.Hour), now, usageRange{Name: "retention"}, filter, now, nil)
	if stats.Summary.Requests != 2 || stats.Summary.TotalTokens != 20 || len(stats.Groups) != 1 || stats.Groups[0].Model != "" {
		t.Fatalf("legacy unnamed-model filter stats = %+v", stats)
	}
	if !modelFilterMatches(legacyUntitledModelName, "") || modelFilterMatches(legacyUntitledModelName, "named") {
		t.Fatal("legacy unnamed-model filter compatibility is incorrect")
	}
}
