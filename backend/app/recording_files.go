package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-ebpf-filter/app/platform"

	"golang.org/x/sys/unix"
)

const (
	eventReplayMaxFileBytes         int64 = 128 * 1024 * 1024
	eventReplayReadChunkBytes             = 256 * 1024
	eventReplayMaxLineBytes               = 4 * 1024 * 1024
	eventReplayMaxScannedLines            = 250000
	eventReplayMaxScannedBytes            = eventReplayMaxFileBytes
	eventReplayMaxRecords                 = 10000
	eventReplayProcessingTimeout          = 15 * time.Second
	browserRecordingExportMaxBytes        = 16 * 1024 * 1024
	browserRecordingOutputMaxBytes        = 32 * 1024 * 1024
	browserRecordingRequestMaxBytes int64 = 20 * 1024 * 1024
	recordingControlRequestMaxBytes int64 = 64 * 1024
)

var (
	errRecordingPathOutsideRoot = errors.New("recording path must be a file directly under the runtime recordings directory")
	errRecordingFileTooLarge    = errors.New("recording file exceeds the replay size limit")
	errRecordingLineTooLarge    = errors.New("recording line exceeds the replay line size limit")
	errRecordingTooManyLines    = errors.New("recording replay scanned too many lines")
	errRecordingScanTooLarge    = errors.New("recording replay scanned too many bytes")
	errBrowserRecordingTooLarge = errors.New("browser recording export exceeds the size limit")
)

func runtimeRecordingsRoot() string {
	return filepath.Join(platform.RuntimeSettingsDir(), "recordings")
}

func openRecordingsRoot(root string) (*os.File, string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, "", errors.New("recordings root is empty")
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, "", fmt.Errorf("resolve recordings root: %w", err)
	}
	if absRoot == string(os.PathSeparator) {
		return nil, "", errors.New("recordings root must not be the filesystem root")
	}
	file, err := platform.SecureOpenOrCreateDir(absRoot)
	if err != nil {
		return nil, "", fmt.Errorf("open recordings root without symlinks: %w", err)
	}
	if err := file.Chmod(0o700); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("set recordings root permissions: %w", err)
	}
	if err := platform.ChownArtifactFile(file); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("set recordings root ownership: %w", err)
	}
	return file, absRoot, nil
}

func resolveRecordingTarget(root, raw, defaultName string) (name, absPath string, err error) {
	absRoot, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return "", "", err
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		value = defaultName
	}
	if value == "" {
		return "", "", errors.New("recording path is required")
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		value = filepath.Join(platform.GetRealHomeDir(), strings.TrimPrefix(value, "~/"))
	}
	if filepath.IsAbs(value) {
		clean := filepath.Clean(value)
		if filepath.Dir(clean) != absRoot {
			return "", "", errRecordingPathOutsideRoot
		}
		value = filepath.Base(clean)
	}
	if value != filepath.Base(value) || value == "." || value == ".." || strings.ContainsRune(value, 0) {
		return "", "", errRecordingPathOutsideRoot
	}
	if len(value) > 240 {
		return "", "", errors.New("recording filename is too long")
	}
	return value, filepath.Join(absRoot, value), nil
}

func openRecordingForAppend(root *os.File, name string) (*os.File, error) {
	file, err := platform.OpenBeneath(root, name, unix.O_WRONLY|unix.O_APPEND, 0)
	if errors.Is(err, unix.ENOENT) {
		file, err = platform.OpenBeneath(root, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL, 0o600)
	}
	if err != nil {
		return nil, err
	}
	if err := platform.ValidateRegularSingleLink(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	if err := platform.PreparePrivateFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func createTruncatedRecording(root *os.File, name string) (*os.File, error) {
	file, tempName, err := platform.CreateTempSibling(root, "events")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = file.Close()
			_ = platform.UnlinkAt(root, tempName)
		}
	}()
	if err := platform.ReplaceFileInDir(root, tempName, name); err != nil {
		return nil, err
	}
	cleanup = false
	return file, nil
}
