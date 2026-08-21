package app

import (
	"agent-ebpf-filter/app/platform"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	researchSessionFileMaxBytes  int64 = 1 << 20
	researchResultsFileMaxBytes  int64 = 32 << 20
	researchEventsFileMaxBytes   int64 = 256 << 20
	researchArtifactMaxBytes     int64 = 256 << 20
	researchMaxPersistedSessions       = 1024
	researchMaxRootScanEntries         = 8192
)

func validateResearchFileComponent(value, label string, allowDot bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 240 || filepath.Base(value) != value || value == "." || value == ".." || strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("invalid research %s", label)
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || allowDot && r == '.' {
			continue
		}
		return "", fmt.Errorf("invalid research %s", label)
	}
	return value, nil
}

func openResearchRoot(base string) (*os.File, error) {
	abs, err := filepath.Abs(filepath.Clean(strings.TrimSpace(base)))
	if err != nil {
		return nil, err
	}
	if abs == string(os.PathSeparator) {
		return nil, errors.New("research root must not be filesystem root")
	}
	f, err := platform.SecureOpenOrCreateDir(abs)
	if err != nil {
		return nil, fmt.Errorf("open research root: %w", err)
	}
	if err = f.Chmod(0700); err != nil {
		f.Close()
		return nil, err
	}
	if err = platform.ChownArtifactFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func openResearchDirChild(parent *os.File, name string, create bool) (*os.File, error) {
	if _, err := validateResearchFileComponent(name, "directory", false); err != nil {
		return nil, err
	}
	var err error
	if create {
		err = unix.Mkdirat(int(parent.Fd()), name, 0700)
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
	}
	fd, err := unix.Openat2(int(parent.Fd()), name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS})
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if f == nil {
		unix.Close(fd)
		return nil, errors.New("invalid research directory fd")
	}
	if err := f.Chmod(0o700); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := platform.ChownArtifactFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
func openResearchSession(base, id string, create bool) (root, session *os.File, err error) {
	id, err = validateResearchFileComponent(id, "session id", false)
	if err != nil {
		return nil, nil, err
	}
	root, err = openResearchRoot(base)
	if err != nil {
		return nil, nil, err
	}
	session, err = openResearchDirChild(root, id, create)
	if err != nil {
		root.Close()
		return nil, nil, err
	}
	return root, session, nil
}
func openResearchArtifacts(session *os.File, create bool) (*os.File, error) {
	return openResearchDirChild(session, "artifacts", create)
}

func atomicWriteResearchFile(dir *os.File, name string, payload []byte, maxBytes int64) error {
	if _, err := validateResearchFileComponent(name, "filename", true); err != nil {
		return err
	}
	if maxBytes <= 0 || int64(len(payload)) > maxBytes {
		return errors.New("research file exceeds size limit")
	}
	f, tmp, err := platform.CreateTempSibling(dir, "research")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		f.Close()
		if cleanup {
			unix.Unlinkat(int(dir.Fd()), tmp, 0)
		}
	}()
	if _, err = f.Write(payload); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = platform.ReplaceFileInDir(dir, tmp, name); err != nil {
		return err
	}
	cleanup = false
	return nil
}
func readResearchFile(dir *os.File, name string, maxBytes int64) ([]byte, error) {
	if _, err := validateResearchFileComponent(name, "filename", true); err != nil {
		return nil, err
	}
	f, err := platform.OpenBeneath(dir, name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	if err := platform.ValidateRegularSingleLink(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if maxBytes <= 0 || info.Size() > maxBytes {
		return nil, errors.New("research file exceeds size limit")
	}
	payload, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) > maxBytes {
		return nil, errors.New("research file exceeds size limit")
	}
	return payload, nil
}
func removeResearchFile(dir *os.File, name string) error {
	if _, err := validateResearchFileComponent(name, "filename", true); err != nil {
		return err
	}
	if err := validateResearchRegularEntry(dir, name); errors.Is(err, unix.ENOENT) {
		return nil
	} else if err != nil {
		return err
	}
	return unix.Unlinkat(int(dir.Fd()), name, 0)
}

func validateResearchRegularEntry(dir *os.File, name string) error {
	if _, err := validateResearchFileComponent(name, "filename", true); err != nil {
		return err
	}
	var st unix.Stat_t
	err := unix.Fstatat(int(dir.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 {
		return errors.New("research target must be a single-link regular file")
	}
	return nil
}

func researchFileInfo(dir *os.File, name string) (os.FileInfo, error) {
	f, err := platform.OpenBeneath(dir, name, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	if err := platform.ValidateRegularSingleLink(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func researchDirectoryNames(dir *os.File, max int) ([]string, error) {
	if _, err := dir.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if max <= 0 {
		return nil, errors.New("research directory entry limit must be positive")
	}
	names, err := dir.Readdirnames(max + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > max {
		return nil, fmt.Errorf("research directory exceeds %d entries", max)
	}
	return names, nil
}

func removeResearchDirectory(parent *os.File, name string) error {
	if _, err := validateResearchFileComponent(name, "directory", false); err != nil {
		return err
	}
	return unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR)
}

func researchArtifactFilename(format string) (string, bool) {
	switch normalizeResearchFormat(format) {
	case "jsonl":
		return "events.jsonl", true
	case "csv":
		return "events.csv", true
	case "json":
		return "research.json", true
	case "bundle":
		return "research-bundle.zip", true
	case "security_json":
		return "security-evaluation.json", true
	case "security_jsonl":
		return "security-evaluation.jsonl", true
	case "security_csv":
		return "security-evaluation.csv", true
	}
	if strings.EqualFold(strings.TrimSpace(format), "manifest") {
		return "manifest.json", true
	}
	return "", false
}

func researchArtifactContentType(format string) string {
	switch normalizeResearchFormat(format) {
	case "jsonl", "security_jsonl":
		return "application/x-ndjson; charset=utf-8"
	case "csv", "security_csv":
		return "text/csv; charset=utf-8"
	case "bundle":
		return "application/zip"
	default:
		return "application/json; charset=utf-8"
	}
}

func researchDisplayArtifactPath(base, id, name string) string {
	return filepath.Join(base, id, "artifacts", name)
}
