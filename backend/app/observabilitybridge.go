package app

import (
	"time"

	"github.com/cilium/ebpf"

	"agent-ebpf-filter/app/observability"
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

		NvmlInitialized: nvmlInitialized,
		FdinfoHistory:   fdinfoHistory,
		FdinfoHistoryMu: &fdinfoHistoryMu,
		FdinfoTime:      &fdinfoTime,
		LegacyWSClientCount: func() int {
			if AppCtx == nil {
				return 0
			}
			return AppCtx.EventClientHub.ClientCount()
		},
		EnvelopeWSClientCount: func() int {
			if AppCtx == nil {
				return 0
			}
			return AppCtx.EnvelopeClientHub.ClientCount()
		},
		PersistQueueStatus: func() observability.PersistQueueStatus {
			status := runtimeSettingsStore.EventLogStatus()
			lastFlushedAt := ""
			if !status.LastFlushedAt.IsZero() {
				lastFlushedAt = status.LastFlushedAt.Format(time.RFC3339Nano)
			}
			return observability.PersistQueueStatus{
				Active:         status.Active,
				Stopping:       status.Stopping,
				QueueLen:       status.QueueLen,
				QueueCap:       status.QueueCap,
				Pending:        status.Pending,
				EnqueuedTotal:  status.EnqueuedTotal,
				PersistedTotal: status.PersistedTotal,
				FailedTotal:    status.FailedTotal,
				DroppedTotal:   status.DroppedTotal,
				LastFlushedAt:  lastFlushedAt,
				LastError:      status.LastError,
			}
		},
		Broadcast: broadcast,
	})
}
