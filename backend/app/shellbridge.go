package app

import (
	"os/exec"

	"agent-ebpf-filter/app/shell"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/pb"
)

// ── Type aliases (backward compat with app/*shell* callers) ────────────────

type (
	ShellSessionCreateRequest = shell.CreateRequest
	ShellSessionInfo          = shell.SessionInfo
	ShellSessionInputRequest  = shell.InputRequest
	shellControlMessage       = shell.ControlMessage
)

// ── Global (backward compat with rest of app/) ────────────────────────────

// shellSessions is the global shell manager used by HTTP handlers.
// Deprecated: prefer explicit dependency injection through shell.Deps.
var shellSessions = shell.NewManager()

// ── Deps builder ───────────────────────────────────────────────────────────

// makeShellDeps creates a shell.Deps wired to app-level globals.
func makeShellDeps() shell.Deps {
	return shell.Deps{
		EmitEvent: func(event *pb.Event) {
			if broadcast != nil {
				tls.SendTLSBridge(broadcast, event)
			}
		},
		DropPrivileges: func(cmd *exec.Cmd) {
			dropPrivileges(cmd)
		},
	}
}
