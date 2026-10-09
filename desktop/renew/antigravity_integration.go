package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Only typed, safe settings are projected out of Antigravity Tools gui_config.json.
// The file may also contain API keys, admin credentials, upstream proxies,
// prompts and identity details. None of these are retained in the snapshot.
type antigravitySettings struct {
	Proxy struct {
		Enabled        bool   `json:"enabled"`
		Port           int    `json:"port"`
		AllowLAN       bool   `json:"allow_lan_access"`
		AuthMode       string `json:"auth_mode"`
		LoggingEnabled bool   `json:"enable_logging"`
		AutoStart      bool   `json:"auto_start"`
	} `json:"proxy"`
	ScheduledWarmup struct {
		Enabled bool `json:"enabled"`
	} `json:"scheduled_warmup"`
	QuotaProtection struct {
		Enabled bool `json:"enabled"`
		Threshold int `json:"threshold_percentage"`
	} `json:"quota_protection"`
	CircuitBreaker struct {
		Enabled bool `json:"enabled"`
	} `json:"circuit_breaker"`
}

type antigravitySnapshot struct {
	Found      bool
	Settings   antigravitySettings
	Usage      localGatewayUsage
	UsageReady bool
	UsageError string
	FetchedAt  time.Time
	Err        string
}

const antigravityUsageSQL = "SELECT COUNT(*) AS requests, COALESCE(SUM(CASE WHEN status < 200 OR status >= 400 THEN 1 ELSE 0 END),0) AS failures, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens FROM request_logs WHERE timestamp >= CAST(strftime('%s','now') AS INTEGER) * 1000 - 3600000;"

// Antigravity Tools supports an optional relocation pointer and ABV_DATA_DIR.
// Renew resolves those without creating directories or writing pointer files.
func antigravityConfigPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENT_RENEW_ANTIGRAVITY_CONFIG")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("AGENT_RENEW_ANTIGRAVITY_CONFIG 必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if envDir := strings.TrimSpace(os.Getenv("ABV_DATA_DIR")); envDir != "" {
		if !filepath.IsAbs(envDir) {
			return "", errors.New("ABV_DATA_DIR 必须是绝对路径")
		}
		return filepath.Join(envDir, "gui_config.json"), nil
	}
	pointerFiles := []string{filepath.Join(home, ".antigravity_tools_location")}
	switch runtime.GOOS {
	case "linux":
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(configHome) {
			configHome = filepath.Join(home, ".config")
		}
		pointerFiles = append(pointerFiles, filepath.Join(configHome, "antigravity-tools", "data_dir.txt"))
	case "windows":
		if appData := os.Getenv("APPDATA"); filepath.IsAbs(appData) {
			pointerFiles = append(pointerFiles, filepath.Join(appData, "antigravity-tools", "data_dir.txt"))
		}
	case "darwin":
		pointerFiles = append(pointerFiles, filepath.Join(home, "Library", "Application Support", "antigravity-tools", "data_dir.txt"))
	}
	for _, pointer := range pointerFiles {
		regular, err := regularGatewayFile(pointer, 4096)
		if err != nil || !regular {
			continue
		}
		raw, err := os.ReadFile(pointer)
		if err != nil {
			continue
		}
		dir := strings.TrimSpace(string(raw))
		if filepath.IsAbs(dir) {
			return filepath.Join(filepath.Clean(dir), "gui_config.json"), nil
		}
	}
	return filepath.Join(home, ".antigravity_tools", "gui_config.json"), nil
}

func loadAntigravitySnapshot(ctx context.Context, configPath string) antigravitySnapshot {
	out := antigravitySnapshot{FetchedAt: time.Now()}
	found, err := regularGatewayFile(configPath, 2<<20)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.Found = found
	if !found {
		return out
	}
	if err := ctx.Err(); err != nil {
		out.Err = "Antigravity 读取已取消"
		return out
	}
	content, err := os.ReadFile(configPath)
	if err != nil || len(content) > 2<<20 {
		out.Err = "Antigravity 配置读取失败或超出大小限制"
		return out
	}
	if err := json.Unmarshal(content, &out.Settings); err != nil {
		out.Err = "Antigravity 配置格式不兼容"
		return out
	}
	// No API keys, model mappings, URLs, auth headers or account details
	// enter persistent desktop state, even when they exist in source JSON.
	out.Settings.Proxy.AuthMode = strings.TrimSpace(out.Settings.Proxy.AuthMode)
	switch out.Settings.Proxy.AuthMode {
	case "off", "strict", "auto", "all_except_health":
	default:
		out.Settings.Proxy.AuthMode = "未识别"
	}
	usagePath := filepath.Join(filepath.Dir(configPath), "proxy_logs.db")
	ready, err := regularGatewayFile(usagePath, 1<<34)
	if err != nil {
		out.UsageError = err.Error()
	} else if !ready {
		out.UsageError = "未发现本地 proxy_logs.db"
	} else {
		var rows []localGatewayUsage
		if err := managementSQLiteQuery(ctx, usagePath, antigravityUsageSQL, &rows); err != nil {
			out.UsageError = err.Error()
		} else if len(rows) == 1 {
			out.Usage, out.UsageReady = rows[0], true
		} else {
			out.UsageError = "代理用量暂不可用"
		}
	}
	return out
}

