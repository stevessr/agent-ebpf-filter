package main

// This is a noninteractive, deterministic demo of Renew's *real* workspace.
// It is intentionally a test, so the existing desktop package and all its
// unexported view methods can be reused without copying or forking the UI.
// Run with RENEW_RENDER_VIDEO=1 go test -run TestRenderRenewDesktopPromoVideo -v .

import (
    "context"
    "fmt"
    "math"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/egoist/mygo/plugins/render"
    "github.com/egoist/mygo/ui"
)

const renewPromoFPS = 24
const renewPromoFrames = 480

func promoEvents() []eventSummary {
    // Synthetic, non-sensitive evidence. Nothing here is live capture.
    base := time.Date(2026, 11, 17, 8, 15, 0, 0, time.Local).UnixMilli()
    rows := []eventSummary{
        {EventID:"demo-001", PID:3184, PPID:1800, RootAgentPID:3184, Comm:"codex", Tag:"Codex", Type:"execve", Target:"git status --short", Decision:"ALLOW", RiskScore:8, AgentRunID:"demo-codex-01", ReceivedAtMS:base+1000},
        {EventID:"demo-002", PID:3184, PPID:1800, RootAgentPID:3184, Comm:"codex", Tag:"Codex", Type:"file_open", Target:"/workspace/src/main.go", Decision:"ALLOW", RiskScore:6, AgentRunID:"demo-codex-01", ReceivedAtMS:base+2000},
        {EventID:"demo-003", PID:4192, PPID:1800, RootAgentPID:4192, Comm:"claude", Tag:"Claude Code", Type:"execve", Target:"go test ./...", Decision:"ALLOW", RiskScore:7, AgentRunID:"demo-claude-02", ReceivedAtMS:base+3000},
        {EventID:"demo-004", PID:3184, PPID:1800, RootAgentPID:3184, Comm:"codex", Tag:"Codex", Type:"connect", Target:"api.openai.com", Domain:"api.openai.com", NetEndpoint:"api.openai.com:443", Network:true, NetBytes:8424, Decision:"ALLOW", RiskScore:12, AgentRunID:"demo-codex-01", ReceivedAtMS:base+4000},
        {EventID:"demo-005", PID:4192, PPID:1800, RootAgentPID:4192, Comm:"claude", Tag:"Claude Code", Type:"connect", Target:"api.anthropic.com", Domain:"api.anthropic.com", Network:true, NetBytes:5172, Decision:"ALLOW", RiskScore:11, AgentRunID:"demo-claude-02", ReceivedAtMS:base+5000},
        {EventID:"demo-006", PID:5210, PPID:1800, RootAgentPID:5210, Comm:"gemini", Tag:"Gemini CLI", Type:"execve", Target:"python tests/smoke.py", Decision:"ALLOW", RiskScore:9, AgentRunID:"demo-gemini-03", ReceivedAtMS:base+6000},
        {EventID:"demo-007", PID:5210, PPID:1800, RootAgentPID:5210, Comm:"gemini", Tag:"Gemini CLI", Type:"connect", Target:"generativelanguage.googleapis.com", Domain:"generativelanguage.googleapis.com", Network:true, NetBytes:4096, Decision:"ALLOW", RiskScore:17, AgentRunID:"demo-gemini-03", ReceivedAtMS:base+7000},
        {EventID:"demo-008", PID:4244, PPID:4192, RootAgentPID:4192, Comm:"bash", Tag:"Claude Code", Type:"file_write", Target:"/workspace/config/.env", Decision:"ALERT", RiskScore:69, AgentRunID:"demo-claude-02", ReceivedAtMS:base+8000},
        {EventID:"demo-009", PID:4244, PPID:4192, RootAgentPID:4192, Comm:"curl", Tag:"Claude Code", Type:"connect", Target:"203.0.113.24", NetEndpoint:"203.0.113.24:443", Network:true, NetBytes:0, Decision:"BLOCK", RiskScore:96, AgentRunID:"demo-claude-02", ReceivedAtMS:base+9000},
        {EventID:"demo-010", PID:3184, PPID:1800, RootAgentPID:3184, Comm:"codex", Tag:"Codex", Type:"file_write", Target:"/workspace/src/handler.go", Decision:"ALLOW", RiskScore:15, AgentRunID:"demo-codex-01", ReceivedAtMS:base+10000},
        {EventID:"demo-011", PID:4244, PPID:4192, RootAgentPID:4192, Comm:"bash", Tag:"Claude Code", Type:"execve", Target:"tar -czf /tmp/archive.tar.gz /workspace", Decision:"ALERT", RiskScore:78, AgentRunID:"demo-claude-02", ReceivedAtMS:base+11000},
        {EventID:"demo-012", PID:5210, PPID:1800, RootAgentPID:5210, Comm:"gemini", Tag:"Gemini CLI", Type:"file_open", Target:"/workspace/README.md", Decision:"ALLOW", RiskScore:4, AgentRunID:"demo-gemini-03", ReceivedAtMS:base+12000},
    }
    for i := range rows {
        rows[i].HasAgentContext = true
    }
    return rows
}

