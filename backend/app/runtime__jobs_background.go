package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"
	"os"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

// ---- moved from backend/zz_merged_backend.go section jobs_background.go ----

func startKernelEventReader(rd *ringbuf.Reader) {
	go func() {
		var event bpfEvent
		selfPid := uint32(os.Getpid())
		for {
			record, err := rd.Read()
			if err != nil {
				return
			}
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("[WARN] failed to decode eBPF event: %v (sample len=%d)", err, len(record.RawSample))
				continue
			}
			if event.PID == selfPid {
				continue
			}
			comm := sanitizeUTF8(event.Comm[:])
			if isCommDisabled(comm) {
				continue
			}
			if isEventTypeDisabled(event.Type) {
				continue
			}
			broadcast <- buildKernelEvent(event)
		}
	}()
}

func startRuntimeBackgroundJobs() {
	startEventBroadcaster()
	go startUDSServer(broadcast)
	startCgroupAttributionGC()
	startDNSCacheGC()
	startTCPStateTrackerGC()
	startFlowAggregatorGC()
	startExfilDetectionLoop()
	go func() {
		time.Sleep(100 * time.Millisecond)
		initGeoIPDatabase()
	}()
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		for range ticker.C {
			globalBandwidthTracker.EvictOlderThan(15 * time.Minute)
		}
	}()
	go func() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			log.Printf("[CGROUP-SANDBOX] not available: %v", err)
		}
	}()
	go func() {
		if err := ensureLsmEnforcerLoaded(); err != nil {
			log.Printf("[LSM-ENFORCER] not available: %v", err)
		}
	}()
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
