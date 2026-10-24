package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestActionAndStatusRemainNativeAndInteractive(t *testing.T) {
	clicks := 0
	for _, highContrast := range []bool{false, true} {
		view := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Children(func() {
				if Action(c, "刷新数据", ActionPrimary).Clicked() {
					clicks++
				}
				Action(c, "次要操作", ActionNeutral)
				Action(c, "幽灵操作", ActionGhost)
				SemanticBadge(c, "高风险", Danger)
				EmptyState(c, "无摘要", "空数据不代表没有风险")
			})
		}, 700, 400)
		view.SetPreferences(ui.Preferences{HighContrast: highContrast, ReduceMotion: true, TextScale: 1})
		for _, text := range []string{"刷新数据", "次要操作", "幽灵操作", "高风险", "无摘要", "空数据不代表没有风险"} {
			if !view.HasText(text) {
				t.Fatalf("missing %q with high contrast=%v", text, highContrast)
			}
		}
		if err := view.Click("刷新数据"); err != nil {
			t.Fatalf("native MyGo click failed: %v", err)
		}
	}
	if clicks != 2 {
		t.Fatalf("native action callback triggered %d times, want 2", clicks)
	}
}
