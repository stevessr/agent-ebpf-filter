#!/usr/bin/env bash
# Deterministic browser smoke test. No real backend, root, policies or secrets.
set -euo pipefail
cd "$(dirname "$0")/.."
API_PORT="${RENEW_FIXTURE_PORT:-5483}"
UI_PORT="${RENEW_SMOKE_UI_PORT:-5486}"
ARTIFACTS="$(mktemp -d /tmp/renew-browser-XXXXXX)"
SESSION="renew-smoke-$$"
FIXTURE_PID=''; VITE_PID=''
ab() { bunx --bun agent-browser --session "$SESSION" "$@"; }
cleanup() {
  ab close >/dev/null 2>&1 || true
  [[ -z "$VITE_PID" ]] || kill "$VITE_PID" 2>/dev/null || true
  [[ -z "$FIXTURE_PID" ]] || kill "$FIXTURE_PID" 2>/dev/null || true
}
trap cleanup EXIT
RENEW_FIXTURE_PORT="$API_PORT" bun scripts/renew-smoke-fixture.ts >"$ARTIFACTS/fixture.log" 2>&1 & FIXTURE_PID=$!
VITE_AGENT_BACKEND_URL="http://127.0.0.1:$API_PORT" bun run dev --host 127.0.0.1 --port "$UI_PORT" --strictPort >"$ARTIFACTS/vite.log" 2>&1 & VITE_PID=$!
for _ in $(seq 1 100); do
  kill -0 "$FIXTURE_PID" "$VITE_PID" 2>/dev/null || { cat "$ARTIFACTS"/*.log; exit 1; }
  if curl -fsS "http://127.0.0.1:$API_PORT/__fixture/state" >/dev/null 2>&1 && curl -fsS "http://127.0.0.1:$UI_PORT/renew" >/dev/null 2>&1; then break; fi
  sleep .1
done
# Verify immediately after the dev server starts.
ab --executable-path "${CHROMIUM_PATH:-/usr/bin/chromium}" open "http://127.0.0.1:$UI_PORT/renew/monitoring"
ab wait --load networkidle
ab snapshot -i >"$ARTIFACTS/monitoring.txt"
ab screenshot "$ARTIFACTS/monitoring.png"
ab eval 'if (!document.querySelector(".renew-shell") || document.querySelector("vite-error-overlay")) throw new Error("Renew did not mount"); true'
ab wait --fn '!document.querySelector("[aria-label=进程行为]").disabled'
ab eval 'const s=document.querySelector("[aria-label=进程行为]"); if (getComputedStyle(s).backgroundColor !== "rgb(185, 71, 25)" || getComputedStyle(s.querySelector(".renew-switch__state")).backgroundColor !== "rgba(0, 0, 0, 0)") throw new Error("switch style overridden by card badge CSS"); true'
ab click '[aria-label="进程行为"]'
ab wait --fn 'document.querySelector("[aria-label=进程行为]").getAttribute("aria-checked") === "false"'
ab reload
ab wait --load networkidle
ab eval 'if (document.querySelector("[aria-label=进程行为]").getAttribute("aria-checked") !== "false") throw new Error("toggle did not persist"); true'
curl -fsS -X POST "http://127.0.0.1:$API_PORT/__fixture/fail-next" >/dev/null
ab click '[aria-label="进程行为"]'
ab wait --text 'fixture rejected update'
ab eval 'if (document.querySelector("[aria-label=进程行为]").getAttribute("aria-checked") !== "false") throw new Error("failed write changed confirmed state"); true'
ab select '[aria-label="Harness 筛选"]' codex
ab find role button click --name 'appstore 事件'
ab wait --url '**/renew/events?harness=codex'
ab eval 'const rows=[...document.querySelectorAll(".renew-activity")]; if (rows.length!==1 || !rows[0].textContent.includes("Codex") || document.querySelectorAll("[aria-label=会话筛选] option").length!==2) throw new Error("event harness filter failed"); true'
ab find role button click --name 'global 网络'
ab wait --url '**/renew/network?harness=codex'
ab eval 'if (!document.querySelector("tbody").textContent.includes("codex.test") || document.querySelector("tbody").textContent.includes("claude.test")) throw new Error("network filter failed"); true'
ab screenshot "$ARTIFACTS/network.png"
ab find role button click --name 'node-index 进程'
ab wait --url '**/renew/processes?harness=codex'
ab eval 'if (document.querySelectorAll("tbody tr").length!==2 || !document.querySelector("tbody").textContent.includes("curl")) throw new Error("process descendant attribution failed"); true'
ab select '[aria-label="Harness 筛选"]' claude
ab wait --fn 'document.querySelectorAll("tbody tr").length === 1'
ab eval 'if (!document.querySelector("tbody").textContent.includes("Claude Code")) throw new Error("Claude process classification failed"); true'
ab reload
ab wait --load networkidle
ab eval 'if (document.querySelector("[aria-label=\"Harness 筛选\"]").value !== "claude") throw new Error("URL filter did not survive reload"); true'
ab find role button click --name 'appstore 事件'
ab wait --url '**/renew/events?harness=claude'
ab click '.renew-activity'
ab wait --text 'claude-network'
ab eval 'if (!document.querySelector(".ant-drawer")) throw new Error("detail drawer missing"); true'
# Close the Ant Design drawer before testing navigation.
ab press Escape
ab wait --fn '!document.querySelector(".ant-drawer-mask") || getComputedStyle(document.querySelector(".ant-drawer-mask")).display === "none"'
ab find role button click --name 'safety-certificate 规则'
ab wait --url '**/renew/rules?harness=claude'
ab wait --fn '!document.querySelector("button[type=submit]").disabled'
ab fill '[aria-label="规则命令名"]' renew-fixture-command
ab click 'button[type=submit]'
ab wait --fn 'document.querySelectorAll("tbody tr").length === 1'
ab find role button click --name '编辑'
ab select '[aria-label="规则动作"]' BLOCK
ab click 'button[type=submit]'
ab wait --fn 'document.querySelector("tbody").textContent.includes("BLOCK")'
ab find role button click --name '删除'
ab find role button click --name '确认删除'
ab wait --fn 'document.querySelectorAll("tbody tr").length === 0'
ab screenshot "$ARTIFACTS/rules.png"
ab find role button click --name 'dashboard 概览'
ab wait --url '**/renew?harness=claude'
ab eval 'if (!document.querySelector(".renew-session-list").textContent.includes("Claude Code") || document.querySelector(".renew-activity-list").textContent.includes("Codex")) throw new Error("overview session filter failed"); true'
ERRORS="$(ab errors)"
[[ -z "$ERRORS" ]] || { printf '%s\n' "$ERRORS"; exit 1; }
echo "PASS: Renew switches, persistence/error, navigation, harness event/network/process filters, detail, rules CRUD. Artifacts: $ARTIFACTS"
