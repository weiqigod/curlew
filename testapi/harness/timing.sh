#!/usr/bin/env bash
# timing.sh — assert that a request's reported duration covers the body read.
#
# This is a shell harness rather than a collection because no assertion can
# express it. The defect makes a WRONG thing PASS: `timing.max_duration_ms: 50`
# succeeds against a request that took a second, so there is no failing request
# to put in testapi/gaps/. The claim is about curlew's own output.
#
# internal/httpexec/executor.go measures
#
#   start := time.Now()
#   resp, err := http.DefaultClient.Do(httpReq)
#   duration := time.Since(start)      <- stops when the HEADERS arrive
#   ...
#   body, err := io.ReadAll(resp.Body) <- not counted
#
# For an ordinary small response the download is negligible and nobody notices.
# Against a stream it is the entire cost: mudflat's /sse?events=5&ms=200 takes
# a second, and curlew reports 0ms. `timing.max_duration_ms` asserts against
# that number, so it cannot fail on a slow body — a documented assertion that
# silently always passes.
#
# curlew already measures the right number in the same request: the --events
# stream carries timing.total_us (~1010000) and timing.download_us (~1010000)
# beside duration_ms: 0. Three orders of magnitude apart, one event.
#
# This harness asserts the DEFECT, and fails when it is fixed — the same
# discipline as testapi/harness/redaction-known-leaks.txt. A fix must delete
# this file and move the assertion into a collection, where
# `timing.max_duration_ms` can finally do its job.
#
# Usage:
#   testapi/harness/timing.sh [--url http://127.0.0.1:8080]
#
# Exit codes:
#   0 — the gap is still present, as documented
#   1 — the gap has closed (promote it), or the measurement could not be made
#   2 — usage or setup error

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MUD_URL="http://127.0.0.1:8080"
while [ $# -gt 0 ]; do
  case "$1" in
    --url) MUD_URL="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,37p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

CURLEW="./curlew"
[ -x "$CURLEW" ] || { echo "build ./curlew first" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Five events, 200ms apart: one second of body, ~0ms of headers.
STREAM_MS=200
STREAM_EVENTS=5
EXPECTED_MS=$((STREAM_MS * STREAM_EVENTS))

cat > "$WORK/slow-stream.yaml" <<YAML
name: a response whose cost is entirely in the body
requests:
  - name: five events, ${STREAM_MS}ms apart
    request:
      method: GET
      url: "${MUD_URL}/sse?events=${STREAM_EVENTS}&ms=${STREAM_MS}"
    assertions:
      status: 200
      timing:
        # Twenty times faster than the request can possibly be. This assertion
        # passing IS the defect.
        max_duration_ms: 50
YAML

echo "=== timing harness: $MUD_URL ===" >&2

set +e
"$CURLEW" run "$WORK/slow-stream.yaml" \
  --format json --events "$WORK/events.ndjson" >"$WORK/out.json" 2>"$WORK/err.txt"
code=$?
set -e

if [ "$code" -ge 2 ]; then
  echo "  FATAL: exited $code — the collection did not run" >&2
  sed 's/^/      /' "$WORK/err.txt" | head -5 >&2
  exit 2
fi

measured="$(python3 - "$WORK/out.json" "$WORK/events.ndjson" <<'PY'
import json, sys

doc = json.load(open(sys.argv[1]))
reqs = doc.get("requests") or []
if not reqs:
    print("no-results\t0\t0\t0")
    sys.exit(0)

reported = reqs[0].get("duration_ms")

# The per-request outcome is a status string, and the assertion outcomes are
# booleans on their own objects. Reading a "passed" key off the request itself
# returns nothing and would silently report the opposite verdict.
timing_passed = None
for a in reqs[0].get("assertions") or []:
    if a.get("type") == "timing":
        timing_passed = a.get("passed")
passed = "unknown" if timing_passed is None else str(timing_passed).lower()

total_us = 0
for line in open(sys.argv[2]):
    try:
        ev = json.loads(line)
    except Exception:
        continue
    if ev.get("kind") == "request.end":
        total_us = (ev.get("timing") or {}).get("total_us") or 0

print("ok\t%s\t%d\t%s" % (reported, total_us, passed))
PY
)"
IFS=$'\t' read -r state reported total_us passed <<<"$measured"

if [ "$state" != "ok" ]; then
  # A harness that reports zero measurements as success is worse than no
  # harness at all.
  echo "  FATAL: no request results to measure" >&2
  exit 1
fi

total_ms=$(( total_us / 1000 ))
echo "  reported duration_ms : ${reported}" >&2
echo "  measured total_us    : ${total_us}  (${total_ms} ms)" >&2
echo "  timing assertion (max 50ms) passed: ${passed}" >&2

# The measurement itself must be sound: if the request did not actually take
# the expected time, this harness is testing nothing.
if [ "$total_ms" -lt $((EXPECTED_MS / 2)) ]; then
  echo "=== timing FAILED: the request took ${total_ms}ms, expected ~${EXPECTED_MS}ms ===" >&2
  echo "The stream did not behave as expected, so nothing here was measured." >&2
  exit 1
fi

if [ "$passed" = "unknown" ]; then
  echo "  FATAL: no timing assertion in the output; nothing was measured" >&2
  exit 1
fi

if [ "$passed" = "false" ]; then
  echo >&2
  echo "=== timing FAILED: the timing assertion now fails, as it should ===" >&2
  echo "max_duration_ms: 50 was refused against a ${total_ms}ms request." >&2
  echo "The gap has CLOSED — see the promotion instructions below." >&2
  exit 1
fi

if [ "$reported" -ge $((EXPECTED_MS / 2)) ]; then
  echo >&2
  echo "=== timing FAILED: duration_ms now covers the body read ===" >&2
  echo "The gap has CLOSED: reported ${reported}ms against a ${total_ms}ms request." >&2
  echo "Delete testapi/harness/timing.sh, drop its step from scripts/ci-local.sh," >&2
  echo "and move the assertion into testapi/collections/92-streaming.yaml, where" >&2
  echo "timing.max_duration_ms can finally fail on a slow body." >&2
  exit 1
fi

echo >&2
echo "=== timing PASS: the gap is still present ===" >&2
echo "duration_ms=${reported} against a ${total_ms}ms request, and max_duration_ms: 50 passed." >&2
echo "See the header of this file, and TESTAPI_SPECIFICATION §11C." >&2
