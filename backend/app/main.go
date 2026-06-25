package app

import (
	"context"
	"fmt"
	"log"

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

	refreshHooksPaths()
	if _, err := runtimeSettingsStore.LoadOrCreate(); err != nil {
		log.Printf("[WARN] failed to load runtime settings: %v", err)
	}
	defer otelExporterStore.Close()

	killPreviousBackendProcesses()

	if err := ensureTrackerMapsLoaded(); err != nil {
		log.Fatalf("failed to initialize eBPF components: %v", err)
	}
	settings := runtimeSettingsStore.Snapshot()
	features := newFeatureRegistry()
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

	registerRoutes(r, features, tlsRuntime.broadcaster, tlsRuntime.controller, tlsRuntime.store, tlsRuntime.rules)

	seedDefaultTrackedCommands()

	actualPort := chooseBackendPort()
	configureRuntimePort(actualPort)

	if features.CompiledIn(FeatureML) {
		startDeferredMLRuntime()
	}
	if features.CompiledIn(FeaturePlugins) {
		startDeferredPluginRuntime()
	}

	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}