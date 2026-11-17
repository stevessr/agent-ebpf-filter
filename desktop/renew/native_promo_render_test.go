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
    "github.com/egoist/mygo/ui/editor"
)

const renewPromoFPS = 24
const renewPromoFrames = 696

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
        t -= 666
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


type renewPromoBeat struct {
    Start int
    Page, Caption, Detail string
    Count int
    Inspector bool
    Focus bool
}

// Explicit, stable timing: 29 seconds at 24 FPS, including opening and
// closing title cards. Every route is a native Renew workspace page.
var renewPromoBeats = []renewPromoBeat{
    {30,  "概览",       "01  /  态势总览",           "系统健康、风险汇总与一键定位",          5, false, false},
    {67,  "事件",       "02  /  eBPF 实时事件流",   "事件类型 / 决策 / 风险分数 / 时间线",    12, true, false},
    {105, "研判",       "03  /  风险研判",           "选择高风险事件、关联进程和目标",       12, false, true},
    {136, "会话",       "04  /  Agent 会话",         "会话、命令与子进程文件修改关联",       12, false, false},
    {167, "AgentSight", "05  /  AgentSight 洞察",   "模型 Token / 缓存 / 会话归因",          12, false, false},
    {200, "派生",       "06  /  Agent 派生链",       "Agent → SubAgent 与进程树",            12, false, false},
    {233, "网络",       "07  /  网络外联",           "IP、连接、流量摘要与风险聚合",         12, false, false},
    {262, "域名",       "08  /  Agent 域名监控",    "域名 / IP 归属与外联目标",              12, false, false},
    {291, "进程",       "09  /  实时进程",           "CPU、内存、PID 与 Agent 归属",         12, false, false},
    {320, "Agent 识别", "10  /  Agent 捕获范围",    "多 Agent 识别与捕获 / 监视名单",        12, false, false},
    {349, "监控",       "11  /  采集与分析设置",     "轻量 / 日常 / 深度与运行时能力",        12, false, false},
    {378, "eBPF 模块", "12  /  原生 eBPF 模块",    "插件加载状态、内核挂载信息",            12, false, false},
    {407, "规则",       "13  /  Wrapper 防护规则", "ALLOW / ALERT / BLOCK / REWRITE",      12, false, false},
    {436, "跟踪",       "14  /  跟踪对象管理",       "命令、文件路径、标签与目录范围",       12, false, false},
    {465, "路径权限",   "15  /  BPF LSM 路径保护", "敏感文件的精确读取 / 写入权限",         12, false, false},
    {494, "CCS",        "16  /  隐私与工具联动",    "请求敏感信息过滤与桌面管理器联动",     12, false, false},
    {523, "系统",       "17  /  系统诊断",           "CPU / RAM / IO / Ringbuf / 采集健康",  12, false, false},
    {552, "终端",       "18  /  本地终端",           "多标签、分屏与普通用户 Shell",         12, false, false},
    {580, "文件",       "19  /  原生文件管理",       "本地文件、目录、搜索与操作",           12, false, false},
    {608, "编辑器",     "20  /  原生代码编辑器",     "语法高亮、多标签与文件编辑",           12, false, false},
    {637, "事件",       "21  /  风险闭环",           "从高风险告警返回详细证据链",           12, true, true},
}

func renewDemoScene(frame int) renewPromoBeat {
    beat := renewPromoBeats[0]
    for _, candidate := range renewPromoBeats {
        if frame < candidate.Start { break }
        beat = candidate
    }
    return beat
}

