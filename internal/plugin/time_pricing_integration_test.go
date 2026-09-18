package plugin

import (
	"encoding/json"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"net/http"
	"testing"
)

func TestTimePricingSaveAPI(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runtime := &pluginRuntime{store: store}
	for _, valid := range []bool{true, false} {
		p := nightPrice()
		if !valid {
			p.TimeZone = "invalid/zone"
		}
		body, _ := json.Marshal(map[string]any{"prices": map[string]ModelPrice{"m": p}})
		response, err := runtime.savePricesResponse(pluginapi.ManagementRequest{Body: body, Headers: http.Header{"Content-Type": []string{"application/json"}}})
		want := http.StatusOK
		if !valid {
			want = http.StatusBadRequest
		}
		if err != nil || response.StatusCode != want {
			t.Fatalf("status=%d want=%d err=%v body=%s", response.StatusCode, want, err, response.Body)
		}
	}
	book, err := store.QueryPriceBook()
	if err != nil || book.Prices["m"].TimeZone != "Asia/Shanghai" {
		t.Fatal(book, err)
	}
}
func TestTimePricingSyncPreservesCatalogSchedule(t *testing.T) {
	store, err := openStore(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	book, err := store.QueryPriceBook()
	if err != nil {
		t.Fatal(err)
	}
	p := nightPrice()
	p.Source = priceSourceModelsDev
	book, err = store.ApplyModelPriceSync(map[string]ModelPrice{"m": p}, book.SyncSettings, PriceSyncMetadata{}, book.Revision)
	if err != nil {
		t.Fatal(err)
	}
	book, err = store.ApplyModelPriceSync(map[string]ModelPrice{"m": {Input: 9, Source: priceSourceModelsDev}}, book.SyncSettings, PriceSyncMetadata{}, book.Revision)
	if err != nil || book.Prices["m"].Input != 9 || len(book.Prices["m"].TimeTiers) != 1 || book.Prices["m"].TimeZone != "Asia/Shanghai" {
		t.Fatal(book, err)
	}
}
