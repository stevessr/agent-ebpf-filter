#!/usr/bin/env bash
set -Eeuo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
case "${1:-help}" in
  install|refresh) exec bash "$root/scripts/codex-cloud/install.sh" ;; # frozen resolution; never upgrade dependencies
  doctor) exec bash "$root/scripts/codex-cloud/validate.sh" doctor ;;
  verify) exec bash "$root/scripts/codex-cloud/validate.sh" full ;;
  start) shift; exec bash "$root/scripts/codex-cloud/start.sh" "${1:-frontend}" ;;
  stop) exec bash "$root/scripts/codex-cloud/start.sh" stop ;;
  cache) source "$root/scripts/codex-cloud/env.sh"; du -sh "$CODEX_CACHE_HOME" 2>/dev/null || true ;;
  *) echo 'Usage: maintain.sh {install|refresh|doctor|verify|start [frontend|docs|all|status]|stop|cache}' ;;
esac
