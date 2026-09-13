package tls

import "sync/atomic"

// TLSCaptureTimingStats summarizes probe-clock alignment health for completed
// TLS transport records. Counters are process-local and intentionally avoid
// connection identifiers or plaintext-derived values.
type TLSCaptureTimingStats struct {
	Samples        uint64 `json:"samples"`
	LastDelayNS    uint64 `json:"lastDelayNs"`
	MaxDelayNS     uint64 `json:"maxDelayNs"`
	UnknownClock   uint64 `json:"unknownClock"`
	ReplaySamples  uint64 `json:"replaySamples"`
	MonotonicSamples uint64 `json:"monotonicSamples"`
}

type tlsCaptureTimingAtomicStats struct {
	samples          atomic.Uint64
	lastDelayNS      atomic.Uint64
	maxDelayNS       atomic.Uint64
	unknownClock     atomic.Uint64
	replaySamples    atomic.Uint64
	monotonicSamples atomic.Uint64
}

var tlsCaptureTimingMetrics tlsCaptureTimingAtomicStats

func recordTLSCaptureTimingObservation(observation bpfKtimeObservation) {
	tlsCaptureTimingMetrics.samples.Add(1)
	tlsCaptureTimingMetrics.lastDelayNS.Store(observation.DelayNS)

	for {
		current := tlsCaptureTimingMetrics.maxDelayNS.Load()
		if observation.DelayNS <= current || tlsCaptureTimingMetrics.maxDelayNS.CompareAndSwap(current, observation.DelayNS) {
			break
		}
	}

	switch observation.Clock {
	case "monotonic":
		tlsCaptureTimingMetrics.monotonicSamples.Add(1)
	case "unix_replay":
		tlsCaptureTimingMetrics.replaySamples.Add(1)
	default:
		tlsCaptureTimingMetrics.unknownClock.Add(1)
	}
}

func TLSCaptureTimingStatsSnapshot() TLSCaptureTimingStats {
	return TLSCaptureTimingStats{
		Samples:          tlsCaptureTimingMetrics.samples.Load(),
		LastDelayNS:      tlsCaptureTimingMetrics.lastDelayNS.Load(),
		MaxDelayNS:       tlsCaptureTimingMetrics.maxDelayNS.Load(),
		UnknownClock:     tlsCaptureTimingMetrics.unknownClock.Load(),
		ReplaySamples:    tlsCaptureTimingMetrics.replaySamples.Load(),
		MonotonicSamples: tlsCaptureTimingMetrics.monotonicSamples.Load(),
	}
}
