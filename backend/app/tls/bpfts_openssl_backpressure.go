package tls

import (
	"errors"
	"fmt"

	"github.com/cilium/ebpf"
)

type BpfTSOpenSSLBackpressureStatus struct {
	KernelDrops      uint64  `json:"kernelDrops"`
	UserReadErrors   uint64  `json:"userReadErrors"`
	AttemptedRecords uint64  `json:"attemptedRecords"`
	DropPercent      float64 `json:"dropPercent"`
	TotalLosses      uint64  `json:"totalLosses"`
	CaptureAttempts  uint64  `json:"captureAttempts"`
	TotalLossPercent float64 `json:"totalLossPercent"`
	Error            string  `json:"error,omitempty"`
}

func saturatingAddUint64(left, right uint64) uint64 {
	if ^uint64(0)-left < right {
		return ^uint64(0)
	}
	return left + right
}

func saturatingSumUint64(values []uint64) uint64 {
	var total uint64
	for _, value := range values {
		total = saturatingAddUint64(total, value)
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

func bpfTSOpenSSLBackpressureFromCounts(records, drops, userReadErrors uint64) BpfTSOpenSSLBackpressureStatus {
	outputAttempts := saturatingAddUint64(records, drops)
	totalLosses := saturatingAddUint64(drops, userReadErrors)
	captureAttempts := saturatingAddUint64(records, totalLosses)
	status := BpfTSOpenSSLBackpressureStatus{
		KernelDrops:      drops,
		UserReadErrors:   userReadErrors,
		AttemptedRecords: outputAttempts,
		TotalLosses:      totalLosses,
		CaptureAttempts:  captureAttempts,
	}
	if outputAttempts > 0 {
		status.DropPercent = float64(drops) * 100 / float64(outputAttempts)
	}
	if captureAttempts > 0 {
		status.TotalLossPercent = float64(totalLosses) * 100 / float64(captureAttempts)
	}
	return status
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

	drops, dropErr := readPerCPUUint64Counter(loaded.Map(bpfTSOpenSSLDropName))
	userReadErrors, readErr := readPerCPUUint64Counter(loaded.Map(bpfTSOpenSSLReadErrorName))
	status := bpfTSOpenSSLBackpressureFromCounts(bridge.records.Load(), drops, userReadErrors)
	if err := errors.Join(dropErr, readErr); err != nil {
		status.Error = err.Error()
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
