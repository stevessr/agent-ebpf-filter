package componentipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"testing"
)

func fixedStream(t *testing.T) StreamID {
	t.Helper()
	id, err := NewStreamID()
	if err != nil {
		t.Fatal(err)
	}
	if id == (StreamID{}) {
		t.Fatal("empty generated stream identifier")
	}
	return id
}

func TestSequencedEnvelopeRoundTripAndBounds(t *testing.T) {
	id := fixedStream(t)
	for _, kind := range []Kind{KindEvent, KindAlert, KindStatus} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			payload := []byte("event-or-health-data")
			raw, err := EncodeSequenced(kind, Sequenced{Stream: id, Sequence: 42, Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			payload[0] = 'X'
			decoded, err := DecodeSequenced(kind, raw)
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Stream != id || decoded.Sequence != 42 || string(decoded.Payload) != "event-or-health-data" {
				t.Fatalf("round trip: %+v", decoded)
			}
			decoded.Payload[0] = 'Z'
			if raw[SequencedHeaderBytes] != 'Z' {
				t.Fatal("decoder did not expose borrowed payload")
			}
		})
	}
	maximum := bytes.Repeat([]byte{'a'}, MaxSequencedPayloadBytes)
	raw, err := EncodeSequenced(KindEvent, Sequenced{Stream: id, Sequence: 1, Payload: maximum})
	if err != nil || len(raw) != MaxMessageBytes {
		t.Fatalf("maximum payload size: %d %v", len(raw), err)
	}
	if _, err := DecodeSequenced(KindEvent, raw); err != nil {
		t.Fatal(err)
	}
	_, err = EncodeSequenced(KindEvent, Sequenced{Stream: id, Sequence: 1, Payload: make([]byte, MaxSequencedPayloadBytes+1)})
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized envelope: %v", err)
	}
	if _, err := DecodeSequenced(KindEvent, append(raw, 0)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("oversized decoding: %v", err)
	}
}

func TestSequencedEnvelopeMalformedInput(t *testing.T) {
	id := fixedStream(t)
	valid, err := EncodeSequenced(KindEvent, Sequenced{Stream: id, Sequence: 1, Payload: []byte("ok")})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{
		name string
		kind Kind
		raw []byte
		want error
	}{
		{"wrong-kind", KindAck, valid, ErrFrameDenied},
		{"short", KindEvent, valid[:SequencedHeaderBytes], ErrInvalidSequence},
		{"empty", KindEvent, nil, ErrInvalidSequence},
		{"unsupported-version", KindEvent, func() []byte { r := bytes.Clone(valid); r[1] = 2; return r }(), ErrVersionMismatch},
		{"zero-stream", KindEvent, func() []byte { r := bytes.Clone(valid); clear(r[2:18]); return r }(), ErrInvalidSequence},
		{"zero-sequence", KindEvent, func() []byte { r := bytes.Clone(valid); clear(r[18:26]); return r }(), ErrInvalidSequence},
	} {
		t.Run(tc.name,func(t *testing.T){
			_,err:=DecodeSequenced(tc.kind,tc.raw)
			if !errors.Is(err,tc.want) { t.Fatalf("got %v, want %v",err,tc.want) }
		})
	}
	for _, frame := range []Sequenced{
		{Stream: StreamID{}, Sequence:1, Payload:[]byte("x")},
		{Stream:id, Sequence:0, Payload:[]byte("x")},
		{Stream:id, Sequence:1},
	} {
		if _, err := EncodeSequenced(KindStatus, frame); !errors.Is(err, ErrInvalidSequence) {
			t.Fatalf("invalid envelope accepted %+v: %v", frame, err)
		}
	}
	if _, err := EncodeSequenced(KindHeartbeat, Sequenced{Stream:id,Sequence:1,Payload:[]byte("x")}); !errors.Is(err,ErrFrameDenied) {
		t.Fatalf("heartbeat accepted as event: %v",err)
	}
}

