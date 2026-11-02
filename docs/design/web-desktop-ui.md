# Web / Desktop UI Foundation

Status: owner-supplied transcription received 2026-11-02; exact X screenshots remain unverified.

Reference: https://x.com/ohayou792/status/2116884456006897800 . Direct retrieval returned HTTP 403, but the owner supplied a detailed rule transcription in chat. These are owner-provided requirements, not an independently verified quotation or screenshot reproduction.

## Product interaction priorities

1. A user entering the workspace should immediately identify **current protection status, latest activity, and next action**. Critical outages or unhealthy capture must remain visible even when historical events are present.
2. Monitoring is read-first. Risk labels must pair **color with text, icon or severity**, never color alone. Separate collector **disconnected**, **loading**, **empty**, **paused**, and **healthy** states; never treat an empty event list as proof of safety.
3. For policy changes and destructive actions, show the scope and impact before confirmation; keep a cancel path and clearly distinguish preview from persisted policy.
4. Preserve information density in event tables without making primary navigation, settings, dialogs, or focus targets undersized.
5. Web and Desktop share semantics and visual rhythm, not rendering technology. Web remains Vue 3 + Ant Design; Renew remains native MyGo. Prefer OS font scaling, native accent, high-contrast support, and reduced motion over artificial cross-platform pixel identity.

## Tokens and density contract

| Token | Web light | Web dark | Native implementation |
| --- | --- | --- | --- |
| Canvas | `#f8fafc` | `#111318` | `Theme.Background` |
| Primary surface | `#ffffff` | `#191c22` | `Theme.Surface` |
| Border | `#d8e2f0` | `#344256` | `Theme.Border` unless high contrast |
| Primary text | `#1f2937` | `#e5edf9` | `Theme.Text` unless high contrast |
| Brand accent (Web) | `#2563eb` | `#7b9fff` | Native `Theme.Accent` stays system-controlled |

- Base Web type: 16px/1.7, weight 400; headings use 500 and 22px/18px/16px, UI minimum 11px. Desktop keeps the OS typography scale.
- Corner radius: 8px for standard shells, dialogs, and controls; internal compact tags may be smaller.
- Control sizes: 36px in the Web titlebar; at least 44px in the activity rail; ordinary navigation rows at least 36px. Density-sensitive table controls may be compact but must remain keyboard-operable.
- Narrow workspaces (up to 720px): navigation becomes an overlay above the event canvas with an explicit close target. On larger screens navigation continues to reserve space.
- Keyboard: Ctrl/Cmd+K focuses feature search, Escape closes narrow navigation. Editors, terminals, editable fields and previously handled events retain their own shortcuts. Every focusable control must show a visible focus indicator.
- Honor `prefers-reduced-motion` and forced-colors/high-contrast modes. Native MyGo preserves system accent, focus, selection, OS font scaling, and high-contrast text/borders.

## Implementation and verification

- Web Ant Design token mapping: `frontend/src/config/uiDesignTokens.ts`, used from `frontend/src/App.vue`. Keyboard decisions are isolated in `frontend/src/composables/workbench/navigationShortcuts.ts` for behavior tests, including editable/terminal focus.
- Workbench CSS never overrides the document root's font size; it scopes density to `.ide-workbench` and allows browser/user scaling.
- Shared CSS surfaces: `frontend/src/style.css` and `frontend/src/components/workbench/workbench.css`.
- Native mapping: `desktop/renew/native_theme.go`.
- Run: `cd frontend && bun run test:ui && bun run build`, then `cd desktop/renew && go test ./...`.
- Manually verify 375px, 768px, 1280px and 1920px workspaces; light/dark/high contrast; mouse/keyboard; screen-reader labels; OS scaling; disconnected/paused/empty/error event streams. Do not call unexecuted visual or E2E checks passed.

## Follow-up on external reference

The owner-supplied text is actionable. Original screenshots can later be compared to the implementation. Avoid aesthetic changes that obscure incidents, make risk severity ambiguous, or degrade dense tables.


