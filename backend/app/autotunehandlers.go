package app

import (
	"agent-ebpf-filter/app/ml"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section autotunehandlers.go ----

func autotuneTunePost(c *gin.Context) {
	if !ml.SnapshotMLRuntime().Enabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}
	if ml.GlobalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	var req ml.MLAutoTuneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if ml.GlobalAutoTuneState.Snapshot().Running {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}
	if ml.GlobalTrainer.IsRunning() {
		c.JSON(409, gin.H{"error": "training already in progress"})
		return
	}

	jobID := fmt.Sprintf("tune-%d", time.Now().UnixNano())
	if !ml.GlobalAutoTuneState.TryBegin(jobID, 0, "自动调参任务已接收") {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}

	entry := newBackendTaskRuntimeEntry(jobID, "ml_auto_tune_params", mlAutoTuneParamsTask{Request: req})
	if err := mlAutoTuneTasks.Submit(entry); err != nil {
		ml.GlobalAutoTuneState.SetError(jobID, err.Error())
		c.JSON(503, gin.H{"error": err.Error()})
		return
	}

	c.JSON(202, gin.H{
		"jobId":   jobID,
		"started": true,
		"message": "自动调参已开始",
	})
}

func autotuneTuneModelsPost(c *gin.Context) {
	if !ml.SnapshotMLRuntime().Enabled {
		c.JSON(400, gin.H{"error": "ML engine is not enabled on this node"})
		return
	}
	if ml.GlobalTrainingStore == nil {
		c.JSON(400, gin.H{"error": "ML training store not initialized"})
		return
	}

	var req ml.MLModelTuneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request"})
		return
	}

	if ml.GlobalAutoTuneState.Snapshot().Running {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}
	if ml.GlobalTrainer.IsRunning() {
		c.JSON(409, gin.H{"error": "training already in progress"})
		return
	}

	modelTypes := normalizeModelTuneTypes(req.ModelTypes)
	if len(modelTypes) == 0 {
		c.JSON(400, gin.H{"error": "no valid model types selected"})
		return
	}

	jobID := fmt.Sprintf("tune-models-%d", time.Now().UnixNano())
	if !ml.GlobalAutoTuneState.TryBeginMode(jobID, "models", len(modelTypes), "模型调优任务已接收") {
		c.JSON(409, gin.H{"error": "auto tuning already in progress"})
		return
	}

	entry := newBackendTaskRuntimeEntry(jobID, "ml_auto_tune_models", mlAutoTuneModelsTask{
		Request:    req,
		ModelTypes: append([]ModelType(nil), modelTypes...),
	})
	if err := mlAutoTuneTasks.Submit(entry); err != nil {
		ml.GlobalAutoTuneState.SetError(jobID, err.Error())
		c.JSON(503, gin.H{"error": err.Error()})
		return
	}

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
		if _, ok := ml.ModelRegistry[t]; !ok {
			return
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, value := range raw {
		add(ModelType(strings.TrimSpace(value)))
	}
	if len(out) == 0 {
		for _, item := range ml.BuiltinModelCatalog() {
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
	for _, item := range ml.BuiltinModelCatalog() {
		if item.Value == string(t) {
			return item.Label, item.Base, item.Recommended
		}
	}
	return ml.ModelName(t), string(ml.BaseModelType(t)), false
}
