package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlersmlconfig.go ----

func HandleMLStatusGet(c *gin.Context) {
	status := Deps.MLStatus()
	var payload gin.H
	if err := json.Unmarshal(Deps.BuildMLStatusJSON(), &payload); err != nil {
		c.JSON(500, gin.H{"error": "Failed to build ML status"})
		return
	}
	Deps.WriteProtoOrJSON(c, 200, status, payload)
}

func HandleMLLogsGet(c *gin.Context) {
	c.JSON(200, Deps.MLGetLogsResponse())
}

func HandleMLTrainCancelPost(c *gin.Context) {
	if !Deps.MLIsRunning() {
		c.JSON(200, gin.H{"message": "no training in progress"})
		return
	}
	Deps.MLCancelTraining()
	c.JSON(200, gin.H{"message": "cancellation requested"})
}

func HandleMLHistoryGet(c *gin.Context) {
	c.JSON(200, Deps.MLGetHistoryResponse())
}

func HandleMLTrainPost(c *gin.Context) {
	if !Deps.MLEnabled() {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}

	var req struct {
		NumTrees       int `json:"numTrees"`
		MaxDepth       int `json:"maxDepth"`
		MinSamplesLeaf int `json:"minSamplesLeaf"`
	}
	_ = c.ShouldBindJSON(&req)

	c.JSON(200, Deps.MLTrain(req.NumTrees, req.MaxDepth, req.MinSamplesLeaf))
}

func HandleMLFeedbackPost(c *gin.Context) {
	var req struct {
		Comm       string `json:"comm"`
		UserAction string `json:"userAction"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLFeedbackResult(req.Comm, req.UserAction))
}

func HandleMLSamplesGet(c *gin.Context) {
	c.JSON(200, Deps.MLSamplesResponse())
}

func HandleMLSampleLabelPut(c *gin.Context) {
	var req struct {
		Index int    `json:"index"`
		Label string `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLSampleLabelResult(req.Index, req.Label))
}

func HandleMLSampleDelete(c *gin.Context) {
	indexStr := c.Param("index")
	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		c.JSON(400, gin.H{"error": "invalid index"})
		return
	}
	c.JSON(200, Deps.MLRemoveSampleResult(index))
}

