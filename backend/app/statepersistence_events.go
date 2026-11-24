package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/recording"
	"agent-ebpf-filter/app/research"
	"agent-ebpf-filter/pb"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

func (s *runtimeState) closeLogWriterLocked() {
	ctx, cancel := runtimeEventLogStopContext()
	defer cancel()
	if err := s.stopLogWriterLocked(ctx); err != nil {
		log.Printf("[WARN] failed to stop runtime event log writer cleanly: %v", err)
	}
}

func (s *runtimeState) stopLogWriterLocked(ctx context.Context) error {
	var stopErr error
	if store := s.eventStore; store != nil {
		if err := store.StopContext(ctx); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
		if s.eventStore == store {
			s.eventStore = nil
		}
	}
	if writer := s.logWriter; writer != nil {
		if err := writer.StopContext(ctx); err != nil {
			stopErr = errors.Join(stopErr, err)
		}
		if s.logWriter == writer {
			s.logWriter = nil
		}
	}
	if s.eventStore == nil && s.logWriter == nil {
		s.logPath = ""
	}
	return stopErr
}

func (s *runtimeState) applyLoggingLocked() error {
	if !s.settings.LogPersistenceEnabled {
		ctx, cancel := runtimeEventLogStopContext()
		defer cancel()
		err := s.stopLogWriterLocked(ctx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err != nil {
			log.Printf("[WARN] disabled runtime event persistence after writer failure: %v", err)
		}
		return nil
	}
	resolvedPath, err := resolveRuntimeEventLogPathWithin(s.eventLogRoot(), expandRuntimeEventLogPath(s.settings.LogFilePath))
	if err != nil {
		return err
	}
	if isPebbleEventStorePath(resolvedPath) {
		if s.eventStore != nil && s.logPath == resolvedPath && s.eventStore.Status().Active {
			s.settings.LogFilePath = resolvedPath
			maxAge, _ := time.ParseDuration(s.settings.EventStoreMaxAge)
			s.eventStore.SetRetention(s.settings.EventStoreMaxRecords, maxAge)
			return nil
		}
		store, resolvedPath, err := openRuntimeEventStoreWithin(s.eventLogRoot(), resolvedPath)
		if err != nil {
			return err
		}
		maxAge, _ := time.ParseDuration(s.settings.EventStoreMaxAge)
		store.SetRetention(s.settings.EventStoreMaxRecords, maxAge)
		ctx, cancel := runtimeEventLogStopContext()
		stopErr := s.stopLogWriterLocked(ctx)
		cancel()
		if errors.Is(stopErr, context.Canceled) || errors.Is(stopErr, context.DeadlineExceeded) {
			stopCtx, stopCancel := runtimeEventLogStopContext()
			_ = store.StopContext(stopCtx)
			stopCancel()
			return stopErr
		}
		if stopErr != nil {
			log.Printf("[WARN] previous runtime event persistence stopped after failure: %v", stopErr)
		}
		s.settings.LogFilePath = resolvedPath
		s.eventStore = store
		s.logPath = resolvedPath
		return nil
	}

	// Existing explicit .jsonl paths stay supported for compatibility and
	// migration. New installs default to the Pebble-backed event database.
	if s.logWriter != nil && s.logPath == resolvedPath && s.logWriter.Status().Active {
		s.settings.LogFilePath = resolvedPath
		return nil
	}
	file, resolvedPath, err := openRuntimeEventLogFileWithin(s.eventLogRoot(), resolvedPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		return err
	}
	writer, err := startRuntimeEventLogWriter(file)
	if err != nil {
		return err
	}
	ctx, cancel := runtimeEventLogStopContext()
	stopErr := s.stopLogWriterLocked(ctx)
	cancel()
	if errors.Is(stopErr, context.Canceled) || errors.Is(stopErr, context.DeadlineExceeded) {
		stopCtx, stopCancel := runtimeEventLogStopContext()
		_ = writer.StopContext(stopCtx)
		stopCancel()
		return stopErr
	}
	if stopErr != nil {
		log.Printf("[WARN] previous runtime event persistence stopped after failure: %v", stopErr)
	}
	s.settings.LogFilePath = resolvedPath
	s.logWriter = writer
	s.logPath = resolvedPath
	return nil
}

func (s *runtimeState) TruncateEventLog() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.eventStore != nil {
		ctx, cancel := runtimeEventLogStopContext()
		defer cancel()
		return s.eventStore.Clear(ctx)
	}

	ctx, cancel := runtimeEventLogStopContext()
	if err := s.stopLogWriterLocked(ctx); errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		cancel()
		return err
	} else if err != nil {
		log.Printf("[WARN] truncating runtime event log after writer failure: %v", err)
	}
	cancel()
	path := strings.TrimSpace(s.settings.LogFilePath)
	if path == "" {
		return s.applyLoggingLocked()
	}
	file, _, err := openRuntimeEventLogFileWithin(s.eventLogRoot(), expandRuntimeEventLogPath(path), os.O_WRONLY|os.O_TRUNC)
	if err != nil {
		return errors.Join(err, s.applyLoggingLocked())
	}
	if err := file.Close(); err != nil {
		return errors.Join(err, s.applyLoggingLocked())
	}
	return s.applyLoggingLocked()
}

