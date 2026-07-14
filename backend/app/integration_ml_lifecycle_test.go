package app

import (
	"context"
	"testing"
	"time"
)

func TestMLAutoTrainLoopSurvivesDisabledRuntimeAndReloadsInterval(t *testing.T) {
	previousRuntime := runtimeSettingsStore
	previousStore := globalTrainingStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{MLConfig: MLConfig{
		Enabled:       false,
		AutoTrain:     false,
		TrainInterval: "5ms",
	}}}
	globalTrainingStore = nil
	t.Cleanup(func() {
		runtimeSettingsStore = previousRuntime
		globalTrainingStore = previousStore
	})

	var intervals []time.Duration
	mlAutoTrainLoopWithWait(context.Background(), func(_ context.Context, interval time.Duration) bool {
		intervals = append(intervals, interval)
		if len(intervals) == 1 {
			runtimeSettingsStore.mu.Lock()
			runtimeSettingsStore.settings.MLConfig.TrainInterval = "9ms"
			runtimeSettingsStore.mu.Unlock()
			return true
		}
		return false
	})
	if len(intervals) != 2 || intervals[0] != 5*time.Millisecond || intervals[1] != 9*time.Millisecond {
		t.Fatalf("scheduler intervals = %v, want [5ms 9ms]", intervals)
	}
}

func TestMLFlushLoopContinuesUntilWaitCancellation(t *testing.T) {
	waits := 0
	flushes := 0
	mlFlushLoopWithWait(context.Background(), time.Second, func(_ context.Context, interval time.Duration) bool {
		if interval != time.Second {
			t.Fatalf("flush interval = %s, want 1s", interval)
		}
		waits++
		return waits < 3
	}, func() {
		flushes++
	})
	if waits != 3 || flushes != 3 {
		t.Fatalf("flush scheduler = waits:%d flushes:%d, want 3/3", waits, flushes)
	}
}

func TestMLAutoTrainIntervalUsesSafeFallback(t *testing.T) {
	if got := mlAutoTrainInterval(MLConfig{TrainInterval: " 25ms "}); got != 25*time.Millisecond {
		t.Fatalf("parsed train interval = %s", got)
	}
	for _, raw := range []string{"", "invalid", "0s", "-1s"} {
		if got := mlAutoTrainInterval(MLConfig{TrainInterval: raw}); got != mlAutoTrainFallbackInterval {
			t.Fatalf("fallback interval for %q = %s", raw, got)
		}
	}
}

func TestMLBackgroundLoopsStopOnContextCancellation(t *testing.T) {
	previousStore := globalTrainingStore
	globalTrainingStore = nil
	t.Cleanup(func() { globalTrainingStore = previousStore })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, run := range map[string]func(context.Context){
		"auto_train": mlAutoTrainLoop,
		"flush":      mlFlushLoop,
	} {
		t.Run(name, func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				run(ctx)
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatalf("%s loop did not stop after context cancellation", name)
			}
		})
	}
}
