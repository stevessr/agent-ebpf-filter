package handlers

import (
	"github.com/gin-gonic/gin"
)

func HandleClearEvents(c *gin.Context) {
	Deps.EventArchiveClear()
	Deps.AgentSightEventsClear()
	if err := Deps.RuntimeSettingsTruncateLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleClearEventsMemory(c *gin.Context) {
	Deps.EventArchiveClear()
	Deps.AgentSightEventsClear()
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleClearEventsPersisted(c *gin.Context) {
	if err := Deps.RuntimeSettingsTruncateLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
