package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

type RuntimeSettings struct {
	LogPersistenceEnabled   bool                       `json:"logPersistenceEnabled"`
	LogFilePath             string                     `json:"logFilePath"`
	AccessToken             string                     `json:"accessToken"`
	MaxEventCount           int                        `json:"maxEventCount"`
	MaxEventAge             string                     `json:"maxEventAge"`
	ShellSessionsEnabled    bool                       `json:"shellSessionsEnabled"`
	SystemRunEnabled        bool                       `json:"systemRunEnabled"`
	HookManagementEnabled   bool                       `json:"hookManagementEnabled"`
	PolicyManagementEnabled bool                       `json:"policyManagementEnabled"`
	OtlpEnabled             bool                       `json:"otlpEnabled"`
	OtlpEndpoint            string                     `json:"otlpEndpoint"`
	OtlpServiceName         string                     `json:"otlpServiceName"`
	OtlpHeaders             map[string]string          `json:"otlpHeaders,omitempty"`
	HookSecrets             map[string]string          `json:"hookSecrets,omitempty"`
	MLConfig                MLConfig                   `json:"mlConfig,omitempty"`
	TlsCaptureEnabled       bool                       `json:"tlsCaptureEnabled"`
	DomainForwardProxy      DomainForwardProxySettings `json:"domainForwardProxy"`
}

type CapturedEventRecord struct {
	ReceivedAt time.Time         `json:"receivedAt"`
	Event      *pb.Event         `json:"event"`
	Envelope   *pb.EventEnvelope `json:"-"`
}

type eventArchive struct {
	mu      sync.RWMutex
	records []CapturedEventRecord
	max     int
}

func newEventArchive(max int) *eventArchive {
	if max <= 0 {
		max = 1000
	}
	return &eventArchive{max: max}
}

func (a *eventArchive) Add(record CapturedEventRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.records = append(a.records, record)
	if len(a.records) > a.max {
		copy(a.records, a.records[len(a.records)-a.max:])
		a.records = a.records[:a.max]
	}
}

func (a *eventArchive) Snapshot(limit int) []CapturedEventRecord {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if limit <= 0 || limit > len(a.records) {
		limit = len(a.records)
	}
	if limit == 0 {
		return nil
	}

	out := make([]CapturedEventRecord, limit)
	copy(out, a.records[len(a.records)-limit:])
	return out
}

func (a *eventArchive) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.records = nil
}

func (a *eventArchive) SetMax(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if n <= 0 {
		n = 1000
	}
	a.max = n
	if len(a.records) > a.max {
		copy(a.records, a.records[len(a.records)-a.max:])
		a.records = a.records[:a.max]
	}
}

func (a *eventArchive) EvictOlderThan(threshold time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	keep := 0
	for _, r := range a.records {
		if !r.ReceivedAt.Before(threshold) {
			a.records[keep] = r
			keep++
		}
	}
	a.records = a.records[:keep]
}

func (a *eventArchive) Count() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.records)
}

type runtimeState struct {
	mu        sync.RWMutex
	settings  RuntimeSettings
	logFile   *os.File
	logWriter *bufio.Writer
}

func newRuntimeState() *runtimeState {
	return &runtimeState{}
}

var (
	runtimeSettingsStore = newRuntimeState()
	capturedEventArchive = newEventArchive(1500)
)

func runtimeSettingsDir() string {
	return filepath.Join(getRealHomeDir(), ".config", "agent-ebpf-filter")
}

func runtimeSettingsPath() string {
	return filepath.Join(runtimeSettingsDir(), "runtime.json")
}

func defaultEventLogPath() string {
	return filepath.Join(runtimeSettingsDir(), "events.jsonl")
}

