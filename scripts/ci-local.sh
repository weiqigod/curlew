#!/usr/bin/env bash
# ci-local.sh — Run the CI gate locally, scoped to what changed.
#
# This script is the gate. .github/workflows/go.yml runs `ci-local.sh --go`
# rather than restating its steps, so the two cannot drift into disagreeing
# about what "green" means; the .NET, web and Playwright gates mirrored here
# live in .github/workflows/e2e-m4.yml and e2e-m5.yml.
#
# Note that no workflow auto-triggers today — all of them are gated on
# workflow_dispatch while GitHub Actions billing is paused — so a local run of
# this script is currently the only thing that verifies a change.
#
# The Go gate always runs; .NET, web, and E2E gates run only when their files
# changed on this branch (detected via `git diff --name-only main...HEAD`).
#
# Usage:
#   ./scripts/ci-local.sh                  # Auto-scope from git diff vs main
#   ./scripts/ci-local.sh --full           # Force every gate (backend + web + e2e)
#   ./scripts/ci-local.sh --go             # Go gate only (skip docker stack)
#   ./scripts/ci-local.sh --down           # Tear down the test stack idempotently; exit 0
#   ./scripts/ci-local.sh --check-signing-keys  # SaaS lint: fail if any signing_key has NULL kms_key_id
#
# Exit codes:
#   0 — all selected gates passed
#   1 — a gate failed (the failing step is the last "=== ... ===" printed)
#   2 — invalid usage

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

MODE="${1:-auto}"

run_backend=0
run_web=0
run_ui=0
run_e2e=0

# Make sure the local view of main is fresh so scope detection is correct.
git fetch origin main --quiet 2>/dev/null || true

case "$MODE" in
  auto)
    CHANGED="$(git diff --name-only main...HEAD 2>/dev/null || git diff --name-only main..HEAD)"
    echo "Changed files vs main:"
    if [ -z "$CHANGED" ]; then
      echo "  (none — running Go gate only)"
    else
      echo "$CHANGED" | sed 's/^/  /'
    fi
    echo
    # The backend project directory is src/ApiTool.Backend — the rebrand
    # renamed the CLI, not the .NET solution. Matching on src/Curlew.Backend
    # here silently skipped the backend and E2E gates for every backend change.
    grep -qE '^src/ApiTool\.Backend'                                    <<<"$CHANGED" && run_backend=1 || true
    grep -qE '^web/'                                                    <<<"$CHANGED" && run_web=1     || true
    grep -qE '^(ui/|internal/uiserver/assets/)'                         <<<"$CHANGED" && run_ui=1      || true
    grep -qE '^(src/ApiTool\.Backend|web/|scripts/(test-stack|seed-|test-token|fake-idp)|docker-compose\.test\.yml)' \
                                                                        <<<"$CHANGED" && run_e2e=1     || true
    # Running E2E implies the backend and web gates (the stack is already up).
    if (( run_e2e )); then run_backend=1; run_web=1; fi
    ;;
  --full|full)
    run_backend=1; run_web=1; run_ui=1; run_e2e=1
    ;;
  --go|go)
    : # defaults (Go gate only)
    ;;
  --down|down)
    # Idempotent: succeeds even when no stack is running.
    cd "$REPO_ROOT"
    ./scripts/test-stack.sh down 2>/dev/null || true
    docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
    echo "=== ci-local DOWN ==="
    exit 0
    ;;
  --check-signing-keys|check-signing-keys)
    # M18-009 (v4-13): SaaS CI lint — fail when any signing_keys row has NULL kms_key_id.
    # Delegated to check-signing-keys.sh for unit-testability.
    exec "$REPO_ROOT/scripts/check-signing-keys.sh"
    ;;
  -h|--help)
    # Print the whole leading comment block. A hardcoded line range silently
    # truncated --help the first time the header grew past it.
    awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "$0"
    exit 0
    ;;
  *)
    echo "unknown mode: $MODE" >&2
    echo "usage: $0 [auto|--full|--go|--down|--check-signing-keys]" >&2
    exit 2
    ;;
