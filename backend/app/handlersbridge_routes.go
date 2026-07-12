package app

import (
	"agent-ebpf-filter/app/handlers"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
)

// ── Bridge functions (delegate to handlers/ subpackage) ─────────

func handleClearEvents(c *gin.Context)          { handlers.HandleClearEvents(c) }
func handleClearEventsMemory(c *gin.Context)    { handlers.HandleClearEventsMemory(c) }
func handleClearEventsPersisted(c *gin.Context) { handlers.HandleClearEventsPersisted(c) }

func handleRunBenchmark(c *gin.Context)        { handlers.HandleRunBenchmark(c) }
func handleGetBenchmarkResults(c *gin.Context) { handlers.HandleGetBenchmarkResults(c) }

func handlePluginsList(c *gin.Context)  { handlers.HandlePluginsList(c) }
func handlePluginGet(c *gin.Context)    { handlers.HandlePluginGet(c) }
func handlePluginUpsert(c *gin.Context) { handlers.HandlePluginUpsert(c) }
func handlePluginDelete(c *gin.Context) { handlers.HandlePluginDelete(c) }
func handlePluginToggle(c *gin.Context) { handlers.HandlePluginToggle(c) }
func handleBPFTemplates(c *gin.Context) { handlers.HandleBPFTemplates(c) }
func handleBPFCompile(c *gin.Context)   { handlers.HandleBPFCompile(c) }
func handleBPFLoad(c *gin.Context)      { handlers.HandleBPFLoad(c) }
func handleBPFUnload(c *gin.Context)    { handlers.HandleBPFUnload(c) }

func handleRegister(c *gin.Context)   { handlers.HandleRegister(c) }
func handleUnregister(c *gin.Context) { handlers.HandleUnregister(c) }

func registerPluginRoutes(rg *gin.RouterGroup) {
	handlers.RegisterPluginRoutes(rg)
}

// System handler bridges
func handleSystemLs(c *gin.Context)          { handlers.HandleSystemLs(c) }
func handleFilePreview(c *gin.Context)       { handlers.HandleFilePreview(c) }
func handleFilePreviewStream(c *gin.Context) { handlers.HandleFilePreviewStream(c) }
func handleFileHex(c *gin.Context)           { handlers.HandleFileHex(c) }
func handleFileELF(c *gin.Context)           { handlers.HandleFileELF(c) }
func handleSystemHome(c *gin.Context)        { handlers.HandleSystemHome(c) }
func handleDownload(c *gin.Context)          { handlers.HandleDownload(c) }
func handleUpload(c *gin.Context)            { handlers.HandleUpload(c) }
func handleRun(c *gin.Context)               { handlers.HandleRun(c) }
func handleSystemdServices(c *gin.Context)   { handlers.HandleSystemdServices(c) }
func handleSystemdControl(c *gin.Context)    { handlers.HandleSystemdControl(c) }
func handleSystemdLogs(c *gin.Context)       { handlers.HandleSystemdLogs(c) }
func handleTrackedComms(c *gin.Context)      { handlers.HandleTrackedComms(c) }
func handleProcessSignal(c *gin.Context)     { handlers.HandleProcessSignal(c) }
func handleProcessMaps(c *gin.Context)       { handlers.HandleProcessMaps(c) }

