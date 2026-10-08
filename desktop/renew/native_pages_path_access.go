package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type commonSensitivePath struct {
	Name string
	Path string
	Note string
}

// File presets are suggestions, not active policies. Exact file paths
// deliberately exclude directories, globs and global basename matching.
func commonSensitivePaths(home string) []commonSensitivePath {
	home = filepath.Clean(home)
	rel := func(path string) string { return filepath.Join(home, path) }
	return []commonSensitivePath{
		{"SSH Ed25519 私钥", rel(".ssh/id_ed25519"), "建议按需阻止读取"},
		{"SSH RSA 私钥", rel(".ssh/id_rsa"), "建议按需阻止读取"},
		{"SSH 客户端配置", rel(".ssh/config"), "防止未经授权地修改连接配置"},
		{"AWS 凭据", rel(".aws/credentials"), "包含长期凭据"},
		{"Kubernetes 凭据", rel(".kube/config"), "包含集群访问上下文"},
		{"GitHub CLI 凭据", rel(".config/gh/hosts.yml"), "可能包含访问令牌"},
		{"Google Cloud 凭据", rel(".config/gcloud/application_default_credentials.json"), "应用默认凭据"},
		{"Git HTTPS 凭据", rel(".git-credentials"), "可能含明文访问令牌"},
		{"npm 用户配置", rel(".npmrc"), "可能包含包仓库令牌"},
		{"系统主机映射", "/etc/hosts", "更改可能影响网络解析"},
		{"系统账户配置", "/etc/passwd", "全局规则可能影响系统服务"},
		{"系统身份认证", "/etc/shadow", "危险：拒绝读取可能破坏系统登录"},
		{"sudo 授权配置", "/etc/sudoers", "危险：拒绝读取可能影响提权"},
		{"SSH 服务端配置", "/etc/ssh/sshd_config", "更改可能影响远程登录"},
	}
}

func pathAccessBits(mode string) (bool, bool, error) {
	switch mode {
	case "阻止读取": return true, false, nil
	case "阻止写入": return false, true, nil
	case "阻止读写": return true, true, nil
	default: return false, false, fmt.Errorf("请选择读写限制")
	}
}

func activePathRule(rules []fileAccessRule, path string) (fileAccessRule, bool) {
	for _, r := range rules {
		if r.Path == path { return r, true }
	}
	return fileAccessRule{}, false
}

// Reading host authentication and privilege configuration files is likely
// to disrupt login or system recovery. Require an explicit second gesture
// in addition to the regular confirmation for these exact paths.
func needsDangerousReadConfirmation(rule fileAccessRule) bool {
	if !rule.DenyRead { return false }
	switch rule.Path {
	case "/etc/shadow", "/etc/passwd", "/etc/sudoers", "/etc/gshadow",
		"/etc/pam.d/common-auth", "/etc/nsswitch.conf":
		return true
	default:
		return false
	}
}

