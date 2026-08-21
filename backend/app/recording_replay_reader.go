package app

import (
	"agent-ebpf-filter/app/platform"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/sys/unix"
)

func readCapturedEventsFileAtRootContext(ctx context.Context, root, path string, limit int) ([]CapturedEventRecord, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if limit <= 0 || limit > eventReplayMaxRecords {
		limit = eventReplayMaxRecords
	}
	rootFile, absRoot, err := openRecordingsRoot(root)
	if err != nil {
		return nil, "", err
	}
	defer rootFile.Close()
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	name, absPath, err := resolveRecordingTarget(absRoot, path, "")
	if err != nil {
		return nil, "", err
	}
	file, err := platform.OpenBeneath(rootFile, name, unix.O_RDONLY, 0)
	if err != nil {
		return nil, "", err
	}
	if err := platform.ValidateRegularSingleLink(file); err != nil {
		_ = file.Close()
		return nil, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if info.Size() < 0 || info.Size() > eventReplayMaxFileBytes {
		return nil, "", fmt.Errorf("%w: %d bytes (limit %d)", errRecordingFileTooLarge, info.Size(), eventReplayMaxFileBytes)
	}

	records, err := readCapturedEventTail(ctx, file, info.Size(), limit)
	if err != nil {
		return nil, "", err
	}
	// The reverse reader intentionally operates on the size snapshot above so
	// an active append writer cannot make one request chase a moving EOF. Still
	// reject a file that crossed the public replay cap while it was being read.
	finalInfo, err := file.Stat()
	if err != nil {
		return nil, "", err
	}
	if finalInfo.Size() < 0 || finalInfo.Size() > eventReplayMaxFileBytes {
		return nil, "", fmt.Errorf("%w: file grew to %d bytes during replay (limit %d)", errRecordingFileTooLarge, finalInfo.Size(), eventReplayMaxFileBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return records, absPath, nil
}

func readCapturedEventTail(ctx context.Context, file io.ReaderAt, size int64, limit int) ([]CapturedEventRecord, error) {
	return readCapturedEventTailWithScanLimit(ctx, file, size, limit, eventReplayMaxScannedBytes)
}

func readCapturedEventTailWithScanLimit(ctx context.Context, file io.ReaderAt, size int64, limit int, maxScannedBytes int64) ([]CapturedEventRecord, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if size <= 0 || limit <= 0 {
		return []CapturedEventRecord{}, nil
	}
	records := make([]CapturedEventRecord, 0, min(limit, 1024))
	position := size
	var scannedBytes int64
	var carry []byte
	scannedLines := 0

	processLine := func(line []byte) (bool, error) {
		scannedLines++
		if scannedLines > eventReplayMaxScannedLines {
			return false, fmt.Errorf("%w: limit %d", errRecordingTooManyLines, eventReplayMaxScannedLines)
		}
		if scannedLines == 1 || scannedLines%128 == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		if len(line) > eventReplayMaxLineBytes {
			return false, fmt.Errorf("%w: %d bytes (limit %d)", errRecordingLineTooLarge, len(line), eventReplayMaxLineBytes)
		}
		if len(bytes.TrimSpace(line)) == 0 {
			return false, nil
		}
		var record CapturedEventRecord
		if err := json.Unmarshal(line, &record); err != nil || record.Event == nil {
			return false, nil
		}
		records = append(records, normalizeCapturedEventRecord(record))
		return len(records) >= limit, nil
	}

	for position > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunkBytes := int64(eventReplayReadChunkBytes)
		if position < chunkBytes {
			chunkBytes = position
		}
		if maxScannedBytes > 0 && scannedBytes > maxScannedBytes-chunkBytes {
			return nil, fmt.Errorf("%w: limit %d", errRecordingScanTooLarge, maxScannedBytes)
		}
		position -= chunkBytes
		scannedBytes += chunkBytes
		block := make([]byte, int(chunkBytes)+len(carry))
		read, err := file.ReadAt(block[:int(chunkBytes)], position)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if read != int(chunkBytes) {
			return nil, fmt.Errorf("read recording snapshot: %w", io.ErrUnexpectedEOF)
		}
		copy(block[int(chunkBytes):], carry)

		lineEnd := len(block)
		for index := len(block) - 1; index >= 0; index-- {
			if block[index] != '\n' {
				continue
			}
			done, err := processLine(block[index+1 : lineEnd])
			if err != nil {
				return nil, err
			}
			if done {
				reverseCapturedEventRecords(records)
				return records, nil
			}
			lineEnd = index
		}
		if lineEnd > eventReplayMaxLineBytes {
			return nil, fmt.Errorf("%w: more than %d bytes without a line boundary", errRecordingLineTooLarge, eventReplayMaxLineBytes)
		}
		carry = append(carry[:0], block[:lineEnd]...)
	}

	if len(carry) > 0 {
		done, err := processLine(carry)
		if err != nil {
			return nil, err
		}
		_ = done
	}
	reverseCapturedEventRecords(records)
	return records, nil
}

func reverseCapturedEventRecords(records []CapturedEventRecord) {
	for left, right := 0, len(records)-1; left < right; left, right = left+1, right-1 {
		records[left], records[right] = records[right], records[left]
	}
}
