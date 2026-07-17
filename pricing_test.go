package main

import (
	"math"
	"testing"
)

func TestNormalizeModelPricesSupportsCacheAndContextTiers(t *testing.T) {
	prices, err := normalizeModelPrices(map[string]ModelPrice{
		" model ": {
			Input: 1, Output: 2, CacheRead: 0.1, CacheCreation: 1.25,
			ContextTiers: []ContextPriceTier{
				{Threshold: 200_000, Input: 2, Output: 3, CacheRead: 0.2, CacheCreation: 2.5},
				{Threshold: 32_000, Input: 1.5, Output: 2.5, CacheRead: 0.15, CacheCreation: 1.75},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	price := prices["model"]
	if price.Source != priceSourceManual || price.CacheRead != 0.1 || price.CacheCreation != 1.25 {
		t.Fatalf("normalized price = %+v", price)
	}
	if len(price.ContextTiers) != 2 || price.ContextTiers[0].Threshold != 32_000 || price.ContextTiers[1].Threshold != 200_000 {
		t.Fatalf("normalized tiers = %+v", price.ContextTiers)
	}
}

func TestNormalizeModelPricesRejectsInvalidTiersAndRates(t *testing.T) {
	for name, prices := range map[string]map[string]ModelPrice{
		"duplicate threshold": {
			"m": {ContextTiers: []ContextPriceTier{{Threshold: 10}, {Threshold: 10}}},
		},
		"zero threshold": {
			"m": {ContextTiers: []ContextPriceTier{{Threshold: 0}}},
		},
		"bad cache read": {
			"m": {CacheRead: -1},
		},
		"bad cache creation": {
			"m": {CacheCreation: math.Inf(1)},
		},
		"bad accounting mode": {
			"m": {AccountingMode: "guess"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeModelPrices(prices); err == nil {
				t.Fatalf("invalid prices accepted: %+v", prices)
			}
		})
	}
}

func TestCloneModelPricesDeepCopiesContextTiers(t *testing.T) {
	original := map[string]ModelPrice{
		"m": {
			Input:        1,
			ContextTiers: []ContextPriceTier{{Threshold: 100, Input: 2}},
		},
	}
	cloned := cloneModelPrices(original)
	clonedPrice := cloned["m"]
	clonedPrice.ContextTiers[0].Input = 99
	cloned["m"] = clonedPrice
	if original["m"].ContextTiers[0].Input != 2 {
		t.Fatalf("clone mutated original: %+v", original["m"])
	}
}

func TestNormalizeModelPricesKeepsFreeModels(t *testing.T) {
	prices, err := normalizeModelPrices(map[string]ModelPrice{
		"free-synced": {Source: priceSourceModelsDev},
		"free-manual": {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prices["free-synced"]; !ok {
		t.Fatal("synced free model was removed")
	}
	if price, ok := prices["free-manual"]; !ok || price.Source != priceSourceManual {
		t.Fatalf("manual free model = %+v, present=%v", price, ok)
	}
}
