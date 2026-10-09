package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWorkspaceNavigationIconCoverage(t *testing.T) {
	// Every route shown in the detailed sidebar has a real, reusable SVG.
	for page := range workspaceNavigationLabels {
		icon := workspaceNavigationIcon(page)
		if icon == nil {
			t.Errorf("missing native navigation SVG for %q", page)
			continue
		}
		if width, height := icon.Size(); width != 24 || height != 24 {
			t.Errorf("icon %q is %.0fx%.0f, expected 24x24", page, width, height)
		}
	}
	seen := make(map[string]bool)
	for _, page := range workspaceRailPages {
		if seen[page] {
			t.Errorf("duplicate activity rail page %q", page)
		}
		seen[page] = true
		if workspaceNavigationIcon(page) == nil {
			t.Errorf("activity rail page %q has no SVG", page)
		}
	}
	if workspaceNavigationIcon("unknown") != nil {
		t.Fatal("unknown page must not silently show an unrelated icon")
	}
}

func TestWorkspaceRailSelectionKeepsIconIdentity(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	view := ui.NewTester(a.view, 1480, 860)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})

	for _, page := range []string{"网络", "事件", "概览"} {
		name := workspaceNavigationLabel(page, true)
		icon := workspaceNavigationIcon(page)
		if err := view.Click(name); err != nil {
			t.Fatalf("click activity rail %q: %v", page, err)
		}
		if a.page != page {
			t.Fatalf("activity rail click selected %q, expected %q", a.page, page)
		}
		if workspaceNavigationIcon(page) != icon {
			t.Fatalf("selection swapped the %q icon instead of highlighting it", page)
		}
		if view.HasText("●") {
			t.Fatal("rail must highlight the selected icon, not replace it with a dot")
		}
	}
}
