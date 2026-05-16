package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func handleExternalAPIHealth(c *gin.Context) {
	settings := runtimeSettingsStore.Snapshot()
	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"service":    "agent-ebpf-filter",
		"apiVersion": "v1",
		"port":       resolveBackendPort(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
		"features": gin.H{
			"logPersistenceEnabled":     settings.LogPersistenceEnabled,
			"shellSessionsEnabled":      settings.ShellSessionsEnabled,
			"systemRunEnabled":          settings.SystemRunEnabled,
			"hookManagementEnabled":     settings.HookManagementEnabled,
			"policyManagementEnabled":   settings.PolicyManagementEnabled,
			"otlpEnabled":               settings.OtlpEnabled,
			"tlsCaptureEnabled":         settings.TlsCaptureEnabled,
			"domainForwardProxyEnabled": settings.DomainForwardProxy.Enabled,
		},
		"bootstrap": bootstrapTracepointStatusStore.Snapshot(),
		"collector": collectorMetricsStore.Snapshot(),
	})
}

func handleExternalAPIOpenAPI(c *gin.Context) {
	c.JSON(http.StatusOK, buildExternalOpenAPISpec())
}

func buildExternalOpenAPISpec() gin.H {
	return gin.H{
		"openapi": "3.0.3",
		"info": gin.H{
			"title":       "Agent eBPF Filter external API",
			"version":     "v1",
			"description": "Stable aliases for external automation, observability collectors, and Kubernetes in-cluster callers.",
		},
		"servers": []gin.H{{"url": "/api/v1"}},
		"components": gin.H{
			"securitySchemes": gin.H{
				"ApiKeyAuth": gin.H{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-KEY",
				},
				"BearerAuth": gin.H{
					"type":   "http",
					"scheme": "bearer",
				},
				"QueryKey": gin.H{
					"type":        "apiKey",
					"in":          "query",
					"name":        "key",
					"description": "Use only for WebSocket/SSE clients or tightly controlled automation where headers are unavailable.",
				},
			},
		},
		"security": []gin.H{{"ApiKeyAuth": []string{}}, {"BearerAuth": []string{}}, {"QueryKey": []string{}}},
		"paths": gin.H{
			"/health":                         endpointSpec("GET", "Service health, collector counters, feature gates, and eBPF bootstrap status."),
			"/events/recent":                  endpointSpec("GET", "Recent captured events. Query: limit, type."),
			"/events/graph":                   endpointSpec("GET", "Aggregated execution graph. Query: agent_run_id, tool_call_id, trace_id, pid, path, domain, risk_min, since, until."),
			"/network/flows":                  endpointSpec("GET", "Attributed TCP/UDP flow summaries. Query: filter, sort, showHistoric, limit, cursor, pid, domain, service, scope."),
			"/network/flows/{flowID}":         endpointSpec("GET", "One network flow by stable flow id."),
			"/network/dns-cache":              endpointSpec("GET", "DNS correlation cache."),
			"/network/interfaces":             endpointSpec("GET", "Per-interface RX/TX counters, packets, errors, and drops."),
			"/network/export/jsonl":           endpointSpec("GET", "Metadata-only flow JSONL export."),
			"/sandbox/cgroup/status":          endpointSpec("GET", "cgroup/connect + sendmsg enforcement status, maps, counters, and active blocks."),
			"/sandbox/lsm/status":             endpointSpec("GET", "BPF LSM enforcement status, maps, counters, and active blocks."),
			"/policies/network/block-ip":      endpointSpec("POST", "Block an IPv4/IPv6 destination through the cgroup sandbox. Body: {\"ip\":\"203.0.113.10\"}."),
			"/policies/network/unblock-ip":    endpointSpec("POST", "Remove an IP destination block. Body: {\"ip\":\"203.0.113.10\"}."),
			"/policies/network/block-port":    endpointSpec("POST", "Block a TCP/UDP destination port. Body: {\"port\":4444}."),
			"/policies/network/unblock-port":  endpointSpec("POST", "Remove a TCP/UDP destination-port block. Body: {\"port\":4444}."),
			"/policies/network/block-pid":     endpointSpec("POST", "Resolve a PID's cgroup v2 inode id and block outbound networking for that cgroup. Body: {\"pid\":1234}."),
			"/policies/network/unblock-pid":   endpointSpec("POST", "Resolve a PID's cgroup v2 inode id and remove its cgroup network block. Body: {\"pid\":1234}."),
			"/policies/lsm/block-exec-path":   endpointSpec("POST", "Block an executable path with BPF LSM. Body: {\"path\":\"/usr/bin/nc\"}."),
			"/policies/lsm/unblock-exec-path": endpointSpec("POST", "Remove an executable-path block. Body: {\"path\":\"/usr/bin/nc\"}."),
			"/policies/lsm/block-exec-name":   endpointSpec("POST", "Block executable basename with BPF LSM. Body: {\"name\":\"nc\"}."),
			"/policies/lsm/unblock-exec-name": endpointSpec("POST", "Remove executable-basename block. Body: {\"name\":\"nc\"}."),
			"/policies/lsm/block-file-name":   endpointSpec("POST", "Block file/directory basename for open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename. Body: {\"name\":\"id_rsa\"}."),
			"/policies/lsm/unblock-file-name": endpointSpec("POST", "Remove file/directory basename block. Body: {\"name\":\"id_rsa\"}."),
			"/agents/register":                endpointSpec("POST", "Register an agent PID and optional run/task/tool context. Body follows the root /register payload."),
			"/agents/unregister":              endpointSpec("POST", "Unregister an agent PID. Body: {\"pid\":1234}."),
			"/config/export":                  endpointSpec("GET", "Export tags, tracked commands, tracked paths, wrapper rules, and runtime settings."),
		},
	}
}

