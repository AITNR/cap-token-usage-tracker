package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestPluginIDFromResourceBase(t *testing.T) {
	got, err := pluginIDFromResourceBase("/v0/resource/plugins/cap-token-usage-tracker")
	if err != nil || got != "cap-token-usage-tracker" {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, invalid := range []string{"/wrong/id", "/v0/resource/plugins/a/b", "/v0/resource/plugins/bad id"} {
		if _, err := pluginIDFromResourceBase(invalid); err == nil {
			t.Fatalf("accepted invalid base %q", invalid)
		}
	}
}

func TestManagementRegistrationUsesDynamicPluginID(t *testing.T) {
	runtime := &pluginRuntime{}
	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/custom-id"})
	registration, err := runtime.registerManagement(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(registration.Routes) != 4 || registration.Routes[0].Path != "/plugins/custom-id/stats" || registration.Routes[2].Method != http.MethodPut || registration.Routes[2].Path != "/plugins/custom-id/prices" || registration.Routes[3].Path != "/plugins/custom-id/prices/sync" || len(registration.Resources) != 6 || registration.Resources[0].Path != "/dashboard" || registration.Resources[1].Path != "/stats" || registration.Resources[2].Path != "/requests" || registration.Resources[3].Path != "/costs" || registration.Resources[4].Path != "/exchange-rate" || registration.Resources[5].Path != "/prices" {
		t.Fatalf("unexpected registration: %+v", registration)
	}
	if registration.Routes[0].Menu != "" {
		t.Fatal("authenticated stats route must not declare a legacy menu")
	}
}

func TestManagementStatsAndReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", statsPath: "/v0/management/plugins/test/stats", resetPath: "/v0/management/plugins/test/reset", dashboardPath: "/v0/resource/plugins/test/dashboard", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests", resourceCostsPath: "/v0/resource/plugins/test/costs", resourceExchangeRatePath: "/v0/resource/plugins/test/exchange-rate", pricesPath: "/v0/management/plugins/test/prices", priceSyncPath: "/v0/management/plugins/test/prices/sync", resourcePricesPath: "/v0/resource/plugins/test/prices",
	}}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 3}}); err != nil {
		t.Fatal(err)
	}

	statsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.statsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err := runtime.handleManagement(statsRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("stats response: %+v, %v", response, err)
	}
	if strings.Contains(string(response.Body), `"ok"`) {
		t.Fatal("HTTP response body was incorrectly wrapped in RPC envelope")
	}
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store header")
	}

	resourceStatsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err = runtime.handleManagement(resourceStatsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total_tokens":3`) {
		t.Fatalf("resource stats response: %+v, %v", response, err)
	}

	requestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: url.Values{"range": []string{"24h"}, "offset": []string{"0"}, "limit": []string{"20"}, "model": []string{"m"}}})
	response, err = runtime.handleManagement(requestsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total":1`) || !strings.Contains(string(response.Body), `"model":"m"`) {
		t.Fatalf("resource requests response: %+v, %v", response, err)
	}

	pricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePricesPath})
	response, err = runtime.handleManagement(pricesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"prices":{}`) {
		t.Fatalf("empty prices response: %+v, %v", response, err)
	}
	savePricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPut, Path: runtime.routes.pricesPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"prices":{"m":{"input":2.5,"output":10}}}`)})
	response, err = runtime.handleManagement(savePricesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"input":2.5`) {
		t.Fatalf("save prices response: %+v, %v", response, err)
	}
	response, err = runtime.handleManagement(pricesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"output":10`) {
		t.Fatalf("persisted prices response: %+v, %v", response, err)
	}
	costsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceCostsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err = runtime.handleManagement(costsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"priced_requests":1`) || !strings.Contains(string(response.Body), `"estimate_basis":"current_price_book"`) {
		t.Fatalf("resource costs response: %+v, %v", response, err)
	}
	catalogServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"openai":{"models":{"m":{"cost":{"input":1,"output":2,"cache_read":0.1,"cache_write":1}}}}}`))
	}))
	defer catalogServer.Close()
	runtime.modelsDevFetcher = &modelsDevFetcher{client: catalogServer.Client(), url: catalogServer.URL}
	syncRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.priceSyncPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"source":"models.dev","models":["m"]}`)})
	response, err = runtime.handleManagement(syncRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"skipped_manual":1`) {
		t.Fatalf("price sync response: %+v, %v", response, err)
	}
	invalidPricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPut, Path: runtime.routes.pricesPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"prices":{"m":{"input":-1,"output":10}}}`)})
	response, _ = runtime.handleManagement(invalidPricesRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid prices status = %d body=%s", response.StatusCode, response.Body)
	}
	for _, body := range []string{
		`{"prices":{},"unknown":true}`,
		`{"prices":{"m":{"input":1,"unknown":true}}}`,
		`{"prices":{}} {"prices":{}}`,
		``,
	} {
		requestRaw, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPut, Path: runtime.routes.pricesPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(body)})
		response, _ = runtime.handleManagement(requestRaw)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("strict prices body %q status = %d body=%s", body, response.StatusCode, response.Body)
		}
	}
	unknownSyncRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.priceSyncPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"source":"models.dev","url":"https://example.com"}`)})
	response, _ = runtime.handleManagement(unknownSyncRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown sync field status = %d body=%s", response.StatusCode, response.Body)
	}

	emptyModelsSyncRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.priceSyncPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"source":"models.dev","models":[]}`)})
	response, _ = runtime.handleManagement(emptyModelsSyncRequest)
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "at least one CLIProxyAPI model") {
		t.Fatalf("empty sync models status = %d body=%s", response.StatusCode, response.Body)
	}

	badRequestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: url.Values{"offset": []string{"bad"}}})
	response, _ = runtime.handleManagement(badRequestsRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad requests query status = %d", response.StatusCode)
	}

	looseContentType, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/jsonp"}}, Body: []byte(`{"confirm":"reset"}`)})
	response, _ = runtime.handleManagement(looseContentType)
	if response.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("loose content type status = %d", response.StatusCode)
	}

	badReset, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"confirm":"no"}`)})
	response, _ = runtime.handleManagement(badReset)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad reset status = %d", response.StatusCode)
	}
	goodReset, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resetPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"confirm":"reset"}`)})
	response, _ = runtime.handleManagement(goodReset)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("good reset status = %d body=%s", response.StatusCode, response.Body)
	}
}

func TestSyncModelsDevUsesProvidedCLIModels(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "usage-only"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1}}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"openai":{"models":{"cli-only":{"cost":{"input":1,"output":2}},"usage-only":{"cost":{"input":9,"output":9}}}}}`))
	}))
	defer server.Close()
	runtime.modelsDevFetcher = &modelsDevFetcher{client: server.Client(), url: server.URL}
	response, err := runtime.syncModelsDev(nil, []string{"cli-only"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.Prices["cli-only"]; !ok {
		t.Fatalf("CLIProxyAPI model was not synchronized: %+v", response.Prices)
	}
	if _, ok := response.Prices["usage-only"]; ok {
		t.Fatalf("usage-only model was synchronized: %+v", response.Prices)
	}
}

func TestConcurrentPriceSyncReturnsConflict(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{pluginID: "test", priceSyncPath: "/v0/management/plugins/test/prices/sync"}}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1}}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		once.Do(func() { close(started) })
		<-release
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"openai":{"models":{"m":{"cost":{"input":1,"output":2}}}}}`))
	}))
	defer server.Close()
	runtime.modelsDevFetcher = &modelsDevFetcher{client: server.Client(), url: server.URL}
	raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.priceSyncPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"source":"models.dev","models":["m"]}`)})
	firstDone := make(chan pluginapi.ManagementResponse, 1)
	go func() {
		response, _ := runtime.handleManagement(raw)
		firstDone <- response
	}()
	<-started
	response, _ := runtime.handleManagement(raw)
	if response.StatusCode != http.StatusConflict {
		close(release)
		t.Fatalf("concurrent sync status = %d body=%s", response.StatusCode, response.Body)
	}
	close(release)
	if response = <-firstDone; response.StatusCode != http.StatusOK {
		t.Fatalf("first sync status = %d body=%s", response.StatusCode, response.Body)
	}
	response, _ = runtime.handleManagement(raw)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("later sync status = %d body=%s", response.StatusCode, response.Body)
	}
}

