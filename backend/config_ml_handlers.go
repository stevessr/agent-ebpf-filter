package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pb "agent-ebpf-filter/pb"
)

// ── ML classification handlers ──

func handleMLStatusGet(c *gin.Context) {
	status := mlStatus()
	var payload gin.H
	if err := json.Unmarshal(buildMLStatusJSON(), &payload); err != nil {
		c.JSON(500, gin.H{"error": "Failed to build ML status"})
		return
	}
	writeProtoOrJSON(c, 200, status, payload)
}

// handleMLLogsGet returns dedicated training log entries
func handleMLLogsGet(c *gin.Context) {
	logs := globalTrainer.GetLogs(200)
	items := make([]gin.H, len(logs))
	for i, entry := range logs {
		items[i] = gin.H{"time": entry.Timestamp.Format("15:04:05"), "message": entry.Message}
	}
	c.JSON(200, gin.H{"logs": items, "total": globalTrainer.logTotal})
}

func handleMLTrainCancelPost(c *gin.Context) {
	if !globalTrainer.isRunning {
		c.JSON(200, gin.H{"message": "no training in progress"})
		return
	}
	globalTrainer.CancelTraining()
	c.JSON(200, gin.H{"message": "cancellation requested"})
}

// handleMLHistoryGet returns training history for visualization
func handleMLHistoryGet(c *gin.Context) {
	history := globalTrainer.GetHistory()
	c.JSON(200, gin.H{"history": history})
}

func handleMLTrainPost(c *gin.Context) {
	if !mlEnabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}

	// Accept optional hyperparameter overrides
	var req struct {
		NumTrees       int `json:"numTrees"`
		MaxDepth       int `json:"maxDepth"`
		MinSamplesLeaf int `json:"minSamplesLeaf"`
	}
	_ = c.ShouldBindJSON(&req)

	numTrees := mlConfig.NumTrees
	if req.NumTrees > 0 {
		numTrees = req.NumTrees
	}
	maxDepth := mlConfig.MaxDepth
	if req.MaxDepth > 0 {
		maxDepth = req.MaxDepth
	}
	minLeaf := mlConfig.MinSamplesLeaf
	if req.MinSamplesLeaf > 0 {
		minLeaf = req.MinSamplesLeaf
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
		c.JSON(400, gin.H{"error": result.Error})
		return
	}
	mlEngine = model
	mlModelLoaded = true

	modelPath := mlConfig.ModelPath
	if modelPath == "" {
		modelPath = defaultMLModelPath()
	}
	if err := model.Serialize(modelPath); err != nil {
		c.JSON(500, gin.H{"error": "model trained but failed to save: " + err.Error()})
		return
	}

	c.JSON(200, gin.H{
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
	})
}

