package app

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/shell"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/internal/geoip"
	netcore "agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/pb"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

func (a *cgroupSandboxAdapter) ValidatePort(port uint16) error {
	return validateCgroupSandboxPort(port)
}
func (a *cgroupSandboxAdapter) BlockIP(ip string) error       { return blockIP(ip) }
func (a *cgroupSandboxAdapter) UnblockIP(ip string) error     { return unblockIP(ip) }
func (a *cgroupSandboxAdapter) BlockPort(port uint16) error   { return blockPort(port) }
func (a *cgroupSandboxAdapter) UnblockPort(port uint16) error { return unblockPort(port) }

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

func (a *lsmEnforcerAdapter) ListExecPaths(blocklist any) []string {
	return listLsmExecPaths(blocklist.(*ebpf.Map))
}
func (a *lsmEnforcerAdapter) ListExecNames(blocklist any) []string {
	return listLsmExecNames(blocklist.(*ebpf.Map))
}
func (a *lsmEnforcerAdapter) ListFileNames(blocklist any) []string {
	return listLsmFileNames(blocklist.(*ebpf.Map))
}
func (a *lsmEnforcerAdapter) NormalizePath(path string) (string, error) {
	return normalizeLsmPathString(path)
}
func (a *lsmEnforcerAdapter) NormalizeName(name string) (string, error) {
	return normalizeLsmNameString(name)
}

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
	batch := make([]agentSightExportEvent, 0, len(events))
	for _, e := range events {
		if ev, ok := e.(agentSightExportEvent); ok {
			batch = append(batch, ev)
		}
	}
	a.store.Add(batch...)
}

// ── Shell session adapter ──────────────────────────────────────────

// shellManagerAdapter wraps *shell.Manager to implement the handlers.Deps.ShellSessions interface.
type shellManagerAdapter struct {
	mgr *shell.Manager
}

type handlerNetworkFlowView struct{}

func (handlerNetworkFlowView) Query(query netcore.FlowQuery) netcore.FlowQueryResult {
	return currentNetworkFlowAggregator().Query(query)
}

func (handlerNetworkFlowView) Get(flowID string) (netcore.NetworkFlowSummary, bool) {
	return currentNetworkFlowAggregator().Get(flowID)
}

type handlerTCPTrackerView struct{}

func (handlerTCPTrackerView) Snapshot() []netcore.TCPConnectionState {
	return currentTCPConnections()
}

type handlerDNSCorrelationView struct{}

func (handlerDNSCorrelationView) LookupIP(ip string) (string, bool) {
	return currentDNSCorrelation().LookupIP(ip)
}

func (handlerDNSCorrelationView) LookupDomain(domain string) (string, bool) {
	return currentDNSCorrelation().LookupDomain(domain)
}

func (handlerDNSCorrelationView) Snapshot() []netcore.DNSCacheSnapshotEntry {
	return currentDNSCorrelation().Snapshot()
}

type handlerGeoIPView struct {
	fallback *geoip.Resolver
}

func (view handlerGeoIPView) Lookup(ip string) (geoip.Record, bool) {
	if manager := currentNetworkManager(); manager != nil {
		return manager.GeoIPLookup(ip)
	}
	return view.fallback.Lookup(ip)
}

