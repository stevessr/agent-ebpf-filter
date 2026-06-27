package app

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

// ── TrackerMaps adapter ─────────────────────────────────────────────────────

// handlerTrackerMapsAdapter wraps a *trackerMapSet to implement the
// handlers.TrackerMaps interface by forwarding map operations.
type handlerTrackerMapsAdapter struct {
	set *trackerMapSet
}

func (a *handlerTrackerMapsAdapter) AgentPidsPut(key, value any) error {
	return a.set.AgentPids.Put(key, value)
}

func (a *handlerTrackerMapsAdapter) AgentPidsDelete(key any) error {
	return a.set.AgentPids.Delete(key)
}

func (a *handlerTrackerMapsAdapter) TrackedCommsIterate() *ebpf.MapIterator {
	return a.set.TrackedComms.Iterate()
}

func (a *handlerTrackerMapsAdapter) TrackedCommsPut(key, value any) error {
	return a.set.TrackedComms.Put(key, value)
}

func (a *handlerTrackerMapsAdapter) TrackedCommsDelete(key any) error {
	return a.set.TrackedComms.Delete(key)
}

func (a *handlerTrackerMapsAdapter) TrackedPathsIterate() *ebpf.MapIterator {
	return a.set.TrackedPaths.Iterate()
}

func (a *handlerTrackerMapsAdapter) TrackedPathsPut(key, value any) error {
	return a.set.TrackedPaths.Put(key, value)
}

