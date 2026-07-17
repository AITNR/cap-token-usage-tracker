package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// EstimatedCost is calculated from one persisted request using the current model price.
type EstimatedCost struct {
	Priced                bool    `json:"priced"`
	Source                string  `json:"source,omitempty"`
	AccountingMode        string  `json:"accounting_mode,omitempty"`
	TierThreshold         uint64  `json:"tier_threshold,omitempty"`
	ContextTokens         uint64  `json:"context_tokens,omitempty"`
	BillableInputTokens   uint64  `json:"billable_input_tokens,omitempty"`
	BilledCacheReadTokens uint64  `json:"billed_cache_read_tokens,omitempty"`
	InputUSD              float64 `json:"input_usd"`
	OutputUSD             float64 `json:"output_usd"`
	CacheReadUSD          float64 `json:"cache_read_usd"`
	CacheCreationUSD      float64 `json:"cache_creation_usd"`
	TotalUSD              float64 `json:"total_usd"`
}

type CostAmounts struct {
	Requests         uint64  `json:"requests"`
	PricedRequests   uint64  `json:"priced_requests"`
	UnpricedRequests uint64  `json:"unpriced_requests"`
	InputUSD         float64 `json:"input_usd"`
	OutputUSD        float64 `json:"output_usd"`
	CacheReadUSD     float64 `json:"cache_read_usd"`
	CacheCreationUSD float64 `json:"cache_creation_usd"`
	TotalUSD         float64 `json:"total_usd"`
}

type ModelCostStats struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	CostAmounts
}

