package componentipc

import (
	"errors"
	"testing"
)

func TestReceiveCopyOwnsPayloadAcrossNextRead(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	done := make(chan error, 1)
	go func() {
		if err := client.Send(KindEvent, []byte("first")); err != nil {
			done <- err
			return
		}
		done <- client.Send(KindEvent, []byte("other"))
	}()
	owned, err := server.ReceiveCopy()
	if err != nil {
		t.Fatal(err)
	}
	if owned.Kind != KindEvent || string(owned.Payload) != "first" {
		t.Fatalf("unexpected copied payload: %+v", owned)
	}
	next, err := server.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if string(next.Payload) != "other" {
		t.Fatalf("unexpected second payload: %q", next.Payload)
	}
	if string(owned.Payload) != "first" {
		t.Fatalf("later Receive mutated owned payload: %q", owned.Payload)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReceiveCopyRejectsMissingSession(t *testing.T) {
	if _, err := (*Session)(nil).ReceiveCopy(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil ReceiveCopy: %v", err)
	}
	if _, err := (&Session{}).ReceiveCopy(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("empty ReceiveCopy: %v", err)
	}
}
