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
	b.ReportAllocs()
	for b.Loop() {
		event := cloneProtoEvent(template)
		event = enrichEventContext(event)
		record := recordCapturedEvent(event)
		if record.Event == nil {
			b.Fatal("nil record")
		}
		for _, alert := range buildSemanticAlerts(event) {
			_ = enrichEventContext(alert)
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
