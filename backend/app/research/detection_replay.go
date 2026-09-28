package research

import (
	"net/http"

	"agent-ebpf-filter/internal/detectionengineering"

	"github.com/gin-gonic/gin"
)

// handleResearchDetectionReplay is deliberately read-only. Candidate rules may
// originate from a human or an LLM, but are not installed into wrapper/cgroup/LSM.
// Registration is under the existing authenticated /research route group.
func handleResearchDetectionReplay(c *gin.Context) {
	var req struct {
		Rule   detectionengineering.Rule `json:"rule"`
		Labels map[string]string         `json:"labels,omitempty"`
	}
	if status, err := bindResearchJSON(c, &req); err != nil {
		c.JSON(status, gin.H{"error": "invalid detection replay payload"})
		return
	}
	// Reject incorrect rule/label input before opening the session artifacts.
	if len(req.Labels) > detectionengineering.MaxLabels {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many labeled scopes"})
		return
	}
	events, err := researchSessionsStore.LoadEvents(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	if len(events) > detectionengineering.MaxEvents {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "too many session events for bounded detection replay"})
		return
	}
	input := make([]detectionengineering.Event, 0, len(events))
	for _, event := range events {
		input = append(input, detectionengineering.Event{
			ID: event.ID, Timestamp: event.Timestamp, TraceID: event.TraceID,
			PID: event.PID, EventType: event.EventType, Source: event.Source,
			Comm: event.Comm,
		})
	}
	// The response contains recorded evidence only, never raw target/payload,
	// model-inferred ground truth or a policy-management side effect.
	c.JSON(http.StatusOK, detectionengineering.Replay(req.Rule, input, req.Labels))
}
