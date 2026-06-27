package app

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/shell"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/geoip"
	"agent-ebpf-filter/pb"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/getkin/kin-openapi/openapi3"
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

// ── Cgroup sandbox adapter ─────────────────────────────────────────

// cgroupSandboxAdapter wraps app-level cgroup sandbox functions to implement handlers.CgroupSandboxOps.
type cgroupSandboxAdapter struct{}

func (a *cgroupSandboxAdapter) Snapshot() handlers.CgroupSandboxSnapshot {
	snap := currentCgroupSandboxSnapshot()
	return handlers.CgroupSandboxSnapshot{
		Available:       snap.available(),
		Attached:        snap.attached(),
		CgroupPath:      snap.CgroupPath,
		LinkCount:       snap.LinkCount,
		LinkPins:        snap.LinkPins,
		LastError:       snap.LastError,
		CgroupBlocklist: snap.CgroupBlocklist,
		IPBlocklist:     snap.IPBlocklist,
		IP6Blocklist:    snap.IP6Blocklist,
		PortBlocklist:   snap.PortBlocklist,
		SandboxStats:    snap.SandboxStats,
	}
}

func (a *cgroupSandboxAdapter) EnsureLoaded() error { return ensureCgroupSandboxLoaded() }

func (a *cgroupSandboxAdapter) GetStats(statsMap any) (map[string]any, error) {
	cgStats, err := getCgroupSandboxStats(statsMap.(*ebpf.Map))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"connectChecked": cgStats.ConnectChecked,
		"connectBlocked": cgStats.ConnectBlocked,
		"connectAllowed": cgStats.ConnectAllowed,
		"checked":        cgStats.Checked,
		"blocked":        cgStats.Blocked,
		"allowed":        cgStats.Allowed,
	}, nil
}

