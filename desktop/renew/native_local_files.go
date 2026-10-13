package main

// All filesystem operations in this workspace run as the desktop user.
// They do not use the privileged collector, backend API, or a shell.
import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

const (
	localDirectoryLimit = 5000
	localEditorMaxBytes = 8 << 20
)

var errLocalFileChanged = errors.New("文件已被其他程序修改；请重新打开并确认后再保存")

type localFileEntry struct {
	name string
	dir bool
	link bool
}

type localFileDocument struct {
	path string
	state *editor.State
	modTime time.Time
	size int64
	mode os.FileMode
	saving bool
}

type localFileWorkspace struct {
	dir string
	address string
	filter string
	items []localFileEntry
	listLoading bool
	listLoaded bool
	listGeneration uint64
	listTruncated bool
	selected string
	newName string
	renameName string
	pendingDelete string
	errorText string
	notice string
	docs []*localFileDocument
	active int
	closePending string
	quitPending bool
	allowQuit bool
}

func (a *renewApp) hasUnsavedLocalDocuments() bool {
	for _, doc := range a.files.docs {
		if doc.saving || doc.state.Buffer.Modified() {
			return true
		}
	}
	return false
}

// Read only a bounded number of directory entries. Directory listing never
// walks recursively, follows an entry symlink, or involves the collector.
func readLocalDirectory(dir string) ([]localFileEntry, bool, error) {
	handle, err := os.Open(dir)
	if err != nil {
		return nil, false, err
	}
	defer handle.Close()
	stat, err := handle.Stat()
	if err != nil {
		return nil, false, err
	}
	if !stat.IsDir() {
		return nil, false, fmt.Errorf("不是目录：%s", dir)
	}
	entries, err := handle.ReadDir(localDirectoryLimit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(entries) > localDirectoryLimit
	if truncated {
		entries = entries[:localDirectoryLimit]
	}
	rows := make([]localFileEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, localFileEntry{
			name: entry.Name(),
			dir: entry.IsDir(),
			link: entry.Type()&os.ModeSymlink != 0,
		})
	}
	slices.SortFunc(rows, func(a, b localFileEntry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	})
	return rows, truncated, nil
}

func validLocalName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, "/\\:\x00") && filepath.Base(name) == name
}

func localTarget(dir, name string) (string, error) {
	if !validLocalName(name) {
		return "", errors.New("请输入不含路径分隔符的文件名")
	}
	return filepath.Join(dir, name), nil
}

func createLocalItem(dir, name string, folder bool) error {
	path, err := localTarget(dir, name)
	if err != nil {
		return err
	}
	if folder {
		return os.Mkdir(path, 0o700)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

func renameLocalItem(dir, oldName, newName string) error {
	oldPath, err := localTarget(dir, oldName)
	if err != nil {
		return err
	}
	newPath, err := localTarget(dir, newName)
	if err != nil {
		return err
	}
	if oldPath == newPath {
		return nil
	}
	if _, err := os.Lstat(newPath); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func deleteLocalItem(dir, name string) error {
	path, err := localTarget(dir, name)
	if err != nil {
		return err
	}
	// os.Remove never recursively destroys a directory.
	return os.Remove(path)
}

func readLocalDocument(path string) (string, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > localEditorMaxBytes {
		return "", nil, errors.New("只支持不超过 8 MiB 的普通文本文件；符号链接、设备和大文件不会被编辑")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, localEditorMaxBytes + 1))
	if err != nil {
		return "", nil, err
	}
	if len(data) > localEditorMaxBytes || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", nil, errors.New("文件过大、不是 UTF-8 文本或包含二进制内容")
	}
	after, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if !after.Mode().IsRegular() || !after.ModTime().Equal(info.ModTime()) || after.Size() != info.Size() {
		return "", nil, errLocalFileChanged
	}
	return string(data), after, nil
}

// Atomic replacement preserves existing permissions and CRLF via Buffer's
// WriteTo semantics. Version/mtime guards prevent silently clobbering external
// edits; it intentionally refuses symlinks. Only call with a saved snapshot.
func saveLocalDocument(path string, expectedModTime time.Time, expectedSize int64, expectedMode os.FileMode, snap editor.Snapshot) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize || !info.ModTime().Equal(expectedModTime) || info.Mode().Perm() != expectedMode.Perm() {
		return nil, errLocalFileChanged
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".renew-save-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		tmp.Close()
		return nil, err
	}
	if _, err := snap.WriteTo(tmp); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	// Check once more immediately before the final replace. Concurrent
	// writers that replace the file during this narrow interval still require
	// external locking to guarantee conflict freedom.
	now, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !now.Mode().IsRegular() || now.Size() != info.Size() || !now.ModTime().Equal(info.ModTime()) || !os.SameFile(info, now) {
		return nil, errLocalFileChanged
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return nil, err
	}
	return os.Lstat(path)
}
