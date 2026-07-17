package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMatchModelsDevPricesUsesPrioritySuffixAndContextTiers(t *testing.T) {
	openAICost := modelsDevCost{Input: 1, Output: 2, CacheRead: 0.1, CacheWrite: 1.25}
	anthropicCost := modelsDevCost{
		Input: 3, Output: 15, CacheRead: 0.3, CacheWrite: 3.75,
		Tiers: []modelsDevCostTier{
			{Input: 6, Output: 22.5, CacheRead: 0.6, CacheWrite: 7.5, Tier: modelsDevTierKind{Type: "context", Size: 200_000}},
			{Input: 99, Tier: modelsDevTierKind{Type: "latency", Size: 1}},
		},
	}
	catalog := map[string]modelsDevProvider{
		"anthropic": {ID: "anthropic", Models: map[string]modelsDevModel{"claude-test": {Cost: &anthropicCost}}},
		"openai":    {ID: "openai", Models: map[string]modelsDevModel{"claude-test": {Cost: &openAICost}}},
	}
	settings := PriceSyncSettings{
		ProviderPriority: []string{"anthropic", "openai"},
		IgnoredSuffixes:  []string{"-thinking"},
	}
	result, err := matchModelsDevPrices(catalog, []string{"Claude-Test-Thinking", "missing"}, settings, time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if result.Observed != 2 || result.Matched != 1 || result.Unmatched != 1 {
		t.Fatalf("match result = %+v", result)
	}
	price := result.Prices["Claude-Test-Thinking"]
	if price.CatalogProvider != "anthropic" || price.Input != 3 || price.CacheCreation != 3.75 {
		t.Fatalf("matched price = %+v", price)
	}
	if len(price.ContextTiers) != 1 || price.ContextTiers[0].Threshold != 200_000 || price.ContextTiers[0].CacheRead != 0.6 {
		t.Fatalf("matched tiers = %+v", price.ContextTiers)
	}
}

func TestNormalizeSyncModelsValidatesCLIProxyAPIList(t *testing.T) {
	models, err := normalizeSyncModels([]string{" model-b ", "model-a", "model-b", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "model-a" || models[1] != "model-b" {
		t.Fatalf("normalized models = %#v", models)
	}
	if _, err := normalizeSyncModels(nil); err == nil {
		t.Fatal("accepted empty CLIProxyAPI model list")
	}
	if _, err := normalizeSyncModels([]string{strings.Repeat("m", maxDimensionRunes+1)}); err == nil {
		t.Fatal("accepted overlong CLIProxyAPI model name")
	}
}

func TestMatchModelsDevPricesAppliesExplicitMapping(t *testing.T) {
	cost := modelsDevCost{Input: 2}
	catalog := map[string]modelsDevProvider{
		"provider": {Models: map[string]modelsDevModel{"catalog-model": {Cost: &cost}}},
	}
	settings := PriceSyncSettings{Mappings: []PriceSyncMapping{{Source: "local-model", Target: "catalog-model"}}}
	result, err := matchModelsDevPrices(catalog, []string{"local-model"}, settings, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result.Prices["local-model"].Input != 2 {
		t.Fatalf("mapped prices = %+v", result.Prices)
	}
}

func TestModelsDevFetcherValidatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"provider":{"models":{"model":{"cost":{"input":1,"output":2}}}}}`))
	}))
	defer server.Close()
	fetcher := &modelsDevFetcher{client: server.Client(), url: server.URL}
	catalog, err := fetcher.fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if catalog["provider"].Models["model"].Cost.Input != 1 {
		t.Fatalf("catalog = %+v", catalog)
	}

	badContent := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "text/plain")
		_, _ = response.Write([]byte(`{}`))
	}))
	defer badContent.Close()
	_, err = (&modelsDevFetcher{client: badContent.Client(), url: badContent.URL}).fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "application/json") {
		t.Fatalf("bad content type error = %v", err)
	}
}

func TestModelsDevFetcherRejectsInvalidResponses(t *testing.T) {
	for name, fixture := range map[string]struct {
		status      int
		contentType string
		body        string
		want        string
	}{
		"non-200":       {status: http.StatusBadGateway, contentType: "application/json", body: `{"secret":"must-not-leak"}`, want: "HTTP 502"},
		"malformed":     {status: http.StatusOK, contentType: "application/json", body: `{`, want: "decode models.dev catalog"},
		"trailing":      {status: http.StatusOK, contentType: "application/json", body: `{} {}`, want: "trailing JSON value"},
		"empty catalog": {status: http.StatusOK, contentType: "application/json", body: `{}`, want: "no providers"},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", fixture.contentType)
				response.WriteHeader(fixture.status)
				_, _ = response.Write([]byte(fixture.body))
			}))
			defer server.Close()
			_, err := (&modelsDevFetcher{client: server.Client(), url: server.URL}).fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error = %v, want %q", err, fixture.want)
			}
			if strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("remote response body leaked: %v", err)
			}
		})
	}
}

func TestModelsDevFetcherRejectsOversizedResponseAndTimeout(t *testing.T) {
	oversized := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(strings.Repeat(" ", modelsDevMaxResponseSize+1)))
	}))
	defer oversized.Close()
	_, err := (&modelsDevFetcher{client: oversized.Client(), url: oversized.URL}).fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}

	blocked := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer blocked.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = (&modelsDevFetcher{client: blocked.Client(), url: blocked.URL}).fetch(ctx)
	if err == nil || !strings.Contains(err.Error(), "fetch models.dev catalog") {
		t.Fatalf("timeout error = %v", err)
	}
	if public := publicModelsDevError(err); errorHTTPStatus(public) != http.StatusGatewayTimeout {
		t.Fatalf("public timeout status = %d, error=%v", errorHTTPStatus(public), public)
	}
}

func TestModelsDevRedirectPolicy(t *testing.T) {
	fetcher := newModelsDevFetcher()
	for _, rawURL := range []string{"http://models.dev/api.json", "https://example.com/api.json", "https://models.dev:444/api.json"} {
		request, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := fetcher.client.CheckRedirect(request, []*http.Request{{}}); err == nil {
			t.Fatalf("redirect to %s was accepted", rawURL)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, modelsDevCatalogURL, nil)
	if err := fetcher.client.CheckRedirect(request, []*http.Request{{}}); err != nil {
		t.Fatalf("same-host HTTPS redirect rejected: %v", err)
	}
}

func TestModelsDevPublicErrorsAreStable(t *testing.T) {
	generic := publicModelsDevError(context.Canceled)
	if errorHTTPStatus(generic) != http.StatusGatewayTimeout || generic.Error() != "models.dev synchronization timed out" {
		t.Fatalf("timeout public error = %v", generic)
	}
	generic = publicModelsDevError(assertionError("proxy secret"))
	if errorHTTPStatus(generic) != http.StatusBadGateway || strings.Contains(generic.Error(), "proxy secret") {
		t.Fatalf("gateway public error = %v", generic)
	}
}

type assertionError string

func (e assertionError) Error() string { return string(e) }

func TestStoreModelPriceSyncPreservesManualOverrides(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.SaveModelPrices(map[string]ModelPrice{"manual": {Input: 1}}); err != nil {
		t.Fatal(err)
	}
	settings := defaultPriceSyncSettings()
	response, err := store.ApplyModelPriceSync(map[string]ModelPrice{
		"manual": {Input: 99, Source: priceSourceModelsDev},
		"synced": {Input: 2, Source: priceSourceModelsDev, CatalogProvider: "openai"},
	}, settings, PriceSyncMetadata{Observed: 2, Matched: 2, CompletedAt: time.Now().UTC()}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if response.Prices["manual"].Input != 1 || response.Prices["synced"].Input != 2 {
		t.Fatalf("synced price book = %+v", response.Prices)
	}
	if response.LastSync == nil || response.LastSync.SkippedManual != 1 || response.LastSync.Created != 1 {
		t.Fatalf("sync metadata = %+v", response.LastSync)
	}
	if response.Revision != 2 {
		t.Fatalf("revision = %d, want 2", response.Revision)
	}
}

func TestPriceBookPersistsSyncMetadata(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	settings := PriceSyncSettings{ProviderPriority: []string{"anthropic"}, IgnoredSuffixes: []string{"-preview"}}
	if _, err := store.ApplyModelPriceSync(map[string]ModelPrice{
		"m": {Input: 3, Source: priceSourceModelsDev, ContextTiers: []ContextPriceTier{{Threshold: 200_000, Input: 6}}},
	}, settings, PriceSyncMetadata{Observed: 1, Matched: 1, CompletedAt: time.Now().UTC()}, 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	response, err := store.QueryPriceBook()
	if err != nil {
		t.Fatal(err)
	}
	if response.Revision != 1 || response.LastSync == nil || response.Prices["m"].ContextTiers[0].Input != 6 {
		t.Fatalf("persisted price book = %+v", response)
	}
	if len(response.SyncSettings.ProviderPriority) != 1 || response.SyncSettings.ProviderPriority[0] != "anthropic" {
		t.Fatalf("persisted settings = %+v", response.SyncSettings)
	}
}

func TestSavingUnchangedSyncedPricePreservesSource(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	response, err := store.ApplyModelPriceSync(map[string]ModelPrice{
		"m": {Input: 3, Source: priceSourceModelsDev, CatalogProvider: "anthropic", CatalogModel: "m"},
	}, defaultPriceSyncSettings(), PriceSyncMetadata{Observed: 1, Matched: 1, CompletedAt: time.Now().UTC()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	response, err = store.SavePriceBook(response.Prices, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Prices["m"].Source != priceSourceModelsDev {
		t.Fatalf("unchanged sync source = %q", response.Prices["m"].Source)
	}
	changed := cloneModelPrices(response.Prices)
	price := changed["m"]
	price.Input = 4
	changed["m"] = price
	response, err = store.SavePriceBook(changed, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Prices["m"].Source != priceSourceManual || response.Prices["m"].CatalogProvider != "" {
		t.Fatalf("edited sync price = %+v", response.Prices["m"])
	}
}

func TestPriceSaveDeletesEntriesByOmission(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	response, err := store.ApplyModelPriceSync(map[string]ModelPrice{
		"synced": {Input: 3, Source: priceSourceModelsDev},
	}, defaultPriceSyncSettings(), PriceSyncMetadata{Observed: 1, Matched: 1, CompletedAt: time.Now().UTC()}, 0)
	if err != nil {
		t.Fatal(err)
	}
	prices := cloneModelPrices(response.Prices)
	prices["manual"] = ModelPrice{Input: 1}
	response, err = store.SavePriceBook(prices, nil)
	if err != nil {
		t.Fatal(err)
	}
	delete(prices, "synced")
	delete(prices, "manual")
	response, err = store.SavePriceBook(prices, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Prices) != 0 {
		t.Fatalf("omitted prices were retained: %+v", response.Prices)
	}
}