func endpointSpec(method, summary string) gin.H {
	return gin.H{
		strings.ToLower(method): gin.H{
			"summary":   summary,
			"responses": gin.H{"200": gin.H{"description": "OK"}},
		},
	}
}

func registerExternalAPIRoutes(rg *gin.RouterGroup) {
	rg.GET("/health", handleExternalAPIHealth)
	rg.GET("/openapi.json", handleExternalAPIOpenAPI)

	rg.GET("/events/recent", handleRecentEvents)
	rg.GET("/events/graph", handleExecutionGraph)

	rg.GET("/network/flows", handleNetworkFlows)
	rg.GET("/network/flows/:flowID", handleNetworkFlowByID)
	rg.GET("/network/dns-cache", handleDNSCache)
	rg.GET("/network/interfaces", handleNetworkInterfaces)
	rg.GET("/network/export/jsonl", handleNetworkFlowJSONLExport)

	rg.GET("/sandbox/cgroup/status", handleCgroupSandboxStatus)
	rg.GET("/sandbox/lsm/status", handleLsmEnforcerStatus)

	policies := rg.Group("/policies", policyManagementEnabledMiddleware())
	{
		policies.POST("/network/block-ip", handleCgroupSandboxBlockIP)
		policies.POST("/network/unblock-ip", handleCgroupSandboxUnblockIP)
		policies.POST("/network/block-port", handleCgroupSandboxBlockPort)
		policies.POST("/network/unblock-port", handleCgroupSandboxUnblockPort)
		policies.POST("/network/block-pid", handleCgroupSandboxBlockPID)
		policies.POST("/network/unblock-pid", handleCgroupSandboxUnblockPID)
		policies.POST("/lsm/block-exec-path", handleLsmBlockExecPath)
		policies.POST("/lsm/unblock-exec-path", handleLsmUnblockExecPath)
		policies.POST("/lsm/block-exec-name", handleLsmBlockExecName)
		policies.POST("/lsm/unblock-exec-name", handleLsmUnblockExecName)
		policies.POST("/lsm/block-file-name", handleLsmBlockFileName)
		policies.POST("/lsm/unblock-file-name", handleLsmUnblockFileName)
	}

	rg.POST("/agents/register", handleRegister)
	rg.POST("/agents/unregister", handleUnregister)
	rg.GET("/config/export", handleConfigExportGet)
}
