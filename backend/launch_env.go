package main

import (
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type LaunchEnvEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var launchEnvExcludedExact = map[string]struct{}{
	"DISABLE_AUTH": {},
	"GIN_MODE":     {},
	"PKEXEC_UID":   {},
	"SUDO_UID":     {},
	"SUDO_GID":     {},
	"SUDO_USER":    {},
}

var launchEnvExcludedPrefixes = []string{
	"AGENT_",
}

func isBackendRuntimeEnvKey(key string) bool {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return true
	}
	if _, ok := launchEnvExcludedExact[trimmed]; ok {
		return true
	}
	for _, prefix := range launchEnvExcludedPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func collectLaunchEnvEntries() []LaunchEnvEntry {
	items := make([]LaunchEnvEntry, 0, len(os.Environ()))
	for _, raw := range os.Environ() {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || isBackendRuntimeEnvKey(key) {
			continue
		}
		items = append(items, LaunchEnvEntry{
			Key:   key,
			Value: value,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			return items[i].Value < items[j].Value
		}
		return strings.ToLower(items[i].Key) < strings.ToLower(items[j].Key)
	})
	return items
}

func handleListLaunchEnvEntries(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"items": collectLaunchEnvEntries(),
	})
}
