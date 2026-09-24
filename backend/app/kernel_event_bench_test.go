package app

import "testing"

// BenchmarkBuildKernelEventFromRawFullPath exercises the real Deps wiring
// (tags, process context, kernel risk scoring) for the two most common
// kernel event shapes.
func BenchmarkBuildKernelEventFromRawFullPath(b *testing.B) {
	b.Run("openat", func(b *testing.B) {
		var raw bpfEvent
		raw.PID = 4242
		raw.Type = 1
		raw.TagID = getTagID("AI Agent")
		copy(raw.Comm[:], "claude")
		copy(raw.Path[:], "/home/steve/project/src/main.go")
		b.ReportAllocs()
		for b.Loop() {
			if buildKernelEventFromRaw(&raw) == nil {
				b.Fatal("nil event")
			}
		}
	})
	b.Run("unlink-sensitive", func(b *testing.B) {
		var raw bpfEvent
		raw.PID = 4242
		raw.Type = 4
		raw.TagID = getTagID("AI Agent")
		copy(raw.Comm[:], "rm")
		copy(raw.Path[:], "/etc/shadow")
		b.ReportAllocs()
		for b.Loop() {
			if buildKernelEventFromRaw(&raw) == nil {
				b.Fatal("nil event")
			}
		}
	})
}
