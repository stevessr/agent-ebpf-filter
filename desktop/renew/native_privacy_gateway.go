package main

import (
    "context"
    "strings"
    "time"

    "github.com/egoist/mygo/ui"
)

// privacyGatewayView is first-party policy management. AstrLink remains a
// separate read-only integration and is never required for this feature.
func (a *renewApp) privacyGatewayView(c *ui.Context) {
    t := c.Theme()
    card(c, "内置 AI 隐私过滤网关", func() {
        ui.Text(c, "对经本机 Domain Forward Proxy 路由的 AI HTTP 请求进行本地敏感信息检测、阻断或占位符脱敏。").TextColor(t.TextMuted)
        ui.Text(c, "默认关闭；不会自动接管应用网络，也不会自行开启 TLS 解密或安装证书。").FontSize(11).TextColor(t.TextMuted)
        if !a.runtimeReady {
            ui.Text(c, "等待读取后端 Runtime 配置…").TextColor(t.TextMuted)
            return
        }
        cfg := a.runtimeCfg.Runtime.DomainForwardProxy
        ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
            if !cfg.Enabled {
                statusPill(c, "代理尚未启用", t.Warning)
            } else if cfg.Privacy.Enabled {
                statusPill(c, "隐私策略已开启", t.Success)
            } else {
                statusPill(c, "隐私策略已关闭", t.TextMuted)
            }
            ui.Spacer(c)
            if ui.Button(c, "刷新配置").Clicked() && !a.configBusy {
                go a.refreshRuntimeConfig(context.Background())
            }
        })
        enabled := cfg.Privacy.Enabled
        if ui.Switch(c, &enabled).Label("检查并保护经过代理的请求").Changed() && !a.configBusy {
            a.updatePrivacyGateway("enabled", enabled)
        }
        action := strings.ToLower(cfg.Privacy.Action)
        if action != "redact" && action != "block" && action != "warn" { action = "block" }
        ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
            ui.Text(c, "命中动作")
            if ui.Select(c, &action, []string{"redact", "block", "warn"}).Label("请求处理策略").Changed() && !a.configBusy {
                a.updatePrivacyGateway("action", action)
            }
        })
        restore := cfg.Privacy.RestoreResponse
        if ui.Switch(c, &restore).Label("在本地响应中还原脱敏占位符").Changed() && !a.configBusy {
            a.updatePrivacyGateway("restoreResponse", restore)
        }
        ui.Text(c, "检测规则：API Token、邮箱、电话号码；更多分类和精确白名单请使用 WebUI 的运行时网络配置。").FontSize(11).TextColor(t.TextMuted)
        ui.Text(c, "SSE 支持按完整事件还原；WebSocket 隐私检查尚未实现，开启防护时会拒绝该类连接。").FontSize(11).TextColor(t.TextMuted)
        if action == "warn" {
            ui.Text(c, "注意：warn 仅检测，不会脱敏，原始敏感信息仍然可能发送至上游。").TextColor(t.Warning)
        }
        if a.configBusy {
            ui.Text(c, "正在等待后端确认策略更新…").TextColor(t.TextMuted)
        } else if a.configErr != "" {
            ui.Text(c, a.configErr).TextColor(t.Warning)
        }
    })
}

func (a *renewApp) updatePrivacyGateway(field string, value any) {
    if a.client == nil || a.configBusy || !a.runtimeReady { return }
    a.configBusy = true
    a.configErr = ""
    go func() {
        ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
        defer cancel()
        cfg, err := a.client.patchPrivacyGateway(ctx, field, value)
        a.update(func() {
            a.configBusy = false
            if err != nil {
                a.configErr = err.Error()
                return
            }
            a.applyRuntimeConfig(cfg)
        })
    }()
}
