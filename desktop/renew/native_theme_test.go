package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRenewWorkspaceThemeFollowsSystem(t *testing.T) {
	for _, system := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		system.Accent = ui.Hex("#6543ac")
		system.FontSize = 21 // system text scaling must not be lost
		var styled ui.Theme
		styleRenewWorkspaceTheme(&styled, system, false)
		if styled.Dark != system.Dark {
			t.Fatalf("appearance was overridden: got dark=%t, want %t", styled.Dark, system.Dark)
		}
		if styled.Background == system.Background {
			t.Fatal("Renew workspace palette was not applied")
		}
		if styled.Text == styled.Background {
			t.Fatal("text must remain visible against its background")
		}
		if styled.Accent != system.Accent || styled.FontSize != system.FontSize ||
			styled.Focus != system.Focus || styled.Selection != system.Selection {
			t.Fatal("system accent, focus or text scaling was lost")
		}
		if system.Dark && (styled.Surface != ui.Hex("#191c22") || styled.Background != ui.Hex("#111318") || styled.Border != ui.Hex("#344256")) {
			t.Fatal("desktop dark palette drifted from shared Web tokens")
		}
		if !system.Dark && (styled.Surface != ui.Hex("#ffffff") || styled.Background != ui.Hex("#f8fafc") || styled.Border != ui.Hex("#d8e2f0")) {
			t.Fatal("desktop light palette drifted from shared Web tokens")
		}

		styleRenewWorkspaceTheme(&styled, system, true)
		if styled.Border != system.Border || styled.TextMuted != system.TextMuted || styled.Text != system.Text {
			t.Fatal("high-contrast borders and secondary text must stay system-controlled")
		}
	}
}

func TestRenewMirrorSVG(t *testing.T) {
	icon, err := ui.ParseSVG(renewMirrorSVGData)
	if err != nil {
		t.Fatalf("embedded mirror SVG cannot be parsed: %v", err)
	}
	w, h := icon.Size()
	if w != 64 || h != 64 || renewMirrorSVG == nil {
		t.Fatalf("unexpected mirror SVG size %v x %v", w, h)
	}
}
