package main

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	maxModelPriceEntries = 1000
	maxTokenPricePerM    = 1_000_000.0
)

// ModelPrice stores USD prices per one million input/output tokens.
type ModelPrice struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

type ModelPricesResponse struct {
	Prices map[string]ModelPrice `json:"prices"`
}

func normalizeModelPrices(input map[string]ModelPrice) (map[string]ModelPrice, error) {
	if len(input) > maxModelPriceEntries {
		return nil, fmt.Errorf("model prices must contain at most %d entries", maxModelPriceEntries)
	}
	result := make(map[string]ModelPrice, len(input))
	seen := make(map[string]string, len(input))
	for rawModel, price := range input {
		model := strings.TrimSpace(rawModel)
		if model == "" {
			return nil, fmt.Errorf("model price name must not be empty")
		}
		if !utf8.ValidString(model) || utf8.RuneCountInString(model) > maxDimensionRunes {
			return nil, fmt.Errorf("model price name %q is invalid or too long", model)
		}
		if previous, exists := seen[model]; exists {
			return nil, fmt.Errorf("model price names %q and %q normalize to the same model", previous, rawModel)
		}
		seen[model] = rawModel
		if err := validateTokenPrice(price.Input, model, "input"); err != nil {
			return nil, err
		}
		if err := validateTokenPrice(price.Output, model, "output"); err != nil {
			return nil, err
		}
		if price.Input == 0 && price.Output == 0 {
			continue
		}
		result[model] = price
	}
	return result, nil
}

func validateTokenPrice(value float64, model, kind string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxTokenPricePerM {
		return fmt.Errorf("%s price for model %q must be between 0 and %.0f USD per 1M tokens", kind, model, maxTokenPricePerM)
	}
	return nil
}

func cloneModelPrices(input map[string]ModelPrice) map[string]ModelPrice {
	result := make(map[string]ModelPrice, len(input))
	for model, price := range input {
		result[model] = price
	}
	return result
}
