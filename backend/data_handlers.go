package main

import "github.com/gin-gonic/gin"

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