type CostSeriesPoint struct {
	Hour     string `json:"hour"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	CostAmounts
}

type MissingPriceStats struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Requests uint64 `json:"requests"`
}

type CostResponse struct {
	SchemaVersion     uint32              `json:"schema_version"`
	GeneratedAt       time.Time           `json:"generated_at"`
	Range             string              `json:"range"`
	Currency          string              `json:"currency"`
	EstimateBasis     string              `json:"estimate_basis"`
	PriceBookRevision uint64              `json:"price_book_revision"`
	Summary           CostAmounts         `json:"summary"`
	Models            []ModelCostStats    `json:"models"`
	Series            []CostSeriesPoint   `json:"series"`
	MissingPrices     []MissingPriceStats `json:"missing_prices"`
}

const maxCostCacheEntries = 16

type costQuerySnapshot struct {
	Range         string
	Cutoff        time.Time
	GeneratedAt   time.Time
	Prices        map[string]ModelPrice
	PriceRevision uint64
	HighWater     uint64
	Generation    uint64
}

type costCacheKey struct {
	Range         string
	CutoffUnix    int64
	PriceRevision uint64
	HighWater     uint64
	Generation    uint64
}

type costFlight struct {
	done     chan struct{}
	response CostResponse
	err      error
}

func (s *Store) QueryCosts(rangeName string) (CostResponse, error) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.closed {
		return CostResponse{}, errors.New("store is closed")
	}
	resp := make(chan costSnapshotResult, 1)
	s.commands <- costSnapshotCommand{rangeName: rangeName, resp: resp}
	result := <-resp
	if result.err != nil {
		return CostResponse{}, result.err
	}
	snapshot := result.snapshot
	key := costCacheKey{
		Range:         snapshot.Range,
		CutoffUnix:    snapshot.Cutoff.UnixNano(),
		PriceRevision: snapshot.PriceRevision,
		HighWater:     snapshot.HighWater,
		Generation:    snapshot.Generation,
	}

	s.costMu.Lock()
	if cached, ok := s.costCache[key]; ok {
		s.costMu.Unlock()
		return cloneCostResponse(cached), nil
	}
	if flight, ok := s.costFlights[key]; ok {
		s.costMu.Unlock()
		<-flight.done
		return cloneCostResponse(flight.response), flight.err
	}
	flight := &costFlight{done: make(chan struct{})}
	s.costFlights[key] = flight
	hook := s.costScanHook
	s.costMu.Unlock()

	if hook != nil {
		hook()
	}
	response, err := s.scanCosts(snapshot)

	s.costMu.Lock()
	flight.response = cloneCostResponse(response)
	flight.err = err
	if err == nil {
		if _, exists := s.costCache[key]; !exists {
			s.costOrder = append(s.costOrder, key)
		}
		s.costCache[key] = cloneCostResponse(response)
		for len(s.costOrder) > maxCostCacheEntries {
			oldest := s.costOrder[0]
			s.costOrder = s.costOrder[1:]
			delete(s.costCache, oldest)
		}
	}
	delete(s.costFlights, key)
	close(flight.done)
	s.costMu.Unlock()
	return response, err
}

func cloneCostResponse(input CostResponse) CostResponse {
	input.Models = append([]ModelCostStats(nil), input.Models...)
	input.Series = append([]CostSeriesPoint(nil), input.Series...)
	input.MissingPrices = append([]MissingPriceStats(nil), input.MissingPrices...)
	return input
}

func (s *Store) scanCosts(snapshot costQuerySnapshot) (CostResponse, error) {
	response := CostResponse{
		SchemaVersion:     1,
		GeneratedAt:       snapshot.GeneratedAt,
		Range:             snapshot.Range,
		Currency:          "USD",
		EstimateBasis:     "current_price_book",
		PriceBookRevision: snapshot.PriceRevision,
	}
	type modelKey struct{ Provider, Model string }
	type seriesKey struct {
		Hour            int64
		Provider, Model string
	}
	models := make(map[modelKey]CostAmounts)
	series := make(map[seriesKey]CostAmounts)
	missing := make(map[modelKey]uint64)

	err := s.db.View(func(tx *bolt.Tx) error {
		requests := tx.Bucket(requestsBucket)
		if requests == nil {
			return errors.New("requests bucket is missing")
		}
		cursor := requests.Cursor()
		key, value := cursor.First()
		if !snapshot.Cutoff.IsZero() {
			key, value = cursor.Seek(encodeRequestKey(snapshot.Cutoff.UnixNano(), 0))
		}
		for ; key != nil; key, value = cursor.Next() {
			if len(key) != 16 || value == nil {
				continue
			}
			requestedAt := time.Unix(0, decodeInt64(key[:8])).UTC()
			if !snapshot.Cutoff.IsZero() && requestedAt.Before(snapshot.Cutoff) {
				continue
			}
			var request RequestDetail
			if err := json.Unmarshal(value, &request); err != nil {
				return fmt.Errorf("decode request detail: %w", err)
			}
			if request.Sequence > snapshot.HighWater {
				continue
			}
			request.EstimatedCost = nil
			cost := estimateRequestCost(request, snapshot.Prices)
			provider := request.Provider
			model := request.Model
			if model == "" {
				model = "未标记模型"
			}
			mKey := modelKey{Provider: provider, Model: model}
			sKey := seriesKey{Hour: requestedAt.Truncate(time.Minute).Unix(), Provider: provider, Model: model}
			modelAmounts := models[mKey]
			seriesAmounts := series[sKey]
			addEstimatedCost(&response.Summary, cost)
			addEstimatedCost(&modelAmounts, cost)
			addEstimatedCost(&seriesAmounts, cost)
			models[mKey] = modelAmounts
			series[sKey] = seriesAmounts
			if !cost.Priced {
				missing[mKey]++
			}
		}
		return nil
	})
	if err != nil {
		return CostResponse{}, fmt.Errorf("query estimated costs: %w", err)
	}

	response.Models = make([]ModelCostStats, 0, len(models))
	for key, amounts := range models {
		response.Models = append(response.Models, ModelCostStats{Provider: key.Provider, Model: key.Model, CostAmounts: amounts})
	}
	sort.Slice(response.Models, func(i, j int) bool {
		if response.Models[i].TotalUSD != response.Models[j].TotalUSD {
			return response.Models[i].TotalUSD > response.Models[j].TotalUSD
		}
		if response.Models[i].Model != response.Models[j].Model {
			return response.Models[i].Model < response.Models[j].Model
		}
		return response.Models[i].Provider < response.Models[j].Provider
	})

	response.Series = make([]CostSeriesPoint, 0, len(series))
	for key, amounts := range series {
		response.Series = append(response.Series, CostSeriesPoint{
			Hour:        time.Unix(key.Hour, 0).UTC().Format(time.RFC3339),
			Provider:    key.Provider,
			Model:       key.Model,
			CostAmounts: amounts,
		})
	}
	sort.Slice(response.Series, func(i, j int) bool {
		if response.Series[i].Hour != response.Series[j].Hour {
			return response.Series[i].Hour < response.Series[j].Hour
		}
		if response.Series[i].Model != response.Series[j].Model {
			return response.Series[i].Model < response.Series[j].Model
		}
		return response.Series[i].Provider < response.Series[j].Provider
	})

	response.MissingPrices = make([]MissingPriceStats, 0, len(missing))
	for key, requests := range missing {
		response.MissingPrices = append(response.MissingPrices, MissingPriceStats{Provider: key.Provider, Model: key.Model, Requests: requests})
	}
	sort.Slice(response.MissingPrices, func(i, j int) bool {
		if response.MissingPrices[i].Requests != response.MissingPrices[j].Requests {
			return response.MissingPrices[i].Requests > response.MissingPrices[j].Requests
		}
		if response.MissingPrices[i].Model != response.MissingPrices[j].Model {
			return response.MissingPrices[i].Model < response.MissingPrices[j].Model
		}
		return response.MissingPrices[i].Provider < response.MissingPrices[j].Provider
	})
	return response, nil
}

func addEstimatedCost(amounts *CostAmounts, cost EstimatedCost) {
	amounts.Requests++
	if cost.Priced {
		amounts.PricedRequests++
	} else {
		amounts.UnpricedRequests++
	}
	amounts.InputUSD += cost.InputUSD
	amounts.OutputUSD += cost.OutputUSD
	amounts.CacheReadUSD += cost.CacheReadUSD
	amounts.CacheCreationUSD += cost.CacheCreationUSD
	amounts.TotalUSD += cost.TotalUSD
}

func estimateRequestCost(request RequestDetail, prices map[string]ModelPrice) EstimatedCost {
	model := request.Model
	if model == "" {
		model = "未标记模型"
	}
	price, ok := prices[model]
	if !ok {
		return EstimatedCost{}
	}

	cacheRead := request.CacheReadTokens
	if cacheRead == 0 {
		cacheRead = request.CachedTokens
	}
	cacheCreation := request.CacheCreationTokens
	mode := price.AccountingMode
	if mode == "" {
		mode = defaultAccountingMode(request.Provider, request.ExecutorType)
	}

	billableInput := request.InputTokens
	contextTokens := request.InputTokens
	if mode == accountingModeInputIncludesCache {
		cacheTokens := saturatingAdd(cacheRead, cacheCreation)
		if billableInput > cacheTokens {
			billableInput -= cacheTokens
		} else {
			billableInput = 0
		}
		contextTokens = saturatingAdd(billableInput, cacheTokens)
	} else {
		contextTokens = saturatingAdd(request.InputTokens, saturatingAdd(cacheRead, cacheCreation))
	}

	rates := price.tokenRates()
	var selectedThreshold uint64
	for _, tier := range price.ContextTiers {
		if contextTokens > tier.Threshold && tier.Threshold >= selectedThreshold {
			rates = tier.tokenRates()
			selectedThreshold = tier.Threshold
		}
	}

	result := EstimatedCost{
		Priced:                true,
		Source:                price.Source,
		AccountingMode:        mode,
		TierThreshold:         selectedThreshold,
		ContextTokens:         contextTokens,
		BillableInputTokens:   billableInput,
		BilledCacheReadTokens: cacheRead,
		InputUSD:              tokenCostUSD(billableInput, rates.Input),
		OutputUSD:             tokenCostUSD(request.OutputTokens, rates.Output),
		CacheReadUSD:          tokenCostUSD(cacheRead, rates.CacheRead),
		CacheCreationUSD:      tokenCostUSD(cacheCreation, rates.CacheCreation),
	}
	result.TotalUSD = result.InputUSD + result.OutputUSD + result.CacheReadUSD + result.CacheCreationUSD
	return result
}

func defaultAccountingMode(provider, executor string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	executor = strings.ToLower(strings.TrimSpace(executor))
	if provider == "anthropic" || executor == "claude" {
		return accountingModeInputExcludesCache
	}
	return accountingModeInputIncludesCache
}

func tokenCostUSD(tokens uint64, pricePerMillion float64) float64 {
	return float64(tokens) * pricePerMillion / 1_000_000
}
