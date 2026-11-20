package main

import (
	"testing"
	"time"
)

func TestParseUIDemoArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		out       string
		stills    string
		requested bool
		wantErr   bool
	}{
		{name: "not requested", args: []string{"--backend", "http://127.0.0.1:8080"}},
		{name: "default output", args: []string{"--render-ui-demo"}, requested: true, out: "artifacts/renew-ui-demo.mp4"},
		{name: "separate path", args: []string{"--render-ui-demo", "/tmp/demo.mp4"}, requested: true, out: "/tmp/demo.mp4"},
		{name: "equals path", args: []string{"--render-ui-demo=/tmp/demo.mp4"}, requested: true, out: "/tmp/demo.mp4"},
		{name: "stills directory", args: []string{"--ui-demo-stills", "/tmp/stills"}, requested: true, out: "artifacts/renew-ui-demo.mp4", stills: "/tmp/stills"},
		{name: "stray positional", args: []string{"--render-ui-demo", "/tmp/demo.mp4", "stray"}, requested: true, wantErr: true},
		{name: "stills without directory", args: []string{"--ui-demo-stills"}, requested: true, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, stills, requested, err := parseUIDemoArgs(tc.args)
			if requested != tc.requested {
				t.Fatalf("requested = %v, want %v", requested, tc.requested)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out != tc.out {
				t.Errorf("out = %q, want %q", out, tc.out)
			}
			if stills != tc.stills {
				t.Errorf("stills = %q, want %q", stills, tc.stills)
			}
		})
	}
}

// The demo scenes must name pages the real workspace actually routes, otherwise
// the video would fall back to the overview for a mistyped page ID.
func TestUIDemoScenesUseRealWorkspacePages(t *testing.T) {
	seen := map[string]bool{}
	for _, scene := range uiDemoScenes {
		if _, ok := workspaceNavigationLabels[scene.page]; !ok {
			t.Fatalf("scene page %q is not a workspace navigation page", scene.page)
		}
		if seen[scene.page] {
			t.Fatalf("scene page %q is used twice", scene.page)
		}
		seen[scene.page] = true
		if scene.frame <= 0 {
			t.Fatalf("scene %q has no still frame", scene.page)
		}
	}
}

// Sample events must land inside the window the behavior page reads, and they
// must not be empty of the fields the tables render from.
func TestUIDemoEventWindowFitsBehaviorWindow(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	full := uiDemoEventWindow(now, len(uiDemoEvents))
	if len(full) != len(uiDemoEvents) {
		t.Fatalf("got %d events, want %d", len(full), len(uiDemoEvents))
	}
	windowMS := int64((5 * time.Minute).Milliseconds())
	for i, event := range full {
		if event.EventID == "" || event.Type == "" || event.Comm == "" {
			t.Fatalf("event %d lacks an ID, type or command: %+v", i, event)
		}
		ageMS := now.UnixMilli() - event.ReceivedAtMS
		if ageMS <= 0 || ageMS > windowMS {
			t.Fatalf("event %d is %dms old, outside the %dms window", i, ageMS, windowMS)
		}
		if risk := eventRisk(event); risk != "高风险" && risk != "需关注" && risk != "正常" {
			t.Fatalf("event %d has risk %q", i, risk)
		}
	}
	if len(uiDemoEventWindow(now, 2)) != 2 {
		t.Fatal("a shorter window must return exactly the newest rows")
	}
}

func TestNewUIDemoAppIsReadyToRender(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	app := newUIDemoApp(now)
	switch {
	case app.starting:
		t.Error("demo app still starting")
	case !app.connected || !app.healthReady:
		t.Error("demo app is not connected and healthy")
	case !app.health.CaptureHealthy:
		t.Error("demo capture is not healthy")
	case len(app.events) != uiDemoInitialRows:
		t.Errorf("initial events = %d, want %d", len(app.events), uiDemoInitialRows)
	case len(app.system.Processes) == 0:
		t.Error("demo system snapshot has no processes")
	case app.lastSync.IsZero():
		t.Error("demo app has no sync time")
	}
}
