package recording

import (
	"agent-ebpf-filter/app/platform"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// MarshalRecord encodes one JSONL line. Only ReceivedAt and Event are
// persisted; the envelope is derived again on read, so it is deliberately
// not built (or cloned) here.
func MarshalRecord(record CapturedEventRecord) ([]byte, error) {
	if record.Event == nil {
		return nil, errors.New("event recording record has no event")
	}
	payload, err := json.Marshal(&record)
	if err != nil {
		return nil, err
	}
	if len(payload) > eventRecordingMaxRecordBytes {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", errEventRecordingRecordTooLarge, len(payload), eventRecordingMaxRecordBytes)
	}
	return payload, nil
}

func marshalNormalizedRecord(record CapturedEventRecord) ([]byte, error) {
	payload, err := json.Marshal(&record)
	if err != nil {
		return nil, err
	}
	if len(payload) > eventRecordingMaxRecordBytes {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", errEventRecordingRecordTooLarge, len(payload), eventRecordingMaxRecordBytes)
	}
	return payload, nil
}

func ReadCapturedEventsFile(path string, limit int) ([]CapturedEventRecord, error) {
	return ReadCapturedEventsFileContext(context.Background(), path, limit)
}

func ReadCapturedEventsFileContext(ctx context.Context, path string, limit int) ([]CapturedEventRecord, error) {
	records, _, err := ReadCapturedEventsFileAtRootContext(ctx, RuntimeRecordingsRoot(), path, limit)
	return records, err
}

func ReadCapturedEventsFileAtRoot(root, path string, limit int) ([]CapturedEventRecord, string, error) {
	return ReadCapturedEventsFileAtRootContext(context.Background(), root, path, limit)
}

func SaveBrowserRecordingExport(path string, payload json.RawMessage) (string, int, error) {
	return saveBrowserRecordingExportAtRoot(RuntimeRecordingsRoot(), path, payload)
}

func saveBrowserRecordingExportAtRoot(root, path string, payload json.RawMessage) (string, int, error) {
	if len(payload) == 0 || string(payload) == "null" {
		return "", 0, errors.New("browser recording export is empty")
	}
	if len(payload) > browserRecordingExportMaxBytes {
		return "", 0, fmt.Errorf("%w: %d bytes (limit %d)", ErrBrowserExportTooLarge, len(payload), browserRecordingExportMaxBytes)
	}
	var normalized any
	if err := json.Unmarshal(payload, &normalized); err != nil {
		return "", 0, err
	}
	pretty, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "", 0, err
	}
	if len(pretty)+1 > browserRecordingOutputMaxBytes {
		return "", 0, fmt.Errorf("%w after formatting: %d bytes (limit %d)", ErrBrowserExportTooLarge, len(pretty)+1, browserRecordingOutputMaxBytes)
	}
	rootFile, absRoot, err := openRecordingsRoot(root)
	if err != nil {
		return "", 0, err
	}
	defer rootFile.Close()
	defaultName := filepath.Base(defaultBrowserRecordingPathAtRoot(absRoot))
	name, absPath, err := resolveRecordingTarget(absRoot, path, defaultName)
	if err != nil {
		return "", 0, err
	}
	tempFile, tempName, err := platform.CreateTempSibling(rootFile, "browser")
	if err != nil {
		return "", 0, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = tempFile.Close()
			_ = unix.Unlinkat(int(rootFile.Fd()), tempName, 0)
		}
	}()
	if _, err := tempFile.Write(append(pretty, '\n')); err != nil {
		return "", 0, err
	}
	if err := tempFile.Sync(); err != nil {
		return "", 0, err
	}
	if err := tempFile.Close(); err != nil {
		return "", 0, err
	}
	if err := platform.ReplaceFileInDir(rootFile, tempName, name); err != nil {
		return "", 0, err
	}
	cleanup = false
	count := 0
	if object, ok := normalized.(map[string]any); ok {
		if snapshots, ok := object["snapshots"].([]any); ok {
			count = len(snapshots)
		}
	}
	return absPath, count, nil
}
