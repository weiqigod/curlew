#!/usr/bin/env bash
# crosscheck.sh — read the raw layer with curl as well as with curlew.
#
# Specification §13.3. The raw layer's responses are hand-written bytes, and the
# hand that wrote them could be wrong. curl is an independent implementation
# with its own reading of RFC 9112, so its verdict on each malformation is
# evidence about the BYTES rather than about Go.
#
# Two questions this answers that a Go-only test cannot:
#
#   1. Is the malformation real? If curl also rejects it, the response is
#      genuinely malformed rather than Go being unusually strict.
#   2. Is the control case actually well-formed? /raw/chunked/trailers must be
#      accepted by both, or the family only ever proves strictness.
#
# Disagreements are reported but not failed: two implementations may legitimately
# differ on what a recipient MAY accept (bare LF, RFC 9112 §2.2, is the standing
# example). Only the must-agree cases below are enforced.
#
# Usage:
#   testapi/harness/crosscheck.sh [--raw-url http://127.0.0.1:8081]
#
# Exit codes:
#   0 — every must-agree case agrees
#   1 — curl and the specification disagree on a case that is not negotiable
#   2 — usage or setup error (curl missing)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

RAW_URL="http://127.0.0.1:8081"
while [ $# -gt 0 ]; do
  case "$1" in
    --raw-url) RAW_URL="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,28p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

command -v curl >/dev/null 2>&1 || {
  echo "curl not found — it is the independent implementation this harness needs" >&2
  exit 2
}

# must_reject: curl has to refuse these, or the bytes are not malformed after all.
MUST_REJECT=(
  "/raw/content-length/duplicate"
  "/raw/chunked/bad-size"
  "/raw/no-status-line"
)

# must_accept: the control cases. A client that cannot read these is broken.
MUST_ACCEPT=(
  "/raw/chunked/trailers"
)

# observe_only: genuinely negotiable, reported for the record.
OBSERVE=(
  "/raw/content-length/over"
  "/raw/content-length/under"
  "/raw/content-length/with-chunked"
  "/raw/chunked/no-terminator"
  "/raw/bare-lf"
  "/raw/header/huge"
  "/raw/header/many"
  "/raw/header/nul"
  "/raw/header/no-colon"
  "/raw/trailing-garbage"
  "/raw/http09"
)

FAILURES=0

# ask returns curl's exit code for a path. 0 means curl was satisfied.
ask() {
  local path="$1"
  set +e
  curl -sS --max-time 10 -o /dev/null "${RAW_URL}${path}" >/dev/null 2>&1
  local code=$?
  set -e
  echo "$code"
}

printf '=== curl cross-check against %s ===\n' "$RAW_URL" >&2
printf '%-40s %-10s %s\n' "ENDPOINT" "CURL" "VERDICT" >&2

for path in "${MUST_REJECT[@]}"; do
  code="$(ask "$path")"
  if [ "$code" -eq 0 ]; then
    printf '%-40s %-10s %s\n' "$path" "exit $code" "FAIL — curl accepted a response that must be rejected" >&2
    FAILURES=$((FAILURES + 1))
  else
    printf '%-40s %-10s %s\n' "$path" "exit $code" "ok — rejected by curl too" >&2
  fi
done

for path in "${MUST_ACCEPT[@]}"; do
  code="$(ask "$path")"
  if [ "$code" -ne 0 ]; then
    printf '%-40s %-10s %s\n' "$path" "exit $code" "FAIL — the control case must be readable" >&2
    FAILURES=$((FAILURES + 1))
  else
    printf '%-40s %-10s %s\n' "$path" "exit $code" "ok — well-formed, accepted" >&2
  fi
done

for path in "${OBSERVE[@]}"; do
  code="$(ask "$path")"
  verdict="curl accepted"
  [ "$code" -ne 0 ] && verdict="curl rejected"
  printf '%-40s %-10s %s\n' "$path" "exit $code" "$verdict (observed, not enforced)" >&2
done

echo >&2
if (( FAILURES )); then
  echo "=== crosscheck FAILED: $FAILURES case(s) where curl contradicts the intent ===" >&2
  exit 1
fi
echo "=== crosscheck PASS: curl agrees on every enforced case ===" >&2