func promoChime(frequency float64) *render.Sound {
    const rate = 48000
    pcm := make([]float32, rate*23/100)
    for i := range pcm {
        t := float64(i) / float64(rate)
        attack := math.Min(t/0.007, 1)
        decay := math.Exp(-17*t)
        wave := math.Sin(2*math.Pi*frequency*t)+0.3*math.Sin(2*math.Pi*frequency*2*t)
        pcm[i] = float32(0.14 * attack * decay * wave)
    }
    return render.SoundOf(rate, 1, pcm)
}

func promoTitle(c *ui.Context, f render.Frame, ending bool) {
    t := float64(f.Index)
    if ending {
        t -= 421
    }
    appear := f.Spring(ui.Snappy.Spring)
    if ending {
        local, _ := f.Shift(421, 0)
        appear = local.Spring(ui.Snappy.Spring)
    }
    fade := float32(math.Min(1, math.Max(0, t/12)))
    ui.Column(c).Fill().Background(ui.Hex("#091220")).AlignItems(ui.Center).Justify(ui.Center).Gap(24).Children(func() {
        ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
            ui.Box(c).Size(18,18).Radius(5).Background(ui.Hex("#38e5c7")).
                Shadow(0,0,26,0,ui.Hex("#38e5c7").Alpha(0.6))
            ui.Text(c,"AGENT / EBPF").FontSize(18).Bold().LetterSpacing(4).TextColor(ui.Hex("#77eada"))
        })
        ui.Text(c,"明镜高悬  ·  RENEW").FontSize(65).Bold().
            LetterSpacing(-1).TextColor(ui.Hex("#f1f7ff")).
            Translate(0, float32(48*(1-appear))).Opacity(fade)
        subtitle := "Understand every agent action."
        if ending { subtitle = "Every action visible. Every boundary accountable." }
        ui.Text(c,subtitle).FontSize(29).TextColor(ui.Hex("#a2b9d4")).
            Opacity(float32(math.Min(1, math.Max(0,(t-8)/16))))
        ui.Row(c).Gap(11).Children(func(){
            for _, v := range []string{"NATIVE UI","eBPF OBSERVABILITY","AGENT SECURITY"} {
                ui.Text(c,v).FontSize(12).LetterSpacing(2).TextColor(ui.Hex("#91dfe4")).
                    Padding(10,17).Radius(10).Background(ui.Hex("#152c3c")).
                    Border(1,ui.Hex("#244c5c"))
            }
        })
        if ending {
            ui.Text(c,"github.com/stevessr/agent-ebpf-filter").FontSize(17).
                TextColor(ui.Hex("#c6d8ed")).Padding(18,0)
        }
        ui.Text(c,"DEMONSTRATION  ·  SIMULATED EVENTS").FontSize(11).
            LetterSpacing(2).TextColor(ui.Hex("#63849b")).Padding(22,0)
    })
}

func renewDemoScene(frame int) (page, caption string, count int, nav, inspector, focus bool) {
    switch {
    case frame < 114: return "概览", "01  /  SEE THE BIG PICTURE", 3, false, false, false
    case frame < 215:
        n := 3 + (frame-114)/14
        if n > 12 { n=12 }
        return "事件", "02  /  WATCH EVERY EVENT", n, true, true, false
    case frame < 288: return "会话", "03  /  CONNECT ACTIONS TO AGENTS", 12, true, false, false
    case frame < 354: return "域名", "04  /  FOLLOW NETWORK DESTINATIONS", 12, true, false, false
    default: return "事件", "05  /  TRIAGE & RESPONSE", 12, true, true, true
    }
}

