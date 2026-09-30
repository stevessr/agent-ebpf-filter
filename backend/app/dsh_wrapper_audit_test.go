package app

import (
	"strings"
	"testing"
)

func TestFormatDshWrapperAuditUsesOnlyLauncherMetadata(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"headless", "private prompt --dump-config"})
	if got != "dsh_mode:profile dsh_profile:headless" {
		t.Fatalf("audit = %q", got)
	}
	if strings.Contains(got, "private") || strings.Contains(got, "dump-config") {
		t.Fatalf("app argument leaked into dsh audit metadata: %q", got)
	}
}

func TestFormatDshWrapperAuditRedactsPluginPayload(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"plugin", "--profile", "tui", "add", "@scope/private-package"})
	if got != "dsh_mode:plugin dsh_profile:tui dsh_operation:add" {
		t.Fatalf("audit = %q", got)
	}
	if strings.Contains(got, "@scope") || strings.Contains(got, "private-package") {
		t.Fatalf("package name leaked into dsh audit metadata: %q", got)
	}
}

func TestFormatDshWrapperAuditRejectsUnsafeProfileToken(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"profile with spaces", "task"})
	if got != "dsh_mode:profile" {
		t.Fatalf("audit = %q", got)
	}
}

func TestDshWrapperTLSAttachDefersUntilExec(t *testing.T) {
	if got := wrapperTLSAttachBinaryPath("dsh", "/usr/local/bin/dsh"); got != "" {
		t.Fatalf("dsh TLS binary path = %q, want deferred discovery", got)
	}
	if got := wrapperTLSAttachBinaryPath("git", "/usr/bin/git"); got != "/usr/bin/git" {
		t.Fatalf("non-dsh TLS binary path = %q", got)
	}
}
