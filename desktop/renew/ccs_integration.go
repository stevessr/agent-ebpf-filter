package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// CCS metadata is deliberately sourced from allowlisted, scalar columns only.
// In particular, settings_config, meta, endpoints, API keys and request bodies
// are never queried or copied into Renew's event model.
type ccsProvider struct {
	App     string `json:"app"`
	Name    string `json:"name"`
	Current int    `json:"current"`
}

type ccsProxy struct {
	App          string `json:"app"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Enabled      int    `json:"enabled"`
	ProxyEnabled int    `json:"proxy_enabled"`
}

type ccsSnapshot struct {
	Found     bool
	Path      string
	Providers []ccsProvider
	Proxies   []ccsProxy
	FetchedAt time.Time
	Err       string
}

const ccsProvidersSQL = "SELECT app_type AS app, name, is_current AS current FROM providers WHERE is_current = 1 ORDER BY app_type, name LIMIT 64;"
const ccsProxySQL = "SELECT app_type AS app, listen_address AS host, listen_port AS port, enabled, proxy_enabled FROM proxy_config ORDER BY app_type LIMIT 16;"

// CCS itself supports a customized data directory. Users can point Renew at
// that directory's database with AGENT_RENEW_CCS_DB; never create the file.
func ccsDatabasePath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENT_RENEW_CCS_DB")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("AGENT_RENEW_CCS_DB 必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cc-switch", "cc-switch.db"), nil
}

// Use the SQLite CLI in strict read-only mode: no new cgo/driver dependency,
// no read-write SQLite connection and no application-level credentials.
func ccsQuery(ctx context.Context, db, sql string, target any) error {
	command := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", "-cmd", ".timeout 250", db, sql)
	output, err := command.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("缺少 sqlite3 命令行工具；安装 sqlite3 后可启用 CCS 读取")
		}
		return fmt.Errorf("CCS 数据库只读查询失败：%w", err)
	}
	if len(output) == 0 {
		output = []byte("[]")
	}
	if err := json.Unmarshal(output, target); err != nil {
		return fmt.Errorf("CCS 数据结构不兼容：%w", err)
	}
	return nil
}

func loadCCSSnapshot(ctx context.Context, db string) ccsSnapshot {
	result := ccsSnapshot{Path: db, FetchedAt: time.Now()}
	info, err := os.Stat(db)
	if errors.Is(err, os.ErrNotExist) {
		return result
	}
	if err != nil {
		result.Err = "读取 CCS 数据库状态失败：" + err.Error()
		return result
	}
	if !info.Mode().IsRegular() {
		result.Err = "CCS 数据库路径不是普通文件"
		return result
	}
	result.Found = true
	if err := ccsQuery(ctx, db, ccsProvidersSQL, &result.Providers); err != nil {
		result.Err = err.Error()
		return result
	}
	if err := ccsQuery(ctx, db, ccsProxySQL, &result.Proxies); err != nil {
		result.Err = err.Error()
	}
	return result
}

func (a *renewApp) refreshCCS(parent context.Context) {
	path, err := ccsDatabasePath()
	if err != nil {
		a.update(func() { a.ccs = ccsSnapshot{Err: err.Error()} })
		return
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	snapshot := loadCCSSnapshot(ctx, path)
	if parent.Err() != nil {
		return
	}
	a.update(func() { a.ccs = snapshot })
}

func (a *renewApp) pollCCS(ctx context.Context) {
	a.refreshCCS(ctx)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshCCS(ctx)
		}
	}
}

// Match only observed executable identities, not child shells or remote
// provider names. This is an attribution hint, NOT proof of API routing.
func ccsAppForEvent(event eventSummary) string {
	comm := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(event.Comm), ".exe"))
	switch comm {
	case "claude", "claude-code":
		return "claude"
	case "codex":
		return "codex"
	case "gemini":
		return "gemini"
	case "grok", "grokbuild", "grok-build":
		return "grokbuild"
	case "opencode":
		return "opencode"
	case "pi":
		return "pi"
	default:
		return ""
	}
}

func ccsAppLabel(app string) string {
	switch app {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "gemini":
		return "Gemini CLI"
	case "grokbuild":
		return "Grok Build"
	case "opencode":
		return "OpenCode"
	case "pi":
		return "Pi"
	default:
		return app
	}
}

func (a *renewApp) ccsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "CC-Switch 联动").FontSize(28).Bold()
	ui.Text(c, "只读取本机 CCS 已选择的 Provider 和代理配置；按进程身份关联 eBPF 事件。不读取 API Key，不修改 CCS。").TextColor(t.TextMuted)
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if !a.ccs.Found {
			statusPill(c, "未发现 CCS 数据库", t.Warning)
		} else if a.ccs.Err != "" {
			statusPill(c, "读取失败", t.Warning)
		} else {
			statusPill(c, "只读连接", t.Success)
		}
		if ui.Button(c, "刷新 CCS").Clicked() {
			go a.refreshCCS(context.Background())
		}
	})
	if a.ccs.Err != "" {
		ui.Text(c, a.ccs.Err).TextColor(t.Warning)
	}
	if !a.ccs.Found {
		card(c, "检测说明", func() {
			ui.Text(c, "未找到默认数据库。确认 CCS 已运行过，或设置 AGENT_RENEW_CCS_DB 为其 cc-switch.db 的绝对路径后重启 Renew。").TextColor(t.TextMuted)
			ui.Text(c, "CCS 联动是可选功能，未安装 CCS 不影响 eBPF 监控。").FontSize(11).TextColor(t.TextMuted)
		})
		return
	}

	card(c, "当前 Provider（CCS 保存的选择）", func() {
		if len(a.ccs.Providers) == 0 {
			ui.Text(c, "CCS 尚未记录当前 Provider，或数据库查询尚未成功。").TextColor(t.TextMuted)
		}
		for _, item := range a.ccs.Providers {
			item := item
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Text(c, ccsAppLabel(item.App)).Width(150).Bold()
				ui.Text(c, item.Name).Grow(1).SingleLine()
				statusPill(c, "已选择", t.Accent)
			})
		}
		ui.Text(c, "仅表示 CCS 持久化的 Provider 选择；实际请求可能经过路由、故障转移或其他进程。").FontSize(11).TextColor(t.TextMuted)
	})

	card(c, "本地代理配置（不代表服务已监听）", func() {
		if len(a.ccs.Proxies) == 0 {
			ui.Text(c, "暂无可读取的代理配置。").TextColor(t.TextMuted)
		}
		for _, item := range a.ccs.Proxies {
			item := item
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, ccsAppLabel(item.App)).Width(150)
				ui.Text(c, fmt.Sprintf("%s:%d", item.Host, item.Port)).Font("monospace").Grow(1)
				if item.Enabled != 0 && item.ProxyEnabled != 0 {
					statusPill(c, "已配置路由", t.Success)
				} else {
					statusPill(c, "未启用路由配置", t.TextMuted)
				}
			})
		}
		ui.Text(c, "这里只显示存储配置，不探测端口，也不读取代理认证材料。").FontSize(11).TextColor(t.TextMuted)
	})

	card(c, "eBPF 关联事件 · 当前摘要窗口", func() {
		counts := map[string]int{}
		for _, event := range a.events {
			if app := ccsAppForEvent(event); app != "" {
				counts[app]++
			}
		}
		for _, app := range []string{"claude", "codex", "gemini", "grokbuild", "opencode", "pi"} {
			app := app
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, ccsAppLabel(app)).Width(150)
				ui.Text(c, fmt.Sprintf("%d 条摘要", counts[app])).Grow(1)
				if ui.Button(c, "查看关联事件").Clicked() {
					a.clearEventFilters()
					a.ccsEventAppFilter = app
					a.eventReturnPage = "CCS"
					a.page = "事件"
				}
			})
		}
		ui.Text(c, "关联条件是事件执行进程名；不把子进程归属、Provider 选择或端口配置当成 API 请求的因果证据。").FontSize(11).TextColor(t.TextMuted)
	})
	if !a.ccs.FetchedAt.IsZero() {
		ui.Text(c, "CCS 配置读取时间："+a.ccs.FetchedAt.Format("15:04:05")).FontSize(11).TextColor(t.TextMuted)
	}
}
