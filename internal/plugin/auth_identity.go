package plugin

import (
	"crypto/sha256"
	"strings"
	"sync"
	"time"
)

const (
	authIdentitySuccessTTL = 10 * time.Minute
	authIdentityFailureTTL = time.Minute
)

// authRuntimeMetadata intentionally contains only presentation metadata from
// host.auth.get_runtime. Credential JSON, auth IDs, paths, and auth indexes are
// never retained after resolving an identity.
type authRuntimeMetadata struct {
	Provider    string
	Type        string
	Email       string
	AccountType string
	Account     string
	Label       string
}

type authRuntimeLookup func(authIndex string) (authRuntimeMetadata, error)

type authIdentityResolver struct {
	lookup authRuntimeLookup
	now    func() time.Time

	mu       sync.Mutex
	entries  map[[sha256.Size]byte]authIdentityCacheEntry
	inFlight map[[sha256.Size]byte]*authIdentityFlight
}

type authIdentityCacheEntry struct {
	metadata authRuntimeMetadata
	err      error
	expires  time.Time
}

type authIdentityFlight struct {
	done chan struct{}
}

type usageIdentity struct {
	Provider string
	Account  string
}

func newAuthIdentityResolver(lookup authRuntimeLookup) *authIdentityResolver {
	return &authIdentityResolver{
		lookup:   lookup,
		now:      time.Now,
		entries:  make(map[[sha256.Size]byte]authIdentityCacheEntry),
		inFlight: make(map[[sha256.Size]byte]*authIdentityFlight),
	}
}

func (r *authIdentityResolver) resolve(authIndex string, usage Dimensions) (usageIdentity, error) {
	if r == nil || r.lookup == nil || strings.TrimSpace(authIndex) == "" {
		return usageIdentity{}, nil
	}
	authIndex = strings.TrimSpace(authIndex)
	cacheKey := sha256.Sum256([]byte(authIndex))
	now := r.now()

	r.mu.Lock()
	if cached, ok := r.entries[cacheKey]; ok && now.Before(cached.expires) {
		r.mu.Unlock()
		if cached.err != nil {
			return usageIdentity{}, cached.err
		}
		return identityFromRuntimeMetadata(cached.metadata, usage), nil
	}
	if flight := r.inFlight[cacheKey]; flight != nil {
		r.mu.Unlock()
		<-flight.done
		return r.resolve(authIndex, usage)
	}
	flight := &authIdentityFlight{done: make(chan struct{})}
	r.inFlight[cacheKey] = flight
	r.mu.Unlock()

	metadata, err := r.lookup(authIndex)
	if err == nil {
		metadata = sanitizeAuthRuntimeMetadata(metadata)
	}
	ttl := authIdentitySuccessTTL
	if err != nil {
		ttl = authIdentityFailureTTL
	}

	r.mu.Lock()
	r.entries[cacheKey] = authIdentityCacheEntry{metadata: metadata, err: err, expires: r.now().Add(ttl)}
	delete(r.inFlight, cacheKey)
	close(flight.done)
	r.mu.Unlock()
	if err != nil {
		return usageIdentity{}, err
	}
	return identityFromRuntimeMetadata(metadata, usage), nil
}

func sanitizeAuthRuntimeMetadata(metadata authRuntimeMetadata) authRuntimeMetadata {
	metadata.Provider = normalizeDimension(metadata.Provider)
	metadata.Type = normalizeDimension(metadata.Type)
	metadata.Email = safeAuthAccount(metadata.Email)
	metadata.AccountType = normalizeDimension(metadata.AccountType)
	if strings.EqualFold(metadata.AccountType, "oauth") {
		metadata.Account = safeAuthAccount(metadata.Account)
	} else {
		metadata.Account = ""
	}
	metadata.Label = safeAuthLabel(metadata.Label)
	return metadata
}

func identityFromRuntimeMetadata(metadata authRuntimeMetadata, usage Dimensions) usageIdentity {
	provider := displayAuthProvider(firstNonEmptyIdentity(metadata.Provider, metadata.Type, usage.Provider, usage.ExecutorType))
	account := safeAuthAccount(metadata.Email)
	if account == "" && strings.EqualFold(strings.TrimSpace(metadata.AccountType), "oauth") {
		account = safeAuthAccount(metadata.Account)
	}
	if account == "" {
		account = safeAuthLabel(metadata.Label)
	}
	if account == "" {
		account = safeAuthAccount(sanitizeDimensionsSource(usage).Source)
	}
	return usageIdentity{Provider: provider, Account: account}
}

func displayAuthProvider(value string) string {
	value = normalizeDimension(value)
	switch strings.ToLower(value) {
	case "codex":
		return "Codex"
	case "antigravity":
		return "Antigravity"
	case "xai", "x-ai", "grok":
		return "Grok"
	default:
		return value
	}
}

func safeAuthAccount(value string) string {
	value = normalizeDimension(value)
	if looksLikeCredential(value) {
		return ""
	}
	return value
}

func safeAuthLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || looksLikeCredential(value) {
		return ""
	}
	return normalizeDimension(value)
}

func firstNonEmptyIdentity(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (r *pluginRuntime) setAuthRuntimeLookup(lookup authRuntimeLookup) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if lookup == nil {
		r.authResolver = nil
		return
	}
	r.authResolver = newAuthIdentityResolver(lookup)
}

func (r *pluginRuntime) resolveUsageIdentity(usage *normalizedUsage) {
	if usage == nil || usage.authIndex == "" {
		return
	}
	defer func() { usage.authIndex = "" }()
	r.mu.RLock()
	resolver := r.authResolver
	r.mu.RUnlock()
	identity, err := resolver.resolve(usage.authIndex, usage.Dimensions)
	if err != nil {
		return
	}
	usage.Dimensions.Source = canonicalUsageSourceWithIdentity(usage.Dimensions, identity.Provider, identity.Account)
}
