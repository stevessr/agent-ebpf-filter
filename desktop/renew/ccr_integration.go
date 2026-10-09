package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// localGatewayUsage is an allowlisted aggregate, not a copy of upstream request
// logs. No prompt, model input, credentials, headers or endpoints enter state.
type localGatewayUsage struct {
	Requests     int64 `json:"requests"`
	Failures     int64 `json:"failures"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// managementSQLiteQuery runs only hard-coded SELECT statements against an
// existing SQLite file. The CLI never gets write access or creates a database.
func managementSQLiteQuery(ctx context.Context, db, query string, target any) error {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SELECT ") {
		return errors.New("仅允许 SQLite SELECT 查询")
	}
	command := exec.CommandContext(ctx, "sqlite3", "-readonly", "-json", "-cmd", ".timeout 250", db, query)
	output, err := command.Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("缺少 sqlite3 命令行工具")
		}
		return errors.New("本地 SQLite 数据结构不兼容或读取失败")
	}
	if len(output) > 64*1024 {
		return errors.New("本地 SQLite 统计数据超出限制")
	}
	if len(output) == 0 {
		output = []byte("[]")
	}
	if err := json.Unmarshal(output, target); err != nil {
		return errors.New("本地 SQLite 统计结果格式无效")
	}
	return nil
}

func regularGatewayFile(path string, limit int64) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return false, errors.New("本地管理器文件不可读取、类型异常或超出大小限制")
	}
	return true, nil
}

type ccrConfigSummary struct {
	Providers         int    `json:"providers"`
	PreferredProvider string `json:"preferred_provider"`
	ConfiguredPort    int    `json:"configured_port"`
}

type ccrSnapshot struct {
	Found      bool
	UsageReady bool
	Config     ccrConfigSummary
	Usage      localGatewayUsage
	UsageError string
	FetchedAt  time.Time
	Err        string
}

const ccrConfigSummarySQL = "SELECT COALESCE(json_array_length(value_json, '$.Providers'), 0) AS providers, COALESCE(json_extract(value_json, '$.preferredProvider'), '') AS preferred_provider, COALESCE(json_extract(value_json, '$.PORT'), 0) AS configured_port FROM app_config WHERE key = 'default' LIMIT 1;"
const ccrUsageSummarySQL = "SELECT COUNT(*) AS requests, COALESCE(SUM(CASE WHEN status_code < 200 OR status_code >= 400 THEN 1 ELSE 0 END),0) AS failures, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens FROM usage_events WHERE created_at >= strftime('%Y-%m-%dT%H:%M:%SZ','now','-1 hour');"

func ccrConfigPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENT_RENEW_CCR_CONFIG_DB")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("AGENT_RENEW_CCR_CONFIG_DB 必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); filepath.IsAbs(appData) {
			return filepath.Join(appData, "claude-code-router", "config.sqlite"), nil
		}
	}
	return filepath.Join(home, ".claude-code-router", "config.sqlite"), nil
}

func loadCCRSnapshot(ctx context.Context, configDB string) ccrSnapshot {
	out := ccrSnapshot{FetchedAt: time.Now()}
	found, err := regularGatewayFile(configDB, 1<<34)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.Found = found
	if !found {
		return out
	}
	var rows []ccrConfigSummary
	if err := managementSQLiteQuery(ctx, configDB, ccrConfigSummarySQL, &rows); err != nil {
		out.Err = err.Error()
		return out
	}
	if len(rows) != 1 {
		out.Err = "CCR 配置不存在或数据库版本不兼容"
		return out
	}
	out.Config = rows[0]
	// Keep display values bounded even when a name comes from a custom config.
	if len(out.Config.PreferredProvider) > 96 {
		out.Config.PreferredProvider = out.Config.PreferredProvider[:96]
	}
	usageDB := filepath.Join(filepath.Dir(configDB), "app-data", "usage.sqlite")
	ready, err := regularGatewayFile(usageDB, 1<<34)
	if err != nil {
		out.UsageError = err.Error()
	} else if !ready {
		out.UsageError = "未检测到 CCR 用量库（可在不启用用量统计时正常使用）"
	} else {
		var usageRows []localGatewayUsage
		if err := managementSQLiteQuery(ctx, usageDB, ccrUsageSummarySQL, &usageRows); err != nil {
			out.UsageError = err.Error()
		} else if len(usageRows) == 1 {
			out.Usage, out.UsageReady = usageRows[0], true
		} else {
			out.UsageError = "CCR 用量数据尚不可用"
		}
	}
	return out
}

func (a *renewApp) refreshCCR(parent context.Context) {
	path, err := ccrConfigPath()
	if err != nil {
		a.update(func() { a.ccr = ccrSnapshot{Err: err.Error()} })
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	out := loadCCRSnapshot(ctx, path)
	if parent.Err() == nil {
		a.update(func() { a.ccr = out })
	}
}

func (a *renewApp) pollCCR(ctx context.Context) {
	a.refreshCCR(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshCCR(ctx)
		}
	}
}

func (a *renewApp) ccrView(c *ui.Context) {
	t := c.Theme()
	card(c, "Claude Code Router · 本地多模型网关", func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if a.ccr.Err != "" {
				statusPill(c, "读取异常", t.Warning)
			} else if a.ccr.Found {
				statusPill(c, "配置只读", t.Success)
			} else {
				statusPill(c, "未发现", t.TextMuted)
			}
			ui.Spacer(c)
			if ui.Button(c, "刷新 CCR").Clicked() {
				go a.refreshCCR(context.Background())
			}
		})
		if !a.ccr.Found {
			ui.Text(c, "未找到 CCR config.sqlite。支持 AGENT_RENEW_CCR_CONFIG_DB 指定数据库绝对路径。").TextColor(t.TextMuted)
			if a.ccr.Err != "" { ui.Text(c, a.ccr.Err).TextColor(t.Warning) }
			return
		}
		if a.ccr.Err != "" {
			ui.Text(c, a.ccr.Err).TextColor(t.Warning)
			return
		}
		ui.Text(c, fmt.Sprintf("已配置 Provider：%d · 首选：%s · 保存端口：%d",
			a.ccr.Config.Providers, nonempty(a.ccr.Config.PreferredProvider, "未选择"), a.ccr.Config.ConfiguredPort)).Bold()
		if a.ccr.UsageReady {
			u := a.ccr.Usage
			ui.Text(c, fmt.Sprintf("最近一小时：请求 %d · 失败 %d · 输入 %d / 输出 %d tokens",
				u.Requests, u.Failures, u.InputTokens, u.OutputTokens))
		} else {
			ui.Text(c, "用量统计："+a.ccr.UsageError).TextColor(t.TextMuted)
		}
		ui.Text(c, "仅查询 SQLite JSON 标量和 usage_events 聚合；配置端口不代表网关实际在线，也不访问密钥表或请求记录。").FontSize(11).TextColor(t.TextMuted)
		if ui.Button(c, "查看 CCR 管理进程事件").Clicked() {
			a.clearEventFilters()
			a.managementEventFilter = "ccr"
			a.eventReturnPage = "CCS"
			a.page = "事件"
		}
	})
}

func nonempty(s, fallback string) string {
	if strings.TrimSpace(s) == "" { return fallback }
	return s
}
