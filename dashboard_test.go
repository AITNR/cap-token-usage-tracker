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
		"var statsURL='/v0/resource/plugins/'",
		"load().catch(function(error)",
		"resetKeyInput.value=''",
		"resetDialog.showModal()",
		"data-theme-value=\"auto\"",
		"data-theme-value=\"white\"",
		"data-theme-value=\"light\"",
		"data-theme-value=\"dark\"",
		"window.matchMedia",
		"cap-token-usage-theme",
		"羊毛纸",
	} {
		if !strings.Contains(html, required) {
			t.Fatalf("dashboard missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"replaceChildren.apply",
		"Math.max.apply",
		"sessionStorage",
		"localStorage.setItem('management",
		"localStorage.setItem('reset",
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