func positiveDuration(raw string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func applyRetentionConfig(settings RuntimeSettings) {
	capturedEventArchive.SetMax(settings.MaxEventCount)
	hotMaxAge := positiveDuration(settings.MaxEventAge)
	if hotMaxAge > 0 {
		capturedEventArchive.EvictOlderThan(time.Now().UTC().Add(-hotMaxAge))
	}

	runtimeSettingsStore.mu.RLock()
	store := runtimeSettingsStore.eventStore
	runtimeSettingsStore.mu.RUnlock()
	if store != nil {
		store.SetRetention(
			settings.EventStoreMaxRecords,
			positiveDuration(settings.EventStoreMaxAge),
		)
		go store.pruneConfiguredRetention()
	}
}

func (s *runtimeState) RecentEvents(limit int) ([]CapturedEventRecord, string, error) {
	return s.RecentEventsContext(context.Background(), limit)
}

func (s *runtimeState) RecentEventsContext(ctx context.Context, limit int) ([]CapturedEventRecord, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, runtimeEventLogRecentTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	} else if limit > runtimeEventLogMaxRecords {
		limit = runtimeEventLogMaxRecords
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.mu.RLock()
	settings := s.settings
	writer := s.logWriter
	store := s.eventStore
	s.mu.RUnlock()

	if settings.LogPersistenceEnabled {
		if store != nil {
			if records, err := store.Recent(ctx, limit); err == nil {
				return records, "pebble", nil
			} else if ctx.Err() != nil {
				return nil, "", ctx.Err()
			} else {
				log.Printf("[WARN] failed to read persisted event database %s: %v", settings.LogFilePath, err)
			}
		}
		logPath := strings.TrimSpace(settings.LogFilePath)
		if logPath != "" && writer != nil {
			if err := writer.FlushContext(ctx); err != nil {
				if ctx.Err() != nil {
					return nil, "", ctx.Err()
				}
				log.Printf("[WARN] failed to flush persisted event log %s before reading: %v", logPath, err)
			}
			if records, err := tailCapturedEventsFileAtRootContext(ctx, s.eventLogRoot(), expandRuntimeEventLogPath(logPath), limit); err == nil {
				return records, "file", nil
			} else if ctx.Err() != nil {
				return nil, "", ctx.Err()
			} else if !errors.Is(err, os.ErrNotExist) {
				log.Printf("[WARN] failed to read persisted event log %s: %v", logPath, err)
			}
		}
	}

	records := capturedEventArchive.Snapshot(limit)
	for index := range records {
		records[index] = normalizeCapturedEventRecord(records[index])
	}
	return records, "memory", nil
}

func (s *runtimeState) EventPageContext(ctx context.Context, limit int, cursor string) ([]CapturedEventRecord, string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, runtimeEventLogRecentTimeout)
	defer cancel()
	if limit <= 0 {
		limit = 50
	} else if limit > runtimeEventLogMaxRecords {
		limit = runtimeEventLogMaxRecords
	}

	s.mu.RLock()
	settings := s.settings
	store := s.eventStore
	s.mu.RUnlock()

	if settings.LogPersistenceEnabled && store != nil {
		records, nextCursor, err := store.Page(ctx, limit, cursor)
		if err != nil {
			return nil, "", "", err
		}
		return records, "pebble", nextCursor, nil
	}
	if strings.TrimSpace(cursor) != "" {
		return nil, "", "", errInvalidEventStoreCursor
	}

	records, source, err := s.RecentEventsContext(ctx, limit)
	return records, source, "", err
}

