package tls

import (
	"fmt"
	"os"
)

// executablePathAllowed checks the pre-attach executable allowlist. Both the
// stable display path and an attach path (for example /proc/<pid>/exe for a
// deleted binary) may be supplied; matching either is sufficient.
func (m *TLSProbeManager) executablePathAllowed(paths ...string) bool {
	if m == nil || m.rules == nil {
		return false
	}
	for _, path := range paths {
		if m.rules.AllowsExecutablePath(path) {
			return true
		}
	}
	return false
}

// autoAttachedPIDs snapshots only processes admitted by auto-discovery. Manual
// PID/path attachments remain explicit operator actions and are not torn down
// merely because the automatic path allowlist changes.
func (m *TLSProbeManager) autoAttachedPIDs() []int {
	state := pidIdentityStateFor(m)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	pids := make([]int, 0, len(state.startTimes))
	for pid := range state.startTimes {
		pids = append(pids, pid)
	}
	state.mu.Unlock()
	return pids
}

func (m *TLSProbeManager) clearPIDAttachBookkeepingLocked(pid int) {
	delete(m.attachedExec, pid)
	for key := range m.attachedGo {
		if attachedPID, ok := pidFromGoAttachKey(key); ok && attachedPID == pid {
			delete(m.attachedGo, key)
		}
	}
	for key := range m.attachedStatic {
		if attachedPID, ok := pidFromStaticAttachKey(key); ok && attachedPID == pid {
			delete(m.attachedStatic, key)
		}
	}
}

// reconcileExecutablePathScope detaches auto-discovered PID-scoped probes that
// no longer match an enabled executable path rule. This makes path narrowing
// converge on the next discovery tick instead of leaving stale plaintext hooks.
func (m *TLSProbeManager) reconcileExecutablePathScope() int {
	if m == nil {
		return 0
	}
	closed := 0
	for _, pid := range m.autoAttachedPIDs() {
		exeLink := fmt.Sprintf("/proc/%d/exe", pid)
		target, err := os.Readlink(exeLink)
		if err != nil {
			continue
		}
		_, displayPath, _, ok := normalizeProcExecutableTarget(exeLink, target, true)
		if !ok || m.executablePathAllowed(displayPath, exeLink) {
			continue
		}

		m.mu.Lock()
		m.clearPIDAttachBookkeepingLocked(pid)
		closed += m.closePIDLinksLocked(pid)
		m.mu.Unlock()
	}
	if closed > 0 {
		if state := discoveryRuntimeStateFor(m); state != nil {
			state.detachedLinks.Add(uint64(closed))
		}
	}
	return closed
}
