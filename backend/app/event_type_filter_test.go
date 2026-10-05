package app

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestEventTypeDisabledBitset(t *testing.T) {
	store := trackingConfigStore{}
	previous := store.DisabledEventTypes()
	t.Cleanup(func() { store.ReplaceDisabledEventTypes(previous) })

	store.ReplaceDisabledEventTypes([]uint32{1, 63, 64, 255})
	for _, eventType := range []uint32{1, 63, 64, 255} {
		if !eventTypeDisabled(eventType) {
			t.Fatalf("event type %d should be disabled", eventType)
		}
	}
	for _, eventType := range []uint32{0, 2, 62, 65, 254, 256} {
		if eventTypeDisabled(eventType) {
			t.Fatalf("event type %d should be enabled", eventType)
		}
	}

	store.AddDisabledEventType(42)
	if !eventTypeDisabled(42) {
		t.Fatal("event type 42 should be disabled after AddDisabledEventType")
	}

	store.RemoveDisabledEventType(64)
	if eventTypeDisabled(64) {
		t.Fatal("event type 64 should be enabled after RemoveDisabledEventType")
	}
	if !eventTypeDisabled(63) || !eventTypeDisabled(255) {
		t.Fatal("adjacent disabled bits were changed unexpectedly")
	}
}


func TestDefaultRenewDailyDisabledEventTypes(t *testing.T) {
	disabled := make(map[uint32]struct{})
	for _, eventType := range defaultRenewDailyDisabledEventTypes() {
		disabled[eventType] = struct{}{}
	}

	for _, eventType := range []pb.EventType{
		pb.EventType_OPENAT,
		pb.EventType_IOCTL,
		pb.EventType_READ,
		pb.EventType_OPEN,
		pb.EventType_NETWORK_SENDTO,
		pb.EventType_NETWORK_RECVFROM,
		pb.EventType_SOCKET,
		pb.EventType_ACCEPT,
		pb.EventType_ACCEPT4,
		pb.EventType_TCP_STATE_CHANGE,
		pb.EventType_GENERIC_SYSCALL,
	} {
		if _, ok := disabled[uint32(eventType)]; !ok {
			t.Fatalf("daily profile should disable high-frequency event type %s", eventType)
		}
	}

	for _, eventType := range []pb.EventType{
		pb.EventType_EXECVE,
		pb.EventType_WRITE,
		pb.EventType_NETWORK_CONNECT,
		pb.EventType_DNS_QUERY,
	} {
		if _, ok := disabled[uint32(eventType)]; ok {
			t.Fatalf("daily profile should keep event type %s enabled", eventType)
		}
	}
}

func TestNormalizeDisabledEventTypes(t *testing.T) {
	got, err := normalizeDisabledEventTypes([]uint32{64, 1, 64, 255})
	if err != nil {
		t.Fatal(err)
	}
	want := []uint32{1, 64, 255}
	if len(got) != len(want) {
		t.Fatalf("normalized = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized = %v, want %v", got, want)
		}
	}
	if _, err := normalizeDisabledEventTypes([]uint32{256}); err == nil {
		t.Fatal("normalizeDisabledEventTypes accepted out-of-range value")
	}
}
