package app

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMarshalCompressedProtoFrameRejectsOversizedPayload(t *testing.T) {
	_, err := marshalCompressedProtoFrame(&wrapperspb.BytesValue{
		Value: make([]byte, signalProgramLogMaxPayloadBytes+1),
	})
	if !errors.Is(err, errSignalProgramLogPayloadTooLarge) {
		t.Fatalf("marshalCompressedProtoFrame() error = %v, want payload size error", err)
	}
	if _, err := marshalCompressedProtoFrame(nil); err == nil {
		t.Fatal("marshalCompressedProtoFrame(nil) succeeded")
	}
}

func TestNormalizeSignalProcessingSettingsBoundsSelectedPrograms(t *testing.T) {
	settings := SignalProcessingSettings{
		SelectedPrograms: make([]SelectedProgramSignalLog, signalMaxSelectedPrograms+10),
	}
	for index := range settings.SelectedPrograms {
		settings.SelectedPrograms[index] = SelectedProgramSignalLog{Program: "codex", Enabled: true}
	}
	normalizeSignalProcessingSettings(&settings)
	if len(settings.SelectedPrograms) != signalMaxSelectedPrograms {
		t.Fatalf("selected program count = %d, want %d", len(settings.SelectedPrograms), signalMaxSelectedPrograms)
	}
}

func TestReadCompressedProtoFramesReaderEnforcesBounds(t *testing.T) {
	newMessage := func() proto.Message { return &wrapperspb.StringValue{} }
	frame, err := marshalCompressedProtoFrame(wrapperspb.String("ok"))
	if err != nil {
		t.Fatalf("marshalCompressedProtoFrame() error = %v", err)
	}

	t.Run("exact frame limit", func(t *testing.T) {
		wire := append(append([]byte(nil), frame...), frame...)
		messages, err := readCompressedProtoFramesReaderWithLimits(bytes.NewReader(wire), newMessage, 2, int64(len(frame)), 64)
		if err != nil {
			t.Fatalf("read exact frame limit: %v", err)
		}
		if len(messages) != 2 {
			t.Fatalf("message count = %d, want 2", len(messages))
		}
	})

	t.Run("too many frames", func(t *testing.T) {
		wire := append(append(append([]byte(nil), frame...), frame...), frame...)
		_, err := readCompressedProtoFramesReaderWithLimits(bytes.NewReader(wire), newMessage, 2, int64(len(frame)), 64)
		if !errors.Is(err, errSignalProgramLogTooManyFrames) {
			t.Fatalf("read excessive frames error = %v, want frame count error", err)
		}
	})

	t.Run("compressed frame too large", func(t *testing.T) {
		var wire bytes.Buffer
		if err := binary.Write(&wire, binary.BigEndian, uint32(9)); err != nil {
			t.Fatalf("write frame header: %v", err)
		}
		_, err := readCompressedProtoFramesReaderWithLimits(&wire, newMessage, 1, 8, 64)
		if !errors.Is(err, errSignalProgramLogFrameTooLarge) {
			t.Fatalf("read oversized frame error = %v, want compressed frame size error", err)
		}
	})

	t.Run("truncated header", func(t *testing.T) {
		_, err := readCompressedProtoFramesReaderWithLimits(bytes.NewReader([]byte{0, 0, 0}), newMessage, 1, 64, 64)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("read truncated header error = %v, want unexpected EOF", err)
		}
	})

	t.Run("truncated frame", func(t *testing.T) {
		wire := []byte{0, 0, 0, 2, 1}
		_, err := readCompressedProtoFramesReaderWithLimits(bytes.NewReader(wire), newMessage, 1, 64, 64)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("read truncated frame error = %v, want unexpected EOF", err)
		}
	})

	t.Run("nil message factory result", func(t *testing.T) {
		_, err := readCompressedProtoFramesReaderWithLimits(bytes.NewReader(frame), func() proto.Message { return nil }, 1, int64(len(frame)), 64)
		if err == nil {
			t.Fatal("nil message factory result was accepted")
		}
	})
}

