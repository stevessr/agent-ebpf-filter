package main

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestRingOverwritesOldest(t *testing.T) {
	r := NewRing[int](3)
	for i := 1; i <= 5; i++ {
		r.Push(i)
	}
	if r.Len() != 3 || r.Cap() != 3 {
		t.Fatalf("len/cap = %d/%d, want 3/3", r.Len(), r.Cap())
	}
	got := []int{r.At(0), r.At(1), r.At(2)}
	if got[0] != 3 || got[1] != 4 || got[2] != 5 {
		t.Fatalf("ring order = %v, want [3 4 5]", got)
	}
	var visited []int
	r.Each(func(_ int, v int) bool {
		visited = append(visited, v)
		return v != 4
	})
	if len(visited) != 2 || visited[1] != 4 {
		t.Fatalf("Each stopped at %v, want [3 4]", visited)
	}
	r.Clear()
	if r.Len() != 0 {
		t.Fatalf("Len after Clear = %d", r.Len())
	}
	r.Push(9)
	if r.At(0) != 9 {
		t.Fatalf("At(0) after Clear+Push = %d", r.At(0))
	}
}

func TestRingZeroCapacityIsUsable(t *testing.T) {
	r := NewRing[string](0)
	r.Push("a")
	r.Push("b")
	if r.Len() != 1 || r.At(0) != "b" {
		t.Fatalf("degenerate ring = len %d first %q", r.Len(), r.At(0))
	}
}

func sampleEvent(typ, comm, path string, pid uint32) *pb.Event {
	return &pb.Event{Type: typ, Comm: comm, Path: path, Pid: pid, Tag: "AI Agent"}
}

func TestParseFilterAndMatch(t *testing.T) {
	events := []*pb.Event{
		sampleEvent("openat", "node", "/home/steve/Project/index.js", 100),
		sampleEvent("execve", "python3", "/usr/bin/python3", 200),
		{Type: "network_connect", Comm: "curl", NetEndpoint: "93.184.216.34:443", Domain: "Example.com", Pid: 300, RiskScore: 55, Decision: "ALERT", Tag: "AI Agent"},
		{Type: "unlink", Comm: "rm", Path: "/etc/shadow", Pid: 400, RiskScore: 90, Decision: "BLOCK", Tag: "Shell"},
	}
	cases := []struct {
		filter string
		want   []uint32
	}{
		{"", []uint32{100, 200, 300, 400}},
		{"NODE", []uint32{100}},
		{"project", []uint32{100}},
		{"type:openat", []uint32{100}},
		{"type:exec", []uint32{200}},
		{"comm:py", []uint32{200}},
		{"pid:300", []uint32{300}},
		{"pid:abc", nil},
		{`tag:"AI Agent"`, []uint32{100, 200, 300}},
		{"tag:shell", []uint32{400}},
		{"risk:>=50", []uint32{300, 400}},
		{"risk:70", []uint32{400}},
		{"decision:block", []uint32{400}},
		{"net:example.com", []uint32{300}},
		{"host:93.184", []uint32{300}},
		{"-type:openat -type:execve", []uint32{300, 400}},
		{"path:/etc", []uint32{400}},
		{"comm:node type:execve", nil},
		{"example.com", []uint32{300}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			f := ParseFilter(tc.filter)
			var got []uint32
			for _, ev := range events {
				if f.Match(ev) {
					got = append(got, ev.GetPid())
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("filter %q matched %v, want %v", tc.filter, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("filter %q matched %v, want %v", tc.filter, got, tc.want)
				}
			}
		})
	}
	if !ParseFilter("   ").IsZero() || ParseFilter("x").IsZero() {
		t.Fatal("IsZero is wrong")
	}
}

func TestContainsFoldDoesNotAllocate(t *testing.T) {
	haystack := "/Home/Steve/Projects/agent-ebpf-filter/backend/app/events/events_network.go"
	if !containsFold(haystack, "app/events") || containsFold(haystack, "missing") {
		t.Fatal("containsFold gave wrong answer")
	}
	if !containsFold("ABC", "abc") || containsFold("ab", "abc") || !containsFold("x", "") {
		t.Fatal("containsFold edge cases failed")
	}
	if allocs := testing.AllocsPerRun(200, func() { containsFold(haystack, "events_network") }); allocs != 0 {
		t.Fatalf("containsFold allocated %.1f per call", allocs)
	}
}

