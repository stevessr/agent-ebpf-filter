package app

import (
	"agent-ebpf-filter/app/handlers"
	"fmt"

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