package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
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
	if len(registration.Routes) != 5 {
		t.Fatalf("management routes = %+v", registration.Routes)
	}
	wantRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/plugins/custom-id/usage"},
		{http.MethodDelete, "/plugins/custom-id/usage"},
		{http.MethodGet, "/plugins/custom-id/stats"},
		{http.MethodPost, "/plugins/custom-id/reset"},
		{http.MethodPut, "/plugins/custom-id/prices"},
	}
	for index, want := range wantRoutes {
		got := registration.Routes[index]
		if got.Method != want.method || got.Path != want.path {
			t.Fatalf("route %d = %+v, want %s %s", index, got, want.method, want.path)
		}
	}
	if len(registration.Resources) != 4 || registration.Resources[0].Path != "/dashboard" || registration.Resources[1].Path != "/stats" || registration.Resources[2].Path != "/requests" || registration.Resources[3].Path != "/prices" {
		t.Fatalf("unexpected resources: %+v", registration.Resources)
	}
	for _, route := range registration.Routes {
		if route.Menu != "" {
			t.Fatalf("authenticated route must not declare a legacy menu: %+v", route)
		}
	}
}

func runtimeRoutesForTest() registeredRoutes {
	return registeredRoutes{
		pluginID:             "test",
		usagePath:            "/v0/management/plugins/test/usage",
		statsPath:            "/v0/management/plugins/test/stats",
		resetPath:            "/v0/management/plugins/test/reset",
		dashboardPath:        "/v0/resource/plugins/test/dashboard",
		resourceStatsPath:    "/v0/resource/plugins/test/stats",
		resourceRequestsPath: "/v0/resource/plugins/test/requests",
		pricesPath:           "/v0/management/plugins/test/prices",
		resourcePricesPath:   "/v0/resource/plugins/test/prices",
	}
}

func TestManagementUsageGetAndDeleteMatchesReferenceAPI(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: runtimeRoutesForTest()}
	defer runtime.shutdown()

	timestamp := time.Date(2026, 7, 15, 8, 0, 0, 123456789, time.UTC)
	record := Record{
		ID:                "sensitive-request",
		Timestamp:         timestamp,
		APIKey:            "client-api-key",
		Provider:          "provider",
		Model:             "model",
		AuthID:            "credential-id",
		AuthIndex:         "4",
		AuthType:          "oauth",
		ExecutorType:      "executor",
		FailureBody:       "upstream private body",
		Failed:            true,
		FailureStatusCode: 503,
		Tokens:            TokenStats{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	if err := store.Record(record); err != nil {
		t.Fatal(err)
	}

	start := timestamp.Add(-time.Second).Format(time.RFC3339)
	end := timestamp.Add(time.Second).Format(time.RFC3339)
	getRequest, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   runtime.routes.usagePath,
		Query:  url.Values{"start": []string{start}, "end": []string{end}},
	})
	response, err := runtime.handleManagement(getRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("usage GET response: %+v, %v", response, err)
	}
	var usage APIUsage
	if err := json.Unmarshal(response.Body, &usage); err != nil {
		t.Fatal(err)
	}
	details := usage["client-api-key"]["model"]
	if len(details) != 1 || details[0].ID != record.ID || details[0].AuthID != "credential-id" || details[0].FailureBody != "upstream private body" {
		t.Fatalf("usage GET body = %s", response.Body)
	}

	invalidRange, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.usagePath, Query: url.Values{"start": []string{"not-a-time"}}})
	response, _ = runtime.handleManagement(invalidRange)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid start status = %d body=%s", response.StatusCode, response.Body)
	}

	deleteRequest, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: http.MethodDelete,
		Path:   runtime.routes.usagePath,
		Body:   []byte(`{"ids":["sensitive-request","sensitive-request","missing"," "]}`),
	})
	response, err = runtime.handleManagement(deleteRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("usage DELETE response: %+v, %v", response, err)
	}
	var deleted DeleteResult
	if err := json.Unmarshal(response.Body, &deleted); err != nil {
		t.Fatal(err)
	}
	if deleted.Deleted != 1 || len(deleted.Missing) != 1 || deleted.Missing[0] != "missing" {
		t.Fatalf("delete result = %+v", deleted)
	}

	missingIDs, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodDelete, Path: runtime.routes.usagePath, Body: []byte(`{"ids":[]}`)})
	response, _ = runtime.handleManagement(missingIDs)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty ids status = %d body=%s", response.StatusCode, response.Body)
	}

	wrongMethod, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.usagePath})
	response, _ = runtime.handleManagement(wrongMethod)
	if response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != "GET, DELETE" {
		t.Fatalf("usage method response = %+v", response)
	}
}

func TestManagementStatsRequestsPricesAndReset(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, routes: runtimeRoutesForTest()}
	defer runtime.shutdown()
	if err := store.Record(Record{
		ID:          "stats-record",
		Timestamp:   nowUTC(),
		APIKey:      "must-not-leak",
		AuthID:      "must-not-leak-auth",
		FailureBody: "must-not-leak-body",
		Model:       "m",
		Tokens:      TokenStats{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
	}); err != nil {
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
	var stats StatsResponse
	if err := json.Unmarshal(response.Body, &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Summary.TotalTokens != 3 || stats.Diagnostics.Processed != 1 || stats.Diagnostics.PersistedSinceOpen != 1 {
		t.Fatalf("stats = %+v", stats)
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
	for _, secret := range []string{"must-not-leak", "must-not-leak-auth", "must-not-leak-body"} {
		if strings.Contains(string(response.Body), secret) {
			t.Fatalf("resource requests leaked %q: %s", secret, response.Body)
		}
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
	invalidPricesRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPut, Path: runtime.routes.pricesPath, Headers: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"prices":{"m":{"input":-1,"output":10}}}`)})
	response, _ = runtime.handleManagement(invalidPricesRequest)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid prices status = %d body=%s", response.StatusCode, response.Body)
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