func renewPromoFixture(a *renewApp, at time.Time) {
    // Every value is a fabricated illustration; no privileged collector,
    // filesystem mutation, network capture or PTY is started by this test.
    a.system = systemSnapshot{
        FetchedAt: at, CPUTotal: 23.7, MemTotal: 16 << 30,
        MemUsed: 6 << 30, MemPercent: 37.5,
        DiskRead: 46 << 20, DiskWrite: 19 << 20,
        NetRecv: 12 << 20, NetSent: 8 << 20,
        Processes: []systemProcess{
            {PID:3184,PPID:1800,Name:"codex",CPU:11.8,MemPercent:1.9,User:"demo",Cmdline:"codex --full-auto",CreateTime:at.Add(-4*time.Minute).UnixMilli()},
            {PID:4192,PPID:1800,Name:"claude",CPU:6.2,MemPercent:1.6,User:"demo",Cmdline:"claude --verbose",CreateTime:at.Add(-3*time.Minute).UnixMilli()},
            {PID:4244,PPID:4192,Name:"bash",CPU:2.1,MemPercent:0.3,User:"demo",Cmdline:"bash -c curl ...",CreateTime:at.Add(-2*time.Minute).UnixMilli()},
            {PID:5210,PPID:1800,Name:"gemini",CPU:4.5,MemPercent:1.1,User:"demo",Cmdline:"gemini",CreateTime:at.Add(-time.Minute).UnixMilli()},
            {PID:5520,PPID:3184,Name:"go",CPU:5.8,MemPercent:0.8,User:"demo",Cmdline:"go test ./...",CreateTime:at.Add(-time.Minute).UnixMilli()},
            {PID:5610,PPID:4192,Name:"codex",CPU:2.5,MemPercent:0.7,User:"demo",Cmdline:"codex exec",CreateTime:at.Add(-time.Minute).UnixMilli()},
        },
    }
    a.agentSightReady = true
    a.agentSightSynced = at
    a.agentSightChecked = at
    a.agentSightReport = agentSightUsageResponse{
        Source:"demo", Limit:300,
        Summary:agentSightUsageSummary{
            EventCount:68,UsageEvents:48,UnattributedEvents:2,
            InputTokens:184230,OutputTokens:35280,
            CacheReadTokens:98200,CacheCreationTokens:12500,
            Models:[]agentSightUsageModel{
                {Model:"gpt-6",UsageEvents:24,InputTokens:82300,OutputTokens:15400,CacheReadTokens:45200,CacheCreationTokens:5000},
                {Model:"claude-sonnet",UsageEvents:18,InputTokens:69700,OutputTokens:12180,CacheReadTokens:31900,CacheCreationTokens:4600},
                {Model:"gemini",UsageEvents:6,InputTokens:32230,OutputTokens:7700,CacheReadTokens:21100,CacheCreationTokens:2900},
            },
            Sessions:[]agentSightUsageSession{
                {ID:"demo-codex-01",EventCount:30,UsageEvents:24,InputTokens:82300,OutputTokens:15400,LastEventTimestamp:at.UnixMilli()},
                {ID:"demo-claude-02",EventCount:29,UsageEvents:18,InputTokens:69700,OutputTokens:12180,LastEventTimestamp:at.UnixMilli()},
                {ID:"demo-gemini-03",EventCount:9,UsageEvents:6,InputTokens:32230,OutputTokens:7700,LastEventTimestamp:at.UnixMilli()},
            },
        },
    }
    a.registryReady = true
    a.registry = registrySnapshot{
        Tags:[]string{"Codex","Claude Code","Gemini CLI"},
        Comms:[]trackedComm{
            {Comm:"codex",Tag:"Codex"},
            {Comm:"claude",Tag:"Claude Code"},
            {Comm:"gemini",Tag:"Gemini CLI"},
        },
        Paths:[]trackedPath{{Path:"/demo/agent-ebpf-filter/README.md",Tag:"Codex"}},
        Prefixes:[]trackedPrefix{{Prefix:"/demo/agent-ebpf-filter",Tag:"Codex"}},
    }
    a.agentScopesReady = true
    a.agentScopes = agentScopePolicy{
        Capture:agentScopeList{Mode:"blacklist",Entries:[]string{"systemd"}},
        Monitor:agentScopeList{Mode:"whitelist",Entries:[]string{"codex","claude","gemini"}},
    }
    a.captureScopeMode, a.monitorScopeMode = "黑名单", "白名单"
    a.captureScopeText, a.monitorScopeText = "systemd", "codex\nclaude\ngemini"
    a.configReady, a.runtimeReady, a.rulesReady = true, true, true
    a.runtimeCfg = runtimeConfigResponse{
        Runtime:runtimeSettings{
            PolicyManagementEnabled:true,
            LoopDetection:runtimeToggle{Enabled:true},
            SignalProcessing:runtimeToggle{Enabled:true},
            ResearchProcessing:runtimeToggle{Enabled:true},
            LogPersistenceEnabled:true,
            DomainForwardProxy:desktopDomainForwardSettings{
                Enabled:true,
                Privacy:desktopPrivacySettings{Enabled:true,Action:"redact",ContextMode:"redact",RestoreResponse:true},
            },
        },
        PersistedEventLogPath:"/demo/agent-ebpf-filter/events.db",
        PersistedEventLogAlive:true,
    }
    a.rules = []wrapperRule{
        {Comm:"curl",Action:"BLOCK",Priority:100,Regex:"203\\.0\\.113\\.24"},
        {Comm:"bash",Action:"ALERT",Priority:80,Regex:"tar.*workspace"},
        {Comm:"git",Action:"ALLOW",Priority:10},
        {Comm:"python",Action:"REWRITE",Priority:5,RewrittenCmd:[]string{"python3","-I"}},
    }
    a.modulesReady = true
    a.modules = []ebpfModule{
        {ID:"agent-exec-observer",Kind:"ebpf",Name:"进程执行跟踪",Description:"跟踪 exec 与进程派生",Loaded:true,Enabled:true,AttachKind:"tracepoint",AttachTarget:"sched/sched_process_exec",ProgramName:"trace_exec"},
        {ID:"agent-network-guard",Kind:"ebpf",Name:"网络连接监控",Description:"观察 Agent 外联目标与决策证据",Loaded:true,Enabled:true,AttachKind:"tracepoint",AttachTarget:"syscalls/sys_enter_connect",ProgramName:"net_guard"},
        {ID:"file-access-probe",Kind:"ebpf",Name:"文件敏感路径监视",Description:"按需诊断文件访问",Loaded:false,Enabled:false,AttachKind:"lsm",AttachTarget:"file_open",ProgramName:"file_probe"},
    }
    a.pathAccessLoaded = true
    a.pathAccessState = lsmSandboxStatus{
        PathAccessSupported:true,Available:true,Attached:true,
        PathAccessRules:[]fileAccessRule{
            {Path:"/demo/secrets/.env",DenyRead:true,DenyWrite:true},
            {Path:"/demo/keys/id_ed25519",DenyRead:true},
        },
    }
    a.pathAccessTarget = "/demo/secrets/.env"
    a.pathAccessMode = "阻止读写"
    // A fake directory and in-memory editor buffer are safe to show without
    // touching the runner filesystem. Never launch a real terminal for video.
    a.terminalInitialized = true
    a.files.dir = "/demo/agent-ebpf-filter"
    a.files.address = a.files.dir
    a.files.listLoaded = true
    a.files.items = []localFileEntry{
        {name:"backend",dir:true},{name:"desktop",dir:true},
        {name:"docs",dir:true},{name:"scripts",dir:true},
        {name:"README.md"},{name:"go.mod"},{name:"policy.go"},
    }
    code := "package main\n\n// Demo-only, synthetic policy view.\nfunc classify(event Event) Decision {\n    if event.RiskScore >= 90 {\n        return Block\n    }\n    if event.RiskScore >= 60 {\n        return Alert\n    }\n    return Allow\n}\n"
    // Give the file browser enough entries to show its real native list
    // scrolling; no files are created on disk.
    for i:=0;i<48;i++ {
        a.files.items = append(a.files.items,localFileEntry{name:fmt.Sprintf("monitoring-event-%02d.json",i)})
    }
    // A multiline read-only-like in-memory document provides enough code
    // for the real embedded MyGo editor's own vertical scrolling to be seen.
    for i:=0;i<85;i++ {
        code += fmt.Sprintf("// %03d | synthetic policy coverage: agent trace, network, filesystem\n",i+1)
    }
    buffer := editor.NewBuffer(code)
    buffer.SetLanguage(editor.LanguageFor("policy.go"))
    a.files.docs = []*localFileDocument{{
        path:"/demo/agent-ebpf-filter/policy.go",
        state:&editor.State{Buffer:buffer,WrapGuides:[]int{100},Label:"policy.go"},
        size:int64(len(code)),
    }}
    a.files.active = 0
}

