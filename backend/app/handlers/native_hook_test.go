package handlers

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestNativeHookProviderTagRecognizesHarnesses(t *testing.T) {
	tests := []struct {
		name, sourceCLI, userAgent, event, want string
	}{
		{name: "dsh header", sourceCLI: "dsh", want: "DeepSeek Harness"},
		{name: "Pi header", sourceCLI: "pi", want: "Pi"},
		{name: "OMP header", sourceCLI: "omp", want: "Oh My Pi"},
		{name: "Pi user agent", userAgent: "pi-coding-agent/1.0", want: "Pi"},
		{name: "OMP user agent", userAgent: "oh-my-pi/1.0", want: "Oh My Pi"},
		{name: "legacy Gemini event", event: "BeforeTool", want: "Gemini CLI"},
		{name: "unknown", want: "Native Hook"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeHookProviderTag(test.sourceCLI, test.userAgent, test.event); got != test.want {
				t.Fatalf("nativeHookProviderTag(%q, %q, %q) = %q, want %q", test.sourceCLI, test.userAgent, test.event, got, test.want)
			}
		})
	}
}

func TestEmitNativeHookEventDoesNotBlockWhenQueueFull(t *testing.T) {
	oldDeps := Deps
	t.Cleanup(func() { Deps = oldDeps })

	queue := make(chan *pb.Event, 1)
	queue <- &pb.Event{Type: "occupied"}
	Deps.BroadcastCh = queue
	Deps.SendTLSBridge = nil

	done := make(chan struct{})
	go func() {
		emitNativeHookEvent(&pb.Event{Type: "native_hook"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("native hook emitter blocked on a full broadcast queue")
	}

	if got := <-queue; got.GetType() != "occupied" {
		t.Fatalf("full queue contents changed: got %q", got.GetType())
	}
}
