package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
)

func main() {
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
	domainForwardProxyService.Activate()
	applyRuntimeDomainForwardProxy(settings)
	defer domainForwardProxyService.Close()

	tlsRuntime := startTLSCaptureRuntime(settings)
	defer tlsRuntime.controller.Close()

	rd, _ := ringbuf.NewReader(trackerMaps.Events)
	defer rd.Close()

	startKernelEventReader(rd)
	startRuntimeBackgroundJobs()

	ApplySandbox()

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())

	// Periodic archive eviction based on MaxEventAge
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startArchiveEvictionLoop(ctx)

	registerRoutes(r, tlsRuntime.broadcaster, tlsRuntime.controller, tlsRuntime.store, tlsRuntime.rules)

	seedDefaultTrackedCommands()

	actualPort := chooseBackendPort()
	configureRuntimePort(actualPort)

	startDeferredMLRuntime()
	startDeferredPluginRuntime()

	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
