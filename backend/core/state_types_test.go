package core

import (
	"fmt"
	"testing"
	"time"
)

func TestEventArchivePreservesNewestRecordsAcrossResizeAndExpiry(t *testing.T) {
	archive := NewEventArchive(3)
	base := time.Date(2026, time.July, 14, 12, 0, 0, 0, time.UTC)
	for offset := 0; offset < 5; offset++ {
		archive.Add(CapturedEventRecord{ReceivedAt: base.Add(time.Duration(offset) * time.Second)})
	}

	assertEventArchiveOffsets(t, archive.Snapshot(0), base, 2, 3, 4)
	assertEventArchiveOffsets(t, archive.Snapshot(2), base, 3, 4)

	archive.SetMax(2)
	assertEventArchiveOffsets(t, archive.Snapshot(0), base, 3, 4)
	archive.SetMax(5)
	archive.Add(CapturedEventRecord{ReceivedAt: base.Add(5 * time.Second)})
	assertEventArchiveOffsets(t, archive.Snapshot(0), base, 3, 4, 5)

	archive.EvictOlderThan(base.Add(5 * time.Second))
	assertEventArchiveOffsets(t, archive.Snapshot(0), base, 5)
	archive.Clear()
	if got := archive.Count(); got != 0 {
		t.Fatalf("archive count after clear = %d, want 0", got)
	}
}

func BenchmarkEventArchiveSteadyStateAdd(b *testing.B) {
	for _, limit := range []int{1500, 10000} {
		b.Run(fmt.Sprintf("limit-%d", limit), func(b *testing.B) {
			archive := NewEventArchive(limit)
			record := CapturedEventRecord{ReceivedAt: time.Now().UTC()}
			for index := 0; index < limit; index++ {
				archive.Add(record)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				archive.Add(record)
			}
		})
	}
}

func assertEventArchiveOffsets(t *testing.T, records []CapturedEventRecord, base time.Time, offsets ...int) {
	t.Helper()
	if len(records) != len(offsets) {
		t.Fatalf("archive length = %d, want %d", len(records), len(offsets))
	}
	for index, offset := range offsets {
		want := base.Add(time.Duration(offset) * time.Second)
		if !records[index].ReceivedAt.Equal(want) {
			t.Fatalf("archive[%d] = %s, want %s", index, records[index].ReceivedAt, want)
		}
	}
}
