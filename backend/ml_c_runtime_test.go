package main

import (
	"encoding/json"
	"testing"
	"time"
)

func resetMLCRuntimeCacheForTest() {
	mlCRuntimeMu.Lock()
	defer mlCRuntimeMu.Unlock()
	mlCRuntimeCached = MLCRuntimeStatus{}
	mlCRuntimeAt = time.Time{}
	mlCRuntimeKey = ""
}

func makeDummyLinearModel() *LogisticModel {
	model := &LogisticModel{NumClasses: 4}
	model.Weights = make([][FeatureDim + 1]float64, model.NumClasses)
	for c := 0; c < model.NumClasses; c++ {
		for d := 0; d <= FeatureDim; d++ {
			model.Weights[c][d] = float64((c+1)*(d+3)) / 1000.0
		}
	}
	return model
}

func TestMLCRuntimeStatusForLinearModel(t *testing.T) {
	initMLTest(t, 240)
	resetMLCRuntimeCacheForTest()

	status := buildMLCRuntimeStatus(makeDummyLinearModel(), globalTrainingStore)
	if !status.Available {
		t.Fatal("expected runtime status to be available")
	}
	if !status.CSupported {
		t.Fatalf("expected linear model to have a C runtime benchmark: %+v", status)
	}
	if status.BenchmarkBackend != "c_cpu" {
		t.Fatalf("unexpected benchmark backend: %s", status.BenchmarkBackend)
	}
	if status.ModelType != string(ModelLogisticRegression) {
		t.Fatalf("unexpected model type: %s", status.ModelType)
	}
	if status.SampleCount == 0 {
		t.Fatal("expected sample count to be populated")
	}
	if status.GoMsPerSample <= 0 || status.CMsPerSample <= 0 {
		t.Fatalf("expected positive benchmark numbers, got go=%f c=%f", status.GoMsPerSample, status.CMsPerSample)
	}
	if status.Speedup <= 0 {
		t.Fatalf("expected positive speedup, got %f", status.Speedup)
	}
	if len(status.Backends) < 3 {
		t.Fatalf("expected three backends, got %d", len(status.Backends))
	}
}

