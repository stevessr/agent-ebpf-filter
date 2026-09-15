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

func TestRingAddBatchFillsThenWrapsInBulk(t *testing.T) {
	ring := New[int](5)
	if evicted := ring.AddBatch([]int{1, 2, 3, 4}); evicted != 0 {
		t.Fatalf("initial batch evicted %d values", evicted)
	}

	// Cross the not-full -> full boundary in one batch. Only the values beyond
	// the final free slot should evict retained entries.
	if evicted := ring.AddBatch([]int{5, 6, 7}); evicted != 2 {
		t.Fatalf("boundary-crossing batch evicted %d values, want 2", evicted)
	}
	assertRingValues(t, ring.Snapshot(), []int{3, 4, 5, 6, 7})

	// start is now non-zero. This batch crosses the physical slice boundary and
	// exercises the two-copy wrapped path.
	if evicted := ring.AddBatch([]int{8, 9, 10, 11}); evicted != 4 {
		t.Fatalf("wrapped bulk batch evicted %d values, want 4", evicted)
	}
	assertRingValues(t, ring.Snapshot(), []int{7, 8, 9, 10, 11})
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

func TestRingRetainCompactsWrappedValuesInLogicalOrder(t *testing.T) {
	ring := New[int](5)
	ring.AddBatch([]int{1, 2, 3, 4, 5})
	ring.Add(6)
	if removed := ring.Retain(func(value int) bool { return value%2 == 0 }); removed != 2 {
		t.Fatalf("retain removed %d values, want 2", removed)
	}
	assertRingValues(t, ring.Snapshot(), []int{2, 4, 6})

	ring.AddBatch([]int{7, 8, 9})
	assertRingValues(t, ring.Snapshot(), []int{4, 6, 7, 8, 9})
	if removed := ring.Retain(nil); removed != 0 {
		t.Fatalf("nil retain removed %d values, want 0", removed)
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

func BenchmarkRingSteadyStateAddBatch(b *testing.B) {
	for _, limit := range []int{64, 10000} {
		for _, batchSize := range []int{1, 8, 64} {
			if batchSize > limit {
				continue
			}
			name := "limit=" + strconv.Itoa(limit) + "/batch=" + strconv.Itoa(batchSize)
			b.Run(name, func(b *testing.B) {
				ring := New[int](limit)
				warm := make([]int, limit)
				ring.AddBatch(warm)
				batch := make([]int, batchSize)
				b.ReportAllocs()
				b.SetBytes(int64(batchSize * 8))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					ring.AddBatch(batch)
				}
			})
		}
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
