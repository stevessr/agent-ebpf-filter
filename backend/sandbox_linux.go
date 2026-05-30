//go:build linux

package main

import "agent-ebpf-filter/internal/sandbox"

func ApplySandbox() {
	sandbox.Apply()
}
