package design

import "github.com/egoist/mygo/ui"

// ActionVariant is an intentionally small surface API. MyGo owns all pointer,
// focus, keyboard and accessibility behavior through ButtonBase.
type ActionVariant uint8

const (
	ActionNeutral ActionVariant = iota
	ActionPrimary
	ActionGhost
)

// Action is a native clickable control whose appearance is owned by Renew.
// The returned MyGo element exposes Clicked, Tooltip, Disabled, etc.
// Avoids a second widget lifecycle, DOM bridge or dependency on Onevra.
func Action(c *ui.Context, label string, variant ActionVariant) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Padding(9, 14).Radius(10)
	bg, foreground, border := t.Surface, t.Text, t.Border
	hover, pressed := t.SurfaceHover, t.SurfacePressed
	switch variant {
	case ActionPrimary:
		bg, foreground, border = t.Accent, t.AccentText, t.Accent
		hover, pressed = t.AccentHover, t.AccentPressed
	case ActionGhost:
		bg, border = ui.Transparent, ui.Transparent
	}
	if !b.IsDisabled() {
		switch {
		case b.Pressed():
			bg = pressed
		case b.Hovered():
			bg = hover
		}
	}
	if c.Preferences().HighContrast {
		border = t.Border.Mix(t.Text, 0.5)
	}
	b.Background(bg).TextColor(foreground).Border(1, border)
	return b.Children(func() {
		ui.Text(c, label).SingleLine().TextColor(foreground).FontSize(12)
	})
}

// TintedBadge accepts a pre-resolved theme color, useful for states whose
// presentation is determined by existing, already-validated application data.
func TintedBadge(c *ui.Context, label string, color ui.Color) ui.Element {
	t := c.Theme()
	if c.Preferences().HighContrast {
		return ui.Badge(c, label).Background(t.Surface).TextColor(t.Text).Border(1, color)
	}
	return ui.Badge(c, label).Background(color.Alpha(0.13)).TextColor(color)
}

// SemanticBadge resolves risk/health labels through the current frame theme.
func SemanticBadge(c *ui.Context, label string, tone Tone) ui.Element {
	return TintedBadge(c, label, ToneColor(c.Theme(), tone))
}

// EmptyState remains visible and accessible even without network/capture data.
// Calling it never retries a backend or requests a privileged detail payload.
func EmptyState(c *ui.Context, title, explanation string) ui.Element {
	t := c.Theme()
	return ui.Column(c).MinWidth(0).Gap(6).Padding(22).Radius(12).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, title).FontSize(14).Bold()
		if explanation != "" {
			ui.Text(c, explanation).FontSize(12).TextColor(t.TextMuted)
		}
	})
}
