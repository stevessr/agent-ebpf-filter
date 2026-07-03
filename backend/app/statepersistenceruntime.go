package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"bufio"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section statepersistenceruntime.go ----

type runtimeState struct {
	mu        sync.RWMutex
	settings  RuntimeSettings
	logFile   *os.File
	logWriter *bufio.Writer
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

func (s *runtimeState) closeLogWriterLocked() {
	if s.logWriter != nil {
		_ = s.logWriter.Flush()
		s.logWriter = nil
	}
	if s.logFile != nil {
		_ = s.logFile.Close()
		s.logFile = nil
	}
}

func (s *runtimeState) applyLoggingLocked() error {
	s.closeLogWriterLocked()
	if !s.settings.LogPersistenceEnabled {
		return nil
	}
	if err := platform.MkdirAllAsRealUser(filepath.Dir(s.settings.LogFilePath), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.settings.LogFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	s.logFile = file
	s.logWriter = bufio.NewWriter(file)
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

	s.settings.LogPersistenceEnabled = enabled
	if strings.TrimSpace(path) != "" {
		s.settings.LogFilePath = path
	}
	if err := normalizeRuntimeSettings(&s.settings); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.saveLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.applyLoggingLocked(); err != nil {
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

	seedRuntimeAccessTokenFromEnv(&settings)
	if settings.MLConfig == (MLConfig{}) {
		settings.MLConfig = s.settings.MLConfig
	} else if settings.MLConfig.LlmAPIKey == "" {
		settings.MLConfig.LlmAPIKey = s.settings.MLConfig.LlmAPIKey
	}
	s.settings = settings
	if err := normalizeRuntimeSettings(&s.settings); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.saveLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	if err := s.applyLoggingLocked(); err != nil {
		return RuntimeSettings{}, err
	}
	mlConfig = s.settings.MLConfig
	mlEnabled = s.settings.MLConfig.Enabled && clusterManagerStore.IsMaster()
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) TruncateEventLog() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeLogWriterLocked()
	path := strings.TrimSpace(s.settings.LogFilePath)
	if path == "" {
		return nil
	}
	if err := os.Truncate(path, 0); err != nil {
		return err
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
	if limit == 0 {
		limit = 50
	}
	s.mu.RLock()
	settings := s.settings
	s.mu.RUnlock()

	if settings.LogPersistenceEnabled {
		logPath := strings.TrimSpace(settings.LogFilePath)
		if logPath != "" {
			if records, err := tailCapturedEventsFile(logPath, limit); err == nil {
				return records, "file", nil
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
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.logWriter == nil {
		return nil
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := s.logWriter.Write(payload); err != nil {
		return err
	}
	if err := s.logWriter.WriteByte('\n'); err != nil {
		return err
	}
	return s.logWriter.Flush()
}

func tailCapturedEventsFile(path string, limit int) ([]CapturedEventRecord, error) {
	if limit == 0 {
		limit = 50
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	buffer := make([]CapturedEventRecord, 0)
	if limit > 0 {
		buffer = make([]CapturedEventRecord, 0, limit)
	}
	for scanner.Scan() {
		var record CapturedEventRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		record = normalizeCapturedEventRecord(record)
		if record.Event == nil {
			continue
		}
		if limit < 0 || len(buffer) < limit {
			buffer = append(buffer, record)
			continue
		}
		copy(buffer, buffer[1:])
		buffer[len(buffer)-1] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return buffer, nil
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
	capturedEventArchive.Add(record)
	appendStart := time.Now()
	if err := runtimeSettingsStore.AppendEvent(record); err != nil {
		log.Printf("[WARN] failed to append captured event: %v", err)
	}
	eventRecordingStore.Record(record)
	collectorMetricsStore.SetPersistAppendLatency(time.Since(appendStart))
	otelExporterStore.Record(record)
	queueLoopDetectionRecord(record)
	queueResearchProcessingRecord(record)
	return record
}
