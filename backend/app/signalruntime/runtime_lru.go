package signalruntime

import (
	"time"
)

func decaySignalScore(score float64, lastMatchedAt, now time.Time, ttlSeconds int) float64 {
	if score <= 0 {
		return 0
	}
	if ttlSeconds <= 0 {
		ttlSeconds = signalDefaultTTLSeconds
	}
	if lastMatchedAt.IsZero() || now.IsZero() || !now.After(lastMatchedAt) {
		return score
	}
	elapsed := now.Sub(lastMatchedAt).Seconds()
	ttl := float64(ttlSeconds)
	if elapsed >= ttl {
		return 0
	}
	remaining := 1 - elapsed/ttl
	if remaining < 0 {
		return 0
	}
	return score * remaining
}

func (w *signalProcessingWorker) evictExpiredLocked(now time.Time) {
	for key, state := range w.states {
		if state == nil || (!state.ExpiresAt.IsZero() && !state.ExpiresAt.After(now)) {
			if state == nil {
				delete(w.states, key)
				w.expiredTotal++
				continue
			}
			w.removeSignalStateLocked(state, false)
		}
	}
}

func (w *signalProcessingWorker) enforceMaxStatesLocked(maxStates int, now time.Time) {
	if maxStates <= 0 {
		maxStates = signalDefaultMaxStates
	}
	for len(w.states) > maxStates && w.stateLRUHead != nil {
		state := w.stateLRUHead
		capacityEviction := state.ExpiresAt.IsZero() || state.ExpiresAt.After(now)
		w.removeSignalStateLocked(state, capacityEviction)
	}
	// All production inserts are linked into the LRU. Keep a bounded defensive
	// fallback for malformed test or migrated state rather than leaving the map
	// above its configured hard limit.
	for len(w.states) > maxStates {
		for key, state := range w.states {
			if state == nil {
				delete(w.states, key)
				w.expiredTotal++
			} else {
				capacityEviction := state.ExpiresAt.IsZero() || state.ExpiresAt.After(now)
				w.removeSignalStateLocked(state, capacityEviction)
			}
			break
		}
	}
}

func (w *signalProcessingWorker) appendSignalStateLocked(state *signalState) {
	if state == nil {
		return
	}
	state.lruPrev = w.stateLRUTail
	state.lruNext = nil
	if w.stateLRUTail == nil {
		w.stateLRUHead = state
	} else {
		w.stateLRUTail.lruNext = state
	}
	w.stateLRUTail = state
}

func (w *signalProcessingWorker) touchSignalStateLocked(state *signalState) {
	if state == nil || w.stateLRUTail == state {
		return
	}
	w.detachSignalStateLocked(state)
	w.appendSignalStateLocked(state)
}

func (w *signalProcessingWorker) removeSignalStateLocked(state *signalState, capacityEviction bool) {
	if state == nil || w.states[state.Key] != state {
		return
	}
	delete(w.states, state.Key)
	w.detachSignalStateLocked(state)
	w.expiredTotal++
	if capacityEviction {
		w.capacityEvictedTotal++
	}
}

func (w *signalProcessingWorker) detachSignalStateLocked(state *signalState) {
	if state == nil {
		return
	}
	if state.lruPrev == nil {
		if w.stateLRUHead == state {
			w.stateLRUHead = state.lruNext
		}
	} else {
		state.lruPrev.lruNext = state.lruNext
	}
	if state.lruNext == nil {
		if w.stateLRUTail == state {
			w.stateLRUTail = state.lruPrev
		}
	} else {
		state.lruNext.lruPrev = state.lruPrev
	}
	state.lruPrev = nil
	state.lruNext = nil
}
