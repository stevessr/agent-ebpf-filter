//go:build !linux

package main

import (
	"fmt"
	"os/exec"
)

func configureBackendLauncher(cmd *exec.Cmd) {}

func configureBackendAskpass(cmd *exec.Cmd, binary string) {}

func runAskpass() error {
	return fmt.Errorf("Renew self-elevation is only supported on Linux")
}

func cancelBackendLauncher(cmd *exec.Cmd) { _ = cmd.Process.Kill() }