// The recording pans the *actual MyGo scroll content*, never the window
// bitmap. Hold the heading briefly, glide down, then reverse toward the top.
// Each page is allowed only the distance its own native scroll can travel.
// Twenty-one deliberately different motion profiles: no two scenes have
// the same dwell, acceleration, depth, return point or easing combination.
// They only affect the real MyGo ScrollState; navigation/footer stay fixed.
type renewPromoScrollProfile struct {
    Dwell, Apex, Distance, Rewind float64
    Curve int
}

var renewPromoCurves = [...]renewPromoScrollProfile{
    {0.17,0.79, 780,0.19,0}, // overview: relaxed smoothstep
    {0.29,0.83, 940,0.10,1}, // event filters first, then fast reveal
    {0.13,0.70, 560,0.31,2}, // investigator
    {0.23,0.75, 680,0.20,3}, // sessions
    {0.09,0.84,1160,0.06,4}, // AgentSight: long continuous glide
    {0.20,0.71, 510,0.34,5}, // lineage: transition between graph modes
    {0.28,0.78, 610,0.29,0}, // network
    {0.32,0.88,1000,0.15,2}, // domain filters before table
    {0.24,0.68, 710,0.38,4}, // process table
    {0.12,0.89,1470,0.05,3}, // Agent range long form
    {0.15,0.81,1310,0.13,5}, // monitoring switches and cards
    {0.31,0.76, 680,0.42,1}, // eBPF modules
    {0.25,0.85,1050,0.25,0}, // wrapper rule form then filtered table
    {0.19,0.72, 810,0.12,4}, // tracking tabs
    {0.10,0.79, 970,0.35,3}, // precise-path rules
    {0.22,0.90, 870,0.09,5}, // privacy gateway / integrations
    {0.16,0.67, 760,0.27,2}, // diagnostics
    {0.08,0.66, 350,0.46,1}, // terminal uses its own fixed-height work area
    {0.21,0.82,1330,0.17,3}, // file manager's separate native list
    {0.14,0.74,1430,0.24,5}, // real editor viewport
    {0.27,0.87, 520,0.40,4}, // final high-risk investigation
}

