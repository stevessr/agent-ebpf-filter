package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section api_external.go ----

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
		"featureManifest": buildFeatureManifest(settings).Features,
		"bootstrap":       bootstrapTracepointStatusStore.Snapshot(),
		"collector":       collectorMetricsStore.Snapshot(),
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
			"/health":        endpointSpec("GET", "Service health, collector counters, feature gates, and eBPF bootstrap status."),
			"/events/recent": endpointSpec("GET", "Recent captured events. Query: limit, type, event_type, source, pid, comm, trace_id, span_id, since, until, redaction_state."),
			"/events/graph":  endpointSpec("GET", "Aggregated execution graph. Query: agent_run_id, tool_call_id, trace_id, pid, path, domain, risk_min, since, until."),
			"/agentsight/events": gin.H{
				"get":  gin.H{"summary": "AgentSight-compatible merged event export. Query: limit, format=json|array|jsonl, include_tls, source, pid, comm, event_type, type, trace_id, span_id, since, until, filter.", "responses": gin.H{"200": gin.H{"description": "OK"}}},
				"post": gin.H{"summary": "Import AgentSight JSON, JSON arrays, {\"events\":[...]}, or JSONL text into the compatibility store.", "responses": gin.H{"200": gin.H{"description": "OK"}}},
			},
			"/agentsight/events.jsonl":              endpointSpec("GET", "AgentSight-compatible JSONL export of merged retained events and TLS capture history."),
			"/agentsight/events/stats":              endpointSpec("GET", "AgentSight storage statistics by semantic source, event type, runner, and command."),
			"/agentsight/events/runners/{id}/stats": endpointSpec("GET", "AgentSight event statistics for one logical runner."),
			"/agentsight/events/query":              endpointSpec("POST", "Advanced AgentSight query with JSON body filters for sources, event types, PIDs, runner, time range, and text search."),
			"/agentsight/events/stream":             endpointSpec("GET", "Server-sent AgentSight-compatible stream for clients that cannot use WebSockets."),
			"/agentsight/runners":                   endpointSpec("GET", "List AgentSight logical runners and their status."),
			"/agentsight/stream/merged":             endpointSpec("GET", "Server-sent AgentSight-compatible merged stream from all logical runners."),
			"/agentsight/stream/runner/{id}":        endpointSpec("GET", "Server-sent AgentSight-compatible stream filtered to one logical runner."),
			"/network/flows":                        endpointSpec("GET", "Attributed TCP/UDP flow summaries. Query: filter, sort, showHistoric, limit, cursor, pid, domain, service, scope."),
			"/network/flows/{flowID}":               endpointSpec("GET", "One network flow by stable flow id."),
			"/network/dns-cache":                    endpointSpec("GET", "DNS correlation cache."),
			"/network/interfaces":                   endpointSpec("GET", "Per-interface RX/TX counters, packets, errors, and drops."),
			"/network/export/jsonl":                 endpointSpec("GET", "Metadata-only flow JSONL export."),
			"/sandbox/cgroup/status":                endpointSpec("GET", "cgroup/connect + sendmsg enforcement status, maps, counters, and active blocks."),
			"/sandbox/lsm/status":                   endpointSpec("GET", "BPF LSM enforcement status, maps, counters, and active blocks."),
			"/policies/network/block-ip":            endpointSpec("POST", "Block an IPv4/IPv6 destination through the cgroup sandbox. Body: {\"ip\":\"203.0.113.10\"}."),
			"/policies/network/unblock-ip":          endpointSpec("POST", "Remove an IP destination block. Body: {\"ip\":\"203.0.113.10\"}."),
			"/policies/network/block-port":          endpointSpec("POST", "Block a TCP/UDP destination port. Body: {\"port\":4444}."),
			"/policies/network/unblock-port":        endpointSpec("POST", "Remove a TCP/UDP destination-port block. Body: {\"port\":4444}."),
			"/policies/network/block-pid":           endpointSpec("POST", "Resolve a PID's cgroup v2 inode id and block outbound networking for that cgroup. Body: {\"pid\":1234}."),
			"/policies/network/unblock-pid":         endpointSpec("POST", "Resolve a PID's cgroup v2 inode id and remove its cgroup network block. Body: {\"pid\":1234}."),
			"/policies/lsm/block-exec-path":         endpointSpec("POST", "Block an executable path with BPF LSM. Body: {\"path\":\"/usr/bin/nc\"}."),
			"/policies/lsm/unblock-exec-path":       endpointSpec("POST", "Remove an executable-path block. Body: {\"path\":\"/usr/bin/nc\"}."),
			"/policies/lsm/block-exec-name":         endpointSpec("POST", "Block executable basename with BPF LSM. Body: {\"name\":\"nc\"}."),
			"/policies/lsm/unblock-exec-name":       endpointSpec("POST", "Remove executable-basename block. Body: {\"name\":\"nc\"}."),
			"/policies/lsm/block-file-name":         endpointSpec("POST", "Block file/directory basename for open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename. Body: {\"name\":\"id_rsa\"}."),
			"/policies/lsm/unblock-file-name":       endpointSpec("POST", "Remove file/directory basename block. Body: {\"name\":\"id_rsa\"}."),
			"/agents/register":                      endpointSpec("POST", "Register an agent PID and optional run/task/tool context. Body follows the root /register payload."),
			"/agents/unregister":                    endpointSpec("POST", "Unregister an agent PID. Body: {\"pid\":1234}."),
			"/config/export":                        endpointSpec("GET", "Export tags, tracked commands, tracked paths, wrapper rules, and runtime settings."),
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

