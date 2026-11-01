package main

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Live stream benchmarks feed event batches into the actual Renew merge,
// update the selected native page and run a complete headless MyGo frame.
// All setup, event generation and font warmup happen before the timed loop.
// No IPC, kernel eBPF, compositor or actual GPU is exercised here.
func BenchmarkRenewLiveEventStream(b *testing.B) {
	for _, tc := range []struct {
		name string
		page string
		batchSize int
		uniquePerBatch int
	}{
		{"events-32-unique", "事件", 32, 32},
		{"events-32-same-id-revisions", "事件", 32, 4},
		{"network-8-unique", "网络", 8, 8},
		{"process-8-unique", "进程", 8, 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			a := newRenewApp("http://127.0.0.1:8080")
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.health.CaptureHealthy = true
			a.eventStreamConnected = true
			a.systemConnected = true
			a.page = tc.page
			a.eventVisibleLimit = 50
			initial := make([]eventSummary, 1200)
			for i := range initial {
				initial[i] = eventSummary{
					EventID: strconv.Itoa(50000-i), ReceivedAtMS: int64(50000-i),
					PID: 10000 + i%200, Type: "network_connect", Network: true,
					Target: "host-" + strconv.Itoa(i%500) + ".example",
					Comm: "codex", RiskScore: float64(i%101),
				}
			}
			a.mergeEventWindow(initial, 1200)
			batches := make([][]eventSummary, b.N)
			for i := range batches {
				batch := make([]eventSummary, tc.batchSize)
				for j := range batch {
					batch[j] = eventSummary{
						EventID: strconv.Itoa(100000 + i*tc.uniquePerBatch + j%tc.uniquePerBatch),
						ReceivedAtMS: int64(100000 + i*tc.batchSize + j),
						PID: 10000 + (j+i)%200,
						Comm: "codex", Type: "network_connect", Network: true,
						Target: "host-" + strconv.Itoa((j+i)%500) + ".example",
						RiskScore: float64((j+i)%101),
						NetBytes: int64(j+1) * 1024,
					}
				}
				batches[i] = batch
			}
			view := ui.NewTester(a.view, 1480, 860)
			view.SetPreferences(ui.Preferences{TextScale: 1})
			for i := 0; i < 3; i++ {
				view.Frame() // warm up native fonts, icons, table and scene
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()
			for i := 0; i < b.N; i++ {
				a.mergeEventWindow(batches[i], 1200)
				view.Frame()
			}
			duration := time.Since(start)
			b.StopTimer()
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(duration.Nanoseconds())/float64(b.N)/float64(time.Second/60)*100, "pct_60Hz_cpu_budget")
			b.ReportMetric(float64(duration.Nanoseconds())/float64(b.N)/float64(time.Second/120)*100, "pct_120Hz_cpu_budget")
			b.ReportMetric(float64(after.NumGC-before.NumGC)*1000/float64(b.N), "gc_cycles/1k_frames")
			b.ReportMetric(float64(after.PauseTotalNs-before.PauseTotalNs)/float64(b.N), "gc_pause_ns/frame")
		})
	}
}

var streamDedupSink int

// This is an isolated before/after microbenchmark of the exact same search
// corpus, using an incoming stream where many summaries amend the same IDs.
func BenchmarkRenewSearchTextDedup(b *testing.B) {
	incoming := make([]eventSummary, 512)
	for i := range incoming {
		incoming[i] = eventSummary{
			EventID: strconv.Itoa(i % 16),
			Type: "network_connect", Comm: "codex",
			Target: "https://events.example/" + strconv.Itoa(i),
			PID: 1000 + i, ReceivedAtMS: int64(i),
		}
	}
	b.Run("previous-build-before-dedup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m := make(map[string]eventSummary, 16)
			for _, event := range incoming {
				event.SearchText = buildEventSearchText(event)
				m[event.EventID] = event
			}
			streamDedupSink = len(m)
		}
	})
	b.Run("build-after-dedup", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			m := make(map[string]eventSummary, 16)
			for _, event := range incoming {
				m[event.EventID] = event
			}
			for key, event := range m {
				event.SearchText = buildEventSearchText(event)
				m[key] = event
			}
			streamDedupSink = len(m)
		}
	})
}
