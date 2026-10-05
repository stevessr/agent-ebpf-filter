package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/recording"

	"github.com/cockroachdb/pebble/v2"
)

var (
	eventStoreRecordPrefix      = []byte{'e'}
	eventStoreRecordUpper       = []byte{'f'}
	eventStoreIDPrefix          = []byte{'i'}
	eventStoreIDUpper           = []byte{'j'}
	errInvalidEventStoreCursor  = errors.New("invalid event database cursor")
)

type runtimeEventStoreItem struct {
	record CapturedEventRecord
	flush  chan error
}

type runtimeEventStore struct {
	mu             sync.Mutex
	pruneMu        sync.Mutex
	db             *pebble.DB
	path           string
	queue          chan runtimeEventStoreItem
	stopCh         chan struct{}
	done           chan struct{}
	accepting      bool
	stopping       bool
	queuedRecords  int
	flushWaiters   int
	enqueuedTotal  uint64
	persistedTotal uint64
	failedTotal    uint64
	droppedTotal   uint64
	lastFlushedAt  time.Time
	lastError      string
	terminalErr    error
	stopRequested  bool
	retentionMaxRecords   int
	retentionMaxAge       time.Duration
	nextRetentionSweep    uint64
	retentionSweepRunning bool
	auditChain     *recording.AuditChain
}

func isPebbleEventStorePath(path string) bool {
	value := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(value, ".pebble")
}

func openRuntimeEventStoreWithin(rootPath, rawPath string) (*runtimeEventStore, string, error) {
	resolved, err := resolveRuntimeEventLogPathWithin(rootPath, rawPath)
	if err != nil {
		return nil, "", err
	}
	dir, err := platform.SecureOpenOrCreateDir(resolved)
	if err != nil {
		return nil, "", fmt.Errorf("open event database directory: %w", err)
	}
	if err := dir.Chmod(0o700); err != nil {
		_ = dir.Close()
		return nil, "", fmt.Errorf("set event database permissions: %w", err)
	}
	if err := platform.ChownArtifactFile(dir); err != nil {
		_ = dir.Close()
		return nil, "", fmt.Errorf("set event database ownership: %w", err)
	}
	if err := dir.Close(); err != nil {
		return nil, "", err
	}

	db, err := pebble.Open(resolved, &pebble.Options{})
	if err != nil {
		return nil, "", fmt.Errorf("open Pebble event database: %w", err)
	}
	auditChain, err := recording.NewAuditChain()
	if err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("initialize event database audit chain: %w", err)
	}
	store := &runtimeEventStore{
		db:         db,
		path:       resolved,
		queue:      make(chan runtimeEventStoreItem, runtimeEventLogQueueSize+1),
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
		accepting:           true,
		nextRetentionSweep: 1024,
		auditChain:          auditChain,
	}
	go store.run()
	return store, resolved, nil
}

func eventStoreRecordKey(record CapturedEventRecord) ([]byte, string, error) {
	record = normalizeCapturedEventRecord(record)
	if record.Event == nil || record.Envelope == nil {
		return nil, "", errors.New("event database record has no normalized envelope")
	}
	eventID := strings.TrimSpace(record.Envelope.GetEventId())
	if eventID == "" {
		return nil, "", errors.New("event database record has no event id")
	}
	key := make([]byte, 1+8+len(eventID))
	key[0] = eventStoreRecordPrefix[0]
	binary.BigEndian.PutUint64(key[1:9], uint64(record.ReceivedAt.UTC().UnixNano()))
	copy(key[9:], eventID)
	return key, eventID, nil
}

func eventStoreIDKey(eventID string) []byte {
	key := make([]byte, 1+len(eventID))
	key[0] = eventStoreIDPrefix[0]
	copy(key[1:], eventID)
	return key
}

func decodeEventStoreRecord(payload []byte) (CapturedEventRecord, error) {
	var record CapturedEventRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return CapturedEventRecord{}, err
	}
	if record.Event == nil {
		return CapturedEventRecord{}, errors.New("event database record has no event")
	}
	return normalizeCapturedEventRecord(record), nil
}

