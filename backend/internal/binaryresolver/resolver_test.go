package binaryresolver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinaryFindsPathAndShebang(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "agent-script")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolved := ResolveBinary("agent-script", dir)

	if resolved.Error != "" {
		t.Fatalf("ResolveBinary error = %q", resolved.Error)
	}
	if resolved.Path != script {
		t.Fatalf("Path = %q, want %q", resolved.Path, script)
	}
	if resolved.Shebang != "/usr/bin/env bash" {
		t.Fatalf("Shebang = %q, want /usr/bin/env bash", resolved.Shebang)
	}
	if resolved.ShebangTarget != "/usr/bin/env" {
		t.Fatalf("ShebangTarget = %q, want /usr/bin/env", resolved.ShebangTarget)
	}
}

func TestResolveBinaryHandlesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	resolved := ResolveBinary(link, "")

	if resolved.Error != "" {
		t.Fatalf("ResolveBinary error = %q", resolved.Error)
	}
	if resolved.RealPath != target {
		t.Fatalf("RealPath = %q, want %q", resolved.RealPath, target)
	}
}

func TestResolveBinaryParsesDockerTarget(t *testing.T) {
	resolved := ResolveBinary("docker://container-1:/usr/bin/python", "")

	if resolved.ContainerTarget != "container-1" {
		t.Fatalf("ContainerTarget = %q, want container-1", resolved.ContainerTarget)
	}
	if resolved.Input != "/usr/bin/python" {
		t.Fatalf("Input = %q, want inner path", resolved.Input)
	}
}
