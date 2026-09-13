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
	payload     []byte
	received    uint16
	receivedMap uint32
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

func expectedTLSFragmentBounds(fragment tlsFragment) (int, int, bool) {
	if fragment.FragCount == 0 || fragment.FragIndex >= fragment.FragCount || fragment.TotalLen == 0 {
		return 0, 0, false
	}
	if fragment.FragCount > tlsMaxFragments || fragment.DataLen == 0 || fragment.DataLen > tlsFragmentSize {
		return 0, 0, false
	}
	if fragment.TotalLen > uint32(tlsFragmentSize*tlsMaxFragments) {
		return 0, 0, false
	}

	start := int(fragment.FragIndex) * tlsFragmentSize
	end := start + int(fragment.DataLen)
	if start < 0 || end < start || end > int(fragment.TotalLen) {
		return 0, 0, false
	}
	// Every non-final fragment emitted by the BPF program is a full chunk. This
	// catches malformed or cross-record corruption early without penalising the
	// shorter final fragment.
	if fragment.FragIndex+1 < fragment.FragCount && fragment.DataLen != tlsFragmentSize {
		return 0, 0, false
	}
	if fragment.FragIndex+1 == fragment.FragCount && end != int(fragment.TotalLen) {
		return 0, 0, false
	}
	return start, end, true
}

func (a *FragmentAssembler) Add(fragment tlsFragment) (*CompletedTLSFragment, bool) {
	start, end, valid := expectedTLSFragmentBounds(fragment)
	if !valid {
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
			payload:     make([]byte, int(fragment.TotalLen)),
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

	bit := uint32(1) << fragment.FragIndex
	if pending.receivedMap&bit != 0 {
		a.dropped++
		return nil, false
	}
	copy(pending.payload[start:end], fragment.Data[:fragment.DataLen])
	pending.receivedMap |= bit
	pending.received++
	if pending.received != pending.fragCount {
		return nil, false
	}

	// FragCount is bounded to 18, so a 32-bit bitset can verify that every
	// fragment arrived without allocating a map or scanning payload slices.
	expectedMask := uint32(1)<<pending.fragCount - 1
	if pending.receivedMap != expectedMask {
		delete(a.pending, key)
		a.dropped++
		return nil, false
	}

	payload := pending.payload
	delete(a.pending, key)
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
