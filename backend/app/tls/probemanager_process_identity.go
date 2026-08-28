package tls

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type tlsPIDIdentityState struct {
	mu         sync.Mutex
	startTimes map[int]uint64
	reuseResets atomic.Uint64
}

var tlsPIDIdentityStates sync.Map // map[*TLSProbeManager]*tlsPIDIdentityState

func pidIdentityStateFor(m *TLSProbeManager) *tlsPIDIdentityState {
	if m == nil {
		return nil
	}
	if existing, ok := tlsPIDIdentityStates.Load(m); ok {
		return existing.(*tlsPIDIdentityState)
	}
	created := &tlsPIDIdentityState{startTimes: make(map[int]uint64)}
	actual, _ := tlsPIDIdentityStates.LoadOrStore(m, created)
	return actual.(*tlsPIDIdentityState)
}

func dropPIDIdentityState(m *TLSProbeManager) {
	if m != nil {
		tlsPIDIdentityStates.Delete(m)
	}
}

// parseProcStartTime extracts field 22 (starttime) from /proc/<pid>/stat.
// The process name in field 2 is parenthesized and may contain spaces or ')',
// so find the final ')' first instead of splitting the whole line.
func parseProcStartTime(stat []byte) (uint64, bool) {
	text := strings.TrimSpace(string(stat))
	closeParen := strings.LastIndexByte(text, ')')
	if closeParen < 0 || closeParen+1 >= len(text) {
		return 0, false
	}
	fields := strings.Fields(text[closeParen+1:])
	// fields[0] is field 3 (state), therefore field 22 is index 19.
	if len(fields) <= 19 {
		return 0, false
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	return startTime, err == nil && startTime > 0
}

func readProcStartTime(pid int) (uint64, bool) {
	if pid <= 0 {
		return 0, false
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	return parseProcStartTime(stat)
}

func (m *TLSProbeManager) rememberAutoAttachProcess(pid int) {
	if m == nil || pid <= 0 {
		return
	}
	startTime, ok := readProcStartTime(pid)
	if !ok {
		return
	}
	state := pidIdentityStateFor(m)
	if state == nil {
		return
	}
	state.mu.Lock()
	state.startTimes[pid] = startTime
	state.mu.Unlock()
}

// autoAttachProcessCurrent distinguishes a still-running process from a new
// process that reused the same numeric PID. PIDs without a recorded identity
// retain the historical process-exists behavior for manual attachments.
func (m *TLSProbeManager) autoAttachProcessCurrent(pid int) bool {
	if pid <= 0 {
		return false
	}
	state := pidIdentityStateFor(m)
	if state == nil {
		return processExists(pid)
	}
	state.mu.Lock()
	recorded, tracked := state.startTimes[pid]
	state.mu.Unlock()
	if !tracked {
		return processExists(pid)
	}
	current, ok := readProcStartTime(pid)
	return ok && current == recorded
}

// pruneProcessIdentities drops identities for exited or PID-reused processes.
// Reuse is counted separately because it explains otherwise intermittent
// "already attached" false positives in fast CLI process churn.
func (m *TLSProbeManager) pruneProcessIdentities() {
	state := pidIdentityStateFor(m)
	if state == nil {
		return
	}
	state.mu.Lock()
	for pid, recorded := range state.startTimes {
		current, ok := readProcStartTime(pid)
		if ok && current == recorded {
			continue
		}
		if ok && current != recorded {
			state.reuseResets.Add(1)
		}
		delete(state.startTimes, pid)
	}
	state.mu.Unlock()
}

func (m *TLSProbeManager) pidReuseResetCount() uint64 {
	state := pidIdentityStateFor(m)
	if state == nil {
		return 0
	}
	return state.reuseResets.Load()
}
