package plugin

import (
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// modelCatalog lists the models the full-mode pricing editor may offer. It is
// derived from plugin-owned data only — the models recorded in usage statistics
// plus the models that already carry a price entry — so opening the editor and
// synchronizing prices from models.dev need nothing but a full-mode session.
type modelCatalog struct {
	Models      []string  `json:"models"`
	Observed    int       `json:"observed"`
	Priced      int       `json:"priced"`
	GeneratedAt time.Time `json:"generated_at"`
}

// validCatalogModel normalizes a model name and reports whether it may enter the
// catalog. The catalog feeds the price book and the models.dev matcher, so it
// applies the same name limits as a synchronization payload.
func validCatalogModel(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxDimensionRunes {
		return "", false
	}
	return name, true
}

func (r *pluginRuntime) modelCatalog() (modelCatalog, error) {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()
	if store == nil {
		return modelCatalog{}, withStatus(http.StatusServiceUnavailable, "plugin storage is not initialized")
	}
	observed, err := store.ObservedModels()
	if err != nil {
		return modelCatalog{}, err
	}
	priceBook, err := store.QueryPriceBook()
	if err != nil {
		return modelCatalog{}, err
	}

	catalog := modelCatalog{
		Models:      make([]string, 0, len(observed)+len(priceBook.Prices)),
		GeneratedAt: nowUTC(),
	}
	seen := make(map[string]struct{}, len(observed)+len(priceBook.Prices))
	// add reports whether name was newly admitted, so the per-source counters
	// describe how much each source contributed to this catalog.
	add := func(name string) bool {
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
		if len(catalog.Models) < maxModelPriceEntries {
			catalog.Models = append(catalog.Models, name)
		}
		return true
	}
	for _, raw := range observed {
		name, ok := validCatalogModel(raw)
		if !ok {
			continue
		}
		if add(name) {
			catalog.Observed++
		}
	}
	for raw := range priceBook.Prices {
		name, ok := validCatalogModel(raw)
		if !ok {
			continue
		}
		if add(name) {
			catalog.Priced++
		}
	}
	sort.Strings(catalog.Models)
	return catalog, nil
}

// fullModeModelCatalogResponse serves the session-protected catalog used by the
// full-mode price editor and by the models.dev synchronization fallback.
func (r *pluginRuntime) fullModeModelCatalogResponse() (pluginapi.ManagementResponse, error) {
	catalog, err := r.modelCatalog()
	if err != nil {
		return jsonResponse(errorHTTPStatus(err), map[string]any{"error": err.Error()}), nil
	}
	return jsonResponse(http.StatusOK, catalog), nil
}
