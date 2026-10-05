package app

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestRoutineSystemNoisePathsOnlyFilterReadTelemetry(t *testing.T) {
	previous := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{
		settings: RuntimeSettings{IgnoredPaths: []string{"/proc"}},
	}
	t.Cleanup(func() { runtimeSettingsStore = previous })

	for _, event := range []*pb.Event{
		{EventType: pb.EventType_OPENAT, Path: "/sys/devices/system/cpu/online"},
		{EventType: pb.EventType_OPEN, Path: "/etc/ld.so.cache"},
		{EventType: pb.EventType_READ, Path: "/usr/share/zoneinfo/UTC"},
		{Type: "statx", Path: "/sys/class/net/lo"},
		{Type: "readlinkat", Path: "/sys/bus"},
	} {
		if !shouldIgnoreEventPath(event) {
			t.Fatalf("routine read-only system event should be ignored: %+v", event)
		}
	}

	for _, event := range []*pb.Event{
		{EventType: pb.EventType_WRITE, Path: "/sys/class/net/lo"},
		{EventType: pb.EventType_IOCTL, Path: "/dev/null"},
		{EventType: pb.EventType_RENAME, Path: "/etc/localtime"},
		{EventType: pb.EventType_EXECVE, Path: "/usr/share/zoneinfo/UTC"},
	} {
		if shouldIgnoreEventPath(event) {
			t.Fatalf("non-read system event must be preserved: %+v", event)
		}
	}
}

func TestRoutineSystemNoiseDisabledWithEmptyIgnoredPaths(t *testing.T) {
	previous := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{
		settings: RuntimeSettings{IgnoredPaths: []string{}},
	}
	t.Cleanup(func() { runtimeSettingsStore = previous })

	event := &pb.Event{
		EventType: pb.EventType_OPENAT,
		Path:      "/sys/devices/system/cpu/online",
	}
	if shouldIgnoreEventPath(event) {
		t.Fatal("explicit empty ignoredPaths should disable built-in system noise filtering")
	}
}
