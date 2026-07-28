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
		`function selectModel(name,options)`,
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
		`<option value="minute">按分钟</option>`,
		`<option value="hour" selected>按小时</option>`,
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

func TestDashboardHeaderKeepsHostClearanceAndControlGroups(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.heading{min-width:0;padding:2px clamp(96px,10vw,152px) 0 0}`,
		`class="control-group control-filters"`,
		`class="control-group control-actions"`,
		`flex-wrap:nowrap`,
		`.control-actions{flex:0 0 auto;margin-left:auto}`,
		`button.control{display:inline-flex;align-items:center;justify-content:center;gap:7px;padding:8px 12px;font-weight:650;white-space:nowrap}`,
		`@media(max-width:820px){.heading{padding-right:clamp(72px,12vw,112px)}`,
		`.control-filters{flex-basis:100%}`,
		`.control-actions{margin-left:0}`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing header layout contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`--host-overlay-safe-inset`,
		`<label class="language-control">`,
		`id="languageSelect"`,
		`__MSG_`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard unexpectedly contains forbidden header markup %q", forbidden)
		}
	}
}

func TestDashboardExportControlKeepsAccessibleName(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`id="exportButton"`,
		`aria-label="导出"`,
		`title="导出"`,
		`.button-label{display:none}`,
		`<span class="button-label">导出</span>`,
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
	if !strings.Contains(snippet, `aria-label="导出"`) || !strings.Contains(snippet, `title="导出"`) {
		t.Fatalf("export button must keep aria-label/title near control markup: %s", snippet)
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
		`var detailText=fmt(item.requests)+' 次 · '+compact(item.total_tokens)+' Tokens'`,
		`var legendTip=nameText+' · '+detailText`,
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
	if !strings.Contains(html, `minmax(4.25em,max-content)`) {
		t.Fatal("legend share column geometry missing")
	}
}

func TestDashboardTokenTrendZoomHelpDoesNotOverlapAxis(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.chart-wrap{position:relative;display:grid;grid-template-rows:minmax(0,1fr) auto`,
		`.chart-footer{display:flex;justify-content:flex-end;align-items:center;min-height:22px`,
		`class="chart-footer" aria-hidden="true"`,
		`.zoom-tip{color:var(--text-quaternary);font-size:10px;line-height:1.3;pointer-events:none`,
		`.chart-wrap{min-height:300px}.chart-wrap svg{min-height:270px;height:100%}`,
		`.chart-footer{display:none}`,
		`document.getElementById('barWrap').addEventListener('wheel'`,
		`<div class="zoom-tip">滚轮缩放 · Shift + 滚轮平移</div>`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing zoom-help geometry contract %q", required)
		}
	}
	if !strings.Contains(html, `<div class="chart-footer" aria-hidden="true"><div class="zoom-tip">滚轮缩放 · Shift + 滚轮平移</div></div>`) {
		t.Fatal("zoom tip must be placed in chart-footer below the svg")
	}
	for _, forbidden := range []string{
		`.zoom-tip{position:absolute;right:6px;bottom:2px`,
		`.chart-wrap,.chart-wrap svg{min-height:300px;height:300px}`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained overlapping zoom-help layout %q", forbidden)
		}
	}
	if strings.Contains(html, `<svg id="chart" viewBox="0 0 900 330" role="img" aria-label="Token 消耗堆叠柱状图"></svg><div class="zoom-tip">`) {
		t.Fatal("zoom tip still overlays the svg plot area")
	}
}

func TestDashboardChartAccessibilityIsKeyboardOperable(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"class:'donut-segment'",
		"tabindex:0,role:'button'",
		"segment.addEventListener('focus'",
		"activateOnKeyboard(segment,function(){selectModel(item.model,{restoreDonutFocus:true});})",
		"if(event.key==='Enter'||event.key===' ')",
		"event.preventDefault();action()",
		"'aria-label':point.item.label+' 费用 '+money(point.item.total_usd)",
		`aria-label="缩小" title="缩小"`,
		`aria-label="放大" title="放大"`,
		`.donut-segment:focus-visible{outline:none;stroke-width:34;filter:drop-shadow(0 0 3px var(--primary-color))}`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing interactive chart accessibility behavior %q", required)
		}
	}
}

func TestDashboardDonutKeyboardSelectionRestoresFocus(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"'data-model':item.model",
		"function selectModel(name,options)",
		"if(options&&options.restoreDonutFocus)",
		"var segments=document.querySelectorAll('#donut .donut-segment')",
		"if(segments[i].getAttribute('data-model')===name)",
		"segments[i].focus()",
		"segment.addEventListener('click',function(){selectModel(item.model);})",
		"activateOnKeyboard(segment,function(){selectModel(item.model,{restoreDonutFocus:true});})",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing donut keyboard focus restore behavior %q", required)
		}
	}
	if strings.Contains(html, "segment.addEventListener('click',function(){selectModel(item.model,{restoreDonutFocus:true});})") {
		t.Fatal("mouse click path must not restore donut focus")
	}
	if !strings.Contains(html, "segment.addEventListener('mouseenter',function(event){showModelTooltip(event,item,percent);})") {
		t.Fatal("donut tooltip mouseenter behavior must remain intact")
	}
	if !strings.Contains(html, "segment.addEventListener('focus',function(event){showModelTooltip(event,item,percent);})") {
		t.Fatal("donut tooltip focus behavior must remain intact")
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
