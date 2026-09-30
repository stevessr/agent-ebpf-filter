package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDshProfileHasPluginReadsProfileDependency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	profile := filepath.Join(home, "profiles", "web")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"dependencies":{"@agent-ebpf/dsh-subprocess":"file:/tmp/plugin"}}`)
	if err := os.WriteFile(filepath.Join(profile, "package.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if !dshProfileHasPlugin("web") {
		t.Fatal("expected plugin dependency to be detected")
	}
}

func TestDshInstallProfilesIncludesShippedAndCustom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DSH_HOME", home)
	custom := filepath.Join(home, "profiles", "research")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "package.json"), []byte(`{"dependencies":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := dshInstallProfiles()
	want := map[string]bool{"acp": true, "web": true, "headless": true, "sdk": true, "sdk-minimal": true, "research": true}
	for _, profile := range got {
		delete(want, profile)
	}
	if len(want) != 0 {
		t.Fatalf("missing profiles: %#v (got %#v)", want, got)
	}
}
