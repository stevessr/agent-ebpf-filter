package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestKernelCapabilities(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{
		"proc/sys/kernel/osrelease": "7.2.6-test\n",
		"sys/kernel/security/lsm":   "lockdown, capability,landlock,bpf\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, version := range []string{"7.2.6-test", "6.18-test"} {
		if err := os.MkdirAll(filepath.Join(root, "lib/modules", version), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got := inspectKernelCapabilities(root, func() error { return nil })
	if got.Release != "7.2.6-test" || !got.BTFReadable || !got.BPFLSMEnabled || got.LSMError != "" {
		t.Fatalf("unexpected status: %+v", got)
	}
	if !reflect.DeepEqual(got.InstalledKernels, []string{"6.18-test", "7.2.6-test"}) {
		t.Fatalf("unexpected kernels: %v", got.InstalledKernels)
	}
	if len(got.LSMs) != 4 {
		t.Fatalf("unexpected LSMs: %v", got.LSMs)
	}
	if err := os.WriteFile(filepath.Join(root, "sys/kernel/security/lsm"), []byte("capability,not_bpf"), 0644); err != nil {
		t.Fatal(err)
	}
	got = inspectKernelCapabilities(root, func() error { return nil })
	if got.BPFLSMEnabled || got.LSMError != "" {
		t.Fatalf("must match exact bpf name: %+v", got)
	}
}

func TestKernelCapabilitiesUnavailable(t *testing.T) {
	got := inspectKernelCapabilities(t.TempDir(), func() error { return errors.New("invalid BTF header") })
	if got.BTFReadable || got.BTFError != "invalid BTF header" || got.LSMError == "" || got.ReleaseError == "" || got.KernelsError == "" || got.BPFLSMEnabled {
		t.Fatalf("unexpected status: %+v", got)
	}
	if got.LSMs == nil || got.InstalledKernels == nil {
		t.Fatal("empty inventories must serialize as arrays")
	}
}
