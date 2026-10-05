package app

import (
	"agent-ebpf-filter/app/recording"
	"agent-ebpf-filter/app/research"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
)

type runtimeState struct {
	mu         sync.RWMutex
	settings   RuntimeSettings
	logWriter  *runtimeEventLogWriter // legacy JSONL compatibility
	eventStore *runtimeEventStore
	logPath    string
	logRoot    string
}

func newRuntimeState() *runtimeState {
	return &runtimeState{}
}

func defaultRenewDailyDisabledEventTypes() []uint32 {
	return []uint32{
		uint32(pb.EventType_OPENAT),
		uint32(pb.EventType_IOCTL),
		uint32(pb.EventType_READ),
		uint32(pb.EventType_OPEN),
		uint32(pb.EventType_NETWORK_SENDTO),
		uint32(pb.EventType_NETWORK_RECVFROM),
		uint32(pb.EventType_SOCKET),
		uint32(pb.EventType_ACCEPT),
		uint32(pb.EventType_ACCEPT4),
		uint32(pb.EventType_TCP_STATE_CHANGE),
		uint32(pb.EventType_GENERIC_SYSCALL),
	}
}

func generateAccessToken() (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := cryptorand.Read(tokenBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func (s *runtimeState) saveLocked() error {
	if err := platform.MkdirAllAsRealUser(platform.RuntimeSettingsDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(platform.RuntimeSettingsPath(), data, 0644)
}

func cloneRuntimeSettings(settings RuntimeSettings) (RuntimeSettings, error) {
	payload, err := json.Marshal(settings)
	if err != nil {
		return RuntimeSettings{}, err
	}
	var cloned RuntimeSettings
	if err := json.Unmarshal(payload, &cloned); err != nil {
		return RuntimeSettings{}, err
	}
	return cloned, nil
}

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

func (s *runtimeState) applyAndSaveSettingsLocked(previous RuntimeSettings) error {
	if err := s.applyLoggingLocked(); err != nil {
		s.settings = previous
		if rollbackErr := s.applyLoggingLocked(); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.settings = previous
		if rollbackErr := s.applyLoggingLocked(); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	return nil
}

func (s *runtimeState) LoadOrCreate() (RuntimeSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings := RuntimeSettings{
		LogPersistenceEnabled: true,
		LogFilePath:           platform.DefaultEventLogPath(),
		EventStoreMaxRecords: defaultEventStoreMaxRecords,
		EventStoreMaxAge:     defaultEventStoreMaxAge,
		DisabledEventTypes:   defaultRenewDailyDisabledEventTypes(),
		IgnoredPaths:          defaultIgnoredEventPaths(),
		MaxEventCount:         1500,
		MaxEventAge:           "0",
		LoopDetection: LoopDetectionSettings{
			WindowSeconds:      30,
			RepeatThreshold:    5,
			MaxContexts:        512,
			QueueSize:          2048,
			EmitSemanticAlerts: true,
		},
		ResearchProcessing: ResearchProcessingSettings{
			MaxEvents:             5000,
			QueueSize:             2048,
			TimelineBucketSeconds: 60,
			TopK:                  20,
			RecentSamples:         25,
			ArtifactRetentionDays: research.ArtifactRetentionDaysDefault,
			MaxSessionEvents:      research.MaxSessionEventsDefault,
			ExportFormats:         research.ExportFormatsDefault,
		},
		SignalProcessing: defaultSignalProcessingSettings(),
	}

	if data, err := os.ReadFile(platform.RuntimeSettingsPath()); err == nil {
		var rawSettings map[string]json.RawMessage
		_ = json.Unmarshal(data, &rawSettings)
		if err := json.Unmarshal(data, &settings); err != nil {
			log.Printf("[WARN] failed to parse runtime settings: %v", err)
			settings = RuntimeSettings{
				LogPersistenceEnabled: true,
				LogFilePath:           platform.DefaultEventLogPath(),
				EventStoreMaxRecords:  defaultEventStoreMaxRecords,
				EventStoreMaxAge:      defaultEventStoreMaxAge,
				DisabledEventTypes:    defaultRenewDailyDisabledEventTypes(),
				IgnoredPaths:           defaultIgnoredEventPaths(),
				MaxEventCount:          1500,
				MaxEventAge:           "0",
				LoopDetection: LoopDetectionSettings{
					WindowSeconds:      30,
					RepeatThreshold:    5,
					MaxContexts:        512,
					QueueSize:          2048,
					EmitSemanticAlerts: true,
				},
				ResearchProcessing: ResearchProcessingSettings{
					MaxEvents:             5000,
					QueueSize:             2048,
					TimelineBucketSeconds: 60,
					TopK:                  20,
					RecentSamples:         25,
					ArtifactRetentionDays: research.ArtifactRetentionDaysDefault,
					MaxSessionEvents:      research.MaxSessionEventsDefault,
					ExportFormats:         research.ExportFormatsDefault,
				},
				SignalProcessing: defaultSignalProcessingSettings(),
			}
		} else {
			if _, explicitlyConfigured := rawSettings["logPersistenceEnabled"]; !explicitlyConfigured {
				// Persistence became the safe default for Renew. Preserve an explicit
				// false from existing installations, but enable it when upgrading a
				// runtime file that predates this field.
				settings.LogPersistenceEnabled = true
			}
			if _, explicitlyConfigured := rawSettings["disabledEventTypes"]; !explicitlyConfigured {
				// Existing installations predate Renew's monitoring profiles and
				// historically collected all event types. Preserve that behavior.
				// Fresh installations use the Daily profile defaults above.
				settings.DisabledEventTypes = nil
			}
		}
	}
	if settings.LoopDetection == (LoopDetectionSettings{}) {
		settings.LoopDetection = LoopDetectionSettings{
			WindowSeconds:      30,
			RepeatThreshold:    5,
			MaxContexts:        512,
			QueueSize:          2048,
			EmitSemanticAlerts: true,
		}
	}
	if settings.ResearchProcessing == (ResearchProcessingSettings{}) {
		settings.ResearchProcessing = ResearchProcessingSettings{
			MaxEvents:             5000,
			QueueSize:             2048,
			TimelineBucketSeconds: 60,
			TopK:                  20,
			RecentSamples:         25,
			ArtifactRetentionDays: research.ArtifactRetentionDaysDefault,
			MaxSessionEvents:      research.MaxSessionEventsDefault,
			ExportFormats:         research.ExportFormatsDefault,
		}
	}
	if settings.SignalProcessing.QueueSize == 0 &&
		settings.SignalProcessing.CronIntervalSeconds == 0 &&
		settings.SignalProcessing.DefaultTTLSeconds == 0 &&
		settings.SignalProcessing.MaxStates == 0 &&
		strings.TrimSpace(settings.SignalProcessing.ProtoLogCompression) == "" &&
		len(settings.SignalProcessing.Rules) == 0 &&
		len(settings.SignalProcessing.SelectedPrograms) == 0 {
		settings.SignalProcessing = defaultSignalProcessingSettings()
	}

	seedRuntimeSettingsFromEnv(&settings)
	if err := normalizeRuntimeSettings(&settings); err != nil {
		return RuntimeSettings{}, err
	}

	s.settings = settings
	if err := s.saveLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.applyLoggingLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	trackingConfigStore{}.ReplaceDisabledEventTypes(s.settings.DisabledEventTypes)
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) Snapshot() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// The accessors below serve per-event gates. They copy one sub-struct under
// the read lock instead of the whole RuntimeSettings, and callers must not
// mutate the slices they share with the live settings.

func (s *runtimeState) SignalProcessingSettings() SignalProcessingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.SignalProcessing
}

func (s *runtimeState) ResearchProcessingSettings() ResearchProcessingSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.ResearchProcessing
}

func (s *runtimeState) LoopDetectionSettings() LoopDetectionSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.LoopDetection
}

func (s *runtimeState) KernelRiskFeedbackGate() (policyManagement bool, feedback KernelRiskFeedbackSettings) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.PolicyManagementEnabled, s.settings.KernelRiskFeedback
}

