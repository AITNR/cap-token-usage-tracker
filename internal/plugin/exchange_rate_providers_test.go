package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type stubExchangeRateSource struct {
	response exchangeRateProviderResponse
	err      error
	calls    *int32
}

func (s stubExchangeRateSource) fetch(context.Context) (exchangeRateProviderResponse, error) {
	if s.calls != nil {
		atomic.AddInt32(s.calls, 1)
	}
	if s.err != nil {
		return exchangeRateProviderResponse{}, s.err
	}
	return s.response, nil
}

func okRate(rate float64) exchangeRateProviderResponse {
	return exchangeRateProviderResponse{
		Result:             "success",
		BaseCode:           "USD",
		TimeLastUpdateUnix: 1752667200,
		Rates:              map[string]float64{"CNY": rate},
	}
}

func TestExchangeRateProviderChainOrder(t *testing.T) {
	cases := map[string][]string{
		exchangeRateProviderAuto:      {exchangeRateProviderCFETS, exchangeRateProviderOpenERAPI, exchangeRateProviderAllRates},
		exchangeRateProviderCFETS:     {exchangeRateProviderCFETS, exchangeRateProviderOpenERAPI, exchangeRateProviderAllRates},
		exchangeRateProviderOpenERAPI: {exchangeRateProviderOpenERAPI, exchangeRateProviderCFETS, exchangeRateProviderAllRates},
		exchangeRateProviderAllRates:  {exchangeRateProviderAllRates, exchangeRateProviderOpenERAPI, exchangeRateProviderCFETS},
	}
	for configured, want := range cases {
		chain := exchangeRateProviderChain(configured)
		if len(chain) != len(want) {
			t.Fatalf("%s: chain length = %d", configured, len(chain))
		}
		for i, id := range want {
			if chain[i].ID != id {
				t.Fatalf("%s: chain[%d] = %s, want %s", configured, i, chain[i].ID, id)
			}
			if chain[i].URL == "" || chain[i].Source == "" {
				t.Fatalf("%s: chain[%d] missing url or source: %+v", configured, i, chain[i])
			}
		}
	}
}

