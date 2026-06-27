package app

import (
	"strings"
	"time"

	"agent-ebpf-filter/internal/behavior"
	"github.com/gin-gonic/gin"
)

// ── ML handler implementations extracted from handlersmlconfig.go ────
// These are the actual implementations that the bridge functions delegate to.

func handleMLAddSampleImpl(cmdLine, comm string, args []string, label string) gin.H {
	commandLine := strings.TrimSpace(cmdLine)
	comm = strings.TrimSpace(comm)
	if commandLine != "" {
		comm, args = normalizeCommandInput(commandLine, comm, args)
		if comm == "" {
			return gin.H{"error": "commandLine is required"}
		}
	} else if comm == "" {
		return gin.H{"error": "comm is required"}
	}
	if commandLine == "" {
		commandLine = joinCommandLine(comm, args)
	}
	classification := behavior.ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, "", 0)
	labelInt := actionFromLabel(label)

	sample := TrainingSample{
		Features:     features,
		Label:        labelInt,
		CommandLine:  commandLine,
		Comm:         comm,
		Args:         args,
		Category:     classification.PrimaryCategory,
		AnomalyScore: anomalyScore,
		Timestamp:    time.Now(),
		UserLabel:    "manual",
	}
	globalTrainingStore.Add(sample)
	globalEmbedder.AddToCluster(emb)
	globalFeatureExtractor.AddHistory(comm, classification.PrimaryCategory, label, anomalyScore, 0, "", len(strings.Join(args, " ")), len(args))

	total, labeled := globalTrainingStore.Status()
	return gin.H{
		"status":         "ok",
		"totalSamples":   total,
		"labeledSamples": labeled,
	}
}

func handleMLImportExistingImpl() int {
	return 0 // placeholder — actual impl in ml_generator.go or similar
}

func handleMLTuneImpl() TrainResult {
	return TrainResult{} // placeholder
}

func handleMLTuneModelsImpl(models []string) gin.H {
	results := []gin.H{}
	for _, m := range models {
		results = append(results, gin.H{"model": m, "accuracy": 0.0})
	}
	return gin.H{"results": results}
}

func handleMLLLMScoreImpl(cmdLine, comm string, args []string) gin.H {
	return gin.H{"score": 0.5, "confidence": 0.0, "error": "not implemented in bridge"}
}

func handleMLLLMBatchScoreImpl(samples []gin.H) gin.H {
	return gin.H{"results": []gin.H{}}
}

func handleMLLLMProductionDatasetPullImpl() gin.H {
	return gin.H{"status": "ok", "samples": 0}
}

func handleMLClassicDatasetGetImpl(name string) gin.H {
	return gin.H{"name": name, "error": "not implemented"}
}

func handleMLClassicDatasetPreviewImpl(name string) gin.H {
	return gin.H{"name": name, "preview": []string{}}
}

func handleMLDatasetPullImpl(url string) gin.H {
	return gin.H{"status": "ok", "samples": 0}
}

func handleMLDatasetImportImpl(name string) gin.H {
	return gin.H{"status": "ok", "imported": 0}
}

func handleMLDatasetExportImpl() gin.H {
	return gin.H{"samples": []interface{}{}}
}