esac

echo "Gates: go=1 backend=$run_backend web=$run_web ui=$run_ui e2e=$run_e2e"
echo

step() { echo; echo "=== $* ==="; }

# Cleanup is registered before the first gate runs, not next to the docker
# section it used to live beside. The dogfood gate starts a background server
# during the Go gate, and a trap installed after that point would not fire if an
# earlier step failed — leaving a listener behind for the next run to collide
# with.
mudflat_pid=""
stack_started=0
stripe_mock_started=0
cleanup() {
  if [ -n "$mudflat_pid" ]; then
    kill "$mudflat_pid" 2>/dev/null || true
    wait "$mudflat_pid" 2>/dev/null || true
  fi
  if (( stripe_mock_started )); then
    step "stripe-mock down"
    docker compose -f docker-compose.test.yml stop stripe-mock 2>/dev/null || true
  fi
  if (( stack_started )); then
    step "test-stack down"
    ./scripts/test-stack.sh down || true
  fi
}
trap cleanup EXIT

# Resolve golangci-lint (CLAUDE.md notes it lives at ~/go/bin and may not be in PATH).
lint_cmd() {
  if [ -x "$HOME/go/bin/golangci-lint" ]; then
    "$HOME/go/bin/golangci-lint" "$@"
  elif command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint "$@"
  else
    echo "golangci-lint not found (checked ~/go/bin and PATH)" >&2
    return 1
  fi
}

# Untracked node_modules (npm install in web/ or site/) can contain vendored
# Go files (e.g. flatted ships a Go port) that have no go.mod, so `./...`
# would pick them up. Filter them out of every package-list expansion.
go_pkgs() { go list ./... | grep -v '/node_modules/'; }

# --- Go gate (always) ---
step "go build"
go build -o curlew ./cmd/curlew

# M22-001: management/backlog.yaml must reconcile against management/tasks/*.yaml.
# This test also runs inside "go test" below; a named step here fails early and
# attributably, and prints the task count so the log records what was actually
# read rather than just that something passed.
#
# -count=1 matches the M7-004 step above. It is not required for correctness:
# Go's test cache tracks the files and directory listings a test reads, and it
# was measured invalidating properly for both an added and a deleted task file.
# It is kept so this gate does not rest on cache heuristics for data that lives
# outside the package, at a cost of about 0.2s.
step "backlog integrity (M22-001)"
go test ./internal/backlog/ -run '^TestBacklog_repository_is_consistent$' -count=1 -v

# M7-004: explicit named marker for the stream-discipline matrix so failures
# show up under a unique header in CI logs (one grep away).
step "go test: TestStreamDisciplineMatrix (M7-004 stream-discipline gate)"
go test -run '^TestStreamDisciplineMatrix$' ./cmd/curlew/... -count=1

step "go test"
go test $(go_pkgs)

step "go test -race"
go test -race $(go_pkgs)

step "go coverage"
go test -coverprofile=coverage.out $(go_pkgs)
go tool cover -func=coverage.out | tail -1

step "golangci-lint"
lint_cmd run

step "M18-008: guard internal/telemetry must not import internal/backend (v4-11)"
if grep -rn '"github.com/weiqigod/curlew/internal/backend"' internal/telemetry/ 2>/dev/null; then
  echo "FAIL: internal/telemetry imports internal/backend; this is forbidden by v4-11" >&2
  exit 1
fi

step "smoke-fail-lint: every FAIL echo in smoke/run.sh must stop the run"
# Regression guard: smoke checks once printed "FAIL: ..." without exit 1, so
# failed assertions did not fail the run. Flag any echo "FAIL..." that has no
# exit 1 on the same line or within the next 6 lines — use the fail() helper
# defined at the top of smoke/run.sh instead.
awk '
  /echo "FAIL/ { pend[NR] = $0 }
  /exit 1/ { for (n in pend) if (NR - n <= 6) delete pend[n] }
  END {
    status = 0
    for (n in pend) {
      printf "smoke/run.sh:%d: echo \"FAIL...\" without exit 1 nearby — use the fail() helper: %s\n", n, pend[n]
      status = 1
    }
    exit status
  }
