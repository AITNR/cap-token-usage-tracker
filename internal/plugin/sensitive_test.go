package plugin

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCryptoContextAndAPIKeyCiphertext(t *testing.T) {
	secret := strings.Repeat("s", 32)
	ctx, err := deriveCryptoContext(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !ctx.enabled || ctx.usesDefaultSecret || len(ctx.keyID) != 32 {
		t.Fatalf("unexpected crypto context: %+v", ctx)
	}
	if ctx.encKey == ctx.indexKey {
		t.Fatal("domain-separated keys must differ")
	}

	key := "test-client-api-key"
	fingerprint := apiKeyFingerprint(key, ctx.indexKey)
	if len(fingerprint) != 32 || fingerprint != strings.ToLower(fingerprint) || fingerprint != apiKeyFingerprint(key, ctx.indexKey) {
		t.Fatalf("invalid deterministic fingerprint %q", fingerprint)
	}
	if fingerprint == apiKeyFingerprint(key+"-other", ctx.indexKey) {
		t.Fatal("different API keys produced the same test fingerprint")
	}

	first, err := encryptAPIKey(ctx, key, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	second, err := encryptAPIKey(ctx, key, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("AES-GCM encryption reused a nonce")
	}
	plain, err := decryptAPIKey(ctx, first, fingerprint)
	if err != nil || plain != key {
		t.Fatalf("decrypt = %q, %v", plain, err)
	}
	if _, err := decryptAPIKey(ctx, first, apiKeyFingerprint("wrong", ctx.indexKey)); err == nil {
		t.Fatal("ciphertext decrypted with the wrong fingerprint/AAD")
	}
	wrong, _ := deriveCryptoContext(strings.Repeat("x", 32))
	if _, err := decryptAPIKey(wrong, first, fingerprint); err == nil {
		t.Fatal("ciphertext decrypted with the wrong key")
	}

	raw, err := base64.RawURLEncoding.DecodeString(first)
	if err != nil {
		t.Fatal(err)
	}
	raw[0]++
	if _, err := decryptAPIKey(ctx, base64.RawURLEncoding.EncodeToString(raw), fingerprint); err == nil {
		t.Fatal("unsupported ciphertext version was accepted")
	}
	if _, err := decryptAPIKey(ctx, base64.RawURLEncoding.EncodeToString([]byte{apiKeyCipherVersion}), fingerprint); err == nil {
		t.Fatal("short ciphertext was accepted")
	}
}

func TestCryptoContextDefaultDisabledAndValidation(t *testing.T) {
	ctx, err := deriveCryptoContext(defaultAPIKeySecret)
	if err != nil || !ctx.enabled || !ctx.usesDefaultSecret {
		t.Fatalf("default context = %+v, %v", ctx, err)
	}
	disabled, err := deriveCryptoContext("")
	if err != nil || disabled.enabled || disabled.keyID != "" {
		t.Fatalf("disabled context = %+v, %v", disabled, err)
	}
	if _, err := deriveCryptoContext("short-secret"); err == nil {
		t.Fatal("short custom secret was accepted")
	}
}

func TestSensitiveRevealDegradesOnlyCorruptItem(t *testing.T) {
	ctx, err := deriveCryptoContext(strings.Repeat("s", 32))
	if err != nil {
		t.Fatal(err)
	}
	goodHash := apiKeyFingerprint("good-key", ctx.indexKey)
	badHash := apiKeyFingerprint("bad-key", ctx.indexKey)
	goodCiphertext, _ := encryptAPIKey(ctx, "good-key", goodHash)
	badCiphertext, _ := encryptAPIKey(ctx, "bad-key", badHash)
	stats := StatsResponse{
		Groups: []GroupStats{
			{Dimensions: Dimensions{APIKey: goodCiphertext, APIKeyHash: goodHash, APIKeyGeneration: 1}},
			{Dimensions: Dimensions{APIKey: badCiphertext, APIKeyHash: goodHash, APIKeyGeneration: 1}},
		},
		APIKeys: []APIKeyOption{
			{Ref: apiKeyRef(1, goodHash), Hash: goodHash, Generation: 1, Key: goodCiphertext},
			{Ref: apiKeyRef(1, badHash), Hash: badHash, Generation: 1, Key: badCiphertext + "corrupt"},
		},
	}
	stats.Reveal(func(ciphertext, fingerprint string, generation uint64) (string, string) {
		plaintext, err := decryptAPIKeyForGeneration(ctx, ciphertext, fingerprint, generation)
		if err != nil {
			return "", apiKeyStatusCiphertextInvalid
		}
		return plaintext, apiKeyStatusAvailable
	})
	if stats.Groups[0].APIKey != "good-key" || stats.APIKeys[0].Key != "good-key" {
		t.Fatalf("valid ciphertext did not reveal: %+v", stats)
	}
	if stats.Groups[1].APIKey != "" || stats.APIKeys[1].Key != "" {
		t.Fatalf("corrupt ciphertext was not isolated: %+v", stats)
	}
	if stats.Groups[1].APIKeyHash != goodHash || stats.APIKeys[1].Hash != badHash {
		t.Fatal("reveal failure removed the stable fingerprint")
	}
}
