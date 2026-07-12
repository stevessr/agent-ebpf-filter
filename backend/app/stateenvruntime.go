package app

import (
	"agent-ebpf-filter/app/platform"
	"errors"
	"path/filepath"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section stateenvruntime.go ----

func normalizeRuntimeSettings(settings *RuntimeSettings) error {
	if settings == nil {
		return errors.New("runtime settings are nil")
	}
	if strings.TrimSpace(settings.LogFilePath) == "" {
		settings.LogFilePath = platform.DefaultEventLogPath()
	}
	logPath, err := resolveRuntimeEventLogPath(settings.LogFilePath)
	if err != nil {
		return err
	}
	settings.LogFilePath = logPath
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
	normalizeKernelRiskFeedbackSettings(&settings.KernelRiskFeedback)
	normalizeLoopDetectionSettings(&settings.LoopDetection)
	normalizeResearchProcessingSettings(&settings.ResearchProcessing)
	normalizeSignalProcessingSettings(&settings.SignalProcessing)
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
		settings.MLConfig.ModelPath = filepath.Join(platform.RuntimeSettingsDir(), "ml_model.bin")
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
	if _, ok := platform.FirstEnv("AGENT_ML_ENABLED"); !ok {
		settings.MLConfig.Enabled = true
	}
	return nil
}

func seedRuntimeSettingsFromEnv(settings *RuntimeSettings) {
	if settings == nil {
		return
	}
	seedRuntimeAccessTokenFromEnv(settings)
	platform.ApplyBoolEnv(&settings.LogPersistenceEnabled, "AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED")
	platform.ApplyStringEnv(&settings.LogFilePath, "AGENT_RUNTIME_LOG_FILE_PATH")
	platform.ApplyIntEnv(&settings.MaxEventCount, "AGENT_RUNTIME_MAX_EVENT_COUNT")
	platform.ApplyStringEnv(&settings.MaxEventAge, "AGENT_RUNTIME_MAX_EVENT_AGE")
	platform.ApplyBoolEnv(&settings.ShellSessionsEnabled, "AGENT_RUNTIME_SHELL_SESSIONS_ENABLED")
	platform.ApplyBoolEnv(&settings.SystemRunEnabled, "AGENT_RUNTIME_SYSTEM_RUN_ENABLED")
	platform.ApplyBoolEnv(&settings.HookManagementEnabled, "AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED")
	platform.ApplyBoolEnv(&settings.PolicyManagementEnabled, "AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED")
	platform.ApplyBoolEnv(&settings.OtlpEnabled, "AGENT_RUNTIME_OTLP_ENABLED")
	platform.ApplyStringEnv(&settings.OtlpEndpoint, "AGENT_RUNTIME_OTLP_ENDPOINT")
	platform.ApplyStringEnv(&settings.OtlpServiceName, "AGENT_RUNTIME_OTLP_SERVICE_NAME")
	platform.ApplyBoolEnv(&settings.TlsCaptureEnabled, "AGENT_RUNTIME_TLS_CAPTURE_ENABLED")
	platform.ApplyBoolEnv(&settings.KernelRiskFeedback.Enabled, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENABLED")
	platform.ApplyFloatEnv(&settings.KernelRiskFeedback.MinRiskScore, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MIN_SCORE")
	platform.ApplyBoolEnv(&settings.KernelRiskFeedback.EnforceNetwork, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_NETWORK")
	platform.ApplyBoolEnv(&settings.KernelRiskFeedback.EnforceFileNames, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_FILE_NAMES")
	platform.ApplyBoolEnv(&settings.KernelRiskFeedback.EnforceExec, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_EXEC")
	platform.ApplyIntEnv(&settings.KernelRiskFeedback.MaxActionsPerMinute, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MAX_ACTIONS_PER_MINUTE")
	platform.ApplyBoolEnv(&settings.LoopDetection.Enabled, "AGENT_RUNTIME_LOOP_DETECTION_ENABLED")
	platform.ApplyIntEnv(&settings.LoopDetection.WindowSeconds, "AGENT_RUNTIME_LOOP_DETECTION_WINDOW_SECONDS")
	platform.ApplyIntEnv(&settings.LoopDetection.RepeatThreshold, "AGENT_RUNTIME_LOOP_DETECTION_REPEAT_THRESHOLD")
	platform.ApplyIntEnv(&settings.LoopDetection.MaxContexts, "AGENT_RUNTIME_LOOP_DETECTION_MAX_CONTEXTS")
	platform.ApplyIntEnv(&settings.LoopDetection.QueueSize, "AGENT_RUNTIME_LOOP_DETECTION_QUEUE_SIZE")
	platform.ApplyBoolEnv(&settings.LoopDetection.EmitSemanticAlerts, "AGENT_RUNTIME_LOOP_DETECTION_EMIT_ALERTS")
	platform.ApplyBoolEnv(&settings.ResearchProcessing.Enabled, "AGENT_RUNTIME_RESEARCH_PROCESSING_ENABLED")
	platform.ApplyIntEnv(&settings.ResearchProcessing.MaxEvents, "AGENT_RUNTIME_RESEARCH_PROCESSING_MAX_EVENTS")
	platform.ApplyIntEnv(&settings.ResearchProcessing.QueueSize, "AGENT_RUNTIME_RESEARCH_PROCESSING_QUEUE_SIZE")
	platform.ApplyIntEnv(&settings.ResearchProcessing.TimelineBucketSeconds, "AGENT_RUNTIME_RESEARCH_PROCESSING_TIMELINE_BUCKET_SECONDS")
	platform.ApplyIntEnv(&settings.ResearchProcessing.TopK, "AGENT_RUNTIME_RESEARCH_PROCESSING_TOP_K")
	platform.ApplyIntEnv(&settings.ResearchProcessing.RecentSamples, "AGENT_RUNTIME_RESEARCH_PROCESSING_RECENT_SAMPLES")
	platform.ApplyIntEnv(&settings.ResearchProcessing.ArtifactRetentionDays, "AGENT_RUNTIME_RESEARCH_PROCESSING_ARTIFACT_RETENTION_DAYS")
	platform.ApplyIntEnv(&settings.ResearchProcessing.MaxSessionEvents, "AGENT_RUNTIME_RESEARCH_PROCESSING_MAX_SESSION_EVENTS")
	platform.ApplyStringEnv(&settings.ResearchProcessing.ExportFormats, "AGENT_RUNTIME_RESEARCH_PROCESSING_EXPORT_FORMATS")
	platform.ApplyBoolEnv(&settings.SignalProcessing.Enabled, "AGENT_RUNTIME_SIGNAL_PROCESSING_ENABLED")
	platform.ApplyIntEnv(&settings.SignalProcessing.QueueSize, "AGENT_RUNTIME_SIGNAL_PROCESSING_QUEUE_SIZE")
	platform.ApplyIntEnv(&settings.SignalProcessing.CronIntervalSeconds, "AGENT_RUNTIME_SIGNAL_PROCESSING_CRON_SECONDS")
	platform.ApplyIntEnv(&settings.SignalProcessing.DefaultTTLSeconds, "AGENT_RUNTIME_SIGNAL_PROCESSING_DEFAULT_TTL_SECONDS")
	platform.ApplyIntEnv(&settings.SignalProcessing.MaxStates, "AGENT_RUNTIME_SIGNAL_PROCESSING_MAX_STATES")
	platform.ApplyStringEnv(&settings.SignalProcessing.ProtoLogCompression, "AGENT_RUNTIME_SIGNAL_PROCESSING_PROTO_LOG_COMPRESSION")
	platform.ApplyBoolEnv(&settings.DomainForwardProxy.Enabled, "AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED")
	platform.ApplyIntEnv(&settings.DomainForwardProxy.HTTPPort, "AGENT_RUNTIME_DOMAIN_HTTP_PORT")
	platform.ApplyIntEnv(&settings.DomainForwardProxy.HTTPSPort, "AGENT_RUNTIME_DOMAIN_HTTPS_PORT")
	platform.ApplyStringEnv(&settings.DomainForwardProxy.DefaultScheme, "AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME")
	platform.ApplyBoolEnv(&settings.DomainForwardProxy.AllowAnyHost, "AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST")
	platform.ApplyStringEnv(&settings.DomainForwardProxy.DNSResolver, "AGENT_RUNTIME_DOMAIN_DNS_RESOLVER")
	platform.ApplyIntEnv(&settings.DomainForwardProxy.DialTimeoutSeconds, "AGENT_RUNTIME_DOMAIN_DIAL_TIMEOUT_SECONDS")
	platform.ApplyStringEnv(&settings.DomainForwardProxy.CertFile, "AGENT_RUNTIME_DOMAIN_CERT_FILE")
	platform.ApplyStringEnv(&settings.DomainForwardProxy.KeyFile, "AGENT_RUNTIME_DOMAIN_KEY_FILE")
	seedRuntimeMLConfigFromEnv(&settings.MLConfig)
}

func normalizeKernelRiskFeedbackSettings(settings *KernelRiskFeedbackSettings) {
	if settings == nil {
		return
	}
	if settings.MinRiskScore <= 0 {
		settings.MinRiskScore = 85
	}
	if settings.MinRiskScore > 100 {
		settings.MinRiskScore = 100
	}
	if settings.MaxActionsPerMinute <= 0 {
		settings.MaxActionsPerMinute = 30
	}
	if settings.MaxActionsPerMinute > 600 {
		settings.MaxActionsPerMinute = 600
	}
	if settings.Enabled && !settings.EnforceNetwork && !settings.EnforceFileNames && !settings.EnforceExec {
		settings.EnforceNetwork = true
		settings.EnforceFileNames = true
		settings.EnforceExec = true
	}
}

func normalizeLoopDetectionSettings(settings *LoopDetectionSettings) {
	if settings == nil {
		return
	}
	if settings.WindowSeconds <= 0 {
		settings.WindowSeconds = 30
	}
	if settings.WindowSeconds > 3600 {
		settings.WindowSeconds = 3600
	}
	if settings.RepeatThreshold <= 0 {
		settings.RepeatThreshold = 5
	}
	if settings.RepeatThreshold < 2 {
		settings.RepeatThreshold = 2
	}
	if settings.RepeatThreshold > 1000 {
		settings.RepeatThreshold = 1000
	}
	if settings.MaxContexts <= 0 {
		settings.MaxContexts = 512
	}
	if settings.MaxContexts > 20000 {
		settings.MaxContexts = 20000
	}
	if settings.QueueSize <= 0 {
		settings.QueueSize = 2048
	}
	if settings.QueueSize < 128 {
		settings.QueueSize = 128
	}
	if settings.QueueSize > 65536 {
		settings.QueueSize = 65536
	}
}

func normalizeResearchProcessingSettings(settings *ResearchProcessingSettings) {
	if settings == nil {
		return
	}
	if settings.MaxEvents <= 0 {
		settings.MaxEvents = 5000
	}
	if settings.MaxEvents < 100 {
		settings.MaxEvents = 100
	}
	if settings.MaxEvents > 100000 {
		settings.MaxEvents = 100000
	}
	if settings.QueueSize <= 0 {
		settings.QueueSize = 2048
	}
	if settings.QueueSize < 128 {
		settings.QueueSize = 128
	}
	if settings.QueueSize > 65536 {
		settings.QueueSize = 65536
	}
	if settings.TimelineBucketSeconds <= 0 {
		settings.TimelineBucketSeconds = 60
	}
	if settings.TimelineBucketSeconds > 86400 {
		settings.TimelineBucketSeconds = 86400
	}
	if settings.TopK <= 0 {
		settings.TopK = 20
	}
	if settings.TopK > 200 {
		settings.TopK = 200
	}
	if settings.RecentSamples <= 0 {
		settings.RecentSamples = 25
	}
	if settings.RecentSamples > 500 {
		settings.RecentSamples = 500
	}
	if settings.ArtifactRetentionDays <= 0 {
		settings.ArtifactRetentionDays = researchProcessingDefaultArtifactRetentionDays
	}
	if settings.ArtifactRetentionDays > 3650 {
		settings.ArtifactRetentionDays = 3650
	}
	if settings.MaxSessionEvents <= 0 {
		settings.MaxSessionEvents = researchProcessingDefaultMaxSessionEvents
	}
	if settings.MaxSessionEvents < 100 {
		settings.MaxSessionEvents = 100
	}
	if settings.MaxSessionEvents > 100000 {
		settings.MaxSessionEvents = 100000
	}
	settings.ExportFormats = normalizeResearchExportFormats(settings.ExportFormats)
}

func seedRuntimeAccessTokenFromEnv(settings *RuntimeSettings) {
	if settings == nil {
		return
	}
	if strings.TrimSpace(settings.AccessToken) != "" {
		return
	}
	if envToken, ok := platform.FirstEnv("AGENT_API_KEY", "AGENT_ACCESS_TOKEN", "AGENT_EBPF_ACCESS_TOKEN"); ok {
		settings.AccessToken = envToken
	}
}

func seedRuntimeMLConfigFromEnv(cfg *MLConfig) {
	if cfg == nil {
		return
	}
	platform.ApplyBoolEnv(&cfg.Enabled, "AGENT_ML_ENABLED")
	platform.ApplyModelTypeEnv((*string)(&cfg.ModelType), "AGENT_ML_MODEL_TYPE")
	platform.ApplyStringEnv(&cfg.ModelPath, "AGENT_ML_MODEL_PATH")
	platform.ApplyBoolEnv(&cfg.AutoTrain, "AGENT_ML_AUTO_TRAIN")
	platform.ApplyStringEnv(&cfg.TrainInterval, "AGENT_ML_TRAIN_INTERVAL")
	platform.ApplyIntEnv(&cfg.MinSamplesForTraining, "AGENT_ML_MIN_SAMPLES_FOR_TRAINING")
	platform.ApplyFloatEnv(&cfg.BlockConfidenceThreshold, "AGENT_ML_BLOCK_CONFIDENCE_THRESHOLD")
	platform.ApplyFloatEnv(&cfg.MlMinConfidence, "AGENT_ML_MIN_CONFIDENCE")
	platform.ApplyFloatEnv(&cfg.LowAnomalyThreshold, "AGENT_ML_LOW_ANOMALY_THRESHOLD")
	platform.ApplyFloatEnv(&cfg.HighAnomalyThreshold, "AGENT_ML_HIGH_ANOMALY_THRESHOLD")
	platform.ApplyBoolEnv(&cfg.ActiveLearningEnabled, "AGENT_ML_ACTIVE_LEARNING_ENABLED")
	platform.ApplyIntEnv(&cfg.FeatureHistorySize, "AGENT_ML_FEATURE_HISTORY_SIZE")
	platform.ApplyIntEnv(&cfg.NumTrees, "AGENT_ML_NUM_TREES")
	platform.ApplyIntEnv(&cfg.MaxDepth, "AGENT_ML_MAX_DEPTH")
	platform.ApplyIntEnv(&cfg.MinSamplesLeaf, "AGENT_ML_MIN_SAMPLES_LEAF")
	platform.ApplyFloatEnv(&cfg.ValidationSplitRatio, "AGENT_ML_VALIDATION_SPLIT_RATIO")
	platform.ApplyBoolEnv(&cfg.BalanceClasses, "AGENT_ML_BALANCE_CLASSES")
	platform.ApplyBoolEnv(&cfg.LlmEnabled, "AGENT_LLM_ENABLED", "LLM_ENABLED")
	platform.ApplyStringEnv(&cfg.LlmBaseURL, "AGENT_LLM_BASE_URL", "LLM_BASE_URL", "OPENAI_BASE_URL")
	platform.ApplyStringEnv(&cfg.LlmAPIKey, "AGENT_LLM_API_KEY", "LLM_API_KEY", "OPENAI_API_KEY")
	platform.ApplyStringEnv(&cfg.LlmModel, "AGENT_LLM_MODEL", "LLM_MODEL", "OPENAI_MODEL")
	platform.ApplyIntEnv(&cfg.LlmTimeoutSeconds, "AGENT_LLM_TIMEOUT_SECONDS", "LLM_TIMEOUT_SECONDS")
	platform.ApplyFloatEnv(&cfg.LlmTemperature, "AGENT_LLM_TEMPERATURE", "LLM_TEMPERATURE")
	platform.ApplyIntEnv(&cfg.LlmMaxTokens, "AGENT_LLM_MAX_TOKENS", "LLM_MAX_TOKENS")
	platform.ApplyStringEnv(&cfg.LlmSystemPrompt, "AGENT_LLM_SYSTEM_PROMPT", "LLM_SYSTEM_PROMPT")
}
