package main

import "github.com/egoist/mygo/ui"

// Each manager owns its connection contract and state. Keep this page
// presentation-only; adding another adapter does not alter privileged eBPF.
func (a *renewApp) integrationsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "管理软件联动").FontSize(28).Bold()
	ui.Text(c, "内置隐私过滤可独立启用；外部管理软件联动保持只读，未安装时自动降级。").TextColor(t.TextMuted)
	a.privacyGatewayView(c)
	a.astrLinkView(c)
	a.ccsView(c)
	a.ccrView(c)
	a.antigravityView(c)
}