// Config handler bridges
func handleConfigTagsGet(c *gin.Context)          { handlers.HandleConfigTagsGet(c) }
func handleConfigTagsPost(c *gin.Context)         { handlers.HandleConfigTagsPost(c) }
func handleConfigCommsGet(c *gin.Context)         { handlers.HandleConfigCommsGet(c) }
func handleConfigCommsPost(c *gin.Context)        { handlers.HandleConfigCommsPost(c) }
func handleConfigCommsDelete(c *gin.Context)      { handlers.HandleConfigCommsDelete(c) }
func handleConfigCommsDisable(c *gin.Context)     { handlers.HandleConfigCommsDisable(c) }
func handleConfigCommsEnable(c *gin.Context)      { handlers.HandleConfigCommsEnable(c) }
func handleConfigEventTypesGet(c *gin.Context)    { handlers.HandleConfigEventTypesGet(c) }
func handleConfigEventTypeDisable(c *gin.Context) { handlers.HandleConfigEventTypeDisable(c) }
func handleConfigEventTypeEnable(c *gin.Context)  { handlers.HandleConfigEventTypeEnable(c) }
func handleConfigPathsGet(c *gin.Context)         { handlers.HandleConfigPathsGet(c) }
func handleConfigPathsPost(c *gin.Context)        { handlers.HandleConfigPathsPost(c) }
func handleConfigPathsDelete(c *gin.Context)      { handlers.HandleConfigPathsDelete(c) }
func handleConfigPrefixesGet(c *gin.Context)      { handlers.HandleConfigPrefixesGet(c) }
func handleConfigPrefixesPost(c *gin.Context)     { handlers.HandleConfigPrefixesPost(c) }
func handleConfigPrefixesDelete(c *gin.Context)   { handlers.HandleConfigPrefixesDelete(c) }
func handleConfigRulesGet(c *gin.Context)         { handlers.HandleConfigRulesGet(c) }
func handleConfigRulesPost(c *gin.Context)        { handlers.HandleConfigRulesPost(c) }
func handleConfigRulesDelete(c *gin.Context)      { handlers.HandleConfigRulesDelete(c) }
func handleMLStatusGet(c *gin.Context)            { handlers.HandleMLStatusGet(c) }
func handleMLLogsGet(c *gin.Context)              { handlers.HandleMLLogsGet(c) }
func handleMLTrainCancelPost(c *gin.Context)      { handlers.HandleMLTrainCancelPost(c) }
func handleMLHistoryGet(c *gin.Context)           { handlers.HandleMLHistoryGet(c) }
func handleMLTrainPost(c *gin.Context)            { handlers.HandleMLTrainPost(c) }
func handleMLFeedbackPost(c *gin.Context)         { handlers.HandleMLFeedbackPost(c) }
func handleMLSamplesGet(c *gin.Context)           { handlers.HandleMLSamplesGet(c) }
func handleMLSampleLabelPut(c *gin.Context)       { handlers.HandleMLSampleLabelPut(c) }
func handleMLSampleDelete(c *gin.Context)         { handlers.HandleMLSampleDelete(c) }
func handleMLSampleAnomalyPut(c *gin.Context)     { handlers.HandleMLSampleAnomalyPut(c) }
func handleMLSamplesPost(c *gin.Context)          { handlers.HandleMLSamplesPost(c) }
func handleMLTunePost(c *gin.Context)             { autotuneTunePost(c) }
func handleMLTuneModelsPost(c *gin.Context)       { autotuneTuneModelsPost(c) }
func handleMLBacktestPost(c *gin.Context)         { handlers.HandleMLBacktestPost(c) }

// Command safety bridges — delegate to fat-bridge Deps closures
func handleMLAssessPost(c *gin.Context)          { handlers.Deps.MLAssessCommandSafety(c) }
func handleMLExistingCommandsGet(c *gin.Context) { handlers.Deps.MLExistingCommandsGetFn(c) }
func handleMLImportExistingPost(c *gin.Context)  { handlers.Deps.MLImportExistingFn(c) }
func handleConfigExportGet(c *gin.Context)       { handlers.HandleConfigExportGet(c) }
func handleConfigImportPost(c *gin.Context)      { handlers.HandleConfigImportPost(c) }
func handleConfigRuntimeGet(c *gin.Context)      { handlers.HandleConfigRuntimeGet(c) }
func handleConfigRuntimePut(c *gin.Context)      { handlers.HandleConfigRuntimePut(c) }
func handleConfigAccessTokenPost(c *gin.Context) { handlers.HandleConfigAccessTokenPost(c) }

// handleConfigRedactionPolicyGet/Put are defined in handlersredaction.go (app package)

