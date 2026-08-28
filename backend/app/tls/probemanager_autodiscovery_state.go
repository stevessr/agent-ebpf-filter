package tls

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	tlsAutoAttachRetryBase = 15 * time.Second
	tlsAutoAttachRetryMax  = 5 * time.Minute
)

type tlsAttachRetryState struct {
	PID         int
	Path        string
	Kind        string
	Failures    int
	LastAttempt time.Time
	NextRetry   time.Time
	LastError   string
}

type tlsDiscoveryRuntimeState struct {
	mu sync.Mutex

	retries       map[string]tlsAttachRetryState
	lastCaptureNS map[int]int64
	lastError     string
	lastErrorAt   time.Time

	attempts      atomic.Uint64
	successes     atomic.Uint64
	failures      atomic.Uint64
	backoffSkips  atomic.Uint64
	detachedLinks atomic.Uint64
}

type TLSAutoDiscoveryStatus struct {
	Attempts          uint64 `json:"attempts"`
	Successes         uint64 `json:"successes"`
	Failures          uint64 `json:"failures"`
	BackoffSkips      uint64 `json:"backoffSkips"`
	DetachedLinks     uint64 `json:"detachedLinks"`
	ActiveBackoffs    int    `json:"activeBackoffs"`
	ObservedPIDs      int    `json:"observedPids"`
	LastError         string `json:"lastError,omitempty"`
	LastErrorAtUnixMs int64  `json:"lastErrorAtUnixMs,omitempty"`
}

var tlsDiscoveryRuntimeStates sync.Map // map[*TLSProbeManager]*tlsDiscoveryRuntimeState

func discoveryRuntimeStateFor(m *TLSProbeManager) *tlsDiscoveryRuntimeState {
	if m == nil {
		return nil
	}
	if existing, ok := tlsDiscoveryRuntimeStates.Load(m); ok {
		return existing.(*tlsDiscoveryRuntimeState)
	}
	created := &tlsDiscoveryRuntimeState{
		retries:       make(map[string]tlsAttachRetryState),
		lastCaptureNS: make(map[int]int64),
	}
	actual, _ := tlsDiscoveryRuntimeStates.LoadOrStore(m, created)
	return actual.(*tlsDiscoveryRuntimeState)
}

func dropDiscoveryRuntimeState(m *TLSProbeManager) {
	if m != nil {
		tlsDiscoveryRuntimeStates.Delete(m)
		dropPIDLinkState(m)
	}
}

func tlsAutoAttachKey(kind string, pid int, path string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", kind, pid, path)
}

func tlsRetryDelay(failures int) time.Duration {
	if failures <= 1 {
		return tlsAutoAttachRetryBase
	}
	shift := failures - 1
	if shift > 8 {
		shift = 8
	}
	delay := tlsAutoAttachRetryBase * time.Duration(1<<shift)
	if delay > tlsAutoAttachRetryMax {
		return tlsAutoAttachRetryMax
	}
	return delay
}

func (m *TLSProbeManager) autoAttachAllowed(kind string, pid int, path string, now time.Time) bool {
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return false
	}
	key := tlsAutoAttachKey(kind, pid, path)
	state.mu.Lock()
	retry, exists := state.retries[key]
	allowed := !exists || !now.Before(retry.NextRetry)
	state.mu.Unlock()
	if !allowed {
		state.backoffSkips.Add(1)
	}
	return allowed
}

func (m *TLSProbeManager) recordAutoAttachAttempt() {
	if state := discoveryRuntimeStateFor(m); state != nil {
		state.attempts.Add(1)
	}
}

func (m *TLSProbeManager) recordAutoAttachSuccess(kind string, pid int, path string) {
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return
	}
	state.successes.Add(1)
	state.mu.Lock()
	delete(state.retries, tlsAutoAttachKey(kind, pid, path))
	state.mu.Unlock()
}

func (m *TLSProbeManager) recordAutoAttachFailure(kind string, pid int, path string, err error, now time.Time) {
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return
	}
	state.failures.Add(1)
	key := tlsAutoAttachKey(kind, pid, path)
	state.mu.Lock()
	retry := state.retries[key]
	retry.PID = pid
	retry.Path = path
	retry.Kind = kind
	retry.Failures++
	retry.LastAttempt = now
	retry.NextRetry = now.Add(tlsRetryDelay(retry.Failures))
	if err != nil {
		retry.LastError = err.Error()
		state.lastError = retry.LastError
		state.lastErrorAt = now
	}
	state.retries[key] = retry
	state.mu.Unlock()
}

func (m *TLSProbeManager) markTLSCaptureObserved(pid int, timestampNS int64) {
	if pid <= 0 {
		return
	}
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return
	}
	if timestampNS <= 0 {
		timestampNS = time.Now().UnixNano()
	}
	state.mu.Lock()
	state.lastCaptureNS[pid] = timestampNS
	state.mu.Unlock()
}

func (m *TLSProbeManager) tlsCaptureObservation(pid int) (int64, bool) {
	state := discoveryRuntimeStateFor(m)
	if state == nil || pid <= 0 {
		return 0, false
	}
	state.mu.Lock()
	lastNS, ok := state.lastCaptureNS[pid]
	state.mu.Unlock()
	return lastNS, ok && lastNS > 0
}

func (m *TLSProbeManager) pruneAutoDiscoveryState() {
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return
	}
	if detached := m.pruneDeadPIDLinks(); detached > 0 {
		state.detachedLinks.Add(uint64(detached))
	}
	state.mu.Lock()
	for key, retry := range state.retries {
		if retry.PID > 0 && !processExists(retry.PID) {
			delete(state.retries, key)
		}
	}
	for pid := range state.lastCaptureNS {
		if !processExists(pid) {
			delete(state.lastCaptureNS, pid)
		}
	}
	state.mu.Unlock()
}

func (m *TLSProbeManager) AutoDiscoveryStatus() TLSAutoDiscoveryStatus {
	state := discoveryRuntimeStateFor(m)
	if state == nil {
		return TLSAutoDiscoveryStatus{}
	}
	now := time.Now()
	status := TLSAutoDiscoveryStatus{
		Attempts:      state.attempts.Load(),
		Successes:     state.successes.Load(),
		Failures:      state.failures.Load(),
		BackoffSkips:  state.backoffSkips.Load(),
		DetachedLinks: state.detachedLinks.Load(),
	}
	state.mu.Lock()
	for _, retry := range state.retries {
		if now.Before(retry.NextRetry) {
			status.ActiveBackoffs++
		}
	}
	status.ObservedPIDs = len(state.lastCaptureNS)
	status.LastError = state.lastError
	if !state.lastErrorAt.IsZero() {
		status.LastErrorAtUnixMs = state.lastErrorAt.UnixMilli()
	}
	state.mu.Unlock()
	return status
}
