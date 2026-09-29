package research

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func runSafetyReviewRequest(body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodPost, "/research/safety/evaluate", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	handleResearchSafetyEvaluate(context)
	return response
}

func TestSafetyReviewStrictRequestBounds(t *testing.T) {
	valid := `{"lineage":[{"id":"operator","uid":1000,"generation":1,"grants":[]}],"watchdog":{"agentId":"operator","nowMs":1000,"heartbeatTimeoutMs":500,"denialWindowMs":500,"denialThreshold":2}}`
	for _, input := range []string{
		valid[:len(valid)-1] + `,"unknown":true}`,
		valid + valid,
		`{"lineage":[],"watchdog":{},"unknown":1}`,
	} {
		if response := runSafetyReviewRequest(input); response.Code != http.StatusBadRequest {
			t.Errorf("unknown/trailing request should be rejected: %d %s", response.Code, response.Body.String())
		}
	}
	if response := runSafetyReviewRequest(strings.Repeat("x", int(researchControlRequestMaxBytes)+1)); response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request not rejected: %d %s", response.Code, response.Body.String())
	}
	response := runSafetyReviewRequest(valid)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"within_boundary"`) ||
		!strings.Contains(response.Body.String(), `"enforcementApplied":false`) {
		t.Fatalf("read-only response unexpected: %d %s", response.Code, response.Body.String())
	}
}
