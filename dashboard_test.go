package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDashboardUsesBoundedSafeRendering(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"document.createDocumentFragment()",
		"AbortController",
		"setTimeout(function(){controller.abort();},timeout)",
		"series.length>240",
		"body.replaceChildren(fragment)",
		"svg.replaceChildren(fragment)",
		"var resourceBase='/v0/resource/plugins/'",
		"var statsURL=resourceBase+'/stats'",
		"load(true).catch(function(error)",
		"resetKeyInput.value=''",
		"resetDialog.showModal()",
		"window.parent.document.documentElement",
		"new MutationObserver",
		"attributeFilter:['data-theme','style','class','lang']",
		"initializeThemeSync()",
		"window.matchMedia",
		"supportedLocales=['en','zh-CN','zh-TW','ru']",
		"function detectLocale()",
		"function normalizeLocale(value)",
		"navigator.languages",
		"window.addEventListener('languagechange'",
		"document.documentElement.lang=locale",
		"formatterLocale=locale==='zh-CN'?'zh-CN':locale==='zh-TW'?'zh-TW':locale==='ru'?'ru-RU':'en-US'",
		"function translateStatic()",
		"function localeNumber(value,options)",
		"function localeDate(value,options)",
		"<html lang=\"zh-CN\" data-theme=\"dark\" style=\"background:#151412;color-scheme:dark\">",
		"<meta name=\"color-scheme\" content=\"dark light\">",
		"<style id=\"initial-theme\">",
		"html{background:#151412;color-scheme:dark}",
		"html:not([data-theme]){background:#faf9f5;color-scheme:light}",
		"html[data-theme='white']{background:#fff;color-scheme:light}",
		"html[data-theme='dark']{background:#151412;color-scheme:dark}",
		"var theme='dark',background='#151412';",
		"getComputedStyle(parentRoot).getPropertyValue('--bg-secondary')",
		"window.frameElement.style.backgroundColor=background",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
	for _, forbidden := range []string{
		`updated_at:base.updated_at||''`,
		"replaceChildren.apply",
		"Math.max.apply",
		"localStorage",
		"sessionStorage",
		"data-theme-value",
		"themePopover",
		"connectButton",
		"logoutButton",
		"innerHTML",
		"column-hide-button",
		"data-hide-column",
		"data-hide-dimension-column",
		"row.hidden=true",
		`preserveAspectRatio="none"`,
		"fetch('stats')",
		`fetch("stats")`,
		`costFor(name,input,output)`,
		`fetch('https://models.dev`,
		`fetch("https://models.dev`,
		`fetch('https://open.er-api.com`,
		`fetch("https://open.er-api.com`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard contains unsafe pattern %q", forbidden)
		}
	}
}

