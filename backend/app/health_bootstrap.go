// Package app bridges between the monolithic app/ package and the
// refactored subpackages. This file delegates health-bootstrap logic
// to the runtime subpackage.
package app

import (
	"agent-ebpf-filter/app/runtime"

	"github.com/gin-gonic/gin"
)

// Bridge: health_bootstrap.go → runtime/

var bootstrapTracepointStatusStore = tracepointBootstrapBridge{}

type tracepointBootstrapBridge struct{}

func (tracepointBootstrapBridge) Snapshot() runtime.TracepointBootstrapStatus {
	return runtime.SnapshotBootstrapTracepointStatus()
}

func recordTracepointBootstrapStatus(compiledCount int, skipped []string) {
	runtime.RecordTracepointBootstrapStatus(compiledCount, skipped)
}

func handleBootstrapHealth(c *gin.Context) {
	runtime.HandleBootstrapHealth(c)
}
