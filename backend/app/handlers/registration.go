package handlers

import (
	"agent-ebpf-filter/app/events"
	"github.com/gin-gonic/gin"
)

// ---- moved from app/handlers_registration.go ----

func HandleRegister(c *gin.Context) {
	if Deps.TrackerMaps == nil {
		c.JSON(500, gin.H{"error": "agent pid map not initialized"})
		return
	}
	var req events.RegisterPayload
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}
	tag := req.Tag
	if tag == "" {
		tag = "AI Agent"
	}
	if err := Deps.TrackerMaps.AgentPidsPut(req.PID, Deps.GetTagID(tag)); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	Deps.ProcessContexts.Set(req.PID, events.ProcessContext{
		RootAgentPid: req.RootAgentPID,
		AgentRunID:   req.AgentRunID,
		TaskID:       req.TaskID,
		ConversationID: req.ConversationID,
		TurnID:       req.TurnID,
		ToolCallID:   req.ToolCallID,
		ToolName:     req.ToolName,
		TraceID:      req.TraceID,
		SpanID:       req.SpanID,
		Decision:     req.Decision,
		ContainerID:  req.ContainerID,
		ArgvDigest:   req.ArgvDigest,
		Cwd:          req.Cwd,
		RiskScore:    req.RiskScore,
	})
	c.JSON(200, gin.H{"status": "ok"})
}

func HandleUnregister(c *gin.Context) {
	if Deps.TrackerMaps == nil {
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
	Deps.TrackerMaps.AgentPidsDelete(req.PID)
	Deps.ProcessContexts.Delete(req.PID)
	c.JSON(200, gin.H{"status": "ok"})
}