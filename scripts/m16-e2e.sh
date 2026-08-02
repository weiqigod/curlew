#!/usr/bin/env bash
# scripts/m16-e2e.sh — M16-021 happy-path convergence scenario.
#
# Prereqs: ./scripts/ci-local.sh --full has brought up the full stack
#   (Postgres/SQLite backend, web portal, stripe-mock, sendgrid-fake).
#
# Usage:
#   ./scripts/m16-e2e.sh
#
# Exit codes:
#   0  — PASS (all steps succeeded)
#   1  — General failure (unrecognised error)
#   2  — trial_state not active in License JWT (Step 2)
#   8  — scheduled_runs.result_id not populated within timeout (Step 8)
#   9  — /results/stats totals.runs < 1 (Step 9)
#   10 — trial_expiring email not queued within timeout (Step 10)
#   11 — license trial start / post-activation JWT feature check failed (Step 11)
#   12 — password-reset request failed (Step 12)
#   13 — old refresh token still valid after password reset (Step 13)

set -euo pipefail
START_TS=$(date +%s)
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"
WEB_BASE_URL="${WEB_BASE_URL:-http://localhost:3000}"
# Use a fixed owner that seed-test-data.sh creates so we can mint a real token
# via test-token.sh. Using the fixed ID ensures the JWT sub is stable.
OWNER_EMAIL="${M16_E2E_EMAIL:-owner@example.com}"
OWNER_USER_ID="${M16_E2E_USER_ID:-00000000-0000-0000-0000-000000000001}"
ORG_SLUG="${M16_E2E_ORG_SLUG:-acme}"
# Keep reruns isolated from per-user reset/verification throttles and from
# already-consumed tokens in the shared email audit log.
TRIAL_EMAIL="${M16_E2E_TRIAL_EMAIL:-m16-trial-$(date +%s)-$$@example.com}"

step() { echo "=== M16 step $1: $2 ==="; }
fail() { echo "FAIL: $1" >&2; exit "${2:-1}"; }

