package plugin

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// exchangeRateProviderAuto picks the source by reachability: a host that can
	// reach CFETS is treated as a mainland-China deployment and served the
	// official central-parity rate, anything else gets the market feed.
	exchangeRateProviderAuto      = "auto"
	exchangeRateProviderCFETS     = "cfets"
	exchangeRateProviderOpenERAPI = "open-er-api"
	exchangeRateProviderAllRates  = "allratestoday"

	cfetsExchangeRateURL     = "https://www.chinamoney.com.cn/r/cms/www/chinamoney/data/fx/ccpr.json"
	allRatesTodayExchangeURL = "https://allratestoday.com/api/open/latest/USD"

	// exchangeRateAttemptTimeout bounds a single provider. CFETS answers in
	// ~0.1s from mainland China, so a host that cannot reach it inside this
	// window is treated as overseas and the next provider is tried.
	exchangeRateAttemptTimeout = 4 * time.Second
	// exchangeRateBudgetTimeout bounds one whole refresh across all providers.
	exchangeRateBudgetTimeout = 10 * time.Second
	// exchangeRateFallbackTTL keeps a fallback result briefly so the preferred
	// provider is retried soon without hammering the fallback upstream.
	exchangeRateFallbackTTL = 10 * time.Minute
	// exchangeRateHeadRetry decides when to retry the head of the chain after a
	// failover, so a recovered network returns to the preferred source.
	exchangeRateHeadRetry = 30 * time.Minute

	cfetsSourceLabel     = "CFETS"
	openERAPISourceLabel = "open.er-api.com"
	allRatesSourceLabel  = "AllRatesToday"
)

// exchangeRateProvider describes one rate source: where to fetch it and how it
// should be attributed in the dashboard.
type exchangeRateProvider struct {
	ID     string
	URL    string
	Source string
}

// exchangeRateProviderChain returns the attempt order for the configured mode.
// The first entry is the preferred source; the rest are the fallbacks in
// descending preference.
func exchangeRateProviderChain(configured string) []exchangeRateProvider {
	cfets := exchangeRateProvider{ID: exchangeRateProviderCFETS, URL: cfetsExchangeRateURL, Source: cfetsSourceLabel}
	erapi := exchangeRateProvider{ID: exchangeRateProviderOpenERAPI, URL: exchangeRateURL, Source: openERAPISourceLabel}
	allrates := exchangeRateProvider{ID: exchangeRateProviderAllRates, URL: allRatesTodayExchangeURL, Source: allRatesSourceLabel}
	switch configured {
	case exchangeRateProviderCFETS:
		return []exchangeRateProvider{cfets, erapi, allrates}
	case exchangeRateProviderOpenERAPI:
		return []exchangeRateProvider{erapi, cfets, allrates}
	case exchangeRateProviderAllRates:
		return []exchangeRateProvider{allrates, erapi, cfets}
	default:
		return []exchangeRateProvider{cfets, erapi, allrates}
	}
}

func isKnownExchangeRateProvider(value string) bool {
	switch value {
	case exchangeRateProviderAuto, exchangeRateProviderCFETS, exchangeRateProviderOpenERAPI, exchangeRateProviderAllRates:
		return true
	}
	return false
}

func newExchangeRateFetcherFor(provider exchangeRateProvider) exchangeRateSource {
	client := newExchangeRateHTTPClient(provider.URL)
	if provider.ID == exchangeRateProviderCFETS {
		return &cfetsExchangeRateFetcher{client: client, url: provider.URL}
	}
	return &exchangeRateFetcher{client: client, url: provider.URL}
}

// newExchangeRateHTTPClient builds a client that keeps redirects on the
// provider's own HTTPS host, so a misconfigured or compromised source cannot
// bounce the request somewhere else.
func newExchangeRateHTTPClient(rawURL string) *http.Client {
	host := ""
	if expected, err := url.Parse(rawURL); err == nil && expected != nil {
		host = expected.Host
	}
	return &http.Client{
		Timeout: exchangeRateRequestTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("exchange-rate provider redirected too many times")
			}
			if request.URL.Scheme != "https" || !strings.EqualFold(request.URL.Host, host) {
				return errors.New("exchange-rate redirect must remain on the configured HTTPS host")
			}
			return nil
		},
	}
}

// cfetsExchangeRateFetcher reads the central-parity table published by CFETS.
// The endpoint is the JSON the chinamoney.com.cn pages load themselves and it
// carries every quoted pair, so only the USD/CNY row is used.
type cfetsExchangeRateFetcher struct {
	client *http.Client
	url    string
}

type cfetsExchangeRateResponse struct {
	Data struct {
		LastDate string `json:"lastDate"`
	} `json:"data"`
	Records []struct {
		Code  string `json:"vrtCode"`
		Name  string `json:"vrtEName"`
		Price string `json:"price"`
	} `json:"records"`
}

func (f *cfetsExchangeRateFetcher) fetch(ctx context.Context) (exchangeRateProviderResponse, error) {
	if f == nil || f.client == nil {
		return exchangeRateProviderResponse{}, errors.New("exchange-rate HTTP client is unavailable")
	}
	var payload cfetsExchangeRateResponse
	if err := fetchExchangeRateJSON(ctx, f.client, f.url, &payload); err != nil {
		return exchangeRateProviderResponse{}, err
	}
	for _, record := range payload.Records {
		if !strings.EqualFold(strings.TrimSpace(record.Name), "USD/CNY") && strings.TrimSpace(record.Code) != "1" {
			continue
		}
		rate, err := strconv.ParseFloat(strings.TrimSpace(record.Price), 64)
		if err != nil || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 100 {
			return exchangeRateProviderResponse{}, errors.New("decode exchange rate: invalid CFETS CNY rate")
		}
		return exchangeRateProviderResponse{
			Result:             "success",
			BaseCode:           "USD",
			TimeLastUpdateUnix: cfetsEffectiveUnix(payload.Data.LastDate),
			Rates:              map[string]float64{"CNY": rate},
		}, nil
	}
	return exchangeRateProviderResponse{}, errors.New("decode exchange rate: missing CFETS USD/CNY record")
}

// cfetsEffectiveUnix converts the published date (for example "2026-11-25 9:15")
// into a timestamp. CFETS publishes in Asia/Shanghai and the hour is not
// zero-padded, so both layouts are accepted; an unparsable date only loses the
// effective timestamp, not the rate.
func cfetsEffectiveUnix(raw string) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	zone := time.FixedZone("CST", 8*60*60)
	for _, layout := range []string{"2006-01-02 15:04", "2006-01-02 3:04"} {
		if parsed, err := time.ParseInLocation(layout, raw, zone); err == nil {
			return parsed.Unix()
		}
	}
	return 0
}
