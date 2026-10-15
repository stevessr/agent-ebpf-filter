//go:build !windows

package main

import "context"

func startLocalWindowsMonitoring(_ context.Context, _ *renewApp) bool { return false }
