package main

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEstimateRequestCostAnthropicUsesAllFourComponents(t *testing.T) {
	request := RequestDetail{
		Dimensions: Dimensions{Provider: "anthropic", ExecutorType: "claude", Model: "m"},
		Counters: Counters{
			InputTokens:         100_000,
			OutputTokens:        10_000,
			CachedTokens:        99_000,
			CacheReadTokens:     20_000,
			CacheCreationTokens: 5_000,
		},
	}
	prices := map[string]ModelPrice{
		"m": {Input: 2, Output: 10, CacheRead: 0.2, CacheCreation: 2.5, Source: priceSourceManual},
	}
	cost := estimateRequestCost(request, prices)
	if !cost.Priced || cost.AccountingMode != accountingModeInputExcludesCache {
		t.Fatalf("cost metadata = %+v", cost)
	}
	if cost.BillableInputTokens != 100_000 || cost.BilledCacheReadTokens != 20_000 || cost.ContextTokens != 125_000 {
		t.Fatalf("cost token quantities = %+v", cost)
	}
	want := 0.2 + 0.1 + 0.004 + 0.0125
	if math.Abs(cost.TotalUSD-want) > 1e-12 {
		t.Fatalf("total cost = %.12f, want %.12f", cost.TotalUSD, want)
	}
}

func TestEstimateRequestCostInputIncludesCacheAndCachedFallback(t *testing.T) {
	request := RequestDetail{
		Dimensions: Dimensions{Provider: "openai", Model: "m"},
		Counters: Counters{
			InputTokens:         100_000,
			OutputTokens:        5_000,
			CachedTokens:        20_000,
			CacheCreationTokens: 5_000,
		},
	}
	prices := map[string]ModelPrice{
		"m": {Input: 1, Output: 2, CacheRead: 0.1, CacheCreation: 1},
	}
	cost := estimateRequestCost(request, prices)
	if cost.BillableInputTokens != 75_000 || cost.BilledCacheReadTokens != 20_000 || cost.ContextTokens != 100_000 {
		t.Fatalf("cost token quantities = %+v", cost)
	}
	want := 0.075 + 0.01 + 0.002 + 0.005
	if math.Abs(cost.TotalUSD-want) > 1e-12 {
		t.Fatalf("total cost = %.12f, want %.12f", cost.TotalUSD, want)
	}
}

func TestEstimateRequestCostSelectsHighestStrictContextTier(t *testing.T) {
	prices := map[string]ModelPrice{
		"m": {
			Input: 1,
			ContextTiers: []ContextPriceTier{
				{Threshold: 32_000, Input: 2},
				{Threshold: 200_000, Input: 3},
			},
		},
	}
	atBoundary := estimateRequestCost(RequestDetail{Dimensions: Dimensions{Provider: "anthropic", Model: "m"}, Counters: Counters{InputTokens: 200_000}}, prices)
	if atBoundary.TierThreshold != 32_000 || atBoundary.InputUSD != 0.4 {
		t.Fatalf("boundary cost = %+v", atBoundary)
	}
	overBoundary := estimateRequestCost(RequestDetail{Dimensions: Dimensions{Provider: "anthropic", Model: "m"}, Counters: Counters{InputTokens: 200_001}}, prices)
	if overBoundary.TierThreshold != 200_000 || math.Abs(overBoundary.InputUSD-0.600003) > 1e-12 {
		t.Fatalf("over-boundary cost = %+v", overBoundary)
	}
}

func TestEstimateRequestCostSelectsServiceTierSchedule(t *testing.T) {
	prices := map[string]ModelPrice{
		"m": {
			Input: 1, Output: 2, CacheRead: 0.1,
			ServiceTiers: map[string]ServiceTierPrice{
				"priority": {
					Input: 10, Output: 20, CacheRead: 1,
					ContextTiers: []ContextPriceTier{
						{Threshold: 200_000, Input: 20, Output: 30, CacheRead: 2},
					},
				},
			},
		},
	}
	priority := estimateRequestCost(RequestDetail{
		Dimensions: Dimensions{Provider: "openai", Model: "m", ServiceTier: "Priority"},
		Counters:   Counters{InputTokens: 300_000, OutputTokens: 10_000, CacheReadTokens: 200_000},
	}, prices)
	wantPriority := float64(100_000)*20/1_000_000 + float64(10_000)*30/1_000_000 + float64(200_000)*2/1_000_000
	if priority.PriceServiceTier != "priority" || priority.TierThreshold != 200_000 || math.Abs(priority.TotalUSD-wantPriority) > 1e-12 {
		t.Fatalf("priority cost = %+v, want %.12f", priority, wantPriority)
	}

	standard := estimateRequestCost(RequestDetail{
		Dimensions: Dimensions{Provider: "openai", Model: "m", ServiceTier: "auto"},
		Counters:   Counters{InputTokens: 100_000, OutputTokens: 10_000},
	}, prices)
	wantStandard := float64(100_000)*1/1_000_000 + float64(10_000)*2/1_000_000
	if standard.PriceServiceTier != "" || math.Abs(standard.TotalUSD-wantStandard) > 1e-12 {
		t.Fatalf("standard cost = %+v, want %.12f", standard, wantStandard)
	}
}

