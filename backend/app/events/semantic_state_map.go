package events

import "container/list"

type semanticStateMapEntry[T any] struct {
	value   T
	element *list.Element
}

// boundedSemanticStateMap is an LRU container without internal synchronization.
// Its owner holds SemanticAlertState.mu around every operation, keeping
// capacity eviction O(1) without an unbounded insertion-order side queue.
type boundedSemanticStateMap[T any] struct {
	capacity int
	entries  map[string]semanticStateMapEntry[T]
	recency  list.List
}

func newBoundedSemanticStateMap[T any](capacity int) *boundedSemanticStateMap[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &boundedSemanticStateMap[T]{
		capacity: capacity,
		// Grow lazily. Preallocating every maximum-sized semantic map would make
		// an otherwise empty state reserve all 24,576 entry buckets up front.
		entries: make(map[string]semanticStateMapEntry[T]),
	}
}

func (m *boundedSemanticStateMap[T]) Get(key string) (T, bool) {
	var zero T
	if m == nil {
		return zero, false
	}
	entry, ok := m.entries[key]
	if !ok {
		return zero, false
	}
	m.recency.MoveToFront(entry.element)
	return entry.value, true
}

// Set inserts or updates key and returns whether one older entry was evicted.
func (m *boundedSemanticStateMap[T]) Set(key string, value T) bool {
	if m == nil || key == "" {
		return false
	}
	if entry, ok := m.entries[key]; ok {
		entry.value = value
		m.entries[key] = entry
		m.recency.MoveToFront(entry.element)
		return false
	}

	evicted := false
	if len(m.entries) >= m.capacity {
		if oldest := m.recency.Back(); oldest != nil {
			delete(m.entries, oldest.Value.(string))
			m.recency.Remove(oldest)
			evicted = true
		}
	}
	element := m.recency.PushFront(key)
	m.entries[key] = semanticStateMapEntry[T]{value: value, element: element}
	return evicted
}

func (m *boundedSemanticStateMap[T]) Delete(key string) bool {
	if m == nil {
		return false
	}
	entry, ok := m.entries[key]
	if !ok {
		return false
	}
	delete(m.entries, key)
	m.recency.Remove(entry.element)
	return true
}

func (m *boundedSemanticStateMap[T]) DeleteIf(predicate func(T) bool) int {
	if m == nil || predicate == nil {
		return 0
	}
	deleted := 0
	for key, entry := range m.entries {
		if !predicate(entry.value) {
			continue
		}
		delete(m.entries, key)
		m.recency.Remove(entry.element)
		deleted++
	}
	return deleted
}

func (m *boundedSemanticStateMap[T]) Len() int {
	if m == nil {
		return 0
	}
	return len(m.entries)
}

func (m *boundedSemanticStateMap[T]) Capacity() int {
	if m == nil {
		return 0
	}
	return m.capacity
}
