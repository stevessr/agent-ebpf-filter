package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

func TestCgroupSandboxObjectSections(t *testing.T) {
	spec, err := bpf.LoadAgentCgroupSandbox()
	if err != nil {
		t.Fatalf("load cgroup sandbox spec: %v", err)
	}

	assertProgramSpec(t, spec, "cgroup_sandbox_connect4", ebpf.CGroupSockAddr, ebpf.AttachCGroupInet4Connect, "cgroup/connect4")
	assertProgramSpec(t, spec, "cgroup_sandbox_connect6", ebpf.CGroupSockAddr, ebpf.AttachCGroupInet6Connect, "cgroup/connect6")
	assertProgramSpec(t, spec, "cgroup_sandbox_sendmsg4", ebpf.CGroupSockAddr, ebpf.AttachCGroupUDP4Sendmsg, "cgroup/sendmsg4")
	assertProgramSpec(t, spec, "cgroup_sandbox_sendmsg6", ebpf.CGroupSockAddr, ebpf.AttachCGroupUDP6Sendmsg, "cgroup/sendmsg6")
	assertMapSpec(t, spec, "cgroup_blocklist", ebpf.Hash, 256, 8, 4)
	assertMapSpec(t, spec, "ip_blocklist", ebpf.Hash, 1024, 4, 4)
	assertMapSpec(t, spec, "ip6_blocklist", ebpf.Hash, 1024, 16, 4)
	assertMapSpec(t, spec, "port_blocklist", ebpf.Hash, 256, 4, 4)
	assertMapSpec(t, spec, "cgroup_sandbox_stats", ebpf.PerCPUArray, 1, 4, 24)
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "ip6_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "ip6_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "cgroup_sandbox_stats")
}

