package app

import (
	"context"
	"testing"
	"time"

	"agent-ebpf-filter/internal/wsfanout"
	"agent-ebpf-filter/pb"
)

func TestEventBroadcasterStopsClientHubsWithContext(t *testing.T) {
	originalContext := AppCtx
	appContext := newAppContext()
	appContext.Broadcast = make(chan *pb.Event)
	appContext.EventClientHub = wsfanout.New(wsfanout.Options{})
	appContext.EnvelopeClientHub = wsfanout.New(wsfanout.Options{})
	AppCtx = appContext
	t.Cleanup(func() {
		appContext.Network.Close()
		AppCtx = originalContext
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runEventBroadcaster(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("event broadcaster did not stop after cancellation")
	}
	if !appContext.EventClientHub.Closed() || !appContext.EnvelopeClientHub.Closed() {
		t.Fatalf("client hubs closed = events:%v envelopes:%v", appContext.EventClientHub.Closed(), appContext.EnvelopeClientHub.Closed())
	}
}
