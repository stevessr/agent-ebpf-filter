package app

import (
	"crypto/rand"
	"encoding/hex"
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
	file, err := secureOpenOrCreateDirectory(absRoot)
	if err != nil {
		return nil, "", fmt.Errorf("open recordings root without symlinks: %w", err)
	}
	if err := file.Chmod(0o700); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("set recordings root permissions: %w", err)
	}
	if err := chownArtifactFile(file); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("set recordings root ownership: %w", err)
	}
	return file, absRoot, nil
}

func secureOpenOrCreateDirectory(absPath string) (*os.File, error) {
	currentFD, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	current := os.NewFile(uintptr(currentFD), "/")
	if current == nil {
		_ = unix.Close(currentFD)
		return nil, errors.New("open filesystem root: invalid file descriptor")
	}
	for _, component := range strings.Split(filepath.Clean(absPath), string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		created := false
		if err := unix.Mkdirat(int(current.Fd()), component, 0o700); err == nil {
			created = true
		} else if !errors.Is(err, unix.EEXIST) {
			_ = current.Close()
			return nil, err
		}
		nextFD, err := unix.Openat2(int(current.Fd()), component, &unix.OpenHow{
			Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW,
			Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
		})
		if err != nil {
			_ = current.Close()
			return nil, err
		}
		next := os.NewFile(uintptr(nextFD), component)
		if next == nil {
			_ = unix.Close(nextFD)
			_ = current.Close()
			return nil, errors.New("open directory component: invalid file descriptor")
		}
		if created {
			if err := next.Chmod(0o700); err != nil {
				_ = next.Close()
				_ = current.Close()
				return nil, err
			}
			if err := chownArtifactFile(next); err != nil {
				_ = next.Close()
				_ = current.Close()
				return nil, err
			}
		}
		_ = current.Close()
		current = next
	}
	return current, nil
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

func openRecordingChild(root *os.File, name string, flags int, mode uint32) (*os.File, error) {
	fd, err := unix.Openat2(int(root.Fd()), name, &unix.OpenHow{
		Flags: uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Mode:  uint64(mode),
		Resolve: unix.RESOLVE_BENEATH |
			unix.RESOLVE_NO_SYMLINKS |
			unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("invalid recording file descriptor")
	}
	if err := validateRecordingRegularFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validateRecordingRegularFile(file *os.File) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("recording target must be a regular file")
	}
	if stat.Nlink != 1 {
		return errors.New("recording target must not have multiple hard links")
	}
	return nil
}

func prepareRecordingFile(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	return chownArtifactFile(file)
}

func openRecordingForAppend(root *os.File, name string) (*os.File, error) {
	file, err := openRecordingChild(root, name, unix.O_WRONLY|unix.O_APPEND, 0)
	if errors.Is(err, unix.ENOENT) {
		file, err = openRecordingChild(root, name, unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT|unix.O_EXCL, 0o600)
	}
	if err != nil {
		return nil, err
	}
	if err := prepareRecordingFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func createRecordingTemp(root *os.File, prefix string) (*os.File, string, error) {
	for attempt := 0; attempt < 32; attempt++ {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := "." + prefix + "-" + hex.EncodeToString(random[:]) + ".tmp"
		file, err := openRecordingChild(root, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0o600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		if err := prepareRecordingFile(file); err != nil {
			_ = file.Close()
			_ = unix.Unlinkat(int(root.Fd()), name, 0)
			return nil, "", err
		}
		return file, name, nil
	}
	return nil, "", errors.New("could not allocate a unique recording file")
}

func rejectUnsafeRecordingDestination(root *os.File, name string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(int(root.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("recording destination must be a single-link regular file")
	}
	return nil
}

func replaceRecordingDestination(root *os.File, tempName, targetName string) error {
	if err := rejectUnsafeRecordingDestination(root, targetName); err != nil {
		return err
	}
	if err := unix.Renameat(int(root.Fd()), tempName, int(root.Fd()), targetName); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
}

func createTruncatedRecording(root *os.File, name string) (*os.File, error) {
	file, tempName, err := createRecordingTemp(root, "events")
	if err != nil {
		return nil, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = file.Close()
			_ = unix.Unlinkat(int(root.Fd()), tempName, 0)
		}
	}()
	if err := replaceRecordingDestination(root, tempName, name); err != nil {
		return nil, err
	}
	cleanup = false
	return file, nil
}
