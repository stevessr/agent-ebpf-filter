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