func TestBuildMLStatusJSONIncludesCRuntime(t *testing.T) {
	initMLTest(t, 180)
	resetMLCRuntimeCacheForTest()

	oldEngine := mlEngine
	oldLoaded := mlModelLoaded
	oldType := currentModelType
	oldEnabled := mlEnabled
	t.Cleanup(func() {
		mlEngine = oldEngine
		mlModelLoaded = oldLoaded
		currentModelType = oldType
		mlEnabled = oldEnabled
		resetMLCRuntimeCacheForTest()
	})

	mlEngine = makeDummyLinearModel()
	mlModelLoaded = true
	currentModelType = ModelLogisticRegression
	mlEnabled = true

	var payload struct {
		ModelType string           `json:"modelType"`
		CRuntime  MLCRuntimeStatus `json:"cRuntime"`
		CUDA      bool             `json:"cudaAvailable"`
		Extra     map[string]any   `json:"-"`
	}
	if err := json.Unmarshal(buildMLStatusJSON(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if payload.ModelType != string(ModelLogisticRegression) {
		t.Fatalf("unexpected payload model type: %s", payload.ModelType)
	}
	if !payload.CRuntime.Available || !payload.CRuntime.CSupported {
		t.Fatalf("expected cRuntime to be present and benchmarkable: %+v", payload.CRuntime)
	}
	if len(payload.CRuntime.Backends) < 3 {
		t.Fatalf("expected backend list in payload, got %+v", payload.CRuntime.Backends)
	}
	if payload.CRuntime.ModelType != string(ModelLogisticRegression) {
		t.Fatalf("unexpected cRuntime model type: %s", payload.CRuntime.ModelType)
	}
}

func TestBuildMLStatusJSONUsesCurrentRuntimeSettings(t *testing.T) {
	initMLTest(t, 120)
	resetMLCRuntimeCacheForTest()

	oldStore := runtimeSettingsStore
	oldEngine := mlEngine
	oldLoaded := mlModelLoaded
	oldType := currentModelType
	oldConfig := mlConfig
	oldEnabled := mlEnabled
	t.Cleanup(func() {
		runtimeSettingsStore = oldStore
		mlEngine = oldEngine
		mlModelLoaded = oldLoaded
		currentModelType = oldType
		mlConfig = oldConfig
		mlEnabled = oldEnabled
		resetMLCRuntimeCacheForTest()
	})

	runtimeCfg := MLConfig{
		Enabled:              true,
		ModelType:            ModelSVM,
		ModelPath:            "/runtime/model.bin",
		ValidationSplitRatio: 0.37,
		NumTrees:             17,
		MaxDepth:             6,
		MinSamplesLeaf:       2,
		LlmEnabled:           true,
		LlmBaseURL:           "https://runtime.example",
		LlmAPIKey:            "runtime-secret",
		LlmModel:             "runtime-llm",
		LlmTimeoutSeconds:    91,
		LlmTemperature:       0.42,
		LlmMaxTokens:         777,
		LlmSystemPrompt:      "runtime prompt",
	}
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{MLConfig: runtimeCfg}}

	// Leave the globals in a conflicting state to ensure the status payload
	// is sourced from the runtime snapshot rather than stale package state.
	mlConfig = MLConfig{
		Enabled:              true,
		ModelType:            ModelRandomForest,
		ModelPath:            "/stale/model.bin",
		ValidationSplitRatio: 0.20,
		NumTrees:             31,
		MaxDepth:             8,
		MinSamplesLeaf:       5,
		LlmEnabled:           false,
		LlmBaseURL:           "https://stale.example",
		LlmModel:             "stale-llm",
		LlmTimeoutSeconds:    11,
		LlmTemperature:       0.99,
		LlmMaxTokens:         12,
		LlmSystemPrompt:      "stale prompt",
	}
	mlEnabled = true
	mlEngine = nil
	mlModelLoaded = false
	currentModelType = ModelRandomForest

	if got := currentMLConfig(); got != runtimeCfg {
		t.Fatalf("currentMLConfig() = %+v, want %+v", got, runtimeCfg)
	}

	var payload struct {
		ModelPath string `json:"modelPath"`
		MLConfig  struct {
			ModelType            string  `json:"modelType"`
			ValidationSplitRatio float64 `json:"validationSplitRatio"`
			LLMEnabled           bool    `json:"llmEnabled"`
			LLMBaseURL           string  `json:"llmBaseUrl"`
			LLMApiKeyConfigured  bool    `json:"llmApiKeyConfigured"`
			LLMModel             string  `json:"llmModel"`
			LLMTimeoutSeconds    int     `json:"llmTimeoutSeconds"`
			LLMTemperature       float64 `json:"llmTemperature"`
			LLMMaxTokens         int     `json:"llmMaxTokens"`
			LLMSystemPrompt      string  `json:"llmSystemPrompt"`
		} `json:"mlConfig"`
		HyperParams struct {
			NumTrees       int `json:"numTrees"`
			MaxDepth       int `json:"maxDepth"`
			MinSamplesLeaf int `json:"minSamplesLeaf"`
		} `json:"hyperParams"`
	}
	if err := json.Unmarshal(buildMLStatusJSON(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if payload.ModelPath != runtimeCfg.ModelPath {
		t.Fatalf("payload.ModelPath = %q, want %q", payload.ModelPath, runtimeCfg.ModelPath)
	}
	if payload.MLConfig.ModelType != string(runtimeCfg.ModelType) {
		t.Fatalf("payload.MLConfig.ModelType = %q, want %q", payload.MLConfig.ModelType, runtimeCfg.ModelType)
	}
	if payload.MLConfig.ValidationSplitRatio != runtimeCfg.ValidationSplitRatio {
		t.Fatalf("payload.MLConfig.ValidationSplitRatio = %f, want %f", payload.MLConfig.ValidationSplitRatio, runtimeCfg.ValidationSplitRatio)
	}
	if payload.MLConfig.LLMEnabled != runtimeCfg.LlmEnabled {
		t.Fatalf("payload.MLConfig.LLMEnabled = %v, want %v", payload.MLConfig.LLMEnabled, runtimeCfg.LlmEnabled)
	}
	if payload.MLConfig.LLMBaseURL != runtimeCfg.LlmBaseURL {
		t.Fatalf("payload.MLConfig.LLMBaseURL = %q, want %q", payload.MLConfig.LLMBaseURL, runtimeCfg.LlmBaseURL)
	}
	if !payload.MLConfig.LLMApiKeyConfigured {
		t.Fatal("expected LLM API key to be reported as configured")
	}
	if payload.MLConfig.LLMModel != runtimeCfg.LlmModel {
		t.Fatalf("payload.MLConfig.LLMModel = %q, want %q", payload.MLConfig.LLMModel, runtimeCfg.LlmModel)
	}
	if payload.MLConfig.LLMTimeoutSeconds != runtimeCfg.LlmTimeoutSeconds {
		t.Fatalf("payload.MLConfig.LLMTimeoutSeconds = %d, want %d", payload.MLConfig.LLMTimeoutSeconds, runtimeCfg.LlmTimeoutSeconds)
	}
	if payload.MLConfig.LLMTemperature != runtimeCfg.LlmTemperature {
		t.Fatalf("payload.MLConfig.LLMTemperature = %f, want %f", payload.MLConfig.LLMTemperature, runtimeCfg.LlmTemperature)
	}
	if payload.MLConfig.LLMMaxTokens != runtimeCfg.LlmMaxTokens {
		t.Fatalf("payload.MLConfig.LLMMaxTokens = %d, want %d", payload.MLConfig.LLMMaxTokens, runtimeCfg.LlmMaxTokens)
	}
	if payload.MLConfig.LLMSystemPrompt != runtimeCfg.LlmSystemPrompt {
		t.Fatalf("payload.MLConfig.LLMSystemPrompt = %q, want %q", payload.MLConfig.LLMSystemPrompt, runtimeCfg.LlmSystemPrompt)
	}
	if payload.HyperParams.NumTrees != runtimeCfg.NumTrees {
		t.Fatalf("payload.HyperParams.NumTrees = %d, want %d", payload.HyperParams.NumTrees, runtimeCfg.NumTrees)
	}
	if payload.HyperParams.MaxDepth != runtimeCfg.MaxDepth {
		t.Fatalf("payload.HyperParams.MaxDepth = %d, want %d", payload.HyperParams.MaxDepth, runtimeCfg.MaxDepth)
	}
	if payload.HyperParams.MinSamplesLeaf != runtimeCfg.MinSamplesLeaf {
		t.Fatalf("payload.HyperParams.MinSamplesLeaf = %d, want %d", payload.HyperParams.MinSamplesLeaf, runtimeCfg.MinSamplesLeaf)
	}
}

func TestMLCRuntimeStatusWithoutSamples(t *testing.T) {
	InitTrainingStore(32)
	if globalTrainingStore != nil {
		globalTrainingStore.Clear()
	}
	resetMLCRuntimeCacheForTest()

	status := buildMLCRuntimeStatus(nil, globalTrainingStore)
	if !status.Available {
		t.Fatal("runtime status should still be available without samples")
	}
	if status.CSupported {
		t.Fatalf("expected no C benchmark support without a model: %+v", status)
	}
	if status.SampleCount != 0 {
		t.Fatalf("expected zero sample count, got %d", status.SampleCount)
	}
	if status.Note == "" {
		t.Fatal("expected note for unsupported model path")
	}
}
