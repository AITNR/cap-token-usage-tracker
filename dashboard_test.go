package main

import (
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
		"attributeFilter:['data-theme','style','class']",
		"initializeThemeSync()",
		"window.matchMedia",
		"<html lang=\"en\" data-theme=\"dark\" style=\"background:#151412;color-scheme:dark\">",
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
		"replaceChildren.apply",
		"Math.max.apply",
		"localStorage",
		"sessionStorage",
		"data-theme-value",
		"themePopover",
		"connectButton",
		"logoutButton",
		"innerHTML",
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
		`model_series`,
		`function selectModel(name)`,
		`function toggleModel(name)`,
		`addEventListener('wheel'`,
		`id="pricingDialog"`,
		`id="pricingKeyInput"`,
		`id="cliModelsKeyInput"`,
		`id="loadCLIModels"`,
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
		`button.textContent=deleted?t('js.undo_delete'):t('js.delete_price')`,
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
		`The model catalog is loaded directly from CLIProxyAPI /v1/models`,
		`async function exportCSV()`,
		`function exportPNG()`,
		`js.no_usage`,
		`grid-template-columns:repeat(4`,
		`grid-template-columns:repeat(2`,
		`<option value="minute">By minute</option>`,
		`<option value="hour" selected>By hour</option>`,
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
		`function zoomTrend(factor,anchorRatio)`,
		`{passive:false,capture:true}`,
		`Generation`,
		`Cache hit`,
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
		`Price coverage`,
		`Unpriced`,
		`Syncing`,
		`Sync failed`,
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
	rootStart := strings.Index(html, `<html lang="en" data-theme="dark" style="background:#151412;color-scheme:dark">`)
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

func TestDashboardHeaderKeepsHostClearanceAndControlGroups(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`--host-overlay-safe-inset`,
		`class="control-group control-filters"`,
		`class="control-group control-actions"`,
		`flex-wrap:nowrap`,
		`.control-actions{flex:0 0 auto;margin-left:auto}`,
		`@media(max-width:820px){.heading{padding-right:clamp(72px,12vw,112px)}`,
		`<label class="language-control">`,
		`<span class="language-label">Language</span>`,
		`<option value="auto" selected>Automatic (browser)</option>`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing header layout contract %q", required)
		}
	}
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	for _, required := range []string{
		`<span class="language-label">语言</span>`,
		`<option value="auto">自动（跟随浏览器）</option>`,
	} {
		if !strings.Contains(zh, required) {
			t.Fatalf("chinese dashboard missing localized language control %q", required)
		}
	}
}

func TestDashboardInternationalization(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`id="languageSelect"`,
		`function t(key,vars)`,
		`function applyLanguagePreference(next)`,
		`function requestResult(item)`,
		`window.__I18N__`,
		`<title>Token Usage Analytics</title>`,
		`<option value="hour" selected>By hour</option>`,
		`Model prices`,
		`t('js.no_usage')`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing i18n feature %q", required)
		}
	}
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	for _, required := range []string{
		`<html lang="zh-CN"`,
		`<title>Token 用量统计</title>`,
		`用量分析`,
		`<option value="hour" selected>按小时</option>`,
		`>模型价格</span>`,
	} {
		if !strings.Contains(zh, required) {
			t.Fatalf("chinese dashboard missing %q", required)
		}
	}
	en := renderDashboardHTML(LanguageEnglish, LanguageEnglish)
	if !strings.Contains(en, `<title>Token Usage Analytics</title>`) {
		t.Fatal("english dashboard missing english title")
	}
	if strings.Contains(en, `<title>Token 用量统计</title>`) {
		t.Fatal("english dashboard shell title is chinese")
	}
}

