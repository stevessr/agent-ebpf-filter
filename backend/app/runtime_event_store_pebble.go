package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/recording"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/pebble/v2"
)

var (
	eventStoreRecordPrefix     = []byte{'e'}
	eventStoreRecordUpper      = []byte{'f'}
	eventStoreIDPrefix         = []byte{'i'}
	eventStoreIDUpper          = []byte{'j'}
	errInvalidEventStoreCursor = errors.New("invalid event database cursor")
)

type runtimeEventStoreItem struct {
	record CapturedEventRecord
	flush  chan error
}

type runtimeEventStore struct {
	mu                    sync.Mutex
	pruneMu               sync.Mutex
	db                    *pebble.DB
	path                  string
	queue                 chan runtimeEventStoreItem
	stopCh                chan struct{}
	done                  chan struct{}
	accepting             bool
	stopping              bool
	queuedRecords         int
	flushWaiters          int
	enqueuedTotal         uint64
	persistedTotal        uint64
	failedTotal           uint64
	droppedTotal          uint64
	lastFlushedAt         time.Time
	lastError             string
	terminalErr           error
	stopRequested         bool
	retentionMaxRecords   int
	retentionMaxAge       time.Duration
	nextRetentionSweep    uint64
	retentionSweepRunning bool
	auditChain            *recording.AuditChain
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
		db:                 db,
		path:               resolved,
		queue:              make(chan runtimeEventStoreItem, runtimeEventLogQueueSize+1),
		stopCh:             make(chan struct{}),
		done:               make(chan struct{}),
		accepting:          true,
		nextRetentionSweep: 1024,
		auditChain:         auditChain,
	}
	go store.run()
	return store, resolved, nil
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
