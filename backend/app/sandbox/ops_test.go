package sandbox

import (
	"errors"
	"net"

	"github.com/cilium/ebpf"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLsmPolicyKeys(t *testing.T) {
	execKey, err := lsmPathKeyFromString("/usr/bin/nc")
	if err != nil {
		t.Fatalf("lsmPathKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(execKey.Path[:]); got != "/usr/bin/nc" {
		t.Fatalf("exec key = %q", got)
	}

	fileKey, err := lsmNameKeyFromString("/home/agent/.ssh/id_rsa")
	if err != nil {
		t.Fatalf("lsmNameKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(fileKey.Name[:]); got != "id_rsa" {
		t.Fatalf("file key = %q", got)
	}

	execNameKey, err := lsmExecNameKeyFromString("/tmp/agent-os-block")
	if err != nil {
		t.Fatalf("lsmExecNameKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(execNameKey.Name[:]); got != "agent-os-block" {
		t.Fatalf("exec name key = %q", got)
	}

	if _, err := lsmPathKeyFromString(strings.Repeat("x", 256)); err == nil {
		t.Fatal("expected overlong exec path error")
	}
	if _, err := lsmNameKeyFromString(strings.Repeat("x", 64)); err == nil {
		t.Fatal("expected overlong file name error")
	}
}

func TestCgroupPIDResolutionHelpers(t *testing.T) {
	rel, err := unifiedCgroupRelativePath([]byte("12:cpu:/legacy\n0::/user.slice/test.scope\n"))
	if err != nil {
		t.Fatalf("unifiedCgroupRelativePath: %v", err)
	}
	if rel != "/user.slice/test.scope" {
		t.Fatalf("unified cgroup path = %q", rel)
	}

	root := t.TempDir()
	resolved, err := resolveCgroupPath(root, "/user.slice/test.scope")
	if err != nil {
		t.Fatalf("resolveCgroupPath: %v", err)
	}
	if want := filepath.Join(root, "user.slice", "test.scope"); resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}

	if got, err := resolveCgroupPath(root, "/"); err != nil || got != root {
		t.Fatalf("root cgroup path = %q, %v; want %q", got, err, root)
	}

	if got := ipv4StringFromBlockKey(0x7f000001); got != "127.0.0.1" {
		t.Fatalf("ipv4StringFromBlockKey = %q", got)
	}
	ip6Key, err := ip6BlockKeyFromIP(net.ParseIP("2001:db8::1"))
	if err != nil {
		t.Fatalf("ip6BlockKeyFromIP: %v", err)
	}
	if got := ip6StringFromBlockKey(ip6Key); got != "2001:db8::1" {
		t.Fatalf("ip6StringFromBlockKey = %q", got)
	}

	if got, err := parseCgroupID([]byte(`"18446744073709551615"`)); err != nil || got != ^uint64(0) {
		t.Fatalf("parse string cgroup id = %d, %v", got, err)
	}
	if got, err := parseCgroupID([]byte(`12345`)); err != nil || got != 12345 {
		t.Fatalf("parse numeric cgroup id = %d, %v", got, err)
	}
	if _, err := parseCgroupID([]byte(`0`)); err == nil {
		t.Fatal("expected zero cgroup id to be rejected")
	}
}

func TestCgroupSandboxAttachPathValidation(t *testing.T) {
	temp := t.TempDir()
	if err := validateCgroupSandboxAttachPath(temp); err == nil {
		t.Fatal("expected non-cgroup attach path to be rejected")
	}

	if st, err := os.Stat("/sys/fs/cgroup"); err == nil && st.IsDir() {
		err := validateCgroupSandboxAttachPath("/sys/fs/cgroup")
		if err != nil && strings.Contains(err.Error(), "not on a cgroup v2 filesystem") {
			t.Fatalf("/sys/fs/cgroup should be recognized as cgroup v2 when mounted: %v", err)
		}
	}
}

func TestCgroupSandboxPortValidation(t *testing.T) {
	if err := ValidatePort(1); err != nil {
		t.Fatalf("port 1 should be valid: %v", err)
	}
	if err := ValidatePort(65535); err != nil {
		t.Fatalf("port 65535 should be valid: %v", err)
	}
	if err := ValidatePort(0); err == nil {
		t.Fatal("port 0 should be rejected")
	}

	data, err := os.ReadFile("../handlers/cgroup_sandbox.go")
	if err != nil {
		t.Fatalf("read handlers/cgroup_sandbox.go: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"Deps.CgroupSandbox.ValidatePort(req.Port)",
		"c.JSON(http.StatusBadRequest",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("port handlers missing %q", want)
		}
	}
}

func TestCgroupSandboxIPValidation(t *testing.T) {
	if ip, text, err := ParseIP(" ::1 "); err != nil || text != "::1" || ip.To16() == nil {
		t.Fatalf("parse IPv6 = %v %q %v, want ::1", ip, text, err)
	}
	if ip, text, err := ParseIP(" ::ffff:127.0.0.1 "); err != nil || text != "127.0.0.1" || ip.To4() == nil {
		t.Fatalf("parse IPv4-mapped IPv6 = %v %q %v, want canonical 127.0.0.1", ip, text, err)
	}
	for _, fn := range []struct {
		name string
		call func(string) error
	}{
		{name: "parse", call: func(s string) error {
			_, _, err := ParseIP(s)
			return err
		}},
		{name: "block", call: BlockIP},
		{name: "unblock", call: UnblockIP},
	} {
		if err := fn.call("not-an-ip"); err == nil {
			t.Fatalf("%s accepted invalid IP", fn.name)
		}
	}
}

func stringFromNULBytes(b []byte) string {
	if idx := strings.IndexByte(string(b), 0); idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}

func TestOSPolicyMapPinsAreRestrictive(t *testing.T) {
	if CgroupSandboxMapPinMode != 0600 {
		t.Fatalf("cgroup sandbox map pin mode = %v, want 0600", CgroupSandboxMapPinMode)
	}
	if LsmEnforcerMapPinMode != 0600 {
		t.Fatalf("BPF LSM map pin mode = %v, want 0600", LsmEnforcerMapPinMode)
	}
}

func TestOSEnforcementUnblockIgnoresMissingMapKeys(t *testing.T) {
	if err := ignoreMissingMapKey(ebpf.ErrKeyNotExist); err != nil {
		t.Fatalf("missing map key should be idempotent: %v", err)
	}
	sentinel := errors.New("sentinel")
	if err := ignoreMissingMapKey(sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("non-missing map error = %v, want sentinel", err)
	}

	checks := []struct {
		paths    []string
		required []string
	}{
		{
			paths: []string{"cgroup_ops.go"},
			required: []string{
				"ignoreMissingMapKey(snap.CgroupBlocklist.Delete",
				"ignoreMissingMapKey(snap.IPBlocklist.Delete",
				"ignoreMissingMapKey(snap.IP6Blocklist.Delete",
				"ignoreMissingMapKey(snap.PortBlocklist.Delete",
			},
		},
		{
			paths: []string{"lsm_control.go"},
			required: []string{
				"ignoreMissingMapKey(snap.ExecPathBlocklist.Delete",
				"ignoreMissingMapKey(snap.ExecNameBlocklist.Delete",
				"ignoreMissingMapKey(snap.FileNameBlocklist.Delete",
			},
		},
	}
	for _, check := range checks {
		source := readSourceFiles(t, check.paths...)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing idempotent unblock wrapper %q", strings.Join(check.paths, ", "), want)
			}
		}
	}
}

func readSourceFiles(t *testing.T, paths ...string) string {
	t.Helper()
	var sb strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		sb.Write(data)
	}
	return sb.String()
}
