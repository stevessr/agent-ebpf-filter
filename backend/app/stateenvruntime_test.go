package app

import (
	"testing"
)

func TestSeedRuntimeSettingsFromEnvAppliesLLMAndBehavior(t *testing.T) {
	t.Setenv("AGENT_ACCESS_TOKEN", "dev-token")
	t.Setenv("AGENT_RUNTIME_EVENT_STORE_MAX_RECORDS", "345678")
	t.Setenv("AGENT_RUNTIME_EVENT_STORE_MAX_AGE", "240h")
	t.Setenv("AGENT_RUNTIME_SHELL_SESSIONS_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_SYSTEM_RUN_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_TLS_CAPTURE_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MIN_SCORE", "92")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_NETWORK", "true")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_FILE_NAMES", "false")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_EXEC", "true")
	t.Setenv("AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MAX_ACTIONS_PER_MINUTE", "12")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_WINDOW_SECONDS", "45")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_REPEAT_THRESHOLD", "7")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_MAX_CONTEXTS", "900")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_QUEUE_SIZE", "4096")
	t.Setenv("AGENT_RUNTIME_LOOP_DETECTION_EMIT_ALERTS", "true")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_MAX_EVENTS", "7000")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_QUEUE_SIZE", "8192")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_TIMELINE_BUCKET_SECONDS", "30")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_TOP_K", "35")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_RECENT_SAMPLES", "45")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_ARTIFACT_RETENTION_DAYS", "21")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_MAX_SESSION_EVENTS", "60000")
	t.Setenv("AGENT_RUNTIME_RESEARCH_PROCESSING_EXPORT_FORMATS", "jsonl,csv")
	t.Setenv("AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_DOMAIN_HTTP_PORT", "18080")
	t.Setenv("AGENT_RUNTIME_DOMAIN_HTTPS_PORT", "18443")
	t.Setenv("AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME", "http")
	t.Setenv("AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST", "true")
	t.Setenv("AGENT_ML_ENABLED", "false")
	t.Setenv("AGENT_ML_MODEL_TYPE", string(ModelLogisticRegression))
	t.Setenv("AGENT_ML_VALIDATION_SPLIT_RATIO", "0.35")
	t.Setenv("AGENT_LLM_ENABLED", "true")
	t.Setenv("AGENT_LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("AGENT_LLM_API_KEY", "local-key")
	t.Setenv("AGENT_LLM_MODEL", "qwen2.5-coder")
	t.Setenv("AGENT_LLM_TIMEOUT_SECONDS", "12")
	t.Setenv("AGENT_LLM_TEMPERATURE", "0.2")
	t.Setenv("AGENT_LLM_MAX_TOKENS", "777")
	t.Setenv("AGENT_LLM_SYSTEM_PROMPT", "strict json")

	settings := RuntimeSettings{}
	seedRuntimeSettingsFromEnv(&settings)
	if err := normalizeRuntimeSettings(&settings); err != nil {
		t.Fatalf("normalizeRuntimeSettings() error = %v", err)
	}

	if settings.AccessToken != "dev-token" {
		t.Fatalf("AccessToken = %q, want env token", settings.AccessToken)
	}
	if settings.EventStoreMaxRecords != 345678 || settings.EventStoreMaxAge != "240h" {
		t.Fatalf("event store retention env seed mismatch: records=%d age=%q", settings.EventStoreMaxRecords, settings.EventStoreMaxAge)
	}
	if !settings.ShellSessionsEnabled || !settings.SystemRunEnabled || !settings.PolicyManagementEnabled || !settings.TlsCaptureEnabled {
		t.Fatalf("runtime behavior booleans were not seeded: %+v", settings)
	}
	if !settings.OtlpEnabled || settings.OtlpEndpoint != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("OTLP env seed mismatch: enabled=%v endpoint=%q", settings.OtlpEnabled, settings.OtlpEndpoint)
	}
	if !settings.KernelRiskFeedback.Enabled || settings.KernelRiskFeedback.MinRiskScore != 92 || settings.KernelRiskFeedback.MaxActionsPerMinute != 12 {
		t.Fatalf("kernel risk feedback numeric env seed mismatch: %+v", settings.KernelRiskFeedback)
	}
	if !settings.KernelRiskFeedback.EnforceNetwork || settings.KernelRiskFeedback.EnforceFileNames || !settings.KernelRiskFeedback.EnforceExec {
		t.Fatalf("kernel risk feedback scope env seed mismatch: %+v", settings.KernelRiskFeedback)
	}
	if !settings.LoopDetection.Enabled || settings.LoopDetection.WindowSeconds != 45 || settings.LoopDetection.RepeatThreshold != 7 {
		t.Fatalf("loop detection window env seed mismatch: %+v", settings.LoopDetection)
	}
	if settings.LoopDetection.MaxContexts != 900 || settings.LoopDetection.QueueSize != 4096 || !settings.LoopDetection.EmitSemanticAlerts {
		t.Fatalf("loop detection runtime env seed mismatch: %+v", settings.LoopDetection)
	}
	if !settings.ResearchProcessing.Enabled || settings.ResearchProcessing.MaxEvents != 7000 || settings.ResearchProcessing.QueueSize != 8192 {
		t.Fatalf("research processing queue env seed mismatch: %+v", settings.ResearchProcessing)
	}
	if settings.ResearchProcessing.TimelineBucketSeconds != 30 || settings.ResearchProcessing.TopK != 35 || settings.ResearchProcessing.RecentSamples != 45 {
		t.Fatalf("research processing summary env seed mismatch: %+v", settings.ResearchProcessing)
	}
	if settings.ResearchProcessing.ArtifactRetentionDays != 21 || settings.ResearchProcessing.MaxSessionEvents != 60000 || settings.ResearchProcessing.ExportFormats != "jsonl,csv" {
		t.Fatalf("research processing artifact env seed mismatch: %+v", settings.ResearchProcessing)
	}
	if !settings.DomainForwardProxy.Enabled || settings.DomainForwardProxy.HTTPPort != 18080 || settings.DomainForwardProxy.HTTPSPort != 18443 {
		t.Fatalf("domain forward env seed mismatch: %+v", settings.DomainForwardProxy)
	}
	if settings.DomainForwardProxy.DefaultScheme != "http" || !settings.DomainForwardProxy.AllowAnyHost {
		t.Fatalf("domain forward scheme/host env seed mismatch: %+v", settings.DomainForwardProxy)
	}
	if settings.MLConfig.Enabled {
		t.Fatalf("MLConfig.Enabled = true, want AGENT_ML_ENABLED=false to be respected")
	}
	if settings.MLConfig.ModelType != ModelLogisticRegression || settings.MLConfig.ValidationSplitRatio != 0.35 {
		t.Fatalf("ML env seed mismatch: %+v", settings.MLConfig)
	}
	if !settings.MLConfig.LlmEnabled || settings.MLConfig.LlmBaseURL != "http://127.0.0.1:11434/v1" || settings.MLConfig.LlmModel != "qwen2.5-coder" {
		t.Fatalf("LLM env seed mismatch: %+v", settings.MLConfig)
	}
	if settings.MLConfig.LlmAPIKey != "local-key" || settings.MLConfig.LlmTimeoutSeconds != 12 || settings.MLConfig.LlmTemperature != 0.2 || settings.MLConfig.LlmMaxTokens != 777 {
		t.Fatalf("LLM numeric/secret env seed mismatch: %+v", settings.MLConfig)
	}
	if settings.MLConfig.LlmSystemPrompt != "strict json" {
		t.Fatalf("LlmSystemPrompt = %q, want env prompt", settings.MLConfig.LlmSystemPrompt)
	}
}

