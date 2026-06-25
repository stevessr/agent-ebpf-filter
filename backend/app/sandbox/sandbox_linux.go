//go:build linux

package sandbox

import "agent-ebpf-filter/internal/sandbox"

func ApplySandbox() {
	sandbox.Apply()
}