func renewPromoEase(t float64, curve int) float64 {
    t = math.Max(0, math.Min(1,t))
    switch curve {
    case 0: return t*t*(3-2*t)
    case 1: return 0.5-0.5*math.Cos(math.Pi*t)
    case 2: return t*t*t*(t*(6*t-15)+10)
    case 3: return 1-math.Pow(1-t,3)
    case 4: return t*t
    default: return 1-math.Pow(1-t,4)
    }
}

func renewPromoScrollOffset(frame, start, end, scene int, maxY float32) float32 {
    if frame <= start || end <= start || maxY <= 0 {return 0}
    profile:=renewPromoCurves[scene%len(renewPromoCurves)]
    t:=float64(frame-start)/float64(end-start)
    distance:=math.Min(float64(maxY),profile.Distance)
    switch {
    case t<=profile.Dwell: return 0
    case t<profile.Apex:
        return float32(distance*renewPromoEase((t-profile.Dwell)/(profile.Apex-profile.Dwell),profile.Curve))
    default:
        reverse:=renewPromoEase((t-profile.Apex)/(1-profile.Apex),(profile.Curve+2)%6)
        return float32(distance*(1-profile.Rewind*reverse))
    }
}

func renewPromoTyped(word string, progress, from, duration float64) string {
    if progress<from {return ""}
    n:=int(math.Ceil((progress-from)/duration*float64(len(word))))
    if n<0 {n=0}
    if n>len(word) {n=len(word)}
    return word[:n]
}

// These are real native form values, filters, tabs and selections. The
// *data* is synthetic; no network/privileged API calls or disk writes
// happen in the offline renderer. Matching tables rebuild from the values.
func renewPromoResetInteraction(a *renewApp) {
    a.search=""
    a.paused=false
    a.eventRiskFilter=""
    a.eventDecisionFilter=""
    a.eventTypeFilter=""
    a.eventSessionFilter=""
    a.eventMinRiskScore=0
    a.eventAttentionOnly=false
    a.domainSearch=""
    a.domainAgentFilter=""
    a.domainRiskOnly=false
    a.domainShowIPs=false
    a.agentSearch=""
    a.ruleSearch=""
    a.registryTab=0
    a.registryFilterTag="全部标签"
    a.registryFilterStatus="全部状态"
    a.files.filter=""
    a.files.selected=""
    a.lineageMode=0
    a.inspectorTab=0
    a.terminalTabScroll.X=0
    a.navigationDetailed=false
}

