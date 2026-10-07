//go:build !linux

package main

import "fmt"

func runInternalBackendIfRequested(args []string) (bool, error) {
	mode, _ := internalBackendArgs(args)
	if !mode {
		return false, nil
	}
	return true, fmt.Errorf("embedded eBPF backend mode is only available on Linux")
}
