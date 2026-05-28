package main

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"strings"

	"agent-ebpf-filter/core"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !releaseAuthEnforced() || clusterRequestAuthAllowed(c) {
			c.Next()
			return
		}
		token := requestAuthToken(c)
		expectedKey := runtimeSettingsStore.ExpectedToken()
		if token == "" || expectedKey == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func releaseAuthEnforced() bool {
	return gin.Mode() == gin.ReleaseMode && os.Getenv("DISABLE_AUTH") != "true"
}

func hookIngressAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !releaseAuthEnforced() || clusterRequestAuthAllowed(c) {
			c.Next()
			return
		}

		if token := requestAuthToken(c); token != "" {
			expectedKey := runtimeSettingsStore.ExpectedToken()
			if subtle.ConstantTimeCompare([]byte(token), []byte(expectedKey)) == 1 {
				c.Next()
				return
			}
		}

		hookID := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Agent-CLI")))
		secret := strings.TrimSpace(c.GetHeader("X-Agent-Hook-Secret"))
		expectedSecret := runtimeSettingsStore.HookSecret(hookID)
		if hookID == "" || secret == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(expectedSecret)) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		c.Next()
	}
}

func runtimeToggleMiddleware(featureName string, enabled func(core.RuntimeSettings) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if enabled(runtimeSettingsStore.Snapshot()) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   featureName + " is disabled",
			"feature": featureName,
		})
	}
}

func shellSessionsEnabledMiddleware() gin.HandlerFunc {
	return runtimeToggleMiddleware("shell_sessions", func(settings core.RuntimeSettings) bool {
		return settings.ShellSessionsEnabled
	})
}

func systemRunEnabledMiddleware() gin.HandlerFunc {
	return runtimeToggleMiddleware("system_run", func(settings core.RuntimeSettings) bool {
		return settings.SystemRunEnabled
	})
}

func hookManagementEnabledMiddleware() gin.HandlerFunc {
	return runtimeToggleMiddleware("hook_management", func(settings core.RuntimeSettings) bool {
		return settings.HookManagementEnabled
	})
}

func policyManagementEnabledMiddleware() gin.HandlerFunc {
	return runtimeToggleMiddleware("policy_management", func(settings core.RuntimeSettings) bool {
		return settings.PolicyManagementEnabled
	})
}

func requestAuthToken(c *gin.Context) string {
	if token := strings.TrimSpace(c.Query("key")); token != "" {
		return token
	}
	if token := strings.TrimSpace(c.GetHeader("X-API-KEY")); token != "" {
		return token
	}
	if authHeader := strings.TrimSpace(c.GetHeader("Authorization")); authHeader != "" {
		lower := strings.ToLower(authHeader)
		if strings.HasPrefix(lower, "bearer ") {
			return strings.TrimSpace(authHeader[len("Bearer "):])
		}
	}
	return ""
}

func buildRuntimeConfigResponseFromSettings(settings core.RuntimeSettings) core.RuntimeConfigResponse {
	logPath := strings.TrimSpace(settings.LogFilePath)
	logAlive := false
	if settings.LogPersistenceEnabled && logPath != "" {
		if info, err := os.Stat(logPath); err == nil && !info.IsDir() {
			logAlive = true
		}
	}
	settings.MLConfig.LlmAPIKey = ""
	if settings.OtlpHeaders == nil {
		settings.OtlpHeaders = map[string]string{}
	} else {
		cloned := make(map[string]string, len(settings.OtlpHeaders))
		for key, value := range settings.OtlpHeaders {
			if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
				cloned[trimmedKey] = strings.TrimSpace(value)
			}
		}
		settings.OtlpHeaders = cloned
	}
	return core.RuntimeConfigResponse{
		Runtime:                settings,
		MCPEndpoint:            fmt.Sprintf("http://127.0.0.1:%d/mcp", resolveBackendPort()),
		AuthHeaderName:         "X-API-KEY",
		BearerAuthHeaderName:   "Authorization: Bearer",
		PersistedEventLogPath:  logPath,
		PersistedEventLogAlive: logAlive,
	}
}

func buildRuntimeConfigResponse() core.RuntimeConfigResponse {
	return buildRuntimeConfigResponseFromSettings(runtimeSettingsStore.Snapshot())
}

func getTagName(id uint32) string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok {
		return name
	}
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok {
		return id
	}
	id := nextTagID
	tagMap[id] = name
	tagNameToID[name] = id
	nextTagID++
	return id
}

func deltaUint64(current, previous uint64) uint64 {
	if current >= previous {
		return current - previous
	}
	return 0
}

// ── Proto response negotiation ─────────────────────────────────────────

func clientPrefersProto(c *gin.Context) bool {
	accept := c.GetHeader("Accept")
	return strings.Contains(accept, "application/x-protobuf") || strings.Contains(accept, "application/protobuf")
}

func writeProtoResponse(c *gin.Context, code int, msg proto.Message) {
	data, err := proto.Marshal(msg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "proto marshal failed"})
		return
	}
	c.Data(code, "application/x-protobuf", data)
}

// writeProtoOrJSON sends a protobuf response if the client prefers it, otherwise JSON.
// jsonData should be a value compatible with gin's c.JSON().
func writeProtoOrJSON(c *gin.Context, code int, msg proto.Message, jsonData interface{}) {
	if clientPrefersProto(c) {
		writeProtoResponse(c, code, msg)
	} else {
		c.JSON(code, jsonData)
	}
}
