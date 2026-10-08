// Package boundedring provides a non-concurrent, insertion-ordered ring for
// fixed-size runtime histories. Callers own synchronization.
package boundedring

// Ring retains the newest limit values in insertion order. Its zero value is
// not usable; construct one with New.
type Ring[T any] struct {
	items []T
	start int
	limit int
}

// New constructs a lazily allocated ring. Invalid limits are normalized to 1.
func New[T any](limit int) *Ring[T] {
	if limit <= 0 {
		limit = 1
	}
	return &Ring[T]{limit: limit}
}

// Add appends one value, overwriting the oldest slot in O(1) when full. It
// reports whether an existing value was evicted.
func (r *Ring[T]) Add(value T) bool {
	if r == nil {
		return false
	}
	if len(r.items) < r.limit {
		r.grow(len(r.items) + 1)
		r.items = append(r.items, value)
		return false
	}
	r.items[r.start] = value
	r.start++
	if r.start == r.limit {
		r.start = 0
	}
	return true
}

// AddBatch appends values and returns the number of old or incoming values not
// retained. A batch at least as large as the limit replaces the ring directly.
func (r *Ring[T]) AddBatch(values []T) int {
	if r == nil || len(values) == 0 {
		return 0
	}
	// Single-value batches are common on event-ingest paths. Preserve Add's
	// minimal branch profile instead of paying the bulk-copy setup cost.
	if len(values) == 1 {
		if r.Add(values[0]) {
			return 1
		}
		return 0
	}
	if len(values) >= r.limit {
		evicted := len(r.items) + len(values) - r.limit
		if cap(r.items) < r.limit {
			r.items = make([]T, r.limit)
		} else {
			// Every retained slot is overwritten below, so clearing first would
			// only add another O(limit) memory pass.
			r.items = r.items[:r.limit]
		}
		copy(r.items, values[len(values)-r.limit:])
		r.start = 0
		return evicted
	}

	// A non-full ring is always normalized (start == 0). Fill its unused
	// contiguous tail first; only the remainder can evict retained values.
	if len(r.items) < r.limit {
		free := r.limit - len(r.items)
		appendCount := len(values)
		if appendCount > free {
			appendCount = free
		}
		if appendCount > 0 {
			r.grow(len(r.items) + appendCount)
			r.items = append(r.items, values[:appendCount]...)
			values = values[appendCount:]
		}
		if len(values) == 0 {
			return 0
		}
	}

	// The ring is full here. Since len(values) < limit, overwriting a wrapped
	// batch takes at most two contiguous copies and at most one start wrap.
	evicted := len(values)
	first := len(values)
	if remaining := r.limit - r.start; first > remaining {
		first = remaining
	}
	copy(r.items[r.start:r.start+first], values[:first])
	if first < len(values) {
		copy(r.items, values[first:])
	}
	r.start += len(values)
	if r.start >= r.limit {
		r.start -= r.limit
	}
	return evicted
}

// Recent returns up to limit newest values in their original insertion order.
// A non-positive limit returns the complete retained history.
func (r *Ring[T]) Recent(limit int) []T {
	if r == nil || len(r.items) == 0 {
		return nil
	}
	if limit <= 0 || limit > len(r.items) {
		limit = len(r.items)
	}
	out := make([]T, limit)
	logicalStart := len(r.items) - limit
	physicalStart := (r.start + logicalStart) % len(r.items)
	copied := copy(out, r.items[physicalStart:])
	if copied < limit {
		copy(out[copied:], r.items[:limit-copied])
	}
	return out
}

// Snapshot returns the complete retained history in insertion order.
func (r *Ring[T]) Snapshot() []T {
	return r.Recent(0)
}

// Retain removes values for which keep returns false while preserving logical
// insertion order. It performs the compaction in place and returns the number
// of removed values. A nil predicate leaves the ring unchanged.
func (r *Ring[T]) Retain(keep func(T) bool) int {
	if r == nil || len(r.items) == 0 || keep == nil {
		return 0
	}
	r.normalize()
	write := 0
	for _, value := range r.items {
		if !keep(value) {
			continue
		}
		r.items[write] = value
		write++
	}
	removed := len(r.items) - write
	clear(r.items[write:])
	r.items = r.items[:write]
	return removed
}

// Reset removes all values while retaining the bounded backing allocation.
func (r *Ring[T]) Reset() {
	if r == nil {
		return
	}
	clear(r.items)
	r.items = r.items[:0]
	r.start = 0
}

// Clear removes all values and releases the backing allocation.
func (r *Ring[T]) Clear() {
	if r == nil {
		return
	}
	clear(r.items)
	r.items = nil
	r.start = 0
}

func (r *Ring[T]) Len() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

func (r *Ring[T]) Cap() int {
	if r == nil {
		return 0
	}
	return cap(r.items)
}

func (r *Ring[T]) Limit() int {
	if r == nil {
		return 0
	}
	return r.limit
}

func (r *Ring[T]) grow(needed int) {
	if r == nil || needed <= cap(r.items) {
		return
	}
	capacity := cap(r.items) * 2
	if capacity < 16 {
		capacity = 16
	}
	if capacity < needed {
		capacity = needed
	}
	if capacity > r.limit {
		capacity = r.limit
	}
	items := make([]T, len(r.items), capacity)
	copy(items, r.items)
	r.items = items
}

func (r *Ring[T]) normalize() {
	if r == nil || r.start == 0 || len(r.items) < 2 {
		return
	}
	reverse(r.items[:r.start])
	reverse(r.items[r.start:])
	reverse(r.items)
	r.start = 0
}

func reverse[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
