package app

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section stateenvruntime.go ----

func runtimeSettingsDir() string {
	return filepath.Join(getRealHomeDir(), ".config", "agent-ebpf-filter")
}

func runtimeSettingsPath() string {
	return filepath.Join(runtimeSettingsDir(), "runtime.json")
}

func defaultEventLogPath() string {
	return filepath.Join(runtimeSettingsDir(), "events.jsonl")
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
	normalizeKernelRiskFeedbackSettings(&settings.KernelRiskFeedback)
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
	applyRuntimeBoolEnv(&settings.KernelRiskFeedback.Enabled, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENABLED")
	applyRuntimeFloatEnv(&settings.KernelRiskFeedback.MinRiskScore, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MIN_SCORE")
	applyRuntimeBoolEnv(&settings.KernelRiskFeedback.EnforceNetwork, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_NETWORK")
	applyRuntimeBoolEnv(&settings.KernelRiskFeedback.EnforceFileNames, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_FILE_NAMES")
	applyRuntimeBoolEnv(&settings.KernelRiskFeedback.EnforceExec, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_EXEC")
	applyRuntimeIntEnv(&settings.KernelRiskFeedback.MaxActionsPerMinute, "AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MAX_ACTIONS_PER_MINUTE")
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
