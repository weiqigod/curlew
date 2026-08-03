#!/usr/bin/env bash
# test-token.sh — Mint dev tokens and seed test data for manual testing.
#
# Usage:
#   ./scripts/test-token.sh <email> [<user-id>]
#     → Mints an HS256 JWT bearer token (existing mode, unchanged).
#
#   ./scripts/test-token.sh refresh <email>
#     → Seeds a refresh token via POST /internal/test/seed-refresh.
#       Prints the opaque plaintext token.
#
#   ./scripts/test-token.sh device <email>
#     → Prints a deterministic device UUID derived from the email.
#       This UUID is used as device_id when calling the refresh endpoint.
#
# Requires: openssl, uuidgen (or python3 for device mode on Linux).
# The 'refresh' mode requires the backend to be running on BACKEND_URL (default http://localhost:5000).
#
# Examples:
#   TOKEN=$(./scripts/test-token.sh owner@example.com)
#   curl -H "Authorization: Bearer $TOKEN" http://localhost:5000/api/v1/organizations
#
#   DEVICE=$(./scripts/test-token.sh device owner@example.com)
#   REFRESH=$(./scripts/test-token.sh refresh owner@example.com)
#   curl -X POST http://localhost:5000/api/v1/auth/refresh \
#     -H "Content-Type: application/json" \
#     -d "{\"refresh_token\":\"$REFRESH\",\"device_id\":\"$DEVICE\"}" | jq 'keys'

set -euo pipefail

MODE="${1:-}"
BACKEND_URL="${BACKEND_URL:-http://localhost:5000}"

# ── device mode ───────────────────────────────────────────────────────────────
if [[ "$MODE" == "device" ]]; then
  EMAIL="${2:-dev@example.com}"
  # Derive a deterministic UUID v4-ish from the email using MD5 (Decision #11 in plan).
  # MD5("device:<email>") → format as UUID.
  if command -v python3 &>/dev/null; then
    python3 -c "
import hashlib, uuid, sys
h = hashlib.md5(('device:' + '$EMAIL').encode()).digest()
# Format as UUID by munging bytes 6 and 8 for v4/variant-1 compatibility.
b = bytearray(h)
b[6] = (b[6] & 0x0f) | 0x40  # version 4
b[8] = (b[8] & 0x3f) | 0x80  # variant 1
print(str(uuid.UUID(bytes=bytes(b))))
"
  else
    printf 'Error: python3 required for device mode on this platform\n' >&2
    exit 1
  fi
  exit 0
fi

# ── refresh mode ─────────────────────────────────────────────────────────────
if [[ "$MODE" == "refresh" ]]; then
  EMAIL="${2:-dev@example.com}"
  # Derive the deterministic device UUID (same as 'device' mode).
  if command -v python3 &>/dev/null; then
    DEVICE_ID=$(python3 -c "
import hashlib, uuid
h = hashlib.md5(('device:' + '$EMAIL').encode()).digest()
b = bytearray(h)
b[6] = (b[6] & 0x0f) | 0x40
b[8] = (b[8] & 0x3f) | 0x80
print(str(uuid.UUID(bytes=bytes(b))))
")
  else
    printf 'Error: python3 required for refresh mode on this platform\n' >&2
    exit 1
  fi

  RESPONSE=$(curl -sS -X POST "${BACKEND_URL}/internal/test/seed-refresh" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"${EMAIL}\",\"device_id\":\"${DEVICE_ID}\"}")

  # Extract plaintext using python3 (no jq dependency required)
  printf '%s' "$RESPONSE" | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(data['plaintext'])
"
  exit 0
fi

# ── jwt mode (original, unchanged) ───────────────────────────────────────────
EMAIL="${1:-dev@example.com}"
USER_ID="${2:-$(uuidgen | tr '[:upper:]' '[:lower:]')}"

# Must match Jwt:* in src/ApiTool.Backend/appsettings.Development.json.
# The issuer/audience are "apitool-dev", not "curlew-dev": the rebrand renamed
# the CLI, not the backend's configuration. A mismatch here mints tokens the
# backend rejects with 401, which surfaces as every seed script failing.
SIGNING_KEY="${JWT_SIGNING_KEY:-development-signing-key-change-me-32-bytes-minimum}"
ISSUER="${JWT_ISSUER:-apitool-dev}"
AUDIENCE="${JWT_AUDIENCE:-apitool-dev}"

# ----- helpers ---------------------------------------------------------------
b64url() {
  # base64url-encode stdin: replace +/= with -_
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

json_b64url() {
  printf '%s' "$1" | b64url
}

hmac_sha256_b64url() {
  local data="$1"
  local key="$2"
  printf '%s' "$data" | openssl dgst -sha256 -hmac "$key" -binary | b64url
}

# ----- build token -----------------------------------------------------------
NOW=$(date +%s)
EXP=$(( NOW + 3600 ))

HEADER='{"alg":"HS256","typ":"JWT"}'
PAYLOAD=$(printf '{"sub":"%s","email":"%s","iss":"%s","aud":"%s","iat":%d,"exp":%d}' \
  "$USER_ID" "$EMAIL" "$ISSUER" "$AUDIENCE" "$NOW" "$EXP")

HEADER_B64=$(json_b64url "$HEADER")
PAYLOAD_B64=$(json_b64url "$PAYLOAD")

SIGNATURE=$(hmac_sha256_b64url "${HEADER_B64}.${PAYLOAD_B64}" "$SIGNING_KEY")

printf '%s.%s.%s\n' "$HEADER_B64" "$PAYLOAD_B64" "$SIGNATURE"
