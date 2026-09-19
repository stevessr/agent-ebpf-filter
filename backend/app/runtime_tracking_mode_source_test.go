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
