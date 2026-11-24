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
	"time"

	"github.com/cockroachdb/pebble/v2"
)

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
