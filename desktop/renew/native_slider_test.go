package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEventDisplaySliderMaximumPreservesExpandedRows(t *testing.T) {
	for _, tc := range []struct {
		limit int
		want  float64
	}{
		{0, 500},
		{50, 500},
		{499, 500},
		{500, 500},
		{501, 550},
		{515, 550}, // locating an event can choose a non-step limit
		{550, 550},
		{1001, 1050},
	} {
		if got := eventDisplaySliderMax(tc.limit); got != tc.want {
			t.Errorf("eventDisplaySliderMax(%d) = %.0f; want %.0f", tc.limit, got, tc.want)
		}
	}
}

// Exercise the public native components themselves, not only their text
// labels, so later MyGo upgrades cannot silently break keyboard adjustment.
func TestMyGoNativeSlidersHandlePointerAndKeyboard(t *testing.T) {
	risk, rows := 0.0, 50.0
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Gap(12).Children(func() {
			ui.Slider(c.Key("test-risk"), &risk, 0, 100).Label("Risk threshold").Width(220)
			ui.StepSlider(c.Key("test-rows"), &rows, 50, 500, 50).Label("Visible rows").Width(220)
		})
	}, 320, 180)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})

	riskBounds, ok := view.Find("Risk threshold")
	if !ok {
		t.Fatal("the labelled native risk slider was not exposed")
	}
	view.ClickAt(riskBounds.X+riskBounds.W*0.75, riskBounds.Y+riskBounds.H/2)
	if risk < 60 || risk > 100 {
		t.Fatalf("click on risk slider did not apply value: %.2f", risk)
	}
	view.Key(0, ui.KeyHome)
	if risk != 0 {
		t.Fatalf("Home should reset risk slider to zero; got %.2f", risk)
	}
	view.Key(0, ui.KeyEnd)
	if risk != 100 {
		t.Fatalf("End should raise risk slider to 100; got %.2f", risk)
	}

	rowBounds, ok := view.Find("Visible rows")
	if !ok {
		t.Fatal("the labelled native stepped slider was not exposed")
	}
	view.ClickAt(rowBounds.X+rowBounds.W*0.55, rowBounds.Y+rowBounds.H/2)
	if rows < 100 || rows > 450 || int(rows)%50 != 0 {
		t.Fatalf("row slider failed to snap to a 50-row step: %.2f", rows)
	}
	before := rows
	view.Key(0, ui.KeyRight)
	if rows != before+50 {
		t.Fatalf("Right should increase row count by 50: %.0f -> %.0f", before, rows)
	}
}
