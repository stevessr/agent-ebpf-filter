package app

import (
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/research"
	"agent-ebpf-filter/pb"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

type runtimeHotSettings struct {
	loopDetection      LoopDetectionSettings
	researchProcessing ResearchProcessingSettings
	signalProcessing   SignalProcessingSettings
}

type runtimeState struct {
	mu          sync.RWMutex
	settings    RuntimeSettings
	hotSettings atomic.Pointer[runtimeHotSettings]
	logWriter   *runtimeEventLogWriter // legacy JSONL compatibility
	eventStore  *runtimeEventStore
	logPath     string
	logRoot     string
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
	if err := platform.MkdirAllAsRealUser(platform.RuntimeSettingsDir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return platform.WriteFileAsRealUser(platform.RuntimeSettingsPath(), data, 0o644)
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
		MLConfig:              MLConfig{AllowRiskScoreThreshold: 0.30},
		EventStoreMaxRecords:  defaultEventStoreMaxRecords,
		EventStoreMaxAge:      defaultEventStoreMaxAge,
		DisabledEventTypes:    defaultRenewDailyDisabledEventTypes(),
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
				MLConfig:              MLConfig{AllowRiskScoreThreshold: 0.30},
				EventStoreMaxRecords:  defaultEventStoreMaxRecords,
				EventStoreMaxAge:      defaultEventStoreMaxAge,
				DisabledEventTypes:    defaultRenewDailyDisabledEventTypes(),
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
	s.publishHotSettingsLocked()
	trackingConfigStore{}.ReplaceDisabledEventTypes(s.settings.DisabledEventTypes)
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) Snapshot() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// publishHotSettingsLocked publishes the immutable sub-settings read on every
// captured event. Configuration changes are rare, while these accessors can be
// called several times per event, so an atomic snapshot avoids repeated
// runtimeState read-lock traffic on the ingestion path.
func (s *runtimeState) publishHotSettingsLocked() {
	s.hotSettings.Store(&runtimeHotSettings{
		loopDetection:      s.settings.LoopDetection,
		researchProcessing: s.settings.ResearchProcessing,
		signalProcessing:   s.settings.SignalProcessing,
	})
}

// The accessors below serve per-event gates. Production runtime updates publish
// immutable snapshots, so reads are lock-free. The locked fallback keeps
// narrow tests and library callers that construct runtimeState literals working.
// Callers must not mutate slices shared with the live settings.

func (s *runtimeState) SignalProcessingSettings() SignalProcessingSettings {
	if s != nil {
		if hot := s.hotSettings.Load(); hot != nil {
			return hot.signalProcessing
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.SignalProcessing
}

func (s *runtimeState) ResearchProcessingSettings() ResearchProcessingSettings {
	if s != nil {
		if hot := s.hotSettings.Load(); hot != nil {
			return hot.researchProcessing
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings.ResearchProcessing
}

func (s *runtimeState) LoopDetectionSettings() LoopDetectionSettings {
	if s != nil {
		if hot := s.hotSettings.Load(); hot != nil {
			return hot.loopDetection
		}
	}
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
	s.publishHotSettingsLocked()
	trackingConfigStore{}.ReplaceDisabledEventTypes(s.settings.DisabledEventTypes)
	ml.UpdateMLRuntimeConfig(s.settings.MLConfig, s.settings.MLConfig.Enabled && clusterManagerStore.IsMaster())
	otelExporterStore.ApplySettings(s.settings)
	return s.settings, nil
}

func (s *runtimeState) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLogWriterLocked(ctx)
}
