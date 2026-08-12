#!/usr/bin/env bash
# gaps.sh — run the expected-failure collections and assert they still fail.
#
# Specification §12.3. Every request under testapi/gaps/ documents something
# curlew cannot do, or something no client can succeed against. Keeping them as
# executable requests rather than prose is what lets them tell you when they
# stop being true.
#
# An UNEXPECTED PASS is a failure here. That is the whole design: a gap that
# closes must close visibly, or the file becomes a museum of things that were
# once broken.
#
# Usage:
#   testapi/harness/gaps.sh [--url http://127.0.0.1:8080]
#
# Exit codes:
#   0 — every request failed, as expected
#   1 — a request passed that was supposed to fail (promote it), or the run
#       could not be evaluated
#   2 — usage or setup error

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MUD_URL="http://127.0.0.1:8080"
while [ $# -gt 0 ]; do
  case "$1" in
    --url) MUD_URL="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,22p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

# The WebSocket family needs a ws:// URL: a protocol: websocket request is
# dialled with gorilla, which rejects http:// outright.
WS_URL="ws${MUD_URL#http}"

# The TLS listener is the structured port + 2. Nothing curlew sends can trust
# its CA (§11.3), which is the point: the gap entry asserts that the failure is
# legible and classified, not that the request succeeds.
TLS_URL="https://127.0.0.1:$(( ${MUD_URL##*:} + 2 ))"

