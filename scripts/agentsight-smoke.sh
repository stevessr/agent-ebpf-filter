#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
API_KEY="${AGENT_API_KEY:-}"

headers=()
if [[ -n "${API_KEY}" ]]; then
  headers=(-H "X-API-KEY: ${API_KEY}")
fi

curl_json() {
  local path="$1"
  curl -fsS "${headers[@]}" "${BASE_URL}${path}"
}

printf 'Checking AgentSight health counters...\n'
health_json="$(curl_json '/api/v1/health')"
python3 -c 'import json, sys
payload = json.loads(sys.argv[1])
collector = payload.get("collector", {})
print(json.dumps({
    "status": payload.get("status"),
    "tlsCaptureEnabled": payload.get("features", {}).get("tlsCaptureEnabled"),
    "agentSightCountersTotal": collector.get("agentSightCountersTotal", {}),
}, indent=2))' "$health_json"

printf 'Checking filtered recent events API...\n'
events_json="$(curl_json '/api/v1/events/recent?limit=25&redaction_state=sanitized')"
python3 -c 'import json, sys
payload = json.loads(sys.argv[1])
events = payload.get("events", [])
print(json.dumps({
    "source": payload.get("source"),
    "events": len(events),
    "hasEnvelope": any(bool(item.get("Envelope")) for item in events),
}, indent=2))' "$events_json"

printf 'AgentSight smoke checks completed.\n'