func registerExternalAPIRoutes(rg *gin.RouterGroup, args ...any) {
	features := newFeatureRegistry()
	var tlsStore *TLSCaptureStore
	for _, arg := range args {
		switch value := arg.(type) {
		case *FeatureRegistry:
			features = value
		case *TLSCaptureStore:
			tlsStore = value
		}
	}
	rg.GET("/health", handleExternalAPIHealth)
	rg.GET("/openapi.json", handleExternalAPIOpenAPI)

	rg.GET("/events/recent", handleRecentEvents)
	rg.GET("/events/graph", handleExecutionGraph)
	if features.CompiledIn(FeatureAgentSight) {
		rg.GET("/agentsight/runners", handleAgentSightRunners(tlsStore))
		rg.GET("/agentsight/events", handleAgentSightEvents(tlsStore, false))
		rg.POST("/agentsight/events", handleAgentSightEventsUpload)
		rg.GET("/agentsight/events.jsonl", handleAgentSightEvents(tlsStore, true))
		rg.GET("/agentsight/events/stats", handleAgentSightEventsStats(tlsStore, ""))
		rg.GET("/agentsight/events/runners/:id/stats", handleAgentSightRunnerStats(tlsStore))
		rg.POST("/agentsight/events/query", handleAgentSightEventsQuery(tlsStore))
		rg.GET("/agentsight/events/stream", handleAgentSightEventsStream(tlsStore))
		rg.GET("/agentsight/stream/merged", handleAgentSightEventsStream(tlsStore))
		rg.GET("/agentsight/stream/runner/:id", handleAgentSightRunnerStream(tlsStore))
	}

	rg.GET("/network/flows", handleNetworkFlows)
	rg.GET("/network/flows/:flowID", handleNetworkFlowByID)
	rg.GET("/network/dns-cache", handleDNSCache)
	rg.GET("/network/interfaces", handleNetworkInterfaces)
	if features.CompiledIn(FeatureNetworkExport) {
		rg.GET("/network/export/jsonl", handleNetworkFlowJSONLExport)
	}

	if features.CompiledIn(FeatureSandboxCgroup) {
		rg.GET("/sandbox/cgroup/status", handleCgroupSandboxStatus)
	}
	if features.CompiledIn(FeatureSandboxLSM) {
		rg.GET("/sandbox/lsm/status", handleLsmEnforcerStatus)
	}

	policyMiddleware := policyManagementEnabledMiddleware()
	if !features.CompiledIn(FeaturePolicyManagement) {
		policyMiddleware = compiledOutFeatureMiddleware(FeaturePolicyManagement)
	}
	policies := rg.Group("/policies", policyMiddleware)
	{
		if features.CompiledIn(FeatureSandboxCgroup) {
			policies.POST("/network/block-ip", handleCgroupSandboxBlockIP)
			policies.POST("/network/unblock-ip", handleCgroupSandboxUnblockIP)
			policies.POST("/network/block-port", handleCgroupSandboxBlockPort)
			policies.POST("/network/unblock-port", handleCgroupSandboxUnblockPort)
			policies.POST("/network/block-pid", handleCgroupSandboxBlockPID)
			policies.POST("/network/unblock-pid", handleCgroupSandboxUnblockPID)
		}
		if features.CompiledIn(FeatureSandboxLSM) {
			policies.POST("/lsm/block-exec-path", handleLsmBlockExecPath)
			policies.POST("/lsm/unblock-exec-path", handleLsmUnblockExecPath)
			policies.POST("/lsm/block-exec-name", handleLsmBlockExecName)
			policies.POST("/lsm/unblock-exec-name", handleLsmUnblockExecName)
			policies.POST("/lsm/block-file-name", handleLsmBlockFileName)
			policies.POST("/lsm/unblock-file-name", handleLsmUnblockFileName)
		}
	}

	rg.POST("/agents/register", handleRegister)
	rg.POST("/agents/unregister", handleUnregister)
	rg.GET("/config/export", handleConfigExportGet)
}
