package app

import (
	"github.com/gin-gonic/gin"
)

// Handler functions moved to app/handlers/api_external.go.
// Helper functions moved to app/handlers/api_external.go (BuildExternalOpenAPISpec, etc.).
// Bridge functions in handlersbridge.go delegate to them.

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
