package app

import (
	"strings"
	"testing"
)

func TestFormatDshWrapperAuditUsesOnlyLauncherMetadata(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"headless", "private prompt --dump-config"}, "")
	if got != "dsh_mode:profile dsh_profile:headless" {
		t.Fatalf("audit = %q", got)
	}
	if strings.Contains(got, "private") || strings.Contains(got, "dump-config") {
		t.Fatalf("app argument leaked into dsh audit metadata: %q", got)
	}
}

func TestFormatDshWrapperAuditRedactsPluginPayload(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"plugin", "--profile", "tui", "add", "@scope/private-package"}, "")
	if got != "dsh_mode:plugin dsh_profile:tui dsh_operation:add" {
		t.Fatalf("audit = %q", got)
	}
	if strings.Contains(got, "@scope") || strings.Contains(got, "private-package") {
		t.Fatalf("package name leaked into dsh audit metadata: %q", got)
	}
}

func TestFormatDshWrapperAuditRejectsUnsafeProfileToken(t *testing.T) {
	got := formatDshWrapperAudit("dsh", []string{"profile with spaces", "task"}, "")
	if got != "dsh_mode:profile" {
		t.Fatalf("audit = %q", got)
	}
}

func TestDshWrapperTLSAttachUsesInspectorInsteadOfUprobe(t *testing.T) {
	if shouldScheduleWrapperTLSAttach("dsh", "") {
		t.Fatal("dsh launcher must not schedule eBPF TLS attach")
	}
	if shouldScheduleWrapperTLSAttach("git", "dsh.exec") {
		t.Fatal("dsh subprocess-provider exec must not schedule eBPF TLS attach")
	}
	if !shouldScheduleWrapperTLSAttach("git", "") {
		t.Fatal("ordinary wrapped commands should retain TLS attach behavior")
	}
}

func TestFormatDshWrapperAuditRecognizesProviderExec(t *testing.T) {
	if got := formatDshWrapperAudit("git", []string{"status"}, "dsh.exec"); got != "dsh_mode:exec" {
		t.Fatalf("audit = %q", got)
	}
}