' smoke/run.sh

step "smoke"
./smoke/run.sh

# --- Dogfood gate: curlew against mudflat ---
#
# The only step in this script that points curlew at a server curlew did not
# write. Everything above it terminates at an httptest handler or the local echo
# fixture, both of which share curlew's own assumptions about HTTP.
#
# Port 18080 rather than mudflat's default 8080, so a developer running the
# server by hand does not collide with the gate. --var overrides every other
# variable source, so the environment file stays pointed at 8080 for manual use.
#
# The testapi/ import guard (§13.1 — nothing under testapi/ may depend on
# curlew) is enforced by TestParity_NoCurlewImportsUnderTestapi in the go test
# step above, rather than restated as a grep here.
step "dogfood: build mudflat"
go build -o mudflat ./testapi/cmd/mudflat

step "dogfood: curlew against mudflat"
MUDFLAT_PORT=18080
./mudflat serve --port "$MUDFLAT_PORT" &
mudflat_pid=$!

mudflat_ready=0
for _ in $(seq 1 50); do
  # Both listeners must be up: the structured layer and the raw one on +1.
  #
  # The raw probe deliberately asks for a path that does not exist. Every real
  # raw endpoint is malformed on purpose — probing /raw/http09 made readiness
  # depend on curl accepting a bare HTTP/0.9 body, which it rightly refuses —
  # whereas the raw layer's own 404 is well-formed.
  if curl -fsS -o /dev/null "http://127.0.0.1:${MUDFLAT_PORT}/capabilities" 2>/dev/null &&
     curl -sS  -o /dev/null "http://127.0.0.1:$((MUDFLAT_PORT + 1))/raw/readiness-probe" 2>/dev/null; then
    mudflat_ready=1
    break
  fi
  sleep 0.1
done
if (( ! mudflat_ready )); then
  echo "mudflat did not become ready on port ${MUDFLAT_PORT}" >&2
  exit 1
fi

MUDFLAT_RAW_PORT=$((MUDFLAT_PORT + 1))
MUDFLAT_URL="http://127.0.0.1:${MUDFLAT_PORT}"
MUDFLAT_RAW_URL="http://127.0.0.1:${MUDFLAT_RAW_PORT}"
# The WebSocket family needs its own scheme: a protocol: websocket request is
# dialled with gorilla, which rejects an http:// URL outright.
MUDFLAT_WS_URL="ws://127.0.0.1:${MUDFLAT_PORT}"
MUDFLAT_TLS_URL="https://127.0.0.1:$((MUDFLAT_PORT + 2))"

./curlew run 'testapi/collections/*.yaml' \
  --env local \
  --var "mud=${MUDFLAT_URL}" \
  --var "raw=${MUDFLAT_RAW_URL}" \
  --var "ws=${MUDFLAT_WS_URL}" \
  --var "tls=${MUDFLAT_TLS_URL}" \
  --var "run=ci$$"

# The barrier cannot pass serially — that is what makes it a proof rather than
# a timing comparison — so it runs separately with the flag it is testing.
step "dogfood: --parallel proven by rendezvous"
./curlew run 'testapi/collections/parallel/*.yaml' \
  --env local --parallel \
  --var "mud=${MUDFLAT_URL}" \
  --var "run=cip$$"

# Phase 2 harnesses. Each asserts something no collection can express: that a
# request still fails, that no secret reached an artefact, that curl reads the
# raw layer the same way.
step "dogfood: expected failures still fail"
./testapi/harness/gaps.sh --url "${MUDFLAT_URL}"

