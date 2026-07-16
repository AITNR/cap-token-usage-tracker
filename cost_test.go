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

func TestEstimateRequestCostMissingPrice(t *testing.T) {
	cost := estimateRequestCost(RequestDetail{Dimensions: Dimensions{Model: "missing"}}, nil)
	if cost.Priced || cost.TotalUSD != 0 {
		t.Fatalf("missing price cost = %+v", cost)
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