func TestSeedRuntimeListSettingsFromEnv(t *testing.T) {
	t.Setenv("AGENT_RUNTIME_DISABLED_EVENT_TYPES", "1, 5,9")
	t.Setenv("AGENT_RUNTIME_IGNORED_PATHS", "/proc, /var/cache/apt ,")

	settings := RuntimeSettings{
		DisabledEventTypes: []uint32{1},
		IgnoredPaths:       []string{"/tmp"},
	}
	seedRuntimeListSettingsFromEnv(&settings)

	wantTypes := []uint32{1, 5, 9}
	if len(settings.DisabledEventTypes) != len(wantTypes) {
		t.Fatalf("DisabledEventTypes = %v, want %v", settings.DisabledEventTypes, wantTypes)
	}
	for index, eventType := range wantTypes {
		if settings.DisabledEventTypes[index] != eventType {
			t.Fatalf("DisabledEventTypes = %v, want %v", settings.DisabledEventTypes, wantTypes)
		}
	}

	wantPaths := []string{"/proc", "/var/cache/apt"}
	if len(settings.IgnoredPaths) != len(wantPaths) {
		t.Fatalf("IgnoredPaths = %v, want %v", settings.IgnoredPaths, wantPaths)
	}
	for index, path := range wantPaths {
		if settings.IgnoredPaths[index] != path {
			t.Fatalf("IgnoredPaths = %v, want %v", settings.IgnoredPaths, wantPaths)
		}
	}
}

