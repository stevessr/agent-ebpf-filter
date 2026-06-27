package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	"github.com/cilium/ebpf/ringbuf"
)

// ---- moved from backend/zz_merged_backend.go section jobs_background.go ----

const (
	bpfEventSampleSize  = int(unsafe.Sizeof(bpfEvent{}))
	bpfEventSampleAlign = uintptr(unsafe.Alignof(bpfEvent{}))
)

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

func startKernelEventReader(rd *ringbuf.Reader) {
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
	go func() {
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
			broadcast <- buildKernelEventFromRaw(event)
		}
	}()
}

func startRuntimeBackgroundJobs(features *FeatureRegistry) {
	startEventBroadcaster()
	startKernelRiskFeedbackWorker()
	go startUDSServer(broadcast)
	startCgroupAttributionGC()
	startDNSCacheGC()
	startTCPStateTrackerGC()
	startFlowAggregatorGC()
	startExfilDetectionLoop()
	go func() {
		time.Sleep(100 * time.Millisecond)
		AppCtx.Network.InitGeoIPDatabase()
	}()
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		for range ticker.C {
			globalBandwidthTracker.EvictOlderThan(15 * time.Minute)
		}
	}()
	if features.CompiledIn(FeatureSandboxCgroup) {
		go func() {
			if err := ensureCgroupSandboxLoaded(); err != nil {
				log.Printf("[CGROUP-SANDBOX] not available: %v", err)
			}
		}()
	}
	if features.CompiledIn(FeatureSandboxLSM) {
		go func() {
			if err := ensureLsmEnforcerLoaded(); err != nil {
				log.Printf("[LSM-ENFORCER] not available: %v", err)
			}
		}()
	}
}

func startArchiveEvictionLoop(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				settings := runtimeSettingsStore.Snapshot()
				if d, err := time.ParseDuration(settings.MaxEventAge); err == nil && d > 0 {
					capturedEventArchive.EvictOlderThan(time.Now().UTC().Add(-d))
				}
			}
		}
	}()
}

func startDeferredMLRuntime() {
	go func() {
		time.Sleep(1 * time.Second)
		settings := runtimeSettingsStore.Snapshot()
		InitMLEngine(settings.MLConfig)
		StartMLEngine()
	}()
}

func startDeferredPluginRuntime() {
	go func() {
		time.Sleep(2 * time.Second)
		ReapplyEBPFPluginsOnBoot()
	}()
}
