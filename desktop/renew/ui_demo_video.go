package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/render"
	"github.com/egoist/mygo/ui"
)

// The demo video is not a re-drawing of the product: the composition's view is
// the application's own workspaceView, so every frame is built by the same code
// that builds the window, with representative state instead of a live backend.
const uiDemoFlag = "--render-ui-demo"
const uiDemoStillsFlag = "--ui-demo-stills"

const (
	uiDemoFPS     = 30
	uiDemoSeconds = 15   // Fast-paced 15s demo (2x speed)
	uiDemoWidth   = 1480 // the real main window's size in DIPs
	uiDemoHeight  = 860
)

// runUIDemoIfRequested handles the offline demo entry points before any
// backend, window or elevated privilege is involved.
func runUIDemoIfRequested(args []string) (bool, error) {
	out, stillsDir, requested, err := parseUIDemoArgs(args)
	if !requested {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if stillsDir != "" {
		return true, writeUIDemoStills(context.Background(), stillsDir)
	}
	return true, renderUIDemoVideo(context.Background(), out)
}

func parseUIDemoArgs(args []string) (out, stillsDir string, requested bool, err error) {
	out = "artifacts/renew-ui-demo.mp4"
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == uiDemoFlag:
			requested = true
			// Accept both --render-ui-demo <path> and --render-ui-demo=<path>:
			// a silently ignored path would write the video somewhere else.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				out = args[i]
			}
		case strings.HasPrefix(arg, uiDemoFlag+"="):
			requested = true
			out = strings.TrimPrefix(arg, uiDemoFlag+"=")
		case arg == uiDemoStillsFlag:
			requested = true
			if i+1 >= len(args) {
				return "", "", true, fmt.Errorf("%s needs a directory", uiDemoStillsFlag)
			}
			i++
			stillsDir = args[i]
		case strings.HasPrefix(arg, "-"):
			// Unknown flags are still the desktop application's business.
		default:
			positional = append(positional, arg)
		}
	}
	if !requested {
		return "", "", false, nil
	}
	if stillsDir == "" && len(positional) > 0 {
		return "", "", true, fmt.Errorf("unexpected arguments: %v", positional)
	}
	if stillsDir == "" && strings.TrimSpace(out) == "" {
		return "", "", true, errors.New("--render-ui-demo needs an output path")
	}
	return out, stillsDir, true, nil
}

// renderUIDemoVideo writes the workspace walkthrough as an MP4 through MyGo's
// render plugin: frames are painted by the GPU renderer (or the CPU renderer
// where there is none) and encoded by the system encoder.
func renderUIDemoVideo(ctx context.Context, out string) error {
	path, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	now := time.Now()
	app := newUIDemoApp(now)
	comp := uiDemoComposition(app, now, -1)
	return render.Video(ctx, comp, path, render.Options{
		Progress: func(p render.Progress) {
			if p.Frame%uiDemoFPS == 0 || p.Frame+1 == comp.Frames {
				fmt.Fprintf(os.Stderr, "[ui-demo] %d/%d frames\n", p.Frame+1, comp.Frames)
			}
		},
	})
}

// writeUIDemoStills renders one frame of each page as a PNG so every screen of
// the walkthrough can be inspected without decoding the video.
func writeUIDemoStills(ctx context.Context, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	now := time.Now()
	for index, scene := range uiDemoScenes {
		app := newUIDemoApp(now)
		comp := uiDemoComposition(app, now, index)
		// render.Images names each frame after its number, so each page gets
		// its own directory with a single still in it.
		pattern := filepath.Join(abs, fmt.Sprintf("%02d-%s", index+1, scene.slug), "frame-%04d.png")
		if err := render.Images(ctx, comp, pattern, render.Options{From: scene.mid(), To: scene.mid() + 1}); err != nil {
			return fmt.Errorf("still %s: %w", scene.page, err)
		}
		fmt.Fprintln(os.Stderr, "[ui-demo] still", fmt.Sprintf(pattern, scene.mid()))
	}
	return nil
}