func renewPromoComposition() render.Composition {
    app := newRenewApp("http://127.0.0.1:8080")
    examples := promoEvents()
    low, high := promoChime(620), promoChime(920)
    return render.Composition{
        Width: 1920, Height: 1080,
        FPS: renewPromoFPS, Frames: renewPromoFrames,
        Scale: 1.25, Dark: true, Background: ui.Hex("#08101c"),
        View: func(c *ui.Context, f render.Frame) {
            switch f.Index {
            case 4, 50, 114, 215, 288, 354, 421:
                f.Audio(low, render.AudioOptions{Volume:0.7})
            case 159, 375:
                f.Audio(high, render.AudioOptions{Volume:0.8})
            }
            if f.Index < 50 {
                promoTitle(c,f,false)
                return
            }
            if f.Index >= 421 {
                promoTitle(c,f,true)
                return
            }
            page, caption, n, nav, inspector, focus := renewDemoScene(f.Index)
            app.page = page
            app.starting = false
            app.connected = true
            app.healthReady = true
            app.health.CaptureHealthy = true
            app.eventStreamConnected = true
            app.systemConnected = true
            app.lastSync = time.Now()
            app.navigationOpen = nav
            app.navigationDetailed = false
            app.inspectorOpen = inspector
            app.inspectorSelectedID = ""
            app.eventAttentionOnly = false
            app.eventSelected = -1
            if focus {
                app.inspectorSelectedID = "demo-009"
                if f.Index >= 389 {
                    app.eventAttentionOnly = true
                }
            }
            app.events = examples[:n]
            app.eventsVersion = uint64(1000 + f.Index)
            app.ownershipCacheValid = false

            // A macOS-style stage around the actual Renew workspace.
            // Everything inside the window body is a.workspaceView(c);
            // this is NOT a reconstructed HTML screenshot or Figma mockup.
            ui.Column(c).Fill().Background(ui.Hex("#09111f")).Padding(18,30).Gap(10).Children(func() {
                ui.Row(c).Height(38).Gap(12).AlignItems(ui.Center).Children(func(){
                    ui.Box(c).Size(9,9).Radius(5).Background(ui.Hex("#35ecc2")).
                        Shadow(0,0,16,0,ui.Hex("#35ecc2").Alpha(0.7))
                    ui.Text(c,"RENEW  /  AGENT OBSERVABILITY").FontSize(16).Bold().
                        LetterSpacing(2).TextColor(ui.Hex("#b9deed"))
                    ui.Spacer(c)
                    ui.Text(c,"NATIVE MYGO UI").FontSize(11).TextColor(ui.Hex("#87b7cf"))
                    ui.Text(c,"  •  DEMO DATA").FontSize(11).TextColor(ui.Hex("#e3bd6f"))
                })
                local := float64(f.Index - 50)
                appear := float32(math.Min(1,math.Max(0,local/15)))
                ui.Column(c).Grow(1).MinHeight(0).Radius(17).Clip().
                    Border(1,ui.Hex("#35506a")).Background(ui.Hex("#111318")).
                    Shadow(0,18,44,0,ui.RGBA(0,0,0,0.55)).
                    Opacity(appear).Translate(0,float32(24*(1-appear))).Children(func() {
                    ui.Row(c).Height(37).Padding(0,14).Gap(7).AlignItems(ui.Center).
                        Background(ui.Hex("#1c2735")).Children(func() {
                        for _, col := range []ui.Color{ui.Hex("#ff5d68"),ui.Hex("#febc4f"),ui.Hex("#42c982")} {
                            ui.Box(c).Size(10,10).Radius(6).Background(col)
                        }
                        ui.Spacer(c)
                        ui.Text(c,"明镜高悬  Renew").FontSize(12).Bold().TextColor(ui.Hex("#bccbde"))
                        ui.Spacer(c)
                        ui.Text(c,"● MONITORING").FontSize(10).TextColor(ui.Hex("#66debd"))
                    })
                    ui.Column(c).Grow(1).MinHeight(0).Children(func() {
                        app.workspaceView(c)
                    })
                })
                ui.Row(c).Height(28).Gap(12).AlignItems(ui.Center).Children(func() {
                    ui.Text(c,caption).FontSize(15).Bold().LetterSpacing(1).
                        TextColor(ui.Hex("#60e4ce"))
                    ui.Spacer(c)
                    ui.Text(c,fmt.Sprintf("%02d:%02d  /  00:20", f.Index/renewPromoFPS/60, f.Index/renewPromoFPS%60)).
                        FontSize(12).TextColor(ui.Hex("#7d99b4"))
                    ui.Text(c,"SIMULATED TELEMETRY").FontSize(11).TextColor(ui.Hex("#e6b86a"))
                })
                ui.Progress(c,float64(f.Index)/float64(renewPromoFrames)).Height(3)
            })
        },
    }
}

func TestRenderRenewDesktopPromoVideo(t *testing.T) {
    if os.Getenv("RENEW_RENDER_VIDEO") != "1" {
        t.Skip("set RENEW_RENDER_VIDEO=1 to export the MyGo-native demo video")
    }
    dst := os.Getenv("RENEW_RENDER_OUTPUT")
    if dst == "" { dst = "../../artifacts/renew-desktop-demo.mp4" }
    if err := os.MkdirAll(filepath.Dir(dst),0755); err != nil {t.Fatal(err)}
    comp := renewPromoComposition()
    err := render.Video(context.Background(),comp,dst,render.Options{
        Renderer:render.Auto,
        Encoder:render.FFmpeg,
        Codec:render.H264,
        Progress:func(p render.Progress) {
            if p.Done%48==0 || p.Done==p.Frames {
                t.Logf("%s %d/%d (%v)",p.Renderer,p.Done,p.Frames,p.Elapsed.Round(time.Second))
            }
        },
    })
    if err != nil { t.Fatalf("native render failed: %v",err) }
    t.Logf("Rendered real Renew Desktop UI to %s",dst)
}
