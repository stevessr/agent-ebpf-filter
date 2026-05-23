package main

import "testing"

func TestSeedRuntimeSettingsFromEnvAppliesLLMAndBehavior(t *testing.T) {
	t.Setenv("AGENT_ACCESS_TOKEN", "dev-token")
	t.Setenv("AGENT_RUNTIME_SHELL_SESSIONS_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_SYSTEM_RUN_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_TLS_CAPTURE_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
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
	if !settings.ShellSessionsEnabled || !settings.SystemRunEnabled || !settings.PolicyManagementEnabled || !settings.TlsCaptureEnabled {
		t.Fatalf("runtime behavior booleans were not seeded: %+v", settings)
	}
	if !settings.OtlpEnabled || settings.OtlpEndpoint != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("OTLP env seed mismatch: enabled=%v endpoint=%q", settings.OtlpEnabled, settings.OtlpEndpoint)
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