CURLEW="./curlew"
[ -x "$CURLEW" ] || { echo "build ./curlew first" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# run_bounded runs a command with a wall-clock ceiling. GNU coreutils' timeout(1)
# is not present on macOS, and a gap request that hangs — /hang exists precisely
# to hang — would otherwise wedge the gate rather than failing it.
run_bounded() {
  local limit="$1"; shift
  "$@" &
  local pid=$!
  ( sleep "$limit"; kill -9 "$pid" 2>/dev/null || true ) &
  local killer=$!
  local code=0
  wait "$pid" 2>/dev/null || code=$?
  kill "$killer" 2>/dev/null || true
  return "$code"
}

FAILURES=0
CHECKED=0

shopt -s nullglob

# Collections named *.parse-fail.yaml must not parse at all. They are checked by
# exit code, because a parse error produces no per-request results to inspect.
for collection in testapi/gaps/*.parse-fail.yaml; do
  label="$(basename "$collection")"
  CHECKED=$((CHECKED + 1))
  echo "=== $label (must not parse) ===" >&2

  set +e
  run_bounded 60 "$CURLEW" run "$collection" \
    --var "mud=$MUD_URL" --var "ws=$WS_URL" --var "tls=$TLS_URL" --var "run=gaps$$" >"$WORK/$label.out" 2>&1
  code=$?
  set -e

  if [ "$code" -eq 3 ]; then
    echo "  rejected at parse time, as expected (exit 3)" >&2
  else
    echo "  UNEXPECTED: exit $code, want 3 (a parse error)" >&2
    echo "    If the parser now accepts this, the defect is fixed: move the" >&2
    echo "    request into testapi/collections/ and delete this file." >&2
    sed 's/^/    /' "$WORK/$label.out" | head -5 >&2
    FAILURES=$((FAILURES + 1))
  fi
done

# Collections named *.run-abort.yaml must abort the whole run. They need their
# own category because they destroy the output the per-request check below
# reads: the request array comes back empty while the summary still counts the
# requests that ran, so an unmodified check would report "nothing evaluated" and
# blame itself rather than the defect.
#
# All three conditions are asserted, so that fixing any one of them fails this
# harness and forces the file to be promoted.
for collection in testapi/gaps/*.run-abort.yaml; do
  label="$(basename "$collection")"
  CHECKED=$((CHECKED + 1))
  echo "=== $label (must abort the run) ===" >&2

  set +e
  run_bounded 60 "$CURLEW" run "$collection" \
    --var "mud=$MUD_URL" --var "ws=$WS_URL" --var "tls=$TLS_URL" --var "run=gaps$$" \
    --format json >"$WORK/$label.json" 2>"$WORK/$label.err"
  code=$?
  set -e

  verdict="$(python3 -c '
import json, sys
try:
    doc = json.load(open(sys.argv[1]))
except Exception as exc:
    print("no-json\t0\t0")
    sys.exit(0)
reqs = doc.get("requests") or []
summary = doc.get("summary") or {}
print("%s\t%d\t%d" % ("ok", len(reqs), summary.get("total", 0)))
' "$WORK/$label.json")"
  IFS=$'\t' read -r parsed n_results n_summary <<<"$verdict"

  if [ "$code" -eq 0 ]; then
    echo "  UNEXPECTED: exit 0 — the run did not abort" >&2
    FAILURES=$((FAILURES + 1))
  elif [ "$parsed" = "no-json" ]; then
    echo "  UNEXPECTED: no JSON output at all; this file documents an abort that still writes a report" >&2
    FAILURES=$((FAILURES + 1))
  elif [ "$n_results" -ne 0 ]; then
    echo "  UNEXPECTED: $n_results request result(s) survived the abort" >&2
    echo "    The run no longer discards its results. Move this file into" >&2
    echo "    testapi/collections/ and delete it from here." >&2
    FAILURES=$((FAILURES + 1))
  elif [ "$n_summary" -eq 0 ]; then
    echo "  UNEXPECTED: the summary also reports nothing; the contradiction this documents is gone" >&2
    FAILURES=$((FAILURES + 1))
  else
    echo "  aborted as documented (exit $code, $n_results result(s) reported, summary counts $n_summary)" >&2
  fi
done

for collection in testapi/gaps/*.yaml; do
  case "$collection" in *.parse-fail.yaml|*.run-abort.yaml) continue ;; esac
  label="$(basename "$collection")"
  out="$WORK/$label.json"
  before=$CHECKED

  echo "=== $label ===" >&2
  set +e
  run_bounded 120 "$CURLEW" run "$collection" \
    --var "mud=$MUD_URL" \
    --var "ws=$WS_URL" \
    --var "tls=$TLS_URL" \
    --var "run=gaps$$" \
    --format json >"$out" 2>"$WORK/$label.err"
  set -e

  if [ ! -s "$out" ]; then
    echo "  could not evaluate: no JSON output" >&2
    sed 's/^/    /' "$WORK/$label.err" | head -5 >&2
    FAILURES=$((FAILURES + 1))
    continue
  fi

  # Every request must have failed. A request with no result at all counts as
  # failed too: an error before the response is exactly what several of these
  # document.
  while IFS=$'\t' read -r name status; do
    CHECKED=$((CHECKED + 1))
    if [ "$status" = "passed" ]; then
      echo "  UNEXPECTED PASS: $name" >&2
      echo "    This request documents a defect that is now fixed. Move it into" >&2
      echo "    testapi/collections/ and delete the entry here." >&2
      FAILURES=$((FAILURES + 1))
    else
      echo "  failed as expected: $name ($status)" >&2
    fi
  done < <(python3 -c '
import json, sys

# The per-request outcome is a STATUS STRING — "passed" / "failed" / "error" /
# "skipped" — not a boolean. An earlier version of this read r["passed"], a
# field that does not exist, so every request looked failed and the one thing
# this harness exists to catch, an unexpected PASS, could never fire. Any row
# whose status is missing is reported as such rather than defaulting to a
# verdict.
doc = json.load(open(sys.argv[1]))
for r in doc.get("requests", []):
    print("%s\t%s" % (r.get("name", "?"), r.get("status") or "MISSING-STATUS"))
' "$out")

  if [ "$CHECKED" -eq "$before" ]; then
    # No request was evaluated. That is not a pass: it means the collection
    # produced no results, and a harness that reports zero checks as success is
    # worse than no harness.
    echo "  NOTHING EVALUATED: the collection produced no request results" >&2
    sed 's/^/    /' "$WORK/$label.err" | head -5 >&2
    FAILURES=$((FAILURES + 1))
  fi
done

echo >&2
if (( FAILURES )); then
  echo "=== gaps FAILED: $FAILURES of $CHECKED request(s) did not fail as expected ===" >&2
  exit 1
fi
echo "=== gaps PASS: all $CHECKED request(s) failed, as documented ===" >&2
