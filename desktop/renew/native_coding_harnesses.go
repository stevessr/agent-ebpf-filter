package main

// Coding harness launchers live entirely in the desktop user's process.
// They never send command strings to the privileged monitoring backend.
import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type codingHarnessSpec struct {
	ID      string
	Label   string
	Command string
	Args    []string
}

// Fixed executable/argument pairs: launch without a shell, interpolation,
// plugin-defined commands, or privilege elevation. A user may still select
// their normal project working directory in a regular terminal.
var codingHarnessCatalog = []codingHarnessSpec{
	{ID: "codex", Label: "Codex", Command: "codex"},
	{ID: "claude", Label: "Claude Code", Command: "claude"},
	{ID: "gemini", Label: "Gemini CLI", Command: "gemini"},
	{ID: "opencode", Label: "OpenCode", Command: "opencode"},
	{ID: "pi", Label: "Pi", Command: "pi"},
	{ID: "copilot", Label: "Copilot CLI", Command: "copilot"},
	{ID: "aider", Label: "Aider", Command: "aider"},
	{ID: "kiro", Label: "Kiro CLI", Command: "kiro-cli", Args: []string{"chat"}},
}

func resolveCodingHarness(spec codingHarnessSpec, lookup func(string) (string, error)) ([]string, error) {
	if spec.ID == "" || spec.Command == "" || strings.ContainsAny(spec.Command, "/\\") {
		return nil, errors.New("invalid coding harness executable")
	}
	program, err := lookup(spec.Command)
	if err != nil {
		return nil, fmt.Errorf("%s 未安装或不在 PATH 中（%s）", spec.Label, spec.Command)
	}
	command := make([]string, 0, 1+len(spec.Args))
	command = append(command, program)
	command = append(command, spec.Args...)
	return command, nil
}

func scanCodingHarnesses() map[string]string {
	found := make(map[string]string, len(codingHarnessCatalog)+1)
	for _, spec := range codingHarnessCatalog {
		if path, err := exec.LookPath(spec.Command); err == nil {
			found[spec.ID] = path
		}
	}
	if path, err := exec.LookPath("dsh"); err == nil {
		found["deepseek"] = path
	}
	if path, err := exec.LookPath("npx"); err == nil {
		found["npx"] = path
	}
	return found
}

func (a *renewApp) launchCodingHarness(spec codingHarnessSpec) {
	command, err := resolveCodingHarness(spec, exec.LookPath)
	if err != nil {
		a.terminalError = err.Error()
		return
	}
	pane, err := a.newTerminalPaneCommand(a.terminalWorkingDir(), command)
	if err != nil {
		a.terminalError = fmt.Sprintf("无法启动 %s：%v", spec.Label, err)
		return
	}
	a.appendTerminalTab(pane, spec.Label)
}

