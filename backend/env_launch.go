package main

import (
	"net/http"

	"agent-ebpf-filter/internal/launchenv"
	"github.com/gin-gonic/gin"
)

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
