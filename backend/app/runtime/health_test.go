package runtime

import "testing"

func TestBuildTracepointBootstrapStatusKeepsEmptySkippedTracepoints(t *testing.T) {
	status := buildTracepointBootstrapStatus(3, nil)
	if status.SkippedTracepoints == nil {
		t.Fatal("expected skipped tracepoints to serialize as an empty array, not null or omitted")
	}
	if len(status.SkippedTracepoints) != 0 {
		t.Fatalf("expected no skipped tracepoints, got %+v", status.SkippedTracepoints)
	}
	if status.Status != "ready" {
		t.Fatalf("expected ready status, got %q", status.Status)
	}
}