func TestStalePriceSyncDoesNotOverwriteNewSettings(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{Dimensions: Dimensions{Model: "m"}, RequestedAt: time.Now().UTC(), Counters: Counters{Requests: 1}}); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-release
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"openai":{"models":{"m":{"cost":{"input":1}}}}}`))
	}))
	defer server.Close()
	runtime.modelsDevFetcher = &modelsDevFetcher{client: server.Client(), url: server.URL}
	done := make(chan error, 1)
	go func() {
		_, err := runtime.syncModelsDev(nil, []string{"m"})
		done <- err
	}()
	<-started
	newSettings := PriceSyncSettings{ProviderPriority: []string{"anthropic"}, IgnoredSuffixes: []string{"-custom"}}
	if _, err := store.SavePriceBook(map[string]ModelPrice{}, &newSettings); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err == nil || errorHTTPStatus(err) != http.StatusConflict {
		t.Fatalf("stale sync error = %v", err)
	}
	book, err := store.QueryPriceBook()
	if err != nil {
		t.Fatal(err)
	}
	if len(book.SyncSettings.ProviderPriority) != 1 || book.SyncSettings.ProviderPriority[0] != "anthropic" || book.Revision != 1 {
		t.Fatalf("new settings were overwritten: %+v", book)
	}
}

func TestDashboardSecurityContract(t *testing.T) {
	response := dashboardResponse()
	html := string(response.Body)
	for _, required := range []string{"/v0/resource/plugins/", "/v0/management/plugins/", "Authorization", "type=\"password\"", "textContent", "replaceChildren", "load().catch"} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
	for _, forbidden := range []string{"localStorage", "sessionStorage", "connectButton", "logoutButton", "fetch('stats')", `fetch("stats")`} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard contains forbidden pattern %q", forbidden)
		}
	}
	if response.Headers.Get("Content-Security-Policy") == "" {
		t.Fatal("missing content security policy")
	}
}
