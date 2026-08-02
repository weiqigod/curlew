#!/usr/bin/env bash
# Tiny apitool device-code stub server. Reusable by M14-005, M14-006, M14-007, M14-021.
#
# Usage:
#   ./testdata/m14/stub-backend.sh start              # foreground
#   ./testdata/m14/stub-backend.sh start --bg         # background; writes stub.pid
#   ./testdata/m14/stub-backend.sh stop               # kills stub.pid
#   ./testdata/m14/stub-backend.sh inject <status> [code]
#                                               # inject a one-shot response for
#                                               # the next /auth/refresh call
#                                               # e.g. inject 500
#                                               #      inject 401 AUTH_REFRESH_EXPIRED
#
# Environment:
#   PORT           Listen port (default: 18080)
#   STUB_BEHAVIOR  Comma-separated poll script, e.g. "pending,success".
#                  Elements: pending, slow_down, expired, success.
#                  Default: "pending,success".
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

case "${1:-start}" in
  start)
    if [[ "${2:-}" == "--bg" ]]; then
      go run "$SCRIPT_DIR/stub_server.go" >"$SCRIPT_DIR/stub.log" 2>&1 &
      echo $! >"$SCRIPT_DIR/stub.pid"
      echo "stub-backend started (pid $(cat "$SCRIPT_DIR/stub.pid"))"
    else
      exec go run "$SCRIPT_DIR/stub_server.go"
    fi
    ;;
  stop)
    if [[ -f "$SCRIPT_DIR/stub.pid" ]]; then
      kill "$(cat "$SCRIPT_DIR/stub.pid")" 2>/dev/null || true
      rm -f "$SCRIPT_DIR/stub.pid"
      echo "stub-backend stopped"
    else
      echo "no stub.pid found; nothing to stop"
    fi
    # Belt-and-suspenders: also kill any surviving process still listening on
    # the port. This handles interrupted runs that left a stale stub alive.
    port="${PORT:-18080}"
    lsof -ti "tcp:${port}" 2>/dev/null | xargs kill -9 2>/dev/null || true
    ;;
  inject)
    status="${2:-500}"
    code="${3:-}"
    port="${PORT:-18080}"
    url="http://127.0.0.1:${port}/__inject?status=${status}"
    if [[ -n "$code" ]]; then
      url="${url}&code=${code}"
    fi
    curl -s -X POST "$url" >/dev/null
    echo "stub-backend: injected status=${status} code=${code} for next /auth/refresh"
    ;;
  *)
    echo "usage: stub-backend.sh {start [--bg] | stop | inject <status> [code]}" >&2
    exit 1
    ;;
esac