func (a *renewApp) pathAccessView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "路径访问与读写权限").FontSize(28).Bold()
	ui.Text(c, "BPF LSM 精确文件路径策略：只在显式确认后阻止打开、读取、写入以及相关 mmap 访问。").TextColor(t.TextMuted)
	ui.Column(c).Padding(14).Gap(7).Radius(12).Background(t.Warning.Alpha(0.07)).Border(1, t.Warning.Alpha(0.3)).Children(func() {
		ui.Text(c, "注意：当前为全系统范围，不只限制 Agent 进程。").Bold()
		ui.Text(c, "任何使用该精确文件路径的进程都可能受影响，包括系统服务。不会递归匹配目录、不会自动保护同名文件，也不覆盖重命名、删除、chmod 或已经建立的映射。请优先在测试环境验证。").FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, "预设默认不生效。策略管理关闭、LSM 未挂载或未连接时不可下发规则。").FontSize(11).TextColor(t.TextMuted)
	})
	if !a.pathAccessLoaded && !a.pathAccessBusy && a.client != nil {
		a.pathAccessLoaded = true
		go a.refreshPathAccess()
	}
	if a.pathAccessErr != "" { ui.Text(c, a.pathAccessErr).TextColor(t.Danger) }
	if a.pathAccessLoaded && !a.pathAccessState.PathAccessSupported {
		ui.Text(c, "当前内核 LSM 尚未确认支持精确路径读写权限；在获得新程序挂载确认前不会启用该功能。").TextColor(t.Warning)
	}
	if a.pathAccessBusy { ui.Spinner(c) }
	card(c, "常见敏感文件 · 建议项（全部默认关闭）", func() {
		home, err := os.UserHomeDir()
		if err != nil || !filepath.IsAbs(home) {
			ui.Text(c, "无法读取当前用户主目录，请手动填写绝对路径。").TextColor(t.Warning)
			return
		}
		for _, suggestion := range commonSensitivePaths(home) {
			s := suggestion
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, s.Name).Bold()
					ui.Text(c, s.Path).Font("monospace").FontSize(10).TextColor(t.TextMuted).SingleLine()
				})
				if rule, ok := activePathRule(a.pathAccessState.PathAccessRules, s.Path); ok {
					statusPill(c, accessLabel(rule), t.Warning)
				} else {
					statusPill(c, "未启用", t.TextMuted)
				}
				if ui.Button(c, "填入").Clicked() {
					a.pathAccessTarget = s.Path
				}
			})
		}
	})
	card(c, "设置精确文件权限", func() {
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.pathAccessTarget).Placeholder("/home/user/.ssh/id_ed25519").Label("绝对文件路径").Grow(1)
			if ui.Button(c, "选择文件…").Clicked() { a.pickAccessFile() }
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Select(c, &a.pathAccessMode, []string{"阻止读取", "阻止写入", "阻止读写"}).Label("限制操作").Width(190)
			if ui.PrimaryButton(c, "检查并确认").Clicked() && !a.pathAccessBusy {
				read, write, err := pathAccessBits(a.pathAccessMode)
				path := filepath.Clean(strings.TrimSpace(a.pathAccessTarget))
				if err != nil || !filepath.IsAbs(path) || path == "/" || len(path) >= 256 {
					a.pathAccessErr = "请输入长度小于 256 字节的绝对文件路径"
				} else if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
					a.pathAccessErr = "请先在「采集与能力」开启策略管理"
				} else if !a.pathAccessState.PathAccessSupported {
					a.pathAccessErr = "当前内核程序未确认精确路径访问控制能力"
				} else {
					a.pathAccessErr = ""
					a.pathAccessPending = fileAccessRule{Path: path, DenyRead: read, DenyWrite: write}
					a.pathAccessConfirmText = ""
					a.pathAccessConfirm = true
				}
			}
		})
	})
	card(c, fmt.Sprintf("内核已配置的文件权限 · %d", len(a.pathAccessState.PathAccessRules)), func() {
		if len(a.pathAccessState.PathAccessRules) == 0 {
			ui.Text(c, "当前未设置文件读写阻断规则。").TextColor(t.TextMuted)
		}
		for _, row := range a.pathAccessState.PathAccessRules {
			entry := row
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, entry.Path).Font("monospace").Grow(1).SingleLine()
				statusPill(c, accessLabel(entry), t.Warning)
				if ui.Button(c, "撤销…").Clicked() && !a.pathAccessBusy {
					a.pathAccessPending = fileAccessRule{Path: entry.Path}
					a.pathAccessConfirmText = ""
					a.pathAccessConfirm = true
				}
			})
		}
		if ui.Button(c, "刷新内核状态").Clicked() && !a.pathAccessBusy { go a.refreshPathAccess() }
	})
	ui.Modal(c, &a.pathAccessConfirm, func() {
		r := a.pathAccessPending
		ui.Text(c, "确认修改全系统文件读写权限").Bold()
		ui.Text(c, r.Path).Font("monospace")
		ui.Text(c, "即将设置："+accessLabel(r))
		ui.Text(c, "操作将作用于所有进程，可能导致系统工具或服务无法访问目标文件。").TextColor(t.Warning)
		if r.DenyRead || r.DenyWrite {
			ui.Text(c, "启用前要求目标是已存在的普通文件、路径不含符号链接。文件硬链接和未来的文件替换不受同一路径之外的规则覆盖。").FontSize(11).TextColor(t.TextMuted)
		}
		if needsDangerousReadConfirmation(r) {
			ui.Text(c, "高风险：此文件涉及系统登录或提权，阻止读取可能导致系统无法正常使用。请输入「确认高风险」继续。").TextColor(t.Danger)
			ui.TextInput(c, &a.pathAccessConfirmText).Placeholder("确认高风险").Label("风险确认")
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() { a.pathAccessConfirm = false }
			if ui.PrimaryButton(c, "确认写入内核策略").Clicked() {
				if needsDangerousReadConfirmation(r) && a.pathAccessConfirmText != "确认高风险" {
					a.pathAccessErr = "请输入「确认高风险」以启用系统关键文件读取限制"
				} else {
					a.pathAccessConfirm = false
					a.pathAccessConfirmText = ""
					a.applyPathAccess(r)
				}
			}
		})
	})
}

func accessLabel(rule fileAccessRule) string {
	switch {
	case rule.DenyRead && rule.DenyWrite: return "禁止读写"
	case rule.DenyRead: return "禁止读取"
	case rule.DenyWrite: return "禁止写入"
	default: return "允许读写（撤销）"
	}
}

func (a *renewApp) pickAccessFile() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: "选择需要限制读写的文件",
			Directory: false, ShowHiddenFiles: true,
		})
		a.update(func() {
			if err != nil { a.pathAccessErr = err.Error() }
			if len(paths) != 0 { a.pathAccessTarget = paths[0]; a.pathAccessErr = "" }
		})
	}()
}

func (a *renewApp) refreshPathAccess() {
	if a.client == nil { return }
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	status, err := a.client.fileAccessStatus(ctx)
	a.update(func() {
		a.pathAccessLoaded = true
		if err != nil { a.pathAccessErr = err.Error(); return }
		a.pathAccessState = status
		a.pathAccessErr = ""
	})
}

func (a *renewApp) applyPathAccess(rule fileAccessRule) {
	if a.client == nil || a.pathAccessBusy ||
		!a.runtimeCfg.Runtime.PolicyManagementEnabled || !a.pathAccessState.PathAccessSupported { return }
	a.pathAccessBusy = true
	a.pathAccessErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		err := a.client.setFileAccess(ctx, rule.Path, rule.DenyRead, rule.DenyWrite)
		var status lsmSandboxStatus
		if err == nil { status, err = a.client.fileAccessStatus(ctx) }
		if err == nil {
			got, ok := activePathRule(status.PathAccessRules, rule.Path)
			if rule.DenyRead || rule.DenyWrite {
				if !ok || got.DenyRead != rule.DenyRead || got.DenyWrite != rule.DenyWrite {
					err = fmt.Errorf("内核未确认所请求的权限位")
				}
			} else if ok { err = fmt.Errorf("规则撤销后仍然存在") }
		}
		a.update(func() {
			a.pathAccessBusy = false
			if err != nil { a.pathAccessErr = err.Error(); return }
			a.pathAccessState = status
			a.pathAccessErr = ""
		})
	}()
}
