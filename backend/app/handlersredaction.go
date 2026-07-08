package app

import (
	"net/http"

	"agent-ebpf-filter/redaction"
	"github.com/gin-gonic/gin"
)

// handleConfigRedactionPolicyGet returns the current redaction policy.
func handleConfigRedactionPolicyGet(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot()
	policy := settings.RedactionPolicy
	if policy.DefaultPlaceholder == "" {
		policy.DefaultPlaceholder = "[REDACTED]"
	}
	c.JSON(http.StatusOK, policy)
}

// handleConfigRedactionPolicyPut updates the redaction policy.
func handleConfigRedactionPolicyPut(c *gin.Context) {
	var req redaction.RedactionPolicy
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid redaction policy: " + err.Error()})
		return
	}
	if req.Level == "" {
		req.Level = redaction.RedactionLevelStandard
	}
	if req.DefaultPlaceholder == "" {
		req.DefaultPlaceholder = "[REDACTED]"
	}

	settings := runtimeSettingsStore.Snapshot()
	settings.RedactionPolicy = req

	updated, err := runtimeSettingsStore.Replace(settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Rebuild the redaction engine with the new policy
	initRedactionEngine()
	policy := updated.RedactionPolicy
	if policy.DefaultPlaceholder == "" {
		policy.DefaultPlaceholder = "[REDACTED]"
	}
	c.JSON(http.StatusOK, policy)
}