func sceneEndFor(beats []renewPromoBeat, index int) int {
    end:=666
    if index+1<len(beats) {end=beats[index+1].Start}
    return end-beats[index].Start
}

func renewPromoInteraction(a *renewApp, scene int, progress float64) string {
    switch scene {
    case 0:
        if progress>0.53 {a.paused=true;return "操作：暂停桌面事件展示（采集不停止）"}
        return "操作：查看监控健康与最新告警"
    case 1:
        if progress>0.35 {a.eventRiskFilter="高风险"}
        if progress>0.69 {a.eventMinRiskScore=80}
        if a.eventMinRiskScore>0 {return "操作：高风险筛选 → 风险分 ≥ 80"}
        if a.eventRiskFilter!="" {return "操作：筛选风险等级 = 高风险"}
        return "操作：查看全部事件 → 打开风险筛选"
    case 2:
        a.inspectorPinnedID="demo-009"
        if progress>0.55 {a.inspectorTab=1;return "操作：切换研判详情标签"}
        return "操作：定位阻断事件 demo-009"
    case 3:
        a.search=renewPromoTyped("claude",progress,0.30,0.32)
        if progress>0.76 {a.sessionSelected=0}
        if a.search!="" {return "操作：搜索会话 "+a.search}
        return "操作：在顶栏搜索 Agent 会话"
    case 4:
        if progress>0.54 {a.agentSightSessionSelected=1;return "操作：选择 Claude 会话，查看 Token 归因"}
        return "操作：查看模型用量、缓存读取与创建"
    case 5:
        if progress>0.48 {a.lineageMode=1;return "操作：派生放射图 → 瀑布时间线"}
        return "操作：查看 Agent → SubAgent 放射图"
    case 6:
        a.search=renewPromoTyped("203.0.113",progress,0.36,0.34)
        if a.search!="" {return "操作：搜索网络外联目标 "+a.search}
        return "操作：按网络目标检索"
    case 7:
        if progress>0.29 {a.domainShowIPs=true}
        if progress>0.53 {a.domainRiskOnly=true}
        a.domainSearch=renewPromoTyped("203.0.113",progress,0.64,0.25)
        if a.domainSearch!="" {return "操作：筛选风险 IP "+a.domainSearch}
        if a.domainRiskOnly {return "操作：只看风险 + 显示仅 IP 外联"}
        if a.domainShowIPs {return "操作：开启 IP-only 外联显示"}
        return "操作：查看域名与 Agent 归属"
    case 8:
        a.search=renewPromoTyped("claude",progress,0.33,0.29)
        if progress>0.76 {a.processSelected=1}
        if a.search!="" {return "操作：搜索进程 "+a.search}
        return "操作：按进程名 / PID 检索"
    case 9:
        a.agentSearch=renewPromoTyped("claude",progress,0.31,0.31)
        if progress>0.72 {a.captureScopeMode="白名单";a.captureScopeText="codex, claude, gemini"}
        if progress>0.72 {return "操作：预览捕获白名单草稿（未保存）"}
        if a.agentSearch!="" {return "操作：过滤 Agent 识别 "+a.agentSearch}
        return "操作：查看捕获 / 监视名单"
    case 10:
        if progress>0.43 {
            a.disabledEventTypes=map[int]bool{}
            for _, m:=range monitoringModules {
                if m.Cost=="高" {for _, id:=range m.EventTypes {a.disabledEventTypes[id]=true}}
            }
            a.runtimeCfg.Runtime.ResearchProcessing.Enabled=false
        }
        if progress>0.75 {
            a.disabledEventTypes=map[int]bool{}
            a.runtimeCfg.Runtime.ResearchProcessing.Enabled=true
        }
        if progress>0.75 {return "操作：切换深度采集预览（模拟配置）"}
        if progress>0.43 {return "操作：切换日常采集预览（模拟配置）"}
        return "操作：查看轻量 / 日常 / 深度配置"
    case 11:
        if progress>0.44 && progress<0.75 {
            a.modulesPendingUnload="agent-network-guard"
            return "操作：展开模块卸载确认（不真正卸载）"
        }
        a.modulesPendingUnload=""
        return "操作：查看挂载状态 / 取消卸载"
    case 12:
        if progress>0.27 {a.ruleComm="curl";a.ruleAction="BLOCK";a.rulePriority="100"}
        a.ruleSearch=renewPromoTyped("curl",progress,0.57,0.25)
        if a.ruleSearch!="" {return "操作：按命令名筛选 Wrapper 规则 "+a.ruleSearch}
        if progress>0.27 {return "操作：填写阻断规则草稿（未保存）"}
        return "操作：查看 ALLOW / ALERT / BLOCK / REWRITE"
    case 13:
        if progress>0.37 {a.registryTab=3}
        if progress>0.66 {a.registryFilterTag="Codex"}
        if progress>0.66 {return "操作：切换递归路径标签页 + Codex 标签筛选"}
        if progress>0.37 {return "操作：命令跟踪 → 递归目录跟踪"}
        return "操作：查看跟踪注册表标签页"
    case 14:
        if progress>0.39 {a.pathAccessMode="阻止写入"}
        if progress>0.66 {a.pathAccessTarget="/demo/keys/id_ed25519"}
        if progress>0.66 {return "操作：预填 SSH 密钥路径（未下发策略）"}
        if progress>0.39 {return "操作：选择阻止写入（仅 UI 草稿）"}
        return "操作：查看 BPF LSM 精确文件权限"
    case 15:
        if progress>0.56 {
            a.runtimeCfg.Runtime.DomainForwardProxy.Privacy.Action="block"
            return "操作：模拟隐私代理 redaction → block 策略预览"
        }
        return "操作：查看隐私代理 redaction 保护"
    case 16:
        if progress>0.52 {
            a.system.CPUTotal=32.4
            a.system.NetRecv=18<<20
            a.system.NetSent=11<<20
            return "操作：查看实时系统指标更新（模拟快照）"
        }
        return "操作：检查采集器、队列与 I/O"
    case 17:
        if progress>0.47 {a.navigationDetailed=true;return "操作：展开桌面导航、查看终端工具栏"}
        return "操作：查看终端标签 / 分屏工具"
    case 18:
        a.files.filter=renewPromoTyped("monitoring",progress,0.34,0.31)
        if progress>0.75 {a.files.selected="monitoring-event-07.json"}
        if a.files.selected!="" {return "操作：选择文件、查看重命名 / 删除入口"}
        if a.files.filter!="" {return "操作：过滤本机文件 "+a.files.filter}
        return "操作：浏览本机文件列表"
    case 19:
        if progress>0.64 {
            a.files.active=1
            return "操作：切换代码编辑器标签并继续滚动"
        }
        return "操作：在原生代码编辑器内纵向浏览"
    default:
        if progress>0.35 {a.eventDecisionFilter="已阻断"}
        if progress>0.67 {a.eventMinRiskScore=90}
        if a.eventMinRiskScore>0 {return "操作：决策 BLOCK + 风险分 ≥ 90"}
        if a.eventDecisionFilter!="" {return "操作：仅看已阻断事件"}
        return "操作：返回风险事件并打开研判"
    }
}

