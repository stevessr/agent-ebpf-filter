package main

import (
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type TracepointBootstrapStatus struct {
	KernelRelease      string    `json:"kernelRelease"`
	CompiledCount      int       `json:"compiledCount"`
	AttachedCount      int       `json:"attachedCount"`
	SkippedCount       int       `json:"skippedCount"`
	SkippedTracepoints []string  `json:"skippedTracepoints,omitempty"`
	Status             string    `json:"status"`
	Message            string    `json:"message"`
	ObservedAt         time.Time `json:"observedAt"`
}

type tracepointBootstrapState struct {
	mu     sync.RWMutex
	status TracepointBootstrapStatus
}

func newTracepointBootstrapState() *tracepointBootstrapState {
	return &tracepointBootstrapState{
		status: TracepointBootstrapStatus{Status: "unknown", Message: "Tracepoint bootstrap has not been observed yet."},
	}
}

var bootstrapTracepointStatusStore = newTracepointBootstrapState()

func currentKernelRelease() string {
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		if rel := strings.TrimSpace(string(data)); rel != "" {
			return rel
		}
	}
	return "unknown"
}

func buildTracepointBootstrapStatus(compiledCount int, skipped []string) TracepointBootstrapStatus {
	status := TracepointBootstrapStatus{
		KernelRelease: currentKernelRelease(),
		CompiledCount: compiledCount,
		AttachedCount: compiledCount - len(skipped),
		SkippedCount:  len(skipped),
		Status:        "ready",
		ObservedAt:    time.Now().UTC(),
	}

	if status.AttachedCount < 0 {
		status.AttachedCount = 0
	}

	if len(skipped) > 0 {
		status.SkippedTracepoints = append([]string(nil), skipped...)
		sort.Strings(status.SkippedTracepoints)
		if status.AttachedCount == 0 {
			status.Status = "error"
			status.Message = "The backend could not attach any compiled tracepoints on this kernel."
		} else {
			status.Status = "partial"
			status.Message = "The backend booted successfully, but some kernel tracepoints were not exposed and were skipped."
		}
	} else {
		status.Message = "All compiled tracepoints attached successfully."
	}

	if status.CompiledCount == 0 {
		status.Status = "error"
		status.Message = "No tracepoint programs were compiled into the backend."
	}

	return status
}

func recordTracepointBootstrapStatus(compiledCount int, skipped []string) {
	bootstrapTracepointStatusStore.mu.Lock()
	bootstrapTracepointStatusStore.status = buildTracepointBootstrapStatus(compiledCount, skipped)
	bootstrapTracepointStatusStore.mu.Unlock()
}

func (s *tracepointBootstrapState) Snapshot() TracepointBootstrapStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := s.status
	if len(status.SkippedTracepoints) > 0 {
		status.SkippedTracepoints = append([]string(nil), status.SkippedTracepoints...)
	}
	return status
}

func handleBootstrapHealth(c *gin.Context) {
	c.JSON(200, bootstrapTracepointStatusStore.Snapshot())
}