func (a *renewApp) codingHarnessToolbar(c *ui.Context) {
	t := c.Theme()
	if a.codingHarnessPaths == nil {
		a.codingHarnessPaths = scanCodingHarnesses()
	}
	installed := 0
	for _, spec := range codingHarnessCatalog {
		if a.codingHarnessPaths[spec.ID] != "" {
			installed++
		}
	}
	ui.Column(c).MinWidth(0).Padding(8, 12).Gap(6).
		Radius(10).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Coding Harness 快捷启动").FontSize(12).Bold()
			statusPill(c, fmt.Sprintf("CLI %d/%d", installed, len(codingHarnessCatalog)), t.Accent)
			ui.Spacer(c)
			if ui.Button(c, "刷新检测").Tooltip("重新检测 PATH 中的 CLI").Clicked() {
				a.codingHarnessPaths = scanCodingHarnesses()
			}
		})
		// MyGo's accessible toolbar overflows without squeezing the PTY on
		// smaller laptop or Wayland tiling windows.
		ui.Toolbar(c, func() {
			for _, spec := range codingHarnessCatalog {
				present := a.codingHarnessPaths[spec.ID] != ""
				label := spec.Label
				if !present {
					label += " · 未安装"
				}
				if ui.Button(c.Key(spec.ID), label).
					Tooltip("在当前工作目录创建独立 PTY；不会通过 eBPF 后端执行命令").
					Clicked() {
					a.launchCodingHarness(spec)
				}
			}
			deepseekLabel := "启动 DeepSeek Harness"
			if a.deepseekPane != nil && !a.deepseekPane.finished.Load() {
				deepseekLabel = "显示 DeepSeek Web"
			} else if a.codingHarnessPaths["deepseek"] == "" && a.codingHarnessPaths["npx"] != "" {
				deepseekLabel = "DeepSeek · npx 联网启动"
			}
			if ui.PrimaryButton(c, deepseekLabel).
				Tooltip("本地执行 dsh web；缺少 dsh 时使用 npx 官方包（可能联网下载），在本页内嵌本地 Web UI").
				Clicked() {
				a.startDeepSeekHarness()
			}
			if a.deepseekVisible && ui.Button(c, "收起网页").Clicked() {
				a.deepseekVisible = false
			}
		}).Grow(1).MinWidth(0).Label("Coding Harness 启动器")
	})
}

func newDeepSeekPort() (int, error) {
	// Allocate an ephemeral loopback port; avoid sharing the predictable
	// default 3080 with an unrelated process already listening there.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	return port, ln.Close()
}

func deepSeekURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/", port)
}

func deepSeekCommand(path string, viaNpx bool, port int) []string {
	cmd := []string{path}
	if viaNpx {
		// Explicitly labeled as a network-capable installation action in UI.
		cmd = append(cmd, "--yes", "@deepseek-ai/dsh")
	}
	return append(cmd, "web", "--no-open", "--host", "127.0.0.1", "--port", strconv.Itoa(port))
}

func (a *renewApp) startDeepSeekHarness() {
	// Detached terminals share the owner's inline WebView. Do not create a
	// webview in a different native window with the wrong ownership.
	if a.detachedParent != nil {
		a.detachedParent.startDeepSeekHarness()
		a.detachedParent.revealDockedTerminal()
		return
	}
	if a.deepseekPane != nil && !a.deepseekPane.finished.Load() {
		a.deepseekVisible = true
		a.page = "终端"
		return
	}
	if a.codingHarnessPaths == nil {
		a.codingHarnessPaths = scanCodingHarnesses()
	}
	program, viaNpx := a.codingHarnessPaths["deepseek"], false
	if program == "" {
		program = a.codingHarnessPaths["npx"]
		viaNpx = true
	}
	if program == "" {
		a.terminalError = "未找到 dsh 或 npx。请安装 Node.js 后执行 npm install -g @deepseek-ai/dsh，再点击刷新检测。"
		return
	}
	port, err := newDeepSeekPort()
	if err != nil {
		a.terminalError = fmt.Sprintf("无法分配本地端口：%v", err)
		return
	}
	pane, err := a.newTerminalPaneCommand(a.terminalWorkingDir(), deepSeekCommand(program, viaNpx, port))
	if err != nil {
		a.terminalError = fmt.Sprintf("DeepSeek Harness 启动失败：%v", err)
		return
	}
	a.appendTerminalTab(pane, "DeepSeek Harness")
	a.deepseekPane = pane
	a.deepseekPort = port
	a.deepseekVisible = true
	a.deepseekStatus = "正在等待本地 Web UI 启动…"
	a.page = "终端"
	go a.awaitDeepSeekWeb(pane, port)
}

