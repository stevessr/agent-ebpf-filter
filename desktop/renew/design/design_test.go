package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestToneColorUsesFrameTheme(t *testing.T) {
	for _, theme := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		if ToneColor(theme, Accent) != theme.Accent ||
			ToneColor(theme, Danger) != theme.Danger ||
			ToneColor(theme, Neutral) != theme.TextMuted {
			t.Fatal("semantic colors must be resolved from the frame theme")
		}
	}
}

func TestDesignComponentsRenderHeadlessly(t *testing.T) {
	view := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Children(func() {
			MetricStrip(c, []Metric{
				{Label: "摘要活动", Value: "12", Detail: "当前已加载窗口"},
				{Label: "高风险", Value: "2", Detail: "需要核查", Tone: Danger},
			})
			Section(c, "最近活动", "只显示近期摘要", func() {
				NoticeCard(c, Notice{Title: "证据边界", Message: "零告警不是安全证明", Tone: Warning})
			})
		})
	}, 960, 700)
	for _, label := range []string{
		"摘要活动", "12", "高风险", "2", "最近活动", "证据边界", "零告警不是安全证明",
	} {
		if !view.HasText(label) {
			t.Fatalf("missing visible text %q", label)
		}
	}
}
