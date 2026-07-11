package events

import (
	"context"
	"testing"
	"time"
)

func TestKernelRiskFeedbackWorkerShutdownAndRestart(t *testing.T) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ShutdownKernelRiskFeedbackWorker(ctx)
	})

	ctx, cancel := context.WithCancel(context.Background())
	StartKernelRiskFeedbackWorker(ctx)
	if !kernelRiskFeedbackWorkerStarted() {
		t.Fatal("worker did not start")
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	if err := ShutdownKernelRiskFeedbackWorker(waitCtx); err != nil {
		waitCancel()
		t.Fatalf("ShutdownKernelRiskFeedbackWorker() error = %v", err)
	}
	waitCancel()
	cancel()
	if kernelRiskFeedbackWorkerStarted() {
		t.Fatal("worker remained started after shutdown")
	}

	restartCtx, restartCancel := context.WithCancel(context.Background())
	StartKernelRiskFeedbackWorker(restartCtx)
	if !kernelRiskFeedbackWorkerStarted() {
		restartCancel()
		t.Fatal("worker did not restart")
	}
	restartCancel()

	waitCtx, waitCancel = context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := ShutdownKernelRiskFeedbackWorker(waitCtx); err != nil {
		t.Fatalf("shutdown after restart error = %v", err)
	}
	if kernelRiskFeedbackWorkerStarted() {
		t.Fatal("restarted worker remained started after cancellation")
	}
}
