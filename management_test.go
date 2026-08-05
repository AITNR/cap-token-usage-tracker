package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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
	if len(registration.Routes) != 6 || registration.Routes[0].Path != "/plugins/custom-id/stats" || registration.Routes[2].Method != http.MethodPut || registration.Routes[2].Path != "/plugins/custom-id/prices" || registration.Routes[3].Path != "/plugins/custom-id/prices/sync" || registration.Routes[4].Method != http.MethodGet || registration.Routes[4].Path != "/plugins/custom-id/backup" || registration.Routes[5].Method != http.MethodPost || registration.Routes[5].Path != "/plugins/custom-id/restore" || len(registration.Resources) != 7 || registration.Resources[0].Path != "/dashboard" || registration.Resources[1].Path != "/stats" || registration.Resources[2].Path != "/requests" || registration.Resources[3].Path != "/costs" || registration.Resources[4].Path != "/exchange-rate" || registration.Resources[5].Path != "/prices" || registration.Resources[6].Path != "/preferences" {
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
		pluginID: "test", statsPath: "/v0/management/plugins/test/stats", resetPath: "/v0/management/plugins/test/reset", dashboardPath: "/v0/resource/plugins/test/dashboard", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests", resourceCostsPath: "/v0/resource/plugins/test/costs", resourceExchangeRatePath: "/v0/resource/plugins/test/exchange-rate", pricesPath: "/v0/management/plugins/test/prices", priceSyncPath: "/v0/management/plugins/test/prices/sync", resourcePricesPath: "/v0/resource/plugins/test/prices", resourcePreferencesPath: "/v0/resource/plugins/test/preferences",
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
	customQuery := url.Values{
		"range": []string{"custom"},
		"start": []string{time.Now().Add(-time.Hour).Format(time.RFC3339)},
		"end":   []string{time.Now().Add(time.Hour).Format(time.RFC3339)},
	}
	customStatsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: customQuery})
	response, err = runtime.handleManagement(customStatsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"range":"custom"`) || !strings.Contains(string(response.Body), `"total_tokens":3`) {
		t.Fatalf("custom stats response: %+v, %v", response, err)
	}
	invalidRangeRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceStatsPath, Query: url.Values{"range": []string{"custom"}, "start": []string{time.Now().Format(time.RFC3339)}}})
	response, err = runtime.handleManagement(invalidRangeRequest)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid custom range response: %+v, %v", response, err)
	}

	requestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: url.Values{"range": []string{"24h"}, "offset": []string{"0"}, "limit": []string{"20"}, "model": []string{"m"}}})
	response, err = runtime.handleManagement(requestsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total":1`) || !strings.Contains(string(response.Body), `"model":"m"`) {
		t.Fatalf("resource requests response: %+v, %v", response, err)
	}
	requestsResultQuery := url.Values{"range": []string{"24h"}, "result": []string{"failed"}}
	requestsResultRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: requestsResultQuery})
	response, err = runtime.handleManagement(requestsResultRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"total":0`) {
		t.Fatalf("filtered resource requests response: %+v, %v", response, err)
	}
	requestsResultQuery.Set("result", "unknown")
	requestsResultRequest, _ = json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: requestsResultQuery})
	response, err = runtime.handleManagement(requestsResultRequest)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid request result response: %+v, %v", response, err)
	}
	customRequestsQuery := url.Values{}
	for key, values := range customQuery {
		customRequestsQuery[key] = append([]string(nil), values...)
	}
	customRequestsQuery.Set("offset", "0")
	customRequestsQuery.Set("limit", "20")
	customRequestsQuery.Set("model", "m")
	customRequestsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceRequestsPath, Query: customRequestsQuery})
	response, err = runtime.handleManagement(customRequestsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"range":"custom"`) || !strings.Contains(string(response.Body), `"total":1`) {
		t.Fatalf("custom requests response: %+v, %v", response, err)
	}

	pricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePricesPath})
	response, err = runtime.handleManagement(pricesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"prices":{}`) {
		t.Fatalf("empty prices response: %+v, %v", response, err)
	}
	preferencesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePreferencesPath})
	response, err = runtime.handleManagement(preferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"request_page_size":100`) || !strings.Contains(string(response.Body), `"hidden_request_columns":[]`) {
		t.Fatalf("default preferences response: %+v, %v", response, err)
	}
	savePreferencesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourcePreferencesPath, Query: url.Values{"save": []string{"1"}, "request_page_size": []string{"50"}, "dimension_page_size": []string{"200"}, "hidden_request_column": []string{"source"}, "hidden_dimension_column": []string{"provider"}}})
	response, err = runtime.handleManagement(savePreferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"request_page_size":50`) || !strings.Contains(string(response.Body), `"hidden_dimension_columns":["provider"]`) {
		t.Fatalf("save preferences response: %+v, %v", response, err)
	}
	response, err = runtime.handleManagement(preferencesRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"dimension_page_size":200`) || !strings.Contains(string(response.Body), `"hidden_request_columns":["source"]`) {
		t.Fatalf("persisted preferences response: %+v, %v", response, err)
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
	customCostsRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceCostsPath, Query: customQuery})
	response, err = runtime.handleManagement(customCostsRequest)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"range":"custom"`) || !strings.Contains(string(response.Body), `"priced_requests":1`) {
		t.Fatalf("custom costs response: %+v, %v", response, err)
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

func TestDashboardPreferencesResourceValidation(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{pluginID: "test", resourcePreferencesPath: "/v0/resource/plugins/test/preferences"}}
	defer runtime.shutdown()

	request := func(method string, query url.Values) pluginapi.ManagementResponse {
		raw, _ := json.Marshal(pluginapi.ManagementRequest{Method: method, Path: runtime.routes.resourcePreferencesPath, Query: query})
		response, handleErr := runtime.handleManagement(raw)
		if handleErr != nil {
			t.Fatal(handleErr)
		}
		return response
	}
	if response := request(http.MethodGet, url.Values{"request_page_size": []string{"100"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("save flag omission status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"0"}, "dimension_page_size": []string{"100"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid page size status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "hidden_request_column": []string{"api_key"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid column status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodGet, url.Values{"save": []string{"1"}, "request_page_size": []string{"100"}, "dimension_page_size": []string{"100"}, "unknown": []string{"value"}}); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown query status = %d body=%s", response.StatusCode, response.Body)
	}
	if response := request(http.MethodPost, nil); response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != "GET" {
		t.Fatalf("wrong method response = %+v", response)
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

func TestManagementSourceFilterAppliesToStatsRequestsAndCosts(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", resourceStatsPath: "/v0/resource/plugins/test/stats", resourceRequestsPath: "/v0/resource/plugins/test/requests", resourceCostsPath: "/v0/resource/plugins/test/costs",
	}}
	defer runtime.shutdown()
	for _, usage := range []normalizedUsage{
		{Dimensions: Dimensions{Model: "alpha", Source: "cli"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 3}},
		{Dimensions: Dimensions{Model: "beta", Source: "web"}, RequestedAt: nowUTC(), Counters: Counters{Requests: 1, TotalTokens: 5}},
	} {
		if err := store.Record(usage); err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{"range": []string{"24h"}, "source": []string{"cli"}}
	for _, path := range []string{runtime.routes.resourceStatsPath, runtime.routes.resourceRequestsPath, runtime.routes.resourceCostsPath} {
		if path == runtime.routes.resourceRequestsPath {
			query.Set("offset", "0")
			query.Set("limit", "100")
		}
		request, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: path, Query: query})
		response, err := runtime.handleManagement(request)
		if err != nil || response.StatusCode != http.StatusOK || strings.Contains(string(response.Body), `"source":"web"`) {
			t.Fatalf("source-filtered %s response: %+v, %v", path, response, err)
		}
		if path == runtime.routes.resourceStatsPath && (!strings.Contains(string(response.Body), `"total_tokens":3`) || !strings.Contains(string(response.Body), `"sources":["cli","web"]`)) {
			t.Fatalf("source-filtered stats response: %+v", response)
		}
		if path == runtime.routes.resourceRequestsPath && !strings.Contains(string(response.Body), `"total":1`) {
			t.Fatalf("source-filtered request response: %+v", response)
		}
		if path == runtime.routes.resourceCostsPath && !strings.Contains(string(response.Body), `"requests":1`) {
			t.Fatalf("source-filtered cost response: %+v", response)
		}
	}

	allCosts, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceCostsPath, Query: url.Values{"range": []string{"24h"}}})
	response, err := runtime.handleManagement(allCosts)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"requests":2`) {
		t.Fatalf("unfiltered costs response: %+v, %v", response, err)
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

func TestManagementBackupAndRestore(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: registeredRoutes{
		pluginID: "test", backupPath: "/v0/management/plugins/test/backup", restorePath: "/v0/management/plugins/test/restore",
	}}
	defer runtime.shutdown()
	if err := store.Record(normalizedUsage{
		Dimensions:  Dimensions{Model: "backup-endpoint", Source: "cli"},
		RequestedAt: nowUTC(),
		Counters:    Counters{Requests: 1, InputTokens: 4, OutputTokens: 5, TotalTokens: 9},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDashboardPreferences(DashboardPreferences{
		RequestPageSize:   25,
		DimensionPageSize: 50,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelPrices(map[string]ModelPrice{
		"backup-endpoint": {Input: 1.5, Output: 6},
	}); err != nil {
		t.Fatal(err)
	}

	backupRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.backupPath})
	response, err := runtime.handleManagement(backupRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("backup response: %+v, %v", response, err)
	}
	if response.Headers.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("backup content type = %q", response.Headers.Get("Content-Type"))
	}
	if !strings.Contains(response.Headers.Get("Content-Disposition"), "attachment;") {
		t.Fatalf("backup disposition = %q", response.Headers.Get("Content-Disposition"))
	}
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("backup cache control = %q", response.Headers.Get("Cache-Control"))
	}
	if response.Headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("backup nosniff header = %q", response.Headers.Get("X-Content-Type-Options"))
	}
	if len(response.Body) == 0 {
		t.Fatal("expected non-empty backup body")
	}

	backupPath := filepath.Join(t.TempDir(), "endpoint-backup.db")
	if err := os.WriteFile(backupPath, response.Body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateRestoreDatabase(backupPath); err != nil {
		t.Fatalf("backup body is not a valid bolt database: %v", err)
	}
	backupBody := append([]byte(nil), response.Body...)

	emptyRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
	})
	response, err = runtime.handleManagement(emptyRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "backup body must not be empty") {
		t.Fatalf("empty restore response: %+v, %v", response, err)
	}

	invalidTypeRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    runtime.routes.restorePath,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte("not-a-database"),
	})
	response, err = runtime.handleManagement(invalidTypeRestore)
	if err != nil || response.StatusCode != http.StatusUnsupportedMediaType || !strings.Contains(string(response.Body), "application/octet-stream") {
		t.Fatalf("invalid content type restore response: %+v, %v", response, err)
	}

	missingConfirmRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Path:    runtime.routes.restorePath,
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body:    backupBody,
	})
	response, err = runtime.handleManagement(missingConfirmRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest || !strings.Contains(string(response.Body), "missing X-Confirm-Restore: replace header") {
		t.Fatalf("missing confirm restore response: %+v, %v", response, err)
	}

	invalidBodyRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: []byte("not-a-database"),
	})
	response, err = runtime.handleManagement(invalidBodyRestore)
	if err != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid body restore response: %+v, %v", response, err)
	}

	// Oversized restore body is rejected at the management layer before touching storage.
	// Call restoreResponse directly to avoid base64-encoding a 64 MiB payload through JSON.
	oversizedBody := make([]byte, maxDatabaseBackupBytes+1)
	response, err = runtime.restoreResponse(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: oversizedBody,
	})
	if err != nil || response.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(response.Body), "backup body is too large") {
		t.Fatalf("oversized restore response: %+v, %v", response, err)
	}

	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}
	// Mutate preferences/prices after backup so restore must reintroduce the originals.
	// Reset only clears usage counters, not preferences or prices.
	if _, err := store.SaveDashboardPreferences(DashboardPreferences{
		RequestPageSize:   100,
		DimensionPageSize: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveModelPrices(map[string]ModelPrice{
		"mutated-model": {Input: 9, Output: 9},
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 0 {
		t.Fatalf("expected empty stats after reset, got %+v", stats.Summary)
	}

	roundTripRestore, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Path:   runtime.routes.restorePath,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: backupBody,
	})
	response, err = runtime.handleManagement(roundTripRestore)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"restored":true`) {
		t.Fatalf("round-trip restore response: %+v, %v", response, err)
	}
	stats, err = store.Query("retention")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 9 {
		t.Fatalf("restored total tokens = %d, want 9", stats.Summary.TotalTokens)
	}
	preferences, err := store.QueryDashboardPreferences()
	if err != nil {
		t.Fatal(err)
	}
	if preferences.RequestPageSize != 25 || preferences.DimensionPageSize != 50 {
		t.Fatalf("restored preferences = %+v, want request=25 dimension=50", preferences)
	}
	prices, err := store.QueryModelPrices()
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 1 || prices["backup-endpoint"].Input != 1.5 || prices["backup-endpoint"].Output != 6 {
		t.Fatalf("restored model prices = %+v", prices)
	}
}