func uiDemoComposition(app *renewApp, now time.Time, fixedScene int) render.Composition {
	return render.Composition{
		Width:  uiDemoWidth,
		Height: uiDemoHeight,
		FPS:    uiDemoFPS,
		Frames: uiDemoFPS * uiDemoSeconds,
		// Scale 1 makes the video pixels match the window's DIPs, so the real
		// workspace lays out exactly as it does on a 1480x860 window.
		Scale:  1,
		Dark:   true,
		Locale: "zh-Hans",
		View: func(c *ui.Context, f render.Frame) {
			uiDemoApplyFrame(app, f, now, fixedScene)
			app.view(c)
		},
	}
}

// uiDemoScene is one page of the walkthrough. setup runs every frame and may
// change state as the scene plays, exactly as user interaction would.
type uiDemoScene struct {
	page  string
	slug  string
	frame int
	setup func(app *renewApp, frame, count int)
}

func (s uiDemoScene) mid() int { return s.frame }

var uiDemoScenes = []uiDemoScene{
	{page: "概览", slug: "overview", setup: func(app *renewApp, frame, count int) {
		// Quick navigation expansion for fast-paced demo
		if frame < count/2 {
			app.navigationOpen = false
		} else {
			app.navigationOpen = true
			app.navigationDetailed = true
		}
	}},
	{page: "事件", slug: "events", setup: func(app *renewApp, frame, count int) {
		// Fast scroll through events
		totalEvents := len(app.events)
		if totalEvents > 0 {
			progress := float32(frame) / float32(max(count, 1))
			scrollRow := int(float32(totalEvents-1) * progress)
			app.eventTable.ScrollTo(min(scrollRow, totalEvents-1), ui.Start)
			app.eventSelected = min(scrollRow, totalEvents-1)
		}
	}},
	{page: "行为", slug: "behavior", setup: func(app *renewApp, frame, count int) {
		// Behavior orbit view - keep detailed nav
		app.navigationDetailed = true
	}},
	{page: "研判", slug: "investigation", setup: func(app *renewApp, frame, count int) {
		// Quick investigation demo with pinned event
		app.inspectorPinnedID = uiDemoHighlightEventID
		app.inspectorSelectedID = uiDemoHighlightEventID
		app.inspectorOpen = true
		// Rapid tab switching
		if frame < count/3 {
			app.inspectorTab = 0
		} else if frame < count*2/3 {
			app.inspectorTab = 1
		} else {
			app.inspectorTab = 2
		}
	}},
	{page: "监控", slug: "monitoring", setup: func(app *renewApp, frame, count int) {
		// Show monitoring/config page (防护与管理)
		app.navigationDetailed = true
	}},
	{page: "规则", slug: "rules", setup: func(app *renewApp, frame, count int) {
		// Show wrapper rules page (防护与管理)
		app.navigationDetailed = true
	}},
	{page: "网络", slug: "network", setup: func(app *renewApp, frame, count int) {
		// Network connections view
		app.navigationDetailed = true
	}},
	{page: "进程", slug: "processes", setup: func(app *renewApp, frame, count int) {
		// Fast process list scroll
		totalProcesses := len(app.system.Processes)
		if totalProcesses > 0 {
			progress := float32(frame) / float32(max(count, 1))
			scrollRow := int(float32(totalProcesses-1) * progress)
			app.processTable.ScrollTo(min(scrollRow, totalProcesses-1), ui.Start)
			app.processSelected = min(scrollRow, totalProcesses-1)
		}
	}},
}

func init() {
	per := uiDemoFPS * uiDemoSeconds / len(uiDemoScenes)
	for i := range uiDemoScenes {
		uiDemoScenes[i].frame = i*per + per/2
	}
}

const uiDemoHighlightEventID = "evt-9f2c41"

// uiDemoApplyFrame advances the demo timeline: it switches pages, streams the
// event window in as if the collector were running, and keeps the sync
// timestamp fresh. Everything else on screen comes from the real views.
func uiDemoApplyFrame(app *renewApp, f render.Frame, now time.Time, fixedScene int) {
	sceneIndex := fixedScene
	per := f.Count / len(uiDemoScenes)
	var local int
	if sceneIndex >= 0 {
		local = f.Index
	} else {
		sceneIndex = f.Index / per
		if sceneIndex >= len(uiDemoScenes) {
			sceneIndex = len(uiDemoScenes) - 1
		}
		local = f.Index - sceneIndex*per
	}
	scene := uiDemoScenes[sceneIndex]

	app.page = scene.page
	// A live collector keeps filling a bounded window; reveal the sample
	// stream from the oldest row to the newest as the scene plays.
	revealed := uiDemoInitialRows + (len(uiDemoEvents)-uiDemoInitialRows)*(local+1)/max(per, 1)
	revealed = min(revealed, len(uiDemoEvents))
	if revealed != len(app.events) {
		app.events = uiDemoEventWindow(now, revealed)
		app.eventsVersion++
	}
	app.lastSync = now
	app.system.FetchedAt = now
	if scene.setup != nil {
		scene.setup(app, local, per)
	}
}

