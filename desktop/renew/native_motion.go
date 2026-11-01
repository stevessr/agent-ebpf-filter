package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Renew motion presets belong to the native UI, not to the event pipeline.
// Keep transitions short and keyed by stable UI identity: an event stream can
// update many times per second and must not restart an animation per sample.
// MyGo automatically honors the OS ReduceMotion preference for these APIs.
func renewNavigationMotion() ui.Animation {
	// A critically damped spring keeps velocity on rapid sidebar toggles but
	// never overshoots into the investigator's center pane.
	return ui.Animation{Spring: ui.SpringOptions{
		Duration: 280 * time.Millisecond,
		Bounce:   0,
	}}
}

func renewRailColorTransition() ui.ElementTransition {
	return ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true}
}

func renewPageTransition() ui.ElementTransition {
	return ui.ElementTransition{
		Duration: 180 * time.Millisecond,
		Enter:    &ui.Motion{Y: 8},
		Exit:     &ui.Motion{Y: -6},
	}
}

func renewInspectorTransition() ui.ElementTransition {
	return ui.ElementTransition{
		Duration: 180 * time.Millisecond,
		Enter:    &ui.Motion{X: 16},
		Exit:     &ui.Motion{X: 12},
	}
}

func renewInspectorTabTransition() ui.ElementTransition {
	// No lingering outgoing diagnostic panel: health and security evidence
	// should never be momentarily shown after the user switches tabs.
	return ui.ElementTransition{
		Duration: 150 * time.Millisecond,
		Enter:    &ui.Motion{Y: 5},
	}
}

func renewStatusColorTransition() ui.ElementTransition {
	// Color only: status updates should not move the dashboard layout.
	return ui.ElementTransition{Duration: 220 * time.Millisecond, Colors: true}
}

func renewRevealTransition() ui.ElementTransition {
	// Use only for low-frequency UI state changes, not live event rows.
	return ui.ElementTransition{
		Duration: 160 * time.Millisecond,
		Enter:    &ui.Motion{Y: 5},
	}
}