func (a *handlerTrackerMapsAdapter) TrackedPathsDelete(key any) error {
	return a.set.TrackedPaths.Delete(key)
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesIterate() *ebpf.MapIterator {
	return a.set.TrackedPrefixes.Iterate()
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesPut(key, value any) error {
	return a.set.TrackedPrefixes.Put(key, value)
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesDelete(key any) error {
	return a.set.TrackedPrefixes.Delete(key)
}

func (a *handlerTrackerMapsAdapter) CollectorStats() *ebpf.Map {
	return a.set.CollectorStats
}

// ── Handlers Dependency wiring ─────────────────────────────────────

func init() {
	// Tracker maps
	handlers.Deps.TrackerMaps = &handlerTrackerMapsAdapter{set: &trackerMaps}
	handlers.Deps.GetTagID = getTagID

	// Process context
	handlers.Deps.ProcessContexts = trackedProcessContexts

	// Event archive / data handlers
	handlers.Deps.EventArchiveClear = capturedEventArchive.Clear
	handlers.Deps.AgentSightEventsClear = agentSightUploadedEvents.Clear
	handlers.Deps.RuntimeSettingsTruncateLog = runtimeSettingsStore.TruncateEventLog

	handlers.Deps.PluginValidateID = validatePluginID
	handlers.Deps.PluginSource = func(id string) (string, bool) { s, err := PluginSource(id); return s, err == nil }
	handlers.Deps.PluginUnloadEBPF = UnloadEBPFPlugin
	handlers.Deps.CompileUserBPF = CompileUserBPF
	handlers.Deps.BPFTemplates = func() []any {
		templates := bpfTemplates()
		result := make([]any, len(templates))
		for i, t := range templates {
			result[i] = t
		}
		return result
	}
	handlers.Deps.PluginList = func() []any {
		list := pluginRegistry.List()
		result := make([]any, len(list))
		for i, v := range list {
			result[i] = v
		}
		return result
	}
	handlers.Deps.PluginGet = func(id string) (any, bool) { return pluginRegistry.Get(id) }
	handlers.Deps.PluginUpsert = func(manifest any) error {
		m, ok := manifest.(*PluginManifest)
		if !ok {
			return fmt.Errorf("expected *PluginManifest, got %T", manifest)
		}
		return pluginRegistry.Upsert(m)
	}
	handlers.Deps.PluginDelete = func(id string) error { return pluginRegistry.Delete(id) }
	handlers.Deps.PluginSetEnabled = func(id string, enabled bool) (any, error) { return pluginRegistry.SetEnabled(id, enabled) }

	// System / platform handlers
	handlers.Deps.GetRealHomeDir = platform.GetRealHomeDir
	handlers.Deps.ResolveWrapperPath = platform.ResolveWrapperPath
	handlers.Deps.DropPrivileges = dropPrivileges
	handlers.Deps.ConfigureCommandForRealUser = configureCommandForRealUser
	handlers.Deps.OriginalInvokerIDs = platform.OriginalInvokerIDs
	handlers.Deps.BuildFilePreview = func(path string) (any, error) { return buildFilePreview(path) }

	// Hardware handlers (camera, microphone, sensors)
	handlers.Deps.Upgrader = &upgrader
	handlers.Deps.WriteProtoOrJSON = writeProtoOrJSON
	handlers.Deps.GetCameraStream = func(devName string) *handlers.CameraStream {
		cs := getCameraStream(devName)
		return &handlers.CameraStream{
			SubscribeFn: func() handlers.CameraSubscriber { return cs.Subscribe() },
		}
	}

	// Stats / observability (system stats WS)
	handlers.Deps.GetGPUMetrics = getGPUMetrics
	handlers.Deps.ReadVMFaultCounters = readVMFaultCounters
	handlers.Deps.VMFaultCountersZero = func() handlers.VmFaultCounters { return vmFaultCounters{} }
	handlers.Deps.GetCoreTypes = getCoreTypes
	handlers.Deps.GetZramStats = getZramStats
	handlers.Deps.BroadcastCh = broadcast
	handlers.Deps.EventSchemaVersion = eventSchemaVersion
	handlers.Deps.SendTLSBridge = tls.SendTLSBridge

	// Export config
	handlers.Deps.RuntimeSettingsReplace = runtimeSettingsStore.Replace
	handlers.Deps.ApplyRetentionConfig = applyRetentionConfig
	handlers.Deps.ApplyRuntimeDomainForwardProxy = applyRuntimeDomainForwardProxy
	handlers.Deps.BuildRuntimeConfigResponse = buildRuntimeConfigResponse
	handlers.Deps.BuildRuntimeConfigResponseFromSettings = buildRuntimeConfigResponseFromSettings
	handlers.Deps.RotateAccessToken = func(s RuntimeSettings) RuntimeSettings {
		settings, _ := runtimeSettingsStore.RotateAccessToken()
		return settings
	}
	handlers.Deps.ApplyMLConfigPatch = func(dst *core.MLConfig, patch interface{}) {
		if p, ok := patch.(handlers.MLConfigPatch); ok {
			// applyMLConfigPatch was defined in app/handlersruntimeconfig.go
			// and is now available through the MLConfigPatch dep
			if p.Enabled != nil {
				dst.Enabled = *p.Enabled
			}
			if p.BlockConfidenceThreshold != nil {
				dst.BlockConfidenceThreshold = *p.BlockConfidenceThreshold
			}
			if p.MlMinConfidence != nil {
				dst.MlMinConfidence = *p.MlMinConfidence
			}
			if p.LowAnomalyThreshold != nil {
				dst.LowAnomalyThreshold = *p.LowAnomalyThreshold
			}
			if p.HighAnomalyThreshold != nil {
				dst.HighAnomalyThreshold = *p.HighAnomalyThreshold
			}
			if p.RuleOverridePriority != nil {
				dst.RuleOverridePriority = *p.RuleOverridePriority
			}
			if p.ModelType != nil {
				dst.ModelType = core.ModelType(strings.TrimSpace(*p.ModelType))
			}
			if p.ModelPath != nil {
				dst.ModelPath = strings.TrimSpace(*p.ModelPath)
			}
			if p.AutoTrain != nil {
				dst.AutoTrain = *p.AutoTrain
			}
			if p.TrainInterval != nil {
				dst.TrainInterval = strings.TrimSpace(*p.TrainInterval)
			}
			if p.MinSamplesForTraining != nil {
				dst.MinSamplesForTraining = *p.MinSamplesForTraining
			}
			if p.ActiveLearningEnabled != nil {
				dst.ActiveLearningEnabled = *p.ActiveLearningEnabled
			}
			if p.FeatureHistorySize != nil {
				dst.FeatureHistorySize = *p.FeatureHistorySize
			}
			if p.NumTrees != nil {
				dst.NumTrees = *p.NumTrees
			}
			if p.MaxDepth != nil {
				dst.MaxDepth = *p.MaxDepth
			}
			if p.MinSamplesLeaf != nil {
				dst.MinSamplesLeaf = *p.MinSamplesLeaf
			}
			if p.ValidationSplitRatio != nil {
				dst.ValidationSplitRatio = *p.ValidationSplitRatio
			}
			if p.BalanceClasses != nil {
				dst.BalanceClasses = *p.BalanceClasses
			}
			if p.EnsembleVoting != nil {
				dst.EnsembleVoting = strings.TrimSpace(*p.EnsembleVoting)
			}
			if p.LlmEnabled != nil {
				dst.LlmEnabled = *p.LlmEnabled
			}
			if p.LlmBaseURL != nil {
				dst.LlmBaseURL = strings.TrimSpace(*p.LlmBaseURL)
			}
			if p.LlmAPIKey != nil {
				if key := strings.TrimSpace(*p.LlmAPIKey); key != "" {
					dst.LlmAPIKey = key
				}
			}
			if p.LlmModel != nil {
				dst.LlmModel = strings.TrimSpace(*p.LlmModel)
			}
			if p.LlmTimeoutSeconds != nil {
				dst.LlmTimeoutSeconds = *p.LlmTimeoutSeconds
			}
			if p.LlmTemperature != nil {
				dst.LlmTemperature = *p.LlmTemperature
			}
			if p.LlmMaxTokens != nil {
				dst.LlmMaxTokens = *p.LlmMaxTokens
			}
			if p.LlmSystemPrompt != nil {
				dst.LlmSystemPrompt = strings.TrimSpace(*p.LlmSystemPrompt)
			}
		}
	}

	// Config handlers
	handlers.Deps.GetTagName = getTagName
	handlers.Deps.ConfigTagNames = func() []string {
		tagsMu.RLock()
		defer tagsMu.RUnlock()
		t := []string{}
		for _, n := range tagMap {
			t = append(t, n)
		}
		return t
	}
	handlers.Deps.IsCommDisabled = func(comm string) bool {
		disabledCommsMu.RLock()
		defer disabledCommsMu.RUnlock()
		_, ok := disabledComms[comm]
		return ok
	}
	handlers.Deps.AddDisabledComm = func(comm string) {
		disabledCommsMu.Lock()
		disabledComms[comm] = struct{}{}
		disabledCommsMu.Unlock()
	}
	handlers.Deps.RemoveDisabledComm = func(comm string) {
		disabledCommsMu.Lock()
		delete(disabledComms, comm)
		disabledCommsMu.Unlock()
	}
	handlers.Deps.DeleteDisabledComm = func(comm string) {
		disabledCommsMu.Lock()
		delete(disabledComms, comm)
		disabledCommsMu.Unlock()
	}
	handlers.Deps.DisabledEventTypes = func() []uint32 {
		disabledEventTypesMu.RLock()
		defer disabledEventTypesMu.RUnlock()
		disabled := make([]uint32, 0, len(disabledEventTypes))
		for et := range disabledEventTypes {
			disabled = append(disabled, et)
		}
		return disabled
	}
	handlers.Deps.AddDisabledEventType = func(et uint32) {
		disabledEventTypesMu.Lock()
		disabledEventTypes[et] = struct{}{}
		disabledEventTypesMu.Unlock()
	}
	handlers.Deps.RemoveDisabledEventType = func(et uint32) {
		disabledEventTypesMu.Lock()
		delete(disabledEventTypes, et)
		disabledEventTypesMu.Unlock()
	}
	handlers.Deps.ConfigRules = func() []*pb.WrapperRule {
		rulesMu.RLock()
		defer rulesMu.RUnlock()
		result := make([]*pb.WrapperRule, 0, len(wrapperRules))
		for _, r := range wrapperRules {
			result = append(result, &pb.WrapperRule{
				Comm:         r.Comm,
				Action:       r.Action,
				RewrittenCmd: r.RewrittenCmd,
				Regex:        r.Regex,
				Replacement:  r.Replacement,
				Priority:     int32(r.Priority),
			})
		}
		return result
	}
	handlers.Deps.UpsertConfigRule = func(comm, action, rewrittenCmd, regex, replacement string, priority int32) {
		rulesMu.Lock()
		wrapperRules[comm] = WrapperRule{
			Comm:         comm,
			Action:       action,
			RewrittenCmd: []string{rewrittenCmd},
			Regex:        regex,
			Replacement:  replacement,
			Priority:     int(priority),
		}
		rulesMu.Unlock()
	}
	handlers.Deps.DeleteConfigRule = func(comm string) {
		rulesMu.Lock()
		delete(wrapperRules, comm)
		rulesMu.Unlock()
	}

	initMLHandlersDeps()
}

// ── ML handler wiring ──────────────────────────────────────────────

func initMLHandlersDeps() {
	handlers.Deps.MLStatus = mlStatus
	handlers.Deps.BuildMLStatusJSON = buildMLStatusJSON
	handlers.Deps.MLEnabled = func() bool { return mlEnabled }
	handlers.Deps.MLConfig = func() core.MLConfig { return mlConfig }
	handlers.Deps.CurrentMLConfig = currentMLConfig
	handlers.Deps.MLIsRunning = func() bool { return globalTrainer.isRunning }
	handlers.Deps.MLLogTotal = func() int { return globalTrainer.logTotal }
	handlers.Deps.MLGetLogsResponse = mlGetLogsResponse
	handlers.Deps.MLCancelTraining = globalTrainer.CancelTraining
	handlers.Deps.MLGetHistoryResponse = mlGetHistoryResponse
	handlers.Deps.MLTrain = mlTrain
	handlers.Deps.MLFeedbackResult = mlFeedbackResult
	handlers.Deps.MLSamplesResponse = mlSamplesResponse
	handlers.Deps.MLSampleLabelResult = mlSampleLabelResult
	handlers.Deps.MLRemoveSampleResult = mlRemoveSampleResult
	handlers.Deps.MLSampleAnomalyResult = mlSampleAnomalyResult
	handlers.Deps.MLAddSample = mlAddSample
	handlers.Deps.MLClassifyAndEmbed = func(comm string, args []string) (interface{}, []float64) { return nil, nil }
	handlers.Deps.MLComputeAnomalyScore = func(emb []float64) float64 { return 0 }
	handlers.Deps.MLPredict = func(comm string, args []string) handlers.MLPrediction { return handlers.MLPrediction{} }
	handlers.Deps.MLNetworkAudit = func(comm string, args []string) handlers.MLNetworkAuditResult { return handlers.MLNetworkAuditResult{} }
	handlers.Deps.MLLLMAssessment = func(comm string, args []string) *handlers.MLLlmAssessment { return &handlers.MLLlmAssessment{} }
	handlers.Deps.MLExistingCommands = func() []string { return []string{} }
	handlers.Deps.MLImportResult = mlImportResult
	handlers.Deps.MLTuneResult = mlTuneResult
	handlers.Deps.MLTuneModelsResult = mlTuneModelsResult
	handlers.Deps.MLLLMScoreResult = mlLLMScoreResult
	handlers.Deps.MLLLMBatchScoreResult = mlLLMBatchScoreResult
	handlers.Deps.MLLlmProductionDatasetPullResult = mlLlmProductionDatasetPullResult
	handlers.Deps.MLClassicDatasetsList = func() gin.H { return gin.H{"datasets": []string{}} }
	handlers.Deps.MLClassicDatasetGetResult = mlClassicDatasetGetResult
	handlers.Deps.MLClassicDatasetPreviewResult = mlClassicDatasetPreviewResult
	handlers.Deps.MLDatasetPullResult = mlDatasetPullResult
	handlers.Deps.MLDatasetImportResult = mlDatasetImportResult
	handlers.Deps.MLDatasetExportResult = mlDatasetExportResult
	handlers.Deps.MLDatasetClear = func() {}
	handlers.Deps.MLHealthProcesses = func() gin.H { return gin.H{"processes": []string{}} }
	handlers.Deps.MLHealthGenerators = func() gin.H { return gin.H{"generators": []string{}} }
	handlers.Deps.MLHealthRegister = func(id string) {}
	handlers.Deps.MLHealthUnregister = func(id string) {}
	handlers.Deps.MLHealthRun = func() gin.H { return gin.H{"results": []string{}} }

	// Hooks config wiring
	handlers.Deps.AvailableHooks = func() []core.HookDef { return availableHooks }
	handlers.Deps.IsHookInstalled = isHookInstalled
	handlers.Deps.InstallNativeHook = installNativeHook
	handlers.Deps.UninstallNativeHook = uninstallNativeHook
	handlers.Deps.GetShellConfigPath = getShellConfigPath
	handlers.Deps.EnsureKiroManagedAgentExists = ensureKiroManagedAgentExists
}

// ── Bridge functions (delegate to handlers/ subpackage) ─────────

func handleClearEvents(c *gin.Context)         { handlers.HandleClearEvents(c) }
func handleClearEventsMemory(c *gin.Context)    { handlers.HandleClearEventsMemory(c) }
func handleClearEventsPersisted(c *gin.Context) { handlers.HandleClearEventsPersisted(c) }

func handleRunBenchmark(c *gin.Context)         { handlers.HandleRunBenchmark(c) }
func handleGetBenchmarkResults(c *gin.Context)  { handlers.HandleGetBenchmarkResults(c) }

func handlePluginsList(c *gin.Context)  { handlers.HandlePluginsList(c) }
func handlePluginGet(c *gin.Context)    { handlers.HandlePluginGet(c) }
func handlePluginUpsert(c *gin.Context) { handlers.HandlePluginUpsert(c) }
func handlePluginDelete(c *gin.Context) { handlers.HandlePluginDelete(c) }
func handlePluginToggle(c *gin.Context) { handlers.HandlePluginToggle(c) }
func handleBPFTemplates(c *gin.Context) { handlers.HandleBPFTemplates(c) }
func handleBPFCompile(c *gin.Context)   { handlers.HandleBPFCompile(c) }
func handleBPFLoad(c *gin.Context)      { handlers.HandleBPFLoad(c) }
func handleBPFUnload(c *gin.Context)     { handlers.HandleBPFUnload(c) }

func handleRegister(c *gin.Context)   { handlers.HandleRegister(c) }
func handleUnregister(c *gin.Context) { handlers.HandleUnregister(c) }

func registerPluginRoutes(rg *gin.RouterGroup) {
	handlers.RegisterPluginRoutes(rg)
}

// System handler bridges
func handleSystemLs(c *gin.Context)          { handlers.HandleSystemLs(c) }
func handleFilePreview(c *gin.Context)        { handlers.HandleFilePreview(c) }
func handleFilePreviewStream(c *gin.Context)  { handlers.HandleFilePreviewStream(c) }
func handleFileHex(c *gin.Context)            { handlers.HandleFileHex(c) }
func handleFileELF(c *gin.Context)            { handlers.HandleFileELF(c) }
func handleSystemHome(c *gin.Context)         { handlers.HandleSystemHome(c) }
func handleDownload(c *gin.Context)           { handlers.HandleDownload(c) }
func handleUpload(c *gin.Context)             { handlers.HandleUpload(c) }
func handleRun(c *gin.Context)                { handlers.HandleRun(c) }
func handleSystemdServices(c *gin.Context)    { handlers.HandleSystemdServices(c) }
func handleSystemdControl(c *gin.Context)     { handlers.HandleSystemdControl(c) }
func handleSystemdLogs(c *gin.Context)        { handlers.HandleSystemdLogs(c) }
func handleTrackedComms(c *gin.Context)       { handlers.HandleTrackedComms(c) }
func handleProcessSignal(c *gin.Context)      { handlers.HandleProcessSignal(c) }
func handleProcessMaps(c *gin.Context)        { handlers.HandleProcessMaps(c) }

// Config handler bridges
func handleConfigTagsGet(c *gin.Context)            { handlers.HandleConfigTagsGet(c) }
func handleConfigTagsPost(c *gin.Context)            { handlers.HandleConfigTagsPost(c) }
func handleConfigCommsGet(c *gin.Context)            { handlers.HandleConfigCommsGet(c) }
func handleConfigCommsPost(c *gin.Context)            { handlers.HandleConfigCommsPost(c) }
func handleConfigCommsDelete(c *gin.Context)         { handlers.HandleConfigCommsDelete(c) }
func handleConfigCommsDisable(c *gin.Context)        { handlers.HandleConfigCommsDisable(c) }
func handleConfigCommsEnable(c *gin.Context)         { handlers.HandleConfigCommsEnable(c) }
func handleConfigEventTypesGet(c *gin.Context)        { handlers.HandleConfigEventTypesGet(c) }
func handleConfigEventTypeDisable(c *gin.Context)     { handlers.HandleConfigEventTypeDisable(c) }
func handleConfigEventTypeEnable(c *gin.Context)      { handlers.HandleConfigEventTypeEnable(c) }
func handleConfigPathsGet(c *gin.Context)             { handlers.HandleConfigPathsGet(c) }
func handleConfigPathsPost(c *gin.Context)            { handlers.HandleConfigPathsPost(c) }
func handleConfigPathsDelete(c *gin.Context)          { handlers.HandleConfigPathsDelete(c) }
func handleConfigPrefixesGet(c *gin.Context)          { handlers.HandleConfigPrefixesGet(c) }
func handleConfigPrefixesPost(c *gin.Context)          { handlers.HandleConfigPrefixesPost(c) }
func handleConfigPrefixesDelete(c *gin.Context)       { handlers.HandleConfigPrefixesDelete(c) }
func handleConfigRulesGet(c *gin.Context)             { handlers.HandleConfigRulesGet(c) }
func handleConfigRulesPost(c *gin.Context)            { handlers.HandleConfigRulesPost(c) }
func handleConfigRulesDelete(c *gin.Context)          { handlers.HandleConfigRulesDelete(c) }
func handleMLStatusGet(c *gin.Context)                { handlers.HandleMLStatusGet(c) }
func handleMLLogsGet(c *gin.Context)                   { handlers.HandleMLLogsGet(c) }
func handleMLTrainCancelPost(c *gin.Context)           { handlers.HandleMLTrainCancelPost(c) }
func handleMLHistoryGet(c *gin.Context)                 { handlers.HandleMLHistoryGet(c) }
func handleMLTrainPost(c *gin.Context)                  { handlers.HandleMLTrainPost(c) }
func handleMLFeedbackPost(c *gin.Context)               { handlers.HandleMLFeedbackPost(c) }
func handleMLSamplesGet(c *gin.Context)                 { handlers.HandleMLSamplesGet(c) }
func handleMLSampleLabelPut(c *gin.Context)             { handlers.HandleMLSampleLabelPut(c) }
func handleMLSampleDelete(c *gin.Context)               { handlers.HandleMLSampleDelete(c) }
func handleMLSampleAnomalyPut(c *gin.Context)           { handlers.HandleMLSampleAnomalyPut(c) }
func handleMLSamplesPost(c *gin.Context)                { handlers.HandleMLSamplesPost(c) }
func handleMLBacktestPost(c *gin.Context)               { handlers.HandleMLBacktestPost(c) }
func handleConfigExportGet(c *gin.Context)             { handlers.HandleConfigExportGet(c) }
func handleConfigImportPost(c *gin.Context)             { handlers.HandleConfigImportPost(c) }
func handleConfigRuntimeGet(c *gin.Context)              { handlers.HandleConfigRuntimeGet(c) }
func handleConfigRuntimePut(c *gin.Context)              { handlers.HandleConfigRuntimePut(c) }
func handleConfigAccessTokenPost(c *gin.Context)          { handlers.HandleConfigAccessTokenPost(c) }

func registerSystemRoutes(rg *gin.RouterGroup) {
	handlers.RegisterSystemRoutes(rg)
}

// Hardware handler bridges (camera, microphone, sensors)
func handleSensors(c *gin.Context)           { handlers.HandleSensors(c) }
func handleCameras(c *gin.Context)            { handlers.HandleCameras(c) }
func handleCameraSnapshot(c *gin.Context)     { handlers.HandleCameraSnapshot(c) }
func handleMicrophones(c *gin.Context)        { handlers.HandleMicrophones(c) }
func serveCameraWS(c *gin.Context)            { handlers.ServeCameraWS(c) }
func serveSensorsWS(c *gin.Context)           { handlers.ServeSensorsWS(c) }
func serveMicrophoneWS(c *gin.Context)        { handlers.ServeMicrophoneWS(c) }

// System stats bridge
func serveSystemStatsWS(c *gin.Context) { handlers.ServeSystemStatsWS(c) }

// Hooks config bridges
func handleConfigHooksList(c *gin.Context)    { handlers.HandleConfigHooksList(c) }
func handleConfigHooksInstall(c *gin.Context)  { handlers.HandleConfigHooksInstall(c) }
func handleConfigHooksRawGet(c *gin.Context)   { handlers.HandleConfigHooksRawGet(c) }
func handleConfigHooksRawPost(c *gin.Context)  { handlers.HandleConfigHooksRawPost(c) }