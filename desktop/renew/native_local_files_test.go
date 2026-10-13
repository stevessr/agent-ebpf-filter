package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
)

func TestLocalFileNameAndCreationCannotEscapeDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"", ".", "..", "../escape", "a/b", "a\\b", "C:escape"} {
		if validLocalName(name) || createLocalItem(dir, name, false) == nil {
			t.Errorf("unsafe local name accepted: %q", name)
		}
	}
	if err := createLocalItem(dir, "rules.json", false); err != nil {
		t.Fatal(err)
	}
	if err := createLocalItem(dir, "rules.json", false); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing file must not be overwritten, got %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "rules.json"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("new file not regular: %v", err)
	}
	if err := createLocalItem(dir, "configs", true); err != nil {
		t.Fatal(err)
	}
	if err := renameLocalItem(dir, "rules.json", "settings.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if err := renameLocalItem(dir, "settings.json", "configs"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("rename must not replace existing path: %v", err)
	}
}

func TestLocalFilesAreBoundedAndSorted(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.txt", "A.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "Folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	items, truncated, err := readLocalDirectory(dir)
	if err != nil || truncated || len(items) != 3 {
		t.Fatalf("bad directory listing: %v %v %+v", err, truncated, items)
	}
	if !items[0].dir || items[0].name != "Folder" || items[1].name != "A.txt" || items[2].name != "z.txt" {
		t.Fatalf("directory is not sorted directories-first: %+v", items)
	}
	if err := deleteLocalItem(dir, "Folder"); err != nil {
		t.Fatal(err)
	}
	if err := deleteLocalItem(dir, "z.txt"); err != nil {
		t.Fatal(err)
	}
	if err := deleteLocalItem(dir, "A.txt"); err != nil {
		t.Fatal(err)
	}
	if err := deleteLocalItem(dir, ".."); err == nil {
		t.Fatal("must not delete parent")
	}
}

func TestLocalEditorSavePreservesPermissionAndDetectsExternalChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	if err := os.WriteFile(path, []byte("package main\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, info, err := readLocalDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	buf := editor.NewBuffer(text)
	buf.Set("package main\n// updated\n")
	snap := buf.Snapshot()
	saved, err := saveLocalDocument(path, info.ModTime(), info.Size(), info.Mode(), snap)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "updated") || saved.Mode().Perm() != 0o600 {
		t.Fatalf("lost edit or permissions: %q %v", data, saved.Mode())
	}
	if err := os.WriteFile(path, []byte("external edit changes size"), 0o600); err != nil {
		t.Fatal(err)
	}
	buf.Set("overwrite")
	_, err = saveLocalDocument(path, saved.ModTime(), saved.Size(), saved.Mode(), buf.Snapshot())
	if !errors.Is(err, errLocalFileChanged) {
		t.Fatalf("must refuse external modifications: %v", err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "external edit changes size" {
		t.Fatalf("unexpected overwrite: %q", data)
	}
}

func TestLocalEditorRejectsBinaryAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "binary")
	if err := os.WriteFile(binary, []byte{0x00, 0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readLocalDocument(binary); err == nil {
		t.Fatal("binary content must not be opened in text editor")
	}
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err == nil {
		if _, _, err := readLocalDocument(link); err == nil {
			t.Fatal("symlink must not be opened for editing")
		}
	}
}

func TestLocalEditorNavigationMetadata(t *testing.T) {
	for _, page := range []string{"文件", "编辑器"} {
		if pageSubtitle(page) == "" || workspaceNavigationIcon(page) == nil {
			t.Fatalf("missing native workspace page metadata: %s", page)
		}
		if workspaceNavigationLabel(page, false) == page {
			t.Fatalf("missing sidebar label for %s", page)
		}
	}
}
