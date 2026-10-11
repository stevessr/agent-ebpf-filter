package componentipc

import (
	"errors"
	"testing"
)

func TestReceiveRecordCopySequencedRoundTrip(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	stream := fixedStream(t)
	written := make(chan error, 1)
	go func() {
		written <- client.SendSequenced(KindEvent, Sequenced{
			Stream: stream, Sequence: 1, Payload: []byte("event"),
		})
	}()
	record, err := server.ReceiveRecordCopy()
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != KindEvent || record.Data.Stream != stream ||
		record.Data.Sequence != 1 || string(record.Data.Payload) != "event" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	ackSent := make(chan error, 1)
	go func() {
		ackSent <- server.SendAck(Ack{Stream: stream, Sequence: 1})
	}()
	receipt, err := client.ReceiveRecordCopy()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Kind != KindAck || receipt.Receipt.Stream != stream || receipt.Receipt.Sequence != 1 {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if err := <-ackSent; err != nil {
		t.Fatal(err)
	}
}

func TestReceiveRecordCopyFailsClosedOnMalformedBody(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	done := make(chan error, 1)
	go func() {
		done <- client.Send(KindEvent, []byte("not-a-sequenced-envelope"))
	}()
	_, err := server.ReceiveRecordCopy()
	if !errors.Is(err, ErrVersionMismatch) && !errors.Is(err, ErrInvalidSequence) {
		t.Fatalf("bad body accepted: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("malformed frame did not finish writing: %v", err)
	}
	if err := client.Send(KindEvent, []byte("later")); err == nil {
		t.Fatal("sender could continue after strict receiver closed socket")
	}
}

func TestDecodeRecordRejectsMixedKindsAndMalformedAck(t *testing.T) {
	stream := fixedStream(t)
	raw, err := EncodeAck(Ack{Stream: stream, Sequence: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRecord(KindHeartbeat, raw); !errors.Is(err, ErrFrameDenied) {
		t.Fatalf("unversioned heartbeat accepted: %v", err)
	}
	if _, err := DecodeRecord(KindAck, append(raw, 0)); !errors.Is(err, ErrInvalidSequence) {
		t.Fatalf("malformed ack accepted: %v", err)
	}
	if _, err := (*Session)(nil).ReceiveRecordCopy(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil session unexpectedly valid: %v", err)
	}
}
