package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	metaBucket            = []byte("meta")
	hoursBucket           = []byte("hours")
	requestsBucket        = []byte("requests")
	schemaKey             = []byte("schema_version")
	sinceKey              = []byte("since_unix_nano")
	lastUsedKey           = []byte("last_used_unix_nano")
	requestSequenceKey    = []byte("request_sequence")
	modelPricesKey        = []byte("model_prices")
	modelPriceRevisionKey = []byte("model_price_revision")
	modelPriceSettingsKey = []byte("model_price_sync_settings")
	modelPriceLastSyncKey = []byte("model_price_last_sync")
)

const persistenceSchemaVersion uint64 = 4

type recordCommand struct {
	usage normalizedUsage
	resp  chan error
}

type queryCommand struct {
	rangeName string
	resp      chan queryResult
}

type queryResult struct {
	stats StatsResponse
	err   error
}

type requestQueryCommand struct {
	rangeName string
	offset    int
	limit     int
	model     string
	resp      chan requestQueryResult
}

type requestQueryResult struct {
	page RequestPage
	err  error
}

type priceQueryCommand struct{ resp chan priceQueryResult }
type priceQueryResult struct {
	response ModelPricesResponse
	err      error
}
type savePricesCommand struct {
	prices   map[string]ModelPrice
	settings *PriceSyncSettings
	resp     chan priceQueryResult
}
type syncPricesCommand struct {
	prices           map[string]ModelPrice
	settings         PriceSyncSettings
	metadata         PriceSyncMetadata
	expectedRevision uint64
	resp             chan priceQueryResult
}
type observedModelsCommand struct {
	now  time.Time
	resp chan observedModelsResult
}
type observedModelsResult struct {
	models []string
	err    error
}
type costSnapshotCommand struct {
	rangeName string
	resp      chan costSnapshotResult
}
type costSnapshotResult struct {
	snapshot costQuerySnapshot
	err      error
}

type resetCommand struct{ resp chan error }
type configCommand struct {
	config Config
	resp   chan error
}
type closeCommand struct{ resp chan error }

type Store struct {
	db           *bolt.DB
	commands     chan any
	done         chan struct{}
	closeOnce    sync.Once
	stateMu      sync.RWMutex
	costMu       sync.Mutex
	costCache    map[costCacheKey]CostResponse
	costOrder    []costCacheKey
	costFlights  map[costCacheKey]*costFlight
	costScanHook func()
	closed       bool
	closeErr     error
}

type storeActor struct {
	db                *bolt.DB
	config            Config
	data              map[aggregateKey]Counters
	dirty             map[aggregateKey]struct{}
	since             time.Time
	lastUsed          time.Time
	pending           int
	lastPruneAt       time.Time
	lastFlushErr      error
	pendingRequests   []RequestDetail
	nextRequestSeq    uint64
	modelPrices       map[string]ModelPrice
	priceRevision     uint64
	priceSyncSettings PriceSyncSettings
	lastPriceSync     *PriceSyncMetadata
	costGeneration    uint64
}