func TestChineseDashboardCoversVisibleAndExportLabels(t *testing.T) {
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	for _, expected := range []string{
		`<div class="series-key"><span><i style="background:var(--input-color)"></i>输入</span><span><i style="background:var(--output-color)"></i>输出</span></div>`,
		`placeholder="API 密钥"`,
		`placeholder="管理密钥"`,
		`<th>层级</th>`,
		`aria-label="仪表盘控件"`,
	} {
		if !strings.Contains(zh, expected) {
			t.Fatalf("chinese dashboard missing localized contract %q", expected)
		}
	}
	for key, expected := range map[string]string{
		"js.token_summary_detail":    "输入 {input} · 输出 {output}",
		"js.price_input":             "输入",
		"js.price_output":            "输出",
		"js.price_cache_read":        "缓存读取",
		"js.price_cache_creation":    "缓存创建",
		"js.csv_input_usd":           "输入费用（USD）",
		"js.csv_output_usd":          "输出费用（USD）",
		"js.csv_cache_read_usd":      "缓存读取费用（USD）",
		"js.csv_cache_creation_usd":  "缓存创建费用（USD）",
		"js.csv_total_usd":           "总费用（USD）",
		"js.csv_tier_threshold":      "层级阈值",
		"js.tier_threshold_aria":     "{model} 上下文层级阈值",
		"placeholder.management_key": "管理密钥",
	} {
		if got := messages[LanguageChinese][key]; got != expected {
			t.Fatalf("chinese %s = %q, want %q", key, got, expected)
		}
	}
	for _, forbidden := range []string{
		"text('tokenDetail','Input '",
		"[t('js.price_head_model'),'Input'",
		"name+' Context Tier threshold'",
		"'Input USD','Output USD'",
		`placeholder="API key"`,
		`placeholder="Management key"`,
	} {
		if strings.Contains(dashboardHTMLTemplate, forbidden) {
			t.Fatalf("dashboard retains hardcoded english label %q", forbidden)
		}
	}
}

func TestDashboardStableModelKeyForFilters(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function modelKey(value)`,
		`function modelName(value){var key=modelKey(value);return key==='unlabeled'?t('js.unlabeled_model'):key;}`,
		`var key=modelKey(group.model)`,
		`if(selectedModel)url+='&model='+encodeURIComponent(selectedModel)`,
		`function requestResult(item){if(item&&item.failed)`,
		`badgeCell(row,requestResult(item),item.failed?'failed':'')`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing stable model/filter contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`var name=modelName(group.model);if(!map[name])`,
		`var name=modelName(group.model);return !hiddenModels.has(name)`,
		`var name=modelName(item.model);if(!map[name])`,
		`var name=modelName(point.model);if(hiddenModels.has(name)`,
		`url+='&model='+encodeURIComponent(modelName(selectedModel))`,
		`encodeURIComponent(t('js.unlabeled_model'))`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard uses localized model label as key/filter %q", forbidden)
		}
	}
}

func TestDashboardServerRenderedPlaceholdersAreReplaced(t *testing.T) {
	for _, lang := range []Language{LanguageEnglish, LanguageChinese} {
		html := renderDashboardHTML(lang, lang)
		if strings.Contains(html, "__MSG_") {
			idx := strings.Index(html, "__MSG_")
			end := idx + 40
			if end > len(html) {
				end = len(html)
			}
			t.Fatalf("%s dashboard still has unresolved placeholder near %q", lang, html[idx:end])
		}
		if strings.Contains(html, "__I18N_BOOTSTRAP__") || strings.Contains(html, "__LANG_TAG__") {
			t.Fatalf("%s dashboard still has unresolved bootstrap placeholder", lang)
		}
	}
}

func TestDashboardExportControlKeepsAccessibleName(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`id="exportButton"`,
		`aria-label="Export"`,
		`title="Export"`,
		`.button-label{display:none}`,
		`<span class="button-label">Export</span>`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing export accessible-name contract %q", required)
		}
	}
	exportIdx := strings.Index(html, `id="exportButton"`)
	if exportIdx < 0 {
		t.Fatal("export button missing")
	}
	snippet := html[exportIdx : exportIdx+280]
	if !strings.Contains(snippet, `aria-label="Export"`) || !strings.Contains(snippet, `title="Export"`) {
		t.Fatalf("export button must keep aria-label/title near control markup: %s", snippet)
	}
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	for _, required := range []string{
		`aria-label="导出"`,
		`title="导出"`,
		`<span class="button-label">导出</span>`,
	} {
		if !strings.Contains(zh, required) {
			t.Fatalf("chinese dashboard missing localized export accessible name %q", required)
		}
	}
}

func TestDashboardModelShareLegendScalesWithoutClipping(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.visual-grid{display:grid;grid-template-columns:minmax(0,1.85fr) minmax(0,.85fr)`,
		`.panel{overflow:hidden;min-width:0}`,
		`.donut-layout{display:grid;grid-template-columns:minmax(0,.95fr) minmax(0,1.05fr)`,
		`.donut-wrap{position:relative;min-width:0`,
		`.donut-wrap svg{display:block;width:100%;max-width:280px;height:auto;aspect-ratio:1`,
		`.legend{min-width:0;max-height:292px;overflow:auto;overflow-x:hidden`,
		`.legend-item{display:grid;grid-template-columns:24px minmax(0,1fr) minmax(4.25em,max-content)`,
		`.legend-share{min-width:4.25em`,
		`text-align:right;white-space:nowrap`,
		`.donut-layout{grid-template-columns:1fr;min-height:0`,
		`.legend{display:grid;grid-template-columns:repeat(2,minmax(0,1fr))`,
		`.legend{grid-template-columns:1fr}`,
		`share.className='legend-share'`,
		`share.textContent=percent.toFixed(1)+'%'`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing model-share responsive contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`.donut-layout{display:grid;grid-template-columns:minmax(210px,.95fr) minmax(150px,1fr)`,
		`.donut-layout{grid-template-columns:minmax(220px,.8fr) minmax(220px,1.2fr)}`,
		`minmax(300px,.75fr)`,
		`.legend-item{display:grid;grid-template-columns:24px minmax(0,1fr) auto`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained clipping-prone layout %q", forbidden)
		}
	}
}

