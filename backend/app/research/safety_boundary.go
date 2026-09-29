package research

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"agent-ebpf-filter/internal/agentboundary"

	"github.com/gin-gonic/gin"
)

// A policy/telemetry SIMULATION, never a sandbox controller. This route
// inherits authentication from registerAuthenticatedAPIRoutes.
func handleResearchSafetyEvaluate(c *gin.Context) {
	var req agentboundary.SafetyCase
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, researchControlRequestMaxBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error":"safety case too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error":"invalid safety case"})
		}
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error":"unexpected trailing JSON"})
		return
	}
	c.JSON(http.StatusOK, agentboundary.EvaluateCase(req))
}