func (s *runtimeState) EventByIDContext(ctx context.Context, eventID string) (CapturedEventRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return CapturedEventRecord{}, os.ErrNotExist
	}
	s.mu.RLock()
	store := s.eventStore
	s.mu.RUnlock()
	if store != nil {
		record, err := store.GetByID(ctx, eventID)
		if err == nil {
			return record, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return CapturedEventRecord{}, err
		}
	}
	for _, record := range capturedEventArchive.Snapshot(capturedEventArchive.Count()) {
		record = normalizeCapturedEventRecord(record)
		if record.Envelope != nil && record.Envelope.GetEventId() == eventID {
			return record, nil
		}
	}
	return CapturedEventRecord{}, os.ErrNotExist
}

func (s *runtimeState) AppendEvent(record CapturedEventRecord) error {
	_, err := s.enqueueEvent(record)
	return err
}

func (s *runtimeState) enqueueEvent(record CapturedEventRecord) (bool, error) {
	s.mu.RLock()
	writer := s.logWriter
	store := s.eventStore
	enabled := s.settings.LogPersistenceEnabled
	if !enabled || (writer == nil && store == nil) {
		s.mu.RUnlock()
		return false, nil
	}
	if store != nil {
		accepted, err := store.Enqueue(record)
		s.mu.RUnlock()
		return accepted, err
	}
	accepted, err := writer.Enqueue(record)
	s.mu.RUnlock()
	return accepted, err
}

func (s *runtimeState) FlushEventLogContext(ctx context.Context) error {
	s.mu.RLock()
	writer := s.logWriter
	store := s.eventStore
	s.mu.RUnlock()
	if store != nil {
		return store.FlushContext(ctx)
	}
	if writer == nil {
		return nil
	}
	return writer.FlushContext(ctx)
}

func (s *runtimeState) EventLogStatus() runtimeEventLogStatus {
	s.mu.RLock()
	writer := s.logWriter
	store := s.eventStore
	s.mu.RUnlock()
	if store != nil {
		return store.Status()
	}
	if writer == nil {
		return runtimeEventLogStatus{}
	}
	return writer.Status()
}

func (s *runtimeState) eventLogRoot() string {
	if strings.TrimSpace(s.logRoot) != "" {
		return s.logRoot
	}
	return platform.RuntimeSettingsDir()
}

// recordCapturedEvent takes ownership of event: it is redacted in place and
// retained by the archive, the persistence queue and the websocket batch.
// Callers must not read or modify it afterwards.
func recordCapturedEvent(event *pb.Event) CapturedEventRecord {
	return recordCapturedEventWithMonitoring(event, monitorAgentEvent(event))
}

// The broadcaster provides the source Agent's monitoring decision for both the
// source and its derived semantic alerts. Those alerts may have a synthetic
// Security tag, so re-matching the alert itself would incorrectly drop work.
func recordCapturedEventWithMonitoring(event *pb.Event, monitorAllowed bool) CapturedEventRecord {
	if event == nil {
		return CapturedEventRecord{}
	}

	collectorMetricsStore.RecordEvent(event)

	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event:      event,
	})
	record = redactCapturedEventRecord(record, globalRedactionEngine)
	if session := activeDesktopSession.Load(); session != nil {
		if summary, ok := buildRenewDesktopEventSummaryNormalized(record); ok {
			session.publishProto(desktopFrameEventSummary, desktopEventSummaryProto(summary))
		}
	}
	capturedEventArchive.Add(record)
	collectorMetricsStore.RecordCapturedArchive()
	appendStart := time.Now()
	_, appendErr := runtimeSettingsStore.enqueueEvent(record)
	if appendErr != nil {
		collectorMetricsStore.RecordCapturedPersistBatch(0, 1, time.Since(appendStart))
	}
	recording.Default().Record(record)
	otelExporterStore.Record(record)
	// Capture and monitoring are independent: non-monitored events stay
	// observable and persistable, but do not enter expensive analysis queues.
	if monitorAllowed {
		queueLoopDetectionRecord(record)
		research.QueueProcessingRecord(record)
		queueSignalProcessingRecord(record)
		persistSignalProgramLog(record)
	}
	return record
}
