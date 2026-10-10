package eventqueue

import (
	"testing"
)

type fakeEvent struct {
	Value int
}

func TestOfferTransferAndFullQueue(t *testing.T) {
	queue := make(chan *fakeEvent, 1)
	event := &fakeEvent{Value: 42}
	if got := Offer(queue, event); got != Accepted {
		t.Fatalf("accepted = %v", got)
	}
	if got := <-queue; got != event {
		t.Fatal("ownership handoff must preserve pointer identity")
	}
	if got := Offer(queue, event); got != Accepted {
		t.Fatalf("second accepted = %v", got)
	}
	if got := Offer(queue, &fakeEvent{}); got != QueueFull {
		t.Fatalf("full queue result = %v", got)
	}
}

func TestOfferRejectsNilAndMissingQueue(t *testing.T) {
	var absent chan *fakeEvent
	var absentEvent *fakeEvent
	if got := Offer(absent, absentEvent); got != NilItem {
		t.Fatalf("nil event precedence = %v", got)
	}
	if got := Offer(absent, &fakeEvent{}); got != QueueUnavailable {
		t.Fatalf("missing queue result = %v", got)
	}
}

func TestDropReasonLabels(t *testing.T) {
	for _, tc := range []struct {
		source string
		result Result
		want   string
	}{
		{" kernel_event_reader ", Accepted, ""},
		{" kernel_event_reader ", NilItem, "kernel_event_reader:nil_event"},
		{"  ", QueueUnavailable, "unknown:queue_unavailable"},
		{"  ", QueueFull, "unknown:queue_full"},
		{"uds", QueueFull, "uds:queue_full"},
	} {
		if got := DropReason(tc.source, tc.result); got != tc.want {
			t.Fatalf("DropReason(%q, %v) = %q, want %q", tc.source, tc.result, got, tc.want)
		}
	}
}
