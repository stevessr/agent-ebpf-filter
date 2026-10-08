//go:build linux

package main

import "agent-ebpf-filter/embedded"

func runInternalBackendIfRequested(args []string) (bool, error) {
	mode, backendArgs := internalBackendArgs(args)
	if !mode {
		return false, nil
	}
	return true, embedded.Run(backendArgs)
}