func TestStatsRateAndTop(t *testing.T) {
	s := NewStats()
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 10; i++ {
		s.Observe(sampleEvent("openat", "node", "/x", 1), base)
	}
	for i := 0; i < 4; i++ {
		s.Observe(sampleEvent("execve", "bash", "/y", 2), base.Add(time.Second))
	}
	s.Observe(&pb.Event{Type: "unlink", Comm: "rm", Decision: "BLOCK"}, base.Add(time.Second))
	s.Observe(&pb.Event{Type: "unlink", Comm: "rm", RiskScore: 45}, base.Add(time.Second))

	if s.Total != 16 || s.Blocked != 1 || s.Risky != 2 {
		t.Fatalf("total/blocked/risky = %d/%d/%d", s.Total, s.Blocked, s.Risky)
	}
	now := base.Add(time.Second)
	if rate := s.Rate(now, 1); rate != 6 {
		t.Fatalf("1s rate = %v, want 6", rate)
	}
	if rate := s.Rate(now, 2); rate != 8 {
		t.Fatalf("2s rate = %v, want 8", rate)
	}
	if rate := s.Rate(now.Add(2*time.Minute), 10); rate != 0 {
		t.Fatalf("stale rate = %v, want 0", rate)
	}
	top := s.Top("type", 2)
	if len(top) != 2 || top[0].Key != "openat" || top[0].Count != 10 || top[1].Key != "execve" {
		t.Fatalf("top types = %+v", top)
	}
	if comms := s.Top("comm", 0); len(comms) != 3 || comms[2].Key != "rm" {
		t.Fatalf("top comms = %+v", comms)
	}
	spark := s.Sparkline(now, 3)
	if len(spark) != 3 || spark[1] != 10 || spark[2] != 6 {
		t.Fatalf("sparkline = %v", spark)
	}
	s.Reset()
	if s.Total != 0 || len(s.Top("type", 0)) != 0 {
		t.Fatal("Reset did not clear stats")
	}
}

func TestModelSnapshotHistoryAndEviction(t *testing.T) {
	m := NewModel(3)
	m.OnHistory([]*pb.CapturedEventRecord{
		{Event: sampleEvent("openat", "node", "/a", 1), Timestamp: 1_000},
		{Event: sampleEvent("openat", "node", "/b", 2), Timestamp: 2_000},
	})
	m.OnEvents([]*pb.Event{sampleEvent("execve", "bash", "/c", 3), nil, sampleEvent("execve", "bash", "/d", 4)}, time.Now())
	// History must not re-seed once events exist.
	m.OnHistory([]*pb.CapturedEventRecord{{Event: sampleEvent("openat", "node", "/z", 9)}})

	rows := m.Snapshot(nil)
	if len(rows) != 3 || rows[0].ev.GetPid() != 2 || rows[2].ev.GetPid() != 4 {
		t.Fatalf("snapshot pids = %v", pids(rows))
	}
	summary := m.Summary(time.Now(), 5)
	if summary.Evicted != 1 || summary.Retained != 3 || summary.Total != 4 {
		t.Fatalf("summary = %+v", summary)
	}
	if !m.ConsumeDirty() || m.ConsumeDirty() {
		t.Fatal("dirty flag did not latch/clear")
	}

	m.SetFilter(ParseFilter("comm:bash"))
	rows = m.Snapshot(rows)
	if len(rows) != 2 || cap(rows) < 3 {
		t.Fatalf("filtered snapshot = %v (cap %d)", pids(rows), cap(rows))
	}
	m.Clear()
	if rows = m.Snapshot(rows); len(rows) != 0 || m.Summary(time.Now(), 0).Total != 0 {
		t.Fatal("Clear left data behind")
	}
}

func pids(rows []entry) []uint32 {
	out := make([]uint32, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ev.GetPid())
	}
	return out
}
