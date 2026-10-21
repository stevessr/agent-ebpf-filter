package main

import (
	"github.com/egoist/mygo/plugins/loading"
	"github.com/egoist/mygo/ui"
)

// renewLoadingIndicator keeps native async feedback consistent with the Renew
// palette. Every label describes the actual pending operation, not collector
// health. MyGo's loading plugin respects the OS Reduce Motion preference.
func renewLoadingIndicator(c *ui.Context, label string) ui.Element {
	return loading.Ring(c, loading.Options{
		Size:  18,
		Color: c.Theme().Accent,
	}).Label(label)
}

// renewLoadingRow is used for a first-load placeholder that has its own text.
// Linear dots are visually quieter than an indeterminate progress ring.
func renewLoadingRow(c *ui.Context, label string) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		loading.LinearDots(c, loading.Options{
			Size:  16,
			Color: t.Accent,
		}).Label(label)
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
	})
}
