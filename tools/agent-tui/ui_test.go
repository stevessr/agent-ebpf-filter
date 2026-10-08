package main

import (
	"strings"
	"testing"
	"time"

	"github.com/rivo/tview"

	"agent-ebpf-filter/pb"
)

func TestSafeTextNeutralisesTagsAndControlChars(t *testing.T) {
	cases := map[string]string{
		"plain":            "plain",
		"":                 "",
		"[red]evil[-]":     tview.Escape("[red]evil[-]"),
		"a\x1b[31mb\nc":    "a·[31mb·c",
		"路径/文件":            "路径/文件",
		"tab\there":        "tab·here",
		"bracket [ only":   tview.Escape("bracket [ only"),
		"\x7fdel":          "·del",
		"[#ff0000::b]x[-]": tview.Escape("[#ff0000::b]x[-]"),
	}
	for in, want := range cases {
		if got := safeText(in); got != want {
			t.Fatalf("safeText(%q) = %q, want %q", in, got, want)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { safeText("/usr/lib/python3/site-packages") }); allocs != 0 {
		t.Fatalf("clean safeText allocated %.1f per call", allocs)
	}
}

func TestEventContentCells(t *testing.T) {
	at := time.Date(2026, 9, 11, 10, 30, 15, 250_000_000, time.UTC)
	content := &eventContent{rows: []entry{
		{at: at, ev: &pb.Event{Type: "network_connect", Comm: "curl", Pid: 42, Tag: "Network Tool", NetEndpoint: "1.1.1.1:443", Domain: "one.one.one.one", RiskScore: 75}},
		{at: at, ev: &pb.Event{Type: "rename", Comm: "mv", Pid: 7, Path: "/tmp/a", ExtraPath: "/tmp/b", Decision: "BLOCK"}},
		{at: at, ev: &pb.Event{Type: "syscall", Comm: "x", ExtraInfo: "nr=17"}},
	}}
	if content.GetRowCount() != 4 || content.GetColumnCount() != len(columns) {
		t.Fatalf("rows/cols = %d/%d", content.GetRowCount(), content.GetColumnCount())
	}
	if header := content.GetCell(0, 1); header == nil || header.Text != "TYPE" {
		t.Fatalf("header cell = %+v", header)
	}
	if content.GetCell(4, 0) != nil || content.GetCell(1, len(columns)) != nil || content.GetCell(-1, 0) != nil {
		t.Fatal("out-of-range cells were not nil")
	}

	row := func(r int) []string {
		out := make([]string, len(columns))
		for c := range columns {
			out[c] = content.GetCell(r, c).Text
		}
		return out
	}
	first := row(1)
	if first[0] != "10:30:15.250" || first[1] != "network_connect" || first[2] != "42" || first[3] != "curl" || first[5] != "75" {
		t.Fatalf("network row = %q", first)
	}
	if !strings.Contains(first[6], "1.1.1.1:443") || !strings.Contains(first[6], "one.one.one.one") {
		t.Fatalf("network target = %q", first[6])
	}
	second := row(2)
	if second[5] != "BLOCK" || second[6] != "/tmp/a → /tmp/b" {
		t.Fatalf("rename row = %q", second)
	}
	if third := row(3); third[6] != "nr=17" || third[5] != "" {
		t.Fatalf("syscall row = %q", third)
	}
	if ref, ok := content.GetCell(1, 0).GetReference().(*pb.Event); !ok || ref.GetPid() != 42 {
		t.Fatal("cell reference does not point at the event")
	}
}

func TestDescribeEventListsPopulatedFields(t *testing.T) {
	e := entry{at: time.Unix(0, 0).UTC(), ev: &pb.Event{
		Type: "openat", Comm: "node", Pid: 5, Path: "/etc/[passwd]",
		EventType: pb.EventType(1), SanitizedFields: []string{"path", "argv"},
	}}
	text := describeEvent(e)
	for _, want := range []string{"comm", "node", "pid", "5", "sanitized_fields", "path, argv", tview.Escape("/etc/[passwd]")} {
		if !strings.Contains(text, want) {
			t.Fatalf("describeEvent output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "risk_score") || strings.Contains(text, "net_endpoint") {
		t.Fatalf("describeEvent listed unset fields:\n%s", text)
	}
}

func TestSparklineScalesToPeak(t *testing.T) {
	if got := sparkline([]uint32{0, 1, 4, 8}); got != " ▁▄█" {
		t.Fatalf("sparkline = %q", got)
	}
	if got := sparkline([]uint32{0, 0}); got != "  " {
		t.Fatalf("empty sparkline = %q", got)
	}
}

func TestUIBuildsAndRefreshesHeadless(t *testing.T) {
	model := NewModel(50)
	model.OnEvents([]*pb.Event{
		{Type: "execve", Comm: "claude", Pid: 1, Path: "/usr/bin/claude"},
		{Type: "unlink", Comm: "rm", Pid: 2, Path: "/x", Decision: "BLOCK", RiskScore: 90},
	}, time.Now())
	model.OnState(StateConnected, "ws://test/ws")
	ui := NewUI(Config{BackendURL: "http://test"}, model, NewTLSModel(50))
	ui.refresh()
	if len(ui.content.rows) != 2 {
		t.Fatalf("table rows = %d, want 2", len(ui.content.rows))
	}
	header := ui.header.GetText(true)
	for _, want := range []string{"http://test", "events 2", "blocked 1", "follow"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header %q missing %q", header, want)
		}
	}
	sidebar := ui.sidebar.GetText(true)
	if !strings.Contains(sidebar, "execve") || !strings.Contains(sidebar, "top event types") {
		t.Fatalf("sidebar = %q", sidebar)
	}
	ui.sidebarMode = 1
	ui.refresh()
	if sidebar := ui.sidebar.GetText(true); !strings.Contains(sidebar, "claude") || !strings.Contains(sidebar, "top processes") {
		t.Fatalf("process sidebar = %q", sidebar)
	}

	// Pausing keeps the rows the user is looking at while the model moves on.
	ui.events.follow = false
	model.OnEvents([]*pb.Event{{Type: "openat", Comm: "cat", Pid: 3}}, time.Now())
	ui.refresh()
	if len(ui.content.rows) != 2 || !strings.Contains(ui.header.GetText(true), "paused") {
		t.Fatalf("paused table advanced to %d rows", len(ui.content.rows))
	}
	ui.events.follow = true
	ui.refresh()
	if len(ui.content.rows) != 3 {
		t.Fatalf("resumed table rows = %d, want 3", len(ui.content.rows))
	}

	model.SetFilter(ParseFilter("decision:block"))
	ui.refresh()
	if len(ui.content.rows) != 1 || ui.content.rows[0].ev.GetComm() != "rm" {
		t.Fatalf("filtered rows = %v", pids(ui.content.rows))
	}
	if !strings.Contains(ui.header.GetText(true), "filter decision:block") {
		t.Fatalf("header does not show filter: %q", ui.header.GetText(true))
	}
}
