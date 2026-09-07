package plugin

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	bolt "go.etcd.io/bbolt"
)

type blockingNonceReader struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingNonceReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	for index := range p {
		p[index] = byte(index + 1)
	}
	return len(p), nil
}

func encryptedUsageForTest(t *testing.T, ctx cryptoContext, key, model string, tokens uint64) normalizedUsage {
	return encryptedUsageForGeneration(t, ctx, 1, key, model, tokens)
}

func encryptedUsageForGeneration(t *testing.T, ctx cryptoContext, generation uint64, key, model string, tokens uint64) normalizedUsage {
	t.Helper()
	hash := apiKeyFingerprint(key, ctx.indexKey)
	ciphertext, err := encryptAPIKeyForGeneration(ctx, key, hash, generation)
	if err != nil {
		t.Fatal(err)
	}
	return normalizedUsage{
		RequestedAt: time.Now().UTC().Add(-time.Second),
		Dimensions:  Dimensions{Model: model, Source: "test", APIKey: ciphertext, APIKeyHash: hash, APIKeyGeneration: generation},
		Counters:    Counters{Requests: 1, TotalTokens: tokens},
	}
}

func revealStatsForCrypto(stats *StatsResponse, ctx cryptoContext, generations map[uint64]APIKeyCryptoGeneration) {
	stats.Reveal(func(ciphertext, fingerprint string, generation uint64) (string, string) {
		metadata, ok := generations[generation]
		if !ok || metadata.IdentityMissing {
			return "", apiKeyStatusIdentityMissing
		}
		if !ctx.enabled || metadata.KeyID != ctx.keyID {
			return "", apiKeyStatusGenerationUnavailable
		}
		plaintext, err := decryptAPIKeyForGeneration(ctx, ciphertext, fingerprint, generation)
		if err != nil {
			return "", apiKeyStatusCiphertextInvalid
		}
		return plaintext, apiKeyStatusAvailable
	})
}

