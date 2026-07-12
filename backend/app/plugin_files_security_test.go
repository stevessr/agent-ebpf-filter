package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func withPluginRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	old := pluginsRootPath
	pluginsRootPath = func() string { return root }
	t.Cleanup(func() { pluginsRootPath = old })
	return root
}

func TestPluginFilesRejectSymlinkHardlinkAndFIFO(t *testing.T) {
	root := withPluginRoot(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "safe-plugin")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, setup := range map[string]func(string) error{
		"manifest.json": func(path string) error { return os.Symlink(outside, path) },
		"source.c":      func(path string) error { return os.Link(outside, path) },
		"program.o":     func(path string) error { return syscall.Mkfifo(path, 0o600) },
	} {
		if err := setup(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		if err := writePluginFile("safe-plugin", name, []byte("changed"), pluginObjectMaxBytes); err == nil {
			t.Fatalf("unsafe %s accepted", name)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "unchanged" {
		t.Fatalf("outside changed: %q %v", got, err)
	}
}

func TestPluginStoreRequiresManifestIDMatch(t *testing.T) {
	root := withPluginRoot(t)
	dir := filepath.Join(root, "safe-plugin")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(PluginManifest{ID: "other-plugin"})
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	store := &pluginStore{entries: make(map[string]*PluginManifest)}
	if err := store.ensureLoaded(); err != nil {
		t.Fatal(err)
	}
	if len(store.entries) != 0 {
		t.Fatalf("mismatched manifest loaded: %#v", store.entries)
	}
}

func TestPluginDeleteFailsClosedOnUnexpectedEntry(t *testing.T) {
	root := withPluginRoot(t)
	store := &pluginStore{entries: map[string]*PluginManifest{"safe-plugin": {ID: "safe-plugin"}}, loaded: true}
	dir := filepath.Join(root, "safe-plugin")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "unexpected"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("safe-plugin"); err == nil {
		t.Fatal("unsafe plugin directory deleted")
	}
	if _, ok := store.entries["safe-plugin"]; !ok {
		t.Fatal("registry entry removed after failed delete")
	}
}

func TestPluginStorePreservesAndInvalidatesArtifactDigests(t *testing.T) {
	withPluginRoot(t)
	store := &pluginStore{entries: make(map[string]*PluginManifest)}
	manifest := testTracepointManifest()
	if err := store.UpsertWithSource(manifest, validTestBPFSource); err != nil {
		t.Fatalf("UpsertWithSource() error = %v", err)
	}
	object := []byte("compiled-object")
	if err := writePluginFile(manifest.ID, "program.o", object, maxUserBPFObjectBytes); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCompile(manifest.ID, sha256Hex([]byte(validTestBPFSource)), sha256Hex(object)); err != nil {
		t.Fatalf("RecordCompile() error = %v", err)
	}

	update := *manifest
	update.Name = "Updated plugin"
	if err := store.Upsert(&update); err != nil {
		t.Fatalf("metadata Upsert() error = %v", err)
	}
	stored, ok := store.Get(manifest.ID)
	if !ok || stored.SourceSHA256 == "" || stored.ObjectSHA256 != sha256Hex(object) {
		t.Fatalf("artifact digests were not preserved: %+v", stored)
	}

	changedSource := validTestBPFSource + "\n// changed\n"
	if err := store.UpsertWithSource(&update, changedSource); err != nil {
		t.Fatalf("changed source Upsert() error = %v", err)
	}
	stored, _ = store.Get(manifest.ID)
	if stored.SourceSHA256 != sha256Hex([]byte(changedSource)) || stored.ObjectSHA256 != "" {
		t.Fatalf("stale object digest survived source change: %+v", stored)
	}
	if _, err := os.Stat(filepath.Join(pluginsRootPath(), manifest.ID, "program.o")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale object still exists: %v", err)
	}
}

func TestPluginStoreClonesCallerOwnedSlices(t *testing.T) {
	withPluginRoot(t)
	store := &pluginStore{entries: make(map[string]*PluginManifest)}
	manifest := &PluginManifest{
		ID:            "safe-plugin",
		Name:          "Safe plugin",
		Kind:          PluginKindCommand,
		CommandArgs:   []string{"first"},
		WebhookEvents: []string{"event"},
	}
	if err := store.Upsert(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.CommandArgs[0] = "mutated"
	manifest.WebhookEvents[0] = "mutated"
	stored, _ := store.Get(manifest.ID)
	if stored.CommandArgs[0] != "first" || stored.WebhookEvents[0] != "event" {
		t.Fatalf("stored slices alias caller memory: %+v", stored)
	}
	stored.CommandArgs[0] = "mutated-again"
	again, _ := store.Get(manifest.ID)
	if again.CommandArgs[0] != "first" {
		t.Fatalf("Get returned registry-owned slice: %+v", again)
	}
}

func TestPluginStoreRejectsUnknownKindBeforeCreatingPluginDirectory(t *testing.T) {
	root := withPluginRoot(t)
	store := &pluginStore{entries: make(map[string]*PluginManifest)}
	err := store.Upsert(&PluginManifest{ID: "safe-plugin", Name: "Unsafe", Kind: PluginKind("unknown")})
	if err == nil {
		t.Fatal("unknown plugin kind accepted")
	}
	if _, statErr := os.Stat(filepath.Join(root, "safe-plugin")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid plugin created directory: %v", statErr)
	}
}

func TestPluginStoreBoundsRegisteredEntries(t *testing.T) {
	root := withPluginRoot(t)
	entries := make(map[string]*PluginManifest, maxRegisteredPlugins)
	for i := 0; i < maxRegisteredPlugins; i++ {
		id := fmt.Sprintf("plugin-%03d", i)
		entries[id] = &PluginManifest{ID: id, Name: id, Kind: PluginKindCommand}
	}
	store := &pluginStore{entries: entries, loaded: true}
	err := store.Upsert(&PluginManifest{ID: "overflow-plugin", Name: "Overflow", Kind: PluginKindCommand})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("plugin registry limit error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "overflow-plugin")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("overflow plugin created directory: %v", statErr)
	}
}
