package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func HandleSandboxRuntimeStatus(c *gin.Context) {
	if Deps.SandboxRuntime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sandbox runtime integration not initialized"})
		return
	}
	c.JSON(http.StatusOK, Deps.SandboxRuntime.Status())
}

func HandleSandboxRuntimeDetect(c *gin.Context) {
	if Deps.SandboxRuntime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sandbox runtime integration not initialized"})
		return
	}
	pid, err := strconv.Atoi(c.Query("pid"))
	if err != nil || pid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pid query parameter must be a positive integer"})
		return
	}
	detection, err := Deps.SandboxRuntime.DetectPID(pid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, detection)
}

func HandleSandboxRuntimeActive(c *gin.Context) {
	if Deps.SandboxRuntime == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "sandbox runtime integration not initialized"})
		return
	}
	limit := 128
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	detections, err := Deps.SandboxRuntime.ListActive(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"count":    len(detections),
		"runtimes": detections,
	})
}
