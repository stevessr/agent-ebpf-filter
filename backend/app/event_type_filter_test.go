package app

import "testing"

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