func openStore(config Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(config.DataPath), 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := bolt.Open(config.DataPath, 0o600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	actor := &storeActor{
		db:     db,
		config: config,
		data:   make(map[aggregateKey]Counters),
		dirty:  make(map[aggregateKey]struct{}),
	}
	if err := actor.initialize(); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &Store{
		db:          db,
		commands:    make(chan any, 256),
		done:        make(chan struct{}),
		costCache:   make(map[costCacheKey]CostResponse),
		costFlights: make(map[costCacheKey]*costFlight),
	}
	go store.run(actor)
	return store, nil
}

func (s *Store) Record(usage normalizedUsage) error {
	resp := make(chan error, 1)
	if err := s.send(recordCommand{usage: usage, resp: resp}); err != nil {
		return err
	}
	return <-resp
}

func (s *Store) Query(rangeName string) (StatsResponse, error) {
	resp := make(chan queryResult, 1)
	if err := s.send(queryCommand{rangeName: rangeName, resp: resp}); err != nil {
		return StatsResponse{}, err
	}
	result := <-resp
	return result.stats, result.err
}

func (s *Store) QueryRequests(rangeName string, offset, limit int, model string) (RequestPage, error) {
	resp := make(chan requestQueryResult, 1)
	if err := s.send(requestQueryCommand{rangeName: rangeName, offset: offset, limit: limit, model: model, resp: resp}); err != nil {
		return RequestPage{}, err
	}
	result := <-resp
	return result.page, result.err
}

func (s *Store) QueryModelPrices() (map[string]ModelPrice, error) {
	response, err := s.QueryPriceBook()
	return response.Prices, err
}

func (s *Store) QueryPriceBook() (ModelPricesResponse, error) {
	resp := make(chan priceQueryResult, 1)
	if err := s.send(priceQueryCommand{resp: resp}); err != nil {
		return ModelPricesResponse{}, err
	}
	result := <-resp
	return result.response, result.err
}

func (s *Store) SaveModelPrices(prices map[string]ModelPrice) (map[string]ModelPrice, error) {
	response, err := s.SavePriceBook(prices, nil)
	return response.Prices, err
}

func (s *Store) SavePriceBook(prices map[string]ModelPrice, settings *PriceSyncSettings) (ModelPricesResponse, error) {
	normalized, err := normalizeModelPrices(cloneModelPrices(prices))
	if err != nil {
		return ModelPricesResponse{}, withStatus(400, "%v", err)
	}
	if settings != nil {
		normalizedSettings, err := normalizePriceSyncSettings(*settings)
		if err != nil {
			return ModelPricesResponse{}, withStatus(400, "%v", err)
		}
		settings = &normalizedSettings
	}
	resp := make(chan priceQueryResult, 1)
	if err := s.send(savePricesCommand{prices: normalized, settings: settings, resp: resp}); err != nil {
		return ModelPricesResponse{}, err
	}
	result := <-resp
	return result.response, result.err
}

func (s *Store) ApplyModelPriceSync(prices map[string]ModelPrice, settings PriceSyncSettings, metadata PriceSyncMetadata, expectedRevision uint64) (ModelPricesResponse, error) {
	normalized, err := normalizeModelPrices(prices)
	if err != nil {
		return ModelPricesResponse{}, withStatus(400, "%v", err)
	}
	normalizedSettings, err := normalizePriceSyncSettings(settings)
	if err != nil {
		return ModelPricesResponse{}, withStatus(400, "%v", err)
	}
	resp := make(chan priceQueryResult, 1)
	if err := s.send(syncPricesCommand{prices: normalized, settings: normalizedSettings, metadata: metadata, expectedRevision: expectedRevision, resp: resp}); err != nil {
		return ModelPricesResponse{}, err
	}
	result := <-resp
	return result.response, result.err
}

func (s *Store) ObservedModels() ([]string, error) {
	resp := make(chan observedModelsResult, 1)
	if err := s.send(observedModelsCommand{now: time.Now().UTC(), resp: resp}); err != nil {
		return nil, err
	}
	result := <-resp
	return result.models, result.err
}

func (s *Store) Reset() error {
	resp := make(chan error, 1)
	if err := s.send(resetCommand{resp: resp}); err != nil {
		return err
	}
	return <-resp
}

func (s *Store) Reconfigure(config Config) error {
	resp := make(chan error, 1)
	if err := s.send(configCommand{config: config, resp: resp}); err != nil {
		return err
	}
	return <-resp
}

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		resp := make(chan error, 1)
		s.stateMu.Lock()
		if s.closed {
			s.stateMu.Unlock()
			return
		}
		s.closed = true
		s.commands <- closeCommand{resp: resp}
		s.stateMu.Unlock()
		s.closeErr = <-resp
		<-s.done
	})
	return s.closeErr
}

func (s *Store) send(command any) error {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if s.closed {
		return errors.New("store is closed")
	}
	s.commands <- command
	return nil
}