func TestReadCompressedProtoFramesReaderRejectsDecompressionBomb(t *testing.T) {
	const maxPayloadBytes = 64
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(make([]byte, maxPayloadBytes+1)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	var wire bytes.Buffer
	if err := binary.Write(&wire, binary.BigEndian, uint32(compressed.Len())); err != nil {
		t.Fatalf("write frame header: %v", err)
	}
	if _, err := wire.Write(compressed.Bytes()); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	_, err := readCompressedProtoFramesReaderWithLimits(
		&wire,
		func() proto.Message { return &wrapperspb.BytesValue{} },
		1,
		int64(compressed.Len()),
		maxPayloadBytes,
	)
	if !errors.Is(err, errSignalProgramLogPayloadTooLarge) {
		t.Fatalf("read decompression bomb error = %v, want payload size error", err)
	}
}

func TestCountCompressedProtoFramesReaderEnforcesBounds(t *testing.T) {
	frame := []byte{0, 0, 0, 1, 1}

	t.Run("exact frame limit", func(t *testing.T) {
		wire := append(append([]byte(nil), frame...), frame...)
		count, err := countCompressedProtoFramesReaderWithLimits(bytes.NewReader(wire), int64(len(wire)), 2, 1)
		if err != nil || count != 2 {
			t.Fatalf("count exact frame limit = %d, %v; want 2, nil", count, err)
		}
	})

	t.Run("too many frames", func(t *testing.T) {
		wire := append(append(append([]byte(nil), frame...), frame...), frame...)
		count, err := countCompressedProtoFramesReaderWithLimits(bytes.NewReader(wire), int64(len(wire)), 2, 1)
		if count != 2 || !errors.Is(err, errSignalProgramLogTooManyFrames) {
			t.Fatalf("count excessive frames = %d, %v; want 2 and frame count error", count, err)
		}
	})

	t.Run("byte limit", func(t *testing.T) {
		count, err := countCompressedProtoFramesReaderWithLimits(bytes.NewReader(frame), int64(len(frame)-1), 2, 1)
		if count != 0 || err == nil {
			t.Fatalf("count over byte limit = %d, %v; want 0 and error", count, err)
		}
	})

	t.Run("compressed frame limit", func(t *testing.T) {
		oversizedFrame := []byte{0, 0, 0, 2, 1, 2}
		count, err := countCompressedProtoFramesReaderWithLimits(bytes.NewReader(oversizedFrame), int64(len(oversizedFrame)), 2, 1)
		if count != 0 || !errors.Is(err, errSignalProgramLogFrameTooLarge) {
			t.Fatalf("count oversized frame = %d, %v; want 0 and frame size error", count, err)
		}
	})
}

func TestReadCompressedProtoFramesRejectsUnsafeAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	frame, err := marshalCompressedProtoFrame(wrapperspb.String("safe"))
	if err != nil {
		t.Fatalf("marshalCompressedProtoFrame() error = %v", err)
	}
	target := filepath.Join(dir, "target.pb.gzlog")
	if err := os.WriteFile(target, frame, 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	newMessage := func() proto.Message { return &wrapperspb.StringValue{} }
	if messages, err := readCompressedProtoFrames(target, newMessage); err != nil || len(messages) != 1 {
		t.Fatalf("read regular file = %d messages, %v; want one message", len(messages), err)
	}

	symlink := filepath.Join(dir, "symlink.pb.gzlog")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if _, err := readCompressedProtoFrames(symlink, newMessage); err == nil {
		t.Fatal("symlink signal program log was accepted")
	}

	hardlink := filepath.Join(dir, "hardlink.pb.gzlog")
	if err := os.Link(target, hardlink); err != nil {
		t.Fatalf("create hardlink: %v", err)
	}
	if _, err := readCompressedProtoFrames(hardlink, newMessage); err == nil {
		t.Fatal("hard-linked signal program log was accepted")
	}

	oversized := filepath.Join(dir, "oversized.pb.gzlog")
	file, err := os.OpenFile(oversized, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create oversized file: %v", err)
	}
	if err := file.Truncate(signalProgramLogMaxBytes + 1); err != nil {
		_ = file.Close()
		t.Fatalf("truncate oversized file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close oversized file: %v", err)
	}
	if _, err := readCompressedProtoFrames(oversized, newMessage); err == nil {
		t.Fatal("oversized signal program log was accepted")
	}
}
