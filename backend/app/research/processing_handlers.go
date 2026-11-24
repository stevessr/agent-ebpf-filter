package research

import (
	"strings"

	"github.com/gin-gonic/gin"
)

func handleResearchProcessingStatus(c *gin.Context) {
	c.JSON(200, ProcessingWorker.Status())
}

func handleResearchProcessingTask(c *gin.Context) {
	var req researchProcessingTaskRequest
	if status, err := bindResearchJSON(c, &req); err != nil {
		c.JSON(status, gin.H{"error": "invalid research processing task"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "scan_recent"
	}
	switch action {
	case "scan_recent", "scan":
		limit := req.Limit
		if limit <= 0 {
			limit = 1000
		}
		if limit > 50000 {
			limit = 50000
		}
		records, _, err := recentCapturedEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !ProcessingWorker.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "research processing worker queue is full or not started"})
			return
		}
		status := ProcessingWorker.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "reset":
		if !ProcessingWorker.EnqueueReset() {
			ProcessingWorker.resetNow()
		}
		status := ProcessingWorker.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported research processing task", "supported": []string{"scan_recent", "reset"}})
	}
}

// HandleProcessingStatus serves GET /research-processing/status.
func HandleProcessingStatus(c *gin.Context) { handleResearchProcessingStatus(c) }

// HandleProcessingTask serves POST /research-processing/task.
func HandleProcessingTask(c *gin.Context) { handleResearchProcessingTask(c) }
