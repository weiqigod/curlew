#!/usr/bin/env bash
# openapi.sh — the §9.P round trip: import the served document, run what comes out.
#
# This is a shell harness rather than a collection because the thing under test
# is a command, not a response. `curlew import openapi` produces a file, and the
# only assertion worth making about that file is whether it runs against the
# server that described it. A golden-file comparison cannot tell you that: an
# import can be byte-perfect against a stored expectation and still fail on
# contact with the API.
#
# The document is hand-written and version-controlled (testapi/openapi/), not
# generated from mudflat's handlers. That is what makes the round trip evidence
# rather than a tautology — a generated document and the server would agree by
# construction whatever either of them did.
#
# Usage:
#   testapi/harness/openapi.sh [--url http://127.0.0.1:8080]
#
# Exit codes:
#   0 — the round trip passed and the recorded gaps are still present
#   1 — the round trip failed, or a recorded gap has closed (promote it)
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

CURLEW="./curlew"
[ -x "$CURLEW" ] || { echo "build ./curlew first" >&2; exit 2; }

SOURCE_DOC="testapi/openapi/mudflat-openapi.json"
[ -f "$SOURCE_DOC" ] || { echo "missing $SOURCE_DOC" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILURES=0
echo "=== openapi round trip: $MUD_URL ===" >&2

# 1. The served document must be the version-controlled one, byte for byte. A
#    server that rewrote it on the way out would be describing something else,
#    and the round trip would be measuring that instead.
if ! curl -fsS "${MUD_URL}/openapi.json" -o "$WORK/served.json"; then
  echo "  FATAL: could not fetch ${MUD_URL}/openapi.json" >&2
  exit 2
fi
if ! cmp -s "$WORK/served.json" "$SOURCE_DOC"; then
  echo "  FAILED: /openapi.json differs from $SOURCE_DOC" >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  served document matches $SOURCE_DOC byte for byte" >&2
fi

# 2. The import must succeed.
if ! "$CURLEW" import openapi "$WORK/served.json" --output "$WORK/imported.yaml" >"$WORK/import.log" 2>&1; then
  echo "  FAILED: import rejected the served document" >&2
  sed 's/^/      /' "$WORK/import.log" | head -5 >&2
  exit 1
fi
ops="$(grep -c '^  - name:' "$WORK/imported.yaml" || true)"
if [ "$ops" -lt 8 ]; then
  # A harness that reports zero imported operations as success is worse than
  # no harness: the run below would pass vacuously.
  echo "  FAILED: import produced $ops operation(s); the document describes 8" >&2
  exit 1
fi
echo "  imported $ops operations" >&2

# 3. §9.P's acceptance criterion, stated literally: run the import's own output
#    with nothing added. It does not pass today — a path parameter becomes
#    {{code}}, and the import emits no variables: entry and no default for it,
#    so the generated collection cannot run as generated. The failure arrives at
#    run time rather than at import time.
set +e
"$CURLEW" run "$WORK/imported.yaml" >"$WORK/bare.log" 2>&1
bare_code=$?
set -e
if [ "$bare_code" -eq 0 ]; then
  echo >&2
  echo "  FAILED: the bare import now runs unaided — the gap has CLOSED" >&2
  echo "    §9.P's criterion is met literally. Delete this block and assert" >&2
  echo "    exit 0 instead." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  bare import does not run (exit $bare_code): $(grep -o 'undefined variable "[^"]*"' "$WORK/bare.log" | head -1)" >&2
fi

# 4. The round trip proper: supply the server and the path parameter the
#    document declares, and require every operation to pass against the server
#    that served the document.
set +e
"$CURLEW" run "$WORK/imported.yaml" \
  --var "base_url=${MUD_URL}" \
  --var "code=200" \
  --format json >"$WORK/run.json" 2>"$WORK/run.err"
run_code=$?
set -e

verdict="$(python3 -c '
import json, sys
try:
    doc = json.load(open(sys.argv[1]))
except Exception:
    print("no-json\t0\t0")
    sys.exit(0)
reqs = doc.get("requests") or []
# The per-request outcome is a status string, not a boolean. Reading a "passed"
# key here would default every row to False and turn this into a vacuous pass.
passed = sum(1 for r in reqs if r.get("status") == "passed")
print("ok\t%d\t%d" % (len(reqs), passed))
' "$WORK/run.json")"
IFS=$'\t' read -r state total passed <<<"$verdict"

if [ "$state" != "ok" ] || [ "$total" -eq 0 ]; then
  echo "  FAILED: the imported collection produced no results (exit $run_code)" >&2
  sed 's/^/      /' "$WORK/run.err" | head -5 >&2
  exit 1
fi
if [ "$passed" -ne "$total" ]; then
  echo "  FAILED: $passed of $total imported operations passed" >&2
  echo "    An import that produces a syntactically valid collection which fails" >&2
  echo "    on contact with the server it was generated from is exactly what this" >&2
  echo "    harness exists to catch (§9.P)." >&2
  FAILURES=$((FAILURES + 1))
else
  echo "  round trip PASSED: $passed of $total operations, against the server that served the document" >&2
fi

# 5. Three ordinary OpenAPI 3.1 constructs the importer rejects, although both
#    docs/CLI_SPECIFICATION.md §18.8 and docs/MANUAL.md promise "OpenAPI 3.x".
#    Each must still fail; when one starts working, this harness fails and the
#    entry is deleted.
echo >&2
echo "  --- documents that must still be rejected ---" >&2
write_31_doc() {
  local file="$1" info_extra="$2" root_extra="$3" schema="$4"
  cat > "$file" <<JSON
{
  "openapi": "3.1.0",
  "info": { "title": "t", "version": "1"${info_extra} },
  "servers": [{ "url": "http://127.0.0.1:1" }],
  "paths": { "/a": { "get": { "operationId": "a",
    "responses": { "200": { "description": "ok"${schema} } } } } }${root_extra}
}
JSON
}

write_31_doc "$WORK/summary.json" ', "summary": "a one-line summary"' '' ''
write_31_doc "$WORK/webhooks.json" '' ', "webhooks": { "hook": { "post": { "operationId": "h", "responses": { "200": { "description": "ok" } } } } }' ''
write_31_doc "$WORK/typearray.json" '' '' ', "content": { "application/json": { "schema": { "type": ["string", "null"] } } }'

for name in summary webhooks typearray; do
  set +e
  "$CURLEW" import openapi "$WORK/$name.json" --output "$WORK/$name.yaml" >"$WORK/$name.log" 2>&1
  code=$?
  set -e
  if [ "$code" -eq 0 ]; then
    echo "  UNEXPECTED: 3.1 '$name' now imports — the gap has CLOSED" >&2
    echo "    Remove it from this list; the importer has caught up with 3.1." >&2
    FAILURES=$((FAILURES + 1))
  else
    echo "  still rejected: $name — $(sed 's/^.*validating openapi spec: //' "$WORK/$name.log" | head -1)" >&2
  fi
done

echo >&2
if (( FAILURES )); then
  echo "=== openapi FAILED: $FAILURES check(s) ===" >&2
  exit 1
fi
echo "=== openapi PASS: round trip green, recorded gaps still present ===" >&2