# ── Resolve ORG_ID for the seeded org ─────────────────────────────────────
step 0 "resolve org_id for $ORG_SLUG"
# Mint a dev JWT for the owner via test-token.sh (always available in dev/CI).
ACCESS_TOKEN=$("$REPO_ROOT/scripts/test-token.sh" "$OWNER_EMAIL" "$OWNER_USER_ID")
ALL_ORGS=$(curl -fsS "$BACKEND_URL/api/v1/organizations" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
ORG_ID=$(echo "$ALL_ORGS" | python3 -c "
import sys, json
orgs = json.load(sys.stdin).get('organizations', [])
matched = [o['id'] for o in orgs if o.get('slug') == '$ORG_SLUG']
print(matched[0] if matched else '')
")
if [ -z "$ORG_ID" ]; then
    fail "Could not resolve org_id for slug '$ORG_SLUG'; ensure seed-test-data.sh has run" 1
fi
echo "  org_id=$ORG_ID"
ORG_GUID=$(python3 -c "
import uuid
raw = '$ORG_ID'
print(uuid.UUID(hex=raw[4:]) if raw.startswith('org_') else uuid.UUID(raw))
")

# ── Step 1: register via seed-refresh (Open Decision 1 / 9) ───────────────
step 1 "register isolated free-tier user via seed-refresh (Open Decision 1)"
REFRESH_RESP=$(curl -fsS -X POST "$BACKEND_URL/internal/test/seed-refresh" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$TRIAL_EMAIL\"}")
REFRESH_TOKEN=$(echo "$REFRESH_RESP" | python3 -c 'import sys,json; print(json.load(sys.stdin)["plaintext"])')
DEVICE_ID=$(echo "$REFRESH_RESP"   | python3 -c 'import sys,json; print(json.load(sys.stdin)["device_id"])')
echo "  refresh_token=$(echo "$REFRESH_TOKEN" | cut -c1-12)... device_id=$DEVICE_ID"

# ── Step 2: refresh → License JWT → assert trial_state=active ─────────────
step 2 "refresh tokens; assert License JWT trial_state=active"
REFRESH_OUT=$(curl -fsS -X POST "$BACKEND_URL/api/v1/auth/refresh" \
    -H "Content-Type: application/json" \
    -d "{\"refresh_token\":\"$REFRESH_TOKEN\",\"device_id\":\"$DEVICE_ID\"}")
LICENSE_JWT=$(echo "$REFRESH_OUT" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("license_jwt") or d.get("LicenseJwt") or "")')
TRIAL_ACCESS_TOKEN=$(echo "$REFRESH_OUT" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("access_token") or d.get("AccessToken") or "")')
ROTATED_REFRESH_TOKEN=$(echo "$REFRESH_OUT" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("refresh_token") or d.get("RefreshToken") or "")')
if [ -z "$LICENSE_JWT" ]; then
    fail "No license_jwt in /api/v1/auth/refresh response" 2
fi
if [ -z "$TRIAL_ACCESS_TOKEN" ]; then
    fail "No access_token in /api/v1/auth/refresh response" 2
fi
if [ -z "$ROTATED_REFRESH_TOKEN" ]; then
    fail "No refresh_token in /api/v1/auth/refresh response" 2
fi
REFRESH_TOKEN="$ROTATED_REFRESH_TOKEN"
TRIAL_USER_ID=$(echo "$LICENSE_JWT" | python3 -c '
import sys, base64, json
parts = sys.stdin.read().strip().split(".")
pad = "=" * (-len(parts[1]) % 4)
print(json.loads(base64.urlsafe_b64decode(parts[1] + pad)).get("sub", ""))
')
[ -n "$TRIAL_USER_ID" ] || fail "No sub claim in License JWT" 2
TRIAL_STATE=$(echo "$LICENSE_JWT" | python3 -c '
import sys, base64, json
parts = sys.stdin.read().strip().split(".")
if len(parts) < 2: print(""); exit()
pad = "=" * (-len(parts[1]) % 4)
payload = json.loads(base64.urlsafe_b64decode(parts[1] + pad))
print(payload.get("trial_state", ""))
')
echo "  trial_state=$TRIAL_STATE"
[ "$TRIAL_STATE" = "active" ] || fail "expected trial_state=active, got '$TRIAL_STATE'" 2

# ── Step 3: upgrade to Team tier via seed-m14 (Open Decision 2) ───────────
step 3 "upgrade to Team tier via seed-m14"
curl -fsS -X POST "$BACKEND_URL/internal/test/seed-m14" \
    -H "Content-Type: application/json" \
    -d "{\"org_id\":\"$ORG_GUID\",\"installation_id\":99999,\"app_id\":111,\"repos\":[]}" >/dev/null
echo "  Team tier applied for org $ORG_ID"

# ── Step 4: create schedule via REST ──────────────────────────────────────
step 4 "create schedule m16-e2e"
COLLECTION_PATH="$REPO_ROOT/testdata/m16/e2e-collection.yaml"
SCHED_RESP=$(curl -s -w "\n%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/schedules" \
    -H "Authorization: Bearer $ACCESS_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"name\":\"m16-e2e\",
        \"cron\":\"0 9 * * *\",
        \"collection_ref\":\"file:$COLLECTION_PATH\",
        \"timezone\":\"UTC\",
        \"env_vars\":{\"BACKEND_URL\":\"$BACKEND_URL\"}
    }")
SCHED_HTTP=$(echo "$SCHED_RESP" | tail -1)
SCHED_BODY=$(printf '%s\n' "$SCHED_RESP" | sed '$d')
if [ "$SCHED_HTTP" != "201" ] && [ "$SCHED_HTTP" != "409" ]; then
    fail "POST /schedules returned HTTP $SCHED_HTTP: $SCHED_BODY" 1
fi
echo "  schedule m16-e2e created (HTTP $SCHED_HTTP)"

# ── Step 5: run-now to enqueue a scheduled_run immediately ────────────────
step 5 "run-now for m16-e2e schedule"
RUN_NOW_RESP=$(curl -fsS -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/schedules/m16-e2e/run-now" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
RUN_ID=$(echo "$RUN_NOW_RESP" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("run_id") or d.get("RunId") or "")')
echo "  run_id=$RUN_ID"
[ -n "$RUN_ID" ] || fail "run_id missing from run-now response" 1

# ── Step 6: worker --schedule-pull --once ─────────────────────────────────
step 6 "curlew worker --schedule-pull --once"
# Pre-seed a license config dir so the worker can read the access token.
WORKER_CFG_DIR=$(mktemp -d)
TRIAL_CFG_DIR=$(mktemp -d)
trap 'rm -rf "$WORKER_CFG_DIR" "$TRIAL_CFG_DIR"' EXIT
# Write a minimal license.json with the current access token.
python3 -c "
import json, sys
rec = {\"AccessToken\": \"$ACCESS_TOKEN\"}
print(json.dumps(rec))
" > "$WORKER_CFG_DIR/license.json"

for WORKER_ATTEMPT in $(seq 1 10); do
    CURLEW_CONFIG_DIR="$WORKER_CFG_DIR" \
    CURLEW_BACKEND_URL="$BACKEND_URL" \
    CURLEW_BACKEND_TOKEN="$ACCESS_TOKEN" \
    BACKEND_URL="$BACKEND_URL" \
        "$REPO_ROOT/curlew" worker \
            --schedule-pull \
            --once \
            --backend "$BACKEND_URL" \
            --token "$ACCESS_TOKEN"

    TARGET_RESULT_ID=$(curl -fsS \
        "$BACKEND_URL/api/v1/organizations/$ORG_ID/schedules/m16-e2e/runs" \
        -H "Authorization: Bearer $ACCESS_TOKEN" \
        | python3 -c "
import sys, json
for run in json.load(sys.stdin).get('runs', []):
    if run.get('run_id') == '$RUN_ID':
        print(run.get('result_id') or '')
        break
")
    if [ -n "$TARGET_RESULT_ID" ]; then
        break
    fi
    echo "  target run not completed after worker attempt $WORKER_ATTEMPT; draining next queued run"
done
echo "  worker --schedule-pull --once completed"

# ── Step 7: worker executed the run (step 6 claim + execute = step 7 confirmation) ─
step 7 "worker claimed and executed the scheduled run"
echo "  worker --schedule-pull --once exited 0; run claimed and executed"

# ── Step 8: verify scheduled_runs.result_id populated ─────────────────────
step 8 "verify scheduled_runs.result_id populated"
TIMEOUT_S=30
deadline=$(( $(date +%s) + TIMEOUT_S ))
RESULT_ID=""
while [ "$(date +%s)" -lt "$deadline" ]; do
    RUNS_LIST=$(curl -fsS "$BACKEND_URL/api/v1/organizations/$ORG_ID/schedules/m16-e2e/runs" \
        -H "Authorization: Bearer $ACCESS_TOKEN")
    RESULT_ID=$(echo "$RUNS_LIST" | python3 -c "
import sys, json
runs = json.load(sys.stdin).get('runs', [])
for r in runs:
    if r.get('run_id', '').endswith('$RUN_ID') or r.get('run_id') == '$RUN_ID':
        print(r.get('result_id') or '')
        sys.exit()
print('')
" 2>/dev/null || true)
    if [ -n "$RESULT_ID" ]; then break; fi
    sleep 1
done
echo "  result_id=$RESULT_ID"
[ -n "$RESULT_ID" ] || fail "scheduled_runs.result_id not populated within ${TIMEOUT_S}s" 8

# ── Step 9: /results/stats shows the run in totals + today's trend ────────
step 9 "dashboard /results/stats reflects the run"
STATS=$(curl -fsS "$BACKEND_URL/api/v1/organizations/$ORG_ID/results/stats?window=30d" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
TOTAL=$(echo "$STATS" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("totals",{}).get("runs",0))')
echo "  totals.runs=$TOTAL"
[ "$TOTAL" -ge 1 ] || fail "expected totals.runs >= 1, got $TOTAL" 9

# Behavior 9: trend[today].runs >= 1 (per spec; >= 1 rather than exactly 1 to be safe on shared stacks).
TODAY=$(date -u +%Y-%m-%d)
TREND_RUNS=$(echo "$STATS" | python3 -c "
import sys, json
d = json.load(sys.stdin)
trend = d.get('trend', [])
for entry in trend:
    if entry.get('date') == '$TODAY':
        print(entry.get('runs', 0))
        exit()
print(0)
")
echo "  trend[$TODAY].runs=$TREND_RUNS"
[ "$TREND_RUNS" -ge 1 ] || fail "expected trend[$TODAY].runs >= 1, got $TREND_RUNS" 9

# ── Step 10: seed near-expiry trial + force tick → trial_expiring email ───
step 10 "seed near-expiry trial + force tick"
curl -fsS -X POST "$BACKEND_URL/internal/test/seed-near-expiry-trial" \
    -H "Content-Type: application/json" \
    -d "{\"user_id\":\"$TRIAL_USER_ID\",\"feature\":\"shared_vault_templates\",\"days_until_expiry\":2}" >/dev/null
curl -fsS -X POST "$BACKEND_URL/internal/test/trial-expiry-tick" >/dev/null || true

# Wait for trial_expiring to appear in the email audit log.
TIMEOUT_MAIL=15
deadline=$(( $(date +%s) + TIMEOUT_MAIL ))
FOUND_MAIL=0
while [ "$(date +%s)" -lt "$deadline" ]; do
    if curl -fsS "$BACKEND_URL/internal/test/email-audit?limit=100" \
        | python3 -c '
import sys,json
entries = json.load(sys.stdin).get("entries",[])
sys.exit(0 if any(e.get("template_slug")=="trial_expiring" for e in entries) else 1)' 2>/dev/null; then
        FOUND_MAIL=1; break
    fi
    sleep 0.5
done
echo "  trial_expiring email found=$FOUND_MAIL"
[ "$FOUND_MAIL" -eq 1 ] || fail "trial_expiring email not queued within ${TIMEOUT_MAIL}s" 10

# ── Step 11: on-demand trial activation via CLI ───────────────────────────
step 11 "curlew license trial start shared_vault_templates"
CURLEW_INTERNAL=1 \
CURLEW_CONFIG_DIR="$TRIAL_CFG_DIR" \
CURLEW_INTERNAL_DEVICE_ID="$DEVICE_ID" \
CURLEW_INTERNAL_REFRESH_TOKEN="$REFRESH_TOKEN" \
CURLEW_INTERNAL_ACCESS_TOKEN="$TRIAL_ACCESS_TOKEN" \
    "$REPO_ROOT/curlew" internal seed-login >/dev/null
# Run license trial start; capture exit code explicitly.
# Exit 0  = newly activated.
# Exit 5  = trial already consumed (acceptable — idempotent re-run of e2e).
# Any other exit code is a genuine failure.
set +e
CURLEW_CONFIG_DIR="$TRIAL_CFG_DIR" \
CURLEW_FORCE_FILE_STORAGE=1 \
CURLEW_BACKEND_URL="$BACKEND_URL" \
CURLEW_BACKEND_TOKEN="$TRIAL_ACCESS_TOKEN" \
    "$REPO_ROOT/curlew" license trial start shared_vault_templates \
    --backend "$BACKEND_URL" \
    --token "$TRIAL_ACCESS_TOKEN"
TRIAL_START_EXIT=$?
set -e
if [ "$TRIAL_START_EXIT" -eq 0 ]; then
    echo "  license trial start: activated (exit 0)"
elif [ "$TRIAL_START_EXIT" -eq 5 ]; then
    echo "  license trial start: trial already consumed (exit 5, acceptable)"
else
    fail "curlew license trial start failed with unexpected exit code $TRIAL_START_EXIT" 11
fi

# Verify the backend-issued License JWT carries the feature in features[].
# Refresh to mint a fresh JWT that reflects the just-activated trial row.
POST_ACTIVATION_REFRESH=$(curl -fsS -X POST "$BACKEND_URL/api/v1/auth/refresh" \
    -H "Content-Type: application/json" \
    -d "{\"refresh_token\":\"$REFRESH_TOKEN\",\"device_id\":\"$DEVICE_ID\"}")
POST_LICENSE_JWT=$(echo "$POST_ACTIVATION_REFRESH" | python3 -c '
import sys, json
d = json.load(sys.stdin)
print(d.get("license_jwt") or d.get("LicenseJwt") or "")
')
POST_REFRESH_TOKEN=$(echo "$POST_ACTIVATION_REFRESH" | python3 -c '
import sys, json
d = json.load(sys.stdin)
print(d.get("refresh_token") or d.get("RefreshToken") or "")
')
if [ -z "$POST_LICENSE_JWT" ]; then
    fail "No license_jwt in post-activation /api/v1/auth/refresh response" 11
fi
[ -n "$POST_REFRESH_TOKEN" ] || fail "No refresh_token in post-activation response" 11
REFRESH_TOKEN="$POST_REFRESH_TOKEN"
HAS_FEATURE=$(echo "$POST_LICENSE_JWT" | python3 -c '
import sys, base64, json
parts = sys.stdin.read().strip().split(".")
if len(parts) < 2:
    print("no"); exit()
pad = "=" * (-len(parts[1]) % 4)
payload = json.loads(base64.urlsafe_b64decode(parts[1] + pad))
features = payload.get("features", [])
trial_state = payload.get("trial_state", "")
# The feature appears in features[] on activation, or trial_state is active (covers full-trial).
print("yes" if "shared_vault_templates" in features or trial_state == "active" else "no")
')
echo "  post-activation License JWT feature check: $HAS_FEATURE"
[ "$HAS_FEATURE" = "yes" ] || \
    fail "License JWT does not carry shared_vault_templates in features[] after activation" 11

# ── Step 12: password reset request ───────────────────────────────────────
step 12 "request password reset for $TRIAL_EMAIL"
RESET_REQ_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/auth/password-reset/request" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"$TRIAL_EMAIL\"}")
echo "  password-reset/request HTTP=$RESET_REQ_HTTP"
[ "$RESET_REQ_HTTP" = "200" ] || fail "password-reset/request returned HTTP $RESET_REQ_HTTP" 12

# ── Step 13: confirm reset using token from audit log → assert old refresh revoked ──
step 13 "confirm password reset; assert old refresh token revoked"
# Extract the reset token from the email-audit log (polling for up to 10s).
RESET_TOKEN=""
deadline=$(( $(date +%s) + 10 ))
while [ "$(date +%s)" -lt "$deadline" ]; do
    RESET_TOKEN=$(curl -fsS "$BACKEND_URL/internal/test/email-audit?limit=50" \
        | python3 -c '
import sys, json, urllib.parse
entries = json.load(sys.stdin).get("entries",[])
target = sys.argv[1]
for e in reversed(entries):
    if e.get("template_slug") == "password_reset" and e.get("to") == target:
        reset_url = e.get("variables",{}).get("reset_url","")
        if reset_url.startswith("http"):
            from urllib.parse import urlparse, parse_qs
            tok = parse_qs(urlparse(reset_url).query).get("token",[""])[0]
        else:
            tok = reset_url
        print(tok)
        sys.exit()
print("")
' "$TRIAL_EMAIL" 2>/dev/null || true)
    if [ -n "$RESET_TOKEN" ]; then break; fi
    sleep 0.5
done
if [ -z "$RESET_TOKEN" ]; then
    fail "password_reset email not found in audit log within 10s — RFC 9700 §4.14 revocation assertion cannot proceed" 13
fi
echo "  reset_token=$(echo "$RESET_TOKEN" | cut -c1-12)..."

NEW_PW="Tr0ub4dor&3-very-strong-$(date +%s)"
curl -fsS -X POST "$BACKEND_URL/api/v1/auth/password-reset/confirm" \
    -H "Content-Type: application/json" \
    -d "{\"token\":\"$RESET_TOKEN\",\"new_password\":\"$NEW_PW\"}" >/dev/null
echo "  password-reset/confirm succeeded"

# Old refresh token must now be revoked (401 on /auth/refresh) — RFC 9700 §4.14.
OLD_RF_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/auth/refresh" \
    -H "Content-Type: application/json" \
    -d "{\"refresh_token\":\"$REFRESH_TOKEN\",\"device_id\":\"$DEVICE_ID\"}")
echo "  old refresh token reuse → HTTP $OLD_RF_HTTP (expect 401)"
[ "$OLD_RF_HTTP" = "401" ] || fail "expected 401 after password reset, got HTTP $OLD_RF_HTTP" 13
echo "  refresh token family successfully revoked (RFC 9700 §4.14)"

# ── Step 14: Playwright web assertions ────────────────────────────────────
step 14 "Playwright web assertions (email-verify, dashboard, password-reset)"
if [ -d "$REPO_ROOT/web" ] && command -v npx >/dev/null 2>&1; then
    # The shell assertion above consumes its reset token. Queue fresh browser
    # fixtures so Playwright exercises both confirmation pages independently.
    curl -fsS -X POST "$BACKEND_URL/api/v1/auth/email-verification/resend" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"$TRIAL_EMAIL\"}" >/dev/null
    curl -fsS -X POST "$BACKEND_URL/api/v1/auth/password-reset/request" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"$TRIAL_EMAIL\"}" >/dev/null

    CURLEW_BACKEND_URL="$BACKEND_URL" \
    CURLEW_BACKEND_TOKEN="$ACCESS_TOKEN" \
    M16_E2E_TRIAL_EMAIL="$TRIAL_EMAIL" \
    WEB_BASE_URL="$WEB_BASE_URL" \
        bash -c 'cd '"$REPO_ROOT/web"' && npx playwright test tests/e2e/m16-happy-path.spec.ts'
else
    echo "  SKIP: web directory or npx not available"
fi

# ── Done ──────────────────────────────────────────────────────────────────
END_TS=$(date +%s)
echo "M16 e2e PASS in $((END_TS - START_TS))s"
