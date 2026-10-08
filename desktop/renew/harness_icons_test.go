package main

import (
	"strings"
	"testing"
)

func TestHarnessAliasesUseConsistentIdentityForIcons(t *testing.T) {
	tests := []struct {
		ids  []string
		want string
	}{
		{[]string{"codex-code-mode-host"}, "Codex"},
		{[]string{"AI Agent", "codex"}, "Codex"},
		{[]string{"claude-code"}, "Claude Code"},
		{[]string{"gemini-cli"}, "Gemini CLI"},
		{[]string{"dsh"}, "DeepSeek Harness"},
		{[]string{"github copilot"}, "GitHub Copilot"},
		{[]string{"cursor-agent"}, "Cursor"},
		{[]string{"opencode"}, "OpenCode"},
		{[]string{"pi-coding-agent"}, "Pi"},
		{[]string{"omp"}, "Oh My Pi"},
		{[]string{"kiro-cli"}, "Kiro CLI"},
		{[]string{"auggie"}, "Augment"},
		{[]string{"agy"}, "Antigravity CLI"},
		{[]string{"zcode.appimage"}, "ZCode"},
		{[]string{"mcode"}, "MiniMax Code"},
		{[]string{"bash"}, "未识别"},
	}
	for _, tt := range tests {
		if got := harnessLabelFor(tt.ids...); got != tt.want {
			t.Errorf("harnessLabelFor(%v) = %q, want %q", tt.ids, got, tt.want)
		}
	}
	if got := eventHarnessLabel(eventSummary{Tag: "AI Agent", Comm: "codex"}); got != "Codex" {
		t.Fatalf("generic tags must not hide an executable harness: %q", got)
	}
	if got := harnessLabelFor(strings.SplitN("Claude Code · run123", " · ", 2)[0]); got != "Claude Code" {
		t.Fatalf("session display identity was not resolved: %q", got)
	}
}

func TestAllBundledHarnessIconsParseAndAreCached(t *testing.T) {
	for name, spec := range harnessIconCatalog {
		icon, ok := harnessIconFor(name)
		if !ok || icon.svg == nil {
			t.Errorf("bundled %q icon %q is unavailable or invalid", name, spec.filename)
			continue
		}
		w, h := icon.svg.Size()
		if w <= 0 || h <= 0 {
			t.Errorf("bundled %q icon has invalid size: %v×%v", name, w, h)
		}
		again, ok := harnessIconFor(name)
		if !ok || again.svg != icon.svg {
			t.Errorf("%q SVG was not cached", name)
		}
	}
	if first, ok := harnessIconFor("Pi"); !ok {
		t.Fatal("Pi mark unavailable")
	} else if other, ok := harnessIconFor("Oh My Pi"); !ok || first.svg != other.svg {
		t.Fatal("shared Pi mark must not be decoded twice")
	}
	for _, missing := range []string{"Augment", "ZCode", "未识别"} {
		if _, ok := harnessIconFor(missing); ok {
			t.Errorf("%q must use the distinct safe fallback", missing)
		}
	}
}