func (s *runtimeEventStore) Enqueue(record CapturedEventRecord) (bool, error) {
	if s == nil {
		return false, errRuntimeEventLogStopped
	}
	if record.Event == nil {
		return false, errors.New("event database record has no event")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accepting {
		err := s.terminalErr
		if err == nil {
			err = errRuntimeEventLogStopped
		}
		s.droppedTotal++
		s.lastError = err.Error()
		return false, err
	}
	queueLimit := cap(s.queue)
	if s.flushWaiters > 0 && queueLimit > 0 {
		queueLimit--
	}
	if s.queuedRecords >= runtimeEventLogQueueSize || len(s.queue) >= queueLimit {
		s.droppedTotal++
		s.lastError = errRuntimeEventLogQueueFull.Error()
		return false, errRuntimeEventLogQueueFull
	}
	select {
	case s.queue <- runtimeEventStoreItem{record: record}:
		s.enqueuedTotal++
		s.queuedRecords++
		return true, nil
	default:
		s.droppedTotal++
		s.lastError = errRuntimeEventLogQueueFull.Error()
		return false, errRuntimeEventLogQueueFull
	}
}

func (s *runtimeEventStore) FlushContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ack := make(chan error, 1)
	retry := time.NewTicker(runtimeEventLogFlushRetry)
	defer retry.Stop()
	registered := false
	defer func() {
		if !registered {
			return
		}
		s.mu.Lock()
		s.flushWaiters--
		s.mu.Unlock()
	}()
	for {
		s.mu.Lock()
		if !s.accepting {
			if registered {
				s.flushWaiters--
				registered = false
			}
			done := s.done
			s.mu.Unlock()
			select {
			case <-done:
				return s.TerminalError()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if !registered {
			s.flushWaiters++
			registered = true
		}
		select {
		case s.queue <- runtimeEventStoreItem{flush: ack}:
			s.flushWaiters--
			registered = false
			s.mu.Unlock()
			select {
			case err := <-ack:
				return err
			case <-s.done:
				return s.TerminalError()
			case <-ctx.Done():
				return ctx.Err()
			}
		default:
			s.mu.Unlock()
		}
		select {
		case <-s.done:
			return s.TerminalError()
		case <-ctx.Done():
			return ctx.Err()
		case <-retry.C:
		}
	}
}

func (s *runtimeEventStore) StopContext(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if !s.stopRequested {
		s.accepting = false
		s.stopping = true
		s.stopRequested = true
		close(s.stopCh)
	}
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
		return s.TerminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *runtimeEventStore) TerminalError() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminalErr
}

func (s *runtimeEventStore) Status() runtimeEventLogStatus {
	if s == nil {
		return runtimeEventLogStatus{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	completed := s.persistedTotal + s.failedTotal
	pending := uint64(0)
	if s.enqueuedTotal > completed {
		pending = s.enqueuedTotal - completed
	}
	chain := recording.AuditChainStatus{}
	if s.auditChain != nil {
		chain = s.auditChain.Status()
	}
	return runtimeEventLogStatus{
		Active:            s.accepting,
		Stopping:          s.stopping,
		QueueLen:          s.queuedRecords,
		QueueCap:          runtimeEventLogQueueSize,
		Pending:           pending,
		EnqueuedTotal:     s.enqueuedTotal,
		PersistedTotal:    s.persistedTotal,
		FailedTotal:       s.failedTotal,
		DroppedTotal:      s.droppedTotal,
		LastFlushedAt:     s.lastFlushedAt,
		LastError:         s.lastError,
		AuditChainVersion: chain.Version,
		AuditChainID:      chain.ChainID,
		AuditSequence:     chain.Sequence,
		AuditLastHash:     chain.LastHash,
	}
}

func (s *runtimeEventStore) noteDequeued(item runtimeEventStoreItem) {
	if item.flush != nil {
		return
	}
	s.mu.Lock()
	if s.queuedRecords > 0 {
		s.queuedRecords--
	}
	s.mu.Unlock()
}

func (s *runtimeEventStore) notePersisted(count uint64, duration time.Duration) {
	if count == 0 {
		return
	}
	scheduleRetention := false
	s.mu.Lock()
	s.persistedTotal += count
	s.lastFlushedAt = time.Now().UTC()
	if !s.retentionSweepRunning &&
		(s.retentionMaxRecords > 0 || s.retentionMaxAge > 0) &&
		s.persistedTotal >= s.nextRetentionSweep {
		s.retentionSweepRunning = true
		s.nextRetentionSweep = s.persistedTotal + 1024
		scheduleRetention = true
	}
	s.mu.Unlock()
	collectorMetricsStore.RecordCapturedPersistBatch(count, 0, duration)
	if scheduleRetention {
		go s.pruneConfiguredRetention()
	}
}

func (s *runtimeEventStore) noteFailed(count uint64, err error, duration time.Duration) {
	s.mu.Lock()
	s.failedTotal += count
	if err != nil {
		s.lastError = err.Error()
	}
	s.mu.Unlock()
	collectorMetricsStore.RecordCapturedPersistBatch(0, count, duration)
}

func (s *runtimeEventStore) stopAccepting(err error) {
	s.mu.Lock()
	s.accepting = false
	s.stopping = true
	if err != nil {
		s.lastError = err.Error()
	}
	s.mu.Unlock()
}

func (s *runtimeEventStore) finish(err error) {
	s.mu.Lock()
	s.accepting = false
	s.stopping = false
	s.terminalErr = err
	if err != nil {
		s.lastError = err.Error()
	}
	done := s.done
	s.mu.Unlock()
	close(done)
}

func (s *runtimeEventStore) run() {
	ticker := time.NewTicker(runtimeEventLogFlushInterval)
	defer ticker.Stop()

	batch := s.db.NewBatch()
	pending := 0
	var terminalErr error

	flush := func() error {
		if pending == 0 {
			return nil
		}
		count := pending
		pending = 0
		started := time.Now()
		err := batch.Commit(pebble.Sync)
		_ = batch.Close()
		batch = s.db.NewBatch()
		if err != nil {
			s.noteFailed(uint64(count), err, time.Since(started))
			return err
		}
		s.notePersisted(uint64(count), time.Since(started))
		return nil
	}

	process := func(record CapturedEventRecord) error {
		started := time.Now()
		recordKey, eventID, err := eventStoreRecordKey(record)
		if err != nil {
			s.noteFailed(1, err, time.Since(started))
			return nil
		}
		payload, err := s.auditChain.MarshalRecord(record)
		if err != nil {
			s.noteFailed(1, err, time.Since(started))
			return nil
		}
		if err := batch.Set(recordKey, payload, pebble.NoSync); err != nil {
			s.noteFailed(1, err, time.Since(started))
			return err
		}
		if err := batch.Set(eventStoreIDKey(eventID), recordKey, pebble.NoSync); err != nil {
			s.noteFailed(1, err, time.Since(started))
			return err
		}
		pending++
		if pending >= runtimeEventLogFlushBatch {
			return flush()
		}
		return nil
	}

	processItem := func(item runtimeEventStoreItem) error {
		if item.flush != nil {
			err := flush()
			item.flush <- err
			return err
		}
		return process(item.record)
	}

	drain := func(reason error) {
		for {
			select {
			case item := <-s.queue:
				s.noteDequeued(item)
				if reason != nil {
					if item.flush != nil {
						item.flush <- reason
					} else {
						s.noteFailed(1, reason, 0)
					}
					continue
				}
				if err := processItem(item); err != nil {
					terminalErr = errors.Join(terminalErr, err)
					reason = err
					s.stopAccepting(err)
				}
			default:
				return
			}
		}
	}

	defer func() {
		if err := flush(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
		}
		_ = batch.Close()
		s.pruneMu.Lock()
		if err := s.db.Close(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
		}
		s.pruneMu.Unlock()
		s.finish(terminalErr)
	}()

	for {
		select {
		case <-s.stopCh:
			drain(nil)
			return
		default:
		}
		select {
		case <-s.stopCh:
			drain(nil)
			return
		case <-ticker.C:
			if err := flush(); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				s.stopAccepting(err)
				drain(err)
				return
			}
		case item := <-s.queue:
			s.noteDequeued(item)
			if err := processItem(item); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				s.stopAccepting(err)
				drain(err)
				return
			}
		}
	}
}

func (s *runtimeEventStore) Recent(ctx context.Context, limit int) ([]CapturedEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("event database is not open")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 50
	} else if limit > runtimeEventLogMaxRecords {
		limit = runtimeEventLogMaxRecords
	}
	if err := s.FlushContext(ctx); err != nil {
		return nil, err
	}
	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: eventStoreRecordPrefix,
		UpperBound: eventStoreRecordUpper,
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	records := make([]CapturedEventRecord, 0, limit)
	for valid := iter.Last(); valid && len(records) < limit; valid = iter.Prev() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		payload := append([]byte(nil), iter.Value()...)
		record, err := decodeEventStoreRecord(payload)
		if err != nil {
			return nil, fmt.Errorf("decode event database record: %w", err)
		}
		records = append(records, record)
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	// Existing callers expect oldest -> newest from the persistence layer.
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	return records, nil
}

