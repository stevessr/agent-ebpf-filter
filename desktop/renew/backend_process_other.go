//go:build !linux

package main

import "os/exec"

func configureBackendLauncher(cmd *exec.Cmd) {}
func cancelBackendLauncher(cmd *exec.Cmd)    { _ = cmd.Process.Kill() }
