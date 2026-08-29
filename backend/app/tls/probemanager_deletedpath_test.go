package tls

import "testing"

func TestNormalizeProcExecutableTargetUsesLivePathNormally(t *testing.T) {
	attach, display, deleted, ok := normalizeProcExecutableTarget(
		"/proc/42/exe",
		"/usr/bin/codex",
		true,
	)
	if !ok || deleted || attach != "/usr/bin/codex" || display != "/usr/bin/codex" {
		t.Fatalf("got attach=%q display=%q deleted=%v ok=%v", attach, display, deleted, ok)
	}
}

func TestNormalizeProcExecutableTargetUsesProcLinkForDeletedBinary(t *testing.T) {
	attach, display, deleted, ok := normalizeProcExecutableTarget(
		"/proc/42/exe",
		"/opt/claude/claude (deleted)",
		true,
	)
	if !ok || !deleted {
		t.Fatalf("deleted executable should remain attachable: deleted=%v ok=%v", deleted, ok)
	}
	if attach != "/proc/42/exe" {
		t.Fatalf("attach = %q, want /proc/42/exe", attach)
	}
	if display != "/opt/claude/claude" {
		t.Fatalf("display = %q, want cleaned original path", display)
	}
}

func TestNormalizeProcExecutableTargetRejectsUnusableDeletedBinary(t *testing.T) {
	attach, display, deleted, ok := normalizeProcExecutableTarget(
		"/proc/42/exe",
		"/opt/claude/claude (deleted)",
		false,
	)
	if ok || !deleted || attach != "" || display != "/opt/claude/claude" {
		t.Fatalf("got attach=%q display=%q deleted=%v ok=%v", attach, display, deleted, ok)
	}
}