func TestSeedRuntimeListSettingsFromEnvClearsDisabledEventTypes(t *testing.T) {
	t.Setenv("AGENT_RUNTIME_DISABLED_EVENT_TYPES", "")
	t.Setenv("AGENT_RUNTIME_IGNORED_PATHS", "")

	settings := RuntimeSettings{
		DisabledEventTypes: []uint32{1, 5, 9, 11},
		IgnoredPaths:       []string{"/proc", "/tmp"},
	}
	seedRuntimeListSettingsFromEnv(&settings)

	if len(settings.DisabledEventTypes) != 0 {
		t.Fatalf("DisabledEventTypes = %v, want explicitly empty list to clear defaults", settings.DisabledEventTypes)
	}
	if len(settings.IgnoredPaths) != 0 {
		t.Fatalf("IgnoredPaths = %v, want explicitly empty list to clear defaults", settings.IgnoredPaths)
	}
}

func TestSeedRuntimeListSettingsInvalidEventType(t *testing.T) {
	t.Setenv("AGENT_RUNTIME_DISABLED_EVENT_TYPES", "1,abc")

	settings := RuntimeSettings{DisabledEventTypes: []uint32{7}}
	seedRuntimeListSettingsFromEnv(&settings)

	if len(settings.DisabledEventTypes) != 1 || settings.DisabledEventTypes[0] != 7 {
		t.Fatalf("DisabledEventTypes = %v, want invalid env to keep previous value", settings.DisabledEventTypes)
	}
}

func TestParseStringListTrimsEmptyEntries(t *testing.T) {
	got := parseStringList(" /proc ,,  /var/log/journal ,")
	want := []string{"/proc", "/var/log/journal"}
	if len(got) != len(want) {
		t.Fatalf("parseStringList() = %v, want %v", got, want)
	}
	for index, value := range want {
		if got[index] != value {
			t.Fatalf("parseStringList() = %v, want %v", got, want)
		}
	}
}

func TestNormalizeRuntimeSettingsPreservesZeroAllowRiskBoundary(t *testing.T) {
	settings := RuntimeSettings{MLConfig: MLConfig{
		AllowRiskScoreThreshold:  0,
		BlockConfidenceThreshold: 0.85,
	}}
	if err := normalizeRuntimeSettings(&settings); err != nil {
		t.Fatalf("normalizeRuntimeSettings() error = %v", err)
	}
	if settings.MLConfig.AllowRiskScoreThreshold != 0 {
		t.Fatalf("allow risk threshold = %v, want explicit 0", settings.MLConfig.AllowRiskScoreThreshold)
	}
}
