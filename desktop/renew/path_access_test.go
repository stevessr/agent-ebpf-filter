package main

import (
	"path/filepath"
	"testing"
)

func TestSensitivePathPresetsAreExactAndUserScoped(t *testing.T) {
	home := "/home/example-user"
	presets := commonSensitivePaths(home)
	if len(presets) < 10 { t.Fatalf("need common path presets, got %d", len(presets)) }
	seen := map[string]bool{}
	for _, p := range presets {
		if !filepath.IsAbs(p.Path) || p.Path == "/" || seen[p.Path] {
			t.Fatalf("unsafe or duplicate preset: %q", p.Path)
		}
		seen[p.Path] = true
	}
	if !seen[home+"/.ssh/id_ed25519"] || !seen[home+"/.aws/credentials"] {
		t.Fatalf("expected home-relative identity presets, got %#v", presets)
	}
}

func TestPathAccessModeIsExplicit(t *testing.T) {
	for _, tc := range []struct{ mode string; read, write bool }{
		{"阻止读取", true, false},
		{"阻止写入", false, true},
		{"阻止读写", true, true},
	} {
		r, w, err := pathAccessBits(tc.mode)
		if err != nil || r != tc.read || w != tc.write { t.Errorf("%s: %v %v %v", tc.mode, r, w, err) }
	}
	if _, _, err := pathAccessBits("默认阻止"); err == nil { t.Fatal("unexpected implicit policy mode") }
}
