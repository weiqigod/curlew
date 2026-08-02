#!/usr/bin/env bash
# check-signing-keys.sh — M18-009 (v4-13): SaaS signing-key CI lint.
#
# Fails (exit 1) when any row in the signing_keys table has a NULL kms_key_id.
# This guards SaaS deployments against signing keys that were provisioned without
# a KMS key reference, ensuring all signing keys are enrolled in the KMS envelope.
#
# Skipped (exit 0 + log note) when APITEST_BUILD_PROFILE != "saas" OR when
# APITEST_SIGNING_KEY_MODE == "file" (self-hosted / developer laptops).
#
# Usage:
#   ./scripts/check-signing-keys.sh
#   APITEST_BUILD_PROFILE=saas DATABASE_URL=postgres://... ./scripts/check-signing-keys.sh
#
# Environment variables:
#   APITEST_BUILD_PROFILE   — must be "saas" to enable the check (default: unset → skip)
#   APITEST_SIGNING_KEY_MODE — "file" overrides profile and skips the check
#   DATABASE_URL             — Postgres connection string (default: postgres://localhost/apitool)
#
# Exit codes:
#   0 — check passed (no NULL kms_key_id rows) OR check was skipped
#   1 — check failed (at least one NULL kms_key_id row found) OR psql connection error

set -euo pipefail

PROFILE="${APITEST_BUILD_PROFILE:-}"
KEY_MODE="${APITEST_SIGNING_KEY_MODE:-}"
DATABASE_URL="${DATABASE_URL:-postgres://localhost/apitool}"

# Skip in self-hosted / file-key-mode scenarios.
if [ "$KEY_MODE" = "file" ]; then
  echo "check-signing-keys: SKIPPED (APITEST_SIGNING_KEY_MODE=file — self-hosted mode)"
  exit 0
fi

# Skip if not a SaaS build.
if [ "$PROFILE" != "saas" ]; then
  echo "check-signing-keys: SKIPPED (APITEST_BUILD_PROFILE=${PROFILE:-<unset>} — only enforced in saas profile)"
  exit 0
fi

echo "check-signing-keys: running SaaS lint against ${DATABASE_URL}"

# Count rows with NULL kms_key_id.
COUNT="$(psql "$DATABASE_URL" \
  --tuples-only \
  --no-align \
  -c "SELECT COUNT(*) FROM signing_keys WHERE kms_key_id IS NULL;")"

COUNT="${COUNT//[[:space:]]/}"  # strip whitespace

if [ "$COUNT" = "0" ]; then
  echo "check-signing-keys: PASS — all signing keys have a kms_key_id"
  exit 0
else
  echo "check-signing-keys: FAIL — ${COUNT} signing_key row(s) have NULL kms_key_id" >&2
  echo "  All signing keys in a SaaS deployment must be enrolled in KMS (v4-13)." >&2
  echo "  Run: psql \"\$DATABASE_URL\" -c \"SELECT kid FROM signing_keys WHERE kms_key_id IS NULL;\"" >&2
  exit 1
fi
