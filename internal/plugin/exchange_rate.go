package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	exchangeRateURL             = "https://open.er-api.com/v6/latest/USD"
	exchangeRateRequestTimeout  = 8 * time.Second
	exchangeRateMaxResponseSize = 1 << 20
	exchangeRateFreshTTL        = time.Hour
	exchangeRateStaleTTL        = 24 * time.Hour
	exchangeRateRetryBackoff    = time.Minute
)

type exchangeRateFetcher struct {
	client *http.Client
	url    string
}

type exchangeRateSource interface {
	fetch(context.Context) (exchangeRateProviderResponse, error)
}

type exchangeRateRefreshResult struct {
	response ExchangeRateResponse
	err      error
}

// exchangeRateAttempt is one provider in a refresh run: the fetcher to call,
// the provider it belongs to, and its index in the configured chain.
type exchangeRateAttempt struct {
	fetcher  exchangeRateSource
	provider exchangeRateProvider
	index    int
}

type exchangeRateRefresh struct {
	done   chan struct{}
	result exchangeRateRefreshResult
}

type exchangeRateService struct {
	mu         sync.Mutex
	fetcher    exchangeRateSource
	providers  []exchangeRateProvider
	fetchers   []exchangeRateSource
	active     int
	headRetry  time.Time
	refresh    *exchangeRateRefresh
	now        func() time.Time
	cached     *ExchangeRateResponse
	freshUntil time.Time
	staleUntil time.Time
	retryAfter time.Time
	lastError  error
}

type exchangeRateProviderResponse struct {
	Result             string             `json:"result"`
	BaseCode           string             `json:"base_code"`
	TimeLastUpdateUnix int64              `json:"time_last_update_unix"`
	Rates              map[string]float64 `json:"rates"`
}

type ExchangeRateResponse struct {
	SchemaVersion uint32    `json:"schema_version"`
	Base          string    `json:"base"`
	Quote         string    `json:"quote"`
	Rate          float64   `json:"rate"`
	EffectiveAt   time.Time `json:"effective_at"`
	FetchedAt     time.Time `json:"fetched_at"`
	Source        string    `json:"source"`
	Stale         bool      `json:"stale"`
	Fallback      bool      `json:"fallback"`
}

func newExchangeRateFetcher() *exchangeRateFetcher {
	return &exchangeRateFetcher{client: newExchangeRateHTTPClient(exchangeRateURL), url: exchangeRateURL}
}

func newExchangeRateService(configured string) *exchangeRateService {
	providers := exchangeRateProviderChain(configured)
	fetchers := make([]exchangeRateSource, 0, len(providers))
	for _, provider := range providers {
		fetchers = append(fetchers, newExchangeRateFetcherFor(provider))
	}
	return &exchangeRateService{providers: providers, fetchers: fetchers, now: nowUTC}
}

