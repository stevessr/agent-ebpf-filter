// Package platform — secure filesystem primitives.
//
// Generic, directory-scoped file operations shared by event recording,
// artifact export, plugins, research sessions and the ML training store.
// All opens use RESOLVE_BENEATH / NO_SYMLINK / NO_MAGICLINKS so callers can
// never escape the anchor directory.
package platform

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ChownArtifactFile restores the original invoker's ownership on an artifact
// created while running elevated. No-op when not root or when the original
// invoker is unknown.
func ChownArtifactFile(file *os.File) error {
	if file == nil || os.Getuid() != 0 {
		return nil
	}
	uid, gid, ok := OriginalInvokerIDs()
	if !ok {
		return nil
	}
	return file.Chown(int(uid), int(gid))
}

// SecureOpenOrCreateDir walks absPath component-by-component beneath "/",
// creating missing directories with 0700 and refusing symlinks. Returns a
// descriptor anchored at the deepest component.
func SecureOpenOrCreateDir(absPath string) (*os.File, error) {
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
			if err := ChownArtifactFile(next); err != nil {
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

// OpenBeneath opens name relative to the anchor directory with symlink and
// magic-link resolution disabled.
func OpenBeneath(root *os.File, name string, flags int, mode uint32) (*os.File, error) {
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
		return nil, errors.New("invalid anchored file descriptor")
	}
	return file, nil
}

// ValidateRegularSingleLink rejects anything that is not a plain regular
// file with exactly one hard link (no /proc oddities, no hardlink attacks).
func ValidateRegularSingleLink(file *os.File) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("target must be a regular file")
	}
	if stat.Nlink != 1 {
		return errors.New("target must not have multiple hard links")
	}
	return nil
}

// PreparePrivateFile restricts an artifact to owner-only before ownership
// is handed back to the original invoker.
func PreparePrivateFile(file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	return ChownArtifactFile(file)
}

// RejectNonRegularOrMultiLink verifies the named entry is either absent or a
// single-link regular file before it is used as a replacement destination.
func RejectNonRegularOrMultiLink(root *os.File, name string) error {
	var stat unix.Stat_t
	err := unix.Fstatat(int(root.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return errors.New("destination must be a single-link regular file")
	}
	return nil
}

// ReplaceFileInDir atomically swaps tempName into targetName within the
// anchor directory and flushes the directory entry.
func ReplaceFileInDir(root *os.File, tempName, targetName string) error {
	if err := RejectNonRegularOrMultiLink(root, targetName); err != nil {
		return err
	}
	if err := unix.Renameat(int(root.Fd()), tempName, int(root.Fd()), targetName); err != nil {
		return err
	}
	return unix.Fsync(int(root.Fd()))
}

// CreateTempSibling allocates a unique 0600 temporary file inside the anchor
// directory. Caller owns closing and unlinking on failure.
func CreateTempSibling(root *os.File, prefix string) (*os.File, string, error) {
	for range 32 {
		var random [12]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := "." + prefix + "-" + hex.EncodeToString(random[:]) + ".tmp"
		file, err := OpenBeneath(root, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL, 0o600)
		if err != nil {
			return nil, "", err
		}
		if err := PreparePrivateFile(file); err != nil {
			_ = file.Close()
			_ = unix.Unlinkat(int(root.Fd()), name, 0)
			return nil, "", err
		}
		return file, name, nil
	}
	return nil, "", errors.New("could not allocate a unique temporary file")
}

// UnlinkAt removes name relative to the anchor directory.
func UnlinkAt(root *os.File, name string) error {
	return unix.Unlinkat(int(root.Fd()), name, 0)
}
