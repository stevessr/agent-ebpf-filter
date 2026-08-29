package bpfts

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf"
)

func exactSizedMapManifest() Manifest {
	return Manifest{
		Version: ManifestVersion,
		Source:  "sized.ts",
		Probes: []ManifestProbe{
			{Name: "onExec", Section: "tracepoint/syscalls/sys_enter_execve", Kind: "tracepoint", Category: "syscalls", Event: "sys_enter_execve"},
		},
		Maps: []ManifestMap{
			{Name: "events", Kind: "ringbuf", MaxEntries: 65536},
			{Name: "counts", Kind: "hash", MaxEntries: 1024, KeySize: 4, ValueSize: 8},
		},
	}
}

func exactSizedCollectionSpec() *ebpf.CollectionSpec {
	return &ebpf.CollectionSpec{
		Programs: map[string]*ebpf.ProgramSpec{
			"onExec": {SectionName: "tracepoint/syscalls/sys_enter_execve"},
		},
		Maps: map[string]*ebpf.MapSpec{
			"events": {Type: ebpf.RingBuf, MaxEntries: 65536},
			"counts": {Type: ebpf.Hash, MaxEntries: 1024, KeySize: 4, ValueSize: 8},
		},
	}
}

func TestValidateCollectionSpecRejectsMapKeyAndValueSizeDrift(t *testing.T) {
	manifest := exactSizedMapManifest()
	if err := validateCollectionSpec(exactSizedCollectionSpec(), manifest); err != nil {
		t.Fatalf("exact map ABI rejected: %v", err)
	}

	spec := exactSizedCollectionSpec()
	spec.Maps["counts"].KeySize = 8
	if err := validateCollectionSpec(spec, manifest); err == nil || !strings.Contains(err.Error(), "keySize mismatch") {
		t.Fatalf("expected keySize mismatch, got %v", err)
	}

	spec = exactSizedCollectionSpec()
	spec.Maps["counts"].ValueSize = 4
	if err := validateCollectionSpec(spec, manifest); err == nil || !strings.Contains(err.Error(), "valueSize mismatch") {
		t.Fatalf("expected valueSize mismatch, got %v", err)
	}
}

func TestManifestMapABISizesArePairedAndLegacyCompatible(t *testing.T) {
	manifest := exactSizedMapManifest()
	manifest.Maps[1].ValueSize = 0
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "keySize and valueSize together") {
		t.Fatalf("expected paired-size validation error, got %v", err)
	}

	legacy := exactSizedMapManifest()
	legacy.Maps[1].KeySize = 0
	legacy.Maps[1].ValueSize = 0
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy version-1 map ABI should remain valid: %v", err)
	}
	if err := validateCollectionSpec(exactSizedCollectionSpec(), legacy); err != nil {
		t.Fatalf("legacy version-1 map ABI should skip width locking: %v", err)
	}
}

func TestManifestRingbufRejectsMeaninglessABISizes(t *testing.T) {
	manifest := exactSizedMapManifest()
	manifest.Maps[0].KeySize = 4
	manifest.Maps[0].ValueSize = 8
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "must not declare keySize/valueSize") {
		t.Fatalf("expected ringbuf size validation error, got %v", err)
	}
}
