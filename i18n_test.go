package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestNormalizeAndResolveLanguage(t *testing.T) {
	if got := normalizeLanguage("EN-us"); got != LanguageEnglish {
		t.Fatalf("normalize en: %q", got)
	}
	if got := normalizeLanguage("zh-Hans"); got != LanguageChinese {
		t.Fatalf("normalize zh-Hans: %q", got)
	}
	for _, tag := range []string{"zh", "zh-TW", "zh-HK", "zh-MO", "zh-Hant"} {
		if got := normalizeLanguage(tag); got != "" {
			t.Fatalf("traditional tag %q mapped to %q", tag, got)
		}
	}
	if got := normalizeLanguage("auto"); got != LanguageAuto {
		t.Fatalf("normalize auto: %q", got)
	}
	if got := resolveLanguage(LanguageAuto, "zh-CN,zh;q=0.9,en;q=0.8", ""); got != LanguageChinese {
		t.Fatalf("accept-language zh: %q", got)
	}
	if got := resolveLanguage(LanguageAuto, "fr-FR,fr;q=0.9", ""); got != LanguageEnglish {
		t.Fatalf("unknown accept-language should fall back to en: %q", got)
	}
	if got := resolveLanguage(LanguageChinese, "en-US", ""); got != LanguageChinese {
		t.Fatalf("configured override ignored: %q", got)
	}
	if got := resolveLanguage(LanguageChinese, "en-US", "en"); got != LanguageEnglish {
		t.Fatalf("query override ignored: %q", got)
	}
}

func TestLanguagePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		configured Language
		accept     string
		query      string
		want       Language
		preferred  Language
	}{
		{name: "auto config negotiates", configured: LanguageAuto, accept: "zh-CN", want: LanguageChinese, preferred: LanguageAuto},
		{name: "english config pins", configured: LanguageEnglish, accept: "zh-CN", want: LanguageEnglish, preferred: LanguageEnglish},
		{name: "chinese config pins", configured: LanguageChinese, accept: "en-US", want: LanguageChinese, preferred: LanguageChinese},
		{name: "explicit english overrides config", configured: LanguageChinese, accept: "zh-CN", query: "en", want: LanguageEnglish, preferred: LanguageEnglish},
		{name: "explicit chinese overrides config", configured: LanguageEnglish, accept: "en-US", query: "zh-CN", want: LanguageChinese, preferred: LanguageChinese},
		{name: "explicit auto overrides english config", configured: LanguageEnglish, accept: "zh-CN", query: "auto", want: LanguageChinese, preferred: LanguageAuto},
		{name: "explicit auto overrides chinese config", configured: LanguageChinese, accept: "en-US", query: "AUTO", want: LanguageEnglish, preferred: LanguageAuto},
		{name: "explicit auto falls back to english", configured: LanguageChinese, accept: "fr-FR", query: "auto", want: LanguageEnglish, preferred: LanguageAuto},
		{name: "invalid query leaves config", configured: LanguageChinese, accept: "en-US", query: "de", want: LanguageChinese, preferred: LanguageChinese},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveLanguage(test.configured, test.accept, test.query); got != test.want {
				t.Fatalf("resolved language = %q, want %q", got, test.want)
			}
			if got := languagePreference(test.configured, test.query); got != test.preferred {
				t.Fatalf("selector preference = %q, want %q", got, test.preferred)
			}
		})
	}
}

func TestMessageCatalogParity(t *testing.T) {
	en := messages[LanguageEnglish]
	if len(en) == 0 {
		t.Fatal("empty english catalog")
	}
	definitions := make(map[Language]languageDefinition, len(languageDefinitions))
	for _, definition := range languageDefinitions {
		definitions[definition.Code] = definition
		catalog := messages[definition.Code]
		if len(catalog) == 0 {
			t.Fatalf("language definition %q has no catalog", definition.Code)
		}
		if catalog[definition.LabelKey] == "" {
			t.Fatalf("language definition %q has no selector label", definition.Code)
		}
		if normalizeLanguage(string(definition.Code)) != definition.Code {
			t.Fatalf("language definition %q is not normalized", definition.Code)
		}
	}
	for lang, catalog := range messages {
		if _, ok := definitions[lang]; !ok {
			t.Fatalf("catalog %q has no language definition", lang)
		}
		for key := range en {
			if _, ok := catalog[key]; !ok {
				t.Fatalf("catalog %q missing english key %q", lang, key)
			}
		}
		for key := range catalog {
			if _, ok := en[key]; !ok {
				t.Fatalf("catalog %q has extra key %q", lang, key)
			}
		}
	}
}

