package runtime

import (
	"os"

	ps "github.com/shirou/gopsutil/v3/process"
)

// KillPreviousBackendProcesses kills any stale backend processes (agent-ebpf-filter or main) owned by this user.
func KillPreviousBackendProcesses() {
	procsList, _ := ps.Processes()
	curr := int32(os.Getpid())
	for _, p := range procsList {
		if p.Pid != curr {
			if n, _ := p.Name(); n == "agent-ebpf-filter" || n == "main" {
				_ = p.Kill()
			}
		}
	}
}
