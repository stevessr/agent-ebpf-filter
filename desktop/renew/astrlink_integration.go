package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// AstrLink publishes a same-user observer locator in ~/.astrlink/control-session.json.
// Do not persist the Windows observer token or request an operator token.
type astrLinkSession struct {
	SchemaVersion int    `json:"schema_version"`
	ControlSocket string `json:"control_socket"`
	ControlURL    string `json:"control_url"`
	ControlToken  string `json:"control_token"`
}

type astrLinkProvider struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}
type astrLinkPolicy struct {
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Detector       string `json:"detector"`
	RequestAction  string `json:"request_action"`
	ResponseAction string `json:"response_action"`
}
type astrLinkUsageTotals struct {
	Requests       int64 `json:"requests"`
	FailedRequests int64 `json:"failed_requests"`
	InputTokens    int64 `json:"input_tokens"`
	OutputTokens   int64 `json:"output_tokens"`
}
type astrLinkSnapshot struct {
	Found         bool
	Connected     bool
	Providers     []astrLinkProvider
	Policies      []astrLinkPolicy
	Usage         astrLinkUsageTotals
	UsageReady    bool
	UsageError    string
	PolicyError   string
	ProviderError string
	FetchedAt     time.Time
	Err           string
}

func astrLinkSessionPath() (string, error) {
	if override := strings.TrimSpace(os.Getenv("AGENT_RENEW_ASTRLINK_SESSION")); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("AGENT_RENEW_ASTRLINK_SESSION 必须是绝对路径")
		}
		return filepath.Clean(override), nil
	}
	home := strings.TrimSpace(os.Getenv("ASTRLINK_AGENT_HOME"))
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(home) {
		return "", errors.New("AstrLink 用户目录必须是绝对路径")
	}
	return filepath.Join(home, ".astrlink", "control-session.json"), nil
}

func readAstrLinkSession(path string) (astrLinkSession, bool, error) {
	var session astrLinkSession
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return session, false, nil
	}
	if err != nil {
		return session, false, errors.New("AstrLink 会话文件无法读取")
	}
	if !info.Mode().IsRegular() || info.Size() > 8192 {
		return session, true, errors.New("AstrLink 会话文件类型或大小异常")
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return session, true, errors.New("AstrLink 会话文件无法读取")
	}
	if err := json.Unmarshal(payload, &session); err != nil || session.SchemaVersion != 1 {
		return astrLinkSession{}, true, errors.New("AstrLink 会话格式不兼容")
	}
	if session.ControlSocket == "" && session.ControlURL == "" {
		return astrLinkSession{}, true, errors.New("AstrLink 会话缺少只读控制入口")
	}
	return session, true, nil
}

func astrLinkClient(session astrLinkSession) (*http.Client, string, string, error) {
	const shortTimeout = 2 * time.Second
	if session.ControlSocket != "" {
		if !filepath.IsAbs(session.ControlSocket) {
			return nil, "", "", errors.New("AstrLink Socket 路径无效")
		}
		fi, err := os.Lstat(session.ControlSocket)
		if err != nil || fi.Mode()&os.ModeSocket == 0 {
			return nil, "", "", errors.New("AstrLink 本地控制 Socket 不可用")
		}
		// Same-user socket authenticates as observer. Ignore any token.
		socketPath := session.ControlSocket
		transport := &http.Transport{
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: shortTimeout}).DialContext(ctx, "unix", socketPath)
			},
		}
		return &http.Client{Transport: transport, Timeout: shortTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}, "http://astrlink.local", "", nil
	}
	// Windows uses an ephemeral local control URL and observer-only token.
	// Never allow session-file contents to redirect Renew to a remote host.
	parsed, err := url.Parse(session.ControlURL)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil ||
		parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Port() == "" || (parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1") ||
		session.ControlToken == "" {
		return nil, "", "", errors.New("AstrLink 控制入口必须是带 Observer 凭据的本地回环地址")
	}
	return &http.Client{Timeout: shortTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, parsed.Scheme + "://" + parsed.Host, session.ControlToken, nil
}

func astrLinkGET(ctx context.Context, client *http.Client, base, token, path string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return errors.New("AstrLink 控制请求创建失败")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("AstrLink 控制接口暂不可用")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AstrLink 控制接口 HTTP %d", resp.StatusCode)
	}
	const maxPayload = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxPayload+1))
	if err != nil || len(raw) > maxPayload {
		return errors.New("AstrLink 控制响应过大或读取失败")
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return errors.New("AstrLink 返回的数据格式不兼容")
	}
	return nil
}

