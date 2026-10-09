package main

import "github.com/egoist/mygo/ui"

// styleRenewWorkspaceTheme recolors the workspace without fixing its
// appearance. MyGo's default frame theme tracks the platform appearance,
// system accent, text scale, and accessibility preferences. Apply only our
// editor-inspired shell palette after copying that live theme.
func styleRenewWorkspaceTheme(dst, system *ui.Theme, highContrast bool) {
	*dst = *system
	if system.Dark {
		dst.Background = ui.Hex("#0d111b")
		dst.Surface = ui.Hex("#151b27")
		dst.SurfaceHover = ui.Hex("#202a3b")
		dst.SurfacePressed = ui.Hex("#29364d")
		dst.Text = ui.Hex("#e8edf5")
		if !highContrast {
			dst.Border = ui.Hex("#2b3546")
			dst.TextMuted = ui.Hex("#94a1b5")
		}
	} else {
		dst.Background = ui.Hex("#f6f8fc")
		dst.Surface = ui.Hex("#ffffff")
		dst.SurfaceHover = ui.Hex("#edf2fa")
		dst.SurfacePressed = ui.Hex("#dde7f5")
		dst.Text = ui.Hex("#1c2738")
		if !highContrast {
			dst.Border = ui.Hex("#d9e1ed")
			dst.TextMuted = ui.Hex("#607187")
		}
	}
	// Native theme values for Accent, HighContrast borders (when enabled),
	// focus/selection, FontSize and Scrollbar stay untouched.
	dst.Radius = 8
}
