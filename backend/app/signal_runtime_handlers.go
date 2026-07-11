package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func handleSignalProcessingStatus(c *gin.Context) {
	c.JSON(200, signalProcessingWorkerStore.Status())
}

func handleSignalProcessingTask(c *gin.Context) {
	var req signalProcessingTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid signal processing task"})
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
		records, _, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !signalProcessingWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "signal processing worker queue is full or not started"})
			return
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "expire", "cron":
		if !signalProcessingWorkerStore.EnqueueExpire() {
			signalProcessingWorkerStore.expireNow(time.Now().UTC())
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "expire", QueueLen: status.QueueLen})
	case "reset":
		if !signalProcessingWorkerStore.EnqueueReset() {
			signalProcessingWorkerStore.resetNow()
		}
		status := signalProcessingWorkerStore.Status()
		c.JSON(202, signalProcessingTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported signal processing action"})
	}
}

func handleSignalRuleTest(c *gin.Context) {
	var req signalRuleTestRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Rule == nil {
		c.JSON(400, gin.H{"error": "invalid signal rule test request"})
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	rule := *req.Rule
	normalizeSignalRule(&rule, settings.DefaultTTLSeconds, 0)

	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 50000 {
		limit = 50000
	}
	records, _, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	response := signalRuleTestResponse{
		Rule:         rule,
		ScannedTotal: len(records),
		Matches:      make([]signalRuleTestMatch, 0, 25),
	}
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		if record.Event == nil || shouldIgnoreSignalProcessingEvent(record.Event) {
			continue
		}
		if !signalRuleMatches(rule, record.Event) {
			continue
		}
		response.MatchedTotal++
		if len(response.Matches) >= 25 {
			continue
		}
		key, target := signalStateKey(rule, record.Event)
		response.Matches = append(response.Matches, signalRuleTestMatch{
			Timestamp:  record.ReceivedAt.UTC(),
			PID:        record.Event.GetPid(),
			TGID:       record.Event.GetTgid(),
			Comm:       strings.TrimSpace(record.Event.GetComm()),
			EventType:  signalEventType(record.Event),
			Target:     target,
			Path:       strings.TrimSpace(record.Event.GetPath()),
			ExtraPath:  strings.TrimSpace(record.Event.GetExtraPath()),
			ExtraInfo:  strings.TrimSpace(record.Event.GetExtraInfo()),
			EventID:    recordEnvelopeID(record),
			SignalKey:  key,
			WouldScore: rule.Weight,
		})
	}
	c.JSON(200, response)
}

func handleSignalProgramLogs(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	c.JSON(200, signalProgramLogsResponse{
		Compression: settings.ProtoLogCompression,
		Logs:        selectedProgramLogStatuses(settings),
	})
}

func handleSignalProgramLogDownload(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	program := c.Query("program")
	path, ok := resolveSelectedProgramLogPath(settings, program)
	if !ok {
		c.JSON(404, gin.H{"error": "selected program log is not configured"})
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(404, gin.H{"error": "selected program log file does not exist"})
			return
		}
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if info.IsDir() {
		c.JSON(400, gin.H{"error": "selected program log path is a directory"})
		return
	}
	c.FileAttachment(path, filepath.Base(path))
}