func TestLsmEnforcerObjectSections(t *testing.T) {
	spec, err := bpf.LoadAgentLsmEnforcer()
	if err != nil {
		t.Fatalf("load BPF LSM enforcer spec: %v", err)
	}

	assertProgramSpec(t, spec, "lsm_enforce_bprm_check", ebpf.LSM, ebpf.AttachLSMMac, "lsm/bprm_check_security")
	assertProgramSpec(t, spec, "lsm_enforce_file_open", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_open")
	assertProgramSpec(t, spec, "lsm_enforce_file_permission", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_permission")
	assertProgramSpec(t, spec, "lsm_enforce_mmap_file", ebpf.LSM, ebpf.AttachLSMMac, "lsm/mmap_file")
	assertProgramSpec(t, spec, "lsm_enforce_file_mprotect", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_mprotect")
	assertProgramSpec(t, spec, "lsm_enforce_inode_setattr", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_setattr")
	assertProgramSpec(t, spec, "lsm_enforce_inode_create", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_create")
	assertProgramSpec(t, spec, "lsm_enforce_inode_link", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_link")
	assertProgramSpec(t, spec, "lsm_enforce_inode_unlink", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_unlink")
	assertProgramSpec(t, spec, "lsm_enforce_inode_symlink", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_symlink")
	assertProgramSpec(t, spec, "lsm_enforce_inode_mkdir", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_mkdir")
	assertProgramSpec(t, spec, "lsm_enforce_inode_rmdir", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_rmdir")
	assertProgramSpec(t, spec, "lsm_enforce_inode_mknod", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_mknod")
	assertProgramSpec(t, spec, "lsm_enforce_inode_rename", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_rename")
	assertMapSpec(t, spec, "lsm_blocked_exec_paths", ebpf.Hash, 512, 256, 4)
	assertMapSpec(t, spec, "lsm_blocked_exec_names", ebpf.Hash, 512, 64, 4)
	assertMapSpec(t, spec, "lsm_blocked_file_names", ebpf.Hash, 512, 64, 4)
	assertMapSpec(t, spec, "lsm_enforcer_stats_map", ebpf.PerCPUArray, 1, 4, 32)
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_blocked_exec_paths")
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_blocked_exec_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_open", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_open", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_permission", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_permission", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_mmap_file", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_mmap_file", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_mprotect", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_mprotect", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_setattr", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_setattr", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_create", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_create", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_link", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_link", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_unlink", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_unlink", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_symlink", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_symlink", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mkdir", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mkdir", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rmdir", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rmdir", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mknod", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mknod", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rename", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rename", "lsm_enforcer_stats_map")
}

func TestCgroupSandboxPolicySourceUsesHostOrderKeys(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("ebpf", "cgroup_sandbox.c"))
	if err != nil {
		t.Fatalf("read cgroup sandbox source: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"bpf_get_current_cgroup_id()",
		"bpf_ntohl(ctx->user_ip4)",
		"bpf_ntohl(ctx->user_ip6[0])",
		"ip6_blocklist",
		"ipv6_is_v4_mapped",
		"mapped_v4_is_blocked",
		"::ffff:a.b.c.d",
		"bpf_ntohs(ctx->user_port)",
		"SEC(\"cgroup/sendmsg4\")",
		"SEC(\"cgroup/sendmsg6\")",
		"unconnected UDP sendto()/sendmsg() gap",
		"return 0; // block",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("cgroup sandbox source missing %q", want)
		}
	}
}

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

func TestLsmPolicySourceUsesCurrentHookArguments(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("ebpf", "lsm_enforcer.c"))
	if err != nil {
		t.Fatalf("read BPF LSM source: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"SEC(\"lsm/file_permission\")",
		"SEC(\"lsm/mmap_file\")",
		"SEC(\"lsm/file_mprotect\")",
		"SEC(\"lsm/inode_setattr\")",
		"struct file *file, int mask, int ret",
		"struct file *file, unsigned long reqprot",
		"struct vm_area_struct *vma, unsigned long reqprot",
		"BPF_CORE_READ(vma, vm_file)",
		"struct mnt_idmap *idmap, struct dentry *dentry",
		"SEC(\"lsm/inode_rename\")",
		"struct inode *new_dir, struct dentry *new_dentry, int ret",
		"BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name)",
		"if (ret != 0)",
		"return -EACCES;",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("BPF LSM source missing hook contract %q", want)
		}
	}
	if strings.Contains(source, "struct inode *new_dir, struct dentry *new_dentry, unsigned int flags, int ret") {
		t.Fatal("BPF LSM inode_rename signature must match the current vmlinux hook and not read ret from the wrong ctx slot")
	}
	if strings.Count(source, "BPF_CORE_READ(old_dentry, d_name.name)") < 2 {
		t.Fatal("BPF LSM should check old_dentry basenames for both hard-link source protection and rename-away protection")
	}
	if strings.Count(source, "BPF_CORE_READ(new_dentry, d_name.name)") < 2 {
		t.Fatal("BPF LSM should check new_dentry basenames for both hard-link destination protection and rename-into protection")
	}
}

func TestOSSmokeScriptExists(t *testing.T) {
	path := filepath.Join("..", "scripts", "os-enforcement-smoke.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, want := range []string{
		"/sandbox/lsm/block-exec-path",
		"/sandbox/lsm/block-exec-name",
		"/sandbox/lsm/block-file-name",
		"/sandbox/cgroup/block-port",
		`"attached":true`,
		"OS_SMOKE_START_BACKEND",
		"OS_SMOKE_PRIVILEGE_CMD",
		"build_backend_privilege_prefix",
		"custom_privilege_prefix",
		"cleanup_policy_entries",
		"assert_idempotent_unblock()",
		"assert_idempotent_unblock /sandbox/lsm/unblock-exec-path",
		"assert_idempotent_unblock /sandbox/lsm/unblock-exec-name",
		"assert_idempotent_unblock /sandbox/lsm/unblock-file-name",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-cgroup",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-pid",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-ip",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-port",
		"blocked_exec_paths",
		"blocked_exec_names",
		"blocked_file_names",
		"blocked_cgroups",
		"blocked_ips",
		"blocked_ports",
		"BPF LSM exec basename symlink-alias block",
		"agent-ebpf-lsm-exec-alias",
		"expected exec basename to block symlink alias execution",
		"BPF LSM file_open write block",
		"expected file write open to be blocked",
		"BPF LSM file_permission existing-fd block",
		"expected existing file descriptor I/O to be blocked",
		"BPF LSM mmap_file existing-fd block",
		"expected existing file descriptor mmap to be blocked",
		"BPF LSM file_mprotect existing-map block",
		"expected existing file mapping mprotect to be blocked",
		"start_existing_fd_setattr_probe",
		"BPF LSM inode_setattr existing-fd ftruncate block",
		"expected existing file descriptor ftruncate to be blocked",
		"start_existing_fd_fchmod_probe",
		"BPF LSM inode_setattr existing-fd fchmod block",
		"expected existing file descriptor fchmod to be blocked",
		"BPF LSM inode_setattr block",
		"BPF LSM inode_unlink block",
		"BPF LSM inode_rmdir block",
		"BPF LSM inode_rename block",
		"BPF LSM inode_rename destination block",
		"agent-ebpf-lsm-rename-dst",
		"expected rename into blocked destination basename to be blocked",
		"/sandbox/cgroup/block-pid",
		"cgroup.procs",
		"json_field cgroupPath",
		"run_cgroup_pid_block_smoke",
		"cgroup/connect PID cgroup unblock-pid",
		"expected PID-cgroup connect to succeed after unblock-pid",
		"run_ip_block_smoke 127.0.0.2 IPv4-loopback-alias",
		"run_ip_block_smoke ::1 IPv6-loopback",
		"run_port_block_smoke ::1 IPv6",
		"python_udp_connect",
		"start_udp_sendto_probe",
		"run_udp_sendto_probe_in_cgroup",
		"expected baseline PID-cgroup UDP sendto to succeed",
		"cgroup/sendmsg PID cgroup UDP sendto block",
		"expected PID-cgroup UDP sendto to be blocked",
		"expected PID-cgroup UDP sendto to succeed after unblock-pid",
		"run_udp_ip_block_smoke 127.0.0.2 IPv4-UDP-loopback-alias",
		"run_udp_port_block_smoke 127.0.0.1 IPv4-UDP",
		"run_udp_ip_block_smoke ::1 IPv6-UDP-loopback",
		"run_udp_port_block_smoke ::1 IPv6-UDP",
		"python_udp_sendto",
		"run_udp_sendto_ip_block_smoke 127.0.0.2 IPv4-UDP-sendto-loopback-alias",
		"run_udp_sendto_port_block_smoke 127.0.0.1 IPv4-UDP-sendto",
		"run_udp_sendto_ip_block_smoke ::1 IPv6-UDP-sendto-loopback",
		"run_udp_sendto_port_block_smoke ::1 IPv6-UDP-sendto",
		"start_connected_udp_send_probe",
		"run_udp_existing_connected_send_ip_block_smoke 127.0.0.2 IPv4-UDP-existing-connected-loopback-alias",
		"run_udp_existing_connected_send_port_block_smoke 127.0.0.1 IPv4-UDP-existing-connected",
		"run_udp_existing_connected_send_ip_block_smoke ::1 IPv6-UDP-existing-connected-loopback",
		"run_udp_existing_connected_send_port_block_smoke ::1 IPv6-UDP-existing-connected",
		"ipv4_mapped_loopback_available",
		"run_ipv4_mapped_ip_block_smoke",
		"run_udp_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-loopback",
		"run_udp_sendto_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-sendto-loopback",
		"run_udp_existing_connected_send_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-existing-connected-loopback",
		"expected existing UDP connected-socket",
		"BPF LSM inode_create block",
		"BPF LSM inode_link block",
		"BPF LSM inode_link source block",
		"agent-ebpf-lsm-link-src-blocked",
		"expected hard link from blocked source basename to be blocked",
		"BPF LSM inode_symlink block",
		"BPF LSM inode_mkdir block",
		"BPF LSM inode_mknod block",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing %s", path, want)
		}
	}
}

func TestOSPreflightScriptExists(t *testing.T) {
	path := filepath.Join("..", "scripts", "os-enforcement-preflight.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, want := range []string{
		"/sys/fs/bpf",
		"/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps",
		"/sys/fs/cgroup/cgroup.controllers",
		"AGENT_CGROUP_SANDBOX_PATH",
		"OS_SMOKE_PRIVILEGE_CMD",
		"cgroup.procs",
		"cgroup2fs",
		"can create a temporary cgroup below the sandbox attach path",
		"run_privileged_preflight",
		"ip6_blocklist",
		"/sys/kernel/security/lsm",
		"pinned maps include expected entries",
		"running as root; sudo is not required",
		"sudo -n true",
		"custom OS_SMOKE_PRIVILEGE_CMD runs commands as root",
		"writable through passwordless sudo",
		"writable through OS_SMOKE_PRIVILEGE_CMD",
		"curl is available",
		"python3 is available",
		"check_object_section",
		"cgroup2fs",
		"cgroup.procs",
		"cgroup/connect6",
		"cgroup/sendmsg4",
		"cgroup/sendmsg6",
		"lsm/file_permission",
		"lsm/mmap_file",
		"lsm/file_mprotect",
		"lsm/inode_setattr",
		"lsm/inode_create",
		"lsm/inode_link",
		"lsm/inode_unlink",
		"lsm/inode_symlink",
		"lsm/inode_mkdir",
		"lsm/inode_rmdir",
		"lsm/inode_mknod",
		"lsm/inode_rename",
		"os-enforcement-smoke-start",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing %s", path, want)
		}
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
	if err := validateCgroupSandboxPort(1); err != nil {
		t.Fatalf("port 1 should be valid: %v", err)
	}
	if err := validateCgroupSandboxPort(65535); err != nil {
		t.Fatalf("port 65535 should be valid: %v", err)
	}
	if err := validateCgroupSandboxPort(0); err == nil {
		t.Fatal("port 0 should be rejected")
	}

	data, err := os.ReadFile("cgroup_sandbox_control.go")
	if err != nil {
		t.Fatalf("read cgroup_sandbox_control.go: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"validateCgroupSandboxPort(req.Port)",
		"c.JSON(http.StatusBadRequest",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("port handlers missing %q", want)
		}
	}
}

func TestCgroupSandboxIPValidation(t *testing.T) {
	if ip, text, err := parseCgroupSandboxIP(" ::1 "); err != nil || text != "::1" || ip.To16() == nil {
		t.Fatalf("parse IPv6 = %v %q %v, want ::1", ip, text, err)
	}
	if ip, text, err := parseCgroupSandboxIP(" ::ffff:127.0.0.1 "); err != nil || text != "127.0.0.1" || ip.To4() == nil {
		t.Fatalf("parse IPv4-mapped IPv6 = %v %q %v, want canonical 127.0.0.1", ip, text, err)
	}
	for _, fn := range []struct {
		name string
		call func(string) error
	}{
		{name: "parse", call: func(s string) error {
			_, _, err := parseCgroupSandboxIP(s)
			return err
		}},
		{name: "block", call: blockIP},
		{name: "unblock", call: unblockIP},
	} {
		if err := fn.call("not-an-ip"); err == nil {
			t.Fatalf("%s accepted invalid IP", fn.name)
		}
	}
}

func TestOSEnforcementMutationHandlersRejectInvalidInputBeforeLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{name: "cgroup block pid", handler: handleCgroupSandboxBlockPID, body: `{"pid":0}`},
		{name: "cgroup unblock pid", handler: handleCgroupSandboxUnblockPID, body: `{"pid":0}`},
		{name: "cgroup block missing pid", handler: handleCgroupSandboxBlockPID, body: `{"pid":2147483647}`},
		{name: "cgroup unblock missing pid", handler: handleCgroupSandboxUnblockPID, body: `{"pid":2147483647}`},
		{name: "lsm block exec path", handler: handleLsmBlockExecPath, body: `{"path":""}`},
		{name: "lsm unblock exec path", handler: handleLsmUnblockExecPath, body: `{"path":""}`},
		{name: "lsm block exec name", handler: handleLsmBlockExecName, body: `{"name":"/"}`},
		{name: "lsm unblock exec name", handler: handleLsmUnblockExecName, body: `{"name":"/"}`},
		{name: "lsm block file name", handler: handleLsmBlockFileName, body: `{"name":""}`},
		{name: "lsm unblock file name", handler: handleLsmUnblockFileName, body: `{"name":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			tc.handler(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s, want 400", w.Code, w.Body.String())
			}
		})
	}
}

func TestOSPolicyMapPinsAreRestrictive(t *testing.T) {
	if cgroupSandboxMapPinMode != 0600 {
		t.Fatalf("cgroup sandbox map pin mode = %v, want 0600", cgroupSandboxMapPinMode)
	}
	if lsmEnforcerMapPinMode != 0600 {
		t.Fatalf("BPF LSM map pin mode = %v, want 0600", lsmEnforcerMapPinMode)
	}
}

func TestOSEnforcementStartsWithoutDefaultBlockEntries(t *testing.T) {
	for _, path := range []string{"main.go", "cgroup_sandbox_control.go", "lsm_enforcer_control.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "autoBlockHighRiskEndpoints") || strings.Contains(string(data), "highRiskPorts") {
			t.Fatalf("%s installs implicit OS block entries; OS enforcement should start from explicit UI/API map entries", path)
		}
	}
}

func TestOSEnforcementMutationRoutesArePolicyGated(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	source := string(data)
	for _, route := range []string{
		"/sandbox/cgroup/block-cgroup",
		"/sandbox/cgroup/unblock-cgroup",
		"/sandbox/cgroup/block-pid",
		"/sandbox/cgroup/unblock-pid",
		"/sandbox/cgroup/block-ip",
		"/sandbox/cgroup/unblock-ip",
		"/sandbox/cgroup/block-port",
		"/sandbox/cgroup/unblock-port",
		"/sandbox/lsm/block-exec-path",
		"/sandbox/lsm/unblock-exec-path",
		"/sandbox/lsm/block-exec-name",
		"/sandbox/lsm/unblock-exec-name",
		"/sandbox/lsm/block-file-name",
		"/sandbox/lsm/unblock-file-name",
	} {
		want := `r.POST("` + route + `", authMiddleware(), policyManagementEnabledMiddleware(),`
		if !strings.Contains(source, want) {
			t.Fatalf("route %s is not registered with auth + policy management gate", route)
		}
	}
}

func TestOSEnforcementStatusRoutesRequireAuth(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	source := string(data)
	for _, route := range []string{
		"/sandbox/cgroup/status",
		"/sandbox/lsm/status",
	} {
		want := `r.GET("` + route + `", authMiddleware(),`
		if !strings.Contains(source, want) {
			t.Fatalf("status route %s is not registered with auth middleware", route)
		}
	}

	for _, check := range []struct {
		path string
		want string
	}{
		{path: filepath.Join("..", "AGENTS.md"), want: "/sandbox/**"},
		{path: filepath.Join("..", "README.md"), want: "OS sandbox (`/sandbox/**`)"},
	} {
		doc, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		if !strings.Contains(string(doc), check.want) {
			t.Fatalf("%s missing auth coverage note %q", check.path, check.want)
		}
	}
}

func TestOSSecurityDocsDescribeCurrentKernelEnforcement(t *testing.T) {
	checks := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: filepath.Join("..", "docs", "threat-model.md"),
			required: []string{
				"Current OS-level enforcement focus",
				"cgroup/connect and cgroup/sendmsg programs can reject",
				"existing connected UDP sends",
				"IPv4-mapped IPv6 destinations",
				"BPF LSM programs can reject",
				"existing-fd `ftruncate` / `fchmod` via `setattr`",
				"not recursive workspace sandboxes",
				"escape defenses",
			},
		},
		{
			path: filepath.Join("..", "docs", "security-model.md"),
			required: []string{
				"`/sandbox/**`",
				"Kernel-enforced policy paths",
				"cgroup/connect and cgroup/sendmsg blocking for exact cgroup ids, IPv4/IPv6 destinations, and",
				"existing connected UDP send",
				"IPv4 block entries are also honored for IPv4-mapped IPv6 socket",
				"BPF LSM blocking for executable paths, executable basenames, and file or",
				"`file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`",
				"`inode_mknod`, and `inode_rename`",
				"existing-fd `ftruncate` / `fchmod`",
				"own cgroup / LSM policy-map mutation and attach lifecycle",
			},
			forbidden: []string{
				"apply future cgroup / LSM policy",
			},
		},
		{
			path: filepath.Join("..", "docs", "policy-semantics.md"),
			required: []string{
				"OS-level cgroup/connect + sendmsg policy",
				"TCP/UDP destination port",
				"unconnected UDP sendto/sendmsg",
				"existing connected UDP sends",
				"IPv4 block entries also deny IPv4-mapped IPv6 destinations",
				"API inputs in that form normalize to the equivalent IPv4 block key",
				"Existing TCP streams established before a matching block is added are not",
				"Existing-fd `ftruncate` / `fchmod`-style",
				"OS-level BPF LSM policy",
				"Matching LSM decisions return `EACCES`",
				"File/directory LSM matching is basename-based today",
				"not in the synchronous cgroup/LSM decision path",
			},
			forbidden: []string{
				"not kernel-enforced policy decisions yet.",
				"optional kernel blocking via cgroup hooks or BPF LSM",
			},
		},
		{
			path: filepath.Join("..", "README.md"),
			required: []string{
				"`GET /sandbox/cgroup/status`",
				"`checked` / `blocked` / `allowed`",
				"legacy `connect*` aliases",
				"IPv4-mapped IPv6-destination",
				"existing connected UDP sends",
			},
		},
		{
			path: "README.md",
			required: []string{
				"`GET /sandbox/cgroup/status` returns",
				"decision counters as `checked` / `blocked` / `allowed` plus legacy `connect*` aliases",
				"IPv4-mapped IPv6-destination",
				"existing connected UDP sends",
			},
		},
	}

	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		doc := string(data)
		for _, want := range check.required {
			if !strings.Contains(doc, want) {
				t.Fatalf("%s missing OS enforcement doc marker %q", check.path, want)
			}
		}
		for _, bad := range check.forbidden {
			if strings.Contains(doc, bad) {
				t.Fatalf("%s still contains stale OS enforcement wording %q", check.path, bad)
			}
		}
	}
}

func TestOSFrontendSecuritySurfaceWiresSandboxEndpoints(t *testing.T) {
	composablePath := filepath.Join("..", "frontend", "src", "composables", "useConfigSecurity.ts")
	composableData, err := os.ReadFile(composablePath)
	if err != nil {
		t.Fatalf("read %s: %v", composablePath, err)
	}
	composable := string(composableData)
	for _, want := range []string{
		"axios.get('/sandbox/cgroup/status')",
		"'/sandbox/cgroup/block-cgroup'",
		"'/sandbox/cgroup/unblock-cgroup'",
		"'/sandbox/cgroup/block-pid'",
		"'/sandbox/cgroup/unblock-pid'",
		"'/sandbox/cgroup/block-ip'",
		"'/sandbox/cgroup/unblock-ip'",
		"'/sandbox/cgroup/block-port'",
		"'/sandbox/cgroup/unblock-port'",
		"axios.get('/sandbox/lsm/status')",
		"'/sandbox/lsm/block-exec-path'",
		"'/sandbox/lsm/unblock-exec-path'",
		"'/sandbox/lsm/block-exec-name'",
		"'/sandbox/lsm/unblock-exec-name'",
		"'/sandbox/lsm/block-file-name'",
		"'/sandbox/lsm/unblock-file-name'",
		"blockedCgroups:",
		"blockedIPs:",
		"ip6Blocklist:",
		"blockedPorts:",
		"checked: 0",
		"blocked: 0",
		"allowed: 0",
		"blockedExecPaths:",
		"blockedExecNames:",
		"blockedFileNames:",
		"请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址",
		"CgroupSandboxSuccessText",
		"axios.post<CgroupSandboxActionResponse>",
		"data.ip || ip",
		"打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename",
		"fetchCgroupSandboxStatus,",
		"fetchLsmEnforcerStatus,",
	} {
		if !strings.Contains(composable, want) {
			t.Fatalf("%s missing frontend sandbox contract %q", composablePath, want)
		}
	}

	componentPath := filepath.Join("..", "frontend", "src", "components", "config", "ConfigSecurityTab.vue")
	componentData, err := os.ReadFile(componentPath)
	if err != nil {
		t.Fatalf("read %s: %v", componentPath, err)
	}
	component := string(componentData)
	for _, want := range []string{
		"OS-Level cgroup Network Interception",
		"TCP/UDP connected sockets",
		"UDP sendto/sendmsg",
		"IPv4-mapped IPv6 socket",
		"OS-Level BPF LSM File / Exec Interception",
		"1.2.3.4, ::ffff:1.2.3.4, or ::1",
		"cgroupSandboxStatus.stats.checked",
		"cgroupSandboxStatus.stats.blocked",
		"cgroupSandboxStatus.stats.allowed",
		"file_open",
		"file_permission",
		"mmap_file",
		"file_mprotect",
		"inode_setattr",
		"inode_create",
		"inode_link",
		"inode_unlink",
		"inode_symlink",
		"inode_mkdir",
		"inode_rmdir",
		"inode_mknod",
		"inode_rename",
		"打开、既有 fd 读写、mmap、mprotect、setattr、创建、link、symlink、删除、mkdir、rmdir、mknod 与 rename",
		"@click=\"blockCgroupID\"",
		"@click=\"blockCgroupPID\"",
		"@click=\"blockCgroupIP\"",
		"@click=\"blockCgroupPort\"",
		"@close.prevent=\"unblockCgroupIDFromTag(id)\"",
		"@close.prevent=\"unblockCgroupIPFromTag(ip)\"",
		"@close.prevent=\"unblockCgroupPortFromTag(port)\"",
		"@click=\"blockLsmExecPath\"",
		"@click=\"blockLsmExecName\"",
		"@click=\"blockLsmFileName\"",
		"@close.prevent=\"unblockLsmExecPath(path)\"",
		"@close.prevent=\"unblockLsmExecName(name)\"",
		"@close.prevent=\"unblockLsmFileName(name)\"",
	} {
		if !strings.Contains(component, want) {
			t.Fatalf("%s missing UI sandbox control %q", componentPath, want)
		}
	}

	viewPath := filepath.Join("..", "frontend", "src", "views", "Config.vue")
	viewData, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("read %s: %v", viewPath, err)
	}
	view := string(viewData)
	for _, want := range []string{
		"fetchCgroupSandboxStatus",
		"fetchLsmEnforcerStatus",
		"onMounted(async () =>",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("%s missing Config page sandbox startup hook %q", viewPath, want)
		}
	}
}

func TestPinnedOSEnforcementPolicyIsPreservedOnReuseFailure(t *testing.T) {
	checks := []struct {
		path     string
		required []string
	}{
		{
			path: "cgroup_sandbox_control.go",
			required: []string{
				"Preserve existing pinned policy maps",
				"link.LoadPinnedLink",
				"updatePinnedCgroupSandboxLinks",
				"ensureCgroupSandboxPinnedMapCompatibility",
				"if len(links) >= 4",
			},
		},
		{
			path: "lsm_enforcer_control.go",
			required: []string{
				"Preserve pinned LSM policy maps",
				"link.LoadPinnedLink",
				"updatePinnedLsmEnforcerLinks",
				"if len(links) >= expectedLsmEnforcerLinks",
			},
		},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		source := string(data)
		if strings.Contains(source, "retrying fresh bootstrap") || strings.Contains(source, "fresh bootstrap:") {
			t.Fatalf("%s can fresh-bootstrap after pinned-map reuse failure; this risks deleting explicit OS policy pins", check.path)
		}
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing %q", check.path, want)
			}
		}
	}
}

func TestOSEnforcementAttachFailureCleansPartialPins(t *testing.T) {
	checks := []struct {
		path     string
		required []string
	}{
		{
			path: "cgroup_sandbox_control.go",
			required: []string{
				"closeLinksAndRemovePins(links, pins)",
				"func closeLinksAndRemovePins",
				"os.Remove(pin)",
			},
		},
		{
			path: "lsm_enforcer_control.go",
			required: []string{
				"closeLinksAndRemovePins(links, pins)",
			},
		},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		source := string(data)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing partial-attach cleanup %q", check.path, want)
			}
		}
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
		path     string
		required []string
	}{
		{
			path: "cgroup_sandbox_control.go",
			required: []string{
				"ignoreMissingMapKey(snap.CgroupBlocklist.Delete",
				"ignoreMissingMapKey(snap.IPBlocklist.Delete",
				"ignoreMissingMapKey(snap.IP6Blocklist.Delete",
				"ignoreMissingMapKey(snap.PortBlocklist.Delete",
			},
		},
		{
			path: "lsm_enforcer_control.go",
			required: []string{
				"ignoreMissingMapKey(snap.ExecPathBlocklist.Delete",
				"ignoreMissingMapKey(snap.ExecNameBlocklist.Delete",
				"ignoreMissingMapKey(snap.FileNameBlocklist.Delete",
			},
		},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		source := string(data)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing idempotent unblock wrapper %q", check.path, want)
			}
		}
	}
}

func TestOSEnforcementStatusUsesRuntimeSnapshots(t *testing.T) {
	checks := []struct {
		path     string
		required []string
	}{
		{
			path: "cgroup_sandbox_control.go",
			required: []string{
				"sync.RWMutex",
				"currentCgroupSandboxSnapshot",
				"listBlockedCgroups(snap.CgroupBlocklist)",
				"getCgroupSandboxStats(snap.SandboxStats)",
				"`json:\"checked\"`",
				"total.Checked = total.ConnectChecked",
				"len(cgroupSandbox.Links) >= 4",
			},
		},
		{
			path: "lsm_enforcer_control.go",
			required: []string{
				"sync.RWMutex",
				"currentLsmEnforcerSnapshot",
				"listLsmExecPaths(snap.ExecPathBlocklist)",
				"getLsmEnforcerStats(snap.Stats)",
				"len(lsmEnforcer.Links) >= expectedLsmEnforcerLinks",
			},
		},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		source := string(data)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing %q", check.path, want)
			}
		}
	}
}

func assertProgramSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.ProgramType, attach ebpf.AttachType, section string) {
	t.Helper()
	prog, ok := spec.Programs[name]
	if !ok {
		t.Fatalf("missing program %s", name)
	}
	if prog.Type != typ || prog.AttachType != attach || prog.SectionName != section {
		t.Fatalf("program %s = type %s attach %s section %q, want type %s attach %s section %q",
			name, prog.Type, prog.AttachType, prog.SectionName, typ, attach, section)
	}
	if len(prog.Instructions) == 0 {
		t.Fatalf("program %s has no instructions", name)
	}
}

func assertMapSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.MapType, maxEntries, keySize, valueSize uint32) {
	t.Helper()
	m, ok := spec.Maps[name]
	if !ok {
		t.Fatalf("missing map %s", name)
	}
	if m.Type != typ || m.MaxEntries != maxEntries || m.KeySize != keySize || m.ValueSize != valueSize {
		t.Fatalf("map %s = type %s max_entries %d key_size %d value_size %d, want type %s max_entries %d key_size %d value_size %d",
			name, m.Type, m.MaxEntries, m.KeySize, m.ValueSize, typ, maxEntries, keySize, valueSize)
	}
}

func assertProgramReferencesMap(t *testing.T, spec *ebpf.CollectionSpec, progName, mapName string) {
	t.Helper()
	prog, ok := spec.Programs[progName]
	if !ok {
		t.Fatalf("missing program %s", progName)
	}
	for _, ins := range prog.Instructions {
		if ins.Reference() == mapName {
			return
		}
	}
	t.Fatalf("program %s does not reference map %s", progName, mapName)
}

func stringFromNULBytes(b []byte) string {
	if idx := strings.IndexByte(string(b), 0); idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}
