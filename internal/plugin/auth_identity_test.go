package plugin

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

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
