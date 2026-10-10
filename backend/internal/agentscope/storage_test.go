package agentscope

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreRoundTripAndPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "agent-scopes.json")
	cfg := Default()
	cfg.Capture = List{Mode: ModeWhitelist, Entries: []string{"codex", "gemini"}}
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	stored, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, cfg) {
		t.Fatalf("round trip mismatch: got %#v want %#v", stored, cfg)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("scope file perms: %v err %v", info, err)
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("scope directory perms: %v err %v", info, err)
	}
}

func TestLoadRejectsMalformedOrInvalidPolicies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-scopes.json")
	if _, err := LoadFile(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
	for _, raw := range []string{
		"not-json",
		`{"capture":{"mode":"whitelist","entries":["bad\nname"]},"monitor":{"mode":"blacklist","entries":[]}}`,
		`{"capture":{"mode":"whatever","entries":[]},"monitor":{"mode":"blacklist","entries":[]}}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFile(path); err == nil {
			t.Fatalf("invalid scope was accepted: %s", raw)
		}
	}
}

func TestSaveReplacesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-scopes.json")
	cfg := Default()
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Monitor = List{Mode: ModeWhitelist, Entries: []string{"codex"}}
	if err := SaveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil || !reflect.DeepEqual(got, cfg) {
		t.Fatalf("updated file mismatch: got=%#v err=%v", got, err)
	}
}
