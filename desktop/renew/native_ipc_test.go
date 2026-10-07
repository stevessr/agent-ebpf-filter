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
	if got.Type != "execve" || got.Tag != "AI Agent" || got.Path != "/usr/bin/git" {
		t.Fatalf("legacy event fields missing: %+v", got)
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
