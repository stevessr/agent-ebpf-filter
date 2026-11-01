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
