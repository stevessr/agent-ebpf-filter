package tls

import "sync"

type tlsPIDLinkState struct {
	mu      sync.Mutex
	indexes map[int][]int
}

var tlsPIDLinkStates sync.Map // map[*TLSProbeManager]*tlsPIDLinkState

func pidLinkStateFor(m *TLSProbeManager) *tlsPIDLinkState {
	if m == nil {
		return nil
	}
	if existing, ok := tlsPIDLinkStates.Load(m); ok {
		return existing.(*tlsPIDLinkState)
	}
	created := &tlsPIDLinkState{indexes: make(map[int][]int)}
	actual, _ := tlsPIDLinkStates.LoadOrStore(m, created)
	return actual.(*tlsPIDLinkState)
}

func dropPIDLinkState(m *TLSProbeManager) {
	if m != nil {
		tlsPIDLinkStates.Delete(m)
	}
}

// registerPIDLinkRangeLocked records links appended since start as owned by pid.
// The caller must hold m.mu and call this only after the whole attach operation
// has been accepted (failed operations truncate their unregistered tail).
func (m *TLSProbeManager) registerPIDLinkRangeLocked(pid int, start int) {
	if m == nil || pid <= 0 || start < 0 || start >= len(m.links) {
		return
	}
	state := pidLinkStateFor(m)
	if state == nil {
		return
	}
	state.mu.Lock()
	indexes := state.indexes[pid]
	for index := start; index < len(m.links); index++ {
		if m.links[index] != nil {
			indexes = append(indexes, index)
		}
	}
	state.indexes[pid] = indexes
	state.mu.Unlock()
}

// closePIDLinksLocked detaches all PID-scoped TLS uprobes belonging to pid and
// replaces their slots with nil so manager Close does not close them twice.
// The caller must hold m.mu.
func (m *TLSProbeManager) closePIDLinksLocked(pid int) int {
	if m == nil || pid <= 0 {
		return 0
	}
	state := pidLinkStateFor(m)
	if state == nil {
		return 0
	}
	state.mu.Lock()
	indexes := append([]int(nil), state.indexes[pid]...)
	delete(state.indexes, pid)
	state.mu.Unlock()

	closed := 0
	for _, index := range indexes {
		if index < 0 || index >= len(m.links) {
			continue
		}
		probeLink := m.links[index]
		if probeLink == nil {
			continue
		}
		_ = probeLink.Close()
		m.links[index] = nil
		closed++
	}
	return closed
}

func (m *TLSProbeManager) pruneDeadPIDLinks() int {
	state := pidLinkStateFor(m)
	if state == nil {
		return 0
	}
	state.mu.Lock()
	pids := make([]int, 0, len(state.indexes))
	for pid := range state.indexes {
		pids = append(pids, pid)
	}
	state.mu.Unlock()

	closed := 0
	m.mu.Lock()
	for _, pid := range pids {
		if !processExists(pid) {
			closed += m.closePIDLinksLocked(pid)
		}
	}
	m.mu.Unlock()
	return closed
}

func (m *TLSProbeManager) pidLinkCount(pid int) int {
	state := pidLinkStateFor(m)
	if state == nil || pid <= 0 {
		return 0
	}
	state.mu.Lock()
	count := len(state.indexes[pid])
	state.mu.Unlock()
	return count
}
