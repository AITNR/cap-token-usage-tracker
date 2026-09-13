package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPIKeyBaseURLSourceSurvivesPersistedReadPath(t *testing.T) {
	config := testConfig(t)
	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config}
	defer runtime.shutdown()
	runtime.setAuthRuntimeLookup(func(authIndex string) (authRuntimeMetadata, error) {
		return authRuntimeMetadata{
			Provider:    "xai",
			AccountType: "api_key",
			Label:       "my-relay-credential",
			BaseURL:     "https://relay.example.com/v1",
		}, nil
	})

	// Record without an inline BaseURL: the identity resolver must contribute
	// the runtime base_url, and the stored source must stay the bare URL.
	raw := []byte(`{"provider":"xai","model":"grok-4","api_key":"xai-secret-1234567890","auth_type":"apikey","source":"xai-secret-1234567890","auth_index":"stable-auth-index","requested_at":"` + time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) + `"}`)
	if _, err := runtime.handleUsage(raw); err != nil {
		t.Fatal(err)
	}

	// Record with an inline BaseURL carrying credentials that must be stripped.
	inlineRaw := []byte(`{"provider":"openai","model":"gpt-5","api_key":"sk-user-secret-1234567890","auth_type":"apikey","source":"sk-user-secret-1234567890","BaseURL":"https://user:secret@relay-two.example.com/v1/?token=leaky","requested_at":"` + time.Now().UTC().Format(time.RFC3339) + `"}`)
	if _, err := runtime.handleUsage(inlineRaw); err != nil {
		t.Fatal(err)
	}

	page, err := store.QueryRequests("24h", 0, 10, "")
	if err != nil || page.Total != 2 {
		t.Fatalf("request page = %+v, %v", page, err)
	}
	sources := map[string]bool{}
	for _, item := range page.Items {
		sources[item.Source] = true
		if item.Source == "https://api.x.ai/v1" || item.Source == "https://api.openai.com/v1" {
			t.Fatalf("hardcoded provider address surfaced instead of configured base URL: %q", item.Source)
		}
	}
	if !sources["https://relay.example.com/v1"] || !sources["https://relay-two.example.com/v1"] {
		t.Fatalf("configured base URLs missing from read path: %v", sources)
	}

	stats, err := store.Query("24h")
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range stats.Groups {
		if group.Source == "https://api.x.ai/v1" || group.Source == "https://api.openai.com/v1" {
			t.Fatalf("hardcoded provider address surfaced in stats: %q", group.Source)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Dir(config.DataPath))
}

