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

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

var (
	errSignalProgramLogFrameTooLarge   = errors.New("signal program log compressed frame exceeds the size limit")
	errSignalProgramLogPayloadTooLarge = errors.New("signal program log decompressed payload exceeds the size limit")
	errSignalProgramLogTooManyFrames   = errors.New("signal program log exceeds the frame count limit")
)

type signalProgramLogMatch struct {
	selected SelectedProgramSignalLog
	reason   string
}

type signalProgramLogWorkItem struct {
	record  CapturedEventRecord
	matches []signalProgramLogMatch
}

func persistSignalProgramLog(record CapturedEventRecord) {
	item, ok := prepareSignalProgramLogWork(record)
	if !ok {
		return
	}
	accepted, active := signalProgramLogWriterStore.Enqueue(item)
	if accepted || active {
		return
	}
	// Unit tests and narrow library-style callers do not start runtime workers.
	// Preserve the historical synchronous behavior only when no writer
	// generation is active; a full/stopping active worker must stay non-blocking.
	persistSignalProgramLogWork(item)
}

func persistSignalProgramLogNow(record CapturedEventRecord) {
	item, ok := prepareSignalProgramLogWork(record)
	if !ok {
		return
	}
	persistSignalProgramLogWork(item)
}

func prepareSignalProgramLogWork(record CapturedEventRecord) (signalProgramLogWorkItem, bool) {
	if record.Event == nil {
		return signalProgramLogWorkItem{}, false
	}
	settings := runtimeSettingsStore.Snapshot().SignalProcessing
	normalizeSignalProcessingSettings(&settings)
	if len(settings.SelectedPrograms) == 0 {
		return signalProgramLogWorkItem{}, false
	}
	matches := make([]signalProgramLogMatch, 0, len(settings.SelectedPrograms))
	for _, selected := range settings.SelectedPrograms {
		if !selected.Enabled || strings.TrimSpace(selected.Program) == "" {
			continue
		}
		matched, reason := selectedProgramMatches(record.Event, selected.Program)
		if !matched {
			continue
		}
		matches = append(matches, signalProgramLogMatch{selected: selected, reason: reason})
	}
	if len(matches) == 0 {
		return signalProgramLogWorkItem{}, false
	}
	return signalProgramLogWorkItem{record: record, matches: matches}, true
}

func persistSignalProgramLogWork(item signalProgramLogWorkItem) (persisted, failed int, lastError string) {
	if item.record.Event == nil || len(item.matches) == 0 {
		return 0, 0, ""
	}
	for _, match := range item.matches {
		selected := match.selected
		path, err := resolveSignalProgramLogPath(selected)
		if err != nil {
			message := fmt.Sprintf("failed to resolve selected program signal log for %q: %v", selected.Program, err)
			reportSignalProgramLogError(message)
			failed++
			lastError = message
			continue
		}
		logRecord := &pb.ProgramSignalLogRecord{
			SchemaVersion: eventSchemaVersion,
			Program:       strings.TrimSpace(selected.Program),
			Reason:        match.reason,
			PersistedAt:   time.Now().UTC().UnixMilli(),
			CapturedEvent: recordToProtoCapturedEvent(item.record),
			SignalKind:    "selected_program",
		}
		if err := appendCompressedProtoRecordForSelected(selected, logRecord); err != nil {
			message := fmt.Sprintf("failed to persist selected program signal log %s: %v", path, err)
			reportSignalProgramLogError(message)
			failed++
			lastError = message
			continue
		}
		persisted++
	}
	return persisted, failed, lastError
}

func reportSignalProgramLogError(message string) {
	log.Printf("[WARN] %s", message)
	signalProcessingWorkerStore.noteError(message)
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
		populateSignalProgramLogStatus(&status, selected)
		statuses = append(statuses, status)
	}
	return statuses
}

