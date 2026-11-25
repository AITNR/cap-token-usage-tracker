package plugin

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func newModelCatalogRuntime(t *testing.T) (*pluginRuntime, string) {
	t.Helper()
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	t.Cleanup(func() { _ = runtime.shutdown() })

	raw, _ := json.Marshal(pluginapi.ManagementRegistrationRequest{ResourceBasePath: "/v0/resource/plugins/test"})
	if _, err := runtime.registerManagement(raw); err != nil {
		t.Fatal(err)
	}
	sessionRequest, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.fullModeSessionPath})
	response, err := runtime.handleManagement(sessionRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode session response: %+v, %v", response, err)
	}
	var session struct {
		Session string `json:"session"`
	}
	if err := json.Unmarshal(response.Body, &session); err != nil || session.Session == "" {
		t.Fatalf("full-mode session payload: %s, %v", response.Body, err)
	}
	return runtime, session.Session
}

// stageFullModePayload runs the three-stage capability upload the full-mode
// dashboard uses, so the tests exercise the same path as the browser.
func stageFullModePayload(t *testing.T, runtime *pluginRuntime, path, session, body string) pluginapi.ManagementResponse {
	t.Helper()
	encoded := base64.RawURLEncoding.EncodeToString([]byte(body))
	headers := http.Header{"X-Full-Mode-Session": []string{session}}

	begin, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    path,
		Headers: headers,
		Query:   map[string][]string{"stage": {"begin"}, "chunks": {"1"}},
	})
	response, err := runtime.handleManagement(begin)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode upload begin: %+v, %v", response, err)
	}
	var upload struct {
		Upload string `json:"upload"`
	}
	if err := json.Unmarshal(response.Body, &upload); err != nil || upload.Upload == "" {
		t.Fatalf("full-mode upload begin payload: %s, %v", response.Body, err)
	}

	chunkHeaders := headers.Clone()
	chunkHeaders.Set("X-Full-Mode-Payload", encoded)
	chunk, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    path,
		Headers: chunkHeaders,
		Query:   map[string][]string{"stage": {"chunk"}, "upload": {upload.Upload}, "index": {"0"}},
	})
	response, err = runtime.handleManagement(chunk)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("full-mode upload chunk: %+v, %v", response, err)
	}

	commit, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    path,
		Headers: headers,
		Query:   map[string][]string{"stage": {"commit"}, "upload": {upload.Upload}},
	})
	response, err = runtime.handleManagement(commit)
	if err != nil {
		t.Fatalf("full-mode upload commit: %v", err)
	}
	return response
}

func TestFullModeModelCatalogNeedsOnlySession(t *testing.T) {
	runtime, session := newModelCatalogRuntime(t)

	missing, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.fullModeModelsPath})
	response, err := runtime.handleManagement(missing)
	if err != nil || response.StatusCode != http.StatusUnauthorized || !strings.Contains(string(response.Body), "full-mode session") {
		t.Fatalf("catalog without session: %+v, %v", response, err)
	}

	emptyRequest, _ := json.Marshal(pluginapi.ManagementRequest{
		Method:  http.MethodGet,
		Path:    runtime.routes.fullModeModelsPath,
		Headers: http.Header{"X-Full-Mode-Session": []string{session}},
	})
	response, err = runtime.handleManagement(emptyRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("empty catalog: %+v, %v", response, err)
	}
	var empty modelCatalog
	if err := json.Unmarshal(response.Body, &empty); err != nil {
		t.Fatalf("empty catalog body: %s, %v", response.Body, err)
	}
	if len(empty.Models) != 0 || empty.Observed != 0 || empty.Priced != 0 {
		t.Fatalf("fresh catalog must be empty: %+v", empty)
	}

	// Saving prices with nothing but the session must succeed: the full-mode
	// capability is the only credential the editor needs.
	saved := stageFullModePayload(t, runtime, runtime.routes.fullModePricesSavePath, session, `{"prices":{"priced-model":{"input":1.25,"output":5}}}`)
	if saved.StatusCode != http.StatusOK || !strings.Contains(string(saved.Body), "priced-model") {
		t.Fatalf("price save without an API key: %+v", saved)
	}
	if err := runtime.store.Record(normalizedUsage{
		Dimensions:  Dimensions{Model: "observed-model"},
		RequestedAt: time.Now().UTC(),
		Counters:    Counters{Requests: 1, InputTokens: 10, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}

	response, err = runtime.handleManagement(emptyRequest)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("catalog after usage: %+v, %v", response, err)
	}
	var catalog modelCatalog
	if err := json.Unmarshal(response.Body, &catalog); err != nil {
		t.Fatalf("catalog body: %s, %v", response.Body, err)
	}
	if strings.Join(catalog.Models, ",") != "observed-model,priced-model" {
		t.Fatalf("catalog models = %+v", catalog.Models)
	}
	if catalog.Observed != 1 || catalog.Priced != 1 || catalog.GeneratedAt.IsZero() {
		t.Fatalf("catalog counters = %+v", catalog)
	}
}

func TestFullModePriceSyncFallsBackToModelCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"provider":{"models":{"observed-model":{"cost":{"input":3,"output":9}}}}}`))
	}))
	defer server.Close()

	runtime, session := newModelCatalogRuntime(t)
	runtime.modelsDevFetcher = &modelsDevFetcher{client: server.Client(), url: server.URL}
	if err := runtime.store.Record(normalizedUsage{
		Dimensions:  Dimensions{Model: "observed-model"},
		RequestedAt: time.Now().UTC(),
		Counters:    Counters{Requests: 1, InputTokens: 10, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}

	// No "models" field: the server derives the list from its own records, so
	// the full-mode sync needs nothing but the capability.
	response := stageFullModePayload(t, runtime, runtime.routes.fullModePriceSyncPath, session, `{"source":"models.dev"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("sync without models: %+v", response)
	}
	var book ModelPricesResponse
	if err := json.Unmarshal(response.Body, &book); err != nil {
		t.Fatalf("sync body: %s, %v", response.Body, err)
	}
	if price, ok := book.Prices["observed-model"]; !ok || price.Input != 3 || price.Output != 9 {
		t.Fatalf("synchronized prices = %+v", book.Prices)
	}
}

func TestFullModePriceSyncRejectsEmptyCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"provider":{"models":{"observed-model":{"cost":{"input":3}}}}}`))
	}))
	defer server.Close()

	runtime, session := newModelCatalogRuntime(t)
	runtime.modelsDevFetcher = &modelsDevFetcher{client: server.Client(), url: server.URL}

	response := stageFullModePayload(t, runtime, runtime.routes.fullModePriceSyncPath, session, `{"source":"models.dev"}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty catalog sync status = %d body=%s", response.StatusCode, response.Body)
	}
	if !strings.Contains(string(response.Body), "no models to synchronize yet") || strings.Contains(string(response.Body), "CLIProxyAPI") {
		t.Fatalf("empty catalog sync error must not mention CLIProxyAPI credentials: %s", response.Body)
	}
}
