package main

import (
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func registerRoutes(r *gin.Engine, tlsBroadcaster *tlsCaptureBroadcaster, tlsManager tlsGoBinaryRegistrar, tlsStore *TLSCaptureStore, tlsRules *TLSCaptureRuleStore) {
	registerWebSocketRoutes(r, tlsBroadcaster)
	registerShellSessionRoutes(r)
	registerEventRoutes(r)
	registerNetworkRoutes(r)
	registerSandboxRoutes(r)
	registerUtilityRoutes(r)
	registerAuthenticatedAPIRoutes(r, tlsManager, tlsStore, tlsRules)
	registerCompatibilityRoutes(r, tlsStore)
	registerStaticRoutes(r)
}

func registerWebSocketRoutes(r gin.IRouter, tlsBroadcaster *tlsCaptureBroadcaster) {
	r.GET("/ws", authMiddleware(), serveEventsWS)
	r.GET("/ws/system", authMiddleware(), serveSystemStatsWS)
	r.GET("/ws/camera", authMiddleware(), serveCameraWS)
	r.GET("/ws/sensors", authMiddleware(), serveSensorsWS)
	r.GET("/ws/microphone", authMiddleware(), serveMicrophoneWS)
	r.GET("/ws/ml-status", authMiddleware(), serveMLStatusWS)
	r.GET("/ws/envelopes", authMiddleware(), serveEventEnvelopesWS)
	r.GET("/ws/events/graph", authMiddleware(), serveExecutionGraphWS)
	r.GET("/ws/tls-capture", authMiddleware(), func(c *gin.Context) { tlsBroadcaster.Serve(c) })
}

func registerShellSessionRoutes(r gin.IRouter) {
	r.POST("/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), handleCreateShellSession)
	r.GET("/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), handleListShellSessions)
	r.DELETE("/shell-sessions/:id", authMiddleware(), shellSessionsEnabledMiddleware(), handleDeleteShellSession)
	r.POST("/shell-sessions/:id/input", authMiddleware(), shellSessionsEnabledMiddleware(), handleSendShellSessionInput)
	r.GET("/ws/shell", authMiddleware(), shellSessionsEnabledMiddleware(), serveShellWS)
	r.GET("/ws/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), serveShellSessionsWS)
}

func registerEventRoutes(r gin.IRouter) {
	r.GET("/events/recent", authMiddleware(), handleRecentEvents)
	r.GET("/events/graph", authMiddleware(), handleExecutionGraph)
	r.GET("/events/recording", authMiddleware(), handleEventRecordingStatus)
	r.POST("/events/recording/start", authMiddleware(), handleStartEventRecording)
	r.POST("/events/recording/stop", authMiddleware(), handleStopEventRecording)
	r.POST("/events/recording/replay", authMiddleware(), handleReplayEventRecording)
	r.POST("/events/recording/browser/save", authMiddleware(), handleSaveBrowserRecording)
}

func registerNetworkRoutes(r gin.IRouter) {
	r.GET("/network/flows", authMiddleware(), handleNetworkFlows)
	r.GET("/network/flows/:flowID", authMiddleware(), handleNetworkFlowByID)
	r.GET("/network/tcp-state", authMiddleware(), handleTCPState)
	r.GET("/network/analyze", authMiddleware(), handleNetworkAnalyze)
	r.GET("/network/dns-lookup", authMiddleware(), handleDNSLookup)
	r.GET("/network/dns-cache", authMiddleware(), handleDNSCache)
	r.GET("/network/interfaces", authMiddleware(), handleNetworkInterfaces)
	r.GET("/network/export/jsonl", authMiddleware(), handleNetworkFlowJSONLExport)
	r.POST("/network/export-pcap", authMiddleware(), handlePCAPExport)
	r.GET("/network/geoip", authMiddleware(), handleGeoIPLookup)
}

func registerSandboxRoutes(r gin.IRouter) {
	r.GET("/sandbox/cgroup/status", authMiddleware(), handleCgroupSandboxStatus)
	r.POST("/sandbox/cgroup/block-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockCgroup)
	r.POST("/sandbox/cgroup/unblock-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockCgroup)
	r.POST("/sandbox/cgroup/block-pid", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockPID)
	r.POST("/sandbox/cgroup/unblock-pid", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockPID)
	r.POST("/sandbox/cgroup/block-ip", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockIP)
	r.POST("/sandbox/cgroup/unblock-ip", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockIP)
	r.POST("/sandbox/cgroup/block-port", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockPort)
	r.POST("/sandbox/cgroup/unblock-port", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockPort)
	r.GET("/sandbox/lsm/status", authMiddleware(), handleLsmEnforcerStatus)
	r.POST("/sandbox/lsm/block-exec-path", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockExecPath)
	r.POST("/sandbox/lsm/unblock-exec-path", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockExecPath)
	r.POST("/sandbox/lsm/block-exec-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockExecName)
	r.POST("/sandbox/lsm/unblock-exec-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockExecName)
	r.POST("/sandbox/lsm/block-file-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockFileName)
	r.POST("/sandbox/lsm/unblock-file-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockFileName)
}

func registerUtilityRoutes(r gin.IRouter) {
	r.GET("/metrics", authMiddleware(), handlePrometheusMetrics)
	r.POST("/hooks/event", hookIngressAuthMiddleware(), handleNativeHookEvent)
	r.POST("/register", authMiddleware(), handleRegister)
	r.POST("/unregister", authMiddleware(), handleUnregister)
	r.POST("/cluster/heartbeat", clusterHeartbeatHandler)
	r.POST("/cluster/register", clusterHeartbeatHandler)
}

func registerAuthenticatedAPIRoutes(r *gin.Engine, tlsManager tlsGoBinaryRegistrar, tlsStore *TLSCaptureStore, tlsRules *TLSCaptureRuleStore) {
	api := r.Group("/", authMiddleware())
	{
		registerConfigRoutes(api.Group("/config"))
		registerSystemRoutes(api.Group("/system"))
		registerTLSCaptureRoutes(api, tlsManager, tlsStore, tlsRules)
		registerAgentSightRoutes(api, tlsStore)
		registerPluginRoutes(api.Group("/plugins"))

		data := api.Group("/data")
		{
			data.POST("/clear-events", handleClearEvents)
			data.POST("/clear-events-memory", handleClearEventsMemory)
			data.POST("/clear-events-persisted", handleClearEventsPersisted)
		}
		api.POST("/shell-sessions/cleanup", shellSessionsEnabledMiddleware(), handleShellSessionsCleanup)
		api.Any("/mcp", gin.WrapH(buildMCPHandler()))
		cluster := api.Group("/cluster")
		{
			cluster.GET("/state", clusterStateHandler)
			cluster.GET("/nodes", clusterNodesHandler)
		}
	}
}

func registerCompatibilityRoutes(r *gin.Engine, tlsStore *TLSCaptureStore) {
	registerAgentSightCompatibilityRoutes(r.Group("/api", authMiddleware()), tlsStore)
	registerExternalAPIRoutes(r.Group("/api/v1", authMiddleware()), tlsStore)
}

func registerStaticRoutes(r *gin.Engine) {
	staticDir := "../frontend/dist"
	if _, err := os.Stat(staticDir); err != nil {
		staticDir = "./frontend/dist"
	}
	r.StaticFile("/", filepath.Join(staticDir, "index.html"))
	r.Static("/assets", filepath.Join(staticDir, "assets"))
	r.NoRoute(func(c *gin.Context) { c.File(filepath.Join(staticDir, "index.html")) })
}