func decodeEventStoreCursor(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	key, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, errInvalidEventStoreCursor
	}
	if len(key) < 9 || key[0] != eventStoreRecordPrefix[0] {
		return nil, errors.New("invalid event database cursor")
	}
	return key, nil
}

func encodeEventStoreCursor(key []byte) string {
	if len(key) == 0 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(key)
}

// Page returns one reverse-chronological window from the persistent event
// database. Cursor paging keeps disk retention independent from Renew's bounded
// in-memory summary window. Records inside a page are returned oldest -> newest to preserve
// the ordering contract used by existing history consumers. nextCursor is an
// opaque key for the next older page and must not be interpreted by clients.
func (s *runtimeEventStore) Page(ctx context.Context, limit int, cursor string) ([]CapturedEventRecord, string, error) {
	if s == nil || s.db == nil {
		return nil, "", errors.New("event database is not open")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 {
		limit = 50
	} else if limit > runtimeEventLogMaxRecords {
		limit = runtimeEventLogMaxRecords
	}
	if err := s.FlushContext(ctx); err != nil {
		return nil, "", err
	}
	cursorKey, err := decodeEventStoreCursor(cursor)
	if err != nil {
		return nil, "", err
	}

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: eventStoreRecordPrefix,
		UpperBound: eventStoreRecordUpper,
	})
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()

	valid := false
	if len(cursorKey) == 0 {
		valid = iter.Last()
	} else {
		valid = iter.SeekLT(cursorKey)
	}

	records := make([]CapturedEventRecord, 0, limit)
	var lastKey []byte
	for valid && len(records) < limit {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		payload := append([]byte(nil), iter.Value()...)
		record, err := decodeEventStoreRecord(payload)
		if err != nil {
			return nil, "", fmt.Errorf("decode event database record: %w", err)
		}
		records = append(records, record)
		lastKey = append(lastKey[:0], iter.Key()...)
		valid = iter.Prev()
	}
	if err := iter.Error(); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if valid && len(lastKey) > 0 {
		nextCursor = encodeEventStoreCursor(lastKey)
	}

	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
	return records, nextCursor, nil
}