func (a *shellManagerAdapter) Subscribe() chan struct{}     { return a.mgr.Subscribe() }
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
	// Convert through JSON to support the handler's anonymous request struct
	// which is structurally identical to shell.CreateRequest.
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal session request: %w", err)
	}
	var cr shell.CreateRequest
	if err := json.Unmarshal(data, &cr); err != nil {
		return nil, fmt.Errorf("unmarshal session request: %w", err)
	}
	return a.mgr.NewSession(cr, deps.(shell.Deps))
}
func (a *shellManagerAdapter) AttachWebSocket(id string, conn *websocket.Conn) error {
	session, ok := a.mgr.Get(id)
	if !ok {
		return fmt.Errorf("shell session not found")
	}
	if err := session.Attach(conn); err != nil {
		return err
	}
	go a.readWebSocket(session, conn)
	return nil
}
func (a *shellManagerAdapter) readWebSocket(session *shell.Session, conn *websocket.Conn) {
	defer session.Detach(conn)
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.TextMessage {
			var ctrl shell.ControlMessage
			if json.Unmarshal(data, &ctrl) == nil && ctrl.Type == "resize" {
				_ = session.Resize(ctrl.Cols, ctrl.Rows)
				continue
			}
		}
		_ = session.WriteInput(data)
	}
}
func (a *shellManagerAdapter) Delete(id string) error { return a.mgr.Delete(id) }
func (a *shellManagerAdapter) SendInput(id string, data []byte) error {
	return a.mgr.SendInput(id, data)
}
func (a *shellManagerAdapter) ClearClosed() { a.mgr.ClearClosed() }

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
	handlers.Deps.RunBenchmark = func() (run any, stats any) {
		result := benchmarkEngineStore.runAll()
		runs := benchmarkEngineStore.runsSnapshot()
		return result, computeBenchmarkStats(runs)
	}
	handlers.Deps.GetBenchmarkResults = func() any {
		runs := benchmarkEngineStore.runsSnapshot()
		return map[string]any{
			"runs":  runs,
			"stats": computeBenchmarkStats(runs),
		}
	}

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
		req, ok := manifest.(*handlers.PluginUpsertRequest)
		if !ok {
			return fmt.Errorf("expected *handlers.PluginUpsertRequest, got %T", manifest)
		}
		kind := PluginKind(strings.TrimSpace(req.Kind))
		if kind == "" {
			kind = PluginKindEBPF
		}
		m := &PluginManifest{
			ID:             strings.TrimSpace(req.ID),
			Name:           sanitizePluginName(req.Name),
			Description:    strings.TrimSpace(req.Description),
			Author:         strings.TrimSpace(req.Author),
			Version:        strings.TrimSpace(req.Version),
			Kind:           kind,
			Enabled:        req.Enabled,
			AttachKind:     PluginAttachKind(strings.TrimSpace(req.AttachKind)),
			AttachTarget:   strings.TrimSpace(req.AttachTarget),
			ProgramName:    strings.TrimSpace(req.ProgramName),
			WebhookURL:     strings.TrimSpace(req.WebhookURL),
			WebhookEvents:  append([]string(nil), req.WebhookEvents...),
			CommandComm:    strings.TrimSpace(req.CommandComm),
			CommandArgs:    append([]string(nil), req.CommandArgs...),
			CommandRule:    strings.TrimSpace(req.CommandRule),
			CommandRewrite: append([]string(nil), req.CommandRewrite...),
		}
		if kind == PluginKindEBPF && strings.TrimSpace(req.Source) != "" {
			m.SourceSHA256 = sha256Hex([]byte(req.Source))
			if err := platform.WritePluginSource(m.ID, req.Source); err != nil {
				return err
			}
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
	handlers.Deps.ApplyRuntimeTLSCapture = applyRuntimeTLSCapture
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
	handlers.Deps.NetworkFlowAggregator = handlerNetworkFlowView{}
	handlers.Deps.TCPTracker = handlerTCPTrackerView{}
	handlers.Deps.DNSCorrelation = handlerDNSCorrelationView{}
	handlers.Deps.GeoIPDB = handlerGeoIPView{fallback: geoip.NewResolver()}
	handlers.Deps.AnalyzeEndpoint = analyzeEndpoint

	// External API
	handlers.Deps.BuildFeatureManifest = func(settings core.RuntimeSettings) any {
		manifest := buildFeatureManifest(settings)
		return handlers.FeatureManifestWrapper{Features: manifest.Features}
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
		if filters == nil {
			return records
		}
		typed, ok := filters.(recentEventFilters)
		if !ok {
			return records
		}
		return filterRecentEventRecords(records, typed)
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
	handlers.Deps.MLIsRunning = globalTrainer.IsRunning
	handlers.Deps.MLLogTotal = globalTrainer.LogTotal
	handlers.Deps.MLGetLogsResponse = mlGetLogsResponse
	handlers.Deps.MLCancelTraining = cancelMLAutoTuneTasks
	handlers.Deps.MLGetHistoryResponse = mlGetHistoryResponse
	handlers.Deps.MLTrain = mlTrain
	handlers.Deps.MLFeedbackResult = mlFeedbackResult
	handlers.Deps.MLSamplesResponse = mlSamplesResponse
	handlers.Deps.MLSampleLabelResult = mlSampleLabelResult
	handlers.Deps.MLRemoveSampleResult = mlRemoveSampleResult
	handlers.Deps.MLSampleAnomalyResult = mlSampleAnomalyResult
	handlers.Deps.MLAddSample = mlAddSample
	handlers.Deps.MLClassifyAndEmbed = func(comm string, args []string) (interface{}, []float64) {
		cls, emb := globalEmbedder.ClassifyAndEmbed(comm, args)
		return cls, emb.Vector[:]
	}
	handlers.Deps.MLComputeAnomalyScore = func(emb []float64) float64 {
		var vec [64]float64
		copy(vec[:], emb)
		return globalEmbedder.ComputeAnomalyScore(behavior.BehaviorEmbedding{Vector: vec})
	}
	handlers.Deps.MLPredict = func(comm string, args []string) handlers.MLPrediction {
		features := globalFeatureExtractor.Extract(comm, args, "", 0)
		pred := mlEngine.Predict(features)
		return handlers.MLPrediction{
			Action:     int(pred.Action),
			Confidence: pred.Confidence,
			Label:      actionLabel[pred.Action],
		}
	}
	handlers.Deps.MLNetworkAudit = func(comm string, args []string) handlers.MLNetworkAuditResult {
		result := AuditNetworkBehavior(comm, strings.Join(args, " "))
		return handlers.MLNetworkAuditResult{RiskLevel: result.RiskLevel}
	}
	handlers.Deps.MLLLMAssessment = func(comm string, args []string) *handlers.MLLlmAssessment {
		if !llmScoringConfigured() {
			return &handlers.MLLlmAssessment{Error: "LLM scoring not configured"}
		}
		return &handlers.MLLlmAssessment{Error: "LLM assessment requires request context"}
	}
	handlers.Deps.MLExistingCommands = func() []string {
		candidates, _, _ := existingCommandCandidates(200)
		cmds := make([]string, 0, len(candidates))
		for _, c := range candidates {
			if c.Comm != "" {
				cmds = append(cmds, c.Comm)
			}
		}
		return cmds
	}
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