func renewPromoComposition() render.Composition {
    app := newRenewApp("http://127.0.0.1:8080")
    examples := promoEvents()
    renewPromoFixture(app, time.Now())
    low, high := promoChime(620), promoChime(920)
    previousScene := -1
    return render.Composition{
        Width:1920, Height:1080,
        FPS:renewPromoFPS, Frames:renewPromoFrames,
        Scale:1.25, Dark:true, Background:ui.Hex("#08101c"),
        View:func(c *ui.Context,f render.Frame) {
            if f.Index == 3 || f.Index == 666 {
                f.Audio(high,render.AudioOptions{Volume:0.75})
            }
            if f.Index < 30 {
                promoTitle(c,f,false)
                return
            }
            if f.Index >= 666 {
                promoTitle(c,f,true)
                return
            }
            beat := renewDemoScene(f.Index)
            sceneIndex := 0
            for i, candidate := range renewPromoBeats {
                if candidate.Start == beat.Start { sceneIndex=i; break }
            }
            if sceneIndex != previousScene {
                renewPromoResetInteraction(app)
                // Reset only when entering a scene, never each frame.
                app.workspacePageScroll(beat.Page).Y = 0
                if beat.Page=="文件" { app.localFileListScroll.Y=0 }
                if beat.Page=="编辑器" {
                    if doc:=app.currentLocalDocument(); doc!=nil {doc.state.SetScrollY(0)}
                }
                f.Audio(low,render.AudioOptions{Volume:0.40})
                previousScene = sceneIndex
            }
            app.page = beat.Page
            app.starting = false
            app.connected = true
            app.healthReady = true
            app.health.CaptureHealthy = true
            app.eventStreamConnected = true
            app.systemConnected = true
            app.lastSync = time.Now()
            app.system.FetchedAt = time.Now()
            app.navigationOpen = f.Index >= 67
            app.navigationDetailed = false
            app.inspectorOpen = beat.Inspector
            app.inspectorSelectedID = ""
            app.eventSelected = -1
            if beat.Focus {
                app.inspectorSelectedID = "demo-009"
                if f.Index >= 637 {
                    app.eventAttentionOnly = true
                }
            }
            segmentProgress:=float64(f.Index-beat.Start)/float64(sceneEndFor(renewPromoBeats,sceneIndex))
            interactionCaption:=renewPromoInteraction(app,sceneIndex,segmentProgress)
            // Frame-indexed easing is deterministic in offline rendering.
            // MaxY is measured by the native layout in the preceding frame.
            // Sidebar/footer stay pinned; only the workspace body scrolls.
            sceneEnd:=666
            if sceneIndex+1<len(renewPromoBeats) {
                sceneEnd=renewPromoBeats[sceneIndex+1].Start
            }
            if beat.Page=="文件" {
                scroll:= &app.localFileListScroll
                scroll.Y=renewPromoScrollOffset(f.Index,beat.Start,sceneEnd,sceneIndex,scroll.MaxY)
            } else if beat.Page=="编辑器" {
                if doc:=app.currentLocalDocument();doc!=nil {
                    doc.state.SetScrollY(float64(renewPromoScrollOffset(f.Index,beat.Start,sceneEnd,sceneIndex,1350)))
                }
            } else if beat.Page!="终端" {
                scroll:=app.workspacePageScroll(beat.Page)
                scroll.Y=renewPromoScrollOffset(f.Index,beat.Start,sceneEnd,sceneIndex,scroll.MaxY)
            }
            app.events = examples[:beat.Count]
            app.eventsVersion = uint64(1000+f.Index)
            app.ownershipCacheValid = false
            // Native workspace body, not a Photoshop/Web screenshot.
            ui.Column(c).Fill().Background(ui.Hex("#09111f")).Padding(18,30).Gap(10).Children(func() {
                ui.Row(c).Height(38).Gap(12).AlignItems(ui.Center).Children(func(){
                    ui.Box(c).Size(9,9).Radius(5).Background(ui.Hex("#35ecc2")).
                        Shadow(0,0,16,0,ui.Hex("#35ecc2").Alpha(0.7))
                    ui.Text(c,"RENEW  /  AGENT OBSERVABILITY").FontSize(16).Bold().
                        LetterSpacing(2).TextColor(ui.Hex("#b9deed"))
                    ui.Spacer(c)
                    ui.Text(c,fmt.Sprintf("%02d / %02d SCENES",sceneIndex+1,len(renewPromoBeats))).
                        FontSize(12).TextColor(ui.Hex("#85adc9"))
                    ui.Text(c,"  •  SIMULATED DEMO").FontSize(11).TextColor(ui.Hex("#e3bd6f"))
                })
                local := float64(f.Index-30)
                appear := float32(math.Min(1,math.Max(0,local/9)))
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
                ui.Row(c).Height(38).Gap(12).AlignItems(ui.Center).Children(func() {
                    ui.Text(c,beat.Caption).FontSize(17).Bold().LetterSpacing(1).
                        TextColor(ui.Hex("#60e4ce"))
                    ui.Text(c,"  "+interactionCaption).FontSize(13).TextColor(ui.Hex("#9bbbd4"))
                    ui.Spacer(c)
                    ui.Text(c,fmt.Sprintf("%02d:%02d  /  00:29",f.Index/renewPromoFPS/60,f.Index/renewPromoFPS%60)).
                        FontSize(12).TextColor(ui.Hex("#7d99b4"))
                    ui.Text(c,"SIMULATED TELEMETRY").FontSize(11).TextColor(ui.Hex("#e6b86a"))
                })
                ui.Progress(c,float64(f.Index)/float64(renewPromoFrames)).Height(3)
            })
        },
    }
}