// newUIDemoApp builds the real application state with representative data. Only
// data is provided here: every pixel comes from the shipped views.
func newUIDemoApp(now time.Time) *renewApp {
	app := newRenewApp("http://127.0.0.1:8080")
	app.starting = false
	app.connected = true
	app.healthReady = true
	app.health = collectorHealth{
		CaptureHealthy:      true,
		RingbufDroppedTotal: 0,
		BackendQueueLen:     3,
		PersistQueueCap:     4096,
	}
	app.eventStreamConnected = true
	app.systemConnected = true
	app.lastSync = now
	app.navigationOpen = true
	app.navigationDetailed = true
	app.inspectorOpen = true
	app.trackedComms = []string{"claude", "codex", "node", "python3", "bash", "curl"}
	app.behaviorWindowMS = int64((5 * time.Minute).Milliseconds())
	app.events = uiDemoEventWindow(now, uiDemoInitialRows)
	app.eventsVersion = 1
	app.system = systemSnapshot{
		CPUTotal:   23.5,
		MemTotal:   33_554_432_000,
		MemUsed:    18_214_000_000,
		MemPercent: 54.3,
		DiskRead:   1_204_224,
		DiskWrite:  486_912,
		NetRecv:    9_437_184,
		NetSent:    2_318_336,
		FetchedAt:  now,
		Processes:  uiDemoProcesses,
	}
	return app
}

const uiDemoInitialRows = 4

// uiDemoEventWindow returns the newest count sample events with receive times
// spread over the live window. Times come from the frame-independent start of
// the render, so the tables, the behavior orbit, the risk counters and the
// 5-minute window all treat the events as current activity.
func uiDemoEventWindow(now time.Time, count int) []eventSummary {
	count = min(count, len(uiDemoEvents))
	out := make([]eventSummary, 0, count)
	for i := 0; i < count; i++ {
		event := uiDemoEvents[i]
		event.ReceivedAtMS = now.Add(-time.Duration(4+i*27) * time.Second).UnixMilli()
		out = append(out, event)
	}
	return out
}