func TestDashboardLegendLabelRecoversFullModelDetails(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`var nameText=modelName(item.model)`,
		`var detailText=t('js.legend_detail',{requests:fmt(item.requests),tokens:compact(item.total_tokens)})`,
		`var legendTip=t('js.legend_tooltip',{model:nameText,detail:detailText})`,
		`label.title=legendTip`,
		`label.setAttribute('aria-label',legendTip)`,
		`share.className='legend-share'`,
		`share.textContent=percent.toFixed(1)+'%'`,
		`.legend-name{display:block;overflow:hidden`,
		`text-overflow:ellipsis;white-space:nowrap`,
		`label.addEventListener('click',function(){selectModel(item.model);})`,
		`toggle.addEventListener('click',function(event){event.stopPropagation();toggleModel(item.model);})`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing legend tooltip/a11y contract %q", required)
		}
	}
	en := renderDashboardHTML(LanguageEnglish, LanguageEnglish)
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	if !strings.Contains(en, "legend_tooltip") {
		t.Fatal("english dashboard bootstrap missing legend_tooltip")
	}
	if !strings.Contains(zh, "legend_tooltip") {
		t.Fatal("chinese dashboard bootstrap missing legend_tooltip")
	}
	if messages[LanguageEnglish]["js.legend_tooltip"] != "{model} · {detail}" {
		t.Fatalf("english legend tooltip = %q", messages[LanguageEnglish]["js.legend_tooltip"])
	}
	if messages[LanguageChinese]["js.legend_tooltip"] != "{model} · {detail}" {
		t.Fatalf("chinese legend tooltip = %q", messages[LanguageChinese]["js.legend_tooltip"])
	}
	if !strings.Contains(html, `minmax(4.25em,max-content)`) {
		t.Fatal("legend share column geometry missing")
	}
}

