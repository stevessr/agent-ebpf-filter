//go:build linux

package componentproc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"agent-ebpf-filter/internal/componentipc"
	"agent-ebpf-filter/internal/componentshm"
	"agent-ebpf-filter/internal/componentwire"
)

// These end-to-end benchmarks compare the current per-event UDS+ACK mode
// against the optional memfd+eventfd batch+ACK mode. They include JSON
// serialization and authenticated UDS ACK handling, but NOT kernel tracing,
// ML inference or policy application. They do not isolate transport alone.
func benchmarkEnginePath(b *testing.B, shared bool) {
	if os.Geteuid() == 0 {
		b.Skip("benchmarks require an unprivileged worker")
	}
	exe := filepath.Join(b.TempDir(), "component-engine-worker")
	cmd := exec.Command("go", "build", "-o", exe, "./cmd/component-engine-worker")
	cmd.Dir = "../.."
	if output, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("build worker: %v: %s", err, output)
	}
	const batchSize = 16
	events := make([]componentwire.Event, batchSize)
	for i := range events {
		events[i] = testEnvelope(strconv.Itoa(i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if shared {
		err := RunEngineShared(ctx, exe, 128, 512, func(session *componentipc.Session, ring *componentshm.Ring) error {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := SendSharedBatch(session, ring, events); err != nil {
					return err
				}
			}
			b.StopTimer()
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	} else {
		err := RunEngine(ctx, exe, func(session *componentipc.Session) error {
			stream, err := componentipc.NewStreamID()
			if err != nil {
				return err
			}
			seq := uint64(0)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, event := range events {
					seq++
					if err := SendEvent(session, stream, seq, event); err != nil {
						return err
					}
				}
			}
			b.StopTimer()
			return nil
		})
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.N*batchSize)/b.Elapsed().Seconds(), "events/s")
}

func BenchmarkEngineUDS16(b *testing.B) {
	benchmarkEnginePath(b, false)
}

func BenchmarkEngineShared16(b *testing.B) {
	benchmarkEnginePath(b, true)
}
