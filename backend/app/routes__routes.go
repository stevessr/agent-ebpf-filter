package app

import (
	codexhandlers "agent-ebpf-filter/codex/capture/handlers"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section routes.go ----

func registerRoutes(r *gin.Engine, features *FeatureRegistry, tlsBroadcaster *tlsCaptureBroadcaster, tlsController *TLSCaptureController, tlsStore *TLSCaptureStore, tlsRules *TLSCaptureRuleStore) {
	registerWebSocketRoutes(r, features, tlsBroadcaster)
	registerShellSessionRoutes(r, features)
	registerEventRoutes(r)
	registerNetworkRoutes(r, features)
	registerSandboxRoutes(r, features)
	registerUtilityRoutes(r, features)
	registerAuthenticatedAPIRoutes(r, features, tlsController, tlsStore, tlsRules, tlsBroadcaster)
	registerCompatibilityRoutes(r, features, tlsStore)
	registerStaticRoutes(r)
}

func registerWebSocketRoutes(r gin.IRouter, features *FeatureRegistry, tlsBroadcaster *tlsCaptureBroadcaster) {
	r.GET("/ws", authMiddleware(), serveEventsWS)
	r.GET("/ws/system", authMiddleware(), serveSystemStatsWS)
	r.GET("/ws/camera", authMiddleware(), serveCameraWS)
	r.GET("/ws/sensors", authMiddleware(), serveSensorsWS)
	r.GET("/ws/microphone", authMiddleware(), serveMicrophoneWS)
	if features.CompiledIn(FeatureML) {
		r.GET("/ws/ml-status", authMiddleware(), serveMLStatusWS)
	}
	r.GET("/ws/envelopes", authMiddleware(), serveEventEnvelopesWS)
	r.GET("/ws/events/graph", authMiddleware(), serveExecutionGraphWS)
	if features.CompiledIn(FeatureTLSCapture) {
		r.GET("/ws/tls-capture", authMiddleware(), func(c *gin.Context) { tlsBroadcaster.Serve(c) })
	}
}

func registerShellSessionRoutes(r gin.IRouter, features *FeatureRegistry) {
	if !features.CompiledIn(FeatureShellSessions) {
		return
	}
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

func registerNetworkRoutes(r gin.IRouter, features *FeatureRegistry) {
	r.GET("/network/flows", authMiddleware(), handleNetworkFlows)
	r.GET("/network/flows/:flowID", authMiddleware(), handleNetworkFlowByID)
	r.GET("/network/tcp-state", authMiddleware(), handleTCPState)
	r.GET("/network/analyze", authMiddleware(), handleNetworkAnalyze)
	r.GET("/network/dns-lookup", authMiddleware(), handleDNSLookup)
	r.GET("/network/dns-cache", authMiddleware(), handleDNSCache)
	r.GET("/network/interfaces", authMiddleware(), handleNetworkInterfaces)
	if features.CompiledIn(FeatureNetworkExport) {
		r.GET("/network/export/jsonl", authMiddleware(), handleNetworkFlowJSONLExport)
		r.POST("/network/export-pcap", authMiddleware(), handlePCAPExport)
	}
	r.GET("/network/geoip", authMiddleware(), handleGeoIPLookup)
}

func registerSandboxRoutes(r gin.IRouter, features *FeatureRegistry) {
	if features.CompiledIn(FeatureSandboxCgroup) {
		r.GET("/sandbox/cgroup/status", authMiddleware(), handleCgroupSandboxStatus)
		if features.CompiledIn(FeaturePolicyManagement) {
			r.POST("/sandbox/cgroup/block-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockCgroup)
			r.POST("/sandbox/cgroup/unblock-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockCgroup)
			r.POST("/sandbox/cgroup/block-pid", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockPID)
			r.POST("/sandbox/cgroup/unblock-pid", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockPID)
			r.POST("/sandbox/cgroup/block-ip", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockIP)
			r.POST("/sandbox/cgroup/unblock-ip", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockIP)
			r.POST("/sandbox/cgroup/block-port", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockPort)
			r.POST("/sandbox/cgroup/unblock-port", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockPort)
		} else {
			r.POST("/sandbox/cgroup/block-cgroup", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/unblock-cgroup", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/block-pid", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/unblock-pid", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/block-ip", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/unblock-ip", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/block-port", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/cgroup/unblock-port", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
		}
	}
	if features.CompiledIn(FeatureSandboxLSM) {
		r.GET("/sandbox/lsm/status", authMiddleware(), handleLsmEnforcerStatus)
		if features.CompiledIn(FeaturePolicyManagement) {
			r.POST("/sandbox/lsm/block-exec-path", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockExecPath)
			r.POST("/sandbox/lsm/unblock-exec-path", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockExecPath)
			r.POST("/sandbox/lsm/block-exec-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockExecName)
			r.POST("/sandbox/lsm/unblock-exec-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockExecName)
			r.POST("/sandbox/lsm/block-file-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmBlockFileName)
			r.POST("/sandbox/lsm/unblock-file-name", authMiddleware(), policyManagementEnabledMiddleware(), handleLsmUnblockFileName)
		} else {
			r.POST("/sandbox/lsm/block-exec-path", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/lsm/unblock-exec-path", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/lsm/block-exec-name", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/lsm/unblock-exec-name", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/lsm/block-file-name", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
			r.POST("/sandbox/lsm/unblock-file-name", authMiddleware(), compiledOutFeatureMiddleware(FeaturePolicyManagement))
		}
	}
}

func registerUtilityRoutes(r gin.IRouter, features *FeatureRegistry) {
	r.GET("/metrics", authMiddleware(), handlePrometheusMetrics)
	if features.CompiledIn(FeatureHooks) {
		r.POST("/hooks/event", hookIngressAuthMiddleware(), handleNativeHookEvent)
	}
	r.POST("/register", authMiddleware(), handleRegister)
	r.POST("/unregister", authMiddleware(), handleUnregister)
	r.POST("/cluster/heartbeat", clusterHeartbeatHandler)
	r.POST("/cluster/register", clusterHeartbeatHandler)
}

func registerAuthenticatedAPIRoutes(r *gin.Engine, features *FeatureRegistry, tlsController *TLSCaptureController, tlsStore *TLSCaptureStore, tlsRules *TLSCaptureRuleStore, tlsBroadcaster *tlsCaptureBroadcaster) {
	api := r.Group("/", authMiddleware())
	{
		registerConfigRoutes(api.Group("/config"), features)
		registerSystemRoutes(api.Group("/system"), features)
		if features.CompiledIn(FeatureTLSCapture) {
			registerTLSCaptureRoutes(api, tlsController, tlsStore, tlsRules)
			codexhandlers.RegisterRoutes(api, codexCaptureSink{store: tlsStore, broadcaster: tlsBroadcaster})
		}
		if features.CompiledIn(FeatureAgentSight) {
			registerAgentSightRoutes(api, tlsStore)
		}
		if features.CompiledIn(FeaturePlugins) {
			registerPluginRoutes(api.Group("/plugins"))
		}

		data := api.Group("/data")
		{
			data.POST("/clear-events", handleClearEvents)
			data.POST("/clear-events-memory", handleClearEventsMemory)
			data.POST("/clear-events-persisted", handleClearEventsPersisted)
		}
		if features.CompiledIn(FeatureShellSessions) {
			api.POST("/shell-sessions/cleanup", shellSessionsEnabledMiddleware(), handleShellSessionsCleanup)
		}
		api.Any("/mcp", gin.WrapH(buildMCPHandler()))
		cluster := api.Group("/cluster")
		{
			cluster.GET("/state", clusterStateHandler)
			cluster.GET("/nodes", clusterNodesHandler)
		}
	}
}

func registerCompatibilityRoutes(r *gin.Engine, features *FeatureRegistry, tlsStore *TLSCaptureStore) {
	if features.CompiledIn(FeatureAgentSight) {
		registerAgentSightCompatibilityRoutes(r.Group("/api", authMiddleware()), tlsStore)
	}
	registerExternalAPIRoutes(r.Group("/api/v1", authMiddleware()), features, tlsStore)
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
