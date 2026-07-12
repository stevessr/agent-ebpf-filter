package app

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"

	"google.golang.org/protobuf/proto"
)

func persistSignalProgramLog(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	if len(settings.SelectedPrograms) == 0 {
		return
	}
	for _, selected := range settings.SelectedPrograms {
		if !selected.Enabled || strings.TrimSpace(selected.Program) == "" {
			continue
		}
		matched, reason := selectedProgramMatches(record.Event, selected.Program)
		if !matched {
			continue
		}
		path, err := resolveSignalProgramLogPath(selected)
		if err != nil {
			message := fmt.Sprintf("failed to resolve selected program signal log for %q: %v", selected.Program, err)
			log.Printf("[WARN] %s", message)
			signalProcessingWorkerStore.noteError(message)
			continue
		}
		logRecord := &pb.ProgramSignalLogRecord{
			SchemaVersion: eventSchemaVersion,
			Program:       strings.TrimSpace(selected.Program),
			Reason:        reason,
			PersistedAt:   time.Now().UTC().UnixMilli(),
			CapturedEvent: recordToProtoCapturedEvent(record),
			SignalKind:    "selected_program",
		}
		if err := appendCompressedProtoRecordForSelected(selected, logRecord); err != nil {
			message := fmt.Sprintf("failed to persist selected program signal log %s: %v", path, err)
			log.Printf("[WARN] %s", message)
			signalProcessingWorkerStore.noteError(message)
		}
	}
}

func recordToProtoCapturedEvent(record CapturedEventRecord) *pb.CapturedEventRecord {
	record = normalizeCapturedEventRecord(record)
	return &pb.CapturedEventRecord{
		Event:     record.Event,
		Timestamp: record.ReceivedAt.UnixMilli(),
		Envelope:  record.Envelope,
	}
}

func selectedProgramMatches(event *pb.Event, program string) (bool, string) {
	if event == nil {
		return false, ""
	}
	program = strings.TrimSpace(program)
	if program == "" {
		return false, ""
	}
	candidates := []struct {
		label string
		value string
	}{
		{label: "comm", value: event.GetComm()},
		{label: "path", value: event.GetPath()},
		{label: "path_basename", value: filepath.Base(event.GetPath())},
		{label: "extra_path", value: event.GetExtraPath()},
		{label: "extra_path_basename", value: filepath.Base(event.GetExtraPath())},
		{label: "tool_name", value: event.GetToolName()},
	}
	for _, candidate := range candidates {
		value := strings.TrimSpace(candidate.value)
		if value == "." || value == "" {
			continue
		}
		if signalProgramPatternMatches(program, value) {
			return true, fmt.Sprintf("selected program %q matched %s=%q", program, candidate.label, value)
		}
	}
	return false, ""
}

func signalProgramPatternMatches(pattern, value string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	value = strings.ToLower(strings.TrimSpace(value))
	if pattern == "" || value == "" {
		return false
	}
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		if ok, err := filepath.Match(pattern, value); err == nil && ok {
			return true
		}
		if ok, err := filepath.Match(pattern, filepath.Base(value)); err == nil && ok {
			return true
		}
	}
	return pattern == value || pattern == filepath.Base(value)
}

func selectedProgramLogStatuses(settings SignalProcessingSettings) []signalProgramLogStatus {
	normalizeSignalProcessingSettings(&settings)
	statuses := make([]signalProgramLogStatus, 0, len(settings.SelectedPrograms))
	for _, selected := range settings.SelectedPrograms {
		program := strings.TrimSpace(selected.Program)
		if program == "" {
			continue
		}
		path, pathErr := resolveSignalProgramLogPath(selected)
		status := signalProgramLogStatus{
			Program: program,
			Enabled: selected.Enabled,
			Path:    path,
		}
		if pathErr != nil {
			status.Error = pathErr.Error()
			statuses = append(statuses, status)
			continue
		}
		file, _, err := openSignalProgramLog(selected, os.O_RDONLY)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				status.Error = err.Error()
			}
			statuses = append(statuses, status)
			continue
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			status.Error = err.Error()
			statuses = append(statuses, status)
			continue
		}
		status.Exists = true
		status.SizeBytes = info.Size()
		status.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		if frames, err := countCompressedProtoFramesReader(file, signalProgramLogMaxBytes); err == nil {
			status.FrameCount = frames
		} else {
			status.Error = err.Error()
		}
		_ = file.Close()
		statuses = append(statuses, status)
	}
	return statuses
}

func resolveSelectedProgramLog(settings SignalProcessingSettings, program string) (SelectedProgramSignalLog, bool) {
	normalizeSignalProcessingSettings(&settings)
	program = strings.TrimSpace(program)
	if program == "" {
		return SelectedProgramSignalLog{}, false
	}
	for _, selected := range settings.SelectedPrograms {
		if strings.EqualFold(strings.TrimSpace(selected.Program), program) || strings.EqualFold(sanitizeSignalFilename(selected.Program), program) {
			return selected, true
		}
	}
	return SelectedProgramSignalLog{}, false
}

func sanitizeSignalFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "program"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		out = "program"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func expandSignalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		return platform.GetRealHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(platform.GetRealHomeDir(), strings.TrimPrefix(path, "~/"))
	}
	return path
}

func appendCompressedProtoRecordForSelected(selected SelectedProgramSignalLog, message proto.Message) error {
	payload, err := marshalCompressedProtoFrame(message)
	if err != nil {
		return err
	}

	signalProgramLogMu.Lock()
	defer signalProgramLogMu.Unlock()

	file, _, err := openSignalProgramLog(selected, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > signalProgramLogMaxBytes-int64(len(payload)) {
		return fmt.Errorf("signal program log exceeds %d bytes", signalProgramLogMaxBytes)
	}
	_, err = file.Write(payload)
	return err
}

func marshalCompressedProtoFrame(message proto.Message) ([]byte, error) {
	payload, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(payload); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	frame := make([]byte, 4+compressed.Len())
	binary.BigEndian.PutUint32(frame, uint32(compressed.Len()))
	copy(frame[4:], compressed.Bytes())
	return frame, nil
}

func readCompressedProtoFrames(path string, newMessage func() proto.Message) ([]proto.Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []proto.Message
	for {
		var frameLen uint32
		if err := binary.Read(file, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return messages, nil
			}
			return nil, err
		}
		if frameLen == 0 || frameLen > 64*1024*1024 {
			return nil, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		compressed := make([]byte, frameLen)
		if _, err := io.ReadFull(file, compressed); err != nil {
			return nil, err
		}
		gz, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, err
		}
		payload, readErr := io.ReadAll(gz)
		closeErr := gz.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		msg := newMessage()
		if err := proto.Unmarshal(payload, msg); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
}

func countCompressedProtoFramesReader(file io.Reader, maxBytes int64) (int, error) {
	limited := &io.LimitedReader{R: file, N: maxBytes + 1}
	count := 0
	for {
		var frameLen uint32
		if err := binary.Read(limited, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			return count, err
		}
		if frameLen == 0 || frameLen > 64*1024*1024 {
			return count, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		if _, err := io.CopyN(io.Discard, limited, int64(frameLen)); err != nil {
			return count, err
		}
		count++
		if limited.N <= 0 {
			return count, fmt.Errorf("signal program log exceeds %d bytes", maxBytes)
		}
	}
}
