package app

import (
	"agent-ebpf-filter/app/research"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"agent-ebpf-filter/app/platform"

	"golang.org/x/sys/unix"
)

const (
	pluginManifestMaxBytes int64 = 1 << 20
	pluginSourceMaxBytes   int64 = 256 << 10
	pluginObjectMaxBytes   int64 = maxUserBPFObjectBytes
)

var pluginsRootPath = platform.PluginsRootDir

const pluginArtifactLockStripes = 64

type pluginArtifactLock struct {
	once sync.Once
	slot chan struct{}
}

var pluginArtifactLocks [pluginArtifactLockStripes]pluginArtifactLock

func acquirePluginArtifactLock(ctx context.Context, id string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var hash uint32 = 2166136261
	for i := 0; i < len(id); i++ {
		hash ^= uint32(id[i])
		hash *= 16777619
	}
	lock := &pluginArtifactLocks[hash%pluginArtifactLockStripes]
	lock.once.Do(func() { lock.slot = make(chan struct{}, 1) })
	select {
	case lock.slot <- struct{}{}:
		return func() { <-lock.slot }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func openPluginsRoot() (*os.File, error) {
	root, err := platform.SecureOpenOrCreateDir(pluginsRootPath())
	if err != nil {
		return nil, fmt.Errorf("open plugins root: %w", err)
	}
	if err = root.Chmod(0o700); err != nil {
		root.Close()
		return nil, err
	}
	if err = platform.ChownArtifactFile(root); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

func openPluginDir(root *os.File, id string, create bool) (*os.File, error) {
	if err := validatePluginID(id); err != nil {
		return nil, err
	}
	if create {
		if err := unix.Mkdirat(int(root.Fd()), id, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
	}
	fd, err := unix.Openat2(int(root.Fd()), id, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, err
	}
	dir := os.NewFile(uintptr(fd), id)
	if dir == nil {
		unix.Close(fd)
		return nil, errors.New("invalid plugin directory fd")
	}
	if err = dir.Chmod(0o700); err != nil {
		dir.Close()
		return nil, err
	}
	if err = platform.ChownArtifactFile(dir); err != nil {
		dir.Close()
		return nil, err
	}
	return dir, nil
}

func readPluginFile(id, name string, max int64) ([]byte, error) {
	root, err := openPluginsRoot()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := openPluginDir(root, id, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return research.ReadFile(dir, name, max)
}

func writePluginFile(id, name string, data []byte, max int64) error {
	root, err := openPluginsRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := openPluginDir(root, id, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	return research.AtomicWriteFile(dir, name, data, max)
}

func writePluginSource(id, source string) error {
	return writePluginFile(id, "source.c", []byte(source), pluginSourceMaxBytes)
}
func pluginDisplayPath(id, name string) string { return filepath.Join(pluginsRootPath(), id, name) }

func removePluginFileIfExists(id, name string) error {
	root, err := openPluginsRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := openPluginDir(root, id, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := research.RemoveFile(dir, name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return unix.Fsync(int(dir.Fd()))
}

func deletePluginFiles(id string) error {
	root, err := openPluginsRoot()
	if err != nil {
		return err
	}
	defer root.Close()
	dir, err := openPluginDir(root, id, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	names, err := research.DirectoryNames(dir, 4)
	if err != nil {
		return err
	}
	for _, name := range names {
		switch name {
		case "manifest.json", "source.c", "program.o":
		default:
			return fmt.Errorf("unexpected plugin entry %q", name)
		}
		if err := research.RemoveFile(dir, name); err != nil {
			return err
		}
	}
	if err = dir.Close(); err != nil {
		return err
	}
	if err = research.RemoveDirectory(root, id); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
}
