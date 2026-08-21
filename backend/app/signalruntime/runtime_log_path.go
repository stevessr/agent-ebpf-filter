package signalruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/app/platform"

	"golang.org/x/sys/unix"
)

const (
	signalProgramLogMaxBytes                int64 = 128 * 1024 * 1024
	signalProgramLogMaxCompressedFrameBytes       = 8 * 1024 * 1024
	signalProgramLogMaxPayloadBytes               = 4 * 1024 * 1024
	signalProgramLogMaxFrames                     = 100000
)

var signalProgramLogsRootPath = func() string {
	return filepath.Join(platform.RuntimeSettingsDir(), "signals", "program-logs")
}

func resolveSignalProgramLogPath(selected SelectedProgramSignalLog) (string, error) {
	return resolveSignalProgramLogPathWithin(signalProgramLogsRootPath(), selected)
}

func resolveSignalProgramLogPathWithin(rootPath string, selected SelectedProgramSignalLog) (string, error) {
	rootPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rootPath)))
	if err != nil {
		return "", fmt.Errorf("resolve signal program log root: %w", err)
	}
	if rootPath == string(os.PathSeparator) {
		return "", errors.New("signal program log root must not be the filesystem root")
	}
	path := expandSignalPath(selected.Path)
	if strings.TrimSpace(path) == "" {
		path = sanitizeSignalFilename(selected.Program) + ".pb.gzlog"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, path)
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve signal program log path: %w", err)
	}
	name := filepath.Base(path)
	if filepath.Dir(path) != rootPath || name == "." || name == ".." || strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("signal program log path must be a file directly under %s", rootPath)
	}
	if len(name) > 240 {
		return "", errors.New("signal program log filename is too long")
	}
	return path, nil
}

func openSignalProgramLog(selected SelectedProgramSignalLog, flags int) (*os.File, string, error) {
	return openSignalProgramLogWithin(signalProgramLogsRootPath(), selected, flags)
}

func openSignalProgramLogWithin(rootPath string, selected SelectedProgramSignalLog, flags int) (*os.File, string, error) {
	resolved, err := resolveSignalProgramLogPathWithin(rootPath, selected)
	if err != nil {
		return nil, "", err
	}
	rootPath, err = filepath.Abs(filepath.Clean(strings.TrimSpace(rootPath)))
	if err != nil {
		return nil, "", err
	}
	root, err := platform.SecureOpenOrCreateDir(rootPath)
	if err != nil {
		return nil, "", fmt.Errorf("open signal program log root: %w", err)
	}
	defer root.Close()
	if err := root.Chmod(0o700); err != nil {
		return nil, "", fmt.Errorf("set signal program log root permissions: %w", err)
	}
	if err := platform.ChownArtifactFile(root); err != nil {
		return nil, "", fmt.Errorf("set signal program log root ownership: %w", err)
	}
	mode := uint64(0)
	if flags&os.O_CREATE != 0 {
		mode = 0o600
	}
	fd, err := unix.Openat2(int(root.Fd()), filepath.Base(resolved), &unix.OpenHow{
		Flags: uint64(flags | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Mode:  mode,
		Resolve: unix.RESOLVE_BENEATH |
			unix.RESOLVE_NO_SYMLINKS |
			unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", fmt.Errorf("open signal program log: %w", err)
	}
	file := os.NewFile(uintptr(fd), resolved)
	if file == nil {
		_ = unix.Close(fd)
		return nil, "", errors.New("open signal program log: invalid file descriptor")
	}
	if err := platform.ValidateRegularSingleLink(file); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("validate signal program log: %w", err)
	}
	if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		if err := platform.PreparePrivateFile(file); err != nil {
			_ = file.Close()
			return nil, "", fmt.Errorf("prepare signal program log: %w", err)
		}
	}
	return file, resolved, nil
}