## Renew product hierarchy and evidence semantics

The brand **明镜高悬** is a low-noise personal security monitor, not a replacement for the research workbench. The default view answers, in this order: **is collection connected and current?**, **which recent actions require attention?**, **what can I inspect or do next?** Show five recent activity summaries independently of event-table filters. Do not imply that no events means the host is safe.

| UI signal | Required interpretation | Must not imply |
| --- | --- | --- |
| Collecting and recently synchronized | A live, bounded observation window | A completed malware scan or total system safety |
| Disconnected / stale / initializing | Evidence is not current or completeness is unknown | "No threats" |
| Paused event presentation | UI summaries are paused, collector/enforcement continue | Protection is disabled |
| Risk warning / ALERT | An observation or policy alert to investigate | Kernel enforcement succeeded |
| BLOCK / DENY | Report the recorded decision **and** confirmed execution evidence separately | Every requested operation was blocked solely because a badge says so |
| Policy preview | Pending proposed scope and effect | A kernel map has already changed |

The Renew navigation contract is a 54-DIP icon rail with optional 198-DIP compact or 302-DIP expanded labels, hiding the investigator rail when the content canvas needs room. Keep **monitoring / management / diagnostics** distinct; eBPF module load and unload are explicit user actions, with a destructive confirmation on unload. Agent, process, PID, session, destination, path and policy references should offer precise related-object navigation without silently broadening filters. The research workbench retains replay, full evidence exploration, experiments and performance tools.

On Web workspaces at widths up to 720px, open with navigation **closed**. Opening a feature or pressing Escape closes the overlay; viewport changes into narrow mode also dismiss any open navigation. Ctrl/Cmd+K opens feature navigation from a non-editable context and focuses feature search; never steal Monaco or terminal shortcuts. Activity-rail buttons expose pressed state in addition to selected color. Preserve native MyGo windowing, system type scaling, system theme and accessibility behavior, rather than introducing a WebView.

## Dependency update policy

- Keep `desktop/renew` (MyGo) isolated from privileged `backend`. Review release notes before changing the pinned stable MyGo version and commit `go.mod` **together with** `go.sum`. As of this change, MyGo is pinned to `v0.7.4` (official release 2026-11-01) for window first-frame, native scrollbars/editor, Windows and Linux packaging improvements. These features are upstream capabilities; no new application-level controls are claimed unless wired in source.
- The existing `.github/workflows/renew-mygo-update.yml` checks stable upstream updates in a read-only job and proposes a separately reviewable dependency PR. Do not disable its tests, merge updates automatically, or run untrusted upstream code with a write-capable token.
- Frontend updates use `frontend/package.json` **and** `frontend/bun.lock` together. Treat major Vue, Vite, Ant Design or MyGo upgrades as migrations with typed-build, interaction, dark/light and accessible-navigation coverage, not version-number-only changes.
- Validate: `cd desktop/renew && go mod tidy -diff && go test ./... && go tool mygo vet .`; `cd frontend && bun install --frozen-lockfile && bun run test:ui && bun run build`. Also execute standard CI packaging and real niri/Wayland, Windows and narrow-screen smoke tests before a production release. This text is a checklist, not a claim that checks ran here.


## Owner-provided seamless and flat UI standard