func registerSystemRoutes(rg *gin.RouterGroup, registries ...*FeatureRegistry) {
	syncHandlerDeps()
	features := newFeatureRegistry()
	if len(registries) > 0 && registries[0] != nil {
		features = registries[0]
	}
	if features.CompiledIn(FeatureSystemRun) {
		handlers.RegisterSystemRoutes(rg, systemRunEnabledMiddleware())
		handlers.RegisterSystemRunRoute(rg, systemRunEnabledMiddleware())
	} else {
		handlers.RegisterSystemRoutes(rg, compiledOutFeatureMiddleware(FeatureSystemRun))
		handlers.RegisterSystemRunRoute(rg, compiledOutFeatureMiddleware(FeatureSystemRun))
	}
	rg.GET("/features", handleSystemFeatures)
	rg.GET("/bootstrap-health", handleBootstrapHealth)
	rg.GET("/collector-health", handleCollectorHealth)
	rg.GET("/otel-health", handleOTelHealth)
	rg.GET("/domain-forward/status", handleDomainForwardProxyStatus)
	rg.GET("/loop-detection/status", handleLoopDetectionStatus)
	rg.POST("/loop-detection/task", handleLoopDetectionTask)
	rg.GET("/research-processing/status", handleResearchProcessingStatus)
	rg.POST("/research-processing/task", handleResearchProcessingTask)
	rg.GET("/signals/status", handleSignalProcessingStatus)
	rg.POST("/signals/task", handleSignalProcessingTask)
	rg.POST("/signals/rules/test", handleSignalRuleTest)
	rg.GET("/signals/program-logs", handleSignalProgramLogs)
	rg.GET("/signals/program-logs/download", handleSignalProgramLogDownload)
}

// Hardware handler bridges (camera, microphone, sensors)
func handleSensors(c *gin.Context)        { handlers.HandleSensors(c) }
func handleCameras(c *gin.Context)        { handlers.HandleCameras(c) }
func handleCameraSnapshot(c *gin.Context) { handlers.HandleCameraSnapshot(c) }
func handleMicrophones(c *gin.Context)    { handlers.HandleMicrophones(c) }
func serveCameraWS(c *gin.Context)        { handlers.ServeCameraWS(c) }
func serveSensorsWS(c *gin.Context)       { handlers.ServeSensorsWS(c) }
func serveMicrophoneWS(c *gin.Context)    { handlers.ServeMicrophoneWS(c) }

// System stats bridge
func serveSystemStatsWS(c *gin.Context) { handlers.ServeSystemStatsWS(c) }

// Hooks config bridges
func handleConfigHooksList(c *gin.Context)    { handlers.HandleConfigHooksList(c) }
func handleConfigHooksInstall(c *gin.Context) { handlers.HandleConfigHooksInstall(c) }
func handleConfigHooksRawGet(c *gin.Context)  { handlers.HandleConfigHooksRawGet(c) }
func handleConfigHooksRawPost(c *gin.Context) { handlers.HandleConfigHooksRawPost(c) }

// Network enrichment bridges
func syncHandlerDeps() {
	handlers.Deps.RuntimeSettings = runtimeSettingsStore
	handlers.Deps.EventArchiveClear = capturedEventArchive.Clear
	handlers.Deps.AgentSightEventsClear = agentSightUploadedEvents.Clear
	handlers.Deps.AgentSightUploadedEvents = &agentSightStoreAdapter{store: agentSightUploadedEvents}
}

func handleNetworkFlows(c *gin.Context) {
	handlers.HandleNetworkFlows(c)
}
func handleNetworkFlowByID(c *gin.Context) {
	handlers.HandleNetworkFlowByID(c)
}
func handleTCPState(c *gin.Context)       { handlers.HandleTCPState(c) }
func handleNetworkAnalyze(c *gin.Context) { handlers.HandleNetworkAnalyze(c) }
func handleGeoIPLookup(c *gin.Context)    { handlers.HandleGeoIPLookup(c) }
func handleDNSLookup(c *gin.Context) {
	handlers.HandleDNSLookup(c)
}
func handleDNSCache(c *gin.Context) {
	handlers.HandleDNSCache(c)
}
func handleNetworkInterfaces(c *gin.Context) { handlers.HandleNetworkInterfaces(c) }
func handleNetworkFlowJSONLExport(c *gin.Context) {
	handlers.HandleNetworkFlowJSONLExport(c)
}

// External API bridges
func handleExternalAPIHealth(c *gin.Context) {
	syncHandlerDeps()
	handlers.HandleExternalAPIHealth(c)
}
func handleExternalAPIOpenAPI(c *gin.Context) { handlers.HandleExternalAPIOpenAPI(c) }
func buildExternalOpenAPISpec() *openapi3.T   { return handlers.BuildExternalOpenAPISpec() }

