package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryFiltersTagAndStatus(t *testing.T) {
	rows := []trackedComm{
		{Comm: "codex", Tag: "AI", Disabled: false},
		{Comm: "claude", Tag: "AI", Disabled: true},
		{Comm: "bash", Tag: "Shell", Disabled: false},
	}
	if got := filterTrackedComms(rows, "AI", "跟踪中"); len(got) != 1 || got[0].Comm != "codex" {
		t.Fatalf("active AI rows = %#v", got)
	}
	if got := filterTrackedComms(rows, "全部标签", "已禁用"); len(got) != 1 || got[0].Comm != "claude" {
		t.Fatalf("disabled rows = %#v", got)
	}
	files := []trackedPath{{Path: "/tmp/a", Tag: "AI"}, {Path: "/tmp/b", Tag: "Shell"}}
	if got := filterTrackedPaths(files, "AI", "全部状态"); len(got) != 1 || got[0].Tag != "AI" {
		t.Fatalf("tagged files = %#v", got)
	}
	if got := filterTrackedPaths(files, "全部标签", "已禁用"); len(got) != 0 {
		t.Fatalf("path rules cannot be disabled, got %#v", got)
	}
}

func TestImmediateFolderModeDoesNotRecurse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.txt"), []byte("one"), 0600); err != nil { t.Fatal(err) }
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(dir, "nested", "two.txt"), []byte("two"), 0600); err != nil { t.Fatal(err) }
	paths, err := listDirectFiles(dir)
	if err != nil { t.Fatal(err) }
	if len(paths) != 1 || paths[0] != filepath.Join(dir, "one.txt") {
		t.Fatalf("expected only immediate regular files, got %v", paths)
	}
	if _, err := listDirectFiles("relative"); err == nil { t.Fatal("relative directory accepted") }
}
