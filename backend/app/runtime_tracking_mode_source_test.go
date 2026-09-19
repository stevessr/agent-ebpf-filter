package app

import (
	"os"
	"strings"
	"testing"
)

func sourceBlock(t *testing.T, src, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(src, startMarker)
	if start < 0 {
		t.Fatalf("missing source marker %q", startMarker)
	}
	end := strings.Index(src[start+len(startMarker):], endMarker)
	if end < 0 {
		t.Fatalf("missing end marker %q after %q", endMarker, startMarker)
	}
	return src[start : start+len(startMarker)+end]
}

func assertGateBeforeUserPathProbe(t *testing.T, block, name string) {
	t.Helper()
	gate := strings.Index(block, "TRACKING_MODE_PATH_ANY")
	probe := strings.Index(block, "bpf_probe_read_user_str")
	if gate < 0 {
		t.Fatalf("%s is missing path-rule fast reject", name)
	}
	if probe < 0 {
		t.Fatalf("%s is missing user path probe", name)
	}
	if gate > probe {
		t.Fatalf("%s probes user path before path-rule fast reject", name)
	}
}

func TestPathRuleFastRejectSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	tailBytes, err := os.ReadFile("../ebpf/agent_tracker_tail.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	tail := string(tailBytes)

	if !strings.Contains(common, "return flags ? *flags : TRACKING_MODE_PATH_ANY;") {
		t.Fatal("tracking_mode lookup must fail open to full path matching")
	}

	for _, name := range []string{"execve", "openat", "mkdirat", "unlinkat"} {
		marker := "int tracepoint__syscalls__sys_enter_" + name + "(struct trace_event_raw_sys_enter *ctx) {"
		block := sourceBlock(t, common, marker, "\n}\n")
		assertGateBeforeUserPathProbe(t, block, name)
	}

	for _, macro := range []string{"SYS_PATH0", "SYS_PATH01", "SYS_PATH1", "SYS_PATH13", "SYS_PATH02", "SYS_PATH4"} {
		marker := "#define " + macro + "(name, nr)"
		start := strings.Index(tail, marker)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := tail[start:]
		end := strings.Index(rest, "\n// ── Macro:")
		if end < 0 {
			end = len(rest)
		}
		assertGateBeforeUserPathProbe(t, rest[:end], macro)
	}
}

func assertPIDLookupBeforeCommRead(t *testing.T, block, name string) {
	t.Helper()
	pid := strings.Index(block, "get_pid_tag_id(pid)")
	comm := strings.Index(block, "bpf_get_current_comm")
	if pid < 0 {
		t.Fatalf("%s is missing PID-first lookup", name)
	}
	if comm < 0 {
		t.Fatalf("%s is missing lazy comm fallback", name)
	}
	if pid > comm {
		t.Fatalf("%s reads comm before checking tracked PID", name)
	}
}

func TestPIDFirstEnterTrackingSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	tailBytes, err := os.ReadFile("../ebpf/agent_tracker_tail.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	tail := string(tailBytes)

	for _, helper := range []string{"get_enter_tag_id_nopath", "get_enter_tag_id_pre_path"} {
		marker := "static __always_inline u32 " + helper
		block := sourceBlock(t, common, marker, "\n}\n")
		assertPIDLookupBeforeCommRead(t, block, helper)
	}

	for _, name := range []string{"execve", "openat", "mkdirat", "unlinkat"} {
		marker := "int tracepoint__syscalls__sys_enter_" + name + "(struct trace_event_raw_sys_enter *ctx) {"
		block := sourceBlock(t, common, marker, "\n}\n")
		if !strings.Contains(block, "get_enter_tag_id_pre_path") {
			t.Fatalf("%s bypasses PID-first path matcher", name)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", name)
		}
	}

	for _, macro := range []string{"SYS_PATH0", "SYS_PATH01", "SYS_PATH1", "SYS_PATH13", "SYS_PATH02", "SYS_PATH4"} {
		marker := "#define " + macro + "(name, nr)"
		start := strings.Index(tail, marker)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := tail[start:]
		end := strings.Index(rest, "\n// ── Macro:")
		if end < 0 {
			end = len(rest)
		}
		block := rest[:end]
		if !strings.Contains(block, "get_enter_tag_id_pre_path") {
			t.Fatalf("%s bypasses PID-first path matcher", macro)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", macro)
		}
	}

	for _, macro := range []string{"SYS_NUM", "SYS_NUM2"} {
		marker := "#define " + macro + "(name, nr"
		start := strings.Index(tail, marker)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := tail[start:]
		end := strings.Index(rest, "\n// ")
		if end < 0 {
			end = len(rest)
		}
		block := rest[:end]
		if !strings.Contains(block, "sys_enter_common_nopath") {
			t.Fatalf("%s bypasses lazy no-path matcher", macro)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", macro)
		}
	}
}

func TestPIDFirstNetworkEnterSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	syscallBytes, err := os.ReadFile("../ebpf/agent_tracker_syscalls.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	syscalls := string(syscallBytes)

	assertLazy := func(src, name string) {
		t.Helper()
		marker := "int tracepoint__syscalls__sys_enter_" + name + "(struct trace_event_raw_sys_enter *ctx) {"
		block := sourceBlock(t, src, marker, "\n}\n")
		if !strings.Contains(block, "get_enter_tag_id_nopath") {
			t.Fatalf("%s bypasses PID-first no-path matcher", name)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", name)
		}
	}

	assertLazy(common, "connect")
	for _, name := range []string{
		"socket", "bind", "sendto", "recvfrom", "close",
		"read", "write", "writev", "readv", "sendmsg", "recvmsg",
	} {
		assertLazy(syscalls, name)
	}
}
