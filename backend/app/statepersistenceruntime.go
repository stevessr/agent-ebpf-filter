package app

import (
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

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
)

// ---- moved from backend/zz_merged_backend.go section statepersistenceruntime.go ----

type runtimeState struct {
	mu        sync.RWMutex
	settings  RuntimeSettings
	logWriter *runtimeEventLogWriter
	logPath   string
	logRoot   string
}

func newRuntimeState() *runtimeState {
	return &runtimeState{}
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
	writer := s.logWriter
	if writer == nil {
		return nil
	}
	err := writer.StopContext(ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if s.logWriter == writer {
		s.logWriter = nil
		s.logPath = ""
	}
	return err
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
			log.Printf("[WARN] disabled runtime event log after writer failure: %v", err)
		}
		return nil
	}
	resolvedPath, err := resolveRuntimeEventLogPathWithin(s.eventLogRoot(), expandRuntimeEventLogPath(s.settings.LogFilePath))
	if err != nil {
		return err
	}
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
		log.Printf("[WARN] previous runtime event log writer stopped after failure: %v", stopErr)
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
		LogPersistenceEnabled: false,
		LogFilePath:           platform.DefaultEventLogPath(),
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
			ArtifactRetentionDays: researchProcessingDefaultArtifactRetentionDays,
			MaxSessionEvents:      researchProcessingDefaultMaxSessionEvents,
			ExportFormats:         researchProcessingDefaultExportFormats,
		},
		SignalProcessing: defaultSignalProcessingSettings(),
	}

	if data, err := os.ReadFile(platform.RuntimeSettingsPath()); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			log.Printf("[WARN] failed to parse runtime settings: %v", err)
			settings = RuntimeSettings{
				LogPersistenceEnabled: false,
				LogFilePath:           platform.DefaultEventLogPath(),
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
					ArtifactRetentionDays: researchProcessingDefaultArtifactRetentionDays,
					MaxSessionEvents:      researchProcessingDefaultMaxSessionEvents,
					ExportFormats:         researchProcessingDefaultExportFormats,
				},
				SignalProcessing: defaultSignalProcessingSettings(),
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
			ArtifactRetentionDays: researchProcessingDefaultArtifactRetentionDays,
			MaxSessionEvents:      researchProcessingDefaultMaxSessionEvents,
			ExportFormats:         researchProcessingDefaultExportFormats,
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
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) Snapshot() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
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
	updateMLRuntimeConfig(s.settings.MLConfig, s.settings.MLConfig.Enabled && clusterManagerStore.IsMaster())
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) TruncateEventLog() error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

func applyRetentionConfig(settings RuntimeSettings) {
	if settings.MaxEventCount > 0 {
		capturedEventArchive.SetMax(settings.MaxEventCount)
	}
	if d, err := time.ParseDuration(settings.MaxEventAge); err == nil && d > 0 {
		capturedEventArchive.EvictOlderThan(time.Now().UTC().Add(-d))
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
	s.mu.RUnlock()

	if settings.LogPersistenceEnabled {
		logPath := strings.TrimSpace(settings.LogFilePath)
		if logPath != "" {
			if writer != nil {
				if err := writer.FlushContext(ctx); err != nil {
					if ctx.Err() != nil {
						return nil, "", ctx.Err()
					}
					log.Printf("[WARN] failed to flush persisted event log %s before reading: %v", logPath, err)
				}
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
	enabled := s.settings.LogPersistenceEnabled
	if writer == nil || !enabled {
		s.mu.RUnlock()
		return false, nil
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
	s.mu.RUnlock()
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
	s.mu.RUnlock()
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

func recordCapturedEvent(event *pb.Event) CapturedEventRecord {
	if event == nil {
		return CapturedEventRecord{}
	}

	collectorMetricsStore.RecordEvent(event)

	eventCopy := cloneProtoEvent(event)
	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event:      eventCopy,
	})
	record = redactCapturedEventRecord(record, globalRedactionEngine)
	capturedEventArchive.Add(record)
	collectorMetricsStore.RecordCapturedArchive()
	appendStart := time.Now()
	_, appendErr := runtimeSettingsStore.enqueueEvent(record)
	if appendErr != nil {
		collectorMetricsStore.RecordCapturedPersistBatch(0, 1, time.Since(appendStart))
	}
	eventRecordingStore.Record(record)
	otelExporterStore.Record(record)
	queueLoopDetectionRecord(record)
	queueResearchProcessingRecord(record)
	queueSignalProcessingRecord(record)
	persistSignalProgramLog(record)
	return record
}