func TestEstimateRequestCostMissingPrice(t *testing.T) {
	cost := estimateRequestCost(RequestDetail{Dimensions: Dimensions{Model: "missing"}}, nil)
	if cost.Priced || cost.TotalUSD != 0 {
		t.Fatalf("missing price cost = %+v", cost)
	}
}

func TestEstimateRequestCostUsesIgnoredSuffixFallback(t *testing.T) {
	resolver := newModelPriceResolver(map[string]ModelPrice{
		"gemini-3.1-pro-preview-gg": {Input: 2, Output: 12, CacheRead: 0.2},
	}, PriceSyncSettings{IgnoredSuffixes: []string{"-gg"}})
	request := RequestDetail{
		Dimensions: Dimensions{Provider: "google", Model: "gemini-3.1-pro-preview"},
		Counters:   Counters{InputTokens: 1_000, OutputTokens: 100},
	}
	cost := estimateRequestCostWithResolver(request, resolver)
	if !cost.Priced {
		t.Fatalf("normalized suffix price was not resolved: %+v", cost)
	}
	want := 1_000.0*2/1_000_000 + 100.0*12/1_000_000
	if math.Abs(cost.TotalUSD-want) > 1e-12 {
		t.Fatalf("normalized suffix cost = %.12f, want %.12f", cost.TotalUSD, want)
	}
}

func TestEstimateRequestCostExactPriceWinsOverNormalizedCandidate(t *testing.T) {
	resolver := newModelPriceResolver(map[string]ModelPrice{
		"model":    {Input: 1},
		"model-gg": {Input: 2},
	}, PriceSyncSettings{IgnoredSuffixes: []string{"-gg"}})
	cost := estimateRequestCostWithResolver(RequestDetail{
		Dimensions: Dimensions{Model: "model"},
		Counters:   Counters{InputTokens: 1_000},
	}, resolver)
	if !cost.Priced || math.Abs(cost.InputUSD-0.001) > 1e-12 {
		t.Fatalf("exact price did not win: %+v", cost)
	}
}

func TestEstimateRequestCostRejectsAmbiguousNormalizedPrices(t *testing.T) {
	resolver := newModelPriceResolver(map[string]ModelPrice{
		"model-gg":      {Input: 1},
		"model-preview": {Input: 2},
	}, PriceSyncSettings{IgnoredSuffixes: []string{"-gg", "-preview", "-local"}})
	cost := estimateRequestCostWithResolver(RequestDetail{
		Dimensions: Dimensions{Model: "model-local"},
		Counters:   Counters{InputTokens: 1_000},
	}, resolver)
	if cost.Priced || cost.TotalUSD != 0 {
		t.Fatalf("ambiguous normalized price was selected: %+v", cost)
	}
}

func TestEstimateRequestCostAllowsEquivalentNormalizedPrices(t *testing.T) {
	resolver := newModelPriceResolver(map[string]ModelPrice{
		"model-gg":      {Input: 2, Output: 4},
		"model-preview": {Input: 2, Output: 4},
	}, PriceSyncSettings{IgnoredSuffixes: []string{"-gg", "-preview", "-local"}})
	cost := estimateRequestCostWithResolver(RequestDetail{
		Dimensions: Dimensions{Model: "model-local"},
		Counters:   Counters{InputTokens: 1_000, OutputTokens: 100},
	}, resolver)
	if !cost.Priced || math.Abs(cost.TotalUSD-0.0024) > 1e-12 {
		t.Fatalf("equivalent normalized prices were not resolved: %+v", cost)
	}
}

