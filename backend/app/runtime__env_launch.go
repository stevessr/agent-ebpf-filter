package app

import (
	"agent-ebpf-filter/internal/launchenv"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section env_launch.go ----

type LaunchEnvEntry = launchenv.Entry

func isBackendRuntimeEnvKey(key string) bool {
	return launchenv.IsBackendRuntimeKey(key)
}

func collectLaunchEnvEntries() []LaunchEnvEntry {
	return launchenv.Collect()
}

func handleListLaunchEnvEntries(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": collectLaunchEnvEntries(),
	})
}
