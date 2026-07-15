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
		"setTimeout(function(){controller.abort();},10000)",
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
		"fetch('stats')",
		`fetch("stats")`,
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
