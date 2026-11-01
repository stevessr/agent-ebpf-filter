package main

import (
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// benchmarkNearestRankNs uses the standard nearest-rank percentile (ceil
// of percentage * sample count) so P95/P99 are not extrapolated from a
// mean or mixed with the Go benchmark's amortized ns/op.
func benchmarkNearestRankNs(sorted []int64, percent int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (percent*len(sorted) + 99) / 100
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func TestBenchmarkNearestRankNs(t *testing.T) {
	for _, tc := range []struct {
		count int
		percent int
		want int64
	}{
		{1, 99, 1},
		{10, 95, 10},
		{100, 95, 95},
		{100, 99, 99},
		{256, 95, 244},
		{256, 99, 254},
	} {
		values := make([]int64, tc.count)
		for i := range values {
			values[i] = int64(i + 1)
		}
		if got := benchmarkNearestRankNs(values, tc.percent); got != tc.want {
			t.Fatalf("count=%d percentile=%d: got %d want %d", tc.count, tc.percent, got, tc.want)
		}
	}
}

// Every recovery cycle observes three consecutive headless UI frames. In
// the legacy variant one frame receives 1200 records; the other two have no
// new events. In the bounded variant the same 1200 retained records are
// delivered in 512/512/176-sized commits over those three frames.
//
// Report both total processing CPU and the P95/P99 of each cycle's
// *slowest* frame, so lower tail latency cannot hide extra aggregate work.
// This measures Go/MyGo CPU, not wall-clock delivery jitter from IPC,
// an actual event ticker, kernel capture, or GPU compositor latency.
func BenchmarkRenewBacklogRecoveryTail(b *testing.B) {
	for _, tc := range []struct {
		name    string
		bounded bool
	}{
		{"legacy-single-1200", false},
		{"bounded-512-per-frame", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			a := newRenewApp("http://127.0.0.1:8080")
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.health.CaptureHealthy = true
			a.eventStreamConnected = true
			a.systemConnected = true
			a.page = "事件"
			a.eventVisibleLimit = 50
			initial := make([]eventSummary, 1200)
			backlog := make([]eventSummary, 1200)
			for i := range initial {
				initial[i] = eventSummary{
					EventID: strconv.Itoa(i+1), ReceivedAtMS: int64(i+1),
					PID: 1000 + i%50, Type: "network_connect",
					Target: "/tmp/initial/" + strconv.Itoa(i%60),
					Comm: "codex", RiskScore: float64(i%101),
				}
				backlog[i] = eventSummary{
					EventID: strconv.Itoa(10000+i), ReceivedAtMS: int64(10000+i),
					PID: 1000 + i%50, Type: "network_connect",
					Target: "/tmp/new/" + strconv.Itoa(i%60),
					Comm: "codex", RiskScore: float64((i+5)%101),
				}
			}
			a.mergeEventWindow(initial, 1200)
			view := ui.NewTester(a.view, 1480, 860)
			view.SetPreferences(ui.Preferences{TextScale: 1})
			for i := 0; i < 3; i++ {
				view.Frame()
			}
			buffer := make([]eventSummary, 1200)
			commitScratch := make([]eventSummary, 0, eventUIFrameCapacity)
			peak := make([]int64, b.N)
			frameSamples := make([]int64, 0, b.N*3)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var pending []eventSummary
				if tc.bounded {
					copy(buffer, backlog)
					pending = buffer
				}
				for frame := 0; frame < 3; frame++ {
					start := time.Now()
					if tc.bounded {
						if len(pending) != 0 {
							var chunk []eventSummary
							chunk, pending = takeOldestEventUIBatch(pending, 0, eventUIFrameCapacity, commitScratch)
							a.mergeEventWindow(chunk, 1200)
						}
					} else if frame == 0 {
						a.mergeEventWindow(backlog, 1200)
					}
					view.Frame()
					ns := time.Since(start).Nanoseconds()
					frameSamples = append(frameSamples, ns)
					if ns > peak[i] {
						peak[i] = ns
					}
				}
			}
			b.StopTimer()
			runtime.ReadMemStats(&after)
			sort.Slice(peak, func(i, j int) bool { return peak[i] < peak[j] })
			sort.Slice(frameSamples, func(i, j int) bool { return frameSamples[i] < frameSamples[j] })
			b.ReportMetric(float64(benchmarkNearestRankNs(peak, 95)), "p95_peak_frame_ns")
			b.ReportMetric(float64(benchmarkNearestRankNs(peak, 99)), "p99_peak_frame_ns")
			b.ReportMetric(float64(benchmarkNearestRankNs(frameSamples, 99)), "p99_all_frames_ns")
			b.ReportMetric(float64(peak[len(peak)-1]), "max_peak_frame_ns")
			b.ReportMetric(float64(after.NumGC-before.NumGC)*1000/float64(b.N*3), "gc_cycles/1k_frames")
			b.ReportMetric(float64(after.PauseTotalNs-before.PauseTotalNs)/float64(b.N*3), "gc_pause_ns/frame")
		})
	}
}