func TestAPIKeyCryptoGenerationsRotateWithoutDataLoss(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = strings.Repeat("a", 32)
	config.SyncOnRecord = true
	ctxA, _ := deriveCryptoContext(config.APIKeySecret)
	store, err := openStoreWithCrypto(config, ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Record(encryptedUsageForGeneration(t, ctxA, 1, "identity-test-key", "original", 7)); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	configB := config
	configB.APIKeySecret = strings.Repeat("b", 32)
	ctxB, _ := deriveCryptoContext(configB.APIKeySecret)
	if err := store.ReconfigureWithCrypto(configB, ctxB); err != nil {
		t.Fatal(err)
	}
	generationB, generations := store.APIKeyCryptoState()
	if generationB != 2 || len(generations) != 2 {
		t.Fatalf("generation state = %d, %+v", generationB, generations)
	}
	if err := store.Record(encryptedUsageForGeneration(t, ctxB, generationB, "replacement-key", "replacement", 5)); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Query("24h")
	if err != nil || stats.Summary.TotalTokens != 12 || stats.Summary.Requests != 2 {
		t.Fatalf("rotated stats = %+v, %v", stats, err)
	}
	revealStatsForCrypto(&stats, ctxB, generations)
	statuses := map[uint64]string{}
	for _, option := range stats.APIKeys {
		statuses[option.Generation] = option.Status
	}
	if statuses[1] != apiKeyStatusGenerationUnavailable || statuses[2] != apiKeyStatusAvailable {
		t.Fatalf("B reveal statuses = %+v", statuses)
	}

	if err := store.ReconfigureWithCrypto(config, ctxA); err != nil {
		t.Fatal(err)
	}
	generationA, generations := store.APIKeyCryptoState()
	if generationA != 1 || len(generations) != 2 {
		t.Fatalf("reactivated generation = %d, %+v", generationA, generations)
	}
	stats, err = store.Query("24h")
	if err != nil || stats.Summary.TotalTokens != 12 {
		t.Fatalf("reactivated stats = %+v, %v", stats, err)
	}
	revealStatsForCrypto(&stats, ctxA, generations)
	statuses = map[uint64]string{}
	for _, option := range stats.APIKeys {
		statuses[option.Generation] = option.Status
	}
	if statuses[1] != apiKeyStatusAvailable || statuses[2] != apiKeyStatusGenerationUnavailable {
		t.Fatalf("A reveal statuses = %+v", statuses)
	}

	disabled := config
	disabled.APIKeySecret = ""
	if err := store.Reconfigure(disabled); err != nil {
		t.Fatal(err)
	}
	stats, err = store.Query("24h")
	if err != nil || stats.Summary.TotalTokens != 12 {
		t.Fatalf("disabled tracking lost data: %+v, %v", stats, err)
	}
}

func TestRestoreAcceptsDifferentCryptoIdentityAndPreservesHistory(t *testing.T) {
	configA := testConfig(t)
	configA.APIKeySecret = strings.Repeat("a", 32)
	configA.SyncOnRecord = true
	ctxA, _ := deriveCryptoContext(configA.APIKeySecret)
	storeA, err := openStoreWithCrypto(configA, ctxA)
	if err != nil {
		t.Fatal(err)
	}
	plainA := "restore-source-api-key"
	if err := storeA.Record(encryptedUsageForTest(t, ctxA, plainA, "source", 11)); err != nil {
		t.Fatal(err)
	}
	backup, err := storeA.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if err := storeA.Close(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(backup, []byte(plainA)) {
		t.Fatal("source backup contains plaintext API key")
	}

	configB := testConfig(t)
	configB.APIKeySecret = strings.Repeat("b", 32)
	configB.SyncOnRecord = true
	ctxB, _ := deriveCryptoContext(configB.APIKeySecret)
	storeB, err := openStoreWithCrypto(configB, ctxB)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	if err := storeB.RestoreBackup(backup); err != nil {
		t.Fatalf("restore rejected a different crypto identity: %v", err)
	}
	stats, err := storeB.Query("24h")
	if err != nil || stats.Summary.TotalTokens != 11 || len(stats.Groups) != 1 || stats.Groups[0].Model != "source" {
		t.Fatalf("restored state = %+v, %v", stats, err)
	}
	generationB, generations := storeB.APIKeyCryptoState()
	if generationB == 0 || len(generations) != 2 {
		t.Fatalf("restored generations = %d, %+v", generationB, generations)
	}
	revealStatsForCrypto(&stats, ctxB, generations)
	if len(stats.APIKeys) != 1 || stats.APIKeys[0].Status != apiKeyStatusGenerationUnavailable {
		t.Fatalf("restored old key status = %+v", stats.APIKeys)
	}
}

func TestReconfigureCryptoIdentityRollsBackWithFailedCandidateFlush(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = ""
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	actor := &storeActor{
		db:                   db,
		config:               config,
		data:                 make(map[aggregateKey]Counters),
		dirty:                make(map[aggregateKey]struct{}),
		modelPrices:          make(map[string]ModelPrice),
		dashboardPreferences: defaultDashboardPreferences(),
		apiKeyCiphertexts:    make(map[string]string),
		apiKeyLabels:         make(map[string]string),
	}
	if err := actor.initialize(); err != nil {
		t.Fatal(err)
	}
	defer actor.db.Close()

	if err := actor.db.Update(func(tx *bolt.Tx) error {
		return tx.DeleteBucket(metaBucket)
	}); err != nil {
		t.Fatal(err)
	}

	candidate := config
	candidate.APIKeySecret = strings.Repeat("c", 32)
	candidateCrypto, err := deriveCryptoContext(candidate.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.reconfigure(candidate, candidateCrypto); err == nil {
		t.Fatal("reconfigure succeeded despite candidate flush failure")
	}
	if actor.config.APIKeySecret != "" || actor.crypto.enabled {
		t.Fatalf("actor retained failed candidate config: config=%q enabled=%v", actor.config.APIKeySecret, actor.crypto.enabled)
	}
	if actor.activeGeneration != 0 || len(actor.generations) != 0 {
		t.Fatalf("failed reconfigure changed generations: %d %+v", actor.activeGeneration, actor.generations)
	}
}

func TestSchemaSixDatabaseMigratesToCryptoIdentity(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = strings.Repeat("d", 32)
	ctx, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(6)); err != nil {
			return err
		}
		if err := meta.Put(sinceKey, encodeInt64(time.Now().UTC().UnixNano())); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(requestsBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStoreWithCrypto(config, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = bolt.Open(config.DataPath, 0o600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if decodeUint64(meta.Get(schemaKey)) != persistenceSchemaVersion {
			t.Fatalf("schema version = %d", decodeUint64(meta.Get(schemaKey)))
		}
		generations, err := loadAPIKeyGenerations(meta)
		if err != nil {
			return err
		}
		if len(generations) != 1 || generations[1].KeyID != ctx.keyID || generations[1].HashVersion != apiKeyHashVersion {
			t.Fatalf("schema-six generations = %+v", generations)
		}
		if len(meta.Get(cryptoKeyIDKey)) != 0 || len(meta.Get(apiKeyHashVersionKey)) != 0 {
			t.Fatal("legacy crypto identity metadata was not removed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationPreservesAPIKeyDataWithoutCryptoIdentity(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = defaultAPIKeySecret
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Minute)
	dimensions, err := json.Marshal(Dimensions{Model: "legacy", APIKeyHash: strings.Repeat("a", 32)})
	if err != nil {
		t.Fatal(err)
	}
	counters, err := json.Marshal(Counters{Requests: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(6)); err != nil {
			return err
		}
		if err := meta.Put(sinceKey, encodeInt64(now.UnixNano())); err != nil {
			return err
		}
		hours, err := tx.CreateBucket(hoursBucket)
		if err != nil {
			return err
		}
		hour, err := hours.CreateBucket(encodeInt64(now.Unix()))
		if err != nil {
			return err
		}
		if err := hour.Put(dimensions, counters); err != nil {
			return err
		}
		_, err = tx.CreateBucket(requestsBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStore(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	active, generations := store.APIKeyCryptoState()
	if active != 2 || len(generations) != 2 || !generations[1].IdentityMissing {
		t.Fatalf("migrated generations = active %d, %+v", active, generations)
	}
	stats, err := store.Query("24h")
	if err != nil || stats.Summary.Requests != 1 || len(stats.APIKeys) != 1 || stats.APIKeys[0].Generation != 1 {
		t.Fatalf("migrated stats = %+v, %v", stats, err)
	}
	ctx, _ := deriveCryptoContext(config.APIKeySecret)
	revealStatsForCrypto(&stats, ctx, generations)
	if stats.APIKeys[0].Status != apiKeyStatusIdentityMissing {
		t.Fatalf("legacy status = %+v", stats.APIKeys[0])
	}
}

func TestSchemaSevenMigrationMergesAggregatesAndPreservesSensitiveMetadata(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = strings.Repeat("m", 32)
	ctx, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	plainKey := "schema-seven-client-key"
	hash := apiKeyFingerprint(plainKey, ctx.indexKey)
	ciphertext, err := encryptAPIKey(ctx, plainKey, hash)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Minute)
	request := RequestDetail{
		Sequence: 1,
		Time:     now,
		Dimensions: Dimensions{
			Model:      "legacy-model",
			APIKey:     ciphertext,
			APIKeyHash: hash,
		},
		Counters: Counters{Requests: 1, TotalTokens: 9},
		Result:   "成功",
	}
	requestValue, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	labelsValue, err := json.Marshal(map[string]string{hash: "legacy label"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		for key, value := range map[string][]byte{
			string(schemaKey):            encodeUint64(7),
			string(sinceKey):             encodeInt64(now.UnixNano()),
			string(requestSequenceKey):   encodeUint64(1),
			string(cryptoKeyIDKey):       []byte(ctx.keyID),
			string(apiKeyHashVersionKey): []byte(apiKeyHashVersion),
			string(apiKeyLabelsKey):      labelsValue,
		} {
			if err := meta.Put([]byte(key), value); err != nil {
				return err
			}
		}
		hours, err := tx.CreateBucket(hoursBucket)
		if err != nil {
			return err
		}
		hour, err := hours.CreateBucket(encodeInt64(now.Unix()))
		if err != nil {
			return err
		}
		firstDimensions := []byte(`{"model":"legacy-model","api_key_hash":"` + hash + `"}`)
		secondDimensions := []byte(`{"api_key_hash":"` + hash + `","model":"legacy-model"}`)
		firstCounters, _ := json.Marshal(Counters{Requests: 2, InputTokens: 3, TotalTokens: 3})
		secondCounters, _ := json.Marshal(Counters{Requests: 3, OutputTokens: 4, TotalTokens: 4})
		if err := hour.Put(firstDimensions, firstCounters); err != nil {
			return err
		}
		if err := hour.Put(secondDimensions, secondCounters); err != nil {
			return err
		}
		requests, err := tx.CreateBucket(requestsBucket)
		if err != nil {
			return err
		}
		return requests.Put(encodeRequestKey(now.UnixNano(), 1), requestValue)
	}); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := openStoreWithCrypto(config, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	active, generations := store.APIKeyCryptoState()
	if active != 1 || len(generations) != 1 || generations[1].KeyID != ctx.keyID {
		t.Fatalf("migrated crypto state = %d, %+v", active, generations)
	}
	stats, err := store.Query("24h")
	if err != nil || stats.Summary.Requests != 5 || stats.Summary.InputTokens != 3 || stats.Summary.OutputTokens != 4 || stats.Summary.TotalTokens != 7 || len(stats.Groups) != 1 {
		t.Fatalf("migrated aggregate stats = %+v, %v", stats, err)
	}
	revealStatsForCrypto(&stats, ctx, generations)
	if len(stats.APIKeys) != 1 || stats.APIKeys[0].Ref != apiKeyRef(1, hash) || stats.APIKeys[0].Key != plainKey || stats.APIKeys[0].Status != apiKeyStatusAvailable {
		t.Fatalf("migrated aggregate API key = %+v", stats.APIKeys)
	}
	page, err := store.QueryRequests("24h", 0, 10, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].APIKeyGeneration != 1 {
		t.Fatalf("migrated requests = %+v, %v", page, err)
	}
	page.Reveal(func(ciphertext, fingerprint string, generation uint64) (string, string) {
		plaintext, decryptErr := decryptAPIKeyForGeneration(ctx, ciphertext, fingerprint, generation)
		if decryptErr != nil {
			return "", apiKeyStatusCiphertextInvalid
		}
		return plaintext, apiKeyStatusAvailable
	})
	if page.Items[0].APIKey != plainKey || page.Items[0].APIKeyStatus != apiKeyStatusAvailable {
		t.Fatalf("migrated v1 request ciphertext = %+v", page.Items[0])
	}
	labels, err := store.APIKeyLabels()
	if err != nil || labels[apiKeyRef(1, hash)] != "legacy label" || len(labels) != 1 {
		t.Fatalf("migrated labels = %+v, %v", labels, err)
	}
}

func TestAPIKeyGenerationRegistryRejectsAmbiguousMetadata(t *testing.T) {
	config := testConfig(t)
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, err := deriveCryptoContext(strings.Repeat("v", 32))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		duplicate, err := json.Marshal([]APIKeyCryptoGeneration{
			{ID: 1, KeyID: ctx.keyID, HashVersion: apiKeyHashVersion},
			{ID: 2, KeyID: ctx.keyID, HashVersion: apiKeyHashVersion},
		})
		if err != nil {
			return err
		}
		if err := meta.Put(apiKeyGenerationsKey, duplicate); err != nil {
			return err
		}
		if _, err := loadAPIKeyGenerations(meta); err == nil || !strings.Contains(err.Error(), "duplicate identities") {
			t.Fatalf("duplicate generation identity error = %v", err)
		}
		generations := map[uint64]APIKeyCryptoGeneration{
			2: {ID: 2, KeyID: ctx.keyID, HashVersion: apiKeyHashVersion},
		}
		if err := meta.Put(apiKeyNextGenerationKey, encodeUint64(2)); err != nil {
			return err
		}
		candidate, err := deriveCryptoContext(strings.Repeat("w", 32))
		if err != nil {
			return err
		}
		if _, _, err := activateAPIKeyGeneration(meta, generations, candidate, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "does not follow existing generation") {
			t.Fatalf("regressed generation sequence error = %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDegradesIncompleteCryptoIdentityMetadata(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = defaultAPIKeySecret
	ctx, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucket(metaBucket)
		if err != nil {
			return err
		}
		if err := meta.Put(schemaKey, encodeUint64(7)); err != nil {
			return err
		}
		if err := meta.Put(sinceKey, encodeInt64(time.Now().UTC().UnixNano())); err != nil {
			return err
		}
		if err := meta.Put(cryptoKeyIDKey, []byte(ctx.keyID)); err != nil {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		_, err = tx.CreateBucket(requestsBucket)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := openStoreWithCrypto(config, ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	active, generations := store.APIKeyCryptoState()
	if active != 2 || len(generations) != 2 || !generations[1].IdentityMissing {
		t.Fatalf("incomplete identity generations = active %d, %+v", active, generations)
	}
}

func TestRestoreResponsePublishesActiveGenerationBeforeNextUsage(t *testing.T) {
	sourceConfig := testConfig(t)
	sourceConfig.APIKeySecret = strings.Repeat("r", 32)
	sourceConfig.SyncOnRecord = true
	sourceCrypto, err := deriveCryptoContext(sourceConfig.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	sourceStore, err := openStoreWithCrypto(sourceConfig, sourceCrypto)
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceStore.Record(encryptedUsageForGeneration(t, sourceCrypto, 1, "restore-old-key", "before-restore", 1)); err != nil {
		t.Fatal(err)
	}
	backup, err := sourceStore.Backup()
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceStore.Close(); err != nil {
		t.Fatal(err)
	}

	targetConfig := testConfig(t)
	targetConfig.APIKeySecret = strings.Repeat("s", 32)
	targetConfig.SyncOnRecord = true
	targetCrypto, err := deriveCryptoContext(targetConfig.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	targetStore, err := openStoreWithCrypto(targetConfig, targetCrypto)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: targetStore, config: targetConfig, crypto: targetCrypto}
	runtime.apiKeyGeneration, runtime.apiKeyGenerations = targetStore.APIKeyCryptoState()
	defer runtime.shutdown()

	response, err := runtime.restoreResponse(pluginapi.ManagementRequest{
		Method: http.MethodPost,
		Headers: http.Header{
			"Content-Type":      []string{"application/octet-stream"},
			"X-Confirm-Restore": []string{"replace"},
		},
		Body: backup,
	})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("restore response = %+v, %v", response, err)
	}
	storeGeneration, generations := targetStore.APIKeyCryptoState()
	runtime.mu.RLock()
	runtimeGeneration := runtime.apiKeyGeneration
	runtime.mu.RUnlock()
	if storeGeneration == 0 || runtimeGeneration != storeGeneration || len(generations) != 2 {
		t.Fatalf("post-restore generations = runtime %d, store %d, %+v", runtimeGeneration, storeGeneration, generations)
	}
	plainKey := "restore-new-key"
	raw, _ := json.Marshal(pluginapi.UsageRecord{
		Model:       "after-restore",
		APIKey:      plainKey,
		RequestedAt: time.Now().UTC(),
		Detail:      pluginapi.UsageDetail{TotalTokens: 2},
	})
	if _, err := runtime.handleUsage(raw); err != nil {
		t.Fatal(err)
	}
	page, err := targetStore.QueryRequests("24h", 0, 10, "after-restore")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("post-restore request page = %+v, %v", page, err)
	}
	item := page.Items[0]
	if item.APIKeyGeneration != storeGeneration {
		t.Fatalf("post-restore request generation = %d, want %d", item.APIKeyGeneration, storeGeneration)
	}
	revealed, err := decryptAPIKeyForGeneration(targetCrypto, item.APIKey, item.APIKeyHash, item.APIKeyGeneration)
	if err != nil || revealed != plainKey {
		t.Fatalf("post-restore API key = %q, %v", revealed, err)
	}
}

func TestResetResponsePublishesRecreatedGenerationBeforeNextUsage(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = strings.Repeat("t", 32)
	config.SyncOnRecord = true
	crypto, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStoreWithCrypto(config, crypto)
	if err != nil {
		t.Fatal(err)
	}
	rotatedConfig := config
	rotatedConfig.APIKeySecret = strings.Repeat("u", 32)
	rotatedCrypto, err := deriveCryptoContext(rotatedConfig.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReconfigureWithCrypto(rotatedConfig, rotatedCrypto); err != nil {
		t.Fatal(err)
	}
	beforeReset, _ := store.APIKeyCryptoState()
	if beforeReset != 2 {
		t.Fatalf("pre-reset generation = %d, want 2", beforeReset)
	}
	runtime := &pluginRuntime{store: store, config: rotatedConfig, crypto: rotatedCrypto}
	runtime.apiKeyGeneration, runtime.apiKeyGenerations = store.APIKeyCryptoState()
	defer runtime.shutdown()

	response, err := runtime.resetResponse(pluginapi.ManagementRequest{
		Method:  http.MethodPost,
		Headers: http.Header{"Content-Type": []string{"application/json"}},
		Body:    []byte(`{"confirm":"reset"}`),
	})
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("reset response = %+v, %v", response, err)
	}
	storeGeneration, generations := store.APIKeyCryptoState()
	runtime.mu.RLock()
	runtimeGeneration := runtime.apiKeyGeneration
	runtime.mu.RUnlock()
	if storeGeneration != 1 || runtimeGeneration != storeGeneration || len(generations) != 1 {
		t.Fatalf("post-reset generations = runtime %d, store %d, %+v", runtimeGeneration, storeGeneration, generations)
	}
	plainKey := "reset-new-key"
	raw, _ := json.Marshal(pluginapi.UsageRecord{
		Model:       "after-reset",
		APIKey:      plainKey,
		RequestedAt: time.Now().UTC(),
		Detail:      pluginapi.UsageDetail{TotalTokens: 3},
	})
	if _, err := runtime.handleUsage(raw); err != nil {
		t.Fatal(err)
	}
	page, err := store.QueryRequests("24h", 0, 10, "after-reset")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("post-reset request page = %+v, %v", page, err)
	}
	item := page.Items[0]
	if item.APIKeyGeneration != storeGeneration {
		t.Fatalf("post-reset request generation = %d, want %d", item.APIKeyGeneration, storeGeneration)
	}
	revealed, err := decryptAPIKeyForGeneration(rotatedCrypto, item.APIKey, item.APIKeyHash, item.APIKeyGeneration)
	if err != nil || revealed != plainKey {
		t.Fatalf("post-reset API key = %q, %v", revealed, err)
	}
}

func TestConcurrentUsageKeepsInFlightCryptoGeneration(t *testing.T) {
	config := testConfig(t)
	config.APIKeySecret = strings.Repeat("e", 32)
	config.SyncOnRecord = true
	ctx, err := deriveCryptoContext(config.APIKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	store, err := openStoreWithCrypto(config, ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &pluginRuntime{store: store, config: config, crypto: ctx}
	defer runtime.shutdown()
	if err := store.Reset(); err != nil {
		t.Fatal(err)
	}

	reader := &blockingNonceReader{entered: make(chan struct{}), release: make(chan struct{})}
	originalReader := cryptorand.Reader
	cryptorand.Reader = reader
	defer func() { cryptorand.Reader = originalReader }()

	plainKey := "concurrent-reset-window-key"
	raw, err := json.Marshal(pluginapi.UsageRecord{
		Model:       "concurrent-model",
		APIKey:      plainKey,
		RequestedAt: time.Now().UTC(),
		Detail:      pluginapi.UsageDetail{TotalTokens: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	usageResult := make(chan error, 1)
	go func() {
		_, err := runtime.handleUsage(raw)
		usageResult <- err
	}()
	<-reader.entered

	candidate := config
	candidate.APIKeySecret = strings.Repeat("f", 32)
	reconfigureResult := make(chan error, 1)
	go func() { reconfigureResult <- runtime.applyConfig(candidate) }()
	close(reader.release)
	if err := <-usageResult; err != nil {
		t.Fatalf("usage failed: %v", err)
	}
	if err := <-reconfigureResult; err != nil {
		t.Fatalf("concurrent reconfigure failed: %v", err)
	}

	page, err := store.QueryRequests("24h", 0, 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("request page = %+v, %v", page, err)
	}
	if page.Items[0].APIKeyGeneration != 1 {
		t.Fatalf("in-flight request generation = %d", page.Items[0].APIKeyGeneration)
	}
	revealed, err := decryptAPIKeyForGeneration(ctx, page.Items[0].APIKey, page.Items[0].APIKeyHash, page.Items[0].APIKeyGeneration)
	if err != nil || revealed != plainKey {
		t.Fatalf("persisted request was not encrypted with the active generation: %q, %v", revealed, err)
	}
	runtime.mu.RLock()
	activeSecret := runtime.config.APIKeySecret
	activeKeyID := runtime.crypto.keyID
	runtime.mu.RUnlock()
	if activeSecret != candidate.APIKeySecret || activeKeyID == ctx.keyID {
		t.Fatal("successful concurrent reconfigure did not publish the replacement generation")
	}
}
