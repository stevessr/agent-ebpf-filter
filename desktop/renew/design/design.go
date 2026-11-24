// Package design contains reusable, product-neutral MyGo presentation
// primitives for Renew. It does not own state, navigation or backend data.
package design

import "github.com/egoist/mygo/ui"

// Tone expresses meaning rather than a hard-coded color. It inherits the
// accessibility and platform preferences of the active MyGo frame theme.
type Tone uint8

const (
	Neutral Tone = iota
	Accent
	Success
	Warning
	Danger
)

// Metric is a presentational snapshot, not a live counter or security verdict.
// Callers must identify the observation window in Detail.
type Metric struct {
	Label  string
	Value  string
	Detail string
	Tone   Tone
}

// Notice is an explanatory message with a semantic emphasis level.
type Notice struct {
	Title   string
	Message string
	Tone    Tone
}

// ToneColor resolves semantic meaning through the active frame theme.
func ToneColor(t *ui.Theme, tone Tone) ui.Color {
	switch tone {
	case Accent:
		return t.Accent
	case Success:
		return t.Success
	case Warning:
		return t.Warning
	case Danger:
		return t.Danger
	default:
		return t.TextMuted
	}
}

// MetricTile is a read-only tile: labels, values, and evidence scopes remain
// visible without hover. It deliberately does not pretend to be a button.
func MetricTile(c *ui.Context, metric Metric) ui.Element {
	t := c.Theme()
	color := ToneColor(t, metric.Tone)
	border := t.Border
	if metric.Tone != Neutral && !c.Preferences().HighContrast {
		border = border.Mix(color, .22)
	}
	return ui.Column(c).MinWidth(180).Grow(1).Padding(16).Gap(9).
		Radius(14).Background(t.Surface).Border(1, border).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(color)
			ui.Text(c, metric.Label).FontSize(12).TextColor(t.TextMuted).Grow(1)
		})
		valueColor := t.Text
		if metric.Tone == Warning || metric.Tone == Danger {
			valueColor = color
		}
		ui.Text(c, metric.Value).FontSize(27).Bold().TextColor(valueColor)
		if metric.Detail != "" {
			ui.Text(c, metric.Detail).FontSize(11).TextColor(t.TextMuted)
		}
	})
}

// MetricStrip wraps on narrow desktop windows without relying on DPI-specific
// screen breakpoints. Callers own the metric values and evidence semantics.
func MetricStrip(c *ui.Context, metrics []Metric) ui.Element {
	return ui.Row(c).Gap(12).Wrap().MinWidth(0).Children(func() {
		for _, metric := range metrics {
			MetricTile(c, metric)
		}
	})
}

// Section provides a consistent panel and optional subordinate explanation.
func Section(c *ui.Context, title, subtitle string, body func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).Gap(12).Padding(18).Radius(14).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(15).Bold()
		}
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted)
		}
		if body != nil {
			body()
		}
	})
}

// NoticeCard always spells out its meaning in text; color is supplementary.
// Empty alerts must never be conflated with successful policy enforcement.
func NoticeCard(c *ui.Context, note Notice) ui.Element {
	t := c.Theme()
	color := ToneColor(t, note.Tone)
	background := t.Surface
	if !c.Preferences().HighContrast {
		background = t.Surface.Mix(color, .065)
	}
	return ui.Row(c).Padding(13).Gap(10).Radius(10).
		Background(background).Border(1, t.Border).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(8, 8).Radius(4).Background(color)
		ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() {
			if note.Title != "" {
				ui.Text(c, note.Title).Bold().FontSize(12)
			}
			ui.Text(c, note.Message).FontSize(11).TextColor(t.TextMuted)
		})
	})
}
