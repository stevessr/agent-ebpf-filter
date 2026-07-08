package shell

const (
	StatusRunning = "running"
	StatusExited  = "exited"
	StatusClosed  = "closed"
	StatusError   = "error"
)

const BacklogLimit = 1 << 20

const (
	KindShell   = "shell"
	KindTmux    = "tmux"
	KindScript  = "script"
	KindWrapper = "wrapper"
)