# Web / Desktop UI Foundation

Status: baseline implemented on 2026-11-02. **External reference not yet verified.**

Reference requested by the project owner: https://x.com/ohayou792/status/2116884456006897800 . The original post was not accessible at implementation time (X returned HTTP 403; available public mirrors did not provide the post). **Do not attribute the design values below to that post** until the text or screenshots have been reviewed. This is an independently auditable baseline and a follow-up reference to revisit.

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

- Base Web type: 14px; telemetry rows may be denser while readable. Desktop keeps the OS typography scale.
- Corner radius: 8px for standard shells, dialogs, and controls; internal compact tags may be smaller.
- Control sizes: 36px in the Web titlebar; at least 44px in the activity rail; ordinary navigation rows at least 36px. Density-sensitive table controls may be compact but must remain keyboard-operable.
- Narrow workspaces (up to 720px): navigation becomes an overlay above the event canvas with an explicit close target. On larger screens navigation continues to reserve space.
- Keyboard: Ctrl/Cmd+K focuses feature search, Escape closes narrow navigation. Every focusable control must show a visible focus indicator.
- Honor `prefers-reduced-motion` and forced-colors/high-contrast modes. Native MyGo preserves system accent, focus, selection, OS font scaling, and high-contrast text/borders.

## Implementation and verification

- Web Ant Design token mapping: `frontend/src/config/uiDesignTokens.ts`, used from `frontend/src/App.vue`.
- Shared CSS surfaces: `frontend/src/style.css` and `frontend/src/components/workbench/workbench.css`.
- Native mapping: `desktop/renew/native_theme.go`.
- Run: `cd frontend && bun run test:ui && bun run build`, then `cd desktop/renew && go test ./...`.
- Manually verify 375px, 768px, 1280px and 1920px workspaces; light/dark/high contrast; mouse/keyboard; screen-reader labels; OS scaling; disconnected/paused/empty/error event streams. Do not call unexecuted visual or E2E checks passed.

## Follow-up on external reference

Once the source X post is available, extract its concrete rules and examples in a separate review, record exact deviations here, and implement only rules suitable for an OS monitoring/control application. Avoid aesthetic changes that obscure incidents, make risk severity ambiguous, or degrade dense tables.


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
