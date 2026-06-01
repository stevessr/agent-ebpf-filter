//go:build !linux

package app

import "agent-ebpf-filter/internal/sandbox"

func ApplySandbox() {
	sandbox.Apply()
}