func TestAckCodecRejectsWrongVersionSizeAndIdentity(t *testing.T) {
	id := fixedStream(t)
	raw, err := EncodeAck(Ack{Stream: id, Sequence: 13})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != SequencedHeaderBytes {
		t.Fatalf("unexpected ack bytes: %d", len(raw))
	}
	got, err := DecodeAck(raw)
	if err != nil || got != (Ack{Stream:id,Sequence:13}) {
		t.Fatalf("decoded ack: %+v %v",got,err)
	}
	if _, err := DecodeAck(append(bytes.Clone(raw), 0)); !errors.Is(err,ErrInvalidSequence) {
		t.Fatalf("trailing ack bytes accepted: %v",err)
	}
	if _, err := DecodeAck(raw[:len(raw)-1]); !errors.Is(err,ErrInvalidSequence) {
		t.Fatalf("short ack accepted: %v",err)
	}
	wrong := bytes.Clone(raw)
	binary.BigEndian.PutUint16(wrong[:2], SequencedSchemaVersion+1)
	if _, err := DecodeAck(wrong); !errors.Is(err,ErrVersionMismatch) {
		t.Fatalf("unsupported ack version accepted: %v",err)
	}
	if _, err := EncodeAck(Ack{Sequence: 1}); !errors.Is(err,ErrInvalidSequence) {
		t.Fatalf("empty stream ack accepted: %v",err)
	}
	if _, err := EncodeAck(Ack{Stream: id}); !errors.Is(err,ErrInvalidSequence) {
		t.Fatalf("zero sequence ack accepted: %v",err)
	}
}

func TestSequenceReceiverCommitAndGapHandling(t *testing.T) {
	id := fixedStream(t)
	receiver, err := NewSequenceReceiver(id)
	if err != nil { t.Fatal(err) }
	first := Sequenced{Stream:id,Sequence:1,Payload:[]byte("a")}
	if err := receiver.Check(first); err != nil { t.Fatal(err) }
	if receiver.LastCommitted()!=0 { t.Fatal("Check advanced unprocessed record") }
	if _, err := receiver.Commit(Sequenced{Stream:id,Sequence:3}); !errors.Is(err,ErrSequenceGap) {
		t.Fatalf("missing sequence: %v",err)
	}
	ack, err := receiver.Commit(first)
	if err != nil || ack.Sequence!=1 || ack.Stream!=id { t.Fatalf("commit: %+v %v",ack,err) }
	if receiver.LastCommitted()!=1 { t.Fatal("commit not recorded") }
	if _, err := receiver.Commit(first); !errors.Is(err,ErrSequenceReplay) { t.Fatalf("duplicate: %v",err) }
	if err := receiver.Check(Sequenced{Stream:id,Sequence:3}); !errors.Is(err,ErrSequenceGap) { t.Fatalf("gap: %v",err) }
	second:=Sequenced{Stream:id,Sequence:2,Payload:[]byte("b")}
	if _,err:=receiver.Commit(second);err!=nil { t.Fatal(err) }
	var foreign StreamID
	foreign[0]=1
	if foreign==id { foreign[0]^=0xff }
	if _,err:=receiver.Commit(Sequenced{Stream:foreign,Sequence:3});!errors.Is(err,ErrForeignStream) {
		t.Fatalf("foreign stream accepted: %v",err)
	}
	if receiver.LastCommitted()!=2 { t.Fatal("rejected record advanced state") }
	if _,err:=NewSequenceReceiver(StreamID{});!errors.Is(err,ErrInvalidSequence) { t.Fatalf("zero stream: %v",err) }
}

func TestAckTrackerRejectsForgedFutureAndStaleReceipts(t *testing.T) {
	id := fixedStream(t)
	tracker, err := NewAckTracker(id)
	if err != nil { t.Fatal(err) }
	if err:=tracker.MarkSent(2); !errors.Is(err,ErrSequenceGap){ t.Fatalf("skipped sent sequence: %v",err) }
	if err:=tracker.MarkSent(1);err!=nil { t.Fatal(err) }
	if err:=tracker.MarkSent(2);err!=nil { t.Fatal(err) }
	if err:=tracker.MarkSent(2); !errors.Is(err,ErrSequenceReplay){ t.Fatalf("duplicate sent: %v",err) }
	if err:=tracker.ApplyAck(Ack{Stream:id,Sequence:3});!errors.Is(err,ErrAckAhead){t.Fatalf("future ack: %v",err)}
	if err:=tracker.ApplyAck(Ack{Stream:id,Sequence:2});err!=nil { t.Fatal(err) }
	if tracker.LastAcked()!=2 { t.Fatal("cumulative ack not committed") }
	if err:=tracker.ApplyAck(Ack{Stream:id,Sequence:1});!errors.Is(err,ErrSequenceReplay){ t.Fatalf("stale ack: %v",err) }
	var foreign StreamID
	foreign[0]=1
	if foreign==id { foreign[0]^=0xff }
	if err:=tracker.ApplyAck(Ack{Stream:foreign,Sequence:3});!errors.Is(err,ErrForeignStream){t.Fatalf("foreign ack: %v",err)}
	if err:=tracker.ApplyAck(Ack{Stream:id,Sequence:0});!errors.Is(err,ErrInvalidSequence){t.Fatalf("zero ack: %v",err)}
	if tracker.LastAcked()!=2 { t.Fatal("rejected ack advanced tracker") }
}

