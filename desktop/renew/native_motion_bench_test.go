package main

import (
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Benchmarks use MyGo's headless CPU frame path. They measure the cost of
// building, laying out and painting Renew's native workspace, not a GPU frame
// interval or real Wayland/niri compositor performance.
//
// Run: go test -run '^$' -bench '^BenchmarkRenewNativeFrame$' -benchmem -count=3
// A 60 Hz frame lasts 16.67 ms; a 120 Hz frame lasts 8.33 ms. The reported
// percentage is descriptive only: CI runners must not gate on wall-clock FPS.
func BenchmarkRenewNativeFrame(b *testing.B) {
	for _, tc := range []struct {
		name    string
		page    string
		events  int
		refresh bool
		reverse bool
		held    bool
	}{
		{"overview-quiet", "概览", 0, false, false, false},
		{"events-1200-steady", "事件", 1200, false, false, false},
		{"events-1200-refresh", "事件", 1200, true, false, false},
		{"events-1200-nav-reverse-warm", "事件", 1200, false, true, false},
		{"events-1200-nav-two-changes", "事件", 1200, false, false, true},
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
			for i := 0; i < tc.events; i++ {
				a.events = append(a.events, eventSummary{
					EventID:      strconv.Itoa(i + 1),
					Type:         "openat",
					Comm:         "codex",
					PID:          i + 1000,
					RiskScore:    float64(i % 101),
					ReceivedAtMS: int64(i * 1000),
				})
			}
			if tc.reverse {
				// Build the full native sidebar before timing. The initial
				// icon/font cache warmup is a separate one-time cost, not
				// the repeated spring reversal being measured here.
				a.navigationOpen = true
			}
			view := ui.NewTester(a.view, 1480, 860)
			view.SetPreferences(ui.Preferences{TextScale: 1})
			if tc.reverse {
				for i := 0; i < 4; i++ {
					a.navigationOpen = !a.navigationOpen
					view.Frame()
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()
			for i := 0; i < b.N; i++ {
				if tc.refresh {
					// Invalidate only the existing bounded event caches,
					// matching a new IPC summary batch without I/O.
					a.eventsVersion++
				}
				if tc.reverse {
					// Force a reversal every frame to measure the hot
					// interaction path, not just the first opening.
					a.navigationOpen = !a.navigationOpen
				}
				if tc.held && (i == 0 || i == b.N/2) {
					// Realistic interaction: navigate twice, let the
					// surrounding frames settle with constant layout.
					a.navigationOpen = !a.navigationOpen
				}
				view.Frame()
			}
			elapsed := time.Since(start)
			b.StopTimer()
			b.ReportMetric(float64(elapsed)/float64(b.N)/float64(time.Second/120)*100, "pct_120Hz_budget")
			b.ReportMetric(float64(elapsed)/float64(b.N)/float64(time.Second/60)*100, "pct_60Hz_budget")
		})
	}
}
