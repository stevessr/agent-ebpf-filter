package tls

import (
	"sync"
	"time"
)

const tlsMaxPendingFragments = 4096

// Values above this threshold are unmistakably Unix-nanosecond timestamps for
// any realistic machine uptime. Live BPF samples use bpf_ktime_get_ns() and
// therefore fall below it; offline/replay tests may carry wall-clock Unix ns.
const tlsPlausibleUnixNSThreshold = uint64(946684800) * uint64(time.Second) // 2000-01-01

type tlsFragmentAssemblerKey struct {
	PID          uint32
	TGID         uint32
	ConnectionID uint64
	TimestampNS  uint64
	Direction    uint8
	LibType      uint8
	Function     uint8
}

type pendingTLSFragment struct {
	firstSeen   time.Time
	fragCount   uint16
	totalLen    uint32
	originalLen uint32
	comm        string
	flags       uint8
	fragMap     map[uint16][]byte
}

type FragmentAssembler struct {
	mu      sync.Mutex
	pending map[tlsFragmentAssemblerKey]*pendingTLSFragment
	timeout time.Duration
	dropped int
}

func NewFragmentAssembler(timeout time.Duration) *FragmentAssembler {
	return &FragmentAssembler{
		pending: make(map[tlsFragmentAssemblerKey]*pendingTLSFragment),
		timeout: timeout,
	}
}

func fragmentAssemblerKey(f tlsFragment) tlsFragmentAssemblerKey {
	return tlsFragmentAssemblerKey{
		PID:          f.PID,
		TGID:         f.TGID,
		ConnectionID: f.ConnectionID,
		TimestampNS:  f.TimestampNS,
		Direction:    f.Direction,
		LibType:      f.LibType,
		Function:     f.Function,
	}
}

func fragmentFirstSeen(timestampNS uint64, arrival time.Time) time.Time {
	// Production eBPF uses monotonic nanoseconds since boot. Timeout those by
	// userspace arrival time so wall-clock CleanupExpired cannot purge them as
	// if they originated near the Unix epoch.
	if timestampNS < tlsPlausibleUnixNSThreshold || timestampNS > uint64(^uint64(0)>>1) {
		return arrival
	}

	// Replay/test records sometimes carry Unix ns. Preserve that behavior when
	// the value is plausible, but reject future timestamps and fall back to the
	// local arrival clock.
	captured := time.Unix(0, int64(timestampNS))
	if captured.After(arrival.Add(time.Minute)) {
		return arrival
	}
	return captured
}

func (a *FragmentAssembler) evictOldestPendingLocked() {
	var oldestKey tlsFragmentAssemblerKey
	var oldest *pendingTLSFragment
	for key, pending := range a.pending {
		if oldest == nil || pending.firstSeen.Before(oldest.firstSeen) {
			oldestKey = key
			oldest = pending
		}
	}
	if oldest != nil {
		delete(a.pending, oldestKey)
		a.dropped++
	}
}

func sanitizeTLSComm(comm [16]byte) string {
	return sanitizeUTF8(comm[:])
}

func (a *FragmentAssembler) Add(fragment tlsFragment) (*CompletedTLSFragment, bool) {
	if fragment.FragCount == 0 || fragment.FragIndex >= fragment.FragCount || fragment.TotalLen == 0 {
		a.mu.Lock()
		a.dropped++
		a.mu.Unlock()
		return nil, false
	}
	if fragment.FragCount > tlsMaxFragments {
		a.mu.Lock()
		a.dropped++
		a.mu.Unlock()
		return nil, false
	}
	if fragment.DataLen == 0 || fragment.DataLen > tlsFragmentSize {
		a.mu.Lock()
		a.dropped++
		a.mu.Unlock()
		return nil, false
	}

	arrival := time.Now()
	firstSeen := fragmentFirstSeen(fragment.TimestampNS, arrival)
	key := fragmentAssemblerKey(fragment)

	a.mu.Lock()
	defer a.mu.Unlock()

	pending := a.pending[key]
	if pending == nil {
		if len(a.pending) >= tlsMaxPendingFragments {
			a.evictOldestPendingLocked()
		}
		pending = &pendingTLSFragment{
			firstSeen:   firstSeen,
			fragCount:   fragment.FragCount,
			totalLen:    fragment.TotalLen,
			originalLen: fragment.OriginalLen,
			comm:        sanitizeTLSComm(fragment.Comm),
			flags:       fragment.Flags,
			fragMap:     make(map[uint16][]byte, fragment.FragCount),
		}
		a.pending[key] = pending
	} else if pending.fragCount != fragment.FragCount ||
		pending.totalLen != fragment.TotalLen ||
		pending.originalLen != fragment.OriginalLen ||
		pending.flags != fragment.Flags {
		delete(a.pending, key)
		a.dropped++
		return nil, false
	}
	if _, exists := pending.fragMap[fragment.FragIndex]; exists {
		a.dropped++
		return nil, false
	}

	chunk := make([]byte, int(fragment.DataLen))
	copy(chunk, fragment.Data[:fragment.DataLen])
	pending.fragMap[fragment.FragIndex] = chunk
	if uint16(len(pending.fragMap)) != pending.fragCount {
		return nil, false
	}

	payload := make([]byte, 0, pending.totalLen)
	for i := uint16(0); i < pending.fragCount; i++ {
		chunk, ok := pending.fragMap[i]
		if !ok {
			delete(a.pending, key)
			a.dropped++
			return nil, false
		}
		payload = append(payload, chunk...)
	}
	delete(a.pending, key)

	if uint32(len(payload)) != pending.totalLen {
		a.dropped++
		return nil, false
	}

	return &CompletedTLSFragment{
		TimestampNS:  fragment.TimestampNS,
		ConnectionID: fragment.ConnectionID,
		PID:          fragment.PID,
		TGID:         fragment.TGID,
		DataLen:      fragment.DataLen,
		TotalLen:     pending.totalLen,
		OriginalLen:  pending.originalLen,
		FragCount:    pending.fragCount,
		LibType:      fragment.LibType,
		Direction:    fragment.Direction,
		Flags:        pending.flags,
		Function:     fragment.Function,
		Comm:         pending.comm,
		Payload:      payload,
	}, true
}

func (a *FragmentAssembler) CleanupExpired(now time.Time) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	removed := 0
	for key, pending := range a.pending {
		if now.Sub(pending.firstSeen) > a.timeout {
			delete(a.pending, key)
			removed++
			a.dropped++
		}
	}
	return removed
}

func (a *FragmentAssembler) Pending() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.pending)
}

func (a *FragmentAssembler) RemoveByTGID(tgid uint32) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	removed := 0
	for key := range a.pending {
		if key.TGID == tgid {
			delete(a.pending, key)
			removed++
		}
	}
	return removed
}

func (a *FragmentAssembler) RemoveByPID(pid uint32) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	removed := 0
	for key := range a.pending {
		if key.PID == pid {
			delete(a.pending, key)
			removed++
		}
	}
	return removed
}

func (a *FragmentAssembler) Dropped() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dropped
}
