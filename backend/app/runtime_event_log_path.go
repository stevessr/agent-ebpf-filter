package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-ebpf-filter/app/platform"
	"golang.org/x/sys/unix"
)

func resolveRuntimeEventLogPath(raw string) (string, error) {
	return resolveRuntimeEventLogPathWithin(platform.RuntimeSettingsDir(), expandRuntimeEventLogPath(raw))
}

func expandRuntimeEventLogPath(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "~" {
		return platform.GetRealHomeDir()
	}
	if strings.HasPrefix(value, "~/") {
		return filepath.Join(platform.GetRealHomeDir(), strings.TrimPrefix(value, "~/"))
	}
	return value
}

func resolveRuntimeEventLogPathWithin(rootPath, raw string) (string, error) {
	rootPath, err := filepath.Abs(filepath.Clean(strings.TrimSpace(rootPath)))
	if err != nil {
		return "", fmt.Errorf("resolve runtime log root: %w", err)
	}
	path := strings.TrimSpace(raw)
	if path == "" {
		path = filepath.Join(rootPath, "events.jsonl")
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(rootPath, path)
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve runtime log path: %w", err)
	}
	if filepath.Dir(path) != rootPath || filepath.Base(path) == "." {
		return "", fmt.Errorf("runtime log path must be a file directly under %s", rootPath)
	}
	return path, nil
}

func openRuntimeEventLogFile(path string, flags int) (*os.File, string, error) {
	return openRuntimeEventLogFileWithin(platform.RuntimeSettingsDir(), expandRuntimeEventLogPath(path), flags)
}

func openRuntimeEventLogFileWithin(rootPath, path string, flags int) (*os.File, string, error) {
	resolved, err := resolveRuntimeEventLogPathWithin(rootPath, path)
	if err != nil {
		return nil, "", err
	}
	rootPath, err = filepath.Abs(filepath.Clean(strings.TrimSpace(rootPath)))
	if err != nil {
		return nil, "", err
	}
	rootFile, err := secureOpenOrCreateDirectory(rootPath)
	if err != nil {
		return nil, "", fmt.Errorf("open runtime log root: %w", err)
	}
	defer rootFile.Close()
	if err := rootFile.Chmod(0o700); err != nil {
		return nil, "", fmt.Errorf("set runtime log root permissions: %w", err)
	}
	if err := chownArtifactFile(rootFile); err != nil {
		return nil, "", fmt.Errorf("set runtime log root ownership: %w", err)
	}
	rel := filepath.Base(resolved)
	mode := uint64(0)
	if flags&os.O_CREATE != 0 {
		mode = 0o600
	}
	truncateAfterValidation := flags&os.O_TRUNC != 0
	openFlags := flags &^ os.O_TRUNC
	fd, err := unix.Openat2(int(rootFile.Fd()), rel, &unix.OpenHow{
		Flags: uint64(openFlags | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Mode:  mode,
		Resolve: unix.RESOLVE_BENEATH |
			unix.RESOLVE_NO_SYMLINKS |
			unix.RESOLVE_NO_MAGICLINKS,
	})
	if err != nil {
		return nil, "", fmt.Errorf("open runtime log file: %w", err)
	}
	file := os.NewFile(uintptr(fd), resolved)
	if file == nil {
		_ = unix.Close(fd)
		return nil, "", fmt.Errorf("open runtime log file: invalid file descriptor")
	}
	info, err := file.Stat()
	var stat unix.Stat_t
	statErr := unix.Fstat(fd, &stat)
	if err != nil || statErr != nil || !info.Mode().IsRegular() || stat.Nlink != 1 {
		_ = file.Close()
		if err != nil {
			return nil, "", fmt.Errorf("inspect opened runtime log: %w", err)
		}
		if statErr != nil {
			return nil, "", fmt.Errorf("inspect opened runtime log links: %w", statErr)
		}
		return nil, "", fmt.Errorf("opened runtime log must be a single-link regular file")
	}
	if flags&(os.O_WRONLY|os.O_RDWR) != 0 {
		if err := file.Chmod(0o600); err != nil {
			_ = file.Close()
			return nil, "", fmt.Errorf("set runtime log permissions: %w", err)
		}
		if err := chownArtifactFile(file); err != nil {
			_ = file.Close()
			return nil, "", fmt.Errorf("set runtime log ownership: %w", err)
		}
		if truncateAfterValidation {
			if err := file.Truncate(0); err != nil {
				_ = file.Close()
				return nil, "", fmt.Errorf("truncate runtime log: %w", err)
			}
		}
	}
	return file, resolved, nil
}
