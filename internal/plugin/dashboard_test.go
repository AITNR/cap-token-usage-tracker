package plugin

import (
	"encoding/json"
	"os/exec"
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
		"var resourceBase=publicPathPrefix+'/v0/resource/plugins/'",
		"var statsURL=resourceBase+'/stats'",
		"load(true).catch(function(error)",
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
		`.panel{overflow:hidden;min-width:0}`,
		`.table-panel{overflow:visible}`,
		`.table-wrap{max-height:540px;overflow:auto;border-radius:0 0 12px 12px;scrollbar-gutter:stable}`,
		`.request-columns-menu{position:absolute`,
		`max-height:calc(100dvh - 32px);overflow:auto`,
		`function positionColumnsMenu(menu,button)`,
		`menu.style.position='fixed'`,
		`window.addEventListener('resize',function(){if(!document.getElementById('requestColumnsMenu').hidden)positionColumnsMenu`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing short-table column menu fix %q", required)
		}
	}
}

func TestDashboardAnimatesEveryDropdownMenu(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.dropdown-surface{opacity:0;pointer-events:none;transform:translateY(-6px) scale(.98)`,
		`.dropdown-surface.is-open{opacity:1;pointer-events:auto;transform:translateY(0) scale(1)}`,
		`.dropdown-surface.is-closing{transition-duration:120ms}`,
		`function openDropdownSurface(menu,button,position)`,
		`function closeDropdownSurface(menu,button,restoreFocus,afterClose)`,
		`event.propertyName==='opacity'`,
		`dropdownCloseTimers.set(menu,setTimeout`,
		`menu.classList.add('is-opening')`,
		`menu.classList.add('is-closing')`,
		`@media(prefers-reduced-motion:reduce)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing dropdown animation contract %q", required)
		}
	}
}

func TestDashboardEnhancesNativeSelectMenus(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function enhanceSelect(select)`,
		`function syncEnhancedSelect(select)`,
		`function enhanceDashboardSelects(root)`,
		`role='combobox'`,
		`role='listbox'`,
		`role='option'`,
		`aria-activedescendant`,
		`new Event('change',{bubbles:true})`,
		`['ArrowDown','ArrowUp','Home','End']`,
		`enhanceSelect(select);renderSourceOptions([]);`,
		`syncEnhancedSelect(select)`,
		`enhanceDashboardSelects(list)`,
		`enhanceDashboardSelects(document)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing enhanced-select contract %q", required)
		}
	}
}

func TestDashboardIncludesInteractiveAnalyticsFeatures(t *testing.T) {
	html := fullDashboardHTML
	for _, required := range []string{
		`id="granularity"`,
		`id="totalCost"`,
		`id="tokenUnitButton"`,
		`id="currencyButton"`,
		`var exchangeRateURL=resourceBase+'/exchange-rate'`,
		`function formatTokenTotal(value)`,
		`function toggleTokenUnit()`,
		`B:{suffix:'B',divisor:1e9}`,
		`order=['full','k','m','B']`,
		`async function toggleCurrency()`,
		`id="topModel"`,
		`id="donut"`,
		`id="legend"`,
		`bar-input`,
		`bar-output`,
		`bar-cache-read`,
		`cache-hit-line`,
		`stroke-dasharray:7 5`,
		`function trendCacheReadTokens(point)`,
		`function trendCacheHitRate(input,cacheRead)`,
		`bucket.cacheRead+=trendCacheReadTokens(point)`,
		`item.cacheHitRate=item.hasData?trendCacheHitRate(item.input,item.cacheRead):null;`,
		`pointStackTotal(point)`,
		`t('trend.cacheHitRate')`,
		`model_series`,
		`function selectModel(name,options)`,
		`function toggleModel(name)`,
		`addEventListener('wheel'`,
		`moneyFormatters[key]`,
		`var costsURL=resourceBase+'/costs'`,
		`function aggregateCostSeries()`,
		`function visibleCostSummary()`,
		`price-cache-read`,
		`price-cache-creation`,
		`id="costCoverage"`,
		`item.estimated_cost`,
		`record.estimated_cost`,
		`estimated.input_usd`,
		`estimated.output_usd`,
		`estimated.cache_read_usd`,
		`estimated.cache_creation_usd`,
		`estimated.total_usd`,
		`async function exportCSV()`,
		`function exportPNG()`,
		`id="exportBackup"`,
		`var backupURL=resourceBase+'/full-mode/backup'`,
		`var restoreURL=resourceBase+'/full-mode/restore'`,
		`async function downloadBackup()`,
		`function restoreBackup()`,
		`async function confirmAndRestore(file)`,
		`if(file.size > 64*1024*1024){text('error',t('backup.fileTooLarge'));return;}`,
		`selectedSource=''`,
		`renderSourceOptions([])`,
		`await loadDashboardPreferences()`,
		`function restoreBackup(){
  closeExportMenu();`,
		`data-i18n="button.downloadBackup"`,
		`data-i18n="button.restoreBackup"`,
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
		`id="requestModelFilter"`,
		`id="requestSourceFilter"`,
		`id="requestResultFilter"`,
		`function renderRequestFilters()`,
		`requestSourceFilter=''`,
		`params.set('model',requestModelFilter)`,
		`params.set('source',requestSourceFilter)`,
		`params.set('result',requestResultFilter)`,
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
		`time_range_mode:appliedRangeMode`,
		`params.set('time_range_mode',value.time_range_mode)`,
		`params.set('save','1')`,
		`params.append('hidden_request_column',key)`,
		`params.append('hidden_dimension_column',key)`,
		`keepalive:true`,
		`window.addEventListener('pagehide'`,
		`loadDashboardPreferences().catch(function(error)`,
		`function loadGroups(sequence,query)`,
		`var statsGroupsURL=resourceBase+'/stats/groups'`,
		`params.set('offset',String(dimensionOffset))`,
		`params.set('sort',dimensionSortKey)`,
		`renderGroups(page.items,Number(page.total||0))`,
		`empty.colSpan=Math.max(1,columns.length)`,
		`function zoomTrend(points,zoom,factor,anchorRatio,render)`,
		`{passive:false,capture:true}`,
		`生成时间`,
		`缓存命中`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing analytics feature %q", required)
		}
	}
}