func (s *Store) run(actor *storeActor) {
	ticker := time.NewTicker(actor.config.FlushInterval)
	defer ticker.Stop()
	defer close(s.done)

	for {
		select {
		case command := <-s.commands:
			switch item := command.(type) {
			case recordCommand:
				// Always accept the new usage into the dirty in-memory aggregate. A
				// previous transient flush failure must not make subsequent usage vanish.
				item.resp <- actor.record(item.usage)
			case queryCommand:
				if err := actor.retryFailedFlush(time.Now().UTC()); err != nil {
					item.resp <- queryResult{err: err}
					continue
				}
				stats, err := buildStats(actor.data, actor.since, actor.lastUsed, item.rangeName, time.Now().UTC())
				item.resp <- queryResult{stats: stats, err: err}
			case requestQueryCommand:
				now := time.Now().UTC()
				if err := actor.flush(now, true); err != nil {
					actor.lastFlushErr = err
					item.resp <- requestQueryResult{err: err}
					continue
				}
				page, err := actor.queryRequests(item.rangeName, item.offset, item.limit, item.model, now)
				item.resp <- requestQueryResult{page: page, err: err}
			case priceQueryCommand:
				item.resp <- priceQueryResult{response: actor.priceBookResponse()}
			case savePricesCommand:
				response, err := actor.saveModelPrices(item.prices, item.settings)
				item.resp <- priceQueryResult{response: response, err: err}
			case syncPricesCommand:
				response, err := actor.applyModelPriceSync(item.prices, item.settings, item.metadata, item.expectedRevision)
				item.resp <- priceQueryResult{response: response, err: err}
			case observedModelsCommand:
				if err := actor.flush(item.now, true); err != nil {
					actor.lastFlushErr = err
					item.resp <- observedModelsResult{err: err}
					continue
				}
				item.resp <- observedModelsResult{models: actor.observedModels(item.now)}
			case costSnapshotCommand:
				now := time.Now().UTC()
				if err := actor.flush(now, true); err != nil {
					actor.lastFlushErr = err
					item.resp <- costSnapshotResult{err: err}
					continue
				}
				rangeName, cutoff, err := queryCutoff(item.rangeName, now)
				item.resp <- costSnapshotResult{snapshot: costQuerySnapshot{
					Range:         rangeName,
					Cutoff:        cutoff,
					GeneratedAt:   now,
					Prices:        cloneModelPrices(actor.modelPrices),
					PriceRevision: actor.priceRevision,
					HighWater:     actor.nextRequestSeq,
					Generation:    actor.costGeneration,
				}, err: err}
			case resetCommand:
				if err := actor.retryFailedFlush(time.Now().UTC()); err != nil {
					item.resp <- err
					continue
				}
				item.resp <- actor.reset()
			case configCommand:
				if err := actor.retryFailedFlush(time.Now().UTC()); err != nil {
					item.resp <- err
					continue
				}
				err := actor.reconfigure(item.config)
				if err == nil {
					ticker.Reset(item.config.FlushInterval)
				}
				item.resp <- err
			case closeCommand:
				flushErr := actor.flush(time.Now().UTC(), true)
				closeErr := actor.db.Close()
				item.resp <- errors.Join(flushErr, closeErr)
				return
			}
		case now := <-ticker.C:
			actor.lastFlushErr = actor.flush(now.UTC(), false)
		}
	}
}

