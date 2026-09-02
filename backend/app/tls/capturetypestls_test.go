package tls

import (
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestBPFKtimeToWallClockPreservesEventAge(t *testing.T) {
	var mono unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &mono); err != nil {
		t.Skipf("CLOCK_MONOTONIC unavailable: %v", err)
	}
	currentMonoNS := uint64(mono.Sec)*uint64(time.Second) + uint64(mono.Nsec)
	const age = 250 * time.Millisecond
	if currentMonoNS <= uint64(age) {
		t.Skip("system monotonic clock is too young for test")
	}

	before := time.Now().UTC()
	got := bpfKtimeToWallClock(currentMonoNS - uint64(age))
	after := time.Now().UTC()

	oldestExpected := before.Add(-age - 50*time.Millisecond)
	newestExpected := after.Add(-age + 50*time.Millisecond)
	if got.Before(oldestExpected) || got.After(newestExpected) {
		t.Fatalf("converted timestamp %s outside expected range [%s, %s]", got, oldestExpected, newestExpected)
	}
}

func TestBPFKtimeToWallClockZeroFallsBackToNow(t *testing.T) {
	before := time.Now().UTC()
	got := bpfKtimeToWallClock(0)
	after := time.Now().UTC()
	if got.Before(before) || got.After(after) {
		t.Fatalf("zero timestamp fallback %s outside [%s, %s]", got, before, after)
	}
}
