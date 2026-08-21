package handlers

import (
	"encoding/json"
	"fmt"

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

func HandleMLAssessPost(c *gin.Context)          { Deps.MLAssessCommandSafety(c) }
func HandleMLExistingCommandsGet(c *gin.Context) { Deps.MLExistingCommandsGetFn(c) }
func HandleMLImportExistingPost(c *gin.Context)  { Deps.MLImportExistingFn(c) }