func TestEstimateRequestCostUsesExplicitMappingFallback(t *testing.T) {
	resolver := newModelPriceResolver(map[string]ModelPrice{
		"local-model": {Input: 3},
	}, PriceSyncSettings{Mappings: []PriceSyncMapping{{Source: "local-model", Target: "catalog-model"}}})
	cost := estimateRequestCostWithResolver(RequestDetail{
		Dimensions: Dimensions{Model: "catalog-model"},
		Counters:   Counters{InputTokens: 1_000},
	}, resolver)
	if !cost.Priced || math.Abs(cost.InputUSD-0.003) > 1e-12 {
		t.Fatalf("mapped price was not resolved: %+v", cost)
	}
}

func TestStoreCostsResolveSynchronizedIgnoredSuffix(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	settings := PriceSyncSettings{IgnoredSuffixes: []string{"-gg"}}
	if _, err := store.ApplyModelPriceSync(map[string]ModelPrice{
		"gemini-3.1-pro-preview-gg": {
			Input: 2, Output: 12, CacheRead: 0.2, Source: priceSourceModelsDev,
			CatalogProvider: "google", CatalogModel: "gemini-3.1-pro-preview",
		},
	}, settings, PriceSyncMetadata{Observed: 1, Matched: 1, CompletedAt: time.Now().UTC()}, 0); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	for index := 0; index < 16; index++ {
		if err := store.Record(normalizedUsage{
			Dimensions:  Dimensions{Provider: "google", Model: "gemini-3.1-pro-preview"},
			RequestedAt: now.Add(time.Duration(index) * time.Millisecond),
			Counters:    Counters{Requests: 1, InputTokens: 1_000, OutputTokens: 100, TotalTokens: 1_100},
		}); err != nil {
			t.Fatal(err)
		}
	}

	costs, err := store.QueryCosts("24h")
	if err != nil {
		t.Fatal(err)
	}
	if costs.Summary.Requests != 16 || costs.Summary.PricedRequests != 16 || costs.Summary.UnpricedRequests != 0 || len(costs.MissingPrices) != 0 {
		t.Fatalf("normalized cost coverage = %+v, missing=%+v", costs.Summary, costs.MissingPrices)
	}
	want := 16 * (1_000.0*2/1_000_000 + 100.0*12/1_000_000)
	if math.Abs(costs.Summary.TotalUSD-want) > 1e-12 {
		t.Fatalf("normalized total cost = %.12f, want %.12f", costs.Summary.TotalUSD, want)
	}

	page, err := store.QueryRequests("24h", 0, 100, "gemini-3.1-pro-preview")
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 16 || len(page.Items) != 16 {
		t.Fatalf("normalized request page = %+v", page)
	}
	for _, item := range page.Items {
		if item.EstimatedCost == nil || !item.EstimatedCost.Priced {
			t.Fatalf("request was not priced through normalized lookup: %+v", item)
		}
	}
}

