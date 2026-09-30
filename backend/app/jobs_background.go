package app

import (
	"agent-ebpf-filter/app/captureprofile"
	"agent-ebpf-filter/app/recording"
	"agent-ebpf-filter/app/research"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"agent-ebpf-filter/app/events"

	"github.com/cilium/ebpf/ringbuf"
)

type runtimeBackgroundJobs struct {
	wg sync.WaitGroup
}

func (jobs *runtimeBackgroundJobs) Go(run func()) {
	if jobs == nil || run == nil {
		return
	}
	jobs.wg.Add(1)
	go func() {
		defer jobs.wg.Done()
		run()
	}()
}

func (jobs *runtimeBackgroundJobs) Wait(ctx context.Context) error {
	if jobs == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		jobs.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var nativeLittleEndian = func() bool {
	var value uint16 = 1
	return *(*byte)(unsafe.Pointer(&value)) == 1
}()

// commDisabled reports whether the raw kernel comm buffer names a command the
// operator disabled. Clean buffers (the overwhelmingly common case) are looked
// up through a transient string view, so the check does not allocate.
func commDisabled(raw []byte) bool {
	comm := events.TrimNUL(raw)
	if len(comm) == 0 {
		return false
	}
	disabledCommsMu.RLock()
	defer disabledCommsMu.RUnlock()
	if len(disabledComms) == 0 {
		return false
	}
	if bytes.IndexByte(comm, 0) < 0 && utf8.Valid(comm) {
		_, ok := disabledComms[string(comm)]
		return ok
	}
	_, ok := disabledComms[sanitizeUTF8(raw)]
	return ok
}

func eventTypeDisabled(eventType uint32) bool {
	disabledEventTypesMu.RLock()
	defer disabledEventTypesMu.RUnlock()
	_, ok := disabledEventTypes[eventType]
	return ok
}

// decodeBPFEventRecord returns a view over the ring-buffer sample when the host
// layout matches the generated little-endian BPF object. The pointer must not be
// retained after the caller finishes processing this record because the sample
// buffer is reused for the next ReadInto call. On non-native endian or
// unaligned samples it falls back to the old binary.Read copy path.
func decodeBPFEventRecord(raw []byte) (*bpfEvent, bool, error) {
	if len(raw) < bpfEventSampleSize {
		return nil, false, fmt.Errorf("short eBPF event sample: got %d bytes, want at least %d", len(raw), bpfEventSampleSize)
	}

	if nativeLittleEndian && len(raw) > 0 {
		ptr := unsafe.Pointer(&raw[0])
		if uintptr(ptr)%bpfEventSampleAlign == 0 {
			return (*bpfEvent)(ptr), true, nil
		}
	}

	event := new(bpfEvent)
	if err := binary.Read(bytes.NewReader(raw[:bpfEventSampleSize]), binary.LittleEndian, event); err != nil {
		return nil, false, err
	}
	return event, false, nil
}

// kernelEventReader is the subset of *ringbuf.Reader the event loop needs.
// ReadInto lets the loop own one sample buffer for its whole lifetime instead
// of allocating a fresh one per record.
type kernelEventReader interface {
	ReadInto(*ringbuf.Record) error
	Close() error
}

func startKernelEventReader(ctx context.Context, rd kernelEventReader, jobs *runtimeBackgroundJobs) {
	if ctx == nil || rd == nil || jobs == nil {
		return
	}
	jobs.Go(func() {
		selfPid := uint32(os.Getpid())
		record := ringbuf.Record{RawSample: make([]byte, bpfEventSampleSize)}
		for {
			if err := rd.ReadInto(&record); err != nil {
				return
			}
			event, zeroCopy, err := decodeBPFEventRecord(record.RawSample)
			collectorMetricsStore.RecordRingbufDecode(zeroCopy)
			if err != nil {
				log.Printf("[WARN] failed to decode eBPF event: %v (sample len=%d)", err, len(record.RawSample))
				continue
			}
			if event.PID == selfPid {
				continue
			}
			if commDisabled(event.Comm[:]) || eventTypeDisabled(event.Type) {
				continue
			}
			enqueueBroadcastEvent(broadcast, buildKernelEventFromRaw(event), "kernel_event_reader")
		}
	})
	jobs.Go(func() {
		<-ctx.Done()
		_ = rd.Close()
	})
}

func startRuntimeBackgroundJobs(ctx context.Context, features *FeatureRegistry) *runtimeBackgroundJobs {
	jobs := &runtimeBackgroundJobs{}
	initRedactionEngine()
	startAPICaptureProfileWatcher(ctx, jobs)
	jobs.Go(func() { runEventBroadcaster(ctx) })
	jobs.Go(func() { runSemanticAlertStateGC(ctx, semanticAlertsState, semanticStateGCInterval) })
	jobs.Go(func() { runToolBaselineGC(ctx, toolBaseline, toolBaselineEvictionInterval) })
	startKernelRiskFeedbackWorker(ctx)
	startLoopDetectionWorker(ctx)
	research.StartProcessingWorker(ctx)
	jobs.Go(func() {
		<-ctx.Done()
		_ = shutdownKernelRiskFeedbackWorker(context.Background())
	})
	jobs.Go(func() {
		<-ctx.Done()
		_ = loopDetectionWorkerStore.Shutdown(context.Background())
	})
	jobs.Go(func() {
		<-ctx.Done()
		_ = research.ProcessingWorker.Shutdown(context.Background())
	})
	startSignalProcessingWorker(ctx)
	jobs.Go(func() {
		<-ctx.Done()
		_ = signalProcessingWorkerStore.Shutdown(context.Background())
	})
	startSignalProgramLogWriter(ctx)
	jobs.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := signalProgramLogWriterStore.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] signal program log writer did not stop cleanly: %v", err)
		}
	})
	jobs.Go(func() {
		<-ctx.Done()
		// Leave the runtime job group enough time to observe this goroutine exit
		// before main's five-second shutdown deadline expires.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if _, err := recording.Default().StopContext(shutdownCtx); err != nil {
			log.Printf("[WARN] event recording writer did not stop cleanly: %v", err)
		}
	})
	jobs.Go(func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := shutdownCameraStreams(shutdownCtx); err != nil {
			log.Printf("[WARN] camera streams did not stop cleanly: %v", err)
		}
	})
	jobs.Go(func() {
		<-ctx.Done()
		if err := shellSessions.Close(); err != nil {
			log.Printf("[WARN] shell sessions did not stop cleanly: %v", err)
		}
	})
	if features != nil && features.CompiledIn(FeatureML) {
		startMLAutoTuneTasks()
	}
	jobs.Go(func() {
		<-ctx.Done()
		cancelMLAutoTuneTasks()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := mlAutoTuneTasks.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] ML auto-tune tasks did not stop cleanly: %v", err)
		}
	})
	research.StartTaskWorker()
	jobs.Go(func() { startUDSServer(ctx, broadcast) })
	jobs.Go(func() {
		runCgroupAttributionGC(ctx, cgroupAttribution, 5*time.Minute, 30*time.Minute)
	})
	AppCtx.Network.StartGC()
	jobs.Go(func() {
		<-ctx.Done()
		AppCtx.Network.Close()
	})
	jobs.Go(func() {
		runFlowAggregatorGC(ctx, currentNetworkFlowAggregator(), 2*time.Minute, 10*time.Minute)
	})
	jobs.Go(func() {
		runArchiveEvictionLoop(ctx, capturedEventArchive, 5*time.Minute)
	})
	jobs.Go(func() {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			AppCtx.Network.InitGeoIPDatabase()
		}
	})
	if features.CompiledIn(FeatureSandboxCgroup) {
		jobs.Go(func() {
			if err := ensureCgroupSandboxLoaded(); err != nil {
				log.Printf("[CGROUP-SANDBOX] not available: %v", err)
			}
		})
	}
	if features.CompiledIn(FeatureSandboxLSM) {
		jobs.Go(func() {
			if err := ensureLsmEnforcerLoaded(); err != nil {
				log.Printf("[LSM-ENFORCER] not available: %v", err)
			}
		})
	}
	return jobs
}