func populateSignalProgramLogStatus(status *signalProgramLogStatus, selected SelectedProgramSignalLog) {
	if status == nil {
		return
	}
	signalProgramLogMu.Lock()
	defer signalProgramLogMu.Unlock()

	file, path, err := openSignalProgramLog(selected, os.O_RDONLY)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			status.Error = err.Error()
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		status.Error = err.Error()
		return
	}
	status.Exists = true
	status.SizeBytes = info.Size()
	status.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	signature := signalProgramLogSignature(info)
	if frames, errorText, ok := signalProgramLogFrameCountCacheStore.Lookup(path, signature); ok {
		if errorText == "" {
			status.FrameCount = frames
		} else {
			status.Error = errorText
		}
		return
	}
	if info.Size() < 0 || info.Size() > signalProgramLogMaxBytes {
		status.Error = fmt.Sprintf("signal program log exceeds %d bytes", signalProgramLogMaxBytes)
		signalProgramLogFrameCountCacheStore.Remember(path, signature, 0, status.Error)
		return
	}
	frames, err := countCompressedProtoFramesReader(file, signalProgramLogMaxBytes)
	if err != nil {
		status.Error = err.Error()
		signalProgramLogFrameCountCacheStore.Remember(path, signature, frames, status.Error)
		return
	}
	status.FrameCount = frames
	signalProgramLogFrameCountCacheStore.Remember(path, signature, frames, "")
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

	file, path, err := openSignalProgramLog(selected, os.O_CREATE|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	before := signalProgramLogSignature(info)
	if info.Size() > signalProgramLogMaxBytes-int64(len(payload)) {
		return fmt.Errorf("signal program log exceeds %d bytes", signalProgramLogMaxBytes)
	}
	for len(payload) > 0 {
		written, writeErr := file.Write(payload)
		if writeErr != nil {
			return writeErr
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[written:]
	}
	if info, statErr := file.Stat(); statErr == nil {
		signalProgramLogFrameCountCacheStore.Advance(path, before, signalProgramLogSignature(info))
	}
	return nil
}

func marshalCompressedProtoFrame(message proto.Message) ([]byte, error) {
	if message == nil {
		return nil, errors.New("signal program log protobuf message is nil")
	}
	payload, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	if len(payload) > signalProgramLogMaxPayloadBytes {
		return nil, fmt.Errorf("%w: %d bytes", errSignalProgramLogPayloadTooLarge, len(payload))
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
	if compressed.Len() > signalProgramLogMaxCompressedFrameBytes {
		return nil, fmt.Errorf("%w: %d bytes", errSignalProgramLogFrameTooLarge, compressed.Len())
	}
	frame := make([]byte, 4+compressed.Len())
	binary.BigEndian.PutUint32(frame, uint32(compressed.Len()))
	copy(frame[4:], compressed.Bytes())
	return frame, nil
}

func readCompressedProtoFrames(path string, newMessage func() proto.Message) ([]proto.Message, error) {
	if newMessage == nil {
		return nil, errors.New("signal program log message factory is nil")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("open signal program log: invalid file descriptor")
	}
	defer file.Close()
	if err := validateRecordingRegularFile(file); err != nil {
		return nil, fmt.Errorf("validate signal program log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > signalProgramLogMaxBytes {
		return nil, fmt.Errorf("signal program log exceeds %d bytes", signalProgramLogMaxBytes)
	}
	limited := &io.LimitedReader{R: file, N: signalProgramLogMaxBytes + 1}
	return readCompressedProtoFramesReader(limited, newMessage)
}

func readCompressedProtoFramesReader(reader io.Reader, newMessage func() proto.Message) ([]proto.Message, error) {
	return readCompressedProtoFramesReaderWithLimits(
		reader,
		newMessage,
		signalProgramLogMaxFrames,
		signalProgramLogMaxCompressedFrameBytes,
		signalProgramLogMaxPayloadBytes,
	)
}

func readCompressedProtoFramesReaderWithLimits(
	reader io.Reader,
	newMessage func() proto.Message,
	maxFrames int,
	maxCompressedFrameBytes int64,
	maxPayloadBytes int64,
) ([]proto.Message, error) {
	if reader == nil || newMessage == nil {
		return nil, errors.New("signal program log reader or message factory is nil")
	}
	if maxFrames <= 0 || maxCompressedFrameBytes <= 0 || maxPayloadBytes <= 0 {
		return nil, errors.New("signal program log read limits must be positive")
	}
	var messages []proto.Message
	for frameIndex := 0; ; frameIndex++ {
		var frameLen uint32
		if err := binary.Read(reader, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return messages, nil
			}
			return nil, fmt.Errorf("read signal program log frame %d header: %w", frameIndex, err)
		}
		if frameIndex >= maxFrames {
			return nil, fmt.Errorf("%w: maximum %d", errSignalProgramLogTooManyFrames, maxFrames)
		}
		if frameLen == 0 {
			return nil, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		if int64(frameLen) > maxCompressedFrameBytes {
			return nil, fmt.Errorf("%w at frame %d: %d bytes", errSignalProgramLogFrameTooLarge, frameIndex, frameLen)
		}
		compressed := make([]byte, frameLen)
		if _, err := io.ReadFull(reader, compressed); err != nil {
			return nil, fmt.Errorf("read signal program log frame %d: %w", frameIndex, err)
		}
		gz, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, fmt.Errorf("open signal program log frame %d gzip payload: %w", frameIndex, err)
		}
		payload, readErr := io.ReadAll(io.LimitReader(gz, maxPayloadBytes+1))
		closeErr := gz.Close()
		if readErr != nil {
			return nil, fmt.Errorf("decompress signal program log frame %d: %w", frameIndex, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close signal program log frame %d: %w", frameIndex, closeErr)
		}
		if int64(len(payload)) > maxPayloadBytes {
			return nil, fmt.Errorf("%w at frame %d", errSignalProgramLogPayloadTooLarge, frameIndex)
		}
		msg := newMessage()
		if msg == nil {
			return nil, fmt.Errorf("signal program log message factory returned nil at frame %d", frameIndex)
		}
		if err := proto.Unmarshal(payload, msg); err != nil {
			return nil, fmt.Errorf("decode signal program log frame %d: %w", frameIndex, err)
		}
		messages = append(messages, msg)
	}
}

func countCompressedProtoFramesReader(file io.Reader, maxBytes int64) (int, error) {
	return countCompressedProtoFramesReaderWithLimits(
		file,
		maxBytes,
		signalProgramLogMaxFrames,
		signalProgramLogMaxCompressedFrameBytes,
	)
}

func countCompressedProtoFramesReaderWithLimits(
	file io.Reader,
	maxBytes int64,
	maxFrames int,
	maxCompressedFrameBytes int64,
) (int, error) {
	if file == nil {
		return 0, errors.New("signal program log reader is nil")
	}
	if maxBytes <= 0 || maxBytes > signalProgramLogMaxBytes {
		maxBytes = signalProgramLogMaxBytes
	}
	if maxFrames <= 0 || maxCompressedFrameBytes <= 0 {
		return 0, errors.New("signal program log count limits must be positive")
	}
	limited := &io.LimitedReader{R: file, N: maxBytes + 1}
	count := 0
	for {
		var frameLen uint32
		if err := binary.Read(limited, binary.BigEndian, &frameLen); err != nil {
			if errors.Is(err, io.EOF) {
				return count, nil
			}
			if limited.N == 0 {
				return count, fmt.Errorf("signal program log exceeds %d bytes", maxBytes)
			}
			return count, err
		}
		if count >= maxFrames {
			return count, fmt.Errorf("%w: maximum %d", errSignalProgramLogTooManyFrames, maxFrames)
		}
		if frameLen == 0 {
			return count, fmt.Errorf("invalid compressed proto frame length %d", frameLen)
		}
		if int64(frameLen) > maxCompressedFrameBytes {
			return count, fmt.Errorf("%w at frame %d: %d bytes", errSignalProgramLogFrameTooLarge, count, frameLen)
		}
		if _, err := io.CopyN(io.Discard, limited, int64(frameLen)); err != nil {
			if limited.N == 0 {
				return count, fmt.Errorf("signal program log exceeds %d bytes", maxBytes)
			}
			return count, err
		}
		if limited.N <= 0 {
			return count, fmt.Errorf("signal program log exceeds %d bytes", maxBytes)
		}
		count++
	}
}
