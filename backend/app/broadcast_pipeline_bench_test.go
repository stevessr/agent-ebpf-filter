package app

import (
	"testing"

	"agent-ebpf-filter/pb"
)

// BenchmarkBroadcastPipelinePerEvent mirrors the per-event work done by
// runEventBroadcaster between the broadcast channel and the batch flush.
func BenchmarkBroadcastPipelinePerEvent(b *testing.B) {
	var raw bpfEvent
	raw.PID = 4242
	raw.Type = 1
	raw.TagID = getTagID("AI Agent")
	copy(raw.Comm[:], "claude")
	copy(raw.Path[:], "/home/steve/project/src/main.go")
	template := buildKernelEventFromRaw(&raw)
	// The pipeline takes ownership of each event, so hand it a pool of
	// producer-built events instead of cloning inside the timed loop.
	pool := make([]*pb.Event, 1024)
	for i := range pool {
		pool[i] = cloneProtoEvent(template)
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		event := enrichEventContext(pool[i%len(pool)])
		i++
		alerts := buildSemanticAlerts(event)
		record := recordCapturedEvent(event)
		if record.Event == nil {
			b.Fatal("nil record")
		}
		for _, alert := range alerts {
			recordCapturedEvent(enrichEventContext(alert))
		}
	}
}

func BenchmarkCloneProtoEvent(b *testing.B) {
	template := &pb.Event{Pid: 1, Type: "openat", Comm: "claude", Path: "/home/steve/project/src/main.go", Tag: "AI Agent", ExtraInfo: "kernel_risk score=8 decision=OBSERVE reasons=agent_context"}
	b.ReportAllocs()
	for b.Loop() {
		if cloneProtoEvent(template) == nil {
			b.Fatal("nil")
		}
	}
}
