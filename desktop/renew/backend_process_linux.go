package main

import (
	"os/exec"
	"syscall"
)

func configureBackendLauncher(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func cancelBackendLauncher(cmd *exec.Cmd) {
	// Cancel the owned launcher and any authorization helper still running as
	// the calling user. Connected root backends are stopped by socket EOF.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}