func generateAccessToken() (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func normalizeRuntimeSettings(settings *RuntimeSettings) error {
	if settings == nil {
		return errors.New("runtime settings are nil")
	}
	if strings.TrimSpace(settings.LogFilePath) == "" {
		settings.LogFilePath = defaultEventLogPath()
	}
	if strings.TrimSpace(settings.AccessToken) == "" {
		token, err := generateAccessToken()
		if err != nil {
			return err
		}
		settings.AccessToken = token
	}
	if settings.MaxEventCount <= 0 {
		settings.MaxEventCount = 1500
	}
	if strings.TrimSpace(settings.MaxEventAge) == "" {
		settings.MaxEventAge = "0"
	}
	if settings.HookSecrets == nil {
		settings.HookSecrets = make(map[string]string)
	}
	if settings.OtlpHeaders == nil {
		settings.OtlpHeaders = make(map[string]string)
	}
	settings.OtlpEndpoint = strings.TrimSpace(settings.OtlpEndpoint)
	if strings.TrimSpace(settings.OtlpServiceName) == "" {
		settings.OtlpServiceName = "agent-ebpf-filter"
	}
	normalizeDomainForwardProxySettings(&settings.DomainForwardProxy)
	for _, hook := range availableHooks {
		if strings.TrimSpace(settings.HookSecrets[hook.ID]) == "" {
			token, err := generateAccessToken()
			if err != nil {
				return err
			}
			settings.HookSecrets[hook.ID] = token
		}
	}
	// ML config defaults
	if settings.MLConfig.BlockConfidenceThreshold == 0 {
		settings.MLConfig.BlockConfidenceThreshold = 0.85
	}
	if settings.MLConfig.MlMinConfidence == 0 {
		settings.MLConfig.MlMinConfidence = 0.60
	}
	if settings.MLConfig.LowAnomalyThreshold == 0 {
		settings.MLConfig.LowAnomalyThreshold = 0.30
	}
	if settings.MLConfig.HighAnomalyThreshold == 0 {
		settings.MLConfig.HighAnomalyThreshold = 0.70
	}
	if settings.MLConfig.RuleOverridePriority == 0 {
		settings.MLConfig.RuleOverridePriority = 100
	}
	if settings.MLConfig.MinSamplesForTraining == 0 {
		settings.MLConfig.MinSamplesForTraining = 1000
	}
	if settings.MLConfig.TrainInterval == "" {
		settings.MLConfig.TrainInterval = "24h"
	}
	if settings.MLConfig.FeatureHistorySize == 0 {
		settings.MLConfig.FeatureHistorySize = 100
	}
	if settings.MLConfig.ModelType == "" {
		settings.MLConfig.ModelType = ModelRandomForest
	}
	if _, ok := modelRegistry[settings.MLConfig.ModelType]; !ok {
		settings.MLConfig.ModelType = ModelRandomForest
	}
	if settings.MLConfig.ModelPath == "" {
		settings.MLConfig.ModelPath = filepath.Join(runtimeSettingsDir(), "ml_model.bin")
	}
	if settings.MLConfig.NumTrees == 0 {
		settings.MLConfig.NumTrees = 31
	}
	if settings.MLConfig.MaxDepth == 0 {
		settings.MLConfig.MaxDepth = 8
	}
	if settings.MLConfig.MinSamplesLeaf == 0 {
		settings.MLConfig.MinSamplesLeaf = 5
	}
	if settings.MLConfig.ValidationSplitRatio == 0 {
		settings.MLConfig.ValidationSplitRatio = 0.20
	}
	if settings.MLConfig.LlmTimeoutSeconds == 0 {
		settings.MLConfig.LlmTimeoutSeconds = 45
	}
	if settings.MLConfig.LlmMaxTokens == 0 {
		settings.MLConfig.LlmMaxTokens = 256
	}
	if strings.TrimSpace(settings.MLConfig.LlmSystemPrompt) == "" {
		settings.MLConfig.LlmSystemPrompt = defaultLLMScoringSystemPrompt
	}
	if _, ok := firstRuntimeEnv("AGENT_ML_ENABLED"); !ok {
		settings.MLConfig.Enabled = true
	}
	return nil
}

func seedRuntimeSettingsFromEnv(settings *RuntimeSettings) {
	if settings == nil {
		return
	}
	seedRuntimeAccessTokenFromEnv(settings)
	applyRuntimeBoolEnv(&settings.LogPersistenceEnabled, "AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED")
	applyRuntimeStringEnv(&settings.LogFilePath, "AGENT_RUNTIME_LOG_FILE_PATH")
	applyRuntimeIntEnv(&settings.MaxEventCount, "AGENT_RUNTIME_MAX_EVENT_COUNT")
	applyRuntimeStringEnv(&settings.MaxEventAge, "AGENT_RUNTIME_MAX_EVENT_AGE")
	applyRuntimeBoolEnv(&settings.ShellSessionsEnabled, "AGENT_RUNTIME_SHELL_SESSIONS_ENABLED")
	applyRuntimeBoolEnv(&settings.SystemRunEnabled, "AGENT_RUNTIME_SYSTEM_RUN_ENABLED")
	applyRuntimeBoolEnv(&settings.HookManagementEnabled, "AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED")
	applyRuntimeBoolEnv(&settings.PolicyManagementEnabled, "AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED")
	applyRuntimeBoolEnv(&settings.OtlpEnabled, "AGENT_RUNTIME_OTLP_ENABLED")
	applyRuntimeStringEnv(&settings.OtlpEndpoint, "AGENT_RUNTIME_OTLP_ENDPOINT")
	applyRuntimeStringEnv(&settings.OtlpServiceName, "AGENT_RUNTIME_OTLP_SERVICE_NAME")
	applyRuntimeBoolEnv(&settings.TlsCaptureEnabled, "AGENT_RUNTIME_TLS_CAPTURE_ENABLED")
	applyRuntimeBoolEnv(&settings.DomainForwardProxy.Enabled, "AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED")
	applyRuntimeIntEnv(&settings.DomainForwardProxy.HTTPPort, "AGENT_RUNTIME_DOMAIN_HTTP_PORT")
	applyRuntimeIntEnv(&settings.DomainForwardProxy.HTTPSPort, "AGENT_RUNTIME_DOMAIN_HTTPS_PORT")
	applyRuntimeStringEnv(&settings.DomainForwardProxy.DefaultScheme, "AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME")
	applyRuntimeBoolEnv(&settings.DomainForwardProxy.AllowAnyHost, "AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST")
	applyRuntimeStringEnv(&settings.DomainForwardProxy.DNSResolver, "AGENT_RUNTIME_DOMAIN_DNS_RESOLVER")
	applyRuntimeIntEnv(&settings.DomainForwardProxy.DialTimeoutSeconds, "AGENT_RUNTIME_DOMAIN_DIAL_TIMEOUT_SECONDS")
	applyRuntimeStringEnv(&settings.DomainForwardProxy.CertFile, "AGENT_RUNTIME_DOMAIN_CERT_FILE")
	applyRuntimeStringEnv(&settings.DomainForwardProxy.KeyFile, "AGENT_RUNTIME_DOMAIN_KEY_FILE")
	seedRuntimeMLConfigFromEnv(&settings.MLConfig)
}

func seedRuntimeAccessTokenFromEnv(settings *RuntimeSettings) {
	if settings == nil {
		return
	}
	if strings.TrimSpace(settings.AccessToken) != "" {
		return
	}
	if envToken, ok := firstRuntimeEnv("AGENT_API_KEY", "AGENT_ACCESS_TOKEN", "AGENT_EBPF_ACCESS_TOKEN"); ok {
		settings.AccessToken = envToken
	}
}

func seedRuntimeMLConfigFromEnv(cfg *MLConfig) {
	if cfg == nil {
		return
	}
	applyRuntimeBoolEnv(&cfg.Enabled, "AGENT_ML_ENABLED")
	applyRuntimeModelTypeEnv(&cfg.ModelType, "AGENT_ML_MODEL_TYPE")
	applyRuntimeStringEnv(&cfg.ModelPath, "AGENT_ML_MODEL_PATH")
	applyRuntimeBoolEnv(&cfg.AutoTrain, "AGENT_ML_AUTO_TRAIN")
	applyRuntimeStringEnv(&cfg.TrainInterval, "AGENT_ML_TRAIN_INTERVAL")
	applyRuntimeIntEnv(&cfg.MinSamplesForTraining, "AGENT_ML_MIN_SAMPLES_FOR_TRAINING")
	applyRuntimeFloatEnv(&cfg.BlockConfidenceThreshold, "AGENT_ML_BLOCK_CONFIDENCE_THRESHOLD")
	applyRuntimeFloatEnv(&cfg.MlMinConfidence, "AGENT_ML_MIN_CONFIDENCE")
	applyRuntimeFloatEnv(&cfg.LowAnomalyThreshold, "AGENT_ML_LOW_ANOMALY_THRESHOLD")
	applyRuntimeFloatEnv(&cfg.HighAnomalyThreshold, "AGENT_ML_HIGH_ANOMALY_THRESHOLD")
	applyRuntimeBoolEnv(&cfg.ActiveLearningEnabled, "AGENT_ML_ACTIVE_LEARNING_ENABLED")
	applyRuntimeIntEnv(&cfg.FeatureHistorySize, "AGENT_ML_FEATURE_HISTORY_SIZE")
	applyRuntimeIntEnv(&cfg.NumTrees, "AGENT_ML_NUM_TREES")
	applyRuntimeIntEnv(&cfg.MaxDepth, "AGENT_ML_MAX_DEPTH")
	applyRuntimeIntEnv(&cfg.MinSamplesLeaf, "AGENT_ML_MIN_SAMPLES_LEAF")
	applyRuntimeFloatEnv(&cfg.ValidationSplitRatio, "AGENT_ML_VALIDATION_SPLIT_RATIO")
	applyRuntimeBoolEnv(&cfg.BalanceClasses, "AGENT_ML_BALANCE_CLASSES")
	applyRuntimeBoolEnv(&cfg.LlmEnabled, "AGENT_LLM_ENABLED", "LLM_ENABLED")
	applyRuntimeStringEnv(&cfg.LlmBaseURL, "AGENT_LLM_BASE_URL", "LLM_BASE_URL", "OPENAI_BASE_URL")
	applyRuntimeStringEnv(&cfg.LlmAPIKey, "AGENT_LLM_API_KEY", "LLM_API_KEY", "OPENAI_API_KEY")
	applyRuntimeStringEnv(&cfg.LlmModel, "AGENT_LLM_MODEL", "LLM_MODEL", "OPENAI_MODEL")
	applyRuntimeIntEnv(&cfg.LlmTimeoutSeconds, "AGENT_LLM_TIMEOUT_SECONDS", "LLM_TIMEOUT_SECONDS")
	applyRuntimeFloatEnv(&cfg.LlmTemperature, "AGENT_LLM_TEMPERATURE", "LLM_TEMPERATURE")
	applyRuntimeIntEnv(&cfg.LlmMaxTokens, "AGENT_LLM_MAX_TOKENS", "LLM_MAX_TOKENS")
	applyRuntimeStringEnv(&cfg.LlmSystemPrompt, "AGENT_LLM_SYSTEM_PROMPT", "LLM_SYSTEM_PROMPT")
}

func firstRuntimeEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, true
		}
	}
	return "", false
}

