#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/docs/agentsight-grafana-compose.yml"
EVENT_LOG="${AGENTSIGHT_EVENT_LOG:-${HOME}/.config/agent-ebpf-filter/events.jsonl}"

if [[ -z "${AGENTSIGHT_EVENT_LOG:-}" ]]; then
  printf 'Note: the backend now defaults to events.pebble. Promtail requires a JSONL file; configure AGENT_RUNTIME_LOG_FILE_PATH=%s when using this legacy log-tail stack.\n' "${EVENT_LOG}" >&2
fi

mkdir -p "$(dirname "${EVENT_LOG}")"
touch "${EVENT_LOG}"

cd "${ROOT_DIR}/docs"
AGENTSIGHT_EVENT_LOG="${EVENT_LOG}" docker compose -f "${COMPOSE_FILE}" up -d

printf 'AgentSight Grafana stack started.\n'
printf 'Grafana: http://127.0.0.1:3000\n'
printf 'Loki:    http://127.0.0.1:3100\n'
printf 'Log:     %s\n' "${EVENT_LOG}"
