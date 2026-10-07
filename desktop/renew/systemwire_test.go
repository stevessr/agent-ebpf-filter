package main

import (
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestDecodeSystemStatsNarrowWireContract(t *testing.T) {
	process := []byte{}
	process = protowire.AppendTag(process, 1, protowire.VarintType)
	process = protowire.AppendVarint(process, 4242)
	process = protowire.AppendTag(process, 2, protowire.VarintType)
	process = protowire.AppendVarint(process, 42)
	process = appendStringField(process, 3, "codex")
	process = protowire.AppendTag(process, 4, protowire.Fixed64Type)
	process = protowire.AppendFixed64(process, math.Float64bits(12.5))
	process = protowire.AppendTag(process, 5, protowire.Fixed32Type)
	process = protowire.AppendFixed32(process, math.Float32bits(3.25))
	process = appendStringField(process, 6, "steve")
	process = appendStringField(process, 10, "codex run task")

	cpu := []byte{}
	cpu = protowire.AppendTag(cpu, 1, protowire.Fixed64Type)
	cpu = protowire.AppendFixed64(cpu, math.Float64bits(18.75))

	memory := []byte{}
	memory = appendVarintField(memory, 1, 16*1024)
	memory = appendVarintField(memory, 2, 8*1024)
	memory = protowire.AppendTag(memory, 3, protowire.Fixed32Type)
	memory = protowire.AppendFixed32(memory, math.Float32bits(50))

	io := []byte{}
	io = appendVarintField(io, 3, 1234)
	io = appendVarintField(io, 4, 5678)

	payload := []byte{}
	payload = appendMessageField(payload, 1, process)
	// Unknown GPU field must be skipped without disturbing later fields.
	payload = appendMessageField(payload, 2, []byte{0x08, 0x01})
	payload = appendMessageField(payload, 3, cpu)
	payload = appendMessageField(payload, 4, memory)
	payload = appendMessageField(payload, 5, io)

	got, err := decodeSystemStats(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got.CPUPercent != 18.75 || got.MemPercent != 50 || got.MemUsed != 8*1024 || got.MemTotal != 16*1024 {
		t.Fatalf("unexpected system totals: %#v", got)
	}
	if got.NetRecv != 1234 || got.NetSent != 5678 {
		t.Fatalf("unexpected network totals: %#v", got)
	}
	if len(got.Processes) != 1 {
		t.Fatalf("process count = %d", len(got.Processes))
	}
	processGot := got.Processes[0]
	if processGot.PID != 4242 || processGot.PPID != 42 || processGot.Name != "codex" ||
		processGot.CPU != 12.5 || processGot.Memory != 3.25 ||
		processGot.User != "steve" || processGot.Command != "codex run task" {
		t.Fatalf("unexpected process: %#v", processGot)
	}
}

func TestDecodeSystemStatsRejectsWrongWireType(t *testing.T) {
	payload := []byte{}
	payload = protowire.AppendTag(payload, 3, protowire.VarintType)
	payload = protowire.AppendVarint(payload, 1)
	if _, err := decodeSystemStats(payload); err == nil {
		t.Fatal("expected malformed CPUInfo wire type to fail")
	}
}

func appendMessageField(dst []byte, number protowire.Number, value []byte) []byte {
	dst = protowire.AppendTag(dst, number, protowire.BytesType)
	return protowire.AppendBytes(dst, value)
}

func appendStringField(dst []byte, number protowire.Number, value string) []byte {
	return appendMessageField(dst, number, []byte(value))
}

func appendVarintField(dst []byte, number protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, number, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}
