package boundedring

import (
	"strconv"
	"testing"
)

func TestRingPreservesOrderAcrossWrapAndBatchReplacement(t *testing.T) {
	ring := New[int](3)
	if evicted := ring.AddBatch([]int{1, 2}); evicted != 0 {
		t.Fatalf("initial batch evicted %d values", evicted)
	}
	if ring.Add(3) {
		t.Fatal("fill append reported an eviction")
	}
	if !ring.Add(4) {
		t.Fatal("wrapped append did not report an eviction")
	}
	assertRingValues(t, ring.Snapshot(), []int{2, 3, 4})
	assertRingValues(t, ring.Recent(2), []int{3, 4})

	if evicted := ring.AddBatch([]int{5, 6}); evicted != 2 {
		t.Fatalf("wrapped batch evicted %d values, want 2", evicted)
	}
	assertRingValues(t, ring.Snapshot(), []int{4, 5, 6})
	if evicted := ring.AddBatch([]int{7, 8, 9, 10}); evicted != 4 {
		t.Fatalf("replacement batch evicted %d values, want 4", evicted)
	}
	assertRingValues(t, ring.Snapshot(), []int{8, 9, 10})
	if ring.Cap() > ring.Limit() {
		t.Fatalf("capacity = %d, limit = %d", ring.Cap(), ring.Limit())
	}
}

func TestRingResetClearsReferencesAndRetainsCapacity(t *testing.T) {
	ring := New[*int](4)
	values := []int{1, 2, 3}
	for index := range values {
		ring.Add(&values[index])
	}
	capacity := ring.Cap()
	ring.Reset()
	if ring.Len() != 0 || ring.Cap() != capacity {
		t.Fatalf("reset ring = len:%d cap:%d, want 0/%d", ring.Len(), ring.Cap(), capacity)
	}
	ring.Add(&values[0])
	ring.Clear()
	if ring.Len() != 0 || ring.Cap() != 0 {
		t.Fatalf("cleared ring = len:%d cap:%d", ring.Len(), ring.Cap())
	}
}

func TestRingNormalizesInvalidLimit(t *testing.T) {
	ring := New[int](0)
	ring.Add(1)
	ring.Add(2)
	assertRingValues(t, ring.Snapshot(), []int{2})
}

func BenchmarkRingSteadyStateAdd(b *testing.B) {
	for _, limit := range []int{64, 10000} {
		b.Run(strconv.Itoa(limit), func(b *testing.B) {
			ring := New[int](limit)
			for index := 0; index < limit; index++ {
				ring.Add(index)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				ring.Add(iteration)
			}
		})
	}
}

func assertRingValues[T comparable](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("values = %v, want %v", got, want)
		}
	}
}