func (a *renewApp) awaitDeepSeekWeb(pane *renewTerminalPane, port int) {
	url := deepSeekURL(port)
	client := &http.Client{
		Timeout: 900 * time.Millisecond,
		Transport: &http.Transport{
			Proxy: nil, // never send a private local harness request to a proxy
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer client.CloseIdleConnections()
	tick := time.NewTicker(350 * time.Millisecond)
	defer tick.Stop()
	timer := time.NewTimer(50 * time.Second)
	defer timer.Stop()
	for {
		if a.terminalClosing.Load() || pane.finished.Load() {
			a.update(func() {
				if a.deepseekPane == pane {
					a.deepseekStatus = "DeepSeek 进程已退出；查看终端日志后可重新启动"
				}
			})
			return
		}
		resp, err := client.Get(url)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
				a.update(func() {
					if a.deepseekPane != pane || pane.finished.Load() || a.terminalClosing.Load() {
						return
					}
					if a.deepseekWebView == nil {
						web, webErr := a.win.NewWebView(mygo.WebViewOptions{URL: url})
						if webErr != nil {
							a.deepseekStatus = fmt.Sprintf("无法创建嵌入网页：%v（Linux 需要 WebKitGTK 4.1）", webErr)
							return
						}
						a.deepseekWebView = web
					} else if webErr := a.deepseekWebView.Page().LoadURL(url); webErr != nil {
						a.deepseekStatus = fmt.Sprintf("无法加载 DeepSeek 网页：%v", webErr)
						return
					}
					a.deepseekStatus = "本地 Web UI 已就绪"
					a.deepseekVisible = true
					a.page = "终端"
				})
				return
			}
		}
		select {
		case <-tick.C:
		case <-timer.C:
			a.update(func() {
				if a.deepseekPane == pane {
					a.deepseekStatus = "等待 Web UI 超时：请检查 DeepSeek Harness 终端日志"
				}
			})
			return
		}
	}
}

func (a *renewApp) terminalWorkArea(c *ui.Context, tab *renewTerminalTab) {
	showWeb := a.detachedParent == nil && a.deepseekVisible && a.deepseekPane != nil
	if !showWeb {
		a.terminalPaneTree(c, tab)
		return
	}
	width, _ := c.Size()
	var layout ui.Element
	if width < 1150 {
		layout = ui.Column(c)
	} else {
		layout = ui.Row(c)
	}
	layout.Grow(1).MinHeight(0).MinWidth(0).Gap(10).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
			a.terminalPaneTree(c, tab)
		})
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Gap(6).Children(func() {
			t := c.Theme()
			ui.Row(c).MinHeight(34).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "DeepSeek Harness · 本地网页").FontSize(12).Bold()
				if a.deepseekPane.finished.Load() {
					statusPill(c, "进程已结束", t.Warning)
				} else {
					ui.Text(c, a.deepseekStatus).FontSize(11).TextColor(t.TextMuted).SingleLine()
				}
				ui.Spacer(c)
				if a.deepseekWebView != nil && ui.Button(c, "刷新").Clicked() {
					a.deepseekWebView.Page().Reload()
				}
				if ui.Button(c, "隐藏").Clicked() {
					a.deepseekVisible = false
				}
			})
			if a.deepseekWebView != nil && !a.deepseekPane.finished.Load() {
				ui.WebView(c, a.deepseekWebView).Grow(1).MinHeight(0).MinWidth(0).
					Radius(10).Border(1, t.Border)
			} else {
				ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
					ui.Text(c, a.deepseekStatus).FontSize(12).TextColor(t.TextMuted)
					ui.Text(c, "仅使用本机 localhost；Shell 以当前用户权限运行。").FontSize(11).
						TextColor(t.TextMuted)
				})
			}
		})
	})
}

func (a *renewApp) terminalPaneTree(c *ui.Context, tab *renewTerminalTab) {
	if tab.zoom != 0 {
		if pane := tab.root.findPane(tab.zoom); pane != nil {
			a.terminalPaneView(c, tab, pane).Grow(1).MinWidth(0).MinHeight(0)
			return
		}
		tab.zoom = 0
	}
	a.terminalNodeView(c, tab, tab.root).Grow(1).MinWidth(0).MinHeight(0)
}
