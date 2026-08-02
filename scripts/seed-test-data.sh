#!/usr/bin/env bash
# seed-test-data.sh — Seeds the test stack with fixture data for E2E tests.
#
# Idempotent: checks if the 'acme' org already exists before creating it.
# Requires: curl, jq, ./scripts/test-token.sh
#
# Env vars:
#   BACKEND_URL         (default: http://localhost:5000)
#   SEED_OWNER_EMAIL    (default: owner@example.com)
#   SEED_ORG_SLUG       (default: acme)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"
OWNER_EMAIL="${SEED_OWNER_EMAIL:-owner@example.com}"
ORG_SLUG="${SEED_ORG_SLUG:-acme}"
# Fixed owner UUID — must match the token minted by test-token.sh in CI.
# SYNC-NOTE: the default value is mirrored by the `SEEDED_USER_IDS` map in
# web/tests/e2e/helpers/auth.ts ('owner@example.com' entry). If you change it
# here, change it there too — seedAuthCookie relies on the pairing to hold
# a stable JWT sub across calls.
OWNER_USER_ID="${SEED_OWNER_USER_ID:-00000000-0000-0000-0000-000000000001}"

# Mint a dev JWT for the owner using the stable user ID
TOKEN=$("$REPO_ROOT/scripts/test-token.sh" "$OWNER_EMAIL" "$OWNER_USER_ID")

# Create the org (idempotent: ignore 409 Conflict)
ORG_RESPONSE=$(curl -s -w "\n%{http_code}" \
  -X POST "$BACKEND_URL/api/v1/organizations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Acme\",\"slug\":\"$ORG_SLUG\"}")

HTTP_CODE=$(echo "$ORG_RESPONSE" | tail -1)
ORG_BODY=$(printf '%s\n' "$ORG_RESPONSE" | sed '$d')

if [ "$HTTP_CODE" = "409" ]; then
  echo "Org '$ORG_SLUG' already exists, skipping creation."
  # Fetch existing org id
  ALL_ORGS=$(curl -s "$BACKEND_URL/api/v1/organizations" \
    -H "Authorization: Bearer $TOKEN")
  ORG_ID=$(echo "$ALL_ORGS" | jq -r ".organizations[] | select(.slug == \"$ORG_SLUG\") | .id")
elif [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
  ORG_ID=$(echo "$ORG_BODY" | jq -r '.id')
  echo "Created org '$ORG_SLUG' with id $ORG_ID"
else
  echo "Unexpected HTTP $HTTP_CODE creating org:" >&2
  echo "$ORG_BODY" >&2
  exit 1
fi

if [ -z "$ORG_ID" ] || [ "$ORG_ID" = "null" ]; then
  echo "ERROR: could not determine org id" >&2
  exit 1
fi

echo "Seeding results for org $ORG_ID..."

# Post each fixture file (skip failing-result.json — seeded after the notification rule)
# Idempotent: count existing results; skip posting if already seeded.
FIXTURE_DIR="$REPO_ROOT/testdata/web/seed-results"

# Count non-failing fixture files we intend to seed.
FIXTURE_COUNT=$(find "$FIXTURE_DIR" -maxdepth 1 -name "*.json" ! -name "failing-result.json" | wc -l | tr -d ' ')

EXISTING_RESULTS=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$BACKEND_URL/api/v1/organizations/$ORG_ID/results?limit=100" \
  | jq '.results | length' 2>/dev/null || echo "0")

if [ "$EXISTING_RESULTS" -ge "$FIXTURE_COUNT" ]; then
  echo "  $EXISTING_RESULTS result(s) already present (expected $FIXTURE_COUNT), skipping fixture posts."
else
  for fixture in "$FIXTURE_DIR"/*.json; do
    NAME=$(basename "$fixture")
    if [ "$NAME" = "failing-result.json" ]; then
      continue
    fi
    HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
      -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/results" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "@$fixture")
    echo "  $NAME → HTTP $HTTP"
  done
fi

# ── Notification rule (idempotent: skip if a rule already exists) ─────────────
echo "Seeding notification rule for org $ORG_ID..."
EXISTING_RULES=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$BACKEND_URL/api/v1/organizations/$ORG_ID/notification-rules" | jq '.rules | length' 2>/dev/null || echo "0")
if [ "$EXISTING_RULES" = "0" ]; then
  RULE_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/notification-rules" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"channel":"slack","target":"https://hooks.slack.test/seed","on":["run_failed"]}')
  echo "  Created slack rule → HTTP $RULE_HTTP"
else
  echo "  Notification rule already exists, skipping."
fi

# ── Failing result to produce a delivery row (idempotent: skip if one already exists) ─
echo "Checking for existing delivery rows..."
EXISTING_DELIVERIES=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$BACKEND_URL/api/v1/organizations/$ORG_ID/notification-deliveries?limit=1" \
  | jq '.deliveries | length' 2>/dev/null || echo "0")
if [ "$EXISTING_DELIVERIES" = "0" ]; then
  FAIL_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/api/v1/organizations/$ORG_ID/results" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "@$FIXTURE_DIR/failing-result.json")
  echo "  failing-result.json → HTTP $FAIL_HTTP"
else
  echo "  Delivery row already exists, skipping failing result post."
fi

# ── M14-021: upgrade acme to team tier + seed github_installations ─────────────
if [ "${SEED_M14:-1}" = "1" ]; then
  echo "Seeding M14-021 convergence state (team tier + github installation)..."
  ORG_GUID_HEX="${ORG_ID#org_}"
  ORG_GUID="${ORG_GUID_HEX:0:8}-${ORG_GUID_HEX:8:4}-${ORG_GUID_HEX:12:4}-${ORG_GUID_HEX:16:4}-${ORG_GUID_HEX:20:12}"
  M14_HTTP=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "$BACKEND_URL/internal/test/seed-m14" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"org_id\":\"$ORG_GUID\",\"installation_id\":12345,\"app_id\":111,\"repos\":[\"acme/api\"],\"stripe_customer_id\":\"cus_test_owner\"}" \
    2>/dev/null || echo "000")
  echo "  seed-m14 → HTTP $M14_HTTP"
fi

echo "Seed complete."
