package app

import (
	"strconv"
	"testing"
)

func BenchmarkAgentSightEventStoreSteadyStateAdd(b *testing.B) {
	for _, limit := range []int{64, 10000} {
		b.Run(strconv.Itoa(limit), func(b *testing.B) {
			store := newAgentSightEventStore(limit)
			warm := make([]agentSightExportEvent, limit)
			store.Add(warm...)
			event := agentSightExportEvent{ID: "new"}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				store.Add(event)
			}
		})
	}
}
