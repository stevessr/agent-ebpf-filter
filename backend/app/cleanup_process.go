package app

import "agent-ebpf-filter/app/runtime"

// killPreviousBackendProcesses delegates to the runtime subpackage.
func killPreviousBackendProcesses() {
	runtime.KillPreviousBackendProcesses()
}
