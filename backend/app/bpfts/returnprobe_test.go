package bpfts

import (
	"strings"
	"testing"
)

func TestManifestAcceptsReturnProbeKinds(t *testing.T) {
	manifestJSON := `{
		"version": 1,
		"source": "tls-read.ts",
		"probes": [
			{"name":"enter","section":"uprobe","kind":"uprobe","target":"SSL_read"},
			{"name":"exit","section":"uretprobe","kind":"uretprobe","target":"SSL_read"},
			{"name":"kernelExit","section":"kretprobe/do_sys_open","kind":"kretprobe","target":"do_sys_open"}
		],
		"maps": [
			{"name":"pending","kind":"hash","maxEntries":1024}
		]
	}`
	manifest, err := ParseManifest(strings.NewReader(manifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if got := manifest.Probes[1].Kind; got != "uretprobe" {
		t.Fatalf("return probe kind = %q, want uretprobe", got)
	}
	if !needsUprobeResolver(manifest.Probes[1].Kind) {
		t.Fatalf("uretprobe must require the userspace resolver")
	}
	if needsUprobeResolver(manifest.Probes[2].Kind) {
		t.Fatalf("kretprobe must not require the userspace resolver")
	}
}

func TestManifestRejectsReturnProbeSectionMismatch(t *testing.T) {
	manifestJSON := `{
		"version": 1,
		"source": "bad.ts",
		"probes": [
			{"name":"exit","section":"uprobe","kind":"uretprobe","target":"SSL_read"}
		],
		"maps": []
	}`
	if _, err := ParseManifest(strings.NewReader(manifestJSON)); err == nil || !strings.Contains(err.Error(), "section mismatch") {
		t.Fatalf("expected return-probe section mismatch, got %v", err)
	}
}

func TestLoadAndAttachRequiresResolverForUretprobeBeforeObjectLoad(t *testing.T) {
	manifest := Manifest{
		Version: ManifestVersion,
		Source:  "tls-read.ts",
		Probes: []ManifestProbe{{
			Name:    "exit",
			Section: "uretprobe",
			Kind:    "uretprobe",
			Target:  "SSL_read",
		}},
	}
	if _, err := LoadAndAttach("does-not-exist.o", manifest, LoadOptions{}); err == nil || !strings.Contains(err.Error(), "requires a UprobeResolver") {
		t.Fatalf("expected uretprobe resolver error before object loading, got %v", err)
	}
}

func TestAttachUserProbeRejectsNilResolverWithoutPanic(t *testing.T) {
	probe := ManifestProbe{Name: "exit", Kind: "uretprobe", Section: "uretprobe", Target: "SSL_read"}
	if _, err := attachUserProbe(probe, nil, nil); err == nil || !strings.Contains(err.Error(), "requires a UprobeResolver") {
		t.Fatalf("expected nil-resolver error, got %v", err)
	}
}
