package tls

// applyTLSCaptureObservation annotates a user-visible event with a clock
// observation that may be shared by every protocol event derived from the same
// completed TLS transport record. This avoids repeating CLOCK_MONOTONIC
// alignment work for HTTP/2/SSE records that fan out into multiple events.
func applyTLSCaptureObservation(event *TLSPlaintextEvent, probeTimestampNS uint64, observation bpfKtimeObservation) {
	if event == nil {
		return
	}
	// HTTP stream assemblers may intentionally preserve the timestamp of the
	// first transport chunk that contributed to a message. Do not replace that
	// semantic timestamp with the final chunk's timestamp at dispatch time.
	if event.Timestamp.IsZero() {
		event.Timestamp = observation.Captured
	}
	event.ProbeTimestampNS = probeTimestampNS
	event.ProbeClock = observation.Clock
	event.IngestTimestamp = observation.Ingested
	event.CaptureDelayNS = observation.DelayNS
}

// applyTLSCaptureTiming remains the convenience entry point for callers that
// do not already have a shared observation.
func applyTLSCaptureTiming(event *TLSPlaintextEvent, probeTimestampNS uint64) {
	if event == nil {
		return
	}
	applyTLSCaptureObservation(event, probeTimestampNS, observeBPFKtime(probeTimestampNS))
}
