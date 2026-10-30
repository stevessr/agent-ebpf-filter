package main

import (
	"os"
	"testing"

	"github.com/cilium/ebpf"
)

func TestReviewedPortInput(t *testing.T) {
	if _, err := parsePorts(""); err == nil {
		t.Fatal("missing explicit review scope accepted")
	}
	if _, err := parsePorts("22,22"); err == nil {
		t.Fatal("duplicate port accepted")
	}
	if _, err := parsePorts("0"); err == nil {
		t.Fatal("port zero accepted")
	}
	if _, err := parsePorts("65536"); err == nil {
		t.Fatal("out-of-range port accepted")
	}
	if ports, err := parsePorts("22,4444"); err != nil || len(ports) != 2 || ports[0] != 22 || ports[1] != 4444 {
		t.Fatalf("valid reviewed ports rejected: %v / %v", ports, err)
	}
}

func TestRootCgroupAlwaysRejected(t *testing.T) {
	if _, err := scopeCgroup("/"); err == nil {
		t.Fatal("host root cgroup scope accepted")
	}
}

func TestCompiledELFLayoutWithoutKernelPrivileges(t *testing.T) {
	object := os.Getenv("KERNEL_GUARD_OBJECT")
	if object == "" {
		t.Skip("set KERNEL_GUARD_OBJECT to compiled ELF for ABI preflight")
	}
	spec, err := ebpf.LoadCollectionSpec(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCollectionSpec(spec); err != nil {
		t.Fatal(err)
	}
	// Exercise a genuinely incompatible object as an explicit failure case.
	invalid := spec.Copy()
	invalid.Maps["kernel_guard_config"].ValueSize++
	if err := validateCollectionSpec(invalid); err == nil {
		t.Fatal("ABI preflight accepted corrupted model map size")
	}
}
