package main

import (
	"encoding/json"
	"strings"
	"time"

	"agent-ebpf-filter/cuda"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// buildMLStatusJSON builds the complete ML status payload as JSON bytes.
// Shared by the HTTP handler and the WebSocket handler.
func buildMLStatusJSON() []byte {
	cfg := currentMLConfig()
	status := mlStatus()
	logs := globalTrainer.GetLogs(100)
	trainAccuracy, validationAccuracy, validationRatio, trainSamples, validationSamples := globalTrainer.SplitMetrics()
	autoTuneState := globalAutoTuneState.snapshot()

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
		"modelType":            string(currentModelType),
		"modelLoaded":          status.ModelLoaded,
		"numTrees":             status.NumTrees,
		"numSamples":           status.NumSamples,
		"numLabeledSamples":    status.NumLabeledSamples,
		"lastTrained":          status.LastTrained,
		"testAccuracy":         status.TestAccuracy,
		"modelPath":            status.ModelPath,
		"trainingInProgress":   status.TrainingInProgress,
		"trainingProgress":     status.TrainingProgress,
		"mlEnabled":            mlEnabled,
		"trainAccuracy":        trainAccuracy,
		"validationAccuracy":   validationAccuracy,
		"trainSamples":         trainSamples,
		"validationSamples":    validationSamples,
		"validationSplitRatio": validationRatio,
		"llmReview":            globalTrainer.LastLLMReview(),
		"autoTuneJobId":        autoTuneState.JobID,
		"autoTuneInProgress":   autoTuneState.Running,
		"autoTuneProgress":     autoTuneState.Progress,
		"autoTuneCompleted":    autoTuneState.Completed,
		"autoTuneTotal":        autoTuneState.Total,
		"autoTuneMessage":      autoTuneState.Message,
		"autoTuneError":        autoTuneState.Error,
		"autoTuneResult":       autoTuneState.Result,
		"mlConfig": map[string]interface{}{
			"modelType":            string(cfg.ModelType),
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

// serveMLStatusWS streams ML status updates via WebSocket using a ticker.
func serveMLStatusWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	intervalStr := c.DefaultQuery("interval", "1000")
	iv, _ := time.ParseDuration(intervalStr + "ms")
	if iv < 500*time.Millisecond {
		iv = 500 * time.Millisecond
	}
	ticker := time.NewTicker(iv)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// Send initial state immediately
	if err := conn.WriteMessage(websocket.TextMessage, buildMLStatusJSON()); err != nil {
		return
	}

	for {
		select {
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.TextMessage, buildMLStatusJSON()); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
