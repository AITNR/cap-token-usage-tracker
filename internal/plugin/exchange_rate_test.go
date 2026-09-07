package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestExchangeRateFetcherValidatesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = response.Write([]byte(`{"result":"success","base_code":"USD","time_last_update_unix":1752667200,"rates":{"CNY":7.18}}`))
	}))
	defer server.Close()
	result, err := (&exchangeRateFetcher{client: server.Client(), url: server.URL}).fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Rates["CNY"] != 7.18 || result.BaseCode != "USD" {
		t.Fatalf("exchange-rate response = %+v", result)
	}
}

func TestExchangeRateFetcherRejectsInvalidResponses(t *testing.T) {
	for name, fixture := range map[string]struct {
		status      int
		contentType string
		body        string
		want        string
	}{
		"non-200":       {status: http.StatusBadGateway, contentType: "application/json", body: `{"secret":"must-not-leak"}`, want: "HTTP 502"},
		"wrong content": {status: http.StatusOK, contentType: "text/plain", body: `{}`, want: "application/json"},
		"malformed":     {status: http.StatusOK, contentType: "application/json", body: `{`, want: "decode exchange rate"},
		"trailing":      {status: http.StatusOK, contentType: "application/json", body: `{} {}`, want: "trailing JSON value"},
		"failed result": {status: http.StatusOK, contentType: "application/json", body: `{"result":"error","base_code":"USD","rates":{"CNY":7}}`, want: "unsuccessful"},
		"wrong base":    {status: http.StatusOK, contentType: "application/json", body: `{"result":"success","base_code":"EUR","rates":{"CNY":7}}`, want: "unexpected base"},
		"missing CNY":   {status: http.StatusOK, contentType: "application/json", body: `{"result":"success","base_code":"USD","rates":{}}`, want: "invalid CNY"},
		"zero CNY":      {status: http.StatusOK, contentType: "application/json", body: `{"result":"success","base_code":"USD","rates":{"CNY":0}}`, want: "invalid CNY"},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", fixture.contentType)
				response.WriteHeader(fixture.status)
				_, _ = response.Write([]byte(fixture.body))
			}))
			defer server.Close()
			_, err := (&exchangeRateFetcher{client: server.Client(), url: server.URL}).fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error = %v, want %q", err, fixture.want)
			}
			if strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("provider response leaked: %v", err)
			}
		})
	}
}

func TestExchangeRateFetcherRejectsOversizedResponseAndTimeout(t *testing.T) {
	oversized := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(strings.Repeat(" ", exchangeRateMaxResponseSize+1)))
	}))
	defer oversized.Close()
	_, err := (&exchangeRateFetcher{client: oversized.Client(), url: oversized.URL}).fetch(context.Background())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("oversized response error = %v", err)
	}

	blocked := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer blocked.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = (&exchangeRateFetcher{client: blocked.Client(), url: blocked.URL}).fetch(ctx)
	if err == nil || !strings.Contains(err.Error(), "fetch exchange rate") {
		t.Fatalf("timeout error = %v", err)
	}
	if public := publicExchangeRateError(err); errorHTTPStatus(public) != http.StatusGatewayTimeout {
		t.Fatalf("public timeout status = %d, error=%v", errorHTTPStatus(public), public)
	}
}

func TestExchangeRateServiceCachesAndFallsBackToStale(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		if fail.Load() {
			response.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = response.Write([]byte(`{"result":"success","base_code":"USD","time_last_update_unix":1752667200,"rates":{"CNY":7.2}}`))
	}))
	defer server.Close()
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	service := &exchangeRateService{
		fetcher: &exchangeRateFetcher{client: server.Client(), url: server.URL},
		now:     func() time.Time { return now },
	}
	first, err := service.latest()
	if err != nil || first.Rate != 7.2 || first.Stale {
		t.Fatalf("first rate = %+v, %v", first, err)
	}
	second, err := service.latest()
	if err != nil || calls.Load() != 1 || second.Stale {
		t.Fatalf("cached rate = %+v, calls=%d, err=%v", second, calls.Load(), err)
	}
	fail.Store(true)
	now = now.Add(exchangeRateFreshTTL + time.Minute)
	stale, err := service.latest()
	if err != nil || !stale.Stale || stale.Rate != first.Rate || calls.Load() != 2 {
		t.Fatalf("stale rate = %+v, calls=%d, err=%v", stale, calls.Load(), err)
	}
	staleAgain, err := service.latest()
	if err != nil || !staleAgain.Stale || calls.Load() != 2 {
		t.Fatalf("backoff stale rate = %+v, calls=%d, err=%v", staleAgain, calls.Load(), err)
	}
	now = now.Add(exchangeRateStaleTTL)
	if _, err := service.latest(); err == nil || errorHTTPStatus(err) != http.StatusBadGateway {
		t.Fatalf("expired stale error = %v", err)
	}
}

func TestExchangeRateResourceRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"result":"success","base_code":"USD","rates":{"CNY":7.25}}`))
	}))
	defer server.Close()
	runtime := &pluginRuntime{
		routes: registeredRoutes{pluginID: "test", resourceExchangeRatePath: "/v0/resource/plugins/test/exchange-rate"},
		exchangeRates: &exchangeRateService{
			fetcher: &exchangeRateFetcher{client: server.Client(), url: server.URL},
			now:     nowUTC,
		},
	}
	request, _ := json.Marshal(pluginapi.ManagementRequest{Method: http.MethodGet, Path: runtime.routes.resourceExchangeRatePath})
	response, err := runtime.handleManagement(request)
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(response.Body), `"rate":7.25`) {
		t.Fatalf("exchange-rate response: %+v, %v", response, err)
	}
	request, _ = json.Marshal(pluginapi.ManagementRequest{Method: http.MethodPost, Path: runtime.routes.resourceExchangeRatePath})
	response, err = runtime.handleManagement(request)
	if err != nil || response.StatusCode != http.StatusMethodNotAllowed || response.Headers.Get("Allow") != http.MethodGet {
		t.Fatalf("exchange-rate method response: %+v, %v", response, err)
	}
}

func TestExchangeRateRedirectPolicy(t *testing.T) {
	fetcher := newExchangeRateFetcher()
	for _, rawURL := range []string{"http://open.er-api.com/v6/latest/USD", "https://example.com/v6/latest/USD", "https://open.er-api.com:444/v6/latest/USD"} {
		request, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := fetcher.client.CheckRedirect(request, []*http.Request{{}}); err == nil {
			t.Fatalf("redirect to %s was accepted", rawURL)
		}
	}
	request, _ := http.NewRequest(http.MethodGet, exchangeRateURL, nil)
	if err := fetcher.client.CheckRedirect(request, []*http.Request{{}}); err != nil {
		t.Fatalf("same-host HTTPS redirect rejected: %v", err)
	}
}

type blockingExchangeRateFetcher struct {
	calls   atomic.Int32
	release chan struct{}
}

func (f *blockingExchangeRateFetcher) fetch(context.Context) (exchangeRateProviderResponse, error) {
	f.calls.Add(1)
	<-f.release
	return exchangeRateProviderResponse{
		Result:             "success",
		BaseCode:           "USD",
		TimeLastUpdateUnix: 1752667200,
		Rates:              map[string]float64{"CNY": 7.21},
	}, nil
}

func TestExchangeRateServiceCoalescesConcurrentRefreshes(t *testing.T) {
	fetcher := &blockingExchangeRateFetcher{release: make(chan struct{})}
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	service := &exchangeRateService{fetcher: fetcher, now: func() time.Time { return now }}

	const callers = 20
	type latestResult struct {
		response ExchangeRateResponse
		err      error
	}
	results := make(chan latestResult, callers)
	for i := 0; i < callers; i++ {
		go func() {
			response, err := service.latest()
			results <- latestResult{response: response, err: err}
		}()
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		service.mu.Lock()
		refreshing := service.refresh != nil
		service.mu.Unlock()
		if refreshing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exchange-rate refresh did not start")
		}
		time.Sleep(time.Millisecond)
	}
	close(fetcher.release)

	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil || result.response.Rate != 7.21 || result.response.Stale {
			t.Fatalf("coalesced result %d = %+v, err = %v", i, result.response, result.err)
		}
	}
	if calls := fetcher.calls.Load(); calls != 1 {
		t.Fatalf("concurrent refresh calls = %d, want 1", calls)
	}
}

type failingExchangeRateFetcher struct {
	calls atomic.Int32
}

func (f *failingExchangeRateFetcher) fetch(context.Context) (exchangeRateProviderResponse, error) {
	f.calls.Add(1)
	return exchangeRateProviderResponse{}, errors.New("upstream unavailable")
}

func TestExchangeRateServiceCoalescesConcurrentFailures(t *testing.T) {
	fetcher := &failingExchangeRateFetcher{}
	now := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	service := &exchangeRateService{fetcher: fetcher, now: func() time.Time { return now }}

	const callers = 20
	results := make(chan error, callers)
	for i := 0; i < callers; i++ {
		go func() {
			_, err := service.latest()
			results <- err
		}()
	}
	for i := 0; i < callers; i++ {
		if err := <-results; err == nil || errorHTTPStatus(err) != http.StatusBadGateway {
			t.Fatalf("coalesced failure %d = %v, want 502", i, err)
		}
	}
	if calls := fetcher.calls.Load(); calls != 1 {
		t.Fatalf("concurrent failure calls = %d, want 1", calls)
	}
}
