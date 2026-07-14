package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	metaBucket  = []byte("meta")
	hoursBucket = []byte("hours")
	schemaKey   = []byte("schema_version")
	sinceKey    = []byte("since_unix_nano")
	lastUsedKey = []byte("last_used_unix_nano")
)

const persistenceSchemaVersion uint64 = 1

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

type resetCommand struct{ resp chan error }
type configCommand struct {
	config Config
	resp   chan error
}
type closeCommand struct{ resp chan error }

type Store struct {
	commands  chan any
	done      chan struct{}
	closeOnce sync.Once
	stateMu   sync.RWMutex
	closed    bool
	closeErr  error
}

type storeActor struct {
	db           *bolt.DB
	config       Config
	data         map[aggregateKey]Counters
	dirty        map[aggregateKey]struct{}
	since        time.Time
	lastUsed     time.Time
	pending      int
	lastPruneAt  time.Time
	lastFlushErr error
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
		commands: make(chan any, 256),
		done:     make(chan struct{}),
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
				if err := actor.retryFailedFlush(time.Now().UTC()); err != nil {
					item.resp <- err
					continue
				}
				item.resp <- actor.record(item.usage)
			case queryCommand:
				if err := actor.retryFailedFlush(time.Now().UTC()); err != nil {
					item.resp <- queryResult{err: err}
					continue
				}
				stats, err := buildStats(actor.data, actor.since, actor.lastUsed, item.rangeName, time.Now().UTC())
				item.resp <- queryResult{stats: stats, err: err}
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
		version := decodeUint64(meta.Get(schemaKey))
		if version != 0 && version != persistenceSchemaVersion {
			return fmt.Errorf("unsupported database schema version %d", version)
		}
		if err := meta.Put(schemaKey, encodeUint64(persistenceSchemaVersion)); err != nil {
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
		return pruneHoursBucket(hours, cutoff)
	}); err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}

	return a.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket(metaBucket)
		hours := tx.Bucket(hoursBucket)
		if meta == nil || hours == nil {
			return errors.New("database buckets are missing")
		}
		a.since = time.Unix(0, decodeInt64(meta.Get(sinceKey))).UTC()
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

func (a *storeActor) record(usage normalizedUsage) error {
	key := aggregateKey{
		Hour:       usage.RequestedAt.UTC().Truncate(time.Hour).Unix(),
		Dimensions: usage.Dimensions,
	}
	counters := a.data[key]
	counters.add(countersForUsage(usage))
	a.data[key] = counters
	a.dirty[key] = struct{}{}
	a.pending++
	if a.lastUsed.IsZero() || usage.RequestedAt.After(a.lastUsed) {
		a.lastUsed = usage.RequestedAt
	}

	if a.config.SyncOnRecord || a.pending >= a.config.FlushMaxRecords {
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
	if len(a.dirty) == 0 && !shouldPrune && !force {
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
		if meta == nil || hours == nil {
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
		if err := meta.Put(sinceKey, encodeInt64(nextSince.UnixNano())); err != nil {
			return err
		}
		if !a.lastUsed.IsZero() {
			if err := meta.Put(lastUsedKey, encodeInt64(a.lastUsed.UnixNano())); err != nil {
				return err
			}
		}
		if shouldPrune {
			return pruneHoursBucket(hours, cutoff)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("flush database: %w", err)
	}

	clear(a.dirty)
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
	return nil
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
		meta := tx.Bucket(metaBucket)
		if meta == nil {
			return errors.New("metadata bucket is missing")
		}
		if err := meta.Put(sinceKey, encodeInt64(now.UnixNano())); err != nil {
			return err
		}
		return meta.Delete(lastUsedKey)
	}); err != nil {
		return fmt.Errorf("reset database: %w", err)
	}
	a.data = make(map[aggregateKey]Counters)
	a.dirty = make(map[aggregateKey]struct{})
	a.pending = 0
	a.lastFlushErr = nil
	a.since = now
	a.lastUsed = time.Time{}
	return nil
}

func retentionCutoff(config Config, now time.Time) int64 {
	return now.UTC().Add(-time.Duration(config.RetentionDays) * 24 * time.Hour).Truncate(time.Hour).Unix()
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