// uiDemoEvents are representative compact event summaries in the backend's own
// order (newest first). They are sample data, not captured traffic.
var uiDemoEvents = []eventSummary{
	{
		EventID: "evt-9f2c41", Type: "write", Comm: "python3", Tag: "DSH",
		Target: "/home/dev/.ssh/authorized_keys", Decision: "DENY", RiskScore: 94,
		PID: 4821, PPID: 4702, RootAgentPID: 4702,
		AgentRunID: "run-7d31", ConversationID: "conv-2f80", ToolName: "Edit",
		HasAgentContext: true,
	},
	{
		EventID: "evt-8b1d02", Type: "connect", Comm: "curl", Tag: "Codex",
		Target: "203.0.113.42:8080", NetEndpoint: "203.0.113.42:8080", Network: true,
		Decision: "ALERT", RiskScore: 71,
		PID: 5011, PPID: 4903, RootAgentPID: 3120,
		AgentRunID: "run-2a90", ToolName: "Bash", HasAgentContext: true,
	},
	{
		EventID: "evt-7c33aa", Type: "openat", Comm: "claude", Tag: "Claude Code",
		Target: "/home/dev/app/.env", Decision: "ALERT", RiskScore: 66,
		PID: 4702, PPID: 3120, RootAgentPID: 4702,
		AgentRunID: "run-5c14", ConversationID: "conv-9b31", ToolName: "Read",
		HasAgentContext: true,
	},
	{
		EventID: "evt-6a90f1", Type: "TLS_PLAINTEXT", Comm: "node", Tag: "Claude Code",
		Target: "api.anthropic.com:443", NetEndpoint: "api.anthropic.com:443", Domain: "api.anthropic.com",
		Network: true, NetBytes: 18_432, Decision: "ALLOW", RiskScore: 12,
		PID: 4903, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-5d18b7", Type: "process_exec", Comm: "bash", Tag: "Codex",
		Target: "/usr/bin/git", Decision: "ALLOW", RiskScore: 8,
		PID: 5011, PPID: 4903, RootAgentPID: 3120, HasAgentContext: true,
	},
	{
		EventID: "evt-4e77c0", Type: "openat", Comm: "claude", Tag: "Claude Code",
		Target: "/home/dev/app/src/main.go", Decision: "ALLOW", RiskScore: 5,
		PID: 4702, PPID: 3120, RootAgentPID: 4702,
		ConversationID: "conv-9b31", ToolName: "Read", HasAgentContext: true,
	},
	{
		EventID: "evt-3b2e55", Type: "connect", Comm: "node", Tag: "DSH",
		Target: "api.deepseek.com:443", NetEndpoint: "api.deepseek.com:443", Domain: "api.deepseek.com",
		Network: true, NetBytes: 9_204, Decision: "ALLOW", RiskScore: 9,
		PID: 4903, PPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-2c81d9", Type: "semantic_alert", Comm: "claude", Tag: "Claude Code",
		Target: "疑似提示注入：忽略先前指令", Decision: "ALERT", RiskScore: 88,
		PID: 4702, PPID: 3120, RootAgentPID: 4702,
		ConversationID: "conv-9b31", HasAgentContext: true,
	},
	{
		EventID: "evt-1a45f3", Type: "write", Comm: "codex", Tag: "Codex",
		Target: "/home/dev/app/AGENTS.md", Decision: "ALERT", RiskScore: 61,
		PID: 3120, PPID: 1, RootAgentPID: 3120, ToolName: "Edit", HasAgentContext: true,
	},
	{
		EventID: "evt-0f93c1", Type: "openat", Comm: "python3", Tag: "DSH",
		Target: "/etc/hosts", Decision: "ALLOW", RiskScore: 22,
		PID: 4821, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	// Additional events for richer demo
	{
		EventID: "evt-e8a4b2", Type: "write", Comm: "codex", Tag: "Codex",
		Target: "/tmp/agent_cache.db", Decision: "ALLOW", RiskScore: 15,
		PID: 3120, PPID: 1, RootAgentPID: 3120, HasAgentContext: true,
	},
	{
		EventID: "evt-d7f3a1", Type: "connect", Comm: "python3", Tag: "DSH",
		Target: "185.199.108.153:443", NetEndpoint: "185.199.108.153:443", Domain: "github.com",
		Network: true, NetBytes: 45_123, Decision: "ALLOW", RiskScore: 8,
		PID: 4821, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-c6e2d0", Type: "process_exec", Comm: "bash", Tag: "Claude Code",
		Target: "/usr/bin/npm", Decision: "ALLOW", RiskScore: 12,
		PID: 5201, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-b5d1c9", Type: "openat", Comm: "npm", Tag: "Claude Code",
		Target: "/home/dev/app/package.json", Decision: "ALLOW", RiskScore: 6,
		PID: 5201, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-a4c0b8", Type: "write", Comm: "bash", Tag: "Codex",
		Target: "/home/dev/.bash_history", Decision: "ALERT", RiskScore: 55,
		PID: 5011, PPID: 4903, RootAgentPID: 3120, HasAgentContext: true,
	},
	{
		EventID: "evt-93b9a7", Type: "TLS_PLAINTEXT", Comm: "curl", Tag: "Codex",
		Target: "registry.npmjs.org:443", NetEndpoint: "registry.npmjs.org:443", Domain: "registry.npmjs.org",
		Network: true, NetBytes: 8_192, Decision: "ALLOW", RiskScore: 10,
		PID: 5011, PPID: 4903, RootAgentPID: 3120, HasAgentContext: true,
	},
	{
		EventID: "evt-82a8b6", Type: "connect", Comm: "node", Tag: "DSH",
		Target: "127.0.0.1:6379", NetEndpoint: "127.0.0.1:6379",
		Network: true, Decision: "ALLOW", RiskScore: 18,
		PID: 4903, PPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-7197a5", Type: "semantic_alert", Comm: "node", Tag: "Claude Code",
		Target: "检测到敏感 API Key 泄露", Decision: "ALERT", RiskScore: 82,
		PID: 4903, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-6086b4", Type: "openat", Comm: "claude", Tag: "Claude Code",
		Target: "/home/dev/.config/agent-ebpf-filter/runtime.json", Decision: "ALLOW", RiskScore: 32,
		PID: 4702, PPID: 3120, RootAgentPID: 4702, ConversationID: "conv-9b31", HasAgentContext: true,
	},
	{
		EventID: "evt-5975a3", Type: "process_exec", Comm: "claude", Tag: "Claude Code",
		Target: "/usr/bin/go", Decision: "ALLOW", RiskScore: 14,
		PID: 5301, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-4864b2", Type: "write", Comm: "go", Tag: "Claude Code",
		Target: "/home/dev/app/go.sum", Decision: "ALLOW", RiskScore: 7,
		PID: 5301, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-3753a1", Type: "connect", Comm: "go", Tag: "Claude Code",
		Target: "proxy.golang.org:443", NetEndpoint: "proxy.golang.org:443", Domain: "proxy.golang.org",
		Network: true, NetBytes: 124_877, Decision: "ALLOW", RiskScore: 9,
		PID: 5301, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
	{
		EventID: "evt-2642c0", Type: "openat", Comm: "codex", Tag: "Codex",
		Target: "/proc/self/maps", Decision: "ALLOW", RiskScore: 41,
		PID: 3120, PPID: 1, RootAgentPID: 3120, HasAgentContext: true,
	},
	{
		EventID: "evt-1531b9", Type: "TLS_PLAINTEXT", Comm: "python3", Tag: "DSH",
		Target: "pypi.org:443", NetEndpoint: "pypi.org:443", Domain: "pypi.org",
		Network: true, NetBytes: 33_456, Decision: "ALLOW", RiskScore: 11,
		PID: 4821, PPID: 4702, RootAgentPID: 4702, HasAgentContext: true,
	},
}

// uiDemoProcesses are representative live process rows for the 进程 page.
var uiDemoProcesses = []systemProcess{
	{PID: 4821, PPID: 4702, Name: "python3", CPU: 12.7, MemPercent: 0.9, User: "dev", Cmdline: "python3 -m http.server 8000"},
	{PID: 4903, PPID: 4702, Name: "node", CPU: 6.1, MemPercent: 2.4, User: "dev", Cmdline: "node /home/dev/app/server.js"},
	{PID: 4702, PPID: 3120, Name: "claude", CPU: 3.4, MemPercent: 1.8, User: "dev", Cmdline: "claude --resume"},
	{PID: 3120, PPID: 1, Name: "codex", CPU: 1.2, MemPercent: 1.1, User: "dev", Cmdline: "codex"},
	{PID: 5011, PPID: 4903, Name: "curl", CPU: 0.4, MemPercent: 0.1, User: "dev", Cmdline: "curl -sS https://api.anthropic.com/v1/models"},
	{PID: 5201, PPID: 4702, Name: "npm", CPU: 8.3, MemPercent: 1.4, User: "dev", Cmdline: "npm install @anthropic-ai/sdk"},
	{PID: 5301, PPID: 4702, Name: "go", CPU: 15.2, MemPercent: 3.1, User: "dev", Cmdline: "go run ./backend/app"},
	{PID: 5401, PPID: 1, Name: "agent-ebpf-filter", CPU: 2.8, MemPercent: 0.7, User: "root", Cmdline: "/usr/local/bin/agent-ebpf-filter"},
	{PID: 5501, PPID: 3120, Name: "bash", CPU: 0.2, MemPercent: 0.3, User: "dev", Cmdline: "bash -c git log --oneline"},
	{PID: 5601, PPID: 5501, Name: "git", CPU: 0.5, MemPercent: 0.2, User: "dev", Cmdline: "git log --oneline"},
	{PID: 5701, PPID: 1, Name: "systemd-resolved", CPU: 0.1, MemPercent: 0.4, User: "systemd-resolve", Cmdline: "/lib/systemd/systemd-resolved"},
	{PID: 5801, PPID: 4903, Name: "redis-server", CPU: 1.1, MemPercent: 0.8, User: "redis", Cmdline: "redis-server *:6379"},
}