func TestDashboardCacheHitRateUsesInputTokenDenominator(t *testing.T) {
	html := dashboardHTML
	if !strings.Contains(html, `return input?Math.min(100,cacheRead/input*100):0;`) {
		t.Fatal("cache hit rate must divide cache read tokens by input tokens")
	}
	if strings.Contains(html, `var context=input+cacheRead;return context?Math.min(100,cacheRead/context*100):0;`) {
		t.Fatal("cache hit rate still double-counts cache read tokens in its denominator")
	}
	for _, required := range []string{
		`function trendCacheReadTokens(point){var cacheRead=Number(point.cache_read_tokens||0);return cacheRead>0?cacheRead:Number(point.cached_tokens||0);}`,
		`bucket.cacheRead+=trendCacheReadTokens(point)`,
		`item.cacheHitRate=item.hasData?trendCacheHitRate(item.input,item.cacheRead):null;`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboardHTML missing cache hit rate aggregation %q", required)
		}
	}
}

func TestDashboardTrendKeepsContinuousTimeBuckets(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function nextBucketInfo(info,granularity)`,
		`function continuousBuckets(map,granularity,emptyBucket)`,
		`var range=resolvedDateRange(),start=bucketInfo(range.start,granularity),end=bucketInfo(new Date(range.end.getTime()-1),granularity)`,
		`while(info.stamp<=end.stamp&&steps<100000)`,
		`function emptyTrendBucket(info){return {key:info.key,label:info.label,stamp:info.stamp,hasData:false`,
		`function emptyCostBucket(info){return {key:info.key,label:info.label,stamp:info.stamp,hasData:false`,
		`item.cacheHitRate=item.hasData?trendCacheHitRate(item.input,item.cacheRead):null;`,
		`if(showCacheHit&&point.hasData&&point.cacheHitRate!==null)`,
		`item.index===previousIndex+1?'L':'M'`,
		`if(!point.hasData){tooltipRow(t('chart.noRequests'),t('locale.unavailable'));`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing continuous time-series behavior %q", required)
		}
	}
	if strings.Contains(html, `return capSeries(result);`) {
		t.Fatal("continuous time buckets must not be downsampled because that would skip empty intervals")
	}
}

func TestDashboardRefreshPreservesTrendZoom(t *testing.T) {
	html := dashboardHTML
	if !strings.Contains(html, "function startTimer(){if(refreshTimer)clearInterval(refreshTimer);refreshTimer=setInterval(function(){load().catch") {
		t.Fatal("dashboard must retain its automatic refresh path")
	}
	if strings.Contains(html, "if(costLoadError)text('error',t('error.costUnavailable',{message:costLoadError}));resetTrendZooms();renderRequestFilters()") {
		t.Fatal("dashboard render must not reset either trend zoom during a refresh")
	}
	for _, required := range []string{
		"function confirmDateRange(){",
		"updateGranularityForRange();resetTrendZooms();",
		"document.getElementById('granularity').addEventListener('change',function(){resetTrendZooms();renderVisuals();})",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard must still reset both trend zooms when their data scope changes: %q", required)
		}
	}
}

func TestDashboardRefreshRetainsMatchingCostSnapshot(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"currentCosts=null,costQuery='',costLoadError=''",
		"render(initial,costQuery===query?currentCosts:null)",
		"costQuery=query;costLoadError='';render(currentData,costs);",
		"costLoadError=error.message;render(currentData,costQuery===query?currentCosts:null);",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard refresh must retain matching cost data until its replacement arrives: %q", required)
		}
	}
}

func TestDashboardRefreshKeepsCostSnapshotDuringRollingRangeRefresh(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"function clearErrors(){text('error','');costLoadError='';}",
		"var costLoading=false,baseRender=render,baseRenderVisuals=renderVisuals",
		"if(!costs&&currentCosts){costLoading=!costLoadError;costs=currentCosts;}",
		"t('status.costRefreshing')",
		"t('status.costRefreshFailed')",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard must retain the previous cost snapshot during refresh: %q", required)
		}
	}
	if strings.Contains(html, "currentCosts=null,costQuery='',costLoadError='',costLoading") {
		t.Fatal("dashboard cost state must not clear the cost snapshot on refresh")
	}
}

func TestDashboardTrendZoomsAreIndependent(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		"tokenTrendZoom={start:0,size:0},costTrendZoom={start:0,size:0}",
		"function resetTrendZooms(){resetTrendZoom(tokenTrendZoom);resetTrendZoom(costTrendZoom);}",
		"function renderBar(){var svg=document.getElementById('chart'),fragment=document.createDocumentFragment(),all=aggregateTrend(),series=visibleTrend(all,tokenTrendZoom)",
		"function renderCostTrend(){var svg=document.getElementById('costChart'),fragment=document.createDocumentFragment(),all=currentCosts?aggregateCostSeries():[];lastCostTrend=all;var series=trendWindow(all,costTrendZoom)",
		`id="costZoomOutButton"`,
		`id="costZoomInButton"`,
		`id="costResetZoomButton"`,
		`id="costWrap"`,
		"document.getElementById('zoomInButton').addEventListener('click',function(){zoomTrend(lastTrend,tokenTrendZoom,.7,.5,renderBar);})",
		"document.getElementById('costZoomInButton').addEventListener('click',function(){zoomTrend(lastCostTrend,costTrendZoom,.7,.5,renderCostTrend);})",
		"document.getElementById('barWrap').addEventListener('wheel',function(event){if(lastTrend.length<2)return;",
		"zoomTrend(lastTrend,tokenTrendZoom,factor,ratio,renderBar);",
		"document.getElementById('costWrap').addEventListener('wheel',function(event){if(lastCostTrend.length<2)return;",
		"zoomTrend(lastCostTrend,costTrendZoom,factor,ratio,renderCostTrend);",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing independent trend zoom behavior %q", required)
		}
	}
	for _, forbidden := range []string{"barZoomStart", "barZoomSize"} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard must not retain shared trend zoom behavior %q", forbidden)
		}
	}
}

func TestDashboardRequestCacheHitRateContract(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`{key:'cache_hit_rate',label:'table.cacheHitRate',numeric:true}`,
		`function cacheAccountingKind(point)`,
		`function cacheHitRateForPoint(point)`,
		`denominator=input+creation+read`,
		`return cacheHitRate(cacheReadTokens(point),cacheHitDenominator(point))`,
		`case 'cache_hit_rate':return cacheHitRateForPoint(item);`,
		`case 'cache_hit_rate':td=cell(row,formatCacheHitRate(cacheHitRateForPoint(item)),'num');break;`,
		`t('value.unavailable')`,
		`localeNumber(rate,{minimumFractionDigits:2,maximumFractionDigits:2})+'%'`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing request cache hit rate contract %q", required)
		}
	}
}

func TestDashboardRequestCacheHitRateExecution(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	start := strings.Index(dashboardHTML, "function cacheCounter(point,key)")
	end := strings.Index(dashboardHTML, "function renderRequestHeaders()")
	if start < 0 || end < start {
		t.Fatal("dashboard cache-hit helpers are missing")
	}
	script := `
var requestSortKey='cache_hit_rate',requestSortDirection='asc',formatterLocale='en-US';
` + dashboardHTML[start:end] + `
function equal(actual,expected,label){if(actual!==expected)throw new Error(label+': expected '+expected+', got '+actual);}
function rate(point){return cacheHitRateForPoint(point);}
var creationOnly=[
  ['openai',{provider:'openai',input_tokens:100,cache_read_tokens:0,cache_creation_tokens:10,cached_tokens:10}],
  ['claude',{provider:'claude',input_tokens:10,cache_read_tokens:0,cache_creation_tokens:10,cached_tokens:10}],
  ['gemini',{provider:'gemini',input_tokens:100,cache_read_tokens:0,cache_creation_tokens:10,cached_tokens:10}]
];
creationOnly.forEach(function(example){equal(rate(example[1]),0,example[0]+' creation-only');});
equal(cacheReadTokens({provider:'unknown',input_tokens:100,cache_read_tokens:0,cache_creation_tokens:10,cached_tokens:10}),0,'unknown creation-only cache read');
equal(rate({provider:'openai',input_tokens:100,cache_read_tokens:30,cache_creation_tokens:0,cached_tokens:0}),30,'subset read-only');
equal(rate({provider:'gemini',input_tokens:100,cache_read_tokens:30,cache_creation_tokens:0,cached_tokens:0}),30,'separate read-only');
equal(rate({provider:'gemini',input_tokens:100,cache_read_tokens:30,cache_creation_tokens:10,cached_tokens:0}),30,'separate read plus creation');
equal(rate({provider:'claude',input_tokens:10,cache_read_tokens:80,cache_creation_tokens:10,cached_tokens:0}),80,'independent read plus creation');
equal(rate({provider:'unknown',input_tokens:100,cache_read_tokens:30,cache_creation_tokens:0,cached_tokens:0}),null,'unknown provider');
var items=[
  {sequence:1,provider:'openai',input_tokens:100,cache_read_tokens:25,cache_creation_tokens:0,cached_tokens:0},
  {sequence:2,provider:'unknown',input_tokens:100,cache_read_tokens:25,cache_creation_tokens:0,cached_tokens:0},
  {sequence:3,provider:'openai',input_tokens:100,cache_read_tokens:50,cache_creation_tokens:0,cached_tokens:0}
];
equal(sortedRequestItems(items).map(function(item){return item.sequence;}).join(','),'1,3,2','ascending null last');
requestSortDirection='desc';
equal(sortedRequestItems(items).map(function(item){return item.sequence;}).join(','),'3,1,2','descending null last');
`
	command := exec.Command(node, "--check", "-")
	command.Stdin = strings.NewReader(script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("dashboard cache-hit test script syntax: %v\n%s", err, output)
	}
	command = exec.Command(node, "-")
	command.Stdin = strings.NewReader(script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("dashboard cache-hit execution: %v\n%s", err, output)
	}
}

func TestDashboardSourceFilterUsesSharedQueryScope(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function initializeSourceFilter()`,
		`select.id='sourceFilter'`,
		`granularity.insertAdjacentElement('afterend',select)`,
		`function renderSourceOptions(sources)`,
		`currentData&&currentData.sources`,
		`params.set('source',selectedSource)`,
		`load(true).catch(function(error){text('error',error.message);})`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing source-filter contract %q", required)
		}
	}
}

func TestDashboardPreservesReverseProxyPathPrefix(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`function readDashboardRoute()`,
		`marker='/v0/resource/plugins/'`,
		`index=path.lastIndexOf(marker)`,
		`publicPathPrefix:path.slice(0,index)`,
		`var publicPathPrefix=dashboardRoute.publicPathPrefix`,
		`var resourceBase=publicPathPrefix+'/v0/resource/plugins/'`,
		`var managementBase=publicPathPrefix+'/v0/management/plugins/'`,
		`var modelsURL=publicPathPrefix+'/v1/models'`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing reverse-proxy path-prefix contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`var resourceBase='/v0/resource/plugins/'`,
		`var managementBase='/v0/management/plugins/'`,
		`var modelsURL='/v1/models'`,
		`parts.indexOf('plugins')`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retains root-only path construction %q", forbidden)
		}
	}
}

func TestDashboardUsesExactBackendCostsAndPricingSync(t *testing.T) {
	html := fullDashboardHTML
	for _, required := range []string{
		`var costsURL=resourceBase+'/costs'`,
		`var pricesURL=resourceBase+'/full-mode/prices'`,
		`var savePricesURL=resourceBase+'/full-mode/prices/save'`,
		`var syncPricesURL=resourceBase+'/full-mode/prices/sync'`,
		`id="pricingDialog"`,
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
		`var modelsURL=publicPathPrefix+'/v1/models'`,
		`function normalizeCLIModels(payload)`,
		`async function fetchCLIModels(renderEditor)`,
		`cliModelsPromise=api(modelsURL`,
		`api(costsURL+'?'+query)`,
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
		`service_tiers`,
		`provider_priority`,
		`ignored_suffixes`,
		`mappings`,
		`last_sync`,
		`source:'models.dev'`,
		`fullModePayloadRequest(savePricesURL,{prices:next,sync_settings:settings})`,
		`fullModePayloadRequest(syncPricesURL,{source:'models.dev',models:models,sync_settings:settings},25000)`,
		`displayCurrency==='CNY'`,
		`value*Number(exchangeRate.rate||0)`,
		`label.textContent=money(value)`,
		`formatTokenTotal(summary.total_tokens)`,
		`renderVisuals();await loadRequests();return responses`,
		`function finishPricingDialogClose(token)`,
		`function closePricingDialog(restoreFocus)`,
		`function openPricing()`,
		`pricingDialogAnimationToken`,
		`pricingDialogCloseTimer`,
		`pricingDialog.classList.add('is-open')`,
		`pricingDialog.classList.add('is-closing')`,
		`pricingDialog.addEventListener('transitionend'`,
		`pricingDialog.addEventListener('cancel',function(event){event.preventDefault();closePricingDialog(true);})`,
		`dialog#pricingDialog.is-open{opacity:1`,
		`dialog#pricingDialog.is-closing{transition-duration:120ms}`,
		`dialog#pricingDialog::backdrop{background:rgb(0 0 0/0)`,
		`await reloadPricesAndCosts();closePricingDialog(true);`,
		`pricingDialog.addEventListener('close',function(){clearCLIModelState();clearPricingDraft();})`,
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

func TestDashboardUsesTwoMonthLocalDateRangePicker(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`id="rangeButton"`,
		`aria-expanded="false" aria-controls="dateRangePopover"`,
		`id="dateRangePopover" class="date-range-popover" role="dialog" aria-modal="false"`,
		`id="dateRangeTitle"`,
		`id="calendarLeft"`,
		`id="calendarRight"`,
		`id="confirmDateRange"`,
		`id="resetDateRange"`,
		`id="startTime" class="native-time-source" type="time" step="1"`,
		`id="endTime" class="native-time-source" type="time" step="1"`,
		`id="startTimeButton" class="time-picker-button" type="button" aria-haspopup="dialog"`,
		`id="endTimeButton" class="time-picker-button" type="button" aria-haspopup="dialog"`,
		`id="startTimePicker" class="time-picker-surface" role="dialog"`,
		`id="endTimePicker" class="time-picker-surface" role="dialog"`,
		`data-time-part="hour" type="number" min="0" max="23"`,
		`data-time-part="minute" type="number" min="0" max="59"`,
		`data-time-part="second" type="number" min="0" max="59"`,
		`function initializeTimePickers()`,
		`function closeTimePickers(restoreFocus)`,
		`function finishTimePickerClose(picker,token)`,
		`input.dispatchEvent(new Event('input',{bubbles:true}))`,
		`.time-picker-surface.is-open{opacity:1`,
		`.time-picker-surface.is-closing{transition-duration:120ms}`,
		`event.key==='Escape'&&activeTimePicker`,
		`data-i18n="range.endExclusive"`,
		`function setDraftTime(boundary,value)`,
		`draftRangeStart.getTime()<draftRangeEnd.getTime()`,
		`document.getElementById('startTime').addEventListener('input'`,
		`document.getElementById('endTime').addEventListener('input'`,
		`appliedRangeStart.toISOString()`,
		`appliedRangeEnd.toISOString()`,
		`function parseSavedRange(value)`,
		`duration<6*60*60*1000?'minute':duration<=2*24*60*60*1000?'hour'`,
		`function positionDateRangePopover()`,
		`placeBelow=below>=popoverHeight||below>=above`,
		`dateRangePopover.dataset.placement=placeBelow?'bottom':'top'`,
		`function closeDateRange(restoreFocus)`,
		`function finishDateRangeClose(token)`,
		`token!==dateRangeAnimationToken`,
		`dateRangePopover.classList.add('is-closing')`,
		`dateRangePopover.classList.add('is-opening')`,
		`dateRangePopover.classList.add('is-open')`,
		`var wasHidden=dateRangePopover.hidden`,
		`clearTimeout(dateRangeCloseTimer)`,
		`dateRangeCloseTimer=setTimeout(function(){finishDateRangeClose(token);},180)`,
		`event.propertyName==='opacity'`,
		`dateRangePopover.hidden=true`,
		`dateRangePopover.hidden=false`,
		`rangeButton.setAttribute('aria-expanded','true')`,
		`dateRangePopover.hidden||dateRangePopover.classList.contains('is-closing')`,
		`event.key==='Escape'&&!dateRangePopover.hidden`,
		`event.composedPath?event.composedPath():[]`,
		`path.indexOf(rangeButton)<0&&path.indexOf(dateRangePopover)<0`,
		`dateRangePopover.addEventListener('click',function(event){event.stopPropagation()`,
		`id="quickRanges" class="quick-ranges" role="group"`,
		`data-range-preset="last_5_hours"`,
		`data-range-preset="last_7_days"`,
		`data-range-preset="last_30_days"`,
		`data-range-preset="current_month"`,
		`button.setAttribute('aria-pressed'`,
		`function applyRangePreset(mode)`,
		`function dateRangeQuery()`,
		`function resolvedDateRange()`,
		`now.getTime()-5*60*60*1000`,
		`today.getFullYear(),today.getMonth(),today.getDate()-6`,
		`today.getFullYear(),today.getMonth(),today.getDate()-29`,
		`today.getFullYear(),today.getMonth(),today.getDate()+1`,
		`new Date(today.getFullYear(),today.getMonth(),1)`,

		`draftRangeMode='custom'`,
		`new Date(calendarBaseMonth.getFullYear(),calendarBaseMonth.getMonth()+1,1)`,

		`document.getElementById('confirmDateRange').disabled=!complete`,
		`params.set('start',range.start.toISOString())`,
		`params.set('end',range.end.toISOString())`,
		`scheduleDashboardPreferencesSave();closeDateRange(true)`,
		`id="calendarPanels" class="calendar-panels"`,
		`panels.classList.add(delta>0?'is-shifting-next':'is-shifting-previous')`,
		`.calendar-panels.is-shifting-next .calendar-panel{animation:calendar-enter-next`,
		`.calendar-panels.is-shifting-previous .calendar-panel{animation:calendar-enter-previous`,
		`if(reducedMotion)return`,
		`.calendar-panels{display:grid;grid-template-columns:repeat(2`,
		`.date-range-popover{--date-popover-shift:-6px;position:fixed`,
		`transform:translateY(var(--date-popover-shift)) scale(.98)`,
		`.date-range-popover[data-placement='top']`,
		`.date-range-popover.is-open{opacity:1;pointer-events:auto`,
		`.date-range-popover.is-closing{transition-duration:120ms}`,
		`@media(prefers-reduced-motion:reduce)`,
		`@media(max-width:560px){.range-control`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing date range picker contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`<select id="range"`,
		`document.getElementById('range').addEventListener('change'`,
		`<dialog id="dateRangeDialog"`,
		`dateRangeDialog.showModal()`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retains obsolete date range picker pattern %q", forbidden)
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

func TestFullModeUsesSeparateProtectedDashboard(t *testing.T) {
	if !strings.Contains(dashboardHTML, `id="fullModeButton"`) || !strings.Contains(dashboardHTML, `id="fullModeDialog"`) {
		t.Fatal("dashboard must provide a full-mode entry button and dialog")
	}
	if !strings.Contains(dashboardHTML, `managementBase+'/full-mode/session'`) || !strings.Contains(dashboardHTML, `method:'POST'`) || !strings.Contains(dashboardHTML, `Authorization':'Bearer '+key`) {
		t.Fatal("full-mode dialog must create an authenticated full-mode session")
	}
	if !strings.Contains(dashboardHTML, `resourceBase+'/full-dashboard#session='+encodeURIComponent(session)`) || strings.Contains(dashboardHTML, `fullModeManagementKey=key`) {
		t.Fatal("homepage must navigate with the opaque session token without retaining the management key")
	}
	for _, forbidden := range []string{
		`id="pricingButton"`, `id="pricingDialog"`, `id="priceList"`, `id="savePricing"`, `id="syncPrices"`,
		`id="exportButton"`, `id="exportMenu"`, `id="exportCSV"`, `id="exportPNG"`, `id="exportBackup"`, `id="restoreBackup"`, `id="backupDialog"`,
		`function exportCSV()`, `function exportPNG()`, `function downloadBackup()`, `function restoreBackup()`, `function confirmAndRestore(file)`,
		`document.getElementById('exportBackup').addEventListener`, `document.getElementById('restoreBackup').addEventListener`,
	} {
		if strings.Contains(dashboardHTML, forbidden) {
			t.Fatalf("normal dashboard must not expose pricing UI %q", forbidden)
		}
	}
	if !strings.Contains(dashboardHTML, `function initializePricingSelectEnhancement(){var list=document.getElementById('priceList');if(!list)return;`) {
		t.Fatal("normal dashboard must skip pricing select initialization when full-mode pricing UI is absent")
	}
	if !strings.Contains(dashboardHTML, `function openDateRange(){closeActiveDropdown(false);if(typeof closeExportMenu==='function')closeExportMenu();`) {
		t.Fatal("normal dashboard date range picker must not require the removed export menu script")
	}
	for _, required := range []string{`var fullModePage=true`, `button.exitFullMode`, `history.replaceState(null,'',window.location.pathname+window.location.search)`} {
		if !strings.Contains(fullDashboardHTML, required) {
			t.Fatalf("full dashboard missing %q", required)
		}
	}
	if !strings.Contains(fullDashboardHTML, `X-Full-Mode-Session`) || !strings.Contains(fullDashboardHTML, `resourceBase+'/full-mode/prices'`) || !strings.Contains(fullDashboardHTML, `function openPricing(){if(!fullModeEnabled||!fullModeSession)return;`) || strings.Contains(fullDashboardHTML, `fullModeManagementKey=key`) {
		t.Fatal("full dashboard must use the server-issued capability for protected endpoints")
	}
	for _, required := range []string{
		`id="pricingButton" class="control"`, `id="pricingDialog"`, `id="priceList"`, `id="savePricing"`, `id="syncPrices"`,
		`id="exportButton"`, `id="exportMenu"`, `id="exportCSV"`, `id="exportPNG"`, `id="exportBackup"`, `id="restoreBackup"`, `id="backupDialog"`,
		`resourceBase+'/full-mode/backup'`, `resourceBase+'/full-mode/restore'`, `async function requireFullModeExportSession()`, `await fullModeBinaryPayloadRequest(restoreURL,file,120000)`,
	} {
		if !strings.Contains(fullDashboardHTML, required) {
			t.Fatalf("full dashboard must provide pricing UI %q", required)
		}
	}
	if !strings.Contains(fullDashboardHTML, `var resetURL=resourceBase+'/full-mode/reset';`) || !strings.Contains(fullDashboardHTML, `async function resetStats(){if(!fullModeEnabled||!fullModeSession){text('error',t('fullMode.keyRequired'));return;}`) {
		t.Fatal("full dashboard reset must use the session-protected resource route")
	}
	for _, forbidden := range []string{`askBackupManagementKey`, `managementBase+'/backup'`, `managementBase+'/restore'`, `Authorization':'Bearer '+managementKey,'Content-Type':'application/octet-stream'`, `function askManagementKey()`, `id="resetDialog"`, `resetKeyInput`, `Authorization':'Bearer '+managementKey`} {
		if strings.Contains(fullDashboardHTML, forbidden) {
			t.Fatalf("full dashboard export must not use a management key %q", forbidden)
		}
	}
	if strings.Contains(fullDashboardHTML, `sensitive_data":[]`) {
		t.Fatal("full dashboard HTML must not embed protected data")
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

func TestDashboardTokenTrendSeriesTogglesAreAccessible(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`class="series-key" role="group"`,
		`data-i18n-aria="trend.series.aria"`,
		`class="series-key-button" data-series="input" aria-pressed="true"`,
		`data-series="output"`,
		`data-series="cacheRead"`,
		`data-series="cacheHitRate"`,
		`function toggleTrendSeries(key)`,
		`function trendSeriesVisible(key)`,
		`function pointStackTotal(point)`,
		`hiddenTrendSeries=new Set()`,
		`button.setAttribute('aria-pressed',visible?'true':'false')`,
		`t('trend.series.hide',{series:trendSeriesLabel(key)})`,
		`t('trend.series.show',{series:trendSeriesLabel(key)})`,
		`t('trend.series.allHidden')`,
		`if(trendSeriesVisible('input'))tooltipRow(t('table.input')`,
		`if(trendSeriesVisible('cacheHitRate'))tooltipRow(t('trend.cacheHitRate')`,
		`showInput=trendSeriesVisible('input')`,
		`showCacheHit=trendSeriesVisible('cacheHitRate')`,
		`if(showCacheHit)fragment.appendChild(svgNode('line',{x1:width-right`,
		`.series-key-button[aria-pressed='false']`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing trend series toggle contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`p.input+p.output+p.cacheRead`,
		`max=trend.reduce(function(value,p){return Math.max(value,p.input+p.output+p.cacheRead);},1)`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained non-toggle-aware trend scaling %q", forbidden)
		}
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
		`data-i18n="trend.zoomTip"`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing zoom-help geometry contract %q", required)
		}
	}
	if !strings.Contains(html, `<div class="chart-footer" aria-hidden="true"><div class="zoom-tip" data-i18n="trend.zoomTip">`) {
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
	if strings.Contains(html, `</svg><div class="zoom-tip"`) {
		t.Fatal("zoom tip still overlays the svg plot area")
	}
}

func TestDashboardModelShareLegendScalesWithoutClipping(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`.visual-grid{display:grid;grid-template-columns:minmax(0,1.85fr) minmax(0,.85fr)`,
		`.panel{overflow:hidden;min-width:0}`,
		`.donut-layout{display:grid;grid-template-columns:minmax(0,.95fr) minmax(0,1.05fr);align-items:center;gap:8px 0px`,
		`.donut-layout{display:grid;grid-template-columns:minmax(0,.95fr) minmax(0,1.05fr);align-items:center;gap:8px 0px;min-width:0;min-height:330px`,
		`.donut-wrap{position:relative;min-width:0`,
		`.donut-wrap svg{display:block;width:100%;max-width:280px;height:auto;aspect-ratio:1`,
		`.legend{min-width:0;max-height:330px;overflow:auto;overflow-x:hidden;padding:4px 2px 4px 2px;scrollbar-gutter:stable`,
		`.legend-item{display:grid;grid-template-columns:18px minmax(0,1fr) minmax(3.5em,max-content);align-items:center;column-gap:2px;min-width:0;margin:1px 0;padding:0 2px`,
		`.legend-toggle{display:flex;width:18px;height:32px`,
		`.legend-share{min-width:0;padding-right:0`,
		`.legend-label{min-width:0;padding:3px 0`,
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
		`var nameText=modelName(item.model),detailText=modelShareDetail(item),legendTip=nameText+' · '+detailText`,
		`label.title=legendTip`,
		`label.setAttribute('aria-label',legendTip)`,
		`share.className='legend-share'`,
		`share.textContent=percent.toFixed(1)+'%'`,
		`.legend-name{display:block;overflow:hidden`,
		`text-overflow:ellipsis;white-space:nowrap`,
		`label.addEventListener('click',function(){selectModel(item.model);})`,
		`toggle.addEventListener('click',function(event){event.stopPropagation();toggleModel(item.model);})`,
		`minmax(3.5em,max-content)`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing legend tooltip/a11y contract %q", required)
		}
	}
}

func TestDashboardModelShareMetricSwitch(t *testing.T) {
	html := fullDashboardHTML
	for _, required := range []string{
		`id="modelShareMetric" class="metric-switch" role="group" data-i18n-aria="modelShare.metric.aria"`,
		`data-model-share-metric="requests" aria-pressed="true"`,
		`data-model-share-metric="tokens" aria-pressed="false"`,
		`data-model-share-metric="cost" aria-pressed="false"`,
		`var selectedSource='',modelShareMetric='requests'`,
		`function renderSourceOptions(sources)`,
		`if(selectedSource)params.set('source',selectedSource)`,
		`function modelShareMetricValue(item)`,
		`if(modelShareMetric==='requests')return Number(item.requests||0)`,
		`if(modelShareMetric==='tokens')return Number(item.total_tokens||0)`,
		`if(modelShareMetric==='tokens')return formatTokenTotal(value)`,
		`return cost?Number(cost.total_usd||0):0`,
		`function syncModelShareMetricButtons()`,
		`var metric=button.getAttribute('data-model-share-metric'),unavailable=metric==='cost'&&!currentCosts`,
		`button.disabled=unavailable`,
		`function setModelShareMetric(metric)`,
		`modelShareMetric=metric;renderDonut()`,
		`total=visible.reduce(function(sum,item){return sum+modelShareMetricValue(item);},0)`,
		`var value=modelShareMetricValue(item),percent=total?value/total*100:0`,
		`main.textContent=formatModelShareMetric(total,true)`,
		`sub.textContent=modelShareMetricLabel()`,
		`document.getElementById('modelShareMetric').addEventListener('click'`,
		`setModelShareMetric(button.getAttribute('data-model-share-metric'))`,
		`pieTotal=pieModels.reduce(function(sum,item){return sum+modelShareMetricValue(item);},0)`,
		`canvasText(ctx,formatModelShareMetric(pieTotal,true)`,
		`tooltipRow(t('chart.totalTokens'),formatTokenTotal(item.total_tokens))`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing model-share metric switch contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`authIdentityFilter`,
		`auth_identity`,
		`auth_provider`,
		`auth_account`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard must not retain authentication identity UI %q", forbidden)
		}
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
		`data-i18n-aria="trend.zoomOut.title"`,
		`data-i18n-aria="trend.zoomIn.title"`,
		`.donut-segment:focus-visible{outline:none;stroke-width:34;filter:drop-shadow(0 0 3px var(--primary-color))}`,
		`class="series-key-button"`,
		`aria-pressed="true"`,
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

func TestDashboardTokenSummaryShowsUpstreamComponents(t *testing.T) {
	html := dashboardHTML
	for _, required := range []string{
		`animateText('totalTokens',formatTokenTotal(summary.total_tokens))`,
		`id="tokenDetail" class="detail" data-i18n="card.tokenDetail"`,
		`text('tokenDetail',t('card.tokenDetailValues',{inputLabel:t('card.input'),input:formatTokenTotal(summary.input_tokens),outputLabel:t('card.output'),output:formatTokenTotal(summary.output_tokens),cacheReadLabel:t('card.cacheRead'),cacheRead:formatTokenTotal(summary.cache_read_tokens)}));`,
		`cache_read_tokens`,
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing total-token component contract %q", required)
		}
	}
	for _, forbidden := range []string{
		`text('tokenDetail','Input '+fmt(summary.input_tokens)+' · Output '+fmt(summary.output_tokens));`,
		`text('tokenDetail','Input '+fmt(summary.input_tokens)+' · Cache '+fmt(summary.cached_tokens)+' · Output '+fmt(summary.output_tokens));`,
		`text('tokenDetail',t('card.tokenDetailValues',{inputLabel:t('card.uncachedInput')`,
		`Input + Output + other Tokens`,
		`summary.total_tokens=summary.input_tokens`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained misleading total-token detail via %q", forbidden)
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
		`.card .detail{position:relative;z-index:1;margin-top:8px;min-height:2.7em`,
		`line-height:1.35`,
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
	for _, forbidden := range []string{
		`.card{position:relative;min-height:124px;padding:17px 18px;overflow:hidden}`,
		`.card .detail{position:relative;z-index:1;margin-top:8px;color:var(--text-tertiary);font-size:11px}`,
		`.card .value{position:relative;z-index:1;margin-top:10px;color:var(--text-primary);`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("dashboard retained non-uniform card rhythm via %q", forbidden)
		}
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
		"button.downloadBackup",
		"button.restoreBackup",
		"backup.preparing",
		"backup.success",
		"backup.failed",
		"backup.keyPrompt",
		"backup.restoreWarning",
		"backup.restoring",
		"backup.restored",
		"backup.fileTooLarge",
		"status.loading",
		"status.costRefreshing",
		"status.costRefreshFailed",
		"chart.noCalls",
		"chart.noRequests",
		"trend.cacheHitRate",
		"table.cacheHitRate",
		"sourceFilter.label",
		"sourceFilter.all",
		"range.quickRanges",
		"range.lastFiveHours",
		"range.lastSevenDays",
		"range.lastThirtyDays",
		"range.currentMonth",
		"range.startTime",
		"range.endTime",
		"range.endExclusive",
		"range.invalidTimeRange",
		"card.successRate",
		"requestFilter.model",
		"requestFilter.source",
		"requestFilter.result",
		"requestFilter.allModels",
		"requestFilter.allSources",
		"requestFilter.allResults",
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
		"trend.series.aria",
		"trend.series.hide",
		"trend.series.show",
		"trend.series.allHidden",
		"card.tokenDetail",
		"card.tokenDetailValues",
		"card.input",
		"card.output",
		"card.cacheRead",
		"result.success",
		"result.failed",
		"result.failedHttp",
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

	// Guard translateRawResult and its call sites so Simplified-Chinese
	// backend result literals stay mapped through the locale catalog.
	for _, required := range []string{
		"function translateRawResult(raw,failed)",
		`/^失败`,
		"translateRawResult(item.result,item.failed)",
		"case 'result':return translateRawResult(item.result,item.failed);",
	} {
		if !strings.Contains(dashboardHTML, required) {
			t.Fatalf("dashboardHTML missing translateRawResult coverage %q", required)
		}
	}
	if !strings.Contains(fullDashboardHTML, "translateRawResult(record.result,record.failed)") {
		t.Fatal("full dashboardHTML missing export translateRawResult coverage")
	}

	// Verify the locale runtime is wired: translateStatic must be called
	// during initialisation (after theme and chart resize setup).
	initSeq := "initializeThemeSync();initializeChartResize();translateStatic();"
	if !strings.Contains(dashboardHTML, initSeq) {
		t.Fatalf("dashboardHTML missing init sequence %q", initSeq)
	}
}

func TestDashboardLocalizesUntitledModelInAllLocales(t *testing.T) {
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatalf("read locale %s: %v", code, err)
		}
		var values map[string]string
		if err := json.Unmarshal(data, &values); err != nil || strings.TrimSpace(values["model.untitled"]) == "" {
			t.Fatalf("locale %s has invalid model.untitled: %v", code, err)
		}
	}
	if !strings.Contains(dashboardHTML, "function modelName(value){return value&&String(value).trim()?String(value):t('model.untitled');}") {
		t.Fatal("dashboard does not localize backend-neutral unnamed models")
	}
}

func TestDashboardTemplateMarkersAreUniqueAndReplaced(t *testing.T) {
	markers := []string{
		"/*LOCALE_PLACEHOLDER*/",
		"/*FULL_MODE_PAGE*/",
		"/*FULL_MODE_APIKEY_STYLES*/",
		"/*FULL_MODE_APIKEY_FILTER*/",
		"/*FULL_MODE_APIKEY_MARKUP*/",
		"/*FULL_MODE_APIKEY_DIALOG*/",
		"/*FULL_MODE_APIKEY_QUERY*/",
		"/*FULL_MODE_APIKEY_LOAD*/",
		"/*FULL_MODE_APIKEY_RENDER*/",
		"/*FULL_MODE_APIKEY_DIMENSION_COLUMN*/",
		"/*FULL_MODE_APIKEY_DIMENSION_SORT*/",
		"/*FULL_MODE_APIKEY_DIMENSION_CELL*/",
		"/*FULL_MODE_APIKEY_REQUEST_COLUMN*/",
		"/*FULL_MODE_APIKEY_REQUEST_SORT*/",
		"/*FULL_MODE_APIKEY_REQUEST_CELL*/",
		"/*FULL_MODE_APIKEY_LANGUAGE*/",
		"/*FULL_MODE_APIKEY_SCRIPT*/",
	}
	for _, marker := range markers {
		if count := strings.Count(dashboardHTMLTemplate, marker); count != 1 {
			t.Fatalf("template marker %s appears %d times, want 1", marker, count)
		}
		if strings.Contains(dashboardHTML, marker) || strings.Contains(fullDashboardHTML, marker) {
			t.Fatalf("generated dashboard retains marker %s", marker)
		}
	}
}