func (s *runtimeEventStore) GetByID(ctx context.Context, eventID string) (CapturedEventRecord, error) {
	if s == nil || s.db == nil {
		return CapturedEventRecord{}, errors.New("event database is not open")
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return CapturedEventRecord{}, pebble.ErrNotFound
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.FlushContext(ctx); err != nil {
		return CapturedEventRecord{}, err
	}
	recordKey, closer, err := s.db.Get(eventStoreIDKey(eventID))
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return CapturedEventRecord{}, os.ErrNotExist
		}
		return CapturedEventRecord{}, err
	}
	keyCopy := append([]byte(nil), recordKey...)
	if err := closer.Close(); err != nil {
		return CapturedEventRecord{}, err
	}
	payload, closer, err := s.db.Get(keyCopy)
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return CapturedEventRecord{}, os.ErrNotExist
		}
		return CapturedEventRecord{}, err
	}
	payloadCopy := append([]byte(nil), payload...)
	if err := closer.Close(); err != nil {
		return CapturedEventRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return CapturedEventRecord{}, err
	}
	return decodeEventStoreRecord(payloadCopy)
}


func eventStoreRecordTimestamp(key []byte) (time.Time, bool) {
	if len(key) < 9 || key[0] != eventStoreRecordPrefix[0] {
		return time.Time{}, false
	}
	ns := int64(binary.BigEndian.Uint64(key[1:9]))
	if ns <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ns).UTC(), true
}

func (s *runtimeEventStore) SetRetention(maxRecords int, maxAge time.Duration) {
	if s == nil {
		return
	}
	if maxRecords < 0 {
		maxRecords = 0
	}
	if maxAge < 0 {
		maxAge = 0
	}
	schedule := false
	s.mu.Lock()
	s.retentionMaxRecords = maxRecords
	s.retentionMaxAge = maxAge
	s.nextRetentionSweep = s.persistedTotal
	if !s.retentionSweepRunning && (maxRecords > 0 || maxAge > 0) {
		s.retentionSweepRunning = true
		schedule = true
	}
	s.mu.Unlock()
	if schedule {
		go s.pruneConfiguredRetention()
	}
}

func (s *runtimeEventStore) configuredRetention() (int, time.Duration) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retentionMaxRecords, s.retentionMaxAge
}