func TestStoreQueryCostsAndRequestPageUseCurrentPriceBook(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SaveModelPrices(map[string]ModelPrice{
		"m": {
			Input: 2, Output: 10, CacheRead: 0.2, CacheCreation: 2.5,
			ContextTiers: []ContextPriceTier{{Threshold: 100, Input: 4, Output: 20, CacheRead: 0.4, CacheCreation: 5}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.Record(normalizedUsage{
		Dimensions:  Dimensions{Provider: "anthropic", ExecutorType: "claude", Model: "m"},
		RequestedAt: now,
		Counters: Counters{
			Requests: 1, InputTokens: 100, OutputTokens: 10, CacheReadTokens: 20, CacheCreationTokens: 5, TotalTokens: 110,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{
		Dimensions:  Dimensions{Provider: "other", Model: "unpriced"},
		RequestedAt: now.Add(time.Second),
		Counters:    Counters{Requests: 1, InputTokens: 10, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}

	costs, err := store.QueryCosts("24h")
	if err != nil {
		t.Fatal(err)
	}
	if costs.Summary.Requests != 2 || costs.Summary.PricedRequests != 1 || costs.Summary.UnpricedRequests != 1 {
		t.Fatalf("cost summary = %+v", costs.Summary)
	}
	want := 100.0*4/1_000_000 + 10.0*20/1_000_000 + 20.0*0.4/1_000_000 + 5.0*5/1_000_000
	if math.Abs(costs.Summary.TotalUSD-want) > 1e-12 || len(costs.Series) != 2 || len(costs.MissingPrices) != 1 {
		t.Fatalf("cost response = %+v", costs)
	}
	page, err := store.QueryRequests("24h", 0, 10, "m")
	if err != nil {
		t.Fatal(err)
	}
	if page.PriceBookRevision == 0 || len(page.Items) != 1 || page.Items[0].EstimatedCost == nil || !page.Items[0].EstimatedCost.Priced || page.Items[0].EstimatedCost.TierThreshold != 100 {
		t.Fatalf("request page = %+v", page)
	}
}

func TestCustomRangeUsesSameExclusiveEndAcrossQueries(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SaveModelPrices(map[string]ModelPrice{"m": {}}); err != nil {
		t.Fatal(err)
	}

	start := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Minute)
	end := start.Add(time.Hour)
	for _, requestedAt := range []time.Time{
		start.Add(-time.Minute),
		start,
		end.Add(-time.Nanosecond),
		end,
	} {
		if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: requestedAt, Counters: Counters{Requests: 1, InputTokens: 10, TotalTokens: 10}}); err != nil {
			t.Fatal(err)
		}
	}

	queryRange := usageRange{Name: "custom", Start: start, End: end}
	stats, err := store.queryStats(queryRange)
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.queryRequestPage(queryRange, 0, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	costs, err := store.queryCosts(queryRange)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.Requests != 2 || page.Total != 2 || costs.Summary.Requests != 2 {
		t.Fatalf("custom range counts differ: stats=%d requests=%d costs=%d", stats.Summary.Requests, page.Total, costs.Summary.Requests)
	}
}

func TestCostCacheInvalidatesWhenSuffixSettingsChange(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prices := map[string]ModelPrice{"model-gg": {Input: 2}}
	before, err := store.SavePriceBook(prices, &PriceSyncSettings{IgnoredSuffixes: []string{"-unused"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{
		Dimensions: Dimensions{Model: "model"}, RequestedAt: time.Now().UTC(),
		Counters: Counters{Requests: 1, InputTokens: 1_000, TotalTokens: 1_000},
	}); err != nil {
		t.Fatal(err)
	}

	first, err := store.QueryCosts("24h")
	if err != nil {
		t.Fatal(err)
	}
	if first.Summary.PricedRequests != 0 || first.Summary.UnpricedRequests != 1 {
		t.Fatalf("cost before suffix setting = %+v", first.Summary)
	}

	after, err := store.SavePriceBook(prices, &PriceSyncSettings{IgnoredSuffixes: []string{"-gg"}})
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision <= before.Revision {
		t.Fatalf("price revision did not advance: before=%d after=%d", before.Revision, after.Revision)
	}
	second, err := store.QueryCosts("24h")
	if err != nil {
		t.Fatal(err)
	}
	if second.Summary.PricedRequests != 1 || second.Summary.UnpricedRequests != 0 || math.Abs(second.Summary.TotalUSD-0.002) > 1e-12 {
		t.Fatalf("cost after suffix setting = %+v", second.Summary)
	}
}

func TestCostQueryCacheInvalidatesOnRecordPriceAndReset(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var scans atomic.Int32
	store.costScanHook = func() { scans.Add(1) }
	if _, err := store.SaveModelPrices(map[string]ModelPrice{"m": {Input: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1, InputTokens: 10}}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := store.QueryCosts("24h"); err != nil {
			t.Fatal(err)
		}
	}
	if scans.Load() != 1 {
		t.Fatalf("cached scans = %d, want 1", scans.Load())
	}
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1, InputTokens: 10}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryCosts("24h"); err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 2 {
		t.Fatalf("record invalidation scans = %d, want 2", scans.Load())
	}
	if _, err := store.SaveModelPrices(map[string]ModelPrice{"m": {Input: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryCosts("24h"); err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 3 {
		t.Fatalf("price invalidation scans = %d, want 3", scans.Load())
	}
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryCosts("24h"); err != nil {
		t.Fatal(err)
	}
	if scans.Load() != 4 {
		t.Fatalf("reset invalidation scans = %d, want 4", scans.Load())
	}
}

func TestConcurrentCostQueriesCoalesce(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SaveModelPrices(map[string]ModelPrice{"m": {Input: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1, InputTokens: 10}}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var scans atomic.Int32
	store.costScanHook = func() {
		scans.Add(1)
		once.Do(func() { close(started) })
		<-release
	}
	const callers = 8
	errs := make(chan error, callers)
	for range callers {
		go func() {
			_, err := store.QueryCosts("24h")
			errs <- err
		}()
	}
	<-started
	close(release)
	for range callers {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if scans.Load() != 1 {
		t.Fatalf("coalesced scans = %d, want 1", scans.Load())
	}
}
