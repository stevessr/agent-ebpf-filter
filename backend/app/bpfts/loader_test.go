package bpfts

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf"
)

func testManifest(t *testing.T) Manifest {
	t.Helper()
	manifest, err := ParseManifest(strings.NewReader(validManifestJSON))
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	return manifest
}

func TestValidateCollectionSpecRequiresExactProgramAndMapContract(t *testing.T) {
	manifest := testManifest(t)
	spec := &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"sslWrite": {SectionName: "uprobe"},
			"onExec":   {SectionName: "tracepoint/syscalls/sys_enter_execve"},
		},
		Maps: map[string]*ebpf.MapSpec{
			"events": {},
			"counts": {},
		},
	}
	if err := validateCollectionSpec(spec, manifest); err != nil {
		t.Fatalf("validateCollectionSpec() error = %v", err)
	}

	spec.Programs["hidden"] = &ebpf.ProgramSpec{SectionName: "kprobe/hidden"}
	if err := validateCollectionSpec(spec, manifest); err == nil || !strings.Contains(err.Error(), "undeclared program") {
		t.Fatalf("expected undeclared-program error, got %v", err)
	}
}

func TestValidateCollectionSpecRejectsWrongSectionAndExtraMap(t *testing.T) {
	manifest := testManifest(t)
	spec := &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"sslWrite": {SectionName: "uprobe/SSL_write"},
			"onExec":   {SectionName: "tracepoint/syscalls/sys_enter_execve"},
		},
		Maps: map[string]*ebpf.MapSpec{
			"events": {},
			"counts": {},
		},
	}
	if err := validateCollectionSpec(spec, manifest); err == nil || !strings.Contains(err.Error(), "section mismatch") {
		t.Fatalf("expected section mismatch, got %v", err)
	}

	spec.Programs["sslWrite"].SectionName = "uprobe"
	spec.Maps["hidden"] = &ebpf.MapSpec{}
	if err := validateCollectionSpec(spec, manifest); err == nil || !strings.Contains(err.Error(), "undeclared map") {
		t.Fatalf("expected undeclared-map error, got %v", err)
	}
}

func TestLoadAndAttachRequiresResolverBeforeLoadingUprobeObject(t *testing.T) {
	manifest := testManifest(t)
	if _, err := LoadAndAttach("does-not-exist.o", manifest, LoadOptions{}); err == nil || !strings.Contains(err.Error(), "requires a UprobeResolver") {
		t.Fatalf("expected resolver error before object loading, got %v", err)
	}
}

func TestLoadAndAttachRejectsInvalidManifestBeforeResolverRequirements(t *testing.T) {
	manifest := testManifest(t)
	manifest.Version = 99
	if _, err := LoadAndAttach("does-not-exist.o", manifest, LoadOptions{}); err == nil || !strings.Contains(err.Error(), "unsupported bpf-ts manifest version") {
		t.Fatalf("expected manifest validation error before resolver error, got %v", err)
	}
}
