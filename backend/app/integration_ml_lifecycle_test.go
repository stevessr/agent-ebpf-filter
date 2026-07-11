package app

import (
	"context"
	"testing"
	"time"
)

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
