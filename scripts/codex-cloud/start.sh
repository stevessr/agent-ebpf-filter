#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
mode="${1:-frontend}"; port="${CODEX_FRONTEND_PORT:-5173}"; docs_port="${CODEX_DOCS_PORT:-5174}"
state="$CODEX_CACHE_HOME/runtime"; mkdir -p "$state"
probe() { local url="$1" pid="$2"; for _ in $(seq 1 30); do kill -0 "$pid" 2>/dev/null || return 1; if curl -fsS --max-time 2 "$url" >/dev/null; then return 0; fi; sleep 1; done; return 1; }
start_one() {
  local label="$1" dir="$2" url="$3"; shift 3
  local pidfile="$state/$label.pid" logfile="$state/$label.log" pid
  if [[ -f "$pidfile" ]]; then
    pid="$(cat "$pidfile")"
    if kill -0 "$pid" 2>/dev/null && curl -fsS --max-time 2 "$url" >/dev/null; then echo "$label already healthy ($url, PID $pid)"; return; fi
    if kill -0 "$pid" 2>/dev/null; then echo "$label has an unhealthy existing PID ($pid); inspect $logfile" >&2; return 1; fi
    rm -f "$pidfile"
  fi
  (cd "$dir"; nohup "$@" >"$logfile" 2>&1 </dev/null & echo "$!" > "$pidfile")
  pid="$(cat "$pidfile")"
  if probe "$url" "$pid"; then echo "$label ready at $url (PID $pid)"; else echo "$label failed readiness; inspect $logfile" >&2; tail -n 25 "$logfile" >&2; return 1; fi
}
stop_one() {
  local pidfile="$state/$1.pid" pid
  [[ -f "$pidfile" ]] || return 0
  pid="$(cat "$pidfile")"; if [[ "$pid" =~ ^[0-9]+$ ]]; then kill "$pid" 2>/dev/null || true; fi
  rm -f "$pidfile"
}
case "$mode" in
  frontend) start_one frontend "$CODEX_ROOT/frontend" "http://127.0.0.1:$port/" bun run dev --host 0.0.0.0 --port "$port" --strictPort ;;
  docs) start_one docs "$CODEX_ROOT" "http://127.0.0.1:$docs_port/" bun run docs:dev --host 0.0.0.0 --port "$docs_port" --strictPort ;;
  all) "$0" frontend; "$0" docs ;;
  status)
    for label in frontend docs; do if [[ -f "$state/$label.pid" ]]; then pid="$(cat "$state/$label.pid")"; if kill -0 "$pid" 2>/dev/null; then echo "$label running PID $pid"; else echo "$label stale PID $pid"; fi; else echo "$label stopped"; fi; done ;;
  stop) stop_one frontend; stop_one docs; echo 'Stopped recorded development server PIDs.' ;;
  *) echo 'Usage: start.sh [frontend|docs|all|status|stop]' >&2; exit 2 ;;
esac
