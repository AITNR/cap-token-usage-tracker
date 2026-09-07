package plugin

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestAPIKeyDashboardIsFullModeOnly(t *testing.T) {
	for _, forbidden := range []string{
		`id="apiKeyControls"`,
		`id="apiKeySecurityWarning"`,
		`id="apiKeyLabelDialog"`,
		"selectedAPIKeyRef",
		"params.set('api_key_ref'",
		`"apiKey.defaultSecretWarning"`,
		`"table.apiKey"`,
	} {
		if strings.Contains(dashboardHTML, forbidden) {
			t.Fatalf("ordinary dashboard contains API-key contract %q", forbidden)
		}
	}
	for _, required := range []string{
		`id="apiKeyControls"`,
		`id="apiKeySecurityWarning"`,
		`id="apiKeyFilterButton"`,
		`id="apiKeyFilterMenu"`,
		`id="apiKeyLabelDialog"`,
		"selectedAPIKeyRefs=[]",
		"function apiKeyRefsQuery(params)",
		"params.append('api_key_ref',ref)",
		"checkbox.dataset.apiKeyRef=item.ref",
		"requestAPIKeyFilterAll.dataset.apiKeyFilterAll='true'",
		"requestAPIKeyFilterAll.indeterminate=selectedAPIKeyRefs.length>0",
		"selectedAPIKeyRefs=selectedAPIKeyRefs.filter(function(ref){return known.has(ref);});",
		"function restoreAPIKeyFilterFocus(menu,ref)",
		"function currentAPIKeyFilterFocusRef()",
		"function keepAPIKeyFilterMenuOpen(event){event.stopPropagation();}",
		"requestAPIKeyFilterAll.addEventListener('click',keepAPIKeyFilterMenuOpen)",
		"checkbox.addEventListener('click',keepAPIKeyFilterMenuOpen)",
		"var focusRef=currentAPIKeyFilterFocusRef()",
		"renderAPIKeyFilter(focusRef)",
		"setSelectedAPIKeyRefs([], 'all')",
		"setSelectedAPIKeyRefs(next,item.ref)",
		"activeDropdown===document.getElementById('apiKeyFilterMenu')",
		"positionAPIKeyFilterMenu();",
		"generation_unavailable:'apiKey.generationUnavailable'",
		"ciphertext_missing:'apiKey.ciphertextMissing'",
		"ciphertext_invalid:'apiKey.ciphertextInvalid'",
		"identity_missing:'apiKey.identityMissing'",
		"apiKeyOptions=next;var known=new Set(next.map(function(item){return item.ref;}))",
		"initializeAPIKeyFullMode(payload)",
		"initializeAPIKeyFullMode(payload)",
		"statsInitialURL",
		"statsTrendsURL",
		"statsGroupsURL",
		"String(url).indexOf(statsInitialURL)===0",
		"String(url).indexOf(statsTrendsURL)===0",
		"String(url).indexOf(statsGroupsURL)===0",
		".apikey-security-warning[hidden]{display:none!important}",
		"api_key_uses_default_secret",
		"api(resourceBase+'/full-mode/data')",
		"String(url).indexOf(statsURL)===0",
		"String(url).indexOf(requestsURL)===0",
		"String(url).indexOf(costsURL)===0",
		"'X-Full-Mode-Session':fullModeSession",
		"'X-API-Key-Label':JSON.stringify({ref:editingAPIKeyRef,label:value})",
	} {
		if !strings.Contains(fullDashboardHTML, required) {
			t.Fatalf("full dashboard missing API-key contract %q", required)
		}
	}
	if strings.Contains(fullDashboardHTML, "/*FULL_MODE_APIKEY_") || strings.Contains(dashboardHTML, "/*FULL_MODE_APIKEY_") {
		t.Fatal("generated dashboard contains unresolved API-key placeholder")
	}
}

func TestAPIKeyFilterIsRenderedBesideSourceFilter(t *testing.T) {
	if strings.Contains(fullDashboardHTML, "/*FULL_MODE_APIKEY_FILTER*/") {
		t.Fatal("full dashboard contains unresolved API-key filter placeholder")
	}
	granularityIndex := strings.Index(fullDashboardHTML, `id="granularity"`)
	apiKeyIndex := strings.Index(fullDashboardHTML, `id="apiKeyFilter"`)
	if granularityIndex < 0 || apiKeyIndex < 0 || apiKeyIndex < granularityIndex {
		t.Fatal("API-key filter is not rendered after the aggregation filter")
	}
	if strings.Contains(fullDashboardHTML, `class="apikey-controls"`) && strings.Contains(fullDashboardHTML, `id="apiKeyFilter"`) {
		controlsIndex := strings.Index(fullDashboardHTML, `class="apikey-controls"`)
		if controlsIndex < apiKeyIndex {
			t.Fatal("API-key filter remains inside the standalone controls panel")
		}
	}
	for _, required := range []string{
		`granularity.insertAdjacentElement('afterend',select)`,
		`select.id='sourceFilter'`,
	} {
		if !strings.Contains(fullDashboardHTML, required) {
			t.Fatalf("full dashboard missing source-filter placement contract %q", required)
		}
	}
}

func TestAPIKeyLocaleCatalog(t *testing.T) {
	required := []string{
		"table.apiKey", "apiKey.filter", "apiKey.all", "apiKey.editLabel",
		"apiKey.label", "apiKey.labelPlaceholder", "apiKey.labelHint",
		"apiKey.saveLabel", "apiKey.deleteLabel", "apiKey.trackingDisabled",
		"apiKey.plaintextUnavailable", "apiKey.generationUnavailable",
		"apiKey.ciphertextMissing", "apiKey.ciphertextInvalid", "apiKey.identityMissing", "apiKey.sourceMissing",
		"apiKey.defaultSecretWarning",
		"apiKey.labelTooLong", "apiKey.saveFailed", "apiKey.filterMenu", "apiKey.filterSelected", "apiKey.filterHint",
	}
	for _, code := range []string{"en", "zh-CN", "zh-TW", "ru"} {
		data, err := localeFS.ReadFile("locales/" + code + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var catalog map[string]string
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("locale %s: %v", code, err)
		}
		for _, key := range required {
			if catalog[key] == "" {
				t.Fatalf("locale %s missing %q", code, key)
			}
		}
		if !strings.Contains(catalog["apiKey.defaultSecretWarning"], defaultAPIKeySecret) || !strings.Contains(catalog["apiKey.defaultSecretWarning"], "32") {
			t.Fatalf("locale %s warning does not explain the default and minimum strength", code)
		}
	}
}

func TestGeneratedDashboardJavaScriptSyntax(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not available")
	}
	for name, html := range map[string]string{"ordinary": dashboardHTML, "full": fullDashboardHTML} {
		t.Run(name, func(t *testing.T) {
			remaining := html
			for index := 0; ; index++ {
				start := strings.Index(remaining, "<script")
				if start < 0 {
					break
				}
				openEnd := strings.Index(remaining[start:], ">")
				if openEnd < 0 {
					t.Fatal("unterminated script tag")
				}
				openEnd += start
				closeAt := strings.Index(remaining[openEnd+1:], "</script>")
				if closeAt < 0 {
					t.Fatal("unterminated script body")
				}
				closeAt += openEnd + 1
				script := remaining[openEnd+1 : closeAt]
				command := exec.Command(node, "--check", "-")
				command.Stdin = strings.NewReader(script)
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatalf("script %d syntax: %v\n%s", index, err, output)
				}
				remaining = remaining[closeAt+len("</script>"):]
			}
		})
	}
}
