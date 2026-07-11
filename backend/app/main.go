package app

import (
	"agent-ebpf-filter/app/runtime"
	"agent-ebpf-filter/app/tls"
	internal_sandbox "agent-ebpf-filter/internal/sandbox"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
)

// tlsCaptureController is exposed for UDS handler to trigger TLS attach
// on newly registered wrapper PIDs (see server_uds.go).
var tlsCaptureController *tls.TLSCaptureController

func applyRuntimeTLSCapture(settings RuntimeSettings) {
	if tlsCaptureController == nil {
		return
	}
	if settings.TlsCaptureEnabled {
		tlsCaptureController.SetAccepting(true)
		return
	}
	if err := tlsCaptureController.Close(); err != nil {
		log.Printf("[WARN] failed to stop TLS capture after runtime disable: %v", err)
	}
}

func Main() error {
	if isBootstrapMode() {
		if err := bootstrapTrackerMaps(); err != nil {
			return fmt.Errorf("bootstrap eBPF components: %w", err)
		}
		return nil
	}
	if relaunched, err := ensureBackendPrivileges(); err != nil {
		return fmt.Errorf("elevate backend privileges: %w", err)
	} else if relaunched {
		return nil
	}

	AppCtx = newAppContext()
	bindAppNetworkState(AppCtx)
	defer AppCtx.Network.Close()
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	go func() {
		<-signalCtx.Done()
		stopSignals()
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := researchTaskStore.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] research task shutdown did not complete cleanly: %v", err)
		}
	}()
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
		return fmt.Errorf("initialize eBPF components: %w", err)
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
	tlsRuntime.Controller.SetEnabledCheck(func() bool {
		return runtimeSettingsStore.Snapshot().TlsCaptureEnabled
	})
	tlsCaptureController = tlsRuntime.Controller // expose for UDS wrapper-triggered TLS attach
	defer tlsRuntime.Controller.Close()

	rd, err := ringbuf.NewReader(trackerMaps.Events)
	if err != nil {
		return fmt.Errorf("open tracker event ring buffer: %w", err)
	}
	defer rd.Close()
	ctx, cancelRuntime := context.WithCancel(signalCtx)

	startKernelEventReader(rd)
	runtimeJobs := startRuntimeBackgroundJobs(ctx, features)
	defer func() {
		cancelRuntime()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer waitCancel()
		if err := runtimeJobs.Wait(waitCtx); err != nil {
			log.Printf("[WARN] runtime background jobs did not stop cleanly: %v", err)
		}
	}()

	internal_sandbox.Apply()

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())
	r.Use(ContextMiddleware(AppCtx))

	startArchiveEvictionLoop(ctx)

	registerRoutes(r, AppCtx, features, tlsRuntime.Broadcaster, tlsRuntime.Controller, tlsRuntime.Store, tlsRuntime.Rules)

	seedDefaultTrackedCommands()

	listener, actualPort, err := listenBackend()
	if err != nil {
		return err
	}
	configureRuntimePort(ctx, actualPort)

	if features.CompiledIn(FeatureML) {
		runtimeJobs.Go(func() {
			timer := time.NewTimer(1 * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			settings := runtimeSettingsStore.Snapshot()
			InitMLEngine(settings.MLConfig)
			AppCtx.MLEngine = mlEngine
			AppCtx.MLEnabled = mlEnabled
			AppCtx.MLModelLoaded = mlModelLoaded
			AppCtx.CurrentModelType = currentModelType
			StartMLEngine()
		})
	}
	if features.CompiledIn(FeaturePlugins) {
		runtimeJobs.Go(func() {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			ReapplyEBPFPluginsOnBoot()
		})
	}

	server := &http.Server{
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := serveHTTPServer(ctx, server, listener, 5*time.Second); err != nil {
		return fmt.Errorf("serve backend HTTP API: %w", err)
	}
	return nil
}