// LSM enforcer bridges
func handleLsmEnforcerStatus(c *gin.Context)  { handlers.HandleLsmEnforcerStatus(c) }
func handleLsmBlockExecPath(c *gin.Context)   { handlers.HandleLsmBlockExecPath(c) }
func handleLsmUnblockExecPath(c *gin.Context) { handlers.HandleLsmUnblockExecPath(c) }
func handleLsmBlockExecName(c *gin.Context)   { handlers.HandleLsmBlockExecName(c) }
func handleLsmUnblockExecName(c *gin.Context) { handlers.HandleLsmUnblockExecName(c) }
func handleLsmBlockFileName(c *gin.Context)   { handlers.HandleLsmBlockFileName(c) }
func handleLsmUnblockFileName(c *gin.Context) { handlers.HandleLsmUnblockFileName(c) }

// Cgroup sandbox bridges
func handleCgroupSandboxStatus(c *gin.Context)        { handlers.HandleCgroupSandboxStatus(c) }
func handleCgroupSandboxBlockCgroup(c *gin.Context)   { handlers.HandleCgroupSandboxBlockCgroup(c) }
func handleCgroupSandboxUnblockCgroup(c *gin.Context) { handlers.HandleCgroupSandboxUnblockCgroup(c) }
func handleCgroupSandboxBlockPID(c *gin.Context)      { handlers.HandleCgroupSandboxBlockPID(c) }
func handleCgroupSandboxUnblockPID(c *gin.Context)    { handlers.HandleCgroupSandboxUnblockPID(c) }
func handleCgroupSandboxBlockIP(c *gin.Context)       { handlers.HandleCgroupSandboxBlockIP(c) }
func handleCgroupSandboxUnblockIP(c *gin.Context)     { handlers.HandleCgroupSandboxUnblockIP(c) }
func handleCgroupSandboxBlockPort(c *gin.Context)     { handlers.HandleCgroupSandboxBlockPort(c) }
func handleCgroupSandboxUnblockPort(c *gin.Context)   { handlers.HandleCgroupSandboxUnblockPort(c) }

// Native hook bridge
func handleNativeHookEvent(c *gin.Context) { handlers.HandleNativeHookEvent(c) }
func extractNativeHookPath(toolInput map[string]interface{}) string {
	return handlers.ExtractNativeHookPath(toolInput)
}
func buildNativeHookExtraInfo(payload map[string]interface{}, hookEvent, toolName string) string {
	return handlers.BuildNativeHookExtraInfo(payload, hookEvent, toolName)
}
func digestHookText(text string) string { return handlers.DigestHookText(text) }

// ML WebSocket bridge
func serveMLStatusWS(c *gin.Context) { handlers.ServeMLStatusWS(c) }

// Feature manifest bridge
func handleSystemFeatures(c *gin.Context) { handlers.HandleSystemFeatures(c) }

// Shell session bridges
func serveShellWS(c *gin.Context)                { handlers.ServeShellWS(c) }
func serveShellSessionsWS(c *gin.Context)        { handlers.ServeShellSessionsWS(c) }
func handleCreateShellSession(c *gin.Context)    { handlers.HandleCreateShellSession(c) }
func handleListShellSessions(c *gin.Context)     { handlers.HandleListShellSessions(c) }
func handleDeleteShellSession(c *gin.Context)    { handlers.HandleDeleteShellSession(c) }
func handleSendShellSessionInput(c *gin.Context) { handlers.HandleSendShellSessionInput(c) }
func handleShellSessionsCleanup(c *gin.Context)  { handlers.HandleShellSessionsCleanup(c) }

// AgentSight bridges
func handleAgentSightEvents(tlsStore *TLSCaptureStore, forceJSONL bool) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightEvents(tlsStore, forceJSONL)
}
func handleAgentSightEventsQuery(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightEventsQuery(tlsStore)
}
func handleAgentSightEventsStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightEventsStream(tlsStore)
}
func handleAgentSightRunnerStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightRunnerStream(tlsStore)
}
func handleAgentSightEventsUpload(c *gin.Context) {
	syncHandlerDeps()
	handlers.HandleAgentSightEventsUpload(c)
}
func handleAgentSightRunners(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightRunners(tlsStore)
}
func handleAgentSightEventsStats(tlsStore *TLSCaptureStore, runnerID string) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightEventsStats(tlsStore, runnerID)
}
func handleAgentSightRunnerStats(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	syncHandlerDeps()
	return handlers.HandleAgentSightRunnerStats(tlsStore)
}