func TestCFETSExchangeRateFetcherParsesCentralParity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"head":{"rep_code":"200"},"data":{"lastDate":"2026-11-25 9:15"},"records":[{"vrtCode":"2","vrtEName":"EUR/CNY","price":"7.8"},{"vrtCode":"1","vrtEName":"USD/CNY","price":"6.7088"}]}`))
	}))
	defer server.Close()

	result, err := (&cfetsExchangeRateFetcher{client: server.Client(), url: server.URL}).fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Rates["CNY"] != 6.7088 || result.BaseCode != "USD" || !strings.EqualFold(result.Result, "success") {
		t.Fatalf("CFETS response = %+v", result)
	}
	want := time.Date(2026, 11, 25, 9, 15, 0, 0, time.FixedZone("CST", 8*60*60)).Unix()
	if result.TimeLastUpdateUnix != want {
		t.Fatalf("effective unix = %d, want %d", result.TimeLastUpdateUnix, want)
	}
}

func TestCFETSExchangeRateFetcherRejectsInvalidPayloads(t *testing.T) {
	for name, fixture := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"missing record":  {status: http.StatusOK, body: `{"data":{},"records":[]}`, want: "missing CFETS USD/CNY record"},
		"invalid price":   {status: http.StatusOK, body: `{"records":[{"vrtEName":"USD/CNY","price":"n/a"}]}`, want: "invalid CFETS CNY rate"},
		"out of range":    {status: http.StatusOK, body: `{"records":[{"vrtEName":"USD/CNY","price":"999"}]}`, want: "invalid CFETS CNY rate"},
		"upstream error":  {status: http.StatusBadGateway, body: `{}`, want: "HTTP 502"},
		"trailing values": {status: http.StatusOK, body: `{"records":[]} {}`, want: "trailing JSON value"},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				response.Header().Set("Content-Type", "application/json")
				response.WriteHeader(fixture.status)
				_, _ = response.Write([]byte(fixture.body))
			}))
			defer server.Close()
			_, err := (&cfetsExchangeRateFetcher{client: server.Client(), url: server.URL}).fetch(context.Background())
			if err == nil || !strings.Contains(err.Error(), fixture.want) {
				t.Fatalf("error = %v, want %q", err, fixture.want)
			}
		})
	}
}

type stubExchangeRateSourceFunc func() (exchangeRateProviderResponse, error)

func (f stubExchangeRateSourceFunc) fetch(context.Context) (exchangeRateProviderResponse, error) {
	return f()
}

func newChainTestService(now *time.Time, sources ...exchangeRateSource) *exchangeRateService {
	providers := []exchangeRateProvider{
		{ID: exchangeRateProviderCFETS, URL: cfetsExchangeRateURL, Source: cfetsSourceLabel},
		{ID: exchangeRateProviderOpenERAPI, URL: exchangeRateURL, Source: openERAPISourceLabel},
		{ID: exchangeRateProviderAllRates, URL: allRatesTodayExchangeURL, Source: allRatesSourceLabel},
	}
	for len(sources) < len(providers) {
		sources = append(sources, stubExchangeRateSource{err: errors.New("unused provider")})
	}
	return &exchangeRateService{
		providers: providers,
		fetchers:  sources[:len(providers)],
		now:       func() time.Time { return *now },
	}
}

func TestExchangeRateServiceFailsOverAndMarksFallback(t *testing.T) {
	now := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	cfetsCalls, erCalls := int32(0), int32(0)
	service := newChainTestService(&now,
		stubExchangeRateSource{err: errors.New("cfets unreachable"), calls: &cfetsCalls},
		stubExchangeRateSource{response: okRate(6.68), calls: &erCalls},
		stubExchangeRateSource{response: okRate(6.66)},
	)

	result, err := service.latest()
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != openERAPISourceLabel || !result.Fallback {
		t.Fatalf("fallback result = %+v", result)
	}
	if result.Rate != 6.68 || result.Stale {
		t.Fatalf("fallback rate = %+v", result)
	}
	if cfetsCalls != 1 || erCalls != 1 {
		t.Fatalf("calls = cfets:%d er:%d", cfetsCalls, erCalls)
	}
	// The fallback is cached briefly and the working source is remembered.
	if !service.freshUntil.Equal(now.Add(exchangeRateFallbackTTL)) {
		t.Fatalf("freshUntil = %v", service.freshUntil)
	}
	if service.active != 1 || !service.headRetry.Equal(now.Add(exchangeRateHeadRetry)) {
		t.Fatalf("sticky state: active=%d headRetry=%v", service.active, service.headRetry)
	}

	now = now.Add(2 * time.Minute)
	result, err = service.latest()
	if err != nil || result.Source != openERAPISourceLabel || !result.Fallback {
		t.Fatalf("cached fallback = %+v, %v", result, err)
	}
	if erCalls != 1 {
		t.Fatalf("fallback result was not cached: er calls = %d", erCalls)
	}
}

func TestExchangeRateServiceRetriesHeadAfterFailover(t *testing.T) {
	now := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	cfetsUp := false
	service := newChainTestService(&now,
		stubExchangeRateSourceFunc(func() (exchangeRateProviderResponse, error) {
			if cfetsUp {
				return okRate(6.7088), nil
			}
			return exchangeRateProviderResponse{}, errors.New("cfets unreachable")
		}),
		stubExchangeRateSource{response: okRate(6.68)},
		stubExchangeRateSource{response: okRate(6.66)},
	)

	if _, err := service.latest(); err != nil {
		t.Fatal(err)
	}
	if service.active != 1 {
		t.Fatalf("active after failover = %d", service.active)
	}

	// Inside the retry window the working source is used directly.
	now = now.Add(15 * time.Minute)
	result, err := service.latest()
	if err != nil || result.Source != openERAPISourceLabel || result.Fallback {
		t.Fatalf("sticky source = %+v, %v", result, err)
	}

	// Once the head retry is due and CFETS is healthy again, the dashboard
	// returns to the mainland-preferred source.
	cfetsUp = true
	now = now.Add(2 * time.Hour)
	result, err = service.latest()
	if err != nil || result.Source != cfetsSourceLabel || result.Fallback {
		t.Fatalf("recovered source = %+v, %v", result, err)
	}
	if service.active != 0 || !service.headRetry.IsZero() {
		t.Fatalf("recovered sticky state: active=%d headRetry=%v", service.active, service.headRetry)
	}
}

func TestExchangeRateServiceFallsBackToAllRatesToday(t *testing.T) {
	now := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	service := newChainTestService(&now,
		stubExchangeRateSource{err: errors.New("cfets unreachable")},
		stubExchangeRateSource{err: errors.New("er-api unreachable")},
		stubExchangeRateSource{response: okRate(6.66)},
	)
	result, err := service.latest()
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != allRatesSourceLabel || !result.Fallback || result.Rate != 6.66 {
		t.Fatalf("last-resort result = %+v", result)
	}
}

func TestConfigValidatesExchangeRateProvider(t *testing.T) {
	config, err := parseConfig([]byte("exchange_rate_provider: cfets\n"))
	if err != nil || config.ExchangeRateProvider != exchangeRateProviderCFETS {
		t.Fatalf("cfets config = %+v, %v", config, err)
	}
	config, err = parseConfig([]byte("exchange_rate_provider: \"\"\n"))
	if err != nil || config.ExchangeRateProvider != exchangeRateProviderAuto {
		t.Fatalf("empty provider must fall back to auto: %+v, %v", config, err)
	}
	config, err = parseConfig(nil)
	if err != nil || config.ExchangeRateProvider != exchangeRateProviderAuto {
		t.Fatalf("default provider = %+v, %v", config, err)
	}
	if _, err := parseConfig([]byte("exchange_rate_provider: bogus\n")); err == nil || !strings.Contains(err.Error(), "exchange_rate_provider") {
		t.Fatalf("unknown provider error = %v", err)
	}
}