func (s *runtimeState) ExpectedToken() string {
	s.mu.RLock()
	token := strings.TrimSpace(s.settings.AccessToken)
	s.mu.RUnlock()
	if token != "" {
		return token
	}
	if envToken, ok := platform.FirstEnv("AGENT_API_KEY", "AGENT_ACCESS_TOKEN", "AGENT_EBPF_ACCESS_TOKEN"); ok {
		return envToken
	}
	return ""
}

func (s *runtimeState) HookSecret(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.settings.HookSecrets == nil {
		return ""
	}
	return strings.TrimSpace(s.settings.HookSecrets[id])
}

func (s *runtimeState) UpdateLogging(enabled bool, path string) (RuntimeSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.settings
	candidate, err := cloneRuntimeSettings(s.settings)
	if err != nil {
		return RuntimeSettings{}, err
	}
	candidate.LogPersistenceEnabled = enabled
	if strings.TrimSpace(path) != "" {
		candidate.LogFilePath = path
	}
	if err := normalizeRuntimeSettings(&candidate); err != nil {
		return RuntimeSettings{}, err
	}
	s.settings = candidate
	if err := s.applyAndSaveSettingsLocked(previous); err != nil {
		return RuntimeSettings{}, err
	}
	return s.settings, nil
}

func (s *runtimeState) RotateAccessToken() (RuntimeSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := generateAccessToken()
	if err != nil {
		return RuntimeSettings{}, err
	}
	s.settings.AccessToken = token
	if err := normalizeRuntimeSettings(&s.settings); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.saveLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	return s.settings, nil
}