func TestDashboardColumnMenusCanOverflowShortTables(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.panel{overflow:hidden}`,
		`.table-panel{overflow:visible}`,
		`.table-wrap{max-height:540px;overflow:auto;border-radius:0 0 12px 12px;scrollbar-gutter:stable}`,
		`.request-columns-menu{position:absolute`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing short-table column menu fix %q", required)
		}
	}
}

func TestDashboardIncludesInteractiveAnalyticsFeatures(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`id="granularity"`,
		`id="totalCost"`,
		`id="tokenUnitButton"`,
		`id="currencyButton"`,
		`var exchangeRateURL=resourceBase+'/exchange-rate'`,
		`function formatTokenTotal(value)`,
		`function toggleTokenUnit()`,
		`async function toggleCurrency()`,
		`id="topModel"`,
		`id="donut"`,
		`id="legend"`,
		`bar-input`,
		`bar-output`,
		`bar-cache-read`,
		`cache-hit-line`,
		`stroke-dasharray:7 5`,
		`function cacheReadTokens(point)`,
		`function cacheHitRate(input,cacheRead)`,
		`bucket.cacheRead+=cacheReadTokens(point)`,
		`item.cacheHitRate=cacheHitRate(item.input,item.cacheRead)`,
		`p.input+p.output+p.cacheRead`,
		`t('trend.cacheHitRate')`,
		`model_series`,
		`function selectModel(name)`,
		`function toggleModel(name)`,
		`addEventListener('wheel'`,
		`id="pricingDialog"`,
		`id="pricingKeyInput"`,
		`id="cliModelsKeyInput"`,
		`id="loadCLIModels"`,
		`id="manualModelInput"`,
		`id="addManualModel"`,
		`manualDraftModels=new Set()`,
		`function addManualModel()`,
		`function rerenderPricingEditor(excludedName)`,
		`manualDraftModels.has(name)||input>0`,
		`if(base.updated_at)value.updated_at=base.updated_at`,
		`manualDraftModels.clear()`,
		`var modelsURL='/v1/models'`,
		`function normalizeCLIModels(payload)`,
		`async function fetchCLIModels(renderEditor)`,
		`cliModelsPromise=api(modelsURL`,
		`moneyFormatters[key]`,
		`var pricesURL=resourceBase+'/prices'`,
		`var costsURL=resourceBase+'/costs'`,
		`var savePricesURL=managementBase+'/prices'`,
		`var syncPricesURL=managementBase+'/prices/sync'`,
		`function applyPrices(values)`,
		`function aggregateCostSeries()`,
		`function visibleCostSummary()`,
		`async function savePricing()`,
		`async function syncPricing()`,
		`price-cache-read`,
		`price-cache-creation`,
		`context-tier-controls`,
		`add-context-tier`,
		`remove-context-tier`,
		`remove-model-price`,
		`pending-delete`,
		`pendingDeletedPrices=new Set()`,
		`button.textContent=deleted?'撤销删除':'删除价格'`,
		`setPriceDeletedState(row,!pendingDeletedPrices.has(name))`,
		`if(pendingDeletedPrices.has(row.dataset.model))return`,
		`clearCLIModelState()`,
		`id="providerPriority"`,
		`id="ignoredSuffixes"`,
		`id="syncMappings"`,
		`id="syncPrices"`,
		`id="costCoverage"`,
		`id="priceCoverageStatus"`,
		`id="missingPriceStatus"`,
		`id="lastSyncStatus"`,
		`item.estimated_cost`,
		`record.estimated_cost`,
		`estimated.input_usd`,
		`estimated.output_usd`,
		`estimated.cache_read_usd`,
		`estimated.cache_creation_usd`,
		`estimated.total_usd`,
		`sync_settings:settings`,
		`模型目录直接读取 CLIProxyAPI /v1/models`,
		`async function exportCSV()`,
		`function exportPNG()`,
		`该时间段内暂无调用记录`,
		`grid-template-columns:repeat(4`,
		`grid-template-columns:repeat(2`,
		`<option value="minute" data-i18n="granularity.minute">`,
		`<option value="hour" selected data-i18n="granularity.hour">`,
		`id="costChart"`,
		`function renderCostTrend()`,
		`id="efficiencyChart"`,
		`function renderEfficiency()`,
		`function chartMetrics(svg,fallbackHeight)`,
		`function initializeChartResize()`,
		`new ResizeObserver`,
		`svg.setAttribute('viewBox','0 0 '+width+' '+height)`,
		`.bar-hit:focus-visible`,
		`.line-hit:focus-visible,.scatter-point:focus-visible`,
		`Math.floor(plotW/90)`,
		`Math.floor(plotW/85)`,
		`id="requestRows"`,
		`var requestsURL=resourceBase+'/requests'`,
		`async function loadRequests()`,
		`id="requestPrev"`,
		`id="requestNext"`,
		`id="requestPageSize"`,
		`requestLimit=Math.max(1,Math.min(500`,
		`id="requestColumnsButton"`,
		`id="requestColumnsMenu"`,
		`id="requestHeaders"`,
		`function requestTime(value)`,
		`second:'2-digit'`,
		`var requestColumns=[`,
		`sortButton.dataset.requestSort=column.key`,
		`function sortedRequestItems(items)`,
		`hiddenRequestColumns=new Set()`,
		`id="dimensionPrev"`,
		`id="dimensionNext"`,
		`id="dimensionPageSize"`,
		`dimensionLimit=Math.max(1,Math.min(500`,
		`id="dimensionColumnsButton"`,
		`id="dimensionColumnsMenu"`,
		`id="dimensionHeaders"`,
		`var dimensionColumns=[`,
		`sortButton.dataset.dimensionSort=column.key`,
		`function sortedDimensionGroups(groups)`,
		`hiddenDimensionColumns=new Set()`,
		`var preferencesURL=resourceBase+'/preferences'`,
		`function dashboardPreferencesPayload()`,
		`function applyDashboardPreferences(value)`,
		`async function loadDashboardPreferences()`,
		`function dashboardPreferencesSaveURL()`,
		`async function saveDashboardPreferences()`,
		`function scheduleDashboardPreferencesSave()`,
		`hidden_request_columns:Array.from(hiddenRequestColumns)`,
		`hidden_dimension_columns:Array.from(hiddenDimensionColumns)`,
		`params.set('save','1')`,
		`params.append('hidden_request_column',key)`,
		`params.append('hidden_dimension_column',key)`,
		`keepalive:true`,
		`window.addEventListener('pagehide'`,
		`loadDashboardPreferences().catch(function(error)`,
		`sorted.slice(dimensionOffset,dimensionOffset+dimensionLimit)`,
		`empty.colSpan=Math.max(1,columns.length)`,
		`function zoomTrend(factor,anchorRatio)`,
		`{passive:false,capture:true}`,
		`生成时间`,
		`缓存命中`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing analytics feature %q", required)
		}
	}
}

func TestDashboardUsesExactBackendCostsAndPricingSync(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`var costsURL=resourceBase+'/costs'`,
		`var syncPricesURL=managementBase+'/prices/sync'`,
		`api(costsURL+'?range='`,
		`currentCosts.models`,
		`currentCosts.series`,
		`price_book_revision`,
		`priced_requests`,
		`unpriced_requests`,
		`input_usd`,
		`output_usd`,
		`cache_read_usd`,
		`cache_creation_usd`,
		`total_usd`,
		`estimated_cost`,
		`accounting_mode`,
		`tier_threshold`,
		`context_tiers`,
		`provider_priority`,
		`ignored_suffixes`,
		`mappings`,
		`last_sync`,
		`source:'models.dev'`,
		`body:JSON.stringify({prices:next,sync_settings:settings})`,
		`body:JSON.stringify({source:'models.dev',models:models,sync_settings:settings})`,
		`displayCurrency==='CNY'`,
		`value*Number(exchangeRate.rate||0)`,
		`label.textContent=money(value)`,
		`formatTokenTotal(summary.total_tokens)`,
		`renderVisuals();await loadRequests();return responses`,
		`pricingDialog.addEventListener('close',function(){pricingKeyInput.value='';clearCLIModelState();clearPricingDraft();})`,
		`价格覆盖`,
		`未定价`,
		`同步中`,
		`同步失败`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing exact-cost/pricing contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`costFor(name,input,output)`,
		`costFor(`,
		`localStorage`,
		`sessionStorage`,
		`fetch('https://models.dev`,
		`fetch("https://models.dev`,
		`fetch('https://open.er-api.com`,
		`fetch("https://open.er-api.com`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard contains forbidden pricing pattern %q", forbidden)
		}
	}
}

func TestDashboardPaintsDarkBeforeRunningThemeSync(t *testing.T) {
	html := dashboardHTML
	rootStart := strings.Index(html, `<html lang="zh-CN" data-theme="dark" style="background:#151412;color-scheme:dark">`)
	initialStyle := strings.Index(html, `<style id="initial-theme">`)
	initialScript := strings.Index(html, `<script>`)
	if rootStart < 0 || initialStyle < 0 || initialScript < 0 || rootStart > initialStyle || initialStyle > initialScript {
		t.Fatal("dark root background and initial stylesheet must be available before theme sync script runs")
	}
}

func TestDashboardSynchronizesHostFrameBackground(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"getComputedStyle(parentRoot).getPropertyValue('--bg-secondary')",
		"root.style.backgroundColor=background",
		"window.frameElement.style.backgroundColor=background",
		"window.frameElement.parentElement.style.backgroundColor=background",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing host background sync %q", required)
		}
	}
}

func TestDashboardResponseHeaders(t *testing.T) {
	response := dashboardResponse()
	if response.Headers.Get("Cache-Control") != "no-store" {
		t.Fatal("missing no-store")
	}
	if response.Headers.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("missing referrer policy")
	}
	csp := response.Headers.Get("Content-Security-Policy")
	for _, directive := range []string{"default-src 'none'", "connect-src 'self'", "base-uri 'none'", "form-action 'none'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("CSP missing %q: %s", directive, csp)
		}
	}
}

func TestDashboardDoesNotServerRenderUsageValues(t *testing.T) {
	malicious := `</td><script>alert(1)</script>`
	if strings.Contains(dashboardHTML, malicious) {
		t.Fatal("dashboard unexpectedly embeds usage fixture")
	}
	if !strings.Contains(dashboardHTML, "td.textContent=value") {
		t.Fatal("usage cells are not rendered with textContent")
	}
}

func TestDashboardLocalesCatalog(t *testing.T) {
	// All four locale codes must be embedded in the HTML.
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		if !strings.Contains(dashboardHTML, `"`+code+`"`) {
			t.Fatalf("dashboardHTML missing locale code %q", code)
		}
	}

	// The embedded JSON blob must decode to a map containing all four locales
	// with the required base keys.
	const marker = "/*LOCALE_PLACEHOLDER*/"
	if strings.Contains(dashboardHTML, marker) {
		t.Fatal("dashboardHTML still contains unresolved locale placeholder")
	}

	// Verify each locale file individually via the embed FS.
	requiredKeys := []string{
		"app.title",
		"button.refresh",
		"button.reset",
		"status.loading",
		"chart.noCalls",
		"trend.cacheHitRate",
		"empty.calls",
		"requestColumns.button",
		"requestColumns.title",
		"requestColumns.showAll",
		"requestColumns.hide",
		"pagination.rowsPerPage",
		"sort.ascending",
		"sort.descending",
		"model.untitled",
		"pricing.title",
		"button.addManualModel",
		"pricing.manualModel",
		"pricing.manualModelPlaceholder",
		"pricing.manualModelHint",
		"pricing.manualDraftSource",
		"error.missingManualModel",
		"error.longManualModel",
		"error.duplicateManualModel",
		"error.tooManyManualModels",
	}
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatalf("locale %s: %v", code, err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("locale %s invalid JSON: %v", code, err)
		}
		for _, key := range requiredKeys {
			if _, ok := m[key]; !ok {
				t.Fatalf("locale %s missing required key %q", code, key)
			}
		}
	}

	// Verify the locale runtime is wired: translateStatic must be called
	// during initialisation (after theme and chart resize setup).
	initSeq := "initializeThemeSync();initializeChartResize();translateStatic();"
	if !strings.Contains(dashboardHTML, initSeq) {
		t.Fatalf("dashboardHTML missing init sequence %q", initSeq)
	}
}