func TestDashboardTokenTrendZoomHelpDoesNotOverlapAxis(t *testing.T) {
	html := dashboardHTML
	template := dashboardHTMLTemplate
	for _, required := range []string{
		`.chart-wrap{position:relative;display:grid;grid-template-rows:minmax(0,1fr) auto`,
		`.chart-footer{display:flex;justify-content:flex-end;align-items:center;min-height:22px`,
		`class="chart-footer" aria-hidden="true"`,
		`.zoom-tip{color:var(--text-quaternary);font-size:10px;line-height:1.3;pointer-events:none`,
		`.chart-wrap{min-height:300px}.chart-wrap svg{min-height:270px;height:100%}`,
		`.chart-footer{display:none}`,
		`document.getElementById('barWrap').addEventListener('wheel'`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing zoom-help geometry contract %q", required)
		}
	}
	if !strings.Contains(template, `class="zoom-tip">__MSG_zoom.tip__`) {
		t.Fatal("template missing zoom tip placeholder inside chart footer")
	}
	if !strings.Contains(template, `<div class="chart-footer" aria-hidden="true"><div class="zoom-tip">__MSG_zoom.tip__</div></div>`) {
		t.Fatal("template must place zoom tip in chart-footer below the svg")
	}
	for _, forbidden := range []string{
		`.zoom-tip{position:absolute;right:6px;bottom:2px`,
		`.chart-wrap,.chart-wrap svg{min-height:300px;height:300px}`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained overlapping zoom-help layout %q", forbidden)
		}
	}
	if strings.Contains(template, `<svg id="chart" viewBox="0 0 900 330" role="img" aria-label="__MSG_chart.token_bars__"></svg><div class="zoom-tip">`) {
		t.Fatal("template still overlays zoom tip on the svg plot area")
	}
	en := renderDashboardHTML(LanguageEnglish, LanguageEnglish)
	zh := renderDashboardHTML(LanguageChinese, LanguageChinese)
	if !strings.Contains(en, "Scroll to zoom · Shift + scroll to pan") {
		t.Fatal("english zoom tip not localized in rendered dashboard")
	}
	if !strings.Contains(zh, "滚轮缩放 · Shift + 滚轮平移") {
		t.Fatal("chinese zoom tip not localized in rendered dashboard")
	}
}

func TestDashboardChartAccessibilityIsLocalizedAndKeyboardOperable(t *testing.T) {
	template := dashboardHTMLTemplate
	for _, required := range []string{
		"class:'donut-segment'",
		"tabindex:0,role:'button'",
		"'aria-label':t('js.donut_segment_aria',{model:modelName(item.model),share:shareText(percent,1)})",
		"segment.addEventListener('focus'",
		"activateOnKeyboard(segment,select)",
		"if(event.key==='Enter'||event.key===' ')",
		"event.preventDefault();action()",
		"'aria-label':t('js.cost_point_aria',{label:point.item.label,cost:money(point.item.total_usd)})",
	} {
		if !strings.Contains(template, required) {
			t.Fatalf("dashboard missing interactive chart accessibility behavior %q", required)
		}
	}
	for _, lang := range []Language{LanguageEnglish, LanguageChinese} {
		html := renderDashboardHTML(lang, lang)
		for _, label := range []string{T(lang, "zoom.out"), T(lang, "zoom.in")} {
			if !strings.Contains(html, `aria-label="`+label+`" title="`+label+`"`) {
				t.Fatalf("%s dashboard missing localized zoom label %q", lang, label)
			}
		}
		for _, key := range []string{"js.donut_segment_aria", "js.cost_point_aria"} {
			if T(lang, key) == "" || T(lang, key) == key {
				t.Fatalf("%s catalog missing accessibility message %q", lang, key)
			}
		}
	}
}

func TestDashboardSummaryCardsShareUniformVerticalRhythm(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.card{position:relative;display:grid;grid-template-rows:auto minmax(2.4em,1fr) auto`,
		`.card .label{display:flex;align-items:center;gap:7px;min-height:28px`,
		`.card .value{position:relative;z-index:1;display:flex;align-items:center;margin-top:10px;min-height:2.4em`,
		`.card .value.model-value{display:flex;align-items:center;gap:9px;min-height:2.4em`,
		`.card .detail{position:relative;z-index:1;margin-top:8px;min-height:1.35em`,
		`.card-switch{position:relative;z-index:2;margin-left:auto;min-height:28px`,
		`class="value model-value"`,
		`id="modelBadge" class="model-badge"`,
		`id="tokenUnitButton" class="card-switch"`,
		`id="currencyButton" class="card-switch"`,
		`.card::after{content:"";position:absolute;right:-29px;bottom:-36px;width:90px;height:90px`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing summary-card rhythm contract %q", required)
		}
	}
}