func (s *runtimeEventStore) pruneConfiguredRetention() {
	defer func() {
		s.mu.Lock()
		s.retentionSweepRunning = false
		s.nextRetentionSweep = s.persistedTotal + 1024
		s.mu.Unlock()
	}()
	maxRecords, maxAge := s.configuredRetention()
	if maxRecords <= 0 && maxAge <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := s.Prune(ctx, maxRecords, maxAge); err != nil &&
		!errors.Is(err, context.Canceled) &&
		!errors.Is(err, context.DeadlineExceeded) &&
		!errors.Is(err, errRuntimeEventLogStopped) {
		s.mu.Lock()
		s.lastError = "event database retention: " + err.Error()
		s.mu.Unlock()
	}
}

func (s *runtimeEventStore) Prune(ctx context.Context, maxRecords int, maxAge time.Duration) (int, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if maxRecords <= 0 && maxAge <= 0 {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.FlushContext(ctx); err != nil {
		return 0, err
	}

	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	var oldestKept []byte
	if maxRecords > 0 {
		iter, err := s.db.NewIter(&pebble.IterOptions{
			LowerBound: eventStoreRecordPrefix,
			UpperBound: eventStoreRecordUpper,
		})
		if err != nil {
			return 0, err
		}
		count := 0
		for valid := iter.Last(); valid; valid = iter.Prev() {
			if err := ctx.Err(); err != nil {
				_ = iter.Close()
				return 0, err
			}
			count++
			if count == maxRecords {
				oldestKept = append([]byte(nil), iter.Key()...)
				break
			}
		}
		if err := iter.Error(); err != nil {
			_ = iter.Close()
			return 0, err
		}
		if err := iter.Close(); err != nil {
			return 0, err
		}
	}

	cutoff := time.Time{}
	if maxAge > 0 {
		cutoff = time.Now().UTC().Add(-maxAge)
	}

	iter, err := s.db.NewIter(&pebble.IterOptions{
		LowerBound: eventStoreRecordPrefix,
		UpperBound: eventStoreRecordUpper,
	})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	batch := s.db.NewBatch()
	defer func() { _ = batch.Close() }()
	deleted := 0
	pending := 0
	commit := func() error {
		if pending == 0 {
			return nil
		}
		if err := batch.Commit(pebble.Sync); err != nil {
			return err
		}
		_ = batch.Close()
		batch = s.db.NewBatch()
		pending = 0
		return nil
	}

	for valid := iter.First(); valid; valid = iter.Next() {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		key := append([]byte(nil), iter.Key()...)
		deleteForCount := len(oldestKept) > 0 && bytes.Compare(key, oldestKept) < 0
		deleteForAge := false
		if !cutoff.IsZero() {
			if receivedAt, ok := eventStoreRecordTimestamp(key); ok {
				deleteForAge = receivedAt.Before(cutoff)
			}
		}
		if !deleteForCount && !deleteForAge {
			continue
		}
		if len(key) <= 9 {
			continue
		}
		if err := batch.Delete(key, pebble.NoSync); err != nil {
			return deleted, err
		}
		idKey := eventStoreIDKey(string(key[9:]))
		indexedKey, closer, lookupErr := s.db.Get(idKey)
		if lookupErr == nil {
			pointsToDeletedRecord := bytes.Equal(indexedKey, key)
			if closeErr := closer.Close(); closeErr != nil {
				return deleted, closeErr
			}
			if pointsToDeletedRecord {
				if err := batch.Delete(idKey, pebble.NoSync); err != nil {
					return deleted, err
				}
			}
		} else if !errors.Is(lookupErr, pebble.ErrNotFound) {
			return deleted, lookupErr
		}
		deleted++
		pending++
		if pending >= 1024 {
			if err := commit(); err != nil {
				return deleted, err
			}
		}
	}
	if err := iter.Error(); err != nil {
		return deleted, err
	}
	if err := commit(); err != nil {
		return deleted, err
	}
	return deleted, nil
}

func (s *runtimeEventStore) Clear(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.FlushContext(ctx); err != nil {
		return err
	}
	s.pruneMu.Lock()
	defer s.pruneMu.Unlock()
	batch := s.db.NewBatch()
	defer func() { _ = batch.Close() }()
	if err := batch.DeleteRange(eventStoreRecordPrefix, eventStoreRecordUpper, pebble.NoSync); err != nil {
		return err
	}
	if err := batch.DeleteRange(eventStoreIDPrefix, eventStoreIDUpper, pebble.NoSync); err != nil {
		return err
	}
	return batch.Commit(pebble.Sync)
}

