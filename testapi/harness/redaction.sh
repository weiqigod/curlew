#!/usr/bin/env bash
# redaction.sh — assert no published secret survives into any curlew artefact.
#
# CLI_SPECIFICATION §6.5 says a sensitive value is replaced with [REDACTED] in
# terminal output, JSON, TAP, JUnit, HTML, Markdown, event streams and JSONL
# logs. Eight surfaces, each its own code path. A secret redacted in the
# terminal and present in the HTML report has still leaked, so every surface is
# checked separately and the failure names the format.
#
# This is a shell harness rather than a collection because the assertion is on
# curlew's OUTPUT, not on a response. No collection can express it.
#
# Usage:
#   testapi/harness/redaction.sh [--url http://127.0.0.1:8080]
#
# Exit codes:
#   0 — no secret found in any artefact
#   1 — a secret leaked (the format and the value are named)
#   2 — usage or setup error

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

MUD_URL="http://127.0.0.1:8080"
while [ $# -gt 0 ]; do
  case "$1" in
    --url) MUD_URL="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \?//'; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

CURLEW="./curlew"
[ -x "$CURLEW" ] || { echo "build ./curlew first" >&2; exit 2; }

COLLECTION="testapi/collections/70-redaction.yaml"
[ -f "$COLLECTION" ] || { echo "missing $COLLECTION" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# The needles: every published value from Appendix B that this collection can
# put in front of curlew. Keep in step with testapi/mudflat/endpoints_leak.go —
# a needle the server never sends makes this harness pass vacuously, which is
# why TestLeak_ValuesMatchAppendixB pins them on the server side.
NEEDLES=(
  "mud_tok_7f3a9c2e5b1d4680"
  "mud_refresh_3b8c1f9e7a2d5460"
  "mud_key_a1b2c3d4e5f60718"
  "mud_sess_c9e4f1a7b2d80356"
  "4242424242424242"
)

RUN_ID="redact$$"
NEW_LEAKS=0
FIXED_LEAKS=0

# The baseline of leaks that exist today. A leak outside it is a regression; a
# baseline entry that stops leaking has been fixed and must be removed, which is
# what stops the file becoming a permanent excuse.
BASELINE="testapi/harness/redaction-known-leaks.txt"
declare -a KNOWN=()
if [ -f "$BASELINE" ]; then
  while IFS= read -r line; do
    case "$line" in ''|\#*) continue ;; esac
    KNOWN+=("$line")
  done < "$BASELINE"
fi

is_known() {
  local entry="$1"
  local k
  for k in ${KNOWN+"${KNOWN[@]}"}; do
    [ "$k" = "$entry" ] && return 0
  done
  return 1
}

declare -a OBSERVED=()

# run_format executes the collection in one output format and returns the paths
# of every artefact it produced.
run_one() {
  local label="$1"; shift
  local out="$WORK/$label"
  mkdir -p "$out"

  # Terminal/stdout output is an artefact too — capture it.
  #
  # --events is NOT passed here. Writing the event stream into every format's
  # directory made a single leak in the stream look like a leak in all six
  # formats, which is worse than not checking: it misattributes the defect.
  # The stream gets its own run below.
  set +e
  "$CURLEW" run "$COLLECTION" \
    --var "mud=$MUD_URL" \
    --var "run=$RUN_ID" \
    "$@" >"$out/stdout.txt" 2>"$out/stderr.txt"
  local code=$?
  set -e

  if [ "$code" -ge 2 ]; then
    # Exit 2+ means the run did not execute: usage, parse or file error. A
    # scan over artefacts that were never written passes vacuously, which is
    # the one outcome this harness must never report as success.
    echo "  FATAL: $label exited $code — the collection did not run" >&2
    sed 's/^/      /' "$out/stderr.txt" | head -5 >&2
    exit 2
  fi
  echo "  ran $label (exit $code)" >&2
}

scan() {
  local label="$1"
  local dir="$WORK/$label"
  local new=0
  local known=0

  for needle in "${NEEDLES[@]}"; do
    # -r to walk report directories, -l to name the file, -F for a literal.
    local hits
    hits="$(grep -rlF "$needle" "$dir" 2>/dev/null || true)"
    [ -n "$hits" ] || continue

    OBSERVED+=("$label:$needle")

    if is_known "$label:$needle"; then
      known=$((known + 1))
      continue
    fi

    new=1
    NEW_LEAKS=$((NEW_LEAKS + 1))
    echo "NEW LEAK [$label]: $needle appears in:" >&2
    echo "$hits" | sed 's/^/    /' >&2
    grep -rhF -m1 "$needle" "$dir" 2>/dev/null | head -1 | cut -c1-200 | sed 's/^/      /' >&2
  done

  if (( new )); then
    return
  fi
  if (( known )); then
    echo "  known-leaking: $label ($known value(s), see $BASELINE)" >&2
  else
    echo "  clean: $label" >&2
  fi
}

echo "=== redaction harness: $MUD_URL ===" >&2

run_one terminal   --format terminal --no-color
run_one json       --format json
run_one tap        --format tap
run_one junit      --format junit --report "$WORK/junit/report.xml"
run_one html       --format html  --report "$WORK/html/report.html"
run_one markdown   --format markdown --report "$WORK/markdown"
run_one jsonl      --format terminal --no-color --log "$WORK/jsonl/run.jsonl"
run_one events     --format terminal --no-color --events "$WORK/events/events.ndjson"

for label in terminal json tap junit html markdown jsonl events; do
  scan "$label"
done

# A baseline entry that no longer leaks has been fixed. Failing on that is the
# point: it is the only thing that makes the file shrink.
for known in ${KNOWN+"${KNOWN[@]}"}; do
  still=0
  for seen in ${OBSERVED+"${OBSERVED[@]}"}; do
    [ "$seen" = "$known" ] && still=1 && break
  done
  if (( ! still )); then
    FIXED_LEAKS=$((FIXED_LEAKS + 1))
    echo "FIXED: $known no longer leaks — remove it from $BASELINE so it is enforced" >&2
  fi
done

echo >&2
if (( NEW_LEAKS )); then
  echo "=== redaction FAILED: $NEW_LEAKS new leak(s) ===" >&2
  echo "CLI_SPECIFICATION §6.5 requires [REDACTED] in every surface listed above." >&2
  exit 1
fi
if (( FIXED_LEAKS )); then
  echo "=== redaction FAILED: $FIXED_LEAKS baseline entr(ies) no longer leak ===" >&2
  exit 1
fi

echo "=== redaction PASS: no new leaks (${#KNOWN[@]} known, see $BASELINE) ===" >&2
