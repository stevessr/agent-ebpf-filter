package app

import (
	"os"

	ps "github.com/shirou/gopsutil/v3/process"
)

// ---- moved from backend/zz_merged_backend.go section cleanup_process.go ----

func killPreviousBackendProcesses() {
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
