package research

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const researchControlRequestMaxBytes int64 = 64 << 10

func bindResearchJSON(c *gin.Context, target any) (int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, researchControlRequestMaxBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func handleResearchSessionsList(c *gin.Context) {
	sessions, err := researchSessionsStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func handleResearchSessionsCreate(c *gin.Context) {
	var req researchCreateSessionRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if status, err := bindResearchJSON(c, &req); err != nil {
			c.JSON(status, gin.H{"error": "invalid research session payload"})
			return
		}
	}
	session, err := researchSessionsStore.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, session)
}

func handleResearchSessionGet(c *gin.Context) {
	session, err := researchSessionsStore.Get(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func handleResearchSessionDelete(c *gin.Context) {
	if err := researchSessionsStore.Delete(c.Param("id")); err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func handleResearchSessionTask(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req researchTaskRequest
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if status, err := bindResearchJSON(c, &req); err != nil {
				c.JSON(status, gin.H{"error": "invalid research task payload"})
				return
			}
		}
		task, err := researchTaskStore.Submit(c.Param("id"), req, tlsStore)
		if err != nil {
			if errors.Is(err, errResearchQueueFull) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleResearchTaskGet(c *gin.Context) {
	task, ok := researchTaskStore.Get(c.Param("taskId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "research task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func handleResearchTasksStatus(c *gin.Context) {
	c.JSON(http.StatusOK, researchTaskStore.Status())
}

func handleResearchTaskCancel(c *gin.Context) {
	task := researchTaskStore.Cancel(c.Param("taskId"))
	status := http.StatusAccepted
	if task.Error == "task not found" {
		status = http.StatusNotFound
	}
	c.JSON(status, task)
}

func handleResearchSessionEvents(c *gin.Context) {
	events, err := researchSessionsStore.LoadEvents(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	filter := researchSourceFilterFromQuery(c)
	timerange := researchTimeRangeFromQuery(c)
	filtered := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, filter, timerange) {
			filtered = append(filtered, event)
		}
	}
	offset := parseResearchIntQuery(c, "offset", 0, 0, len(filtered))
	limit := parseResearchIntQuery(c, "limit", researchDefaultPageLimit, 1, researchMaxPageLimit)
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := []ResearchEvent{}
	if offset < len(filtered) {
		page = filtered[offset:end]
	}
	c.JSON(http.StatusOK, gin.H{"events": page, "total": len(filtered), "offset": offset, "limit": limit})
}

func handleResearchSessionResults(c *gin.Context) {
	results, err := researchSessionsStore.LoadResults(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, results)
}

func handleResearchSessionExport(c *gin.Context) {
	format := normalizeResearchFormat(c.Query("format"))
	if format == "" {
		format = "bundle"
	}
	if format != "jsonl" && format != "csv" && format != "bundle" && format != "json" && format != "security_json" && format != "security_jsonl" && format != "security_csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported export format", "supported": []string{"jsonl", "csv", "json", "bundle", "security-json", "security-jsonl", "security-csv"}})
		return
	}
	ref, payload, err := researchSessionsStore.ExportArtifact(c.Param("id"), format)
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", ref.Name))
	c.Header("X-Research-Artifact-SHA256", ref.SHA256)
	c.Data(http.StatusOK, ref.ContentType, payload)
}

func registerResearchRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	researchTaskStore.Start(snapshotRuntimeSettings().ResearchProcessing.QueueSize)
	router.GET("/sessions", handleResearchSessionsList)
	router.POST("/sessions", handleResearchSessionsCreate)
	router.GET("/sessions/:id", handleResearchSessionGet)
	router.DELETE("/sessions/:id", handleResearchSessionDelete)
	router.POST("/sessions/:id/tasks", handleResearchSessionTask(tlsStore))
	router.GET("/tasks/status", handleResearchTasksStatus)
	router.GET("/tasks/:taskId", handleResearchTaskGet)
	router.POST("/tasks/:taskId/cancel", handleResearchTaskCancel)
	router.GET("/sessions/:id/events", handleResearchSessionEvents)
	router.POST("/sessions/:id/detections/replay", handleResearchDetectionReplay)
	router.POST("/safety/evaluate", handleResearchSafetyEvaluate)
	router.GET("/sessions/:id/results", handleResearchSessionResults)
	router.GET("/sessions/:id/training", handleResearchSessionTraining)
	router.POST("/sessions/:id/training/import", handleResearchSessionTrainingImport)
	router.GET("/sessions/:id/export", handleResearchSessionExport)
}

func researchWriteStoreError(c *gin.Context, err error) {
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// RegisterRoutes wires the research workbench API onto the router; the
// transport entry point kept importable from the app layer.
func RegisterRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	registerResearchRoutes(router, tlsStore)
}
