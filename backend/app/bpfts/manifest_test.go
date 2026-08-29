package bpfts

import (
	"strings"
	"testing"
)

const validManifestJSON = `{
  "version": 1,
  "source": "examples/tls-write.ts",
  "probes": [
    {
      "name": "sslWrite",
      "section": "uprobe",
      "kind": "uprobe",
      "target": "SSL_write"
    },
    {
      "name": "onExec",
      "section": "tracepoint/syscalls/sys_enter_execve",
      "kind": "tracepoint",
      "category": "syscalls",
      "event": "sys_enter_execve"
    }
  ],
  "maps": [
    {"name": "events", "kind": "ringbuf", "maxEntries": 65536},
    {"name": "counts", "kind": "hash", "maxEntries": 1024}
  ]
}`

func TestParseManifestAcceptsVersionOneContract(t *testing.T) {
	manifest, err := ParseManifest(strings.NewReader(validManifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if manifest.Version != 1 || len(manifest.Probes) != 2 || len(manifest.Maps) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestParseManifestRejectsUnknownAndTrailingData(t *testing.T) {
	withUnknown := strings.Replace(validManifestJSON, `"source": "examples/tls-write.ts"`, `"source": "examples/tls-write.ts", "future": true`, 1)
	if _, err := ParseManifest(strings.NewReader(withUnknown)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}

	if _, err := ParseManifest(strings.NewReader(validManifestJSON + ` {"version":1}`)); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("expected trailing-value error, got %v", err)
	}
}

func TestManifestRejectsAttachMetadataMismatch(t *testing.T) {
	manifest, err := ParseManifest(strings.NewReader(validManifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	manifest.Probes[0].Section = "uprobe/SSL_write"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "section mismatch") {
		t.Fatalf("expected section mismatch, got %v", err)
	}
}

func TestManifestRejectsInvalidRingbufCapacityAndNameCollision(t *testing.T) {
	manifest, err := ParseManifest(strings.NewReader(validManifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	manifest.Maps[0].MaxEntries = 6000
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "power of two") {
		t.Fatalf("expected ringbuf capacity error, got %v", err)
	}

	manifest, err = ParseManifest(strings.NewReader(validManifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	manifest.Maps[0].Name = manifest.Probes[0].Name
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("expected name conflict, got %v", err)
	}
}
