# MyGo native animated architecture visualizations

This command replaces the old approach of animating flattened slide screenshots with a
**native vector scene rendered by MyGo**. The output is an ordinary GIF for papers,
presentations and project demos. It uses `github.com/egoist/mygo/ui.Render` to
render deterministic off-screen RGBA frames, then the standard Go `image/gif`
encoder. No browser, WebView, window, display server, root privileges or running
eBPF collector is required.

## Render

From the repository root:

```sh
cd desktop/renew
go run ./cmd/visualgif -scene boundary -out ../../artifacts/boundary.gif
go run ./cmd/visualgif -scene observability -out ../../artifacts/observability.gif
go run ./cmd/visualgif -scene decision -out ../../artifacts/decision.gif
go test ./cmd/visualgif
```

Options: `-fps 12`, `-seconds 6`, `-scene boundary|observability|decision`,
`-out /path/to/output.gif`. Exports are 1280×720, loop indefinitely, and
encode frames from integer frame indices rather than wall-clock timing.

A workflow at `.github/workflows/mygo-visualgif.yml` validates these scenes
and uploads all three GIFs for inspection. The command stays inside the isolated
desktop Go module and does not introduce a MyGo dependency into the privileged
backend.

## Storyboard

| Scene | Animated evidence flow | Limit of the claim |
| --- | --- | --- |
| `boundary` | Agent intent → syscall → eBPF probe → policy engine | The prompt is context, not an observed kernel fact |
| `observability` | Root process → descendants → file/network events | PID/PPID ancestry does not prove logical or remote SubAgent calls |
| `decision` | Suspected prompt injection + kernel evidence → BPF LSM gate → example denial | Denial requires a configured and attached enforcement hook |

These are **synthetic explanatory animations**, not live telemetry, an attack
replay, or benchmark evidence. They intentionally avoid numerical accuracy and
overhead claims from presentation slides. The source decks are the
`Agent_eBPF_Filter_Transparent_Boundary` and `eBPF_Security_and_Observability`
presentations. Add measured facts through the existing backend/recording APIs
rather than silently depicting a generated demo as a measured result.

## Renderer contract

- MyGo supplies the vector paint engine through `ui.Render`; GIF encoding is
  standard-library output formatting only.
- Do not call `p.AnimationFrame` inside the offline renderer: animation
  advances explicitly by frame index and is reproducible for CI snapshots.
- Every frame is fully drawn. There are no cached `ui.Element` values or
  goroutines touching `ui.Context`.
- Colors and drawings intentionally share the native Renew visual language,
  while the headless demo runs independently of the privileged application.
