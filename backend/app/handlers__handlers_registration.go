package app

import (
	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section handlers_registration.go ----

func handleRegister(c *gin.Context) {
	if trackerMaps.AgentPids == nil {
		c.JSON(500, gin.H{"error": "agent pid map not initialized"})
		return
	}
	var req registerPayload
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}
	tag := req.Tag
	if tag == "" {
		tag = "AI Agent"
	}
	if err := trackerMaps.AgentPids.Put(req.PID, getTagID(tag)); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	trackedProcessContexts.Set(req.PID, buildProcessContextFromRegister(req))
	c.JSON(200, gin.H{"status": "ok"})
}

func handleUnregister(c *gin.Context) {
	if trackerMaps.AgentPids == nil {
		c.JSON(500, gin.H{"error": "agent pid map not initialized"})
		return
	}
	var req struct {
		PID uint32 `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}
	_ = trackerMaps.AgentPids.Delete(req.PID)
	trackedProcessContexts.Delete(req.PID)
	c.JSON(200, gin.H{"status": "ok"})
}