step "dogfood: no secret reached an output artefact"
./testapi/harness/redaction.sh --url "${MUDFLAT_URL}"

step "dogfood: a slow body is still missing from the reported duration"
./testapi/harness/timing.sh --url "${MUDFLAT_URL}"

step "dogfood: the OpenAPI round trip"
./testapi/harness/openapi.sh --url "${MUDFLAT_URL}"

step "dogfood: curl agrees with the raw layer"
./testapi/harness/crosscheck.sh --raw-url "${MUDFLAT_RAW_URL}"

kill "$mudflat_pid" 2>/dev/null || true
wait "$mudflat_pid" 2>/dev/null || true
mudflat_pid=""

# --- Test stack (only if an E2E gate will run) ---
if (( run_e2e )); then
  step "test-stack up"
  ./scripts/test-stack.sh up
  stack_started=1
fi

# --- Backend gate ---
if (( run_backend )); then
  # Stripe integration tests need stripe-mock on :12111 (Spec :6862, :6874).
  # ~30s container startup, runs once per CI job.
  step "stripe-mock up"
  docker compose -f docker-compose.test.yml up -d stripe-mock
  stripe_mock_started=1
  # Wait for stripe-mock to become ready (up to 30 attempts × 1s).
  for i in $(seq 1 30); do
    status="$(curl -s -o /dev/null -w '%{http_code}' http://localhost:12111/v1/customers \
        -H "Authorization: Bearer sk_test_dummy" 2>/dev/null || echo "000")"
    if [ "$status" = "200" ] || [ "$status" = "401" ] || [ "$status" = "404" ]; then
      echo "stripe-mock is ready (attempt $i, status=$status)"
      break
    fi
    sleep 1
    [ "$i" = "30" ] && { echo "stripe-mock did not become ready" >&2; exit 1; }
  done

  step "dotnet test"
  # APITOOL__ prefix: the backend binds options under the "ApiTool:" root
  # (StripeOptions.Section). The CLI rebrand did not rename it.
  APITOOL__STRIPE__APIBASE="http://localhost:12111" \
  APITOOL__STRIPE__APIKEY="sk_test_123" \
    dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
fi

# --- Web gate ---
if (( run_web )); then
  step "web: npm ci"
  ( cd web && npm ci )

  step "web: svelte-check"
  ( cd web && npm run check )

  step "web: lint"
  ( cd web && npm run lint )

  step "web: test:unit"
  ( cd web && npm run test:unit )

  step "web: build"
  ( cd web && npm run build )
fi

# --- curlew ui SPA gate (UI_SPECIFICATION.md §13.4) ---
if (( run_ui )); then
  step "ui: npm ci"
  ( cd ui && npm ci )

  step "ui: svelte-check"
  ( cd ui && npm run check )

  step "ui: lint"
  ( cd ui && npm run lint )

  step "ui: vitest"
  ( cd ui && npm run test )

  step "ui: build"
  ( cd ui && npm run build )
fi

# --- E2E gate ---
if (( run_e2e )); then
  step "web: playwright install"
  ( cd web && npx playwright install --with-deps chromium )

  # The convergence specs seed the backend over HTTP and mint their own
  # tokens, so there is no CLI binary to build and no token to export here.
  export CURLEW_BACKEND_URL="${CURLEW_BACKEND_URL:-http://localhost:5000}"

  step "playwright: convergence specs"
  ( cd web && npx playwright test \
      tests/e2e/full-pipeline.spec.ts \
      tests/e2e/enterprise-full.spec.ts \
      tests/e2e/m14-revenue-loop.spec.ts \
      tests/e2e/m16-happy-path.spec.ts \
      tests/e2e/m18-compliance.spec.ts )
fi

if [ "${CURLEW_RUN_SELF_HOSTED:-0}" = "1" ]; then
  step "self-hosted smoke"
  ./scripts/test-self-hosted.sh
fi

echo
echo "=== ci-local PASS ==="
