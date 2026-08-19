#!/usr/bin/env bash
# ledger.sh — check curlew's account of a run against mudflat's.
#
# Specification §18 Phase 4. Every other harness in this directory asks
# whether a request succeeded or failed. This one asks whether the NUMBERS
# are right, because a wrong number is what a passing run looks like from the
# outside: exit 0, "status": "passed", and a summary that does not add up.
#
# Two oracles, both applied to one run of testapi/collections/25-ledger.yaml:
#
#   A. Internal consistency — summary.total == passed+failed+skipped ==
#      len(requests[]). docs/MANUAL.md's worked JSON example documents this
#      contract; §11D.1 is where it was found broken by a data_driven request
#      that expands one declared item into one result per row.
#
#   B. External agreement — after the run, this script makes its own
#      independent, read-only GET to mudflat's /resources endpoint for the
#      same session and checks the total against 3, the number of rows in
#      testapi/collections/data/bulk.csv. This does not merely re-read
#      curlew's own JSON: it is a second HTTP client asking the server
#      directly, so a curlew defect that made its own assertion evaluation
#      lie (report PASS regardless of the actual value) would still be
#      caught. It also cross-checks the retried request's own
#      attempt_details count against mudflat's attempt field for the same
#      response — two independent code paths inside curlew (the retry loop's
#      bookkeeping, and a JSONPath extraction of the raw response) that must
#      agree.
#
# Usage:
#   testapi/harness/ledger.sh [--url http://127.0.0.1:8080]
#
# Exit codes:
#   0 — every count agreed
#   1 — a count disagreed, or nothing could be evaluated
#   2 — usage or setup error

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MUD_URL="http://127.0.0.1:8080"
while [ $# -gt 0 ]; do
  case "$1" in
    --url) MUD_URL="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,29p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

CURLEW="./curlew"
[ -x "$CURLEW" ] || { echo "build ./curlew first" >&2; exit 2; }

COLLECTION="testapi/collections/25-ledger.yaml"
[ -f "$COLLECTION" ] || { echo "missing $COLLECTION" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0
CHECKED=0

# A session id unique to this invocation, so a rerun does not see stale
# resources left by a previous one (§6.2 isolation).
RUN_ID="ledger$$"

echo "=== ledger: $MUD_URL (session ${RUN_ID}-ledger) ===" >&2

set +e
"$CURLEW" run "$COLLECTION" \
  --var "mud=${MUD_URL}" --var "run=${RUN_ID}" \
  --format json >"$WORK/run.json" 2>"$WORK/run.err"
run_code=$?
set -e

verdict="$(python3 -c '
import json, sys

try:
    doc = json.load(open(sys.argv[1]))
except Exception as e:
    print("no-json\t0\t0\t0\t0\t0\t0")
    sys.exit(0)

summary = doc.get("summary") or {}
total = summary.get("total", 0)
passed = summary.get("passed", 0)
failed = summary.get("failed", 0)
skipped = summary.get("skipped", 0)
reqs = doc.get("requests") or []

# The retried request carries attempt_details (curlew''s own retry
# bookkeeping) and an assertion against $.attempt (a JSONPath extraction of
# mudflat''s raw response). Oracle B compares the two.
attempt_details_len = 0
attempt_actual = -1
for r in reqs:
    if "attempt_details" in r and r["attempt_details"]:
        attempt_details_len = len(r["attempt_details"])
        for a in r.get("assertions", []):
            if a.get("target") == "$.attempt":
                try:
                    attempt_actual = int(a.get("actual", "-1"))
                except ValueError:
                    attempt_actual = -1

print("ok\t%d\t%d\t%d\t%d\t%d\t%d\t%d" % (
    total, passed, failed, skipped, len(reqs), attempt_details_len, attempt_actual))
' "$WORK/run.json")"

IFS=$'\t' read -r state total passed failed skipped nreqs attempt_details_len attempt_actual <<<"$verdict"

if [ "$state" != "ok" ] || [ "$run_code" -ne 0 ]; then
  echo "  FATAL: the ledger run produced no usable JSON (exit $run_code)" >&2
  sed 's/^/    /' "$WORK/run.err" | head -10 >&2
  cat "$WORK/run.json" | head -10 >&2
  exit 1
fi

# --- Oracle A: internal consistency ---
CHECKED=$((CHECKED + 1))
sum=$((passed + failed + skipped))
if [ "$total" -ne "$sum" ]; then
  echo "  LEDGER MISMATCH: summary.total=$total but passed($passed)+failed($failed)+skipped($skipped)=$sum" >&2
  echo "    curlew's own report does not add up. If this is now correct, the" >&2
  echo "    defect is fixed: keep this check (it is the guard that proves it" >&2
  echo "    stayed fixed) and record the change in specification §11D." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  internal consistency: total=$total, passed+failed+skipped=$sum" >&2
fi

CHECKED=$((CHECKED + 1))
if [ "$total" -ne "$nreqs" ]; then
  echo "  LEDGER MISMATCH: summary.total=$total but requests[] holds $nreqs entries" >&2
  echo "    curlew's report does not add up. If this is now correct, the defect" >&2
  echo "    is fixed: keep this check and record the change in specification §11D." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  internal consistency: total=$total, len(requests[])=$nreqs" >&2
fi

# --- Oracle B: external agreement ---
# A read-only GET, made by this script rather than by curlew, against the
# same session curlew just used. bulk.csv has exactly 3 data rows.
CHECKED=$((CHECKED + 1))
resources_total="$(curl -fsS "${MUD_URL}/s/${RUN_ID}-ledger/resources" 2>/dev/null | python3 -c 'import json,sys; print(json.load(sys.stdin).get("total", -1))' 2>/dev/null || echo -1)"
if [ "$resources_total" != "3" ]; then
  echo "  LEDGER MISMATCH: mudflat reports $resources_total resource(s) in session ${RUN_ID}-ledger, want 3" >&2
  echo "    This is queried directly from mudflat, independent of anything" >&2
  echo "    curlew claims -- a data-driven request created the wrong number of" >&2
  echo "    resources, or curlew's own report of what it did was wrong." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  external agreement: mudflat independently reports 3 resources" >&2
fi

CHECKED=$((CHECKED + 1))
if [ "$attempt_details_len" -eq 0 ] || [ "$attempt_actual" -eq -1 ]; then
  echo "  NOTHING EVALUATED: no retried request with a \$.attempt assertion was found" >&2
  FAILURES=$((FAILURES + 1))
elif [ "$attempt_details_len" -ne "$attempt_actual" ]; then
  echo "  LEDGER MISMATCH: curlew's attempt_details has $attempt_details_len entries but mudflat's \$.attempt=$attempt_actual" >&2
  echo "    Two independent code paths inside curlew -- the retry loop's own" >&2
  echo "    bookkeeping and a JSONPath read of the server's response -- disagree" >&2
  echo "    about how many attempts this request took." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  external agreement: attempt_details ($attempt_details_len) matches mudflat's \$.attempt ($attempt_actual)" >&2
fi

if [ "$CHECKED" -eq 0 ]; then
  # A harness that reports zero checks as success is worse than no harness.
  echo "  NOTHING EVALUATED: no check ran" >&2
  FAILURES=$((FAILURES + 1))
fi

echo >&2
if (( FAILURES )); then
  echo "=== ledger FAILED: $FAILURES of $CHECKED check(s) disagreed ===" >&2
  exit 1
fi
echo "=== ledger PASS: all $CHECKED check(s) agreed ===" >&2
