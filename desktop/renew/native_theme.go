package main

import "github.com/egoist/mygo/ui"

// styleRenewWorkspaceTheme recolors the workspace without fixing its
// appearance. MyGo's default frame theme tracks the platform appearance,
// system accent, text scale, and accessibility preferences. Apply only our
// shared Web/Desktop shell palette after copying that live theme.
// See docs/design/web-desktop-ui.md for the cross-platform token contract.
func styleRenewWorkspaceTheme(dst, system *ui.Theme, highContrast bool) {
	*dst = *system
	if system.Dark {
		dst.Background = ui.Hex("#111318")
		dst.Surface = ui.Hex("#191c22")
		dst.SurfaceHover = ui.Hex("#212834")
		dst.SurfacePressed = ui.Hex("#232c3b")
		if !highContrast {
			dst.Text = ui.Hex("#e5edf9")
			dst.Border = ui.Hex("#344256")
			dst.TextMuted = ui.Hex("#98a9c1")
		}
	} else {
		dst.Background = ui.Hex("#f8fafc")
		dst.Surface = ui.Hex("#ffffff")
		dst.SurfaceHover = ui.Hex("#f1f5f9")
		dst.SurfacePressed = ui.Hex("#e9f1fe")
		if !highContrast {
			dst.Text = ui.Hex("#1f2937")
			dst.Border = ui.Hex("#d8e2f0")
			dst.TextMuted = ui.Hex("#64748b")
		}
	}
	// Native theme values for Accent, HighContrast borders (when enabled),
	// focus/selection, FontSize and Scrollbar stay untouched.
	dst.Radius = 8
}