func (s *runtimeState) Replace(settings RuntimeSettings) (RuntimeSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	previous := s.settings
	seedRuntimeAccessTokenFromEnv(&settings)
	if settings.MLConfig == (MLConfig{}) {
		settings.MLConfig = s.settings.MLConfig
	} else if settings.MLConfig.LlmAPIKey == "" {
		settings.MLConfig.LlmAPIKey = s.settings.MLConfig.LlmAPIKey
	}
	if err := normalizeRuntimeSettings(&settings); err != nil {
		return RuntimeSettings{}, err
	}
	s.settings = settings
	if err := s.applyAndSaveSettingsLocked(previous); err != nil {
		return RuntimeSettings{}, err
	}
	trackingConfigStore{}.ReplaceDisabledEventTypes(s.settings.DisabledEventTypes)
	ml.UpdateMLRuntimeConfig(s.settings.MLConfig, s.settings.MLConfig.Enabled && clusterManagerStore.IsMaster())
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
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
	if s == nil {
		return false, nil
	}
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
	if s == nil {
		return nil
	}
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
	if s == nil {
		return runtimeEventLogStatus{}
	}
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

func (s *runtimeState) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLogWriterLocked(ctx)
}

func (s *runtimeState) eventLogRoot() string {
	if s != nil && strings.TrimSpace(s.logRoot) != "" {
		return s.logRoot
	}
	return platform.RuntimeSettingsDir()
}

// recordCapturedEvent takes ownership of event: it is redacted in place and
// retained by the archive, the persistence queue and the websocket batch.
// Callers must not read or modify it afterwards.
func recordCapturedEvent(event *pb.Event) CapturedEventRecord {
	if event == nil {
		return CapturedEventRecord{}
	}

	collectorMetricsStore.RecordEvent(event)

	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event:      event,
	})
	record = redactCapturedEventRecord(record, globalRedactionEngine)
	capturedEventArchive.Add(record)
	collectorMetricsStore.RecordCapturedArchive()
	appendStart := time.Now()
	_, appendErr := runtimeSettingsStore.enqueueEvent(record)
	if appendErr != nil {
		collectorMetricsStore.RecordCapturedPersistBatch(0, 1, time.Since(appendStart))
	}
	recording.Default().Record(record)
	otelExporterStore.Record(record)
	queueLoopDetectionRecord(record)
	research.QueueProcessingRecord(record)
	queueSignalProcessingRecord(record)
	persistSignalProgramLog(record)
	return record
}