func startAPICaptureProfileWatcher(ctx context.Context, jobs *runtimeBackgroundJobs) {
	if ctx == nil || jobs == nil {
		return
	}
	path := captureProfileOverlayPath()
	if err := ensureCaptureProfileOverlayFile(path); err != nil {
		log.Printf("[WARN] API capture profile control plane unavailable: %v", err)
		return
	}
	if err := captureprofile.ReloadDefaultJSON(path); err != nil {
		log.Printf("[WARN] initial API capture profile load failed: %v", err)
	} else {
		log.Printf("[INFO] API capture profiles loaded from %s", path)
	}
	jobs.Go(func() {
		captureprofile.WatchDefaultJSON(ctx, path, 2*time.Second, func(err error) {
			log.Printf("[WARN] API capture profile reload rejected; keeping last known-good rules: %v", err)
		})
	})
}

func runSemanticAlertStateGC(ctx context.Context, state *events.SemanticAlertState, interval time.Duration) {
	if ctx == nil || state == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			state.EvictExpired(now.UTC())
		}
	}
}

func runToolBaselineGC(ctx context.Context, state *toolBaselineStore, interval time.Duration) {
	if ctx == nil || state == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			state.EvictExpired(now.UTC())
		}
	}
}

func runArchiveEvictionLoop(ctx context.Context, archive *eventArchive, interval time.Duration) {
	if ctx == nil || archive == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			settings := runtimeSettingsStore.Snapshot()
			if d, err := time.ParseDuration(settings.MaxEventAge); err == nil && d > 0 {
				archive.EvictOlderThan(time.Now().UTC().Add(-d))
			}
		}
	}
}
