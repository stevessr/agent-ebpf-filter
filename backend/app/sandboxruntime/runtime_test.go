package sandboxruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSnapshotGVisorRunsc(t *testing.T) {
	d := detectSnapshot(
		4242,
		"runsc\n",
		[]byte("runsc\x00--root\x00/run/containerd/runsc\x00boot\x000123456789abcdef0123456789abcdef\x00"),
		"0::/system.slice/containerd.service\n",
	)
	if d.Kind != "gvisor" {
		t.Fatalf("kind=%q, want gvisor", d.Kind)
	}
	if d.ContainerID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("container id=%q", d.ContainerID)
	}
	if d.GuestSyscallsVisible {
		t.Fatal("gVisor guest syscalls must not be marked host-visible")
	}
	if d.Confidence < 0.9 {
		t.Fatalf("confidence=%v, want strong process attribution", d.Confidence)
	}
}

func TestDetectSnapshotGVisorNamedContainer(t *testing.T) {
	d := detectSnapshot(
		99,
		"runsc\n",
		[]byte("runsc\x00--root\x00/run/runsc\x00boot\x00sandbox.dev-1\x00"),
		"0::/system.slice/runsc.service\n",
	)
	if d.ContainerID != "sandbox.dev-1" {
		t.Fatalf("container id=%q, want named gVisor id", d.ContainerID)
	}
}

func TestDetectSnapshotKataFromShimID(t *testing.T) {
	d := detectSnapshot(
		77,
		"containerd-shim-kata-v2\n",
		[]byte("containerd-shim-kata-v2\x00-namespace\x00k8s.io\x00-id\x00pod-sandbox-123\x00"),
		"0::/kubepods.slice/pod123.slice\n",
	)
	if d.Kind != "kata" {
		t.Fatalf("kind=%q, want kata", d.Kind)
	}
	if d.ContainerID != "pod-sandbox-123" {
		t.Fatalf("container id=%q", d.ContainerID)
	}
}

func TestDetectSnapshotOCIFromCgroup(t *testing.T) {
	const id = "9f0d73e8071d69cf8b03793d41fb57968c6e554a89db5ec4b3fa941e7a18f2b1"
	d := detectSnapshot(
		101,
		"python\n",
		[]byte("python\x00agent.py\x00"),
		"0::/system.slice/docker-"+id+".scope\n",
	)
	if d.ContainerID != id {
		t.Fatalf("container id=%q, want %q", d.ContainerID, id)
	}
	if d.Kind != "" {
		t.Fatalf("kind=%q, Docker cgroup alone cannot prove OCI runtime", d.Kind)
	}
}

func TestDetectSnapshotRejectsRuntimeNameInApplicationArgs(t *testing.T) {
	d := detectSnapshot(1234, "python\n",
		[]byte("python\x00--id\x00fake-123\x00runsc\x00"),
		"0::/system.slice/app.service\n")
	if d.Kind != "" || d.ContainerID != "" {
		t.Fatalf("runtime and container must not be inferred from unrelated application argv: %+v", d)
	}
}

func TestManagerDetectPIDFromProcFixture(t *testing.T) {
	root := t.TempDir()
	pidDir := filepath.Join(root, "55")
	if err := os.Mkdir(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(pidDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("comm", []byte("firecracker\n"))
	write("cmdline", []byte("firecracker\x00--id\x00vm-01\x00"))
	write("cgroup", []byte("0::/machine.slice/firecracker-vm-01.scope\n"))

	m := newManagerForTest(root, func(string) (string, error) { return "", errors.New("not found") })
	d, err := m.DetectPID(55)
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "firecracker" || d.ContainerID != "vm-01" {
		t.Fatalf("unexpected detection: %+v", d)
	}
}

func TestStatusReportsKnownRuntimes(t *testing.T) {
	m := newManagerForTest("/proc", func(name string) (string, error) {
		if name == "runsc" {
			return "/usr/bin/runsc", nil
		}
		return "", errors.New("missing")
	})
	s := m.Status()
	if len(s.SupportedRuntimes) < 6 {
		t.Fatalf("supported runtimes=%d", len(s.SupportedRuntimes))
	}
	if got := s.SupportedRuntimes[0].DetectedBinaries; len(got) != 1 || got[0] != "/usr/bin/runsc" {
		t.Fatalf("gVisor binaries=%v", got)
	}
}
