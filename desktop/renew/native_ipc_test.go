package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"
)

func appendProtoString(dst []byte, field protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, field, protowire.BytesType)
	return protowire.AppendBytes(dst, []byte(value))
}

func appendProtoVarint(dst []byte, field protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}

func appendProtoDouble(dst []byte, field protowire.Number, value float64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.Fixed64Type)
	return protowire.AppendFixed64(dst, math.Float64bits(value))
}

func TestDecodeEventEnvelopeSummary(t *testing.T) {
	var legacy []byte
	legacy = appendProtoVarint(legacy, 1, 41)
	legacy = appendProtoVarint(legacy, 2, 7)
	legacy = appendProtoVarint(legacy, 3, 1000)
	legacy = appendProtoString(legacy, 4, "execve")
	legacy = appendProtoString(legacy, 5, "AI Agent")
	legacy = appendProtoString(legacy, 6, "codex")
	legacy = appendProtoString(legacy, 7, "/usr/bin/git")
	legacy = appendProtoString(legacy, 36, "ALLOW")
	legacy = appendProtoDouble(legacy, 37, 0.15)

	capturedAt := time.Date(2026, 10, 7, 10, 0, 0, 123000000, time.UTC)
	var envelope []byte
	envelope = appendProtoString(envelope, 2, "evt_native_1")
	envelope = appendProtoVarint(envelope, 3, uint64(capturedAt.UnixNano()))
	envelope = appendProtoString(envelope, 5, "run-1")
	envelope = appendProtoString(envelope, 7, "conversation-1")
	envelope = appendProtoString(envelope, 10, "shell")
	envelope = appendProtoVarint(envelope, 13, 42)
	envelope = appendProtoString(envelope, 18, "codex")
	envelope = appendProtoString(envelope, 23, "ALERT")
	envelope = appendProtoDouble(envelope, 24, 0.9)
	envelope = appendProtoVarint(envelope, 25, 8)
	envelope = protowire.AppendTag(envelope, 26, protowire.BytesType)
	envelope = protowire.AppendBytes(envelope, legacy)

	got, err := decodeEventEnvelopeSummary(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "evt_native_1" || got.PID != 42 || got.PPID != 7 {
		t.Fatalf("identity mismatch: %+v", got)
	}
	if got.Type != "execve" || got.Tag != "AI Agent" || got.Target != "/usr/bin/git" {
		t.Fatalf("legacy compact fields missing: %+v", got)
	}
	if got.Decision != "ALERT" || math.Abs(got.RiskScore-0.9) > 1e-9 {
		t.Fatalf("envelope policy fields did not win: %+v", got)
	}
	if got.AgentRunID != "run-1" || got.ConversationID != "conversation-1" || got.ToolName != "shell" {
		t.Fatalf("context fields missing: %+v", got)
	}
	if got.ReceivedAtMS != capturedAt.UnixMilli() {
		t.Fatalf("timestamp = %d, want %d", got.ReceivedAtMS, capturedAt.UnixMilli())
	}
}


func TestDecodeDesktopEventSummary(t *testing.T) {
	var data []byte
	data = appendProtoString(data, 1, "evt_compact_1")
	data = appendProtoVarint(data, 2, 123456)
	data = appendProtoVarint(data, 3, 42)
	data = appendProtoVarint(data, 4, 7)
	data = appendProtoVarint(data, 5, 41)
	data = appendProtoVarint(data, 6, 8)
	data = appendProtoString(data, 7, "NETWORK_CONNECT")
	data = appendProtoString(data, 8, "AI Agent")
	data = appendProtoString(data, 9, "codex")
	data = appendProtoString(data, 10, "example.com:443")
	data = appendProtoVarint(data, 11, 1)
	data = appendProtoVarint(data, 12, 4096)
	data = appendProtoString(data, 13, "ALERT")
	data = appendProtoDouble(data, 14, 72)
	data = appendProtoString(data, 15, "run-1")
	data = appendProtoString(data, 16, "conv-1")
	data = appendProtoString(data, 17, "shell")
	data = appendProtoVarint(data, 18, 1)

	got, err := decodeDesktopEventSummary(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventID != "evt_compact_1" || got.PID != 42 || got.PPID != 7 || got.RootAgentPID != 41 {
		t.Fatalf("identity mismatch: %+v", got)
	}
	if !got.Network || got.Target != "example.com:443" || got.NetBytes != 4096 {
		t.Fatalf("network projection mismatch: %+v", got)
	}
	if got.AgentRunID != "run-1" || got.ConversationID != "conv-1" || !got.HasAgentContext {
		t.Fatalf("agent context mismatch: %+v", got)
	}
	if got.Decision != "ALERT" || got.RiskScore != 72 {
		t.Fatalf("risk mismatch: %+v", got)
	}
}

func TestReadNativeFrame(t *testing.T) {
	payload := []byte{1, 2, 3, 4}
	var stream bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)+1))
	stream.Write(header[:])
	stream.WriteByte(nativeFrameSystemStats)
	stream.Write(payload)

	kind, got, err := readNativeFrame(&stream)
	if err != nil {
		t.Fatal(err)
	}
	if kind != nativeFrameSystemStats || !bytes.Equal(got, payload) {
		t.Fatalf("frame kind=%d payload=%v", kind, got)
	}
}


func TestReadNativeFrameIntoReusesBuffer(t *testing.T) {
	var stream bytes.Buffer
	writeFrame := func(kind byte, payload []byte) {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], uint32(len(payload)+1))
		stream.Write(header[:])
		stream.WriteByte(kind)
		stream.Write(payload)
	}
	writeFrame(nativeFrameEventEnvelope, []byte{1, 2, 3, 4})
	writeFrame(nativeFrameSystemStats, []byte{5, 6})

	kind, first, buf, err := readNativeFrameInto(&stream, nil)
	if err != nil {
		t.Fatal(err)
	}
	if kind != nativeFrameEventEnvelope || !bytes.Equal(first, []byte{1, 2, 3, 4}) {
		t.Fatalf("first frame kind=%d payload=%v", kind, first)
	}
	if cap(buf) < 5 {
		t.Fatalf("unexpected buffer capacity %d", cap(buf))
	}
	firstPtr := &buf[0]

	kind, second, reused, err := readNativeFrameInto(&stream, buf)
	if err != nil {
		t.Fatal(err)
	}
	if kind != nativeFrameSystemStats || !bytes.Equal(second, []byte{5, 6}) {
		t.Fatalf("second frame kind=%d payload=%v", kind, second)
	}
	if &reused[0] != firstPtr {
		t.Fatal("frame buffer was not reused")
	}
}
