package app

import (
	"time"

	"golang.org/x/sys/unix"
)

type kernelCaptureObservation struct {
	CapturedAt time.Time
	IngestedAt time.Time
	DelayNS    uint64
	Clock      string
}

// alignKernelKtime is pure so audit clock behavior is deterministic in tests.
// bpf_ktime_get_ns() and CLOCK_MONOTONIC share the same boot-relative domain.
func alignKernelKtime(rawNS uint64, observedAt time.Time, monotonicNowNS uint64) kernelCaptureObservation {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	out := kernelCaptureObservation{
		CapturedAt: observedAt,
		IngestedAt: observedAt,
		Clock:      "unknown",
	}
	if rawNS == 0 || monotonicNowNS == 0 || rawNS > monotonicNowNS {
		return out
	}

	delta := monotonicNowNS - rawNS
	if delta > uint64(^uint64(0)>>1) {
		return out
	}
	out.DelayNS = delta
	out.CapturedAt = observedAt.Add(-time.Duration(delta))
	out.Clock = "monotonic"
	return out
}

func observeKernelKtime(rawNS uint64, observedAt time.Time) kernelCaptureObservation {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	var mono unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &mono); err != nil || mono.Sec < 0 || mono.Nsec < 0 {
		return alignKernelKtime(rawNS, observedAt, 0)
	}
	monoNS := uint64(mono.Sec)*uint64(time.Second) + uint64(mono.Nsec)
	return alignKernelKtime(rawNS, observedAt, monoNS)
}
