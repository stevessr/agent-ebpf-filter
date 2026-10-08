package signalruntime

import (
	"agent-ebpf-filter/core"
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"

	"google.golang.org/protobuf/proto"
)

func TestSignalProgramLogWriterPersistsAndDrainsAcceptedWork(t *testing.T) {
	tempDir := t.TempDir()
	oldRoot := signalProgramLogsRootPath
	signalProgramLogsRootPath = func() string { return tempDir }
	t.Cleanup(func() { signalProgramLogsRootPath = oldRoot })

	record := CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event: &pb.Event{
			Pid:  42,
			Comm: "codex",
			Path: "/tmp/queued.txt",
		},
	}
	item := signalProgramLogWorkItem{
		record: record,
		matches: []signalProgramLogMatch{{
			selected: SelectedProgramSignalLog{Program: "codex", Enabled: true, Path: "codex.pb.gzlog"},
			reason:   "test match",
		}},
	}

	writer := newSignalProgramLogWriter()
	ctx, cancel := context.WithCancel(context.Background())
	writer.Start(ctx, 8)
	cleanupSignalProgramLogWriter(t, writer, cancel)
	for index := 0; index < 5; index++ {
		accepted, active := writer.Enqueue(item)
		if !accepted || !active {
			t.Fatalf("Enqueue(%d) = accepted:%v active:%v, want true/true", index, accepted, active)
		}
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := writer.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	status := writer.Status()
	if status.Running || status.Accepting {
		t.Fatalf("writer still active after shutdown: %+v", status)
	}
	if status.EnqueuedTotal != 5 || status.CompletedTotal != 5 || status.PersistedTotal != 5 || status.FailedTotal != 0 || status.DroppedTotal != 0 {
		t.Fatalf("unexpected writer status after drain: %+v", status)
	}

	frames, err := readCompressedProtoFrames(filepath.Join(tempDir, "codex.pb.gzlog"), func() proto.Message {
		return &pb.ProgramSignalLogRecord{}
	})
	if err != nil {
		t.Fatalf("read queued signal log: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("persisted frames = %d, want 5", len(frames))
	}
}

func TestSignalProgramLogWriterQueueIsBoundedAndNonBlocking(t *testing.T) {
	writer := newSignalProgramLogWriter()
	// A one-slot generation whose consumer never reads: the second enqueue
	// deterministically hits the full-queue path.
	var live <-chan signalProgramLogWorkItem
	ready := make(chan struct{})
	blockedCtx, unblock := context.WithCancel(context.Background())
	defer unblock()
	writer.queue.Start(blockedCtx, 1, func(ctx context.Context, items <-chan signalProgramLogWorkItem) {
		live = items
		close(ready)
		<-ctx.Done()
	})
	<-ready

	if accepted, active := writer.Enqueue(signalProgramLogWorkItem{}); !accepted || !active {
		t.Fatalf("first Enqueue() = accepted:%v active:%v, want true/true", accepted, active)
	}
	if accepted, active := writer.Enqueue(signalProgramLogWorkItem{}); accepted || !active {
		t.Fatalf("full Enqueue() = accepted:%v active:%v, want false/true", accepted, active)
	}
	status := writer.Status()
	if status.QueueLen != 1 || status.QueueCap != 1 || status.EnqueuedTotal != 1 || status.DroppedTotal != 1 {
		t.Fatalf("unexpected full queue status: %+v", status)
	}

	writer.queue.StopAccepting(live)
	if accepted, active := writer.Enqueue(signalProgramLogWorkItem{}); accepted || !active {
		t.Fatalf("stopping Enqueue() = accepted:%v active:%v, want false/true", accepted, active)
	}
	if status := writer.Status(); status.DroppedTotal != 2 {
		t.Fatalf("stopping drop count = %d, want 2", status.DroppedTotal)
	}

	inactive := newSignalProgramLogWriter()
	if accepted, active := inactive.Enqueue(signalProgramLogWorkItem{}); accepted || active {
		t.Fatalf("inactive Enqueue() = accepted:%v active:%v, want false/false", accepted, active)
	}
	if status := inactive.Status(); status.DroppedTotal != 0 {
		t.Fatalf("inactive writer recorded a drop: %+v", status)
	}
}

// Stopping-generation semantics live in internal/workerqueue; this checks
// the writer delegates to it and reports an idle status afterwards.
func TestSignalProgramLogWriterShutdownTimeoutKeepsGeneration(t *testing.T) {
	writer := newSignalProgramLogWriter()
	writer.Start(context.Background(), 4)
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	_ = writer.Shutdown(expired)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := writer.Shutdown(waitCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if status := writer.Status(); status.Running || status.Accepting || status.QueueCap != 0 {
		t.Fatalf("writer after shutdown = %+v", status)
	}
}

func TestSignalProgramLogWriterConcurrentShutdownDrainsEveryAcceptedItem(t *testing.T) {
	writer := newSignalProgramLogWriter()
	ctx, cancel := context.WithCancel(context.Background())
	writer.Start(ctx, 32)
	cleanupSignalProgramLogWriter(t, writer, cancel)

	start := make(chan struct{})
	acceptedOne := make(chan struct{})
	var acceptedOnce sync.Once
	var producers sync.WaitGroup
	for producer := 0; producer < 8; producer++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			<-start
			for index := 0; index < 200; index++ {
				accepted, _ := writer.Enqueue(signalProgramLogWorkItem{})
				if accepted {
					acceptedOnce.Do(func() { close(acceptedOne) })
				}
			}
		}()
	}
	close(start)
	select {
	case <-acceptedOne:
	case <-time.After(time.Second):
		t.Fatal("writer did not accept concurrent work")
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := writer.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	producers.Wait()

	status := writer.Status()
	if status.Running || status.Accepting {
		t.Fatalf("writer still active after concurrent shutdown: %+v", status)
	}
	if status.CompletedTotal != status.EnqueuedTotal {
		t.Fatalf("completed = %d, enqueued = %d; accepted work was lost", status.CompletedTotal, status.EnqueuedTotal)
	}
}

func TestPersistSignalProgramLogUsesActiveWriter(t *testing.T) {
	tempDir := t.TempDir()
	oldRoot := signalProgramLogsRootPath
	oldSnapshot := SnapshotSettingsHook
	oldWriterStore := signalProgramLogWriterStore
	signalProgramLogsRootPath = func() string { return tempDir }
	SnapshotSettingsHook = func() core.RuntimeSettings {
		return core.RuntimeSettings{SignalProcessing: SignalProcessingSettings{
			QueueSize: 8,
			SelectedPrograms: []SelectedProgramSignalLog{{
				Program: "codex",
				Enabled: true,
				Path:    "codex.pb.gzlog",
			}},
		}}
	}
	writer := newSignalProgramLogWriter()
	signalProgramLogWriterStore = writer
	t.Cleanup(func() {
		signalProgramLogsRootPath = oldRoot
		SnapshotSettingsHook = oldSnapshot
		signalProgramLogWriterStore = oldWriterStore
	})

	ctx, cancel := context.WithCancel(context.Background())
	writer.Start(ctx, 8)
	cleanupSignalProgramLogWriter(t, writer, cancel)
	persistSignalProgramLog(CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event:      &pb.Event{Pid: 7, Comm: "codex", Path: "/tmp/async.txt"},
	})

	deadline := time.Now().Add(2 * time.Second)
	for writer.Status().CompletedTotal < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := writer.Status()
	if status.EnqueuedTotal != 1 || status.CompletedTotal != 1 || status.PersistedTotal != 1 {
		t.Fatalf("active writer did not process persisted event: %+v", status)
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutdownCancel()
	if err := writer.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func cleanupSignalProgramLogWriter(t *testing.T, writer *signalProgramLogWriter, cancel context.CancelFunc) {
	t.Helper()
	t.Cleanup(func() {
		cancel()
		ctx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		_ = writer.Shutdown(ctx)
	})
}
