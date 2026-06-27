package app

import (
	"time"

	"github.com/gin-gonic/gin"
)

// ── ML handler bridge functions ─────────────────────────────────────
// These wire the handlers/ ML closures to app-level globals.

func mlGetLogsResponse() gin.H {
	logs := globalTrainer.GetLogs(200)
	items := make([]gin.H, len(logs))
	for i, entry := range logs {
		items[i] = gin.H{"time": entry.Timestamp.Format("15:04:05"), "message": entry.Message}
	}
	return gin.H{"logs": items, "total": globalTrainer.logTotal}
}

func mlGetHistoryResponse() gin.H {
	return gin.H{"history": globalTrainer.GetHistory()}
}

func mlTrain(numTrees, maxDepth, minLeaf int) gin.H {
	if !mlEnabled {
		return gin.H{"error": "ML engine is not enabled on this node"}
	}

	cfg := currentMLConfig()
	if numTrees > 0 {
		cfg.NumTrees = numTrees
	}
	if maxDepth > 0 {
		cfg.MaxDepth = maxDepth
	}
	if minLeaf > 0 {
		cfg.MinSamplesLeaf = minLeaf
	}

	model, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result.Error != "" {
		return gin.H{"error": result.Error}
	}
	mlEngine = model
	mlModelLoaded = true

	modelPath := mlConfig.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}
	if err := model.Serialize(modelPath); err != nil {
		return gin.H{"error": "model trained but failed to save: " + err.Error()}
	}

	return gin.H{
		"status":              "ok",
		"accuracy":            result.Accuracy,
		"trainAccuracy":       result.TrainAccuracy,
		"validationAccuracy":  result.ValidationAccuracy,
		"numTrees":            result.NumTrees,
		"numSamples":          result.NumSamples,
		"trainSamples":        result.TrainSamples,
		"validationSamples":   result.ValidationSamples,
		"llmScoredSamples":    result.LLMScoredSamples,
		"llmAverageRiskScore": result.LLMAverageRiskScore,
		"llmAgreement":        result.LLMAgreement,
	}
}

func mlFeedbackResult(comm, userAction string) gin.H {
	if globalTrainingStore == nil {
		return gin.H{"error": "ML training store not initialized"}
	}
	matched := globalTrainingStore.ApplyFeedback(comm, userAction)
	return gin.H{"status": "ok", "matched": matched}
}

func mlSamplesResponse() gin.H {
	if globalTrainingStore == nil {
		return gin.H{"error": "ML training store not initialized"}
	}
	items := globalTrainingStore.AllSamplesWithIndex()
	type sampleJSON struct {
		Index        int      `json:"index"`
		CommandLine  string   `json:"commandLine"`
		Comm         string   `json:"comm"`
		Args         []string `json:"args"`
		Label        string   `json:"label"`
		Category     string   `json:"category"`
		AnomalyScore float64  `json:"anomalyScore"`
		Timestamp    string   `json:"timestamp"`
		UserLabel    string   `json:"userLabel"`
	}
	out := make([]sampleJSON, 0, len(items))
	for _, it := range items {
		lbl := "-"
		if it.Sample.Label >= 0 {
			lbl = actionLabel[it.Sample.Label]
		}
		out = append(out, sampleJSON{
			Index:        it.Index,
			CommandLine:  trainingSampleCommandLine(it.Sample),
			Comm:         it.Sample.Comm,
			Args:         it.Sample.Args,
			Label:        lbl,
			Category:     it.Sample.Category,
			AnomalyScore: it.Sample.AnomalyScore,
			Timestamp:    it.Sample.Timestamp.Format(time.RFC3339),
			UserLabel:    it.Sample.UserLabel,
		})
	}
	return gin.H{"samples": out, "total": len(out)}
}

func mlSampleLabelResult(index int, label string) gin.H {
	if globalTrainingStore == nil {
		return gin.H{"error": "ML training store not initialized"}
	}
	if !globalTrainingStore.LabelSample(index, label) {
		return gin.H{"error": "invalid index or sample not found"}
	}
	return gin.H{"status": "ok"}
}

func mlRemoveSampleResult(index int) gin.H {
	if globalTrainingStore == nil {
		return gin.H{"error": "ML training store not initialized"}
	}
	if !globalTrainingStore.RemoveSample(index) {
		return gin.H{"error": "invalid index or sample not found"}
	}
	return gin.H{"status": "ok"}
}

func mlSampleAnomalyResult(index int, score float64) gin.H {
	if globalTrainingStore == nil {
		return gin.H{"error": "ML training store not initialized"}
	}
	if score < 0 || score > 1 {
		return gin.H{"error": "anomaly score must be between 0 and 1"}
	}
	if !globalTrainingStore.UpdateSampleAnomaly(index, score) {
		return gin.H{"error": "invalid index or sample not found"}
	}
	return gin.H{"status": "ok"}
}

func mlAddSample(cmdLine, comm string, args []string, label string) gin.H {
	return handleMLAddSampleImpl(cmdLine, comm, args, label)
}

func mlImportResult() gin.H {
	count := handleMLImportExistingImpl()
	return gin.H{"status": "ok", "imported": count}
}

func mlTuneResult() gin.H {
	result := handleMLTuneImpl()
	if result.Error != "" {
		return gin.H{"error": result.Error}
	}
	return gin.H{"status": "ok", "accuracy": result.Accuracy}
}

func mlTuneModelsResult(models []string) gin.H {
	return handleMLTuneModelsImpl(models)
}

func mlLLMScoreResult(cmdLine, comm string, args []string) gin.H {
	return handleMLLLMScoreImpl(cmdLine, comm, args)
}

func mlLLMBatchScoreResult(samples []gin.H) gin.H {
	return handleMLLLMBatchScoreImpl(samples)
}

func mlLlmProductionDatasetPullResult() gin.H {
	return handleMLLLMProductionDatasetPullImpl()
}

func mlClassicDatasetGetResult(name string) gin.H {
	return handleMLClassicDatasetGetImpl(name)
}

func mlClassicDatasetPreviewResult(name string) gin.H {
	return handleMLClassicDatasetPreviewImpl(name)
}

func mlDatasetPullResult(url string) gin.H {
	return handleMLDatasetPullImpl(url)
}

func mlDatasetImportResult(name string) gin.H {
	return handleMLDatasetImportImpl(name)
}

func mlDatasetExportResult() gin.H {
	return handleMLDatasetExportImpl()
}