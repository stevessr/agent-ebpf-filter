#!/usr/bin/env bash
# Acceptance scope: virtual X11 or Weston headless, Mesa llvmpipe.
# Verifies *actual window startup* and collects artifacts. This is NOT a
# substitute for real-GPU, human-in-the-loop Wayland/X11 QA.
set -Eeuo pipefail

mode="${1:?specify x11 or wayland}"
out="${2:?specify artifact directory}"
mkdir -p "$out"
out="$(realpath "$out")"
case "$mode" in x11|wayland) ;; *) echo "unknown display mode: $mode" >&2; exit 2 ;; esac

repo="$(cd "$(dirname "$0")/.." && pwd)"
app="$repo/desktop/visualizer-makepad/target/debug/agent-ebpf-codecity"
[[ -x "$app" ]] || { echo "missing debug binary: $app" >&2; exit 2; }
mock_pid="" compositor_pid="" app_pid="" wm_pid=""
cleanup() {
  for pid in "$app_pid" "$wm_pid" "$compositor_pid" "$mock_pid"; do
    if [[ -n "$pid" ]]; then kill "$pid" 2>/dev/null || true; fi
  done
  for pid in "$app_pid" "$wm_pid" "$compositor_pid" "$mock_pid"; do
    if [[ -n "$pid" ]]; then wait "$pid" 2>/dev/null || true; fi
  done
  if [[ -n "${display_temp:-}" ]]; then rm -rf "$display_temp"; fi
}
trap cleanup EXIT
display_temp="$(mktemp -d)"
export LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe
export AGENT_EBPF_WS_URL=ws://127.0.0.1:18765/ws/event-summaries
export AGENT_EBPF_WORKSPACE_ROOT=/workspaces/codecity-demo
export AGENT_EBPF_INDEX_JSON="$display_temp/codecity-index.json"
printf '{"version":1,"files":["src/main.rs","src/view.rs","src/agent.rs","README.md","tests/smoke.rs"]}\n' > "$AGENT_EBPF_INDEX_JSON"
python3 "$repo/scripts/codecity-smoke-feed.py" --port 18765 >"$out/feed.log" 2>&1 &
mock_pid=$!
for _ in $(seq 1 100); do
  if [[ -s "$out/feed.log" ]] && grep -q 'ready' "$out/feed.log"; then break; fi
  kill -0 "$mock_pid" || { cat "$out/feed.log"; exit 1; }
  sleep 0.1
done
grep -q 'ready' "$out/feed.log"

echo "display-mode=$mode" | tee "$out/summary.txt"
if [[ "$mode" == "x11" ]]; then
  export DISPLAY=:98
  unset WAYLAND_DISPLAY
  export XDG_SESSION_TYPE=x11
  Xvfb "$DISPLAY" -screen 0 1280x800x24 +extension GLX -noreset >"$out/compositor.log" 2>&1 &
  compositor_pid=$!
  for _ in $(seq 1 80); do
    if xdpyinfo -display "$DISPLAY" >/dev/null 2>&1; then break; fi
    kill -0 "$compositor_pid" || { cat "$out/compositor.log"; exit 1; }
    sleep 0.2
  done
  xdpyinfo -display "$DISPLAY" >"$out/display-info.log"
  glxinfo -B >"$out/renderer-info.log" 2>&1 || true
  openbox >"$out/window-manager.log" 2>&1 &
  wm_pid=$!
  "$app" --linux-backend=x11 >"$out/app.log" 2>&1 &
  app_pid=$!
  window_id=""
  for _ in $(seq 1 100); do
    window_id="$(xdotool search --onlyvisible --name 'CodeCity' 2>/dev/null | head -n 1 || true)"
    [[ -n "$window_id" ]] && break
    if ! kill -0 "$app_pid" 2>/dev/null; then
      echo "X11 app exited before visible toplevel" >&2
      tail -n 100 "$out/app.log" >&2
      exit 1
    fi
    sleep 0.2
  done
  [[ -n "$window_id" ]] || { echo "X11 CodeCity window not found" >&2; tail -n 100 "$out/app.log"; exit 1; }
  echo "x11-window-id=$window_id" >>"$out/summary.txt"
  xwininfo -id "$window_id" >"$out/window-info.log"
  xdotool windowactivate --sync "$window_id" || true
  sleep 5
  import -display "$DISPLAY" -window "$window_id" "$out/x11-before.png"
  colors="$(identify -format '%k' "$out/x11-before.png")"
  echo "x11-first-frame-unique-colors=$colors" | tee -a "$out/summary.txt"
  # An empty/undrawn window is typically just one clear color. Allow
  # moderately sparse GPU screenshots, but do not pass a blank framebuffer.
  [[ "$colors" -ge 24 ]] || { echo "X11 screenshot appears blank: $colors colors" >&2; exit 1; }
  xdotool mousemove --window "$window_id" 240 300 mousedown 1 sleep 0.3 \
    mousemove --window "$window_id" 420 400 mouseup 1
  xdotool mousemove --window "$window_id" 440 410 click 4 click 5
  xdotool key --clearmodifiers Left Right Escape
  sleep 2
  kill -0 "$app_pid" || { tail -n 100 "$out/app.log"; echo "X11 app crashed after input" >&2; exit 1; }
  import -display "$DISPLAY" -window "$window_id" "$out/x11-after-input.png"
  echo "x11-capture-and-basic-input=PASS" | tee -a "$out/summary.txt"
