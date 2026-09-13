package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

func HandleMLStatusGet(c *gin.Context) {
	status := Deps.ML.Status()
	var payload gin.H
	if err := json.Unmarshal(Deps.ML.StatusJSON(), &payload); err != nil {
		c.JSON(500, gin.H{"error": "Failed to build ML status"})
		return
	}
	Deps.WriteProtoOrJSON(c, 200, status, payload)
}

func HandleMLLogsGet(c *gin.Context) {
	c.JSON(200, Deps.ML.Logs())
}

func HandleMLTrainCancelPost(c *gin.Context) {
	if !Deps.ML.IsTraining() {
		c.JSON(200, gin.H{"message": "no training in progress"})
		return
	}
	Deps.ML.CancelTraining()
	c.JSON(200, gin.H{"message": "cancellation requested"})
}

func HandleMLHistoryGet(c *gin.Context) {
	c.JSON(200, Deps.ML.History())
}

func HandleMLTrainPost(c *gin.Context) {
	if !Deps.ML.Enabled() {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}

	var req struct {
		NumTrees       int `json:"numTrees"`
		MaxDepth       int `json:"maxDepth"`
		MinSamplesLeaf int `json:"minSamplesLeaf"`
	}
	_ = c.ShouldBindJSON(&req)

	c.JSON(200, Deps.ML.Train(req.NumTrees, req.MaxDepth, req.MinSamplesLeaf))
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
	c.JSON(200, Deps.ML.Feedback(req.Comm, req.UserAction))
}

func HandleMLSamplesGet(c *gin.Context) {
	c.JSON(200, Deps.ML.Samples())
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
	c.JSON(200, Deps.ML.LabelSample(req.Index, req.Label))
}

func HandleMLSampleDelete(c *gin.Context) {
	indexStr := c.Param("index")
	var index int
	if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
		c.JSON(400, gin.H{"error": "invalid index"})
		return
	}
	c.JSON(200, Deps.ML.RemoveSample(index))
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
	c.JSON(200, Deps.ML.SetSampleAnomaly(req.Index, req.AnomalyScore))
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
	c.JSON(200, Deps.ML.AddSample(req.CommandLine, req.Comm, req.Args, req.Label))
}
