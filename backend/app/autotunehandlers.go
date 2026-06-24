package app

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section autotunehandlers.go ----

func handleMLTunePost(c *gin.Context) {
	if !mlEnabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	var req MLAutoTuneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if globalAutoTuneState.snapshot().Running {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}
	if globalTrainer.isRunning {
		c.JSON(409, gin.H{"error": "training already in progress"})
		return
	}

	jobID := fmt.Sprintf("tune-%d", time.Now().UnixNano())
	if !globalAutoTuneState.tryBegin(jobID, 0, "自动调参任务已接收") {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}

	go func(jobID string, req MLAutoTuneRequest) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ML] Auto-tune panic: %v", r)
				globalAutoTuneState.setError(jobID, fmt.Sprintf("panic: %v", r))
			}
		}()
		log.Printf("[ML] Auto-tune started: jobID=%s, model=%s, grid=%dx%d, x=%s, y=%s",
			jobID, mlConfig.ModelType, req.GridSize, req.GridSize, req.XAxis, req.YAxis)
		resp, err := globalTrainer.AutoTune(globalTrainingStore, req, func(completed, total int, message string) {
			if completed%5 == 0 || completed == total {
				log.Printf("[ML] Auto-tune progress: %d/%d — %s", completed, total, message)
			}
			globalAutoTuneState.update(jobID, completed, total, message)
		})
		if err != nil {
			log.Printf("[ML] Auto-tune error: %v", err)
		} else {
			log.Printf("[ML] Auto-tune done: %d cells, best score=%.4f", len(resp.Cells), autoTuneBestScore(resp))
		}
		globalAutoTuneState.finish(jobID, resp, err)
	}(jobID, req)

	c.JSON(202, gin.H{
		"jobId":   jobID,
		"started": true,
		"message": "自动调参已开始",
	})
}

func handleMLTuneModelsPost(c *gin.Context) {
	if !mlEnabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}
	if globalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	var req MLModelTuneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if globalAutoTuneState.snapshot().Running {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}
	if globalTrainer.isRunning {
		c.JSON(409, gin.H{"error": "training already in progress"})
		return
	}

	modelTypes := normalizeModelTuneTypes(req.ModelTypes)
	if len(modelTypes) == 0 {
		c.JSON(400, gin.H{"error": "no valid model types selected"})
		return
	}

	jobID := fmt.Sprintf("tune-models-%d", time.Now().UnixNano())
	if !globalAutoTuneState.tryBeginMode(jobID, "models", len(modelTypes), "模型调优任务已接收") {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}

	go func(jobID string, req MLModelTuneRequest, modelTypes []ModelType) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ML] Model auto-tune panic: %v", r)
				globalAutoTuneState.setError(jobID, fmt.Sprintf("panic: %v", r))
			}
		}()
		resp, err := runModelAutoTune(globalTrainingStore, req, modelTypes, func(completed, total int, message string) {
			globalAutoTuneState.update(jobID, completed, total, message)
		})
		if err != nil {
			log.Printf("[ML] Model auto-tune error: %v", err)
		} else {
			best := ""
			if resp.Best != nil {
				best = resp.Best.ModelType
			}
			log.Printf("[ML] Model auto-tune done: %d candidates, best=%s", len(resp.Candidates), best)
		}
		globalAutoTuneState.finishModelTune(jobID, resp, err)
	}(jobID, req, modelTypes)

	c.JSON(202, gin.H{
		"jobId":   jobID,
		"started": true,
		"mode":    "models",
		"message": "模型调优已开始",
	})
}

func normalizeModelTuneTypes(raw []string) []ModelType {
	seen := make(map[ModelType]bool)
	out := make([]ModelType, 0, len(raw))
	add := func(t ModelType) {
		if t == "" || seen[t] {
			return
		}
		if _, ok := modelRegistry[t]; !ok {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, value := range raw {
		add(ModelType(strings.TrimSpace(value)))
	}
	if len(out) == 0 {
		for _, item := range BuiltinModelCatalog() {
			if item.Recommended {
				add(ModelType(item.Value))
			}
		}
	}
	if len(out) == 0 {
		add(ModelRandomForest)
	}
	return out
}

func modelTuneCatalogInfo(t ModelType) (label, base string, recommended bool) {
	for _, item := range BuiltinModelCatalog() {
		if item.Value == string(t) {
			return item.Label, item.Base, item.Recommended
		}
	}
	return modelName(t), string(baseModelType(t)), false
}
