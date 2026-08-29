package tls

import (
	"fmt"

	"github.com/cilium/ebpf"
)

type BpfTSOpenSSLBackpressureStatus struct {
	KernelDrops     uint64  `json:"kernelDrops"`
	AttemptedRecords uint64 `json:"attemptedRecords"`
	DropPercent     float64 `json:"dropPercent"`
	Error           string  `json:"error,omitempty"`
}

func saturatingSumUint64(values []uint64) uint64 {
	var total uint64
	for _, value := range values {
		if ^uint64(0)-total < value {
			return ^uint64(0)
		}
		total += value
	}
	return total
}

func readPerCPUUint64Counter(counterMap *ebpf.Map) (uint64, error) {
	if counterMap == nil {
		return 0, fmt.Errorf("per-CPU counter map is unavailable")
	}
	key := uint32(0)
	var values []uint64
	if err := counterMap.Lookup(&key, &values); err != nil {
		return 0, fmt.Errorf("lookup per-CPU counter: %w", err)
	}
	return saturatingSumUint64(values), nil
}

func (bridge *BpfTSOpenSSLBridgeRuntime) BackpressureStatus() BpfTSOpenSSLBackpressureStatus {
	if bridge == nil {
		return BpfTSOpenSSLBackpressureStatus{Error: "bpf-ts OpenSSL bridge runtime is unavailable"}
	}

	// Serialize map inspection with Start/Stop so the collection cannot be closed
	// between obtaining the map pointer and the per-CPU lookup.
	bridge.transitionMu.Lock()
	defer bridge.transitionMu.Unlock()
	bridge.mu.RLock()
	loaded := bridge.runtime
	bridge.mu.RUnlock()
	if loaded == nil {
		return BpfTSOpenSSLBackpressureStatus{}
	}

	drops, err := readPerCPUUint64Counter(loaded.Map(bpfTSOpenSSLDropName))
	if err != nil {
		return BpfTSOpenSSLBackpressureStatus{Error: err.Error()}
	}
	records := bridge.records.Load()
	attempted := records
	if ^uint64(0)-attempted < drops {
		attempted = ^uint64(0)
	} else {
		attempted += drops
	}
	status := BpfTSOpenSSLBackpressureStatus{
		KernelDrops:      drops,
		AttemptedRecords: attempted,
	}
	if attempted > 0 {
		status.DropPercent = float64(drops) * 100 / float64(attempted)
	}
	return status
}

func (c *TLSCaptureController) BpfTSOpenSSLBackpressureStatus() BpfTSOpenSSLBackpressureStatus {
	if c == nil {
		return BpfTSOpenSSLBackpressureStatus{Error: "TLS capture controller is unavailable"}
	}
	c.mu.Lock()
	bridge := c.bpfTSBridge
	c.mu.Unlock()
	if bridge == nil {
		return BpfTSOpenSSLBackpressureStatus{}
	}
	return bridge.BackpressureStatus()
}