func (a *storeActor) initialize() error {
	now := time.Now().UTC()
	if err := a.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(metaBucket)
		if err != nil {
			return err
		}
		hours, err := tx.CreateBucketIfNotExists(hoursBucket)
		if err != nil {
			return err
		}
		requests, err := tx.CreateBucketIfNotExists(requestsBucket)
		if err != nil {
			return err
		}
		version := decodeUint64(meta.Get(schemaKey))
		if version > persistenceSchemaVersion {
			return fmt.Errorf("unsupported database schema version %d", version)
		}
		if err := migratePriceMetadata(meta, version); err != nil {
			return err
		}
		var since time.Time
		if raw := meta.Get(sinceKey); len(raw) == 8 {
			since = time.Unix(0, decodeInt64(raw)).UTC()
		} else {
			since = now
		}
		cutoff := retentionCutoff(a.config, now)
		cutoffTime := time.Unix(cutoff, 0).UTC()
		if cutoffTime.After(since) {
			since = cutoffTime
		}
		if err := meta.Put(sinceKey, encodeInt64(since.UnixNano())); err != nil {
			return err
		}
		if err := pruneHoursBucket(hours, cutoff); err != nil {
			return err
		}
		if err := pruneRequestsBucket(requests, time.Unix(cutoff, 0).UTC().UnixNano()); err != nil {
			return err
		}
		return meta.Put(schemaKey, encodeUint64(persistenceSchemaVersion))
	}); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}

	return a.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		hours := tx.Bucket(hoursBucket)
		requests := tx.Bucket(requestsBucket)
		if meta == nil || hours == nil || requests == nil {
			return errors.New("database buckets are missing")
		}
		a.since = time.Unix(0, decodeInt64(meta.Get(sinceKey))).UTC()
		a.nextRequestSeq = decodeUint64(meta.Get(requestSequenceKey))
		a.modelPrices = make(map[string]ModelPrice)
		if raw := meta.Get(modelPricesKey); len(raw) > 0 {
			var stored map[string]ModelPrice
			if err := json.Unmarshal(raw, &stored); err != nil {
				return fmt.Errorf("decode model prices: %w", err)
			}
			normalized, err := normalizeModelPrices(stored)
			if err != nil {
				return fmt.Errorf("validate model prices: %w", err)
			}
			a.modelPrices = normalized
		}
		a.priceRevision = decodeUint64(meta.Get(modelPriceRevisionKey))
		if a.priceRevision == 0 && len(a.modelPrices) > 0 {
			a.priceRevision = 1
		}
		a.priceSyncSettings = defaultPriceSyncSettings()
		if raw := meta.Get(modelPriceSettingsKey); len(raw) > 0 {
			var stored PriceSyncSettings
			if err := json.Unmarshal(raw, &stored); err != nil {
				return fmt.Errorf("decode model price sync settings: %w", err)
			}
			normalized, err := normalizePriceSyncSettings(stored)
			if err != nil {
				return fmt.Errorf("validate model price sync settings: %w", err)
			}
			a.priceSyncSettings = normalized
		}
		if raw := meta.Get(modelPriceLastSyncKey); len(raw) > 0 {
			var stored PriceSyncMetadata
			if err := json.Unmarshal(raw, &stored); err != nil {
				return fmt.Errorf("decode model price last sync: %w", err)
			}
			a.lastPriceSync = &stored
		}
		if raw := meta.Get(lastUsedKey); len(raw) > 0 {
			a.lastUsed = time.Unix(0, decodeInt64(raw)).UTC()
		}
		return hours.ForEach(func(hourKey, value []byte) error {
			if value != nil {
				return nil
			}
			hourBucket := hours.Bucket(hourKey)
			if hourBucket == nil {
				return nil
			}
			hour := decodeInt64(hourKey)
			return hourBucket.ForEach(func(dimensionKey, counterValue []byte) error {
				var dimensions Dimensions
				if err := json.Unmarshal(dimensionKey, &dimensions); err != nil {
					return fmt.Errorf("decode dimensions: %w", err)
				}
				var counters Counters
				if err := json.Unmarshal(counterValue, &counters); err != nil {
					return fmt.Errorf("decode counters: %w", err)
				}
				a.data[aggregateKey{Hour: hour, Dimensions: dimensions}] = counters
				return nil
			})
		})
	})
}

