package main

// ── Shell session status constants ───────────────────────────────────────────

const (
	shellSessionStatusRunning = "running"
	shellSessionStatusExited  = "exited"
	shellSessionStatusClosed  = "closed"
	shellSessionStatusError   = "error"
)

const shellSessionBacklogLimit = 1 << 20

const (
	shellSessionKindShell   = "shell"
	shellSessionKindTmux    = "tmux"
	shellSessionKindScript  = "script"
	shellSessionKindWrapper = "wrapper"
)