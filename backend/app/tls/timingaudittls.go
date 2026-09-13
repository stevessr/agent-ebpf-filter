package tls

// applyTLSCaptureTiming annotates a user-visible event with both the raw probe
// timestamp and the userspace observation. Keeping both clocks makes ordering,
// capture delay and replay provenance auditable without exposing connection
// pointer identities.
func applyTLSCaptureTiming(event *TLSPlaintextEvent, probeTimestampNS uint64) {
	if event == nil {
		return
	}
	observation := observeBPFKtime(probeTimestampNS)
	event.Timestamp = observation.Captured
	event.ProbeTimestampNS = probeTimestampNS
	event.ProbeClock = observation.Clock
	event.IngestTimestamp = observation.Ingested
	event.CaptureDelayNS = observation.DelayNS
}