func migratePriceMetadata(meta *bolt.Bucket, version uint64) error {
	prices := make(map[string]ModelPrice)
	if raw := meta.Get(modelPricesKey); len(raw) > 0 {
		var stored map[string]ModelPrice
		if err := json.Unmarshal(raw, &stored); err != nil {
			return fmt.Errorf("decode model prices: %w", err)
		}
		normalized, err := normalizeModelPrices(stored)
		if err != nil {
			return fmt.Errorf("validate model prices: %w", err)
		}
		prices = normalized
	}
	settings := defaultPriceSyncSettings()
	if raw := meta.Get(modelPriceSettingsKey); len(raw) > 0 {
		var stored PriceSyncSettings
		if err := json.Unmarshal(raw, &stored); err != nil {
			return fmt.Errorf("decode model price sync settings: %w", err)
		}
		normalized, err := normalizePriceSyncSettings(stored)
		if err != nil {
			return fmt.Errorf("validate model price sync settings: %w", err)
		}
		settings = normalized
	}
	if raw := meta.Get(modelPriceLastSyncKey); len(raw) > 0 {
		var stored PriceSyncMetadata
		if err := json.Unmarshal(raw, &stored); err != nil {
			return fmt.Errorf("decode model price last sync: %w", err)
		}
	}
	if version >= persistenceSchemaVersion {
		return nil
	}
	encodedPrices, err := json.Marshal(prices)
	if err != nil {
		return fmt.Errorf("encode migrated model prices: %w", err)
	}
	encodedSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode migrated model price sync settings: %w", err)
	}
	if len(prices) == 0 {
		if err := meta.Delete(modelPricesKey); err != nil {
			return err
		}
	} else if err := meta.Put(modelPricesKey, encodedPrices); err != nil {
		return err
	}
	if err := meta.Put(modelPriceSettingsKey, encodedSettings); err != nil {
		return err
	}
	revision := decodeUint64(meta.Get(modelPriceRevisionKey))
	if revision == 0 && len(prices) > 0 {
		revision = 1
	}
	if err := meta.Put(modelPriceRevisionKey, encodeUint64(revision)); err != nil {
		return err
	}
	return nil
}

func (a *storeActor) record(usage normalizedUsage) error {
	key := aggregateKey{
		Hour:       usage.RequestedAt.UTC().Truncate(time.Minute).Unix(),
		Dimensions: usage.Dimensions,
	}
	counters := a.data[key]
	counters.add(countersForUsage(usage))
	a.data[key] = counters
	a.dirty[key] = struct{}{}
	a.nextRequestSeq++
	if a.nextRequestSeq == 0 {
		a.nextRequestSeq = 1
	}
	a.pendingRequests = append(a.pendingRequests, requestDetailForUsage(usage, a.nextRequestSeq))
	a.pending++
	if a.lastUsed.IsZero() || usage.RequestedAt.After(a.lastUsed) {
		a.lastUsed = usage.RequestedAt
	}

	if a.lastFlushErr != nil || a.config.SyncOnRecord || a.pending >= a.config.FlushMaxRecords {
		a.lastFlushErr = a.flush(time.Now().UTC(), false)
		return a.lastFlushErr
	}
	return nil
}

func (a *storeActor) retryFailedFlush(now time.Time) error {
	if a.lastFlushErr == nil {
		return nil
	}
	a.lastFlushErr = a.flush(now, true)
	return a.lastFlushErr
}