func TestIdentityFromRuntimeMetadataBuildsSafeProviderAccountLabel(t *testing.T) {
	tests := []struct {
		name     string
		metadata authRuntimeMetadata
		usage    Dimensions
		want     usageIdentity
	}{
		{name: "codex email", metadata: authRuntimeMetadata{Provider: "codex", Email: "user@example.com"}, want: usageIdentity{Provider: "Codex", Account: "user@example.com"}},
		{name: "same account antigravity", metadata: authRuntimeMetadata{Provider: "antigravity", Email: "user@example.com"}, want: usageIdentity{Provider: "Antigravity", Account: "user@example.com"}},
		{name: "xai becomes grok", metadata: authRuntimeMetadata{Provider: "xai", Email: "user@example.com"}, want: usageIdentity{Provider: "Grok", Account: "user@example.com"}},
		{name: "oauth account fallback", metadata: authRuntimeMetadata{Provider: "codex", AccountType: "oauth", Account: "oauth-account"}, want: usageIdentity{Provider: "Codex", Account: "oauth-account"}},
		{name: "safe label fallback", metadata: authRuntimeMetadata{Provider: "custom", Label: "team-account"}, want: usageIdentity{Provider: "custom", Account: "team-account"}},
		{name: "api key account ignored", metadata: authRuntimeMetadata{Provider: "codex", AccountType: "api_key", Account: "sk-secret-1234567890"}, usage: Dimensions{Source: "cli"}, want: usageIdentity{Provider: "Codex", Account: "cli"}},
		{name: "source fallback is sanitized", metadata: authRuntimeMetadata{Provider: "codex"}, usage: Dimensions{Source: "https://user:secret@example.com/v1/?api_key=secret"}, want: usageIdentity{Provider: "Codex", Account: "https://example.com/v1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := identityFromRuntimeMetadata(test.metadata, test.usage); got != test.want {
				t.Fatalf("identity = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestAuthIdentityResolverCachesSanitizedMetadataAndUsesCurrentSourceFallback(t *testing.T) {
	var calls atomic.Int32
	resolver := newAuthIdentityResolver(func(authIndex string) (authRuntimeMetadata, error) {
		calls.Add(1)
		return authRuntimeMetadata{Provider: "codex", AccountType: "api_key", Account: "sk-secret-1234567890"}, nil
	})
	first, err := resolver.resolve("stable-auth-index", Dimensions{Source: "first"})
	if err != nil || first != (usageIdentity{Provider: "Codex", Account: "first"}) {
		t.Fatalf("first resolve = %+v, %v", first, err)
	}
	second, err := resolver.resolve("stable-auth-index", Dimensions{Source: "second"})
	if err != nil || second != (usageIdentity{Provider: "Codex", Account: "second"}) {
		t.Fatalf("cached resolve = %+v, %v", second, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls.Load())
	}
	for _, entry := range resolver.entries {
		if entry.metadata.Account != "" || entry.metadata.Label != "" {
			t.Fatalf("credential-like runtime metadata persisted in cache: %+v", entry.metadata)
		}
	}
}

func TestAuthIdentityResolverCoalescesConcurrentLookups(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	resolver := newAuthIdentityResolver(func(authIndex string) (authRuntimeMetadata, error) {
		calls.Add(1)
		close(started)
		<-release
		return authRuntimeMetadata{Provider: "antigravity", Email: "user@example.com"}, nil
	})
	const workers = 8
	results := make(chan usageIdentity, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			identity, err := resolver.resolve("stable-auth-index", Dimensions{Source: "cli"})
			results <- identity
			errs <- err
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	if calls.Load() != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls.Load())
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for identity := range results {
		if identity != (usageIdentity{Provider: "Antigravity", Account: "user@example.com"}) {
			t.Fatalf("identity = %+v", identity)
		}
	}
}

func TestAuthIdentityResolverNegativeCachesFailures(t *testing.T) {
	var calls atomic.Int32
	resolver := newAuthIdentityResolver(func(authIndex string) (authRuntimeMetadata, error) {
		calls.Add(1)
		return authRuntimeMetadata{}, errors.New("runtime lookup failed")
	})
	for range 2 {
		if _, err := resolver.resolve("stable-auth-index", Dimensions{}); err == nil {
			t.Fatal("lookup failure was not returned")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("lookup calls = %d, want 1", calls.Load())
	}
}

func TestResolveUsageIdentityAppliesRuntimeBaseURLForAPIKeyAuth(t *testing.T) {
	runtime := &pluginRuntime{}
	runtime.setAuthRuntimeLookup(func(authIndex string) (authRuntimeMetadata, error) {
		return authRuntimeMetadata{
			Provider:    "xai",
			AccountType: "api_key",
			Label:       "my-relay-credential",
			BaseURL:     "https://user:secret@relay.example.com/v1/?token=leaky",
		}, nil
	})

	usage := &normalizedUsage{
		Dimensions: Dimensions{
			Provider: "xai",
			AuthType: "apikey",
			Source:   "xai-secret-key-1234567890",
		},
		authIndex: "stable-auth-index",
	}
	runtime.resolveUsageIdentity(usage)
	if usage.Dimensions.Source != "https://relay.example.com/v1" {
		t.Fatalf("source = %q, want bare sanitized runtime base URL", usage.Dimensions.Source)
	}
	if usage.authIndex != "" {
		t.Fatalf("auth index = %q, want cleared", usage.authIndex)
	}
}

func TestResolveUsageIdentityPrefersRecordBaseURLOverRuntimeLookup(t *testing.T) {
	runtime := &pluginRuntime{}
	runtime.setAuthRuntimeLookup(func(authIndex string) (authRuntimeMetadata, error) {
		return authRuntimeMetadata{Provider: "xai", AccountType: "api_key", BaseURL: "https://runtime.example.com/v1"}, nil
	})

	usage := &normalizedUsage{
		Dimensions: Dimensions{
			Provider: "xai",
			AuthType: "apikey",
			Source:   "xai-secret-key-1234567890",
		},
		authIndex: "stable-auth-index",
		baseURL:   "https://record.example.com/v1",
	}
	runtime.resolveUsageIdentity(usage)
	if usage.Dimensions.Source != "https://record.example.com/v1" {
		t.Fatalf("source = %q, want record base URL to win", usage.Dimensions.Source)
	}
}

func TestResolveUsageIdentityFallsBackToCompositeLabelWithoutBaseURL(t *testing.T) {
	runtime := &pluginRuntime{}
	runtime.setAuthRuntimeLookup(func(authIndex string) (authRuntimeMetadata, error) {
		return authRuntimeMetadata{Provider: "codex", AccountType: "oauth", Email: "user@example.com"}, nil
	})

	usage := &normalizedUsage{
		Dimensions: Dimensions{
			Provider: "codex",
			AuthType: "oauth",
			Source:   "user@example.com",
		},
		authIndex: "stable-auth-index",
	}
	runtime.resolveUsageIdentity(usage)
	if usage.Dimensions.Source != "Codex-user@example.com" {
		t.Fatalf("source = %q, want composite identity label", usage.Dimensions.Source)
	}
}

func TestSanitizeAuthRuntimeMetadataSanitizesBaseURL(t *testing.T) {
	metadata := sanitizeAuthRuntimeMetadata(authRuntimeMetadata{
		Provider: "xai",
		BaseURL:  "https://user:secret@relay.example.com/v1/?token=leaky",
	})
	if metadata.BaseURL != "https://relay.example.com/v1" {
		t.Fatalf("base URL = %q, want sanitized", metadata.BaseURL)
	}
	if metadata := sanitizeAuthRuntimeMetadata(authRuntimeMetadata{Provider: "xai", BaseURL: "not a url"}); metadata.BaseURL != "" {
		t.Fatalf("invalid base URL = %q, want empty", metadata.BaseURL)
	}
}