func TestRenewPromoStoryboard(t *testing.T) {
    if got := float64(renewPromoFrames)/renewPromoFPS; got >= 30 {
        t.Fatalf("duration %.2fs must remain under 30s",got)
    }
    if len(renewPromoBeats) < 20 { t.Fatalf("missing features: %d beats",len(renewPromoBeats)) }
    pageSet := map[string]bool{}
    last := 29
    for _, beat := range renewPromoBeats {
        if beat.Start <= last || beat.Start >= 666 {
            t.Fatalf("invalid storyboard start %d after %d",beat.Start,last)
        }
        if beat.Count < 1 || beat.Count > len(promoEvents()) {t.Fatalf("invalid synthetic event count %d",beat.Count)}
        if beat.Caption == "" || beat.Detail == "" {t.Fatal("unlabelled beat")}
        pageSet[beat.Page] = true
        last = beat.Start
    }
    for _, page := range []string{
        "概览","事件","研判","会话","AgentSight","派生","网络","域名",
        "进程","Agent 识别","监控","eBPF 模块","规则","跟踪","路径权限",
        "CCS","系统","终端","文件","编辑器",
    } {
        if !pageSet[page] {t.Errorf("Desktop route %s missing",page)}
    }
}
func TestRenewPromoVerticalScroll(t *testing.T) {
    if len(renewPromoCurves)!=len(renewPromoBeats) {
        t.Fatalf("need one distinct curve per scene: %d vs %d",len(renewPromoCurves),len(renewPromoBeats))
    }
    seen:=map[renewPromoScrollProfile]bool{}
    for i,profile:=range renewPromoCurves {
        if seen[profile] {t.Fatalf("duplicate scroll curve at %d",i)}
        seen[profile]=true
        if profile.Dwell<0 || profile.Apex<=profile.Dwell || profile.Apex>=1 ||
            profile.Distance<0 || profile.Rewind<0 || profile.Rewind>=1 {
            t.Fatalf("invalid scroll profile %d: %+v",i,profile)
        }
        start,end:=30,130
        if got:=renewPromoScrollOffset(start,start,end,i,1500);got!=0 {t.Fatalf("scene %d did not start at top",i)}
        peak:=renewPromoScrollOffset(start+int(profile.Apex*float64(end-start)),start,end,i,1500)
        tail:=renewPromoScrollOffset(end-1,start,end,i,1500)
        if peak<100 || tail>=peak {t.Fatalf("scene %d lacks down/up movement peak %.1f end %.1f",i,peak,tail)}
        if got:=renewPromoScrollOffset(70,start,end,i,120);got>120 {
            t.Fatalf("scene %d overscrolls native content: %.1f",i,got)
        }
    }
}

