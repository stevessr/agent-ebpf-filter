package events

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func benchmarkKernelEventFixture(typ uint32, path string) *BpfEvent {
	event := &BpfEvent{PID: 4242, TGID: 4242, PPID: 1, UID: 1000, GID: 1000, Type: typ, TagID: 1, Retval: 0}
	copy(event.Comm[:], "claude-code")
	copy(event.Path[:], path)
	return event
}

func withBenchmarkDeps(b *testing.B) {
	b.Helper()
	saved := Deps
	b.Cleanup(func() { Deps = saved })
	Deps.GetTagName = func(uint32) string { return "agent" }
	Deps.SyscallName = func(uint32) string { return "" }
	Deps.ApplyBestEffortProcessContextToEvent = func(*pb.Event) {}
	Deps.KernelRisk = func(*BpfEvent, *pb.Event) {}
	Deps.Network = NoopNetworkSink{}
}

func BenchmarkSanitizeUTF8Path(b *testing.B) {
	var buf [256]byte
	copy(buf[:], "/home/steve/projects/agent-ebpf-filter/backend/app/events/events_network.go")
	b.ReportAllocs()
	for b.Loop() {
		_ = SanitizeUTF8(buf[:])
	}
}

func BenchmarkSanitizeUTF8Comm(b *testing.B) {
	var buf [16]byte
	copy(buf[:], "claude-code")
	b.ReportAllocs()
	for b.Loop() {
		_ = SanitizeUTF8(buf[:])
	}
}

func BenchmarkSanitizeUTF8EmbeddedNUL(b *testing.B) {
	var buf [256]byte
	copy(buf[:], "arg0\x00arg1\x00arg2")
	b.ReportAllocs()
	for b.Loop() {
		_ = SanitizeUTF8(buf[:])
	}
}

func BenchmarkBuildKernelEventOpenat(b *testing.B) {
	withBenchmarkDeps(b)
	event := benchmarkKernelEventFixture(1, "/home/steve/projects/agent-ebpf-filter/backend/app/events/events_network.go")
	b.ReportAllocs()
	for b.Loop() {
		_ = BuildKernelEventFromRaw(event)
	}
}

func BenchmarkBuildKernelEventConnect(b *testing.B) {
	withBenchmarkDeps(b)
	event := benchmarkKernelEventFixture(2, "")
	event.NetFamily = 2
	event.NetDirection = 1
	event.NetPort = 443
	copy(event.NetAddr[:], []byte{93, 184, 216, 34})
	event.Extra2 = 0x0100007f
	event.Extra3 = 0x22d8b85d
	b.ReportAllocs()
	for b.Loop() {
		_ = BuildKernelEventFromRaw(event)
	}
}

func BenchmarkBuildKernelEventSendto(b *testing.B) {
	withBenchmarkDeps(b)
	event := benchmarkKernelEventFixture(uint32(pb.EventType_NETWORK_SENDTO), "")
	event.NetFamily = 2
	event.NetDirection = 1
	event.NetBytes = 29
	event.NetPort = 53
	copy(event.NetAddr[:], []byte{8, 8, 8, 8})
	copy(event.Extra4[:], []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
	})
	b.ReportAllocs()
	for b.Loop() {
		_ = BuildKernelEventFromRaw(event)
	}
}
