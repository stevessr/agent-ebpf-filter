package agentfeatures

import (
	"reflect"
	"testing"
)

func event(id, host, run, kind string, timestamp int64) Event {
	return Event{
		ID: id, HostID: host, RunID: run, Source: "native_hook",
		Kind: kind, EventTimeNS: timestamp, EventEvidence: Observed,
	}
}

func feature(t *testing.T, f Frame, id string) Value {
	t.Helper()
	for _, v := range f.Values {
		if v.ID == id {
			return v
		}
	}
	t.Fatalf("feature %q missing", id)
	return Value{}
}

func TestExtractIsolationAndDeterministicOrder(t *testing.T) {
	w := Window{StartNS: 100, EndNS: 200}
	first := event("1", "host-b", "same-run", "tool", 100)
	first.Phase = "start"
	second := event("2", "host-a", "same-run", "process_exec", 120)
	second.Source = "ebpf"
	noRun := event("3", "host-a", "", "tool", 130)
	outside := event("4", "host-a", "same-run", "tool", 200)
	noTime := event("5", "host-a", "same-run", "tool", 0)
	items := []Event{first, second, second, noRun, outside, noTime}
	report, err := Extract(items, w)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 2 || report.Frames[0].HostID != "host-a" || report.Frames[1].HostID != "host-b" {
		t.Fatalf("unexpected ordering/scope: %+v", report.Frames)
	}
	if report.Duplicates != 1 || report.SkippedUnknownScope != 1 ||
		report.SkippedOutOfWindow != 1 || report.SkippedNoTime != 1 {
		t.Fatalf("bad counts: %+v", report)
	}
	if v := feature(t, report.Frames[0], "os.process_exec"); !v.Present || v.Value != 1 {
		t.Fatalf("missing OS exec feature: %+v", v)
	}
	reversed := []Event{noTime, outside, noRun, second, second, first}
	replayed, err := Extract(reversed, w)
	if err != nil || !reflect.DeepEqual(report, replayed) {
		t.Fatalf("not replay deterministic: %v\n%+v\n%+v", err, report, replayed)
	}
}

func TestMissingMasksAndExplicitAttribution(t *testing.T) {
	w := Window{StartNS: 1, EndNS: 1000}
	tool := event("t1", "h", "r", "tool", 10)
	tool.Phase = "start"
	failed := event("t2", "h", "r", "tool", 20)
	failed.Phase, failed.Outcome = "result", "failure"
	ok := event("t3", "h", "r", "tool", 30)
	ok.Phase, ok.Outcome = "result", "success"
	os1 := event("o1", "h", "r", "file_write", 40)
	os1.ToolCallID, os1.AttributionEvidence = "tool-1", Observed
	os2 := event("o2", "h", "r", "process_exec", 50)
	os2.ToolCallID, os2.AttributionEvidence = "tool-1", Derived
	os3 := event("o3", "h", "r", "network_connect", 60)
	os3.AttributionEvidence = Unknown
	os3.CaptureAudited, os3.CaptureLoss = true, true
	unknownTool := event("t4", "h", "r", "tool", 70)
	unknownTool.Phase, unknownTool.Outcome, unknownTool.EventEvidence = "result", "failure", Inferred

	report, err := Extract([]Event{tool, failed, ok, os1, os2, os3, unknownTool}, w)
	if err != nil || len(report.Frames) != 1 {
		t.Fatalf("extract: %v %+v", err, report)
	}
	f := report.Frames[0]
	for id, want := range map[string]float64{
		"tool.start_events": 1, "tool.result_events": 2,
		"tool.failure_ratio": .5, "os.file_write": 1,
		"os.explicit_tool_ratio": .5, "evidence.inferred_ratio": 1.0 / 7.0,
		"capture.loss_ratio": 1,
	} {
		v := feature(t, f, id)
		if !v.Present || v.Value != want {
			t.Fatalf("%s: got %+v want %v", id, v, want)
		}
	}
	empty := event("e", "h", "r", "other", 10)
	report, _ = Extract([]Event{empty}, w)
	for _, id := range []string{"tool.failure_ratio", "os.explicit_tool_ratio", "capture.loss_ratio"} {
		v := feature(t, report.Frames[0], id)
		if v.Present || v.Reason != "denominator_unobserved" {
			t.Fatalf("%s must be missing not zero: %+v", id, v)
		}
	}
}

func TestSourceScopedDedupAndNoSyntheticIdentity(t *testing.T) {
	w := Window{StartNS: 1, EndNS: 100}
	a := event("same-id", "host", "run", "tool", 10)
	a.Phase = "start"
	b := a
	b.Source = "otlp"
	c := a
	c.RunID = "another-run"
	d := a
	d.HostID = ""
	r, err := Extract([]Event{a, b, c, d}, w)
	if err != nil {
		t.Fatal(err)
	}
	if r.SkippedUnknownScope != 1 || len(r.Frames) != 2 ||
		feature(t, r.Frames[1], "events.total").Value != 2 {
		t.Fatalf("cross-source collision or phantom scope: %+v", r)
	}
}

func TestBatchBoundariesAndCatalogCopy(t *testing.T) {
	if _, err := Extract(nil, Window{StartNS: 10, EndNS: 10}); err == nil {
		t.Fatal("expected invalid window")
	}
	if _, err := Extract(make([]Event, MaxBatchEvents+1), Window{StartNS: 1, EndNS: 2}); err == nil {
		t.Fatal("expected bounded batch rejection")
	}
	c := Catalog()
	if len(c) != 10 {
		t.Fatalf("unexpected registry size: %d", len(c))
	}
	c[0].ID = "modified"
	if Catalog()[0].ID != "events.total" {
		t.Fatal("catalog unexpectedly mutable")
	}
}

func TestConflictingSourceEventIDsFailClosed(t *testing.T) {
	w := Window{StartNS: 1, EndNS: 100}
	a := event("shared", "host", "run", "tool", 10)
	b := a
	b.Kind = "file_write"
	if _, err := Extract([]Event{a, b}, w); err == nil {
		t.Fatal("conflicting duplicate IDs must not silently choose input order")
	}
	if _, err := Extract([]Event{b, a}, w); err == nil {
		t.Fatal("reverse order must also reject conflicting duplicate IDs")
	}
}