func TestSequencedRoundTripOverAuthenticatedPipe(t *testing.T) {
	client,server:=handshakePair(t,RoleCollector,RoleEngine)
	id:=fixedStream(t)
	data:=Sequenced{Stream:id,Sequence:1,Payload:[]byte("collector payload")}
	collectorTracker,err:=NewAckTracker(id)
	if err!=nil{t.Fatal(err)}
	engineReceiver,err:=NewSequenceReceiver(id)
	if err!=nil{t.Fatal(err)}
	sent:=make(chan error,1)
	go func(){sent<-client.SendSequenced(KindEvent,data)}()
	received,err:=server.ReceiveCopy()
	if err!=nil { t.Fatal(err) }
	decoded,err:=DecodeSequenced(received.Kind,received.Payload)
	if err!=nil {t.Fatal(err)}
	if decoded.Sequence!=1 || decoded.Stream!=id || string(decoded.Payload)!="collector payload" {t.Fatalf("decoded: %+v",decoded)}
	if err:=engineReceiver.Check(decoded);err!=nil {t.Fatal(err)}
	receipt,err:=engineReceiver.Commit(decoded)
	if err!=nil {t.Fatal(err)}
	if err:=<-sent;err!=nil {t.Fatal(err)}
	if err:=collectorTracker.MarkSent(data.Sequence);err!=nil {t.Fatal(err)}
	acks:=make(chan error,1)
	go func(){acks<-server.SendAck(receipt)}()
	receivedAck,err:=client.ReceiveCopy()
	if err!=nil {t.Fatal(err)}
	if receivedAck.Kind!=KindAck {t.Fatalf("kind: %d",receivedAck.Kind)}
	decodedAck,err:=DecodeAck(receivedAck.Payload)
	if err!=nil {t.Fatal(err)}
	if err:=collectorTracker.ApplyAck(decodedAck);err!=nil {t.Fatal(err)}
	if err:=<-acks;err!=nil {t.Fatal(err)}
	if collectorTracker.LastAcked()!=1 {t.Fatal("ack not recorded")}
	if err:=client.SendSequenced(KindAlert,data);!errors.Is(err,ErrFrameDenied){t.Fatalf("collector forged alert: %v",err)}
}

func TestSequencedRejectsNilSessionWithoutSocket(t *testing.T){
	id:=fixedStream(t)
	data:=Sequenced{Stream:id,Sequence:1,Payload:[]byte("x")}
	var s *Session
	if err:=s.SendSequenced(KindEvent,data);!errors.Is(err,ErrInvalidConfig){t.Fatalf("nil session: %v",err)}
	if err:=s.SendAck(Ack{Stream:id,Sequence:1});!errors.Is(err,ErrInvalidConfig){t.Fatalf("nil ack session: %v",err)}
	if _,err:=NewAckTracker(StreamID{});!errors.Is(err,ErrInvalidSequence){t.Fatalf("invalid tracker: %v",err)}
	if err:=(*AckTracker)(nil).MarkSent(1);!errors.Is(err,ErrInvalidConfig){t.Fatalf("nil tracker: %v",err)}
	if err:=(*AckTracker)(nil).ApplyAck(Ack{Stream:id,Sequence:1});!errors.Is(err,ErrInvalidConfig){t.Fatalf("nil ack tracker: %v",err)}
	_ = net.ErrClosed
}
