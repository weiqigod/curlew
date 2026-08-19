#!/usr/bin/env bash
# test-stack.sh — Manages the docker-compose test stack for E2E tests.
#
# Usage:
#   ./scripts/test-stack.sh up    # Build and start backend + web, then seed data
#   ./scripts/test-stack.sh down  # Tear down containers and volumes
#   ./scripts/test-stack.sh seed  # Seed test data only (stack must be running)
#
# Env vars:
#   BACKEND_URL           (default: http://localhost:5000)
#   WEB_URL               (default: http://localhost:3000)
#   STACK_HEALTH_TIMEOUT  (default: 60) seconds to wait for each service to
#                         report healthy before giving up

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"
WEB_URL="${WEB_URL:-http://localhost:3000}"
# How long to wait for a service to report healthy. 60s is generous against
# the measurement: on 2026-08-19 the backend container reached "Application
# started" 1.2s after `docker compose up -d` returned. It is an env var rather
# than a literal so a genuinely slow host can raise it without editing this
# script -- but raising it is the wrong reflex for the failure this most often
# produces, which diagnose_url below exists to name.
STACK_HEALTH_TIMEOUT="${STACK_HEALTH_TIMEOUT:-60}"
CMD="${1:-up}"

wait_for_url() {
  local url="$1"
  local name="$2"
  local max="$STACK_HEALTH_TIMEOUT"
  echo "Waiting for $name at $url (up to ${max}s)..."
  for i in $(seq 1 $max); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "$name is healthy (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "ERROR: $name did not become healthy within ${max}s" >&2
  diagnose_url "$url"
  return 1
}

# A wait that times out has two causes, and the message above cannot tell them
# apart. Either nothing is listening yet (the service is slow, and raising
# STACK_HEALTH_TIMEOUT is the fix), or something *else* owns the port and has
# been answering the whole time -- which is not a timeout at all, and no window
# is long enough. The second has a specific culprit on macOS: AirPlay Receiver
# binds :5000 and answers every request 403, so a perfectly healthy backend
# looks like one that never started. Measured on this repository 2026-08-19,
# which is why this function exists.
diagnose_url() {
  local url="$1"
  local code
  # curl already prints 000 on a connection failure, so `|| echo 000` would
  # concatenate two of them ("000000") and take the squatter branch below for a
  # port nothing is listening on. Swallow the exit status instead.
  code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$url" 2>/dev/null || true)"
  if [ -z "$code" ] || [ "$code" = "000" ]; then
    echo "  Nothing is listening at $url -- the service never started." >&2
    echo "  Check the container logs above; raise STACK_HEALTH_TIMEOUT if it is simply slow." >&2
    return 0
  fi
  local server
  server="$(curl -s -D - -o /dev/null --max-time 5 "$url" 2>/dev/null \
    | awk -F': ' 'tolower($1) == "server" { sub(/\r/, "", $2); print $2; exit }')"
  echo "  Something IS answering at $url with HTTP ${code}${server:+ (Server: ${server})}." >&2
  echo "  This is not a slow start and no timeout is long enough: another process" >&2
  echo "  owns that port. On macOS this is usually AirPlay Receiver on :5000 --" >&2
  echo "  turn it off under System Settings > General > AirDrop & Handoff." >&2
}

dump_logs_on_failure() {
  echo "==================== backend logs ====================" >&2
  docker compose -f docker-compose.test.yml logs --tail 200 backend >&2 || true
  echo "==================== fake-idp logs ====================" >&2
  docker compose -f docker-compose.test.yml logs --tail 100 fake-idp >&2 || true
}

case "$CMD" in
  up)
    cd "$REPO_ROOT"
    # Install the ERR trap before `compose up --build` so a build-time failure
    # still dumps whatever container logs exist (and is visible at the top of
    # the CI log stream rather than swallowed by compose's own stderr).
    trap dump_logs_on_failure ERR
    docker compose -f docker-compose.test.yml up --build -d
    wait_for_url "$BACKEND_URL/swagger/v1/swagger.json" "backend"
    wait_for_url "$WEB_URL/" "web"
    wait_for_url "http://localhost:8088/healthz" "fake-idp"
    "$REPO_ROOT/scripts/seed-test-data.sh"
    "$REPO_ROOT/scripts/seed-enterprise.sh" acme qa-lead "results.upload,results.view,dashboard.view"
    trap - ERR
    echo "Test stack is ready."
    ;;
  down)
    cd "$REPO_ROOT"
    docker compose -f docker-compose.test.yml down -v
    echo "Test stack torn down."
    ;;
  seed)
    "$REPO_ROOT/scripts/seed-test-data.sh"
    ;;
  *)
    echo "Usage: $0 {up|down|seed}" >&2
    exit 2
    ;;
esac