func (a *renewApp) refreshAntigravity(parent context.Context) {
	path, err := antigravityConfigPath()
	if err != nil {
		a.update(func() { a.antigravity = antigravitySnapshot{Err: err.Error()} })
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	out := loadAntigravitySnapshot(ctx, path)
	if parent.Err() == nil {
		a.update(func() { a.antigravity = out })
	}
}

func (a *renewApp) pollAntigravity(ctx context.Context) {
	a.refreshAntigravity(ctx)
	ticker := time.NewTicker(30*time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshAntigravity(ctx)
		}
	}
}

func (a *renewApp) antigravityView(c *ui.Context) {
	t := c.Theme()
	card(c, "Antigravity Tools · 多账号与代理管理", func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if a.antigravity.Err != "" {
				statusPill(c, "读取异常", t.Warning)
			} else if a.antigravity.Found {
				statusPill(c, "配置只读", t.Success)
			} else {
				statusPill(c, "未发现", t.TextMuted)
			}
			ui.Spacer(c)
			if ui.Button(c, "刷新 Antigravity").Clicked() {
				go a.refreshAntigravity(context.Background())
			}
		})
		if !a.antigravity.Found {
			ui.Text(c, "未检测到 Antigravity Tools 配置；支持迁移目录指针或 AGENT_RENEW_ANTIGRAVITY_CONFIG 覆盖。").TextColor(t.TextMuted)
			if a.antigravity.Err != "" { ui.Text(c, a.antigravity.Err).TextColor(t.Warning) }
			return
		}
		if a.antigravity.Err != "" {
			ui.Text(c, a.antigravity.Err).TextColor(t.Warning)
			return
		}
		p := a.antigravity.Settings.Proxy
		state := "未启用"
		if p.Enabled { state = "已配置启用" }
		ui.Text(c, fmt.Sprintf("反向代理：%s · 保存端口：%d · 认证模式：%s", state, p.Port, p.AuthMode)).Bold()
		ui.Text(c, fmt.Sprintf("局域网访问：%s · 请求日志：%s · 自动启动：%s",
			yesNo(p.AllowLAN), yesNo(p.LoggingEnabled), yesNo(p.AutoStart)))
		ui.Text(c, fmt.Sprintf("配额保护：%s（阈值 %d%%） · 熔断：%s · 预热：%s",
			yesNo(a.antigravity.Settings.QuotaProtection.Enabled),
			a.antigravity.Settings.QuotaProtection.Threshold,
			yesNo(a.antigravity.Settings.CircuitBreaker.Enabled),
			yesNo(a.antigravity.Settings.ScheduledWarmup.Enabled)))
		if a.antigravity.UsageReady {
			u := a.antigravity.Usage
			ui.Text(c, fmt.Sprintf("最近一小时：请求 %d · 失败 %d · 输入 %d / 输出 %d tokens", u.Requests, u.Failures, u.InputTokens, u.OutputTokens))
		} else {
			ui.Text(c, "用量统计："+a.antigravity.UsageError).TextColor(t.TextMuted)
		}
		ui.Text(c, "这里展示的是持久化配置，而非实时代理存活状态；不会读取代理 Key、账户资料或请求正文。").FontSize(11).TextColor(t.TextMuted)
		if ui.Button(c, "查看 Antigravity 进程事件").Clicked() {
			a.clearEventFilters()
			a.managementEventFilter = "antigravity"
			a.eventReturnPage = "CCS"
			a.page = "事件"
		}
	})
}

func yesNo(ok bool) string {
	if ok { return "开启" }
	return "关闭"
}