func TestDisplayModelAndResultCompatibility(t *testing.T) {
	if displayModelName("") != legacyUnlabeledModel || displayModelName(unlabeledModel) != legacyUnlabeledModel {
		t.Fatal("synthesized model representation changed")
	}
	if !matchesModelFilter("", unlabeledModel) || !matchesModelFilter(legacyUnlabeledModel, unlabeledModel) {
		t.Fatal("model filter compatibility failed")
	}
	if matchesModelFilter("", "Unlabeled model") || matchesModelFilter(legacyUnlabeledModel, "Unlabeled model") {
		t.Fatal("localized unlabeled display text must not match as a filter key")
	}
	if formatResultLabel(false, 0) != legacyResultSuccess {
		t.Fatal("success representation changed")
	}
	if formatResultLabel(true, 500) != "失败 (HTTP 500)" {
		t.Fatal("failed representation changed")
	}
}

func TestLanguageConfigParsing(t *testing.T) {
	cfg, err := parseConfig([]byte("language: zh-CN\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != LanguageChinese {
		t.Fatalf("got %q", cfg.Language)
	}
	if _, err := parseConfig([]byte("language: de\n")); err == nil {
		t.Fatal("expected invalid language error")
	}
	cfg, err = parseConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != LanguageAuto {
		t.Fatalf("default language %q", cfg.Language)
	}
}

func TestDashboardLanguageNegotiation(t *testing.T) {
	runtime := &pluginRuntime{config: Config{Language: LanguageAuto}}
	request := pluginapi.ManagementRequest{
		Headers: http.Header{"Accept-Language": []string{"zh-CN"}},
	}
	body := string(runtime.dashboardResponse(request).Body)
	if !strings.Contains(body, `<html lang="zh-CN"`) || !strings.Contains(body, `<title>Token 用量统计</title>`) {
		t.Fatal("expected chinese negotiated dashboard")
	}
	request = pluginapi.ManagementRequest{
		Headers: http.Header{"Accept-Language": []string{"zh-CN"}},
		Query:   url.Values{"lang": []string{"en"}},
	}
	body = string(runtime.dashboardResponse(request).Body)
	if !strings.Contains(body, `<html lang="en"`) || !strings.Contains(body, `<title>Token Usage Analytics</title>`) || strings.Contains(body, `<title>Token 用量统计</title>`) {
		t.Fatal("query lang should override accept-language")
	}
}

func TestDashboardExplicitAutoOverridesPinnedConfig(t *testing.T) {
	tests := []struct {
		name       string
		configured Language
		accept     string
		wantLang   string
		wantTitle  string
	}{
		{name: "english config to chinese browser", configured: LanguageEnglish, accept: "zh-CN", wantLang: "zh-CN", wantTitle: "Token 用量统计"},
		{name: "chinese config to english browser", configured: LanguageChinese, accept: "en-US", wantLang: "en", wantTitle: "Token Usage Analytics"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &pluginRuntime{config: Config{Language: test.configured}}
			response := runtime.dashboardResponse(pluginapi.ManagementRequest{
				Headers: http.Header{"Accept-Language": []string{test.accept}},
				Query:   url.Values{"lang": []string{"auto"}},
			})
			body := string(response.Body)
			if response.Headers.Get("Content-Language") != test.wantLang {
				t.Fatalf("Content-Language = %q, want %q", response.Headers.Get("Content-Language"), test.wantLang)
			}
			if !strings.Contains(body, `<html lang="`+test.wantLang+`"`) || !strings.Contains(body, `<title>`+test.wantTitle+`</title>`) {
				t.Fatalf("dashboard did not negotiate %q from explicit auto", test.wantLang)
			}
			if !strings.Contains(body, `<option value="auto" selected>`) {
				t.Fatal("selector did not retain explicit automatic preference")
			}
		})
	}
}

