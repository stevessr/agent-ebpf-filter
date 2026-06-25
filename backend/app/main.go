package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
)

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
	AppCtx.Broadcast = broadcast
	AppCtx.Clients = clients
	AppCtx.EnvelopeClients = envelopeClients
	AppCtx.Upgrader = upgrader
	AppCtx.RuntimeSettings = newRuntimeState()
	AppCtx.CapturedEventArchive = newEventArchive(1500)
	AppCtx.ShellSessions = shellSessions
	AppCtx.PluginRegistry = pluginRegistry
	AppCtx.NetworkFlowAggregator = networkFlowAggregator

	refreshHooksPaths()
	if _, err := AppCtx.RuntimeSettings.LoadOrCreate(); err != nil {
		log.Printf("[WARN] failed to load runtime settings: %v", err)
	}
	defer otelExporterStore.Close()

	killPreviousBackendProcesses()

	if err := ensureTrackerMapsLoaded(); err != nil {
		log.Fatalf("failed to initialize eBPF components: %v", err)
	}
	AppCtx.TrackerMaps = trackerMaps
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
	tlsRuntime := startTLSCaptureRuntime(settings)
	defer tlsRuntime.controller.Close()

	rd, _ := ringbuf.NewReader(trackerMaps.Events)
	defer rd.Close()

	startKernelEventReader(rd)
	startRuntimeBackgroundJobs(features)

	ApplySandbox()

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startArchiveEvictionLoop(ctx)

	registerRoutes(r, AppCtx, features, tlsRuntime.broadcaster, tlsRuntime.controller, tlsRuntime.store, tlsRuntime.rules)

	seedDefaultTrackedCommands()

	actualPort := chooseBackendPort()
	configureRuntimePort(actualPort)

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