func (a *storeActor) flush(now time.Time, force bool) error {
	shouldPrune := a.lastPruneAt.IsZero() || now.Sub(a.lastPruneAt) >= time.Hour
	if len(a.dirty) == 0 && len(a.pendingRequests) == 0 && !shouldPrune && !force {
		return nil
	}
	cutoff := retentionCutoff(a.config, now)
	nextSince := a.since
	if shouldPrune {
		cutoffTime := time.Unix(cutoff, 0).UTC()
		if cutoffTime.After(nextSince) {
			nextSince = cutoffTime
		}
	}
	err := a.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		hours := tx.Bucket(hoursBucket)
		requests := tx.Bucket(requestsBucket)
		if meta == nil || hours == nil || requests == nil {
			return errors.New("database buckets are missing")
		}
		for key := range a.dirty {
			hourBucket, err := hours.CreateBucketIfNotExists(encodeInt64(key.Hour))
			if err != nil {
				return err
			}
			dimensions, err := json.Marshal(key.Dimensions)
			if err != nil {
				return err
			}
			counters, err := json.Marshal(a.data[key])
			if err != nil {
				return err
			}
			if err := hourBucket.Put(dimensions, counters); err != nil {
				return err
			}
		}
		for _, request := range a.pendingRequests {
			encoded, err := json.Marshal(request)
			if err != nil {
				return err
			}
			if err := requests.Put(encodeRequestKey(request.Time.UnixNano(), request.Sequence), encoded); err != nil {
				return err
			}
		}
		if err := meta.Put(sinceKey, encodeInt64(nextSince.UnixNano())); err != nil {
			return err
		}
		if !a.lastUsed.IsZero() {
			if err := meta.Put(lastUsedKey, encodeInt64(a.lastUsed.UnixNano())); err != nil {
				return err
			}
		}
		if err := meta.Put(requestSequenceKey, encodeUint64(a.nextRequestSeq)); err != nil {
			return err
		}
		if shouldPrune {
			if err := pruneHoursBucket(hours, cutoff); err != nil {
				return err
			}
			return pruneRequestsBucket(requests, time.Unix(cutoff, 0).UTC().UnixNano())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("flush database: %w", err)
	}

	clear(a.dirty)
	a.pendingRequests = a.pendingRequests[:0]
	a.pending = 0
	a.lastFlushErr = nil
	if shouldPrune {
		a.since = nextSince
		for key := range a.data {
			if key.Hour < cutoff {
				delete(a.data, key)
			}
		}
		a.lastPruneAt = now
	}
	return nil
}

func (a *storeActor) reconfigure(config Config) error {
	if config.DataPath != a.config.DataPath {
		return errors.New("data_path changes require opening a new store")
	}
	if err := a.flush(time.Now().UTC(), true); err != nil {
		a.lastFlushErr = err
		return err
	}
	previous := a.config
	previousPrune := a.lastPruneAt
	a.config = config
	a.lastPruneAt = time.Time{}
	if err := a.flush(time.Now().UTC(), true); err != nil {
		a.config = previous
		a.lastPruneAt = previousPrune
		a.lastFlushErr = err
		return err
	}
	a.lastFlushErr = nil
	a.costGeneration++
	return nil
}

func (a *storeActor) priceBookResponse() ModelPricesResponse {
	return ModelPricesResponse{
		SchemaVersion: 2,
		Revision:      a.priceRevision,
		Prices:        cloneModelPrices(a.modelPrices),
		SyncSettings:  clonePriceSyncSettings(a.priceSyncSettings),
		LastSync:      clonePriceSyncMetadata(a.lastPriceSync),
	}
}

func (a *storeActor) saveModelPrices(prices map[string]ModelPrice, settings *PriceSyncSettings) (ModelPricesResponse, error) {
	next := make(map[string]ModelPrice, len(prices))
	now := time.Now().UTC()
	for model, price := range prices {
		current, exists := a.modelPrices[model]
		if exists && current.Source == priceSourceModelsDev && sameEditableModelPrice(current, price) {
			next[model] = current
			continue
		}
		price.Source = priceSourceManual
		price.CatalogProvider = ""
		price.CatalogModel = ""
		price.UpdatedAt = now
		next[model] = price
	}
	nextSettings := a.priceSyncSettings
	if settings != nil {
		nextSettings = clonePriceSyncSettings(*settings)
	}
	nextRevision := a.priceRevision + 1
	if nextRevision == 0 {
		nextRevision = 1
	}
	if err := a.persistPriceBook(next, nextSettings, a.lastPriceSync, nextRevision); err != nil {
		return ModelPricesResponse{}, err
	}
	a.modelPrices = cloneModelPrices(next)
	a.priceSyncSettings = clonePriceSyncSettings(nextSettings)
	a.priceRevision = nextRevision
	return a.priceBookResponse(), nil
}