func TestAcceptLanguageQualityZero(t *testing.T) {
	if got := parseAcceptLanguage("zh-CN;q=0,en;q=0.5"); got != LanguageEnglish {
		t.Fatalf("zh-CN q=0 should be ignored, got %q", got)
	}
	if got := parseAcceptLanguage("zh-CN;q=0"); got != LanguageEnglish {
		t.Fatalf("only unacceptable language should fall back to en, got %q", got)
	}
	if got := parseAcceptLanguage("en;q=0,zh-CN;q=0.8"); got != LanguageChinese {
		t.Fatalf("en q=0 should lose to zh-CN, got %q", got)
	}
	if got := resolveLanguage(LanguageAuto, "zh-CN;q=0,en;q=0.1", ""); got != LanguageEnglish {
		t.Fatalf("resolve with q=0 chinese: %q", got)
	}
}

func TestLanguageDefinitionDrivesUIAndHostConfig(t *testing.T) {
	values := supportedLanguageValues(true)
	if strings.Join(values, ",") != "auto,en,zh-CN" {
		t.Fatalf("supported language values = %v", values)
	}
	registration := pluginRegistration()
	var enumValues []string
	for _, field := range registration.Metadata.ConfigFields {
		if field.Name == "language" {
			enumValues = field.EnumValues
		}
	}
	if strings.Join(enumValues, ",") != strings.Join(values, ",") {
		t.Fatalf("host language enum = %v, definitions = %v", enumValues, values)
	}
	html := renderDashboardHTML(LanguageAuto, LanguageChinese)
	for _, expected := range []string{`<option value="auto" selected>自动（跟随浏览器）</option>`, `<option value="en">English</option>`, `<option value="zh-CN">中文</option>`, `"locale":"zh-CN"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("dashboard language metadata missing %q", expected)
		}
	}
}

func TestAutomaticDashboardKeepsServerNegotiatedLanguage(t *testing.T) {
	html := renderDashboardHTML(LanguageAuto, LanguageChinese)
	if !strings.Contains(html, "function applyLanguagePreference(next){I18N.preferred=next||'auto';") {
		t.Fatal("automatic initialization does not preserve bootstrap preference")
	}
	if strings.Contains(html, "navigator.language") || strings.Contains(html, "navigator.languages") {
		t.Fatal("automatic initialization recomputes server-negotiated language")
	}
	if !strings.Contains(html, "function currentLocale(){return I18N.locale||I18N.lang||'en';}") || !strings.Contains(html, ".toLocaleString(currentLocale())") {
		t.Fatal("client formatting does not use server-provided locale metadata")
	}
	if !strings.Contains(html, "url.searchParams.set('lang',next);window.location.replace(url.pathname+url.search+url.hash)") {
		t.Fatal("selector changes must retain an explicit query preference before server negotiation")
	}
	if strings.Contains(html, "url.searchParams.delete('lang')") {
		t.Fatal("automatic selection must not remove its explicit config override")
	}
}

func TestChineseDashboardEyebrowTranslated(t *testing.T) {
	if messages[LanguageChinese]["dashboard.eyebrow"] != "用量分析" {
		t.Fatalf("dashboard.eyebrow chinese = %q", messages[LanguageChinese]["dashboard.eyebrow"])
	}
	body := renderDashboardHTML(LanguageChinese, LanguageChinese)
	if !strings.Contains(body, `<div class="eyebrow"><span class="eyebrow-mark"></span>用量分析</div>`) {
		t.Fatal("chinese dashboard missing translated eyebrow shell text")
	}
	if strings.Contains(body, `<div class="eyebrow"><span class="eyebrow-mark"></span>Usage analytics</div>`) {
		t.Fatal("chinese dashboard shell still uses english eyebrow")
	}
}
