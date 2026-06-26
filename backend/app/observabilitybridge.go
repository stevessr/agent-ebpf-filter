package app

import (
	"agent-ebpf-filter/app/observability"
	"github.com/cilium/ebpf"
)

// ── TrackerMapSet adapter ─────────────────────────────────────────────────

type observabilityTrackerMapSet struct{}

func (observabilityTrackerMapSet) GetCollectorStats() *ebpf.Map {
	return trackerMaps.CollectorStats
}

// ── Init observability subpackage (optional) ──────────────────────────────

// initObservability is called during app initialization to inject global
// dependencies into the observability subpackage.
func initObservability() {
	observability.Init(observability.Deps{
		TrackerMaps: observabilityTrackerMapSet{},

		NvmlInitialized:     nvmlInitialized,
		FdinfoHistory:       fdinfoHistory,
		FdinfoHistoryMu:     &fdinfoHistoryMu,
		FdinfoTime:          &fdinfoTime,
		Clients:             clients,
		ClientsMu:           &clientsMu,
		EnvelopeClients:     envelopeClients,
		EnvelopeClientsMu:   &envelopeClientsMu,
		Broadcast:           broadcast,
	})
}