- **Seamless:** avoid decorative surrounding frames, artificial gradient layers and unrelated palette. The Web workbench shell is transparent over the app canvas; native Renew inherits OS appearance, font scale, focus and accent.
- **Flat:** no decorative gradients, mesh, noise, heavy shadows, blur, glow or neon in chrome. Functional focus rings, state outlines and safety overlays remain. Cards use a 0.5px border and 12px corner radius; normal controls use 8px.
- **Compact:** foreground current collection health, evidence, severity and next action. Remove decorative emoji, uppercase metadata and ornamental symbols. Prefer short headings; limit diagram box subtitles to five words. Keep explanatory prose in documentation or accessible disclosures instead of diagrams.
- **Typography:** body 16px/1.7 weight 400, headings 22/18/16px weight 500, minimum 11px. Prefer Anthropic Sans when installed, with a system fallback; never redistribute or hotlink a proprietary font. MyGo preserves native typography.
- **Tokens:** Web uses `--surface-{0,1,2}`, `--text-{primary,secondary,muted,accent,success,danger,warning}`, `--border`, semantic backgrounds and borders, `--font-{sans,voice,mono}`, `--radius`, `--pad-*` and `--gap-*`. Both light and dark themes are mandatory. Use at most two color ramps in diagrams and a legend when color conveys state.
- **Icons:** no emoji or manually authored SVG icon paths. Use accessible labels for icon-only buttons. Tabler outline is the owner's preferred icon set; this independent Vue application currently bundles Ant Design outline icons and MyGo uses native equivalents. Do not claim Tabler is preloaded; migrate with bundled assets and updated lockfile, not a CDN.
- **Keyboard:** Ctrl/Cmd+K must not steal editor/terminal/form shortcuts. Escape closes the narrow navigation only from the shell/search context; return focus to navigation toggle.

### Embedded-widget rules do not map one-to-one

The original instructions about hiding Claude host edges, `sendPrompt()`, 680px streaming iframe sizing, mandatory `style`-HTML-`script` ordering, avoiding tabs and `display:none` during token streaming, and `position:fixed` inside an auto-height iframe are **specific to embedded generated widgets**. The standalone Vue app and native MyGo desktop are not streaming output. Preserve functional DockNode tabs, app routing, native OS windowing and safe navigation overlays. Never replace routing with `sendPrompt()` or force an iframe layout into the native desktop.

### Merge and manual acceptance

CI must pass frontend tests/build, native Go tests/vet, package checks and Web bilingual acceptance. Test light/dark/high contrast, 375px/768px/1280px/1920px and niri/Wayland plus Windows before production distribution. CI alone does not prove the pixel-level visual reference or assistive-technology usability.


## Deep interaction and freshness regression contract

- Dock tabs use roving tabindex and horizontal ArrowLeft/ArrowRight/Home/End navigation, with every selected tab labelling its panel. Splitter keyboard input respects the current split axis and 15–85% bounds; avoid stealing terminal shortcuts.
- During pointer resize, Vue may produce many layout mutations per second. Coalesce localStorage writes with a 250ms trailing flush, and flush pending state on page hide, tab backgrounding, and component unmount. Only serialized layout is persisted; no event payloads or credentials.
- Renew freshness is different from connectivity. When a nonzero lastSync is more than two minutes old, collector and pipeline health badges must become warning instead of success. A timestamp more than 30s in the future is a clock-skew warning. Explicit capture failures remain danger; high-risk evidence remains visible. The home summary links diagnostics rather than claiming a clean scan.
- Tests cover the timer coalescer, panel focus semantics, stale status precedence, and fresh/expired cases with deterministic timestamps. UI and native accessibility still need hands-on screen reader and keyboard QA.

## Narrow search and pointer resizing

- Focusing feature search on viewports up to 720px opens the navigation result overlay; selecting a destination clears the query and closes the overlay. Search remains an ordinary input and does not capture editor or terminal shortcuts.
- Pointer resizing now filters events by the initiating pointer ID and primary button, ignores zero-size split containers, schedules at most one live resize per animation frame, and commits the final pointer position before releasing listeners. Cancel/blur/unmount discards a pending frame and restores the document cursor/user-selection state.
- Accessibility focus rings and keyboard resizing remain available independently from pointer interaction; this is a UI scheduling change, not an eBPF collector update.

## Missing sync timestamp is not a healthy sample

A live backend connection and healthy collector flag are insufficient for a green realtime badge without a trustworthy `lastSync`. When that timestamp is zero, the collector and pipeline show a warning and the overview offers diagnostics. Fresh samples restore the normal state; stale samples, future timestamps, explicit capture failures and high-risk evidence retain their existing precedence. This is a UI evidence semantics change only and does not alter kernel enforcement.