func handleMLFeedbackPost(c *gin.Context) {
	var req struct {
		Comm       string `json:"comm"`
		UserAction string `json:"userAction"` // "accepted" or "rejected"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}
	matched := globalTrainingStore.ApplyFeedback(req.Comm, req.UserAction)
	c.JSON(200, gin.H{"status": "ok", "matched": matched})
}

// handleMLSamplesGet returns all training samples for the data browser
func handleMLSamplesGet(c *gin.Context) {
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
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
	c.JSON(200, gin.H{"samples": out, "total": len(out)})
}

// handleMLSampleLabelPut labels a specific sample by its ring index
func handleMLSampleLabelPut(c *gin.Context) {
	var req struct {
		Index int    `json:"index"`
		Label string `json:"label"` // "BLOCK", "ALERT", "ALLOW"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}
	if !globalTrainingStore.LabelSample(req.Index, req.Label) {
		c.JSON(400, gin.H{"error": "invalid index or sample not found"})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// handleMLSampleDelete removes a training sample by index
func handleMLSampleDelete(c *gin.Context) {
	indexStr := c.Param("index")
	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		c.JSON(400, gin.H{"error": "invalid index"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}
	if !globalTrainingStore.RemoveSample(index) {
		c.JSON(400, gin.H{"error": "invalid index or sample not found"})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// handleMLSampleAnomalyPut updates the anomaly score of a sample
func handleMLSampleAnomalyPut(c *gin.Context) {
	var req struct {
		Index        int     `json:"index"`
		AnomalyScore float64 `json:"anomalyScore"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}
	if req.AnomalyScore < 0 || req.AnomalyScore > 1 {
		c.JSON(400, gin.H{"error": "anomaly score must be between 0 and 1"})
		return
	}
	if !globalTrainingStore.UpdateSampleAnomaly(req.Index, req.AnomalyScore) {
		c.JSON(400, gin.H{"error": "invalid index or sample not found"})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// handleMLSamplesPost adds a manually labeled training sample
func handleMLSamplesPost(c *gin.Context) {
	var req struct {
		CommandLine string   `json:"commandLine"`
		Comm        string   `json:"comm"`
		Args        []string `json:"args"`
		Label       string   `json:"label"` // "BLOCK", "ALERT", "ALLOW"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	commandLine := strings.TrimSpace(req.CommandLine)
	comm := strings.TrimSpace(req.Comm)
	args := req.Args
	if commandLine != "" {
		comm, args = normalizeCommandInput(commandLine, comm, req.Args)
		if comm == "" {
			c.JSON(400, gin.H{"error": "commandLine is required"})
			return
		}
	} else if comm == "" {
		c.JSON(400, gin.H{"error": "comm is required"})
		return
	}
	if commandLine == "" {
		commandLine = joinCommandLine(comm, args)
	}
	// Build feature vector and classification for the sample
	classification := ClassifyBehavior(comm, args)
	_, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
	anomalyScore := globalEmbedder.ComputeAnomalyScore(emb)
	features := globalFeatureExtractor.Extract(comm, args, "", 0)

	labelInt := actionFromLabel(req.Label)

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

	// Also add to history buffer and cluster
	globalEmbedder.AddToCluster(emb)
	globalFeatureExtractor.AddHistory(comm, classification.PrimaryCategory, req.Label, anomalyScore)

	total, labeled := globalTrainingStore.Status()
	c.JSON(200, gin.H{
		"status":         "ok",
		"totalSamples":   total,
		"labeledSamples": labeled,
	})
}

// handleMLBacktestPost runs a point-in-time risk assessment on a given command
func handleMLBacktestPost(c *gin.Context) {
	handleMLAssessPost(c)
}

// computeRiskScore combines classification, anomaly, and ML into a 0-100 risk score
func computeRiskScore(classification *pb.BehaviorClassification, anomalyScore float64, mlPrediction Prediction, netAudit NetworkAuditResult, llmAssessment *llmAssessment) float64 {
	score := 0.0

	// Category-based contribution (0-35)
	if classification != nil {
		switch classification.PrimaryCategory {
		case "SENSITIVE":
			score += 35
		case "FILE_DELETE", "PROCESS_KILL":
			score += 28
		case "FILE_PERMISSION", "NETWORK":
			score += 18
		case "PROCESS_EXEC", "FILE_WRITE":
			score += 13
		case "CONTAINER", "DATABASE":
			score += 8
		case "PACKAGE_MANAGER", "COMPRESSION":
			score += 5
		}

		if classification.Confidence == "high" {
			score += 10
		} else if classification.Confidence == "medium" {
			score += 5
		}
	}

	// Anomaly contribution (0-30)
	score += anomalyScore * 30

	// ML prediction contribution (0-25)
	if mlPrediction.Confidence >= 0.60 {
		switch mlPrediction.Action {
		case 1: // BLOCK
			score += mlPrediction.Confidence * 25
		case 3: // ALERT
			score += mlPrediction.Confidence * 15
		case 2: // REWRITE
			score += mlPrediction.Confidence * 8
		}
	}

	// Network audit contribution (0-20)
	switch netAudit.RiskLevel {
	case "CRITICAL":
		score += 20
	case "HIGH":
		score += 15
	case "MEDIUM":
		score += 10
	case "LOW":
		score += 5
	}

	// LLM contribution (0-20)
	if llmAssessment != nil && strings.TrimSpace(llmAssessment.Error) == "" {
		score += clampFloat64(llmAssessment.RiskScore*0.18, 0, 20)
		if llmAssessment.Confidence > 0 {
			score += clampFloat64(llmAssessment.Confidence*6, 0, 6)
		}
		switch llmAssessment.RecommendedAction {
		case "BLOCK":
			score += 8
		case "ALERT":
			score += 5
		case "REWRITE":
			score += 3
		}
	}

	if score > 100 {
		score = 100
	}
	return math.Round(score)
}

func riskLevel(score float64) string {
	switch {
	case score >= 80:
		return "CRITICAL"
	case score >= 60:
		return "HIGH"
	case score >= 40:
		return "MEDIUM"
	case score >= 20:
		return "LOW"
	default:
		return "SAFE"
	}
}
