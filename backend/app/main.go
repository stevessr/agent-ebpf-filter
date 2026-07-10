package app

import (
	"agent-ebpf-filter/app/runtime"
	"agent-ebpf-filter/app/tls"
	internal_sandbox "agent-ebpf-filter/internal/sandbox"
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
)

// tlsCaptureController is exposed for UDS handler to trigger TLS attach
// on newly registered wrapper PIDs (see server_uds.go).
var tlsCaptureController *tls.TLSCaptureController

func Main() {
	if isBootstrapMode() {
		if err := bootstrapTrackerMaps(); err != nil {
			log.Fatalf("failed to bootstrap eBPF components: %v", err)
		}
		return
	}
	if relaunched, err := ensureBackendPrivileges(); err != nil {
		log.Fatalf("failed to elevate backend privileges: %v", err)
	} else if relaunched {
		return
	}

	AppCtx = newAppContext()
	bindAppNetworkState(AppCtx)
	defer AppCtx.Network.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	AppCtx.Broadcast = broadcast
	AppCtx.Upgrader = upgrader
	AppCtx.RuntimeSettings = runtimeSettingsStore
	AppCtx.CapturedEventArchive = capturedEventArchive
	AppCtx.ShellSessions = shellSessions
	AppCtx.PluginRegistry = pluginRegistry
	AppCtx.EventRecordingStore = eventRecordingStore
	AppCtx.CollectorMetricsStore = &collectorMetricsStore
	AppCtx.OTelExporterStore = otelExporterStore
	AppCtx.ClusterManager = clusterManagerStore
	AppCtx.TrackedProcessContexts = trackedProcessContexts
	AppCtx.CgroupAttribution = cgroupAttribution
	AppCtx.ToolBaseline = toolBaseline
	AppCtx.SemanticAlertsState = semanticAlertsState
	AppCtx.ProtoCache = protoCache
	AppCtx.AgentSightUploadedEvents = agentSightUploadedEvents
	AppCtx.DomainForwardProxyService = domainForwardProxyService

	refreshHooksPaths()
	if _, err := AppCtx.RuntimeSettings.LoadOrCreate(); err != nil {
		log.Printf("[WARN] failed to load runtime settings: %v", err)
	}
	defer otelExporterStore.Close()

	runtime.KillPreviousBackendProcesses()

	initRedactionEngine()

	if err := ensureTrackerMapsLoaded(); err != nil {
		log.Fatalf("failed to initialize eBPF components: %v", err)
	}
	AppCtx.TrackerMaps = trackerMaps
	initObservability()
	settings := AppCtx.RuntimeSettings.Snapshot()
	features := newFeatureRegistry()
	AppCtx.FeatureRegistry = features
	if features.CompiledIn(FeatureDomainForward) {
		domainForwardProxyService.Activate()
		applyRuntimeDomainForwardProxy(settings)
		defer domainForwardProxyService.Close()
	}

	if !features.CompiledIn(FeatureTLSCapture) {
		settings.TlsCaptureEnabled = false
	}
	initTLS()
	tlsRuntime := tls.StartTLSCaptureRuntime(settings)
	tlsCaptureController = tlsRuntime.Controller // expose for UDS wrapper-triggered TLS attach
	defer tlsRuntime.Controller.Close()

	rd, _ := ringbuf.NewReader(trackerMaps.Events)
	defer rd.Close()

	startKernelEventReader(rd)
	startRuntimeBackgroundJobs(ctx, features)

	internal_sandbox.Apply()

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())
	r.Use(ContextMiddleware(AppCtx))

	startArchiveEvictionLoop(ctx)

	registerRoutes(r, AppCtx, features, tlsRuntime.Broadcaster, tlsRuntime.Controller, tlsRuntime.Store, tlsRuntime.Rules)

	seedDefaultTrackedCommands()

	actualPort := chooseBackendPort()
	configureRuntimePort(ctx, actualPort)

	if features.CompiledIn(FeatureML) {
		go func() {
			time.Sleep(1 * time.Second)
			settings := runtimeSettingsStore.Snapshot()
			InitMLEngine(settings.MLConfig)
			AppCtx.MLEngine = mlEngine
			AppCtx.MLEnabled = mlEnabled
			AppCtx.MLModelLoaded = mlModelLoaded
			AppCtx.CurrentModelType = currentModelType
			StartMLEngine()
		}()
	}
	if features.CompiledIn(FeaturePlugins) {
		go func() {
			time.Sleep(2 * time.Second)
			ReapplyEBPFPluginsOnBoot()
		}()
	}

	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
