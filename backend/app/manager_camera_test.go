package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCameraSubscriberContextCancellationWakesWithoutFrame(t *testing.T) {
	stream, sub := newTestCameraSubscriber()
	t.Cleanup(func() { stopTestCameraSubscriber(stream, sub) })

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := sub.NextFrame(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("NextFrame() error = %v, want deadline exceeded", err)
	}
}

func TestCameraSubscriberReceivesEachFrameOnce(t *testing.T) {
	stream, sub := newTestCameraSubscriber()
	t.Cleanup(func() { stopTestCameraSubscriber(stream, sub) })

	stream.publishFrame([]byte("frame-one"))
	frame, err := sub.NextFrame(context.Background())
	if err != nil {
		t.Fatalf("NextFrame() error = %v", err)
	}
	if string(frame) != "frame-one" {
		t.Fatalf("NextFrame() = %q, want frame-one", frame)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := sub.NextFrame(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("repeated NextFrame() error = %v, want deadline exceeded", err)
	}

	stream.publishFrame([]byte("frame-two"))
	frame, err = sub.NextFrame(context.Background())
	if err != nil {
		t.Fatalf("NextFrame() after publish error = %v", err)
	}
	if string(frame) != "frame-two" {
		t.Fatalf("NextFrame() = %q, want frame-two", frame)
	}
}

func TestCameraSubscriberUnsubscribeWakesWaiter(t *testing.T) {
	stream, sub := newTestCameraSubscriber()
	t.Cleanup(func() { stopTestCameraSubscriber(stream, sub) })

	result := make(chan error, 1)
	go func() {
		_, err := sub.NextFrame(context.Background())
		result <- err
	}()
	sub.Unsubscribe()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("NextFrame() error = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("NextFrame() remained blocked after unsubscribe")
	}
}

func TestShutdownCameraStreamsCancelsAndJoinsProducer(t *testing.T) {
	producerCtx, cancelProducer := context.WithCancel(context.Background())
	done := make(chan struct{})
	stream := &CameraStream{
		devName:      "/dev/video-test",
		frameNotify:  make(chan struct{}),
		cancelFunc:   cancelProducer,
		producerDone: done,
		running:      true,
	}
	go func() {
		<-producerCtx.Done()
		stream.streamMu.Lock()
		stream.running = false
		stream.stopping = false
		stream.streamMu.Unlock()
		close(done)
	}()

	streamsMu.Lock()
	oldStreams := activeStreams
	activeStreams = map[string]*CameraStream{stream.devName: stream}
	streamsMu.Unlock()
	t.Cleanup(func() {
		cancelProducer()
		streamsMu.Lock()
		activeStreams = oldStreams
		streamsMu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdownCameraStreams(ctx); err != nil {
		t.Fatalf("shutdownCameraStreams() error = %v", err)
	}
	stream.streamMu.Lock()
	running := stream.running
	stream.streamMu.Unlock()
	if running {
		t.Fatal("camera producer remained running after shutdown")
	}
}

func newTestCameraSubscriber() (*CameraStream, *CameraSubscriber) {
	stream := &CameraStream{
		frameNotify:     make(chan struct{}),
		running:         true,
		subscriberCount: 1,
	}
	return stream, &CameraSubscriber{
		stream: stream,
		done:   make(chan struct{}),
	}
}

func stopTestCameraSubscriber(stream *CameraStream, sub *CameraSubscriber) {
	stream.streamMu.Lock()
	stream.running = false
	if stream.stopTimer != nil {
		stream.stopTimer.Stop()
		stream.stopTimer = nil
	}
	stream.streamMu.Unlock()
	sub.Unsubscribe()
}
