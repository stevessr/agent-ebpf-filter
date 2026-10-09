package main

import "github.com/egoist/mygo/ui"

// Each manager owns its connection contract and state. Keep this page
// presentation-only; adding another adapter does not alter privileged eBPF.
func (a *renewApp) integrationsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "管理软件联动").FontSize(28).Bold()
	ui.Text(c, "汇总本机 Agent 管理器、隐私网关与 eBPF 事件。联动全部只读，外部产品未安装时自动降级。").TextColor(t.TextMuted)
	a.astrLinkView(c)
	a.ccsView(c)
	a.ccrView(c)
	a.antigravityView(c)
}
