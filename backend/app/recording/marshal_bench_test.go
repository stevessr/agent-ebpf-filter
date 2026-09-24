package recording

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func BenchmarkMarshalRecord(b *testing.B) {
	record := CapturedEventRecord{
		ReceivedAt: time.Unix(1_700_000_000, 0).UTC(),
		Event: &pb.Event{
			Pid: 4242, Ppid: 1, Uid: 1000, Type: "openat", Tag: "AI Agent", Comm: "claude",
			Path: "/home/steve/project/src/main.go", ExtraInfo: "kernel_risk score=8 decision=OBSERVE reasons=agent_context",
			SchemaVersion: "v1", CgroupId: 12345, RiskScore: 8,
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := MarshalRecord(record); err != nil {
			b.Fatal(err)
		}
	}
}
