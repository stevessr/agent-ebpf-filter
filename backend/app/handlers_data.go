package app

import (
	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlers_data.go ----

func handleClearEvents(c *gin.Context) {
	capturedEventArchive.Clear()
	agentSightUploadedEvents.Clear()
	if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleClearEventsMemory(c *gin.Context) {
	capturedEventArchive.Clear()
	agentSightUploadedEvents.Clear()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleClearEventsPersisted(c *gin.Context) {
	if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
