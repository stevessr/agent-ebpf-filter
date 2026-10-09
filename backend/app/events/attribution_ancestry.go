package events

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
)

const maxAgentAncestorHops = 12

// procParentPID reads just the parent PID from /proc/<pid>/stat. The comm
// field is parenthesized and may contain spaces or ')', so splitting the
// entire stat record on whitespace would read the wrong field.
func procParentPID(pid uint32) (uint32, bool) {
	if pid <= 1 {
		return 0, false
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	end := bytes.LastIndexByte(raw, ')')
	if end < 0 {
		return 0, false
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) < 2 {
		return 0, false
	}
	parent, err := strconv.ParseUint(fields[1], 10, 32)
	if err != nil || parent < 1 || uint32(parent) == pid {
		return 0, false
	}
	return uint32(parent), true
}

// resolveAncestorAgentContext follows a bounded, cycle-safe live ancestry
// chain to a registered Agent process. This is only a fallback: direct event
// PID / PPID evidence and explicit tool/hook attribution take precedence.
// Never equate the executable name (bash, fish, node, python, pwsh, etc.)
// with an Agent, and never assign a context based on a common cgroup alone.
func resolveAncestorAgentContext(pid, ppid uint32, store *ProcessContextStore, parentOf func(uint32) (uint32, bool)) (ProcessContext, bool) {
	if store == nil || pid == 0 || parentOf == nil {
		return ProcessContext{}, false
	}
	visited := map[uint32]struct{}{pid: {}}
	parent := ppid
	if parent == 0 {
		var ok bool
		parent, ok = parentOf(pid)
		if !ok {
			return ProcessContext{}, false
		}
	}
	for hops := 0; hops < maxAgentAncestorHops && parent > 1; hops++ {
		if _, seen := visited[parent]; seen {
			break
		}
		visited[parent] = struct{}{}
		if ctx, ok := store.Get(parent); ok {
			return ctx, true
		}
		next, ok := parentOf(parent)
		if !ok || next == parent {
			break
		}
		parent = next
	}
	return ProcessContext{}, false
}

// Bounded procfs fallback is reserved for file and process activity. Most
// unrelated network/metric events must not trigger repeated procfs walks.
func shouldResolveAgentAncestor(event *pb.Event) bool {
	if event == nil || event.Pid <= 1 {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(event.Type))
	if strings.HasPrefix(kind, "process_") || strings.HasPrefix(kind, "file_") {
		return true
	}
	if event.Path != "" || event.ExtraPath != "" {
		return true
	}
	switch kind {
	case "exec", "execve", "fork", "clone", "write", "writev",
		"pwrite", "pwrite64", "rename", "renameat", "renameat2",
		"unlink", "unlinkat", "truncate", "ftruncate",
		"mkdir", "mkdirat", "rmdir", "chmod", "chown", "creat",
		"open", "openat", "openat2":
		return true
	}
	return false
}

 
// propagateAgentContextOnFork uses the kernel's explicit child PID evidence,
// not a process-name guess. Carrying the context at fork time covers scripts
// which exit before a later procfs ancestry fallback could inspect them.
func propagateAgentContextOnFork(event *pb.Event, parent ProcessContext, store *ProcessContextStore) {
	if event == nil || store == nil || event.Type != "process_fork" {
		return
	}
	child := platform.ParseUintField(event.ExtraInfo, "child_pid")
	if child <= 1 || child == event.Pid {
		return
	}
	if _, registered := store.Get(child); !registered {
		store.Set(child, parent)
	}
}

// A comm-tracked Agent can emit a sched_process_fork before an adapter has
// registered its PID. Only the explicitly Agent-tagged, known CLI entrypoints
// may seed a root context; ordinary tracked runtimes and tools must not.
func shouldSeedAgentRootAtFork(event *pb.Event) bool {
	if event == nil || event.Type != "process_fork" || event.Pid == 0 ||
		!strings.EqualFold(strings.TrimSpace(event.Tag), "Agent CLI") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(event.Comm)) {
	case "codex", "claude", "gemini", "dsh", "pi", "omp",
		"kiro", "kiro-cli", "cursor", "cursor-agent",
		"opencode", "copilot", "auggie", "augment", "agy",
		"antigravity", "zcode", "zcode.appimage", "mcode", "minimax-code":
		return true
	}
	return false
}