func loadAstrLinkSnapshot(ctx context.Context, locatorPath string, now time.Time) astrLinkSnapshot {
	out := astrLinkSnapshot{FetchedAt: now}
	session, found, err := readAstrLinkSession(locatorPath)
	out.Found = found
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if !found {
		return out
	}
	client, base, token, err := astrLinkClient(session)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	var health struct{ Status string `json:"status"` }
	if err := astrLinkGET(ctx, client, base, token, "/control/v1/health", &health); err != nil || health.Status != "ok" {
		out.Err = "AstrLink 网关未就绪或不可连接"
		return out
	}
	out.Connected = true

	var providers struct {
		Items []astrLinkProvider `json:"items"`
	}
	if err := astrLinkGET(ctx, client, base, token, "/control/v1/services?limit=100", &providers); err != nil {
		out.ProviderError = err.Error()
	} else {
		out.Providers = providers.Items
	}
	var policies struct {
		Items []astrLinkPolicy `json:"items"`
	}
	if err := astrLinkGET(ctx, client, base, token, "/control/v1/policies", &policies); err != nil {
		out.PolicyError = err.Error()
	} else {
		out.Policies = policies.Items
	}

	// Use only API-level aggregates. Never request /requests, /audit or
	// subscription token information. A one-hour UTC range is valid in v1.
	from := now.UTC().Truncate(time.Second).Add(-time.Hour).Format(time.RFC3339)
	to := now.UTC().Truncate(time.Second).Format(time.RFC3339)
	query := url.Values{"from": {from}, "to": {to}, "bucket": {"hour"}, "time_zone": {"Etc/UTC"}}
	var usage struct {
		Totals astrLinkUsageTotals `json:"totals"`
	}
	if err := astrLinkGET(ctx, client, base, token, "/control/v1/usage-summary?"+query.Encode(), &usage); err != nil {
		out.UsageError = err.Error()
	} else {
		out.Usage = usage.Totals
		out.UsageReady = true
	}
	return out
}

func (a *renewApp) refreshAstrLink(parent context.Context) {
	path, err := astrLinkSessionPath()
	if err != nil {
		a.update(func() { a.astrlink = astrLinkSnapshot{Err: err.Error()} })
		return
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	out := loadAstrLinkSnapshot(ctx, path, time.Now())
	if parent.Err() == nil {
		a.update(func() { a.astrlink = out })
	}
}

func (a *renewApp) pollAstrLink(ctx context.Context) {
	a.refreshAstrLink(ctx)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.refreshAstrLink(ctx)
		}
	}
}

func (a *renewApp) astrLinkView(c *ui.Context) {
	t := c.Theme()
	card(c, "AstrLink · 本地隐私网关", func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			status := "未发现"
			tone := t.TextMuted
			switch {
			case a.astrlink.Connected:
				status, tone = "Observer 已连接", t.Success
			case a.astrlink.Found:
				status, tone = "未就绪", t.Warning
			}
			statusPill(c, status, tone)
			ui.Spacer(c)
			if ui.Button(c, "刷新 AstrLink").Clicked() {
				go a.refreshAstrLink(context.Background())
			}
		})
		if !a.astrlink.Found {
			ui.Text(c, "未检测到 ~/.astrlink/control-session.json；未启动 AstrLink 不影响 Renew 运行。").TextColor(t.TextMuted)
			return
		}
		if a.astrlink.Err != "" {
			ui.Text(c, a.astrlink.Err).TextColor(t.Warning)
			return
		}
		if !a.astrlink.Connected {
			return
		}
		ui.Text(c, "使用 AstrLink 官方 Observer 控制接口，仅读取启用状态、策略摘要和聚合用量。").FontSize(11).TextColor(t.TextMuted)
		if a.astrlink.ProviderError != "" {
			ui.Text(c, "Provider 状态："+a.astrlink.ProviderError).TextColor(t.Warning)
		} else {
			active := 0
			for _, p := range a.astrlink.Providers { if p.Enabled { active++ } }
			ui.Text(c, fmt.Sprintf("API 服务：%d 个 · 已启用 %d 个", len(a.astrlink.Providers), active)).Bold()
			for i, p := range a.astrlink.Providers {
				if i >= 12 {
					ui.Text(c, "更多 API 服务已省略").FontSize(11).TextColor(t.TextMuted)
					break
				}
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).Width(220).SingleLine()
					ui.Text(c, p.Kind).Grow(1).FontSize(11).TextColor(t.TextMuted)
					if p.Enabled { statusPill(c, "启用", t.Success) } else { statusPill(c, "停用", t.TextMuted) }
				})
			}
		}
		if a.astrlink.PolicyError != "" {
			ui.Text(c, "隐私策略："+a.astrlink.PolicyError).TextColor(t.Warning)
		} else if len(a.astrlink.Policies) == 0 {
			ui.Text(c, "没有可读取的隐私策略摘要。").TextColor(t.TextMuted)
		} else {
			for _, p := range a.astrlink.Policies {
				status := "关闭"
				tone := t.TextMuted
				if p.Enabled { status, tone = "已启用", t.Success }
				ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).Width(200).Bold()
					statusPill(c, status, tone)
					ui.Text(c, fmt.Sprintf("检测 %s · 请求 %s · 响应 %s", p.Detector, p.RequestAction, p.ResponseAction)).FontSize(11).TextColor(t.TextMuted)
				})
			}
		}
		if a.astrlink.UsageReady {
			u := a.astrlink.Usage
			ui.Text(c, fmt.Sprintf("最近一小时：请求 %d · 失败 %d · 输入 %d / 输出 %d tokens", u.Requests, u.FailedRequests, u.InputTokens, u.OutputTokens)).Bold()
		} else {
			ui.Text(c, "用量统计："+a.astrlink.UsageError).TextColor(t.TextMuted)
		}
		ui.Text(c, "统计来自 AstrLink 自身的 HTTP 请求聚合，不等于 eBPF 的完整流量计数，也不提供单次请求的因果归属。").FontSize(11).TextColor(t.TextMuted)
	})
}