func TestRenewPromoFiltersAffectRealTables(t *testing.T) {
    app:=newRenewApp("http://127.0.0.1:8080")
    renewPromoFixture(app,time.Now())
    app.events=promoEvents()
    app.eventsVersion=1
    renewPromoResetInteraction(app)
    full:=len(app.filteredEvents())
    renewPromoInteraction(app,1,0.77)
    filtered:=len(app.filteredEvents())
    if filtered==0 || filtered>=full {t.Fatalf("risk UI filter did not change event table: %d -> %d",full,filtered)}
    renewPromoResetInteraction(app)
    app.eventsVersion++
    before:=len(app.filteredRules())
    renewPromoInteraction(app,12,0.95)
    after:=len(app.filteredRules())
    if after!=1 || before<=after {t.Fatalf("rule UI search did not filter %d -> %d",before,after)}
    renewPromoResetInteraction(app)
    renewPromoInteraction(app,7,0.94)
    if !app.domainRiskOnly || !app.domainShowIPs || app.domainSearch!="203.0.113" {
        t.Fatalf("domain UI filters missing: %+v",app)
    }
    renewPromoResetInteraction(app)
    renewPromoInteraction(app,13,0.83)
    if app.registryTab!=3 || app.registryFilterTag!="Codex" {t.Fatal("tracking tab/filter action missing")}
    renewPromoResetInteraction(app)
    renewPromoInteraction(app,5,0.65)
    if app.lineageMode!=1 {t.Fatal("lineage view mode not switched")}
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
