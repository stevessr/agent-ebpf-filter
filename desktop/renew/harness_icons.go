package main

import (
	"embed"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// LobeHub static SVG marks are shipped with Renew so native monitoring never
// depends on the network, a WebView, or the React @lobehub/icons package.
//go:embed harness_icons/*.svg
var harnessIconFiles embed.FS

type harnessIconSpec struct {
	filename string
	mono     bool
}

type harnessIconAsset struct {
	svg  *ui.SVG
	mono bool
}

var harnessIconCatalog = map[string]harnessIconSpec{
	"Codex":           {"codex-color.svg", false},
	"Claude Code":     {"claudecode-color.svg", false},
	"Gemini CLI":      {"geminicli-color.svg", false},
	"DeepSeek Harness": {"deepseek-color.svg", false},
	"GitHub Copilot":  {"githubcopilot.svg", true},
	"Cursor":          {"cursor.svg", true},
	"OpenCode":        {"opencode.svg", true},
	"Pi":              {"pi.svg", true},
	"Oh My Pi":        {"pi.svg", true},
	"Kiro CLI":        {"kiro-color.svg", false},
	"Antigravity CLI": {"antigravity.svg", true},
	"MiniMax Code":    {"minimax-color.svg", false},
}

var (
	harnessIconOnce sync.Once
	harnessIconCache map[string]harnessIconAsset
)

// harnessLabelFor uses the same aliases for grouping and icon selection.
// Check the semantic tag before the executable name, but fall back to the
// latter when the tag is a generic label.
func harnessLabelFor(identities ...string) string {
	for _, identity := range identities {
		if label := harnessLabels[strings.ToLower(strings.TrimSpace(identity))]; label != "" {
			return label
		}
	}
	return "未识别"
}

func harnessIconFor(label string) (harnessIconAsset, bool) {
	harnessIconOnce.Do(func() {
		harnessIconCache = make(map[string]harnessIconAsset, len(harnessIconCatalog))
		parsed := make(map[string]*ui.SVG)
		for name, spec := range harnessIconCatalog {
			icon := parsed[spec.filename]
			if icon == nil {
				data, err := harnessIconFiles.ReadFile("harness_icons/" + spec.filename)
				if err != nil {
					continue
				}
				icon, err = ui.ParseSVG(data)
				if err != nil {
					continue
				}
				parsed[spec.filename] = icon
			}
			harnessIconCache[name] = harnessIconAsset{svg: icon, mono: spec.mono}
		}
	})
	mark, ok := harnessIconCache[label]
	return mark, ok
}

// drawHarnessIcon is intentionally render-only. Its resource cache is set up
// once and no SVG decoding, I/O, or allocation is needed for every live row.
func drawHarnessIcon(c *ui.Context, label string) {
	if mark, ok := harnessIconFor(label); ok && mark.svg != nil {
		if mark.mono {
			ui.Icon(c, mark.svg).Size(18, 18).TextColor(c.Theme().Text)
		} else {
			ui.Image(c, mark.svg).Size(18, 18)
		}
		return
	}
	// Some tools (e.g. Augment and ZCode) do not have an upstream Lobe
	// brand mark yet. Keep their identity legible rather than borrowing an
	// unrelated vendor logo or showing a broken image.
	glyph := "?"
	if label != "" && label != "未识别" {
		glyph = strings.ToUpper(string([]rune(label)[0]))
	}
	t := c.Theme()
	ui.Box(c).Size(18, 18).Radius(5).Background(t.Accent.Alpha(0.15)).Center().Children(func() {
		ui.Text(c, glyph).FontSize(10).Bold().TextColor(t.Accent)
	})
}

// harnessIdentity decorates only confirmed harness identities. Ordinary
// processes keep their original text, without implying that they are agents.
func harnessIdentity(c *ui.Context, visible string, identities ...string) {
	label := harnessLabelFor(identities...)
	if label == "未识别" {
		ui.Text(c, visible).SingleLine()
		return
	}
	ui.Row(c).Gap(7).AlignItems(ui.Center).MinWidth(0).Children(func() {
		drawHarnessIcon(c, label)
		ui.Text(c, visible).SingleLine()
	})
}
