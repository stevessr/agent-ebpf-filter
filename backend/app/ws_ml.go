package app

import (
	"agent-ebpf-filter/cuda"
	"encoding/json"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section ws_ml.go ----

// buildMLStatusJSON builds the complete ML status payload as JSON bytes.
// Shared by the HTTP handler and the WebSocket handler.
func buildMLStatusJSON() []byte {
	cfg := currentMLConfig()
	status := mlStatus()
	logs := globalTrainer.GetLogs(100)
	trainAccuracy, validationAccuracy, validationRatio, trainSamples, validationSamples := globalTrainer.SplitMetrics()
	autoTuneState := globalAutoTuneState.snapshot()
	trainingReadiness := buildMLTrainingReadiness(globalTrainingStore, cfg)

	logItems := make([]map[string]string, len(logs))
	for i, entry := range logs {
		logItems[i] = map[string]string{"time": entry.Timestamp.Format("15:04:05"), "message": entry.Message}
	}

	cudaAvailable := cuda.IsAvailable()
	cudaInfo := ""
	if cudaAvailable {
		cudaInfo = cuda.DeviceInfo()
	}

	payload := map[string]interface{}{
		"cudaAvailable":        cudaAvailable,
		"cudaInfo":             cudaInfo,
		"cudaMemUsedMB":        cuda.MemUsedMB(),
		"cudaMemTotalMB":       cuda.MemTotalMB(),
		"cRuntime":             buildMLCRuntimeStatus(mlEngine, globalTrainingStore),
		"modelType":            string(currentModelType),
		"availableModelTypes":  AllModelTypeStrings(),
		"builtinModels":        BuiltinModelCatalog(),
		"modelLoaded":          status.GetModelLoaded(),
		"numTrees":             status.GetNumTrees(),
		"numSamples":           status.GetNumSamples(),
		"numLabeledSamples":    status.GetNumLabeledSamples(),
		"lastTrained":          status.GetLastTrained(),
		"testAccuracy":         status.GetTestAccuracy(),
		"modelPath":            status.GetModelPath(),
		"trainingInProgress":   status.GetTrainingInProgress(),
		"trainingProgress":     status.GetTrainingProgress(),
		"mlEnabled":            mlEnabled,
		"trainAccuracy":        trainAccuracy,
		"validationAccuracy":   validationAccuracy,
		"trainSamples":         trainSamples,
		"validationSamples":    validationSamples,
		"validationSplitRatio": validationRatio,
		"trainingReadiness":    trainingReadiness,
		"llmReview":            globalTrainer.LastLLMReview(),
		"autoTuneJobId":        autoTuneState.JobID,
		"autoTuneMode":         autoTuneState.Mode,
		"autoTuneInProgress":   autoTuneState.Running,
		"autoTuneProgress":     autoTuneState.Progress,
		"autoTuneCompleted":    autoTuneState.Completed,
		"autoTuneTotal":        autoTuneState.Total,
		"autoTuneMessage":      autoTuneState.Message,
		"autoTuneError":        autoTuneState.Error,
		"autoTuneResult":       autoTuneState.Result,
		"modelTuneResult":      autoTuneState.ModelResult,
		"autoTuneRuntime":      mlAutoTuneTasks.Stats(),
		"mlConfig": map[string]interface{}{
			"modelType":            string(cfg.ModelType),
			"ensembleVoting":       cfg.EnsembleVoting,
			"validationSplitRatio": cfg.ValidationSplitRatio,
			"llmEnabled":           cfg.LlmEnabled,
			"llmBaseUrl":           cfg.LlmBaseURL,
			"llmApiKeyConfigured":  strings.TrimSpace(cfg.LlmAPIKey) != "",
			"llmModel":             cfg.LlmModel,
			"llmTimeoutSeconds":    cfg.LlmTimeoutSeconds,
			"llmTemperature":       cfg.LlmTemperature,
			"llmMaxTokens":         cfg.LlmMaxTokens,
			"llmSystemPrompt":      cfg.LlmSystemPrompt,
		},
		"trainingLogs": logItems,
		"hyperParams": map[string]interface{}{
			"numTrees":       cfg.NumTrees,
			"maxDepth":       cfg.MaxDepth,
			"minSamplesLeaf": cfg.MinSamplesLeaf,
		},
	}

	data, _ := json.Marshal(payload)
	return data
}

// serveMLStatusWS moved to app/handlers/ml_ws.go