else
  unset DISPLAY
  export XDG_SESSION_TYPE=wayland
  export XDG_RUNTIME_DIR="$display_temp/runtime"
  mkdir -p "$XDG_RUNTIME_DIR"
  chmod 700 "$XDG_RUNTIME_DIR"
  export WAYLAND_DISPLAY=codecity-test
  # Headless GL rendering uses the Mesa software rasterizer in CI.
  weston --backend=headless-backend.so --renderer=gl \
    --socket="$WAYLAND_DISPLAY" --width=1280 --height=800 \
    --idle-time=0 --no-config >"$out/compositor.log" 2>&1 &
  compositor_pid=$!
  for _ in $(seq 1 100); do
    [[ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]] && break
    if ! kill -0 "$compositor_pid" 2>/dev/null; then
      echo "Weston headless compositor exited before socket" >&2
      cat "$out/compositor.log" >&2
      exit 1
    fi
    sleep 0.2
  done
  [[ -S "$XDG_RUNTIME_DIR/$WAYLAND_DISPLAY" ]] || { echo "Weston Wayland socket not ready" >&2; exit 1; }
  wayland-info >"$out/display-info.log" 2>&1 || true
  WAYLAND_DEBUG=client "$app" --linux-backend=wayland >"$out/app.log" 2>&1 &
  app_pid=$!
  # wl_surface.commit + xdg_toplevel means the Wayland client constructed and
  # submitted at least one window surface. This is not pixel/screenshot proof.
  for _ in $(seq 1 100); do
    if grep -Eq 'wl_surface@[0-9]+\.commit' "$out/app.log" &&
       grep -q 'Selected: Wayland backend' "$out/app.log"; then
      break
    fi
    if ! kill -0 "$app_pid" 2>/dev/null; then
      echo "Wayland app exited before surface commit" >&2
      tail -n 100 "$out/app.log" >&2
      exit 1
    fi
    sleep 0.2
  done
  grep -q 'Selected: Wayland backend' "$out/app.log" || { echo "unexpected windowing backend" >&2; exit 1; }
  grep -Eq 'wl_surface@[0-9]+\.commit' "$out/app.log" || { echo "no Wayland surface commit observed" >&2; exit 1; }
  sleep 4
  kill -0 "$app_pid" || { echo "Wayland app crashed after surface creation" >&2; tail -n 100 "$out/app.log"; exit 1; }
  echo "wayland-protocol-surface-commit=PASS" | tee -a "$out/summary.txt"
  echo "wayland-screen-pixels=NOT_CAPTURED (Weston headless)" >>"$out/summary.txt"
fi

echo "renderer=llvmpipe software; not real GPU" >>"$out/summary.txt"
echo "result=PASS" >>"$out/summary.txt"