func (a *cgroupSandboxAdapter) ListBlockedCgroups(blocklist any) []string {
	return listBlockedCgroups(blocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) ListBlockedIPs(ipBlocklist, ip6Blocklist any) []string {
	return listBlockedIPs(ipBlocklist.(*ebpf.Map), ip6Blocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) ListBlockedPorts(portBlocklist any) []uint16 {
	return listBlockedPorts(portBlocklist.(*ebpf.Map))
}

func (a *cgroupSandboxAdapter) BlockCgroup(cgroupID uint64) error   { return blockCgroup(cgroupID) }
func (a *cgroupSandboxAdapter) UnblockCgroup(cgroupID uint64) error { return unblockCgroup(cgroupID) }

func (a *cgroupSandboxAdapter) CgroupIDForPID(pid int, cgroupPath string) (uint64, string, error) {
	return cgroupIDForPID(pid, cgroupPath)
}

func (a *cgroupSandboxAdapter) ParseIP(ip string) (bool, string, error) {
	_, ipText, err := parseCgroupSandboxIP(ip)
	return false, ipText, err // first return is unused in handlers
}

func (a *cgroupSandboxAdapter) ParseCgroupID(raw string) (uint64, error) {
	return parseCgroupIDStr(raw)
}

func (a *cgroupSandboxAdapter) ValidatePort(port uint16) error { return validateCgroupSandboxPort(port) }
func (a *cgroupSandboxAdapter) BlockIP(ip string) error        { return blockIP(ip) }
func (a *cgroupSandboxAdapter) UnblockIP(ip string) error      { return unblockIP(ip) }
func (a *cgroupSandboxAdapter) BlockPort(port uint16) error    { return blockPort(port) }
func (a *cgroupSandboxAdapter) UnblockPort(port uint16) error  { return unblockPort(port) }

// parseCgroupIDStr wraps parseCgroupID which takes json.RawMessage.
func parseCgroupIDStr(raw string) (uint64, error) {
	return parseCgroupID(json.RawMessage(raw))
}

// ── LSM enforcer adapter ────────────────────────────────────────────

// lsmEnforcerAdapter wraps app-level LSM enforcer functions to implement handlers.LsmEnforcerOps.
type lsmEnforcerAdapter struct{}

func (a *lsmEnforcerAdapter) Snapshot() handlers.LsmEnforcerSnapshot {
	snap := currentLsmEnforcerSnapshot()
	return handlers.LsmEnforcerSnapshot{
		Available:         snap.available(),
		Attached:          snap.attached(),
		LinkCount:         snap.LinkCount,
		LinkPins:          snap.LinkPins,
		LastError:         snap.LastError,
		ExecPathBlocklist: snap.ExecPathBlocklist,
		ExecNameBlocklist: snap.ExecNameBlocklist,
		FileNameBlocklist: snap.FileNameBlocklist,
		Stats:             snap.Stats,
	}
}

func (a *lsmEnforcerAdapter) EnsureLoaded() error { return ensureLsmEnforcerLoaded() }

func (a *lsmEnforcerAdapter) GetStats(statsMap any) (map[string]any, error) {
	lsmStats, err := getLsmEnforcerStats(statsMap.(*ebpf.Map))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"execChecked": lsmStats.ExecChecked,
		"execBlocked": lsmStats.ExecBlocked,
		"fileChecked": lsmStats.FileChecked,
		"fileBlocked": lsmStats.FileBlocked,
	}, nil
}

func (a *lsmEnforcerAdapter) ListExecPaths(blocklist any) []string  { return listLsmExecPaths(blocklist.(*ebpf.Map)) }
func (a *lsmEnforcerAdapter) ListExecNames(blocklist any) []string  { return listLsmExecNames(blocklist.(*ebpf.Map)) }
func (a *lsmEnforcerAdapter) ListFileNames(blocklist any) []string  { return listLsmFileNames(blocklist.(*ebpf.Map)) }
func (a *lsmEnforcerAdapter) NormalizePath(path string) (string, error) { return normalizeLsmPathString(path) }
func (a *lsmEnforcerAdapter) NormalizeName(name string) (string, error) { return normalizeLsmNameString(name) }

func (a *lsmEnforcerAdapter) BlockExecPath(path string) error   { return blockLsmExecPath(path) }
func (a *lsmEnforcerAdapter) UnblockExecPath(path string) error { return unblockLsmExecPath(path) }
func (a *lsmEnforcerAdapter) BlockExecName(name string) error   { return blockLsmExecName(name) }
func (a *lsmEnforcerAdapter) UnblockExecName(name string) error { return unblockLsmExecName(name) }
func (a *lsmEnforcerAdapter) BlockFileName(name string) error   { return blockLsmFileName(name) }
func (a *lsmEnforcerAdapter) UnblockFileName(name string) error { return unblockLsmFileName(name) }

// normalizeLsmNameString is a thin wrapper for the 2-arg version.
func normalizeLsmNameString(name string) (string, error) {
	return normalizeLsmNameStringWithLabel(name, "name")
}

// ── AgentSight event store adapter ──────────────────────────────────

// agentSightStoreAdapter wraps *agentSightEventStore to implement the
// handlers.Deps.AgentSightUploadedEvents interface using any.
type agentSightStoreAdapter struct {
	store *agentSightEventStore
}

func (a *agentSightStoreAdapter) Clear() { a.store.Clear() }

func (a *agentSightStoreAdapter) Recent(limit int) []any {
	events := a.store.Recent(limit)
	out := make([]any, len(events))
	for i, e := range events {
		out[i] = e
	}
	return out
}

func (a *agentSightStoreAdapter) Add(events ...any) {
	for _, e := range events {
		if ev, ok := e.(agentSightExportEvent); ok {
			a.store.Add(ev)
		}
	}
}

// ── Shell session adapter ──────────────────────────────────────────

// shellManagerAdapter wraps *shell.Manager to implement the handlers.Deps.ShellSessions interface.
type shellManagerAdapter struct {
	mgr *shell.Manager
}

func (a *shellManagerAdapter) Subscribe() chan struct{}  { return a.mgr.Subscribe() }
func (a *shellManagerAdapter) Unsubscribe(ch chan struct{}) { a.mgr.Unsubscribe(ch) }
func (a *shellManagerAdapter) List() []any {
	list := a.mgr.List()
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}
func (a *shellManagerAdapter) NewSession(req any, deps any) (any, error) {
	return a.mgr.NewSession(req.(shell.CreateRequest), deps.(shell.Deps))
}
func (a *shellManagerAdapter) Delete(id string) error          { return a.mgr.Delete(id) }
func (a *shellManagerAdapter) SendInput(id string, data []byte) error { return a.mgr.SendInput(id, data) }
func (a *shellManagerAdapter) ClearClosed()                     { a.mgr.ClearClosed() }

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
	handlers.Deps.AgentSightUploadedEvents = &agentSightStoreAdapter{store: agentSightUploadedEvents}
	handlers.Deps.RuntimeSettings = runtimeSettingsStore
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
			SubscribeFn: func() handlers.CameraSubscriber {
				sub := cs.Subscribe()
				if sub == nil {
					return nil
				}
				return sub
			},
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
	handlers.Deps.BuildProcessContextFromHookPayload = func(payload map[string]any, toolName, path string) (uint32, handlers.ProcessContext) {
		return buildProcessContextFromHookPayload(payload, toolName, path)
	}
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

		// Network enrichment handlers
		handlers.Deps.NetworkFlowAggregator = networkFlowAggregator
		handlers.Deps.TCPTracker = tcpTracker
		handlers.Deps.DNSCorrelation = dnsCorrelation
		handlers.Deps.GeoIPDB = geoip.NewResolver()
		handlers.Deps.AnalyzeEndpoint = analyzeEndpoint

		// External API
		handlers.Deps.BuildFeatureManifest = func(settings core.RuntimeSettings) any {
			return buildFeatureManifest(settings)
		}
		handlers.Deps.BootstrapTracepointStatus = func() any {
			return bootstrapTracepointStatusStore.Snapshot()
		}
		handlers.Deps.CollectorHealth = func() any {
			return collectorMetricsStore.Snapshot()
		}

		// Shell sessions
		handlers.Deps.ShellSessions = &shellManagerAdapter{mgr: shellSessions}
		handlers.Deps.MakeShellDeps = func() any { return makeShellDeps() }

	// Cgroup sandbox
	handlers.Deps.CgroupSandbox = &cgroupSandboxAdapter{}

	// LSM enforcer
	handlers.Deps.LsmEnforcer = &lsmEnforcerAdapter{}

		// AgentSight data pipeline
		handlers.Deps.RecentEventFiltersFromRequest = func(c any) any {
			return recentEventFiltersFromRequest(c.(*gin.Context))
		}
		handlers.Deps.FilterRecentEventRecords = func(records []CapturedEventRecord, filters any) []CapturedEventRecord {
			return filterRecentEventRecords(records, filters.(recentEventFilters))
		}
		handlers.Deps.NormalizeCapturedEventRecord = normalizeCapturedEventRecord
		handlers.Deps.EventEnvelopeToJSONValue = eventEnvelopeToJSONValue
		handlers.Deps.EnvelopeEventTypeName = envelopeEventTypeName
		handlers.Deps.ParseRecentEventTime = parseRecentEventTime

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
	handlers.Deps.MLAssessCommandSafety = func(c *gin.Context) { cmdsafetyAssessPost(c) }
	handlers.Deps.MLExistingCommandsGetFn = func(c *gin.Context) { cmdsafetyExistingCommandsGet(c) }
	handlers.Deps.MLImportExistingFn = func(c *gin.Context) { cmdsafetyImportExistingPost(c) }
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
// Command safety bridges — delegate to fat-bridge Deps closures
func handleMLAssessPost(c *gin.Context)          { handlers.Deps.MLAssessCommandSafety(c) }
func handleMLExistingCommandsGet(c *gin.Context)  { handlers.Deps.MLExistingCommandsGetFn(c) }
func handleMLImportExistingPost(c *gin.Context)   { handlers.Deps.MLImportExistingFn(c) }
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

// Network enrichment bridges
func handleNetworkFlows(c *gin.Context)            { handlers.HandleNetworkFlows(c) }
func handleNetworkFlowByID(c *gin.Context)          { handlers.HandleNetworkFlowByID(c) }
func handleTCPState(c *gin.Context)                  { handlers.HandleTCPState(c) }
func handleNetworkAnalyze(c *gin.Context)            { handlers.HandleNetworkAnalyze(c) }
func handleGeoIPLookup(c *gin.Context)               { handlers.HandleGeoIPLookup(c) }
func handleDNSLookup(c *gin.Context)                  { handlers.HandleDNSLookup(c) }
func handleDNSCache(c *gin.Context)                   { handlers.HandleDNSCache(c) }
func handleNetworkInterfaces(c *gin.Context)          { handlers.HandleNetworkInterfaces(c) }
func handleNetworkFlowJSONLExport(c *gin.Context)     { handlers.HandleNetworkFlowJSONLExport(c) }

// External API bridges
func handleExternalAPIHealth(c *gin.Context) { handlers.HandleExternalAPIHealth(c) }
func handleExternalAPIOpenAPI(c *gin.Context) { handlers.HandleExternalAPIOpenAPI(c) }
func buildExternalOpenAPISpec() *openapi3.T  { return handlers.BuildExternalOpenAPISpec() }

// LSM enforcer bridges
func handleLsmEnforcerStatus(c *gin.Context)     { handlers.HandleLsmEnforcerStatus(c) }
func handleLsmBlockExecPath(c *gin.Context)      { handlers.HandleLsmBlockExecPath(c) }
func handleLsmUnblockExecPath(c *gin.Context)    { handlers.HandleLsmUnblockExecPath(c) }
func handleLsmBlockExecName(c *gin.Context)      { handlers.HandleLsmBlockExecName(c) }
func handleLsmUnblockExecName(c *gin.Context)    { handlers.HandleLsmUnblockExecName(c) }
func handleLsmBlockFileName(c *gin.Context)      { handlers.HandleLsmBlockFileName(c) }
func handleLsmUnblockFileName(c *gin.Context)    { handlers.HandleLsmUnblockFileName(c) }

// Cgroup sandbox bridges
func handleCgroupSandboxStatus(c *gin.Context)        { handlers.HandleCgroupSandboxStatus(c) }
func handleCgroupSandboxBlockCgroup(c *gin.Context)    { handlers.HandleCgroupSandboxBlockCgroup(c) }
func handleCgroupSandboxUnblockCgroup(c *gin.Context)  { handlers.HandleCgroupSandboxUnblockCgroup(c) }
func handleCgroupSandboxBlockPID(c *gin.Context)       { handlers.HandleCgroupSandboxBlockPID(c) }
func handleCgroupSandboxUnblockPID(c *gin.Context)     { handlers.HandleCgroupSandboxUnblockPID(c) }
func handleCgroupSandboxBlockIP(c *gin.Context)        { handlers.HandleCgroupSandboxBlockIP(c) }
func handleCgroupSandboxUnblockIP(c *gin.Context)      { handlers.HandleCgroupSandboxUnblockIP(c) }
func handleCgroupSandboxBlockPort(c *gin.Context)      { handlers.HandleCgroupSandboxBlockPort(c) }
func handleCgroupSandboxUnblockPort(c *gin.Context)    { handlers.HandleCgroupSandboxUnblockPort(c) }

// Native hook bridge
func handleNativeHookEvent(c *gin.Context) { handlers.HandleNativeHookEvent(c) }

// ML WebSocket bridge
func serveMLStatusWS(c *gin.Context) { handlers.ServeMLStatusWS(c) }

// Feature manifest bridge
func handleSystemFeatures(c *gin.Context) { handlers.HandleSystemFeatures(c) }

// Shell session bridges
func serveShellSessionsWS(c *gin.Context)        { handlers.ServeShellSessionsWS(c) }
func handleCreateShellSession(c *gin.Context)      { handlers.HandleCreateShellSession(c) }
func handleListShellSessions(c *gin.Context)       { handlers.HandleListShellSessions(c) }
func handleDeleteShellSession(c *gin.Context)      { handlers.HandleDeleteShellSession(c) }
func handleSendShellSessionInput(c *gin.Context)   { handlers.HandleSendShellSessionInput(c) }
func handleShellSessionsCleanup(c *gin.Context)    { handlers.HandleShellSessionsCleanup(c) }

// AgentSight bridges
func handleAgentSightEvents(tlsStore *TLSCaptureStore, forceJSONL bool) gin.HandlerFunc {
	return handlers.HandleAgentSightEvents(tlsStore, forceJSONL)
}
func handleAgentSightEventsQuery(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return handlers.HandleAgentSightEventsQuery(tlsStore)
}
func handleAgentSightEventsStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return handlers.HandleAgentSightEventsStream(tlsStore)
}
func handleAgentSightRunnerStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return handlers.HandleAgentSightRunnerStream(tlsStore)
}
func handleAgentSightEventsUpload(c *gin.Context)                { handlers.HandleAgentSightEventsUpload(c) }
func handleAgentSightRunners(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return handlers.HandleAgentSightRunners(tlsStore)
}
func handleAgentSightEventsStats(tlsStore *TLSCaptureStore, runnerID string) gin.HandlerFunc {
	return handlers.HandleAgentSightEventsStats(tlsStore, runnerID)
}
func handleAgentSightRunnerStats(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return handlers.HandleAgentSightRunnerStats(tlsStore)
}