package tls

import (
	"strconv"
	"testing"
)

func BenchmarkTLSCaptureStoreSteadyStateAdd(b *testing.B) {
	for _, limit := range []int{64, 2000} {
		b.Run(strconv.Itoa(limit), func(b *testing.B) {
			store := NewTLSCaptureStore(limit)
			for index := 0; index < limit; index++ {
				store.Add(TLSPlaintextEvent{PID: uint32(index)})
			}
			event := TLSPlaintextEvent{PID: 42}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				store.Add(event)
			}
		})
	}
}
