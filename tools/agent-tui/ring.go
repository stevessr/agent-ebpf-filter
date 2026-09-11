package main

// Ring is a fixed-capacity FIFO that overwrites its oldest entry when full.
// Elements are stored by value; callers keep pointers in T when they want the
// ring to hold views rather than copies.
type Ring[T any] struct {
	buf  []T
	head int // index of the oldest element
	n    int
}

func NewRing[T any](capacity int) *Ring[T] {
	if capacity <= 0 {
		capacity = 1
	}
	return &Ring[T]{buf: make([]T, capacity)}
}

// Push appends v, evicting the oldest element when the ring is full.
func (r *Ring[T]) Push(v T) {
	if r.n < len(r.buf) {
		r.buf[(r.head+r.n)%len(r.buf)] = v
		r.n++
		return
	}
	r.buf[r.head] = v
	r.head = (r.head + 1) % len(r.buf)
}

func (r *Ring[T]) Len() int { return r.n }

func (r *Ring[T]) Cap() int { return len(r.buf) }

// At returns the i-th oldest element; 0 is the oldest, Len()-1 the newest.
func (r *Ring[T]) At(i int) T {
	return r.buf[(r.head+i)%len(r.buf)]
}

// Each visits elements from oldest to newest until fn returns false.
func (r *Ring[T]) Each(fn func(i int, v T) bool) {
	for i := 0; i < r.n; i++ {
		if !fn(i, r.At(i)) {
			return
		}
	}
}

// Clear drops every element and lets the GC reclaim what they referenced.
func (r *Ring[T]) Clear() {
	clear(r.buf)
	r.head, r.n = 0, 0
}