func HandleMLSampleAnomalyPut(c *gin.Context) {
	var req struct {
		Index        int     `json:"index"`
		AnomalyScore float64 `json:"anomalyScore"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLSampleAnomalyResult(req.Index, req.AnomalyScore))
}

func HandleMLSamplesPost(c *gin.Context) {
	var req struct {
		CommandLine string   `json:"commandLine"`
		Comm        string   `json:"comm"`
		Args        []string `json:"args"`
		Label       string   `json:"label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLAddSample(req.CommandLine, req.Comm, req.Args, req.Label))
}

func HandleMLBacktestPost(c *gin.Context) {
	HandleMLAssessPost(c)
}

func HandleMLAssessPost(c *gin.Context) {
	var req struct {
		Comm string   `json:"comm"`
		Args []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	classification := behavior.ClassifyBehavior(req.Comm, req.Args)
	_, emb := Deps.MLClassifyAndEmbed(req.Comm, req.Args)
	anomalyScore := Deps.MLComputeAnomalyScore(emb)
	prediction := Deps.MLPredict(req.Comm, req.Args)
	netResult := Deps.MLNetworkAudit(req.Comm, req.Args)
	llmAssess := Deps.MLLLMAssessment(req.Comm, req.Args)

	score := computeRiskScore(classification, anomalyScore, prediction, netResult, llmAssess)
	level := riskLevel(score)
	action := "ALLOW"
	if level == "CRITICAL" || level == "HIGH" {
		action = "BLOCK"
	} else if level == "MEDIUM" {
		action = "ALERT"
	}
	c.JSON(200, gin.H{
		"riskScore":      score,
		"riskLevel":      level,
		"recommended":    action,
		"classification": classification,
		"anomalyScore":   anomalyScore,
		"llmAssessment":  llmAssess,
	})
}

func HandleMLExistingCommandsGet(c *gin.Context) {
	c.JSON(200, gin.H{"commands": Deps.MLExistingCommands()})
}

func HandleMLImportExistingPost(c *gin.Context) {
	c.JSON(200, Deps.MLImportResult())
}

func HandleMLTunePost(c *gin.Context) {
	c.JSON(200, Deps.MLTuneResult())
}

func HandleMLTuneModelsPost(c *gin.Context) {
	var req struct {
		Models []string `json:"models"`
	}
	_ = c.ShouldBindJSON(&req)
	c.JSON(200, Deps.MLTuneModelsResult(req.Models))
}

func HandleMLLLMScorePost(c *gin.Context) {
	var req struct {
		CommandLine string `json:"commandLine"`
		Comm        string `json:"comm"`
		Args        []string `json:"args"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLLLMScoreResult(req.CommandLine, req.Comm, req.Args))
}

func HandleMLLLMBatchScorePost(c *gin.Context) {
	var req struct {
		Samples []gin.H `json:"samples"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLLLMBatchScoreResult(req.Samples))
}

func HandleMLLLMProductionDatasetPullPost(c *gin.Context) {
	c.JSON(200, Deps.MLLlmProductionDatasetPullResult())
}

func HandleClassicDatasetsListGet(c *gin.Context) {
	c.JSON(200, Deps.MLClassicDatasetsList())
}

func HandleClassicDatasetGet(c *gin.Context) {
	c.JSON(200, Deps.MLClassicDatasetGetResult(c.Param("name")))
}

func HandleClassicDatasetPreviewPost(c *gin.Context) {
	c.JSON(200, Deps.MLClassicDatasetPreviewResult(c.Param("name")))
}

func HandleMLDatasetPullPost(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLDatasetPullResult(req.URL))
}

func HandleMLDatasetImportPost(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	c.JSON(200, Deps.MLDatasetImportResult(req.Name))
}

func HandleMLDatasetExportGet(c *gin.Context) {
	c.JSON(200, Deps.MLDatasetExportResult())
}

func HandleMLDatasetClearDelete(c *gin.Context) {
	Deps.MLDatasetClear()
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleMLHealthProcessesGet(c *gin.Context) {
	c.JSON(200, Deps.MLHealthProcesses())
}

func HandleMLHealthGeneratorsGet(c *gin.Context) {
	c.JSON(200, Deps.MLHealthGenerators())
}

func HandleMLHealthRegisterPost(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	Deps.MLHealthRegister(req.ID)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleMLHealthUnregisterPost(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}
	Deps.MLHealthUnregister(req.ID)
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleMLHealthRunPost(c *gin.Context) {
	c.JSON(200, Deps.MLHealthRun())
}

// computeRiskScore combines classification, anomaly, and ML into a 0-100 risk score.
func computeRiskScore(classification *pb.BehaviorClassification, anomalyScore float64, mlPrediction MLPrediction, netAudit MLNetworkAuditResult, llmAssessment *MLLlmAssessment) float64 {
	score := 0.0

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

	score += anomalyScore * 30

	if mlPrediction.Confidence >= 0.60 {
		switch mlPrediction.Action {
		case 1:
			score += mlPrediction.Confidence * 25
		case 3:
			score += mlPrediction.Confidence * 15
		case 2:
			score += mlPrediction.Confidence * 8
		}
	}

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

func clampFloat64(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// ── ML type re-exports ─────────────────────────────────────────────

type MLPrediction struct {
	Action     int     `json:"action"`
	Confidence float64 `json:"confidence"`
	Label      string  `json:"label"`
}

type MLNetworkAuditResult struct {
	RiskLevel string `json:"riskLevel"`
}

type MLLlmAssessment struct {
	Error             string  `json:"error"`
	RiskScore         float64 `json:"riskScore"`
	Confidence        float64 `json:"confidence"`
	RecommendedAction string  `json:"recommendedAction"`
}