func applyRuntimeStringEnv(dst *string, keys ...string) {
	if value, ok := firstRuntimeEnv(keys...); ok {
		*dst = value
	}
}

func applyRuntimeBoolEnv(dst *bool, keys ...string) {
	if value, ok := firstRuntimeEnv(keys...); ok {
		*dst = parseBoolEnv(value)
	}
}

func applyRuntimeIntEnv(dst *int, keys ...string) {
	if value, ok := firstRuntimeEnv(keys...); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			*dst = parsed
		}
	}
}

func applyRuntimeFloatEnv(dst *float64, keys ...string) {
	if value, ok := firstRuntimeEnv(keys...); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			*dst = parsed
		}
	}
}

func applyRuntimeModelTypeEnv(dst *ModelType, keys ...string) {
	if value, ok := firstRuntimeEnv(keys...); ok {
		*dst = ModelType(value)
	}
}

func (s *runtimeState) saveLocked() error {
	if err := mkdirAllAsRealUser(runtimeSettingsDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.settings, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAsRealUser(runtimeSettingsPath(), data, 0644)
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
	if err := mkdirAllAsRealUser(filepath.Dir(s.settings.LogFilePath), 0755); err != nil {
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
		LogFilePath:           defaultEventLogPath(),
		MaxEventCount:         1500,
		MaxEventAge:           "0",
	}

	if data, err := os.ReadFile(runtimeSettingsPath()); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			log.Printf("[WARN] failed to parse runtime settings: %v", err)
			settings = RuntimeSettings{
				LogPersistenceEnabled: false,
				LogFilePath:           defaultEventLogPath(),
				MaxEventCount:         1500,
				MaxEventAge:           "0",
			}
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
	if envToken, ok := firstRuntimeEnv("AGENT_API_KEY", "AGENT_ACCESS_TOKEN", "AGENT_EBPF_ACCESS_TOKEN"); ok {
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
	if limit <= 0 {
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
	if limit <= 0 {
		limit = 50
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	buffer := make([]CapturedEventRecord, 0, limit)
	for scanner.Scan() {
		var record CapturedEventRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		record = normalizeCapturedEventRecord(record)
		if record.Event == nil {
			continue
		}
		if len(buffer) < limit {
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
	return record
}
