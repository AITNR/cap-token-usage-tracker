package plugin

import (
	"math"
	"testing"
	"time"
)

func nightPrice() ModelPrice {
	return ModelPrice{Input: 4, TimeZone: "Asia/Shanghai", TimeTiers: []TimePriceTier{{Name: "night", Days: []int{1}, Start: "23:00", End: "08:00", TokenRates: TokenRates{Input: 1, Output: 2, CacheRead: 0.5, CacheCreation: 0.25}}}}
}
func TestTimePricingBoundaries(t *testing.T) {
	price := nightPrice()
	for _, tc := range []struct {
		stamp string
		hit   bool
	}{
		{"2026-09-14T22:59:59+08:00", false}, {"2026-09-14T23:00:00+08:00", true},
		{"2026-09-15T07:59:59+08:00", true}, {"2026-09-15T08:00:00+08:00", false},
		{"2026-09-16T02:00:00+08:00", false},
	} {
		stamp, _ := time.Parse(time.RFC3339, tc.stamp)
		_, hit := selectTimePriceTier(price, stamp)
		if hit != tc.hit {
			t.Errorf("%s hit=%v", tc.stamp, hit)
		}
	}
	price.TimeTiers[0].Days = []int{7}
	stamp, _ := time.Parse(time.RFC3339, "2026-09-14T02:00:00+08:00")
	if _, hit := selectTimePriceTier(price, stamp); !hit {
		t.Fatal("Sunday overnight must include Monday morning")
	}
	price.TimeZone = "America/Los_Angeles"
	price.TimeTiers[0].Days = nil
	price.TimeTiers[0].Start = "01:00"
	price.TimeTiers[0].End = "02:00"
	for _, s := range []string{"2026-11-01T08:30:00Z", "2026-11-01T09:30:00Z"} {
		stamp, _ := time.Parse(time.RFC3339, s)
		if _, hit := selectTimePriceTier(price, stamp); !hit {
			t.Fatal("DST repeated hour", s)
		}
	}
}
func TestTimePricingValidation(t *testing.T) {
	for name, mutate := range map[string]func(*ModelPrice){
		"zone": func(p *ModelPrice) { p.TimeZone = "bad/zone" }, "local": func(p *ModelPrice) { p.TimeZone = "Local" },
		"clock": func(p *ModelPrice) { p.TimeTiers[0].Start = "24:00" }, "equal": func(p *ModelPrice) { p.TimeTiers[0].End = "23:00" },
		"day": func(p *ModelPrice) { p.TimeTiers[0].Days = []int{0} }, "duplicate day": func(p *ModelPrice) { p.TimeTiers[0].Days = []int{1, 1} },
		"rate": func(p *ModelPrice) { p.TimeTiers[0].Input = math.NaN() },
		"overlap": func(p *ModelPrice) {
			p.TimeTiers = append(p.TimeTiers, TimePriceTier{Name: "next", Days: []int{2}, Start: "07:00", End: "09:00"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := nightPrice()
			mutate(&p)
			if _, err := normalizeModelPrice("m", p); err == nil {
				t.Fatal("accepted invalid config")
			}
		})
	}
	p := nightPrice()
	p.TimeZone = ""
	normalized, err := normalizeModelPrice("m", p)
	if err != nil || normalized.TimeZone != "UTC" {
		t.Fatal(normalized, err)
	}
	copy := cloneModelPrices(map[string]ModelPrice{"m": p})
	v := copy["m"]
	v.TimeTiers[0].Days[0] = 2
	if p.TimeTiers[0].Days[0] != 1 || sameEditableModelPrice(p, v) {
		t.Fatal("copy/equality ignores days")
	}
}
func TestTimePricingCostPrecedence(t *testing.T) {
	p := nightPrice()
	p.ServiceTiers = map[string]ServiceTierPrice{"priority": {Input: 10, ContextTiers: []ContextPriceTier{{Threshold: 2000000, Input: 20}}}}
	stamp, _ := time.Parse(time.RFC3339, "2026-09-14T23:00:00+08:00")
	req := RequestDetail{Time: stamp, Dimensions: Dimensions{Model: "m", ServiceTier: "priority"}, Counters: Counters{InputTokens: 1000000}}
	cost := estimateRequestCost(req, map[string]ModelPrice{"m": p})
	if cost.TotalUSD != 1 || cost.PriceTimeTier != "night" {
		t.Fatal(cost)
	}
	req.InputTokens = 3000000
	cost = estimateRequestCost(req, map[string]ModelPrice{"m": p})
	if cost.TotalUSD != 60 || cost.PriceTimeTier != "" {
		t.Fatal(cost)
	}
	req.Time = stamp.Add(12 * time.Hour)
	req.InputTokens = 1000000
	cost = estimateRequestCost(req, map[string]ModelPrice{"m": p})
	if cost.TotalUSD != 10 {
		t.Fatal(cost)
	}
}
func TestTimePricingPersistenceSyncAndCache(t *testing.T) {
	config := testConfig(t)
	config.SyncOnRecord = true
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	now := time.Now().UTC()
	p := nightPrice()
	p.TimeZone = "UTC"
	p.TimeTiers[0].Days = nil
	// This one-minute window always contains the recorded instant, including midnight.
	p.TimeTiers[0].Start = now.Format("15:04")
	p.TimeTiers[0].End = now.Add(time.Minute).Format("15:04")
	book, err := store.SavePriceBook(map[string]ModelPrice{"m": p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(normalizedUsage{RequestedAt: now, Dimensions: Dimensions{Model: "m"}, Counters: Counters{Requests: 1, InputTokens: 1000000}}); err != nil {
		t.Fatal(err)
	}
	costs, err := store.QueryCosts("24h")
	if err != nil || costs.Summary.TotalUSD != 1 {
		t.Fatal(costs, err)
	}
	p.TimeTiers[0].Input = 2
	book, err = store.SavePriceBook(map[string]ModelPrice{"m": p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	costs, err = store.QueryCosts("24h")
	if err != nil || costs.Summary.TotalUSD != 2 {
		t.Fatal(costs, err)
	}
	book, err = store.ApplyModelPriceSync(map[string]ModelPrice{"m": {Input: 99, Source: priceSourceModelsDev}}, book.SyncSettings, PriceSyncMetadata{}, book.Revision)
	if err != nil || book.Prices["m"].TimeTiers[0].Input != 2 || book.LastSync.SkippedManual != 1 {
		t.Fatal(book, err)
	}
	backup, backupErr := store.Backup()
	if backupErr != nil {
		t.Fatal(backupErr)
	}
	if err := store.RestoreBackup(backup); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	book, err = store.QueryPriceBook()
	if err != nil || book.Prices["m"].TimeZone != "UTC" || len(book.Prices["m"].TimeTiers) != 1 {
		t.Fatal(book, err)
	}
}
