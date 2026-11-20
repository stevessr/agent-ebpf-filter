package app

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/sandboxruntime"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/geoip"
	"fmt"

	"github.com/cilium/ebpf"
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
	return putTrackedSelector(a.set, a.set.TrackedPaths, key, value)
}

func (a *handlerTrackerMapsAdapter) TrackedPathsDelete(key any) error {
	return deleteTrackedSelector(a.set, a.set.TrackedPaths, key)
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesIterate() *ebpf.MapIterator {
	return a.set.TrackedPrefixes.Iterate()
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesPut(key, value any) error {
	return putTrackedSelector(a.set, a.set.TrackedPrefixes, key, value)
}

func (a *handlerTrackerMapsAdapter) TrackedPrefixesDelete(key any) error {
	return deleteTrackedSelector(a.set, a.set.TrackedPrefixes, key)
}

func (a *handlerTrackerMapsAdapter) CollectorStats() *ebpf.Map {
	return a.set.CollectorStats
}

// ── Handlers Dependency wiring ─────────────────────────────────────

func init() {
	// Tracker maps
	handlers.Deps.TrackerMaps = &handlerTrackerMapsAdapter{set: &trackerMaps}

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

	handlers.Deps.Plugins = pluginService{}

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

	// Config handlers
	handlers.Deps.Config = trackingConfigStore{}

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

	// Sandbox runtime attribution (read-only host boundary discovery)
	handlers.Deps.SandboxRuntime = sandboxruntime.NewManager()

	// Cgroup sandbox
	handlers.Deps.CgroupSandbox = &cgroupSandboxAdapter{}

	// LSM enforcer
	handlers.Deps.LsmEnforcer = &lsmEnforcerAdapter{}

	initMLHandlersDeps()
}

// ── ML handler wiring ──────────────────────────────────────────────

func initMLHandlersDeps() {
	handlers.Deps.ML = mlService{}

	// Hooks config wiring
	handlers.Deps.AvailableHooks = func() []core.HookDef { return availableHooks }
	handlers.Deps.IsHookInstalled = isHookInstalled
	handlers.Deps.InstallNativeHook = installNativeHook
	handlers.Deps.UninstallNativeHook = uninstallNativeHook
	handlers.Deps.InstallPluginHook = func(h core.HookDef) error {
		if h.ID != "dsh-exec" {
			return fmt.Errorf("plugin hook installer is not implemented for %s", h.ID)
		}
		return installDshSubprocessPlugin()
	}
	handlers.Deps.UninstallPluginHook = func(h core.HookDef) error {
		if h.ID != "dsh-exec" {
			return fmt.Errorf("plugin hook uninstaller is not implemented for %s", h.ID)
		}
		return uninstallDshSubprocessPlugin()
	}
	handlers.Deps.GetShellConfigPath = getShellConfigPath
	handlers.Deps.EnsureKiroManagedAgentExists = ensureKiroManagedAgentExists
}

// ── Cgroup sandbox adapter ─────────────────────────────────────────

// cgroupSandboxAdapter wraps app-level cgroup sandbox functions to implement handlers.CgroupSandboxOps.
type cgroupSandboxAdapter struct{}

func (a *cgroupSandboxAdapter) Snapshot() handlers.CgroupSandboxSnapshot {
	snap := currentCgroupSandboxSnapshot()
	return handlers.CgroupSandboxSnapshot{
		Available:       snap.Available(),
		Attached:        snap.Attached(),
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

func parseCgroupIDStr(raw string) (uint64, error) { return parseSandboxCgroupID(raw) }

// ── LSM enforcer adapter ────────────────────────────────────────────

// lsmEnforcerAdapter wraps app-level LSM enforcer functions to implement handlers.LsmEnforcerOps.
type lsmEnforcerAdapter struct{}

func (a *lsmEnforcerAdapter) Snapshot() handlers.LsmEnforcerSnapshot {
	snap := currentLsmEnforcerSnapshot()
	return handlers.LsmEnforcerSnapshot{
		Available:           snap.Available(),
		Attached:            snap.Attached(),
		LinkCount:           snap.LinkCount,
		LinkPins:            snap.LinkPins,
		LastError:           snap.LastError,
		PathAccessSupported: snap.PathAccessSupported,
		ExecPathBlocklist:   snap.ExecPathBlocklist,
		ExecNameBlocklist:   snap.ExecNameBlocklist,
		FileNameBlocklist:   snap.FileNameBlocklist,
		Stats:               snap.Stats,
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

func (a *lsmEnforcerAdapter) ListFileAccessPaths(m any) []handlers.LsmFileAccessRule {
	if m == nil {
		return nil
	}
	rows := listLsmFileAccessPaths(m.(*ebpf.Map))
	out := make([]handlers.LsmFileAccessRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, handlers.LsmFileAccessRule{
			Path: row.Path, DenyRead: row.DenyRead, DenyWrite: row.DenyWrite,
		})
	}
	return out
}

func (a *lsmEnforcerAdapter) SetFileAccessPath(path string, read, write bool) error {
	return setLsmFileAccessPath(path, read, write)
}

func (a *lsmEnforcerAdapter) NormalizeFileAccessPath(path string) (string, error) {
	return normalizeLsmFileAccessPath(path)
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