func (a *storeActor) applyModelPriceSync(prices map[string]ModelPrice, settings PriceSyncSettings, metadata PriceSyncMetadata, expectedRevision uint64) (ModelPricesResponse, error) {
	if a.priceRevision != expectedRevision {
		return ModelPricesResponse{}, withStatus(409, "model prices changed while synchronization was running")
	}
	next := cloneModelPrices(a.modelPrices)
	metadata.Created = 0
	metadata.Updated = 0
	metadata.SkippedManual = 0
	for model, price := range prices {
		current, exists := next[model]
		if exists && current.Source != priceSourceModelsDev {
			metadata.SkippedManual++
			continue
		}
		if exists {
			metadata.Updated++
		} else {
			metadata.Created++
		}
		next[model] = price
	}
	metadata.Source = priceSourceModelsDev
	metadata.CompletedAt = metadata.CompletedAt.UTC()
	if metadata.CompletedAt.IsZero() {
		metadata.CompletedAt = time.Now().UTC()
	}
	nextRevision := a.priceRevision + 1
	if nextRevision == 0 {
		nextRevision = 1
	}
	if err := a.persistPriceBook(next, settings, &metadata, nextRevision); err != nil {
		return ModelPricesResponse{}, err
	}
	a.modelPrices = cloneModelPrices(next)
	a.priceSyncSettings = clonePriceSyncSettings(settings)
	a.lastPriceSync = clonePriceSyncMetadata(&metadata)
	a.priceRevision = nextRevision
	return a.priceBookResponse(), nil
}

func (a *storeActor) persistPriceBook(prices map[string]ModelPrice, settings PriceSyncSettings, lastSync *PriceSyncMetadata, revision uint64) error {
	encodedPrices, err := json.Marshal(prices)
	if err != nil {
		return fmt.Errorf("encode model prices: %w", err)
	}
	encodedSettings, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode model price sync settings: %w", err)
	}
	var encodedLastSync []byte
	if lastSync != nil {
		encodedLastSync, err = json.Marshal(lastSync)
		if err != nil {
			return fmt.Errorf("encode model price last sync: %w", err)
		}
	}
	if err := a.db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return errors.New("metadata bucket is missing")
		}
		if len(prices) == 0 {
			if err := meta.Delete(modelPricesKey); err != nil {
				return err
			}
		} else if err := meta.Put(modelPricesKey, encodedPrices); err != nil {
			return err
		}
		if err := meta.Put(modelPriceSettingsKey, encodedSettings); err != nil {
			return err
		}
		if lastSync == nil {
			if err := meta.Delete(modelPriceLastSyncKey); err != nil {
				return err
			}
		} else if err := meta.Put(modelPriceLastSyncKey, encodedLastSync); err != nil {
			return err
		}
		return meta.Put(modelPriceRevisionKey, encodeUint64(revision))
	}); err != nil {
		return fmt.Errorf("save model prices: %w", err)
	}
	return nil
}

func (a *storeActor) observedModels(now time.Time) []string {
	seen := make(map[string]struct{})
	cutoff := retentionCutoff(a.config, now)
	for key := range a.data {
		if key.Hour < cutoff {
			continue
		}
		model := strings.TrimSpace(key.Dimensions.Model)
		if model != "" {
			seen[model] = struct{}{}
		}
	}
	models := make([]string, 0, len(seen))
	for model := range seen {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func (a *storeActor) reset() error {
	now := time.Now().UTC()
	if err := a.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(hoursBucket); err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
			return err
		}
		if _, err := tx.CreateBucket(hoursBucket); err != nil {
			return err
		}
		if err := tx.DeleteBucket(requestsBucket); err != nil && !errors.Is(err, bolt.ErrBucketNotFound) {
			return err
		}
		if _, err := tx.CreateBucket(requestsBucket); err != nil {
			return err
		}
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return errors.New("metadata bucket is missing")
		}
		if err := meta.Put(sinceKey, encodeInt64(now.UnixNano())); err != nil {
			return err
		}
		if err := meta.Put(requestSequenceKey, encodeUint64(0)); err != nil {
			return err
		}
		return meta.Delete(lastUsedKey)
	}); err != nil {
		return fmt.Errorf("reset database: %w", err)
	}
	a.data = make(map[aggregateKey]Counters)
	a.dirty = make(map[aggregateKey]struct{})
	a.pending = 0
	a.pendingRequests = nil
	a.nextRequestSeq = 0
	a.lastFlushErr = nil
	a.since = now
	a.lastUsed = time.Time{}
	a.costGeneration++
	return nil
}

