//go:build !windows

package main

import "context"

// The Linux backend and Native IPC remain the default outside Windows.
func (a *renewApp) runLocalMonitor(_ context.Context) {}
