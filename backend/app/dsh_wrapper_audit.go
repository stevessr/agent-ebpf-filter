package app

import (
	"strings"
	"unicode"

	"agent-ebpf-filter/internal/dshcli"
)

// formatDshWrapperAudit adds only launcher-level dsh metadata. App arguments,
// prompts, package names, patch paths, and other free-form values stay out of
// ExtraInfo and remain covered only by the argv digest/redaction pipeline.
func formatDshWrapperAudit(comm string, args []string, toolName string) string {
	if toolName == "dsh.exec" {
		return "dsh_mode:exec"
	}
	if !dshcli.IsCommand(comm) {
		return ""
	}
	inv := dshcli.Parse(args)
	parts := []string{"dsh_mode:" + inv.Mode}
	if profile := safeDshAuditToken(inv.Profile); profile != "" {
		parts = append(parts, "dsh_profile:"+profile)
	}
	if operation := safeDshAuditToken(inv.Operation); operation != "" {
		parts = append(parts, "dsh_operation:"+operation)
	}
	return strings.Join(parts, " ")
}

func safeDshAuditToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return value
}

// shouldScheduleWrapperTLSAttach excludes DeepSeek Harness from eBPF TLS
// uprobes. dsh exposes plaintext network activity through its Inspector/CDP
// debugging API, so both the launcher and subprocess-provider execs use that
// userspace source instead of probing Node or descendant TLS libraries.
func shouldScheduleWrapperTLSAttach(comm, toolName string) bool {
	return !dshcli.IsCommand(comm) && toolName != "dsh.exec"
}
