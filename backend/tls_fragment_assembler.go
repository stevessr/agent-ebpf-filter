package main

import (
	"sync"
	"time"
)

const tlsMaxPendingFragments = 4096

type tlsFragmentAssemblerKey struct {
	PID         uint32
	TGID        uint32
	TimestampNS uint64
	Direction   uint8
}

type pendingTLSFragment struct {
	firstSeen time.Time
	fragCount uint16
	totalLen  uint32
	comm      string
	fragMap   map[uint16]tlsFragment
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
		PID:         f.PID,
		TGID:        f.TGID,
		TimestampNS: f.TimestampNS,
		Direction:   f.Direction,
	}
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

func (a *FragmentAssembler) Add(fragment tlsFragment) (*completedTLSFragment, bool) {
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
	if fragment.DataLen > tlsFragmentSize {
		a.mu.Lock()
		a.dropped++
		a.mu.Unlock()
		return nil, false
	}

	now := time.Unix(0, int64(fragment.TimestampNS))
	if now.IsZero() {
		now = time.Now()
	}
	key := fragmentAssemblerKey(fragment)

	a.mu.Lock()
	defer a.mu.Unlock()

	pending := a.pending[key]
	if pending == nil {
		if len(a.pending) >= tlsMaxPendingFragments {
			a.evictOldestPendingLocked()
		}
		pending = &pendingTLSFragment{
			firstSeen: now,
			fragCount: fragment.FragCount,
			totalLen:  fragment.TotalLen,
			comm:      sanitizeTLSComm(fragment.Comm),
			fragMap:   make(map[uint16]tlsFragment, fragment.FragCount),
		}
		a.pending[key] = pending
	} else if pending.fragCount != fragment.FragCount || pending.totalLen != fragment.TotalLen {
		delete(a.pending, key)
		a.dropped++
		return nil, false
	}
	if _, exists := pending.fragMap[fragment.FragIndex]; exists {
		a.dropped++
		return nil, false
	}

	fragCopy := fragment
	pending.fragMap[fragment.FragIndex] = fragCopy
	if uint16(len(pending.fragMap)) != pending.fragCount {
		return nil, false
	}

	payload := make([]byte, 0, pending.totalLen)
	for i := uint16(0); i < pending.fragCount; i++ {
		frag, ok := pending.fragMap[i]
		if !ok {
			delete(a.pending, key)
			a.dropped++
			return nil, false
		}
		payload = append(payload, frag.Data[:frag.DataLen]...)
	}
	delete(a.pending, key)

	return &completedTLSFragment{
		TimestampNS: fragment.TimestampNS,
		PID:         fragment.PID,
		TGID:        fragment.TGID,
		DataLen:     fragment.DataLen,
		TotalLen:    fragment.TotalLen,
		FragCount:   fragment.FragCount,
		LibType:     fragment.LibType,
		Direction:   fragment.Direction,
		Comm:        pending.comm,
		Payload:     payload,
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
