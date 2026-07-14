package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
	"unsafe"

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

// ---- moved from backend/zz_merged_backend.go section jobs_background.go ----

var nativeLittleEndian = func() bool {
	var value uint16 = 1
	return *(*byte)(unsafe.Pointer(&value)) == 1
}()

// Pointers to filtering functions set at init time.
var (
	isCommDisabledFunc      func(comm string) bool
	isEventTypeDisabledFunc func(et uint32) bool
)

// isCommDisabled checks whether a command name has been disabled via config.
// Must only be called after startKernelEventReader has wired it.
func isCommDisabled(comm string) bool {
	if isCommDisabledFunc != nil {
		return isCommDisabledFunc(comm)
	}
	return false
}

// isEventTypeDisabled checks whether an event type has been disabled via config.
// Must only be called after startKernelEventReader has wired it.
func isEventTypeDisabled(et uint32) bool {
	if isEventTypeDisabledFunc != nil {
		return isEventTypeDisabledFunc(et)
	}
	return false
}

// decodeBPFEventRecord returns a view over the ring-buffer sample when the host
// layout matches the generated little-endian BPF object. The pointer must not be
// retained after the caller finishes processing this record because RawSample is
// backed by the ringbuf reader's mmap window. On non-native endian or unaligned
// samples it falls back to the old binary.Read copy path.
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

type kernelEventReader interface {
	Read() (ringbuf.Record, error)
	Close() error
}

func startKernelEventReader(ctx context.Context, rd kernelEventReader, jobs *runtimeBackgroundJobs) {
	if ctx == nil || rd == nil || jobs == nil {
		return
	}
	// Wire config filter functions from the app-level globals.
	isCommDisabledFunc = func(comm string) bool {
		disabledCommsMu.RLock()
		defer disabledCommsMu.RUnlock()
		_, ok := disabledComms[comm]
		return ok
	}
	isEventTypeDisabledFunc = func(et uint32) bool {
		disabledEventTypesMu.RLock()
		defer disabledEventTypesMu.RUnlock()
		_, ok := disabledEventTypes[et]
		return ok
	}
	jobs.Go(func() {
		selfPid := uint32(os.Getpid())
		for {
			record, err := rd.Read()
			if err != nil {
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
			comm := sanitizeUTF8(event.Comm[:])
			if isCommDisabledFunc(comm) {
				continue
			}
			if isEventTypeDisabledFunc(event.Type) {
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
	jobs.Go(func() { runEventBroadcaster(ctx) })
	startKernelRiskFeedbackWorker(ctx)
	startLoopDetectionWorker(ctx)
	startResearchProcessingWorker(ctx)
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
		_ = researchProcessingWorkerStore.Shutdown(context.Background())
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
		if _, err := eventRecordingStore.StopContext(shutdownCtx); err != nil {
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
	jobs.Go(func() {
		<-ctx.Done()
		cancelMLAutoTuneTasks()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := mlAutoTuneTasks.Shutdown(shutdownCtx); err != nil {
			log.Printf("[WARN] ML auto-tune tasks did not stop cleanly: %v", err)
		}
	})
	startResearchTaskWorker()
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
