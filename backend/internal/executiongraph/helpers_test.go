package executiongraph

import (
	"context"
	"errors"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestParseIntervalBoundsNumericInputWithoutOverflow(t *testing.T) {
	tests := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "", want: 1500 * time.Millisecond},
		{raw: "1", want: 500 * time.Millisecond},
		{raw: "1500", want: 1500 * time.Millisecond},
		{raw: "30001", want: 30 * time.Second},
		{raw: "9223372036854775807", want: 30 * time.Second},
		{raw: "2s", want: 2 * time.Second},
		{raw: "invalid", want: 1500 * time.Millisecond},
	}
	for _, test := range tests {
		if got := ParseInterval(test.raw); got != test.want {
			t.Errorf("ParseInterval(%q) = %s, want %s", test.raw, got, test.want)
		}
	}
}

func TestBuildExecutionGraphPIDTreeReverseChainIsLinearAndCancelable(t *testing.T) {
	const processCount = 10000
	records := make([]Record, 0, processCount-1)
	for pid := processCount; pid >= 2; pid-- {
		records = append(records, Record{Event: &pb.Event{Pid: uint32(pid), Ppid: uint32(pid - 1), Type: "read"}})
	}
	seed := uint32(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tree, err := buildExecutionGraphPIDTreeContext(ctx, records, Filters{PID: &seed, ProcessTree: true})
	if err != nil {
		t.Fatalf("buildExecutionGraphPIDTreeContext() error = %v", err)
	}
	if len(tree) != processCount {
		t.Fatalf("PID tree size = %d, want %d", len(tree), processCount)
	}
	if _, ok := tree[processCount]; !ok {
		t.Fatalf("PID tree omitted deepest descendant %d", processCount)
	}

	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, err := buildExecutionGraphPIDTreeContext(canceled, records, Filters{PID: &seed, ProcessTree: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled PID tree error = %v, want context.Canceled", err)
	}
}
