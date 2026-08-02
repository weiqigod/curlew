#!/usr/bin/env bash
# test-stack.sh — Manages the docker-compose test stack for E2E tests.
#
# Usage:
#   ./scripts/test-stack.sh up    # Build and start backend + web, then seed data
#   ./scripts/test-stack.sh down  # Tear down containers and volumes
#   ./scripts/test-stack.sh seed  # Seed test data only (stack must be running)
#
# Env vars:
#   BACKEND_URL   (default: http://localhost:5000)
#   WEB_URL       (default: http://localhost:3000)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"
WEB_URL="${WEB_URL:-http://localhost:3000}"
CMD="${1:-up}"

wait_for_url() {
  local url="$1"
  local name="$2"
  local max=60
  echo "Waiting for $name at $url..."
  for i in $(seq 1 $max); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      echo "$name is healthy (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "ERROR: $name did not become healthy within ${max}s" >&2
  return 1
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