func (s *exchangeRateService) latest() (ExchangeRateResponse, error) {
	if s == nil {
		return ExchangeRateResponse{}, withStatus(http.StatusServiceUnavailable, "exchange-rate service is unavailable")
	}

	s.mu.Lock()
	now := time.Now().UTC()
	if s.now != nil {
		now = s.now().UTC()
	}
	if s.cached != nil && now.Before(s.freshUntil) {
		result := *s.cached
		result.Stale = false
		s.mu.Unlock()
		return result, nil
	}
	if s.lastError != nil && now.Before(s.retryAfter) {
		if s.cached != nil && now.Before(s.staleUntil) {
			result := *s.cached
			result.Stale = true
			s.mu.Unlock()
			return result, nil
		}
		err := publicExchangeRateError(s.lastError)
		s.mu.Unlock()
		return ExchangeRateResponse{}, err
	}
	if s.refresh != nil {
		refresh := s.refresh
		s.mu.Unlock()
		<-refresh.done
		return refresh.result.response, refresh.result.err
	}

	// Build the attempt order while holding the lock. A failover sticks to the
	// source that worked, and the head of the chain is retried after
	// exchangeRateHeadRetry so a recovered network goes back to the preferred
	// source instead of staying on a lower-priority one forever.
	attempts := make([]exchangeRateAttempt, 0, len(s.fetchers)+1)
	if s.fetcher != nil {
		attempts = append(attempts, exchangeRateAttempt{fetcher: s.fetcher, provider: exchangeRateProvider{Source: openERAPISourceLabel}})
	} else if len(s.fetchers) == 0 {
		attempts = append(attempts, exchangeRateAttempt{fetcher: newExchangeRateFetcher(), provider: exchangeRateProvider{Source: openERAPISourceLabel}})
	} else {
		head := 0
		if s.active > 0 && s.active < len(s.fetchers) && now.Before(s.headRetry) {
			head = s.active
		}
		for offset := 0; offset < len(s.fetchers); offset++ {
			index := (head + offset) % len(s.fetchers)
			attempts = append(attempts, exchangeRateAttempt{fetcher: s.fetchers[index], provider: s.providers[index], index: index})
		}
	}
	refresh := &exchangeRateRefresh{done: make(chan struct{})}
	s.refresh = refresh
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), exchangeRateBudgetTimeout)
	defer cancel()
	var provider exchangeRateProviderResponse
	var err error
	used := 0
	for position, attempt := range attempts {
		attemptCtx, cancelAttempt := context.WithTimeout(ctx, exchangeRateAttemptTimeout)
		provider, err = attempt.fetcher.fetch(attemptCtx)
		cancelAttempt()
		if err == nil {
			used = position
			break
		}
		if ctx.Err() != nil {
			break
		}
	}

	s.mu.Lock()
	if err == nil {
		effectiveAt := now
		if provider.TimeLastUpdateUnix > 0 {
			effectiveAt = time.Unix(provider.TimeLastUpdateUnix, 0).UTC()
		}
		result := ExchangeRateResponse{
			SchemaVersion: 1,
			Base:          "USD",
			Quote:         "CNY",
			Rate:          provider.Rates["CNY"],
			EffectiveAt:   effectiveAt,
			FetchedAt:     now,
			Source:        attempts[used].provider.Source,
			Fallback:      used > 0,
		}
		s.cached = &result
		if used > 0 {
			// Keep a fallback briefly so the preferred source is retried soon,
			// and remember which source answered so the next refresh starts there.
			s.freshUntil = now.Add(exchangeRateFallbackTTL)
			s.active = attempts[used].index
			s.headRetry = now.Add(exchangeRateHeadRetry)
		} else {
			s.freshUntil = now.Add(exchangeRateFreshTTL)
			s.active = attempts[used].index
			if s.active == 0 {
				s.headRetry = time.Time{}
			}
		}
		s.staleUntil = now.Add(exchangeRateStaleTTL)
		s.retryAfter = time.Time{}
		s.lastError = nil
	} else {
		s.retryAfter = now.Add(exchangeRateRetryBackoff)
		s.lastError = err
	}
	var result exchangeRateRefreshResult
	if err == nil {
		result.response = *s.cached
		result.response.Stale = false
	} else if s.cached != nil && now.Before(s.staleUntil) {
		result.response = *s.cached
		result.response.Stale = true
	} else {
		result.err = publicExchangeRateError(err)
	}
	refresh.result = result
	s.refresh = nil
	s.mu.Unlock()
	close(refresh.done)
	return result.response, result.err
}

func (f *exchangeRateFetcher) fetch(ctx context.Context) (exchangeRateProviderResponse, error) {
	if f == nil || f.client == nil {
		return exchangeRateProviderResponse{}, errors.New("exchange-rate HTTP client is unavailable")
	}
	var result exchangeRateProviderResponse
	if err := fetchExchangeRateJSON(ctx, f.client, f.url, &result); err != nil {
		return exchangeRateProviderResponse{}, err
	}
	if !strings.EqualFold(result.Result, "success") || !strings.EqualFold(result.BaseCode, "USD") {
		return exchangeRateProviderResponse{}, errors.New("decode exchange rate: provider returned an unsuccessful or unexpected base response")
	}
	rate, ok := result.Rates["CNY"]
	if !ok || math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return exchangeRateProviderResponse{}, errors.New("decode exchange rate: missing or invalid CNY rate")
	}
	return result, nil
}

// fetchExchangeRateJSON performs the shared HTTP plumbing for every provider:
// same-origin redirects only, JSON content type, bounded size, and a single
// strict JSON value.
func fetchExchangeRateJSON(ctx context.Context, client *http.Client, rawURL string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create exchange-rate request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "cap-token-usage-tracker/"+version)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch exchange rate: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch exchange rate: HTTP %d", response.StatusCode)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("fetch exchange rate: expected application/json response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, exchangeRateMaxResponseSize+1))
	if err != nil {
		return fmt.Errorf("read exchange rate: %w", err)
	}
	if len(body) > exchangeRateMaxResponseSize {
		return fmt.Errorf("read exchange rate: response exceeds %d bytes", exchangeRateMaxResponseSize)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode exchange rate: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("decode exchange rate: trailing JSON value")
		}
		return fmt.Errorf("decode exchange rate: %w", err)
	}
	return nil
}

func publicExchangeRateError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return withStatus(http.StatusGatewayTimeout, "exchange-rate request timed out")
	}
	return withStatus(http.StatusBadGateway, "exchange-rate request failed")
}