func (a *storeActor) queryRequests(requestedRange string, offset, limit int, model string, now time.Time) (RequestPage, error) {
	rangeName, cutoff, err := queryCutoff(requestedRange, now)
	if err != nil {
		return RequestPage{}, err
	}
	if offset < 0 {
		return RequestPage{}, withStatus(400, "offset must not be negative")
	}
	if limit == 0 {
		limit = defaultRequestPageSize
	}
	if limit < 1 || limit > maxRequestPageSize {
		return RequestPage{}, withStatus(400, "limit must be between 1 and %d", maxRequestPageSize)
	}

	page := RequestPage{
		GeneratedAt:       now.UTC(),
		Range:             rangeName,
		PriceBookRevision: a.priceRevision,
		Offset:            offset,
		Limit:             limit,
		Items:             make([]RequestDetail, 0, limit),
	}
	err = a.db.View(func(tx *bolt.Tx) error {
		requests := tx.Bucket(requestsBucket)
		if requests == nil {
			return errors.New("requests bucket is missing")
		}
		cursor := requests.Cursor()
		for key, value := cursor.Last(); key != nil; key, value = cursor.Prev() {
			if len(key) != 16 || value == nil {
				continue
			}
			requestedAt := time.Unix(0, decodeInt64(key[:8])).UTC()
			if !cutoff.IsZero() && requestedAt.Before(cutoff) {
				break
			}
			var item RequestDetail
			if err := json.Unmarshal(value, &item); err != nil {
				return fmt.Errorf("decode request detail: %w", err)
			}
			itemModel := item.Model
			if itemModel == "" {
				itemModel = "未标记模型"
			}
			if model != "" && itemModel != model {
				continue
			}
			page.Total++
			if page.Total <= offset || len(page.Items) >= limit {
				continue
			}
			cost := estimateRequestCost(item, a.modelPrices)
			item.EstimatedCost = &cost
			page.Items = append(page.Items, item)
		}
		return nil
	})
	if err != nil {
		return RequestPage{}, fmt.Errorf("query request details: %w", err)
	}
	return page, nil
}

func retentionCutoff(config Config, now time.Time) int64 {
	return now.UTC().Add(-time.Duration(config.RetentionDays) * 24 * time.Hour).Truncate(time.Minute).Unix()
}

func pruneHoursBucket(hours *bolt.Bucket, cutoff int64) error {
	var expired [][]byte
	if err := hours.ForEach(func(key, value []byte) error {
		if value == nil && decodeInt64(key) < cutoff {
			expired = append(expired, append([]byte(nil), key...))
		}
		return nil
	}); err != nil {
		return err
	}
	for _, key := range expired {
		if err := hours.DeleteBucket(key); err != nil {
			return err
		}
	}
	return nil
}

func encodeRequestKey(unixNano int64, sequence uint64) []byte {
	result := make([]byte, 16)
	copy(result[:8], encodeInt64(unixNano))
	binary.BigEndian.PutUint64(result[8:], sequence)
	return result
}

func pruneRequestsBucket(requests *bolt.Bucket, cutoffUnixNano int64) error {
	cursor := requests.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		if len(key) != 16 {
			continue
		}
		if decodeInt64(key[:8]) >= cutoffUnixNano {
			break
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func encodeUint64(value uint64) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint64(result, value)
	return result
}

func decodeUint64(value []byte) uint64 {
	if len(value) != 8 {
		return 0
	}
	return binary.BigEndian.Uint64(value)
}

func encodeInt64(value int64) []byte {
	return encodeUint64(uint64(value) ^ (uint64(1) << 63))
}

func decodeInt64(value []byte) int64 {
	return int64(decodeUint64(value) ^ (uint64(1) << 63))
}
