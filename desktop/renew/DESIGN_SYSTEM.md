# Renew Native Design System（MyGo）

Renew uses **MyGo native UI** (Go and GPU drawing), **not** a webview. Our small
`design` package implements opinionated monitoring presentation on top of
MyGo's accessible primitives. It has no dependencies beyond the MyGo version
already pinned in `desktop/renew/go.mod`, does not import application code and
does not call the privileged backend.

## Ecosystem evaluation (2026-10)

| Option | Strength | Decision |
| --- | --- | --- |
| [MyGo native `ui`](https://github.com/egoist/mygo/tree/main/ui) | GPU-native rendering, layout, input/IME, focus, tables, virtual lists, accessibility, test harness | **Foundation**: keep native semantics and use for controls |
| [Onevra](https://github.com/oneo-me/onevra/tree/main/source) | Existing MyGo v0.5.5-compatible components, theme, sidebar/shells, chart and command packages; MIT | **Design reference / optional future dependency**; avoid wholesale theme replacement and transitive migration now |
| [Fyne](https://github.com/fyne-io/fyne) / [Gio](https://github.com/gioui/gio) | Mature alternative native Go GUI approaches | Not drop-in MyGo components; would introduce another event/rendering stack |
| [shadcn/ui](https://ui.shadcn.com/) / [Radix](https://www.radix-ui.com/) | Excellent web component primitives | Appropriate for the separate Vue/web UI only with framework integration; not native MyGo |
| Renew `design` (this package) | Bounded-summary metrics, evidence notices, product semantics, host theme adherence | **Domain-specific layer**; reusable across native Renew pages |

Onevra is MIT licensed. Our code is an original composition using public MyGo
APIs and does **not** copy Onevra source or assets. Any future vendor/import must
retain its license and be independently pinned, audited and tested.

## Components

| API | Usage | Contract |
| --- | --- | --- |
| `Action(context, label, variant)` | Native primary/neutral/ghost action, MyGo event lifecycle and accessibility | No DOM bridge or external component dependency |
| `SemanticBadge(context, label, tone)` / `TintedBadge` | State and risk chips | Theme-driven, high-contrast-safe |
| `EmptyState(context, title, explanation)` | Missing data or collector state | Does not imply that zero alerts prove safety |
| `ToneColor(theme, Tone)` | Semantic color resolver | Uses current theme's accent/success/warning/danger/muted; no fixed color |
| `MetricTile(context, Metric)` | Desktop KPI / observation snapshot | Not a button; always supply timeframe or summary boundary in Detail |
| `MetricStrip(context, []Metric)` | Responsive metrics row | Wraps on narrow windows; works with MyGo scaling |
| `Section(context, title, subtitle, content)` | Common section surface | Caller owns content and interactions |
| `NoticeCard(context, Notice)` | Evidence caveat / advisory | Text-first; never show green “safe” just because event count is zero |

Components use the **current per-frame theme**, including live appearance
changes, system accent, font scaling, reduced-motion setting and high-contrast
preference. Layout avoids hover-only meaning; the native widgets continue to own
focus, keyboard and screen-reader behavior. No animated chart should be added by
default to high-rate event streams; favor virtualized/aggregated views and
bounded input sizes.

### Wiring / examples

```go
// UI build pass, within a MyGo native view:
design.MetricStrip(c, []design.Metric{
    {
        Label: "摘要活动", Value: "12",
        Detail: "当前已加载窗口 · 非完整历史", Tone: design.Accent,
    },
})
design.NoticeCard(c, design.Notice{
    Title: "证据边界",
    Message: "ALERT 不是 BLOCK；完整处置证据需要查看事件详情。",
    Tone: design.Warning,
})
```

The Overview page projects the existing bounded, redacted event buffer into
metrics. Existing native cards delegate to `design.Section`; common status chips
use `design.TintedBadge`. A subset of home actions uses native `Action` to
exercise the self-owned interaction layer without replacing MyGo input/focus. Other pages' existing `statCard` helper now shares `MetricTile`
styling. UI work changes **no** collectors, policies, permission gates,
network access, telemetry, secrets or event retention.

## Next components (keep in the native design package)

1. **VirtualEventTable:** adapt MyGo table/list virtualization with a stable
   identity key and explicit dropped/stale state. Never render raw payloads by
   default.
2. **RiskTimeline:** bounded and downsampled bins with separate alert and
   confirmed-block outcomes; synchronize cursor with the event detail view.
3. **AgentGraph:** keyboard-selectable, virtualized lineage explorer; preserve
   uncertainty in process attribution and differentiate root agents/subagents.
4. **CommandPalette / Preferences:** reuse MyGo or audited Onevra primitives,
   preserving existing privilege confirmation flows.
5. **Theme gallery / visual regression:** screenshots at narrow/wide sizes,
   dark/light and high contrast on Wayland, Windows and macOS.

## Verification

From `desktop/renew`, run `go test ./design ./...` (Go 1.27.1 or newer) and
`go vet ./...`, followed by the existing Renew CI/package workflow.
`ui.NewTester` tests enforce visible text. Theme colors and snapshot
semantics are checked separately. A PR should not be merged without a full
headless test run and, where possible, desktop visual checks.


## Performance work

- Filter conditions now use a comparable `eventFilterSignature` struct rather
  than constructing a NUL-delimited string with repeated integer/float/boolean
  formatting. This prevents delimiter collisions and repeated key allocations.
- `summaryOwnershipIndex()` caches attribution for a single event version:
  per-row render, inspector and quick navigation share one index. The index
  rebuilds on the next event-window version; direct unversioned snapshots are
  deliberately uncached.
- Native actions are thin wrappers over `ui.ButtonBase`, not a second input,
  graphics or widget lifecycle. All presentation components remain synchronous
  and avoid timers, background work and independent event subscription.
- `BenchmarkStructuredEventFilterKey` and
  `BenchmarkSharedSummaryOwnershipIndex` provide allocation/CPU regression
  baselines. Use `go test -run '^$' -bench
  'Benchmark(StructuredEventFilterKey|SharedSummaryOwnershipIndex)' -benchmem
  ./...` on the supported Go toolchain.
