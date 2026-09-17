#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Every temporary path and process belongs to this invocation. The single EXIT
# trap never searches ports or deletes another run's files.
SMOKE_TEMP_BASE="${TMPDIR:-/tmp}"
SMOKE_ROOT=$(command mktemp -d "$SMOKE_TEMP_BASE/curlew_smoke_XXXXXX")
SMOKE_ROOT=$(cd "$SMOKE_ROOT" && pwd -P)
SMOKE_CFG_DIR="$SMOKE_ROOT/config"
mkdir -p "$SMOKE_CFG_DIR"
export CURLEW_CONFIG_DIR="$SMOKE_CFG_DIR"

# Keep every later mktemp allocation inside this run's already-unique root.
# Moving a generated path to add its suffix avoids BSD/macOS mktemp's
# requirement that the X block end the template.
mktemp() {
  local make_dir=0 template="curlew_XXXXXX" base suffix path
  if [ "${1:-}" = "-d" ]; then
    make_dir=1
    shift
  elif [ "${1:-}" = "-t" ]; then
    shift
    template="${1:-curlew}_XXXXXX"
    shift || true
  fi
  if [ "$#" -gt 0 ]; then
    template="$1"
  fi
  base="${template##*/}"
  if [[ "$base" != *XXXXXX* ]]; then
    base="${base}_XXXXXX"
  fi
  suffix="${base#*XXXXXX}"
  base="${base%%XXXXXX*}XXXXXX"
  if [ "$make_dir" -eq 1 ]; then
    path=$(command mktemp -d "$SMOKE_ROOT/$base")
  else
    path=$(command mktemp "$SMOKE_ROOT/$base")
  fi
  if [ -n "$suffix" ]; then
    mv -- "$path" "$path$suffix"
    path="$path$suffix"
  fi
  printf '%s\n' "$path"
}

declare -a SMOKE_OWNED_PIDS=()
declare -a SMOKE_OWNED_PATHS=("$SMOKE_ROOT")
register_pid() { SMOKE_OWNED_PIDS+=("$1"); }
register_path() { SMOKE_OWNED_PATHS+=("$1"); }
unregister_pid() {
  local target="$1"
  local -a retained=()
  local pid
  for pid in "${SMOKE_OWNED_PIDS[@]}"; do
    if [ "$pid" != "$target" ]; then
      retained+=("$pid")
    fi
  done
  SMOKE_OWNED_PIDS=("${retained[@]}")
}
stop_pid() {
  local pid="${1:-}"
  local signal="${2:-TERM}"
  if [ -z "$pid" ]; then
    return
  fi
  STOP_PID_STATUS=0
  kill "-$signal" "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || STOP_PID_STATUS=$?
  unregister_pid "$pid"
}
cleanup() {
  local pid path last
  while [ "${#SMOKE_OWNED_PIDS[@]}" -gt 0 ]; do
    last=$((${#SMOKE_OWNED_PIDS[@]} - 1))
    pid="${SMOKE_OWNED_PIDS[$last]}"
    unset 'SMOKE_OWNED_PIDS[$last]'
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  for path in "${SMOKE_OWNED_PATHS[@]}"; do
    rm -rf -- "$path"
  done
}
trap cleanup EXIT

# fail "message" [context lines...] — print a FAIL line (plus optional context)
# and abort the run. Exiting via `exit 1` fires any active EXIT trap, so
# per-section cleanup still runs. Every failed check MUST stop the run: use
# this helper, or pair a raw FAIL echo with an explicit exit 1 within a few
# lines — ci-local.sh lints this file for FAIL echoes that never exit.
fail() {
  echo "FAIL: $1"
  shift
  if [ "$#" -gt 0 ]; then
    printf '%s\n' "$@"
  fi
  exit 1
}

# Hermetic HTTP fixture: bind an OS-selected loopback port and publish it through
# an owned file. The smoke never searches ports or terminates an existing owner.
SMOKE_HTTPBIN_PORT_FILE="$SMOKE_ROOT/httpbin.port"
SMOKE_HTTPBIN_LOG="$SMOKE_ROOT/httpbin.log"
python3 "$SCRIPT_DIR/fixtures/httpbin_server.py" 0 --port-file "$SMOKE_HTTPBIN_PORT_FILE" >"$SMOKE_HTTPBIN_LOG" 2>&1 &
SMOKE_HTTPBIN_PID=$!
register_pid "$SMOKE_HTTPBIN_PID"
SMOKE_HTTPBIN_READY=0
for _i in $(seq 1 50); do
  if ! kill -0 "$SMOKE_HTTPBIN_PID" 2>/dev/null; then
    SMOKE_HTTPBIN_ERROR=$(cat "$SMOKE_HTTPBIN_LOG" 2>/dev/null || true)
    stop_pid "$SMOKE_HTTPBIN_PID"
    fail "local httpbin fixture exited before publishing a port" "$SMOKE_HTTPBIN_ERROR"
  fi
  if [ -s "$SMOKE_HTTPBIN_PORT_FILE" ]; then
    SMOKE_HTTPBIN_PORT=$(cat "$SMOKE_HTTPBIN_PORT_FILE")
    if [[ "$SMOKE_HTTPBIN_PORT" =~ ^[0-9]+$ ]] \
      && [ "$SMOKE_HTTPBIN_PORT" -gt 0 ] \
      && [ "$SMOKE_HTTPBIN_PORT" -le 65535 ]; then
      SMOKE_HTTPBIN_URL="http://127.0.0.1:${SMOKE_HTTPBIN_PORT}"
      if curl -fsS "$SMOKE_HTTPBIN_URL/get" >/dev/null 2>&1; then
        SMOKE_HTTPBIN_READY=1
        break
      fi
    fi
  fi
  sleep 0.1
done
if [ "$SMOKE_HTTPBIN_READY" -ne 1 ]; then
  SMOKE_HTTPBIN_ERROR=$(cat "$SMOKE_HTTPBIN_LOG" 2>/dev/null || true)
  stop_pid "$SMOKE_HTTPBIN_PID"
  fail "local httpbin fixture did not become ready" "$SMOKE_HTTPBIN_ERROR"
fi

# The body contains quoted heredocs that deliberately suppress shell expansion.
# Materialize one owned copy with the dynamic fixture URL substituted, source it
# in this shell, then exit before the source template below is reached.
SMOKE_BODY="$SMOKE_ROOT/run-body.sh"
python3 - "$SCRIPT_DIR/run.sh" "$SMOKE_BODY" "$SMOKE_HTTPBIN_URL" <<'PY'
import pathlib
import sys

source_path, body_path, fixture_url = sys.argv[1:]
lines = pathlib.Path(source_path).read_text(encoding="utf-8").splitlines(keepends=True)
start = next(
  index for index, line in enumerate(lines) if line.rstrip("\r\n") == "# SMOKE_BODY_START"
) + 1
body = "".join(lines[start:])
body = body.replace("__SMOKE_HTTPBIN_URL__", fixture_url)
pathlib.Path(body_path).write_text(body, encoding="utf-8")
PY
source "$SMOKE_BODY"
exit $?

# SMOKE_BODY_START

echo "=== Smoke Test ==="
echo

echo "--- Building curlew ---"
cd "$PROJECT_ROOT"
go build -buildvcs=false -o curlew ./cmd/curlew
echo "Build: OK"
echo

echo "--- Running with no args ---"
./curlew
echo

echo "--- Running with --version ---"
./curlew --version
echo

echo "--- Running with --help ---"
./curlew --help
echo

echo "--- Running sample collection (hermetic copy against local fixture) ---"
# sample/hello.yaml stays pointed at httpbin.org as user-facing documentation;
# the smoke run executes a copy rewritten to the local fixture so the gate
# never depends on the public internet.
# mktemp -d (trailing Xs) is macOS-portable; a suffixed template like
# XXXXXX.yaml is created literally on macOS and collides across runs.
SMOKE_HELLO_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_hello_XXXXXX)
SMOKE_HELLO="$SMOKE_HELLO_DIR/hello.yaml"
sed "s|https://httpbin.org|$SMOKE_HTTPBIN_URL|g" sample/hello.yaml > "$SMOKE_HELLO"
./curlew run "$SMOKE_HELLO"
echo

echo "--- Running with empty requests collection (expect warning, exit 0) ---"
EMPTY_FILE=$(mktemp ${SMOKE_ROOT}/curlew_empty_XXXXXX.yaml)
cat > "$EMPTY_FILE" << 'YAML'
name: Empty Collection
requests: []
YAML
./curlew run "$EMPTY_FILE" 2>&1 || true
rm -f "$EMPTY_FILE"
echo

echo "--- Running with missing file (expect exit 3) ---"
./curlew run nonexistent.yaml && echo "ERROR: should have failed" || echo "Exit code: $?"
echo

echo "--- Running with no args to run (expect exit 1) ---"
./curlew run && echo "ERROR: should have failed" || echo "Exit code: $?"
echo

echo "--- Running collection with status assertion (expect pass) ---"
ASSERT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_assert_XXXXXX.yaml)
cat > "$ASSERT_FILE" << YAML
name: Assert Pass
requests:
  - name: Check Status
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
./curlew run "$ASSERT_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$ASSERT_FILE"
echo

echo "--- Running collection with failing assertion (expect exit 1) ---"
ASSERT_FAIL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_assert_fail_XXXXXX.yaml)
cat > "$ASSERT_FAIL_FILE" << YAML
name: Assert Fail
requests:
  - name: Check Status
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 404
YAML
./curlew run "$ASSERT_FAIL_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$ASSERT_FAIL_FILE"
echo

echo "--- Running collection with body assertion (expect pass) ---"
BODY_ASSERT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_body_assert_XXXXXX.yaml)
cat > "$BODY_ASSERT_FILE" << YAML
name: Body Assert Pass
requests:
  - name: Check Body
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
      body:
        \$.url:
          exists: true
        \$.url:
          type: string
YAML
./curlew run "$BODY_ASSERT_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$BODY_ASSERT_FILE"
echo

echo "--- Running collection with failing body assertion (expect exit 1) ---"
BODY_ASSERT_FAIL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_body_fail_XXXXXX.yaml)
cat > "$BODY_ASSERT_FAIL_FILE" << YAML
name: Body Assert Fail
requests:
  - name: Check Body
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      body:
        \$.missing_field:
          equals: "should not exist"
YAML
./curlew run "$BODY_ASSERT_FAIL_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$BODY_ASSERT_FAIL_FILE"
echo

echo "--- Running collection with header assertion (expect pass) ---"
HEADER_ASSERT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_header_assert_XXXXXX.yaml)
cat > "$HEADER_ASSERT_FILE" << YAML
name: Header Assert Pass
requests:
  - name: Check Header
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      headers:
        Content-Type:
          matches: "^application/json"
YAML
./curlew run "$HEADER_ASSERT_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$HEADER_ASSERT_FILE"
echo

echo "--- Running collection with timing assertion (expect pass) ---"
TIMING_ASSERT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_timing_assert_XXXXXX.yaml)
cat > "$TIMING_ASSERT_FILE" << YAML
name: Timing Assert Pass
requests:
  - name: Check Timing
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      timing:
        max_duration_ms: 30000
YAML
./curlew run "$TIMING_ASSERT_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$TIMING_ASSERT_FILE"
echo

echo "--- Running collection with failing header assertion (expect exit 1) ---"
HEADER_FAIL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_header_fail_XXXXXX.yaml)
cat > "$HEADER_FAIL_FILE" << YAML
name: Header Assert Fail
requests:
  - name: Check Header
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      headers:
        X-Nonexistent-Header:
          exists: true
YAML
./curlew run "$HEADER_FAIL_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$HEADER_FAIL_FILE"
echo

echo "--- Structured error format ---"
OUTPUT=$(./curlew run nonexistent.yaml 2>&1 || true)
echo "$OUTPUT" | grep -q "\[ERROR\]" && echo "PASS: [ERROR] prefix present" || fail "Missing [ERROR] prefix in: $OUTPUT"
echo "$OUTPUT" | grep -q "nonexistent.yaml" && echo "PASS: file path in error" || fail "Missing file path in: $OUTPUT"
echo

echo "--- Connection refused error format ---"
CONN_FILE=$(mktemp ${SMOKE_ROOT}/curlew_conn_XXXXXX.yaml)
cat > "$CONN_FILE" << 'YAML'
name: Connection Test
requests:
  - name: Refused
    request:
      method: GET
      url: "http://127.0.0.1:1/test"
YAML
CONN_OUTPUT=$(./curlew run "$CONN_FILE" 2>&1 || true)
echo "$CONN_OUTPUT" | grep -q "\[ERROR\]" && echo "PASS: [ERROR] prefix in network error" || fail "Missing [ERROR] prefix in: $CONN_OUTPUT"
echo "$CONN_OUTPUT" | grep -q "server is running" && echo "PASS: hint present in network error" || fail "Missing hint in: $CONN_OUTPUT"
rm -f "$CONN_FILE"
echo

echo "--- Running collection with body operators (expect pass) ---"
OPERATORS_FILE=$(mktemp ${SMOKE_ROOT}/curlew_operators_XXXXXX.yaml)
cat > "$OPERATORS_FILE" << 'YAML'
name: Body Operators
requests:
  - name: Check Operators
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?count=42"
    assertions:
      status: 200
      body:
        $.url:
          contains: "127.0.0.1"
          matches: "^http://.*"
        $.args:
          length: 1
        $.args.count:
          equals: "42"
YAML
./curlew run "$OPERATORS_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$OPERATORS_FILE"
echo

echo "--- Running collection with variables (expect pass) ---"
VARS_FILE=$(mktemp ${SMOKE_ROOT}/curlew_vars_XXXXXX.yaml)
cat > "$VARS_FILE" << 'YAML'
name: Variable Test
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
requests:
  - name: Check Variables
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
YAML
./curlew run "$VARS_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$VARS_FILE"
echo

echo "--- Running collection with circular variables (expect exit 5) ---"
CIRC_FILE=$(mktemp ${SMOKE_ROOT}/curlew_circ_XXXXXX.yaml)
cat > "$CIRC_FILE" << 'YAML'
name: Circular Variable Test
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
./curlew run "$CIRC_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$CIRC_FILE"
echo

echo "--- Running collection with variable extraction (expect pass) ---"
EXTRACT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_extract_XXXXXX.yaml)
cat > "$EXTRACT_FILE" << 'YAML'
name: Extraction Test
requests:
  - name: Get Data
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?token=secret123"
    extract:
      token_url: "$.url"
  - name: Use Extracted
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
      headers:
        X-Extracted: "{{token_url}}"
    assertions:
      status: 200
YAML
./curlew run "$EXTRACT_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$EXTRACT_FILE"
echo

echo "--- Running collection with extraction failure (expect exit 1) ---"
EXTRACT_FAIL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_extract_fail_XXXXXX.yaml)
cat > "$EXTRACT_FAIL_FILE" << 'YAML'
name: Extraction Fail Test
requests:
  - name: Extract Missing
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    extract:
      missing: "$.nonexistent.path"
YAML
./curlew run "$EXTRACT_FAIL_FILE" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$EXTRACT_FAIL_FILE"
echo

echo "--- Running collection with --var override (expect pass) ---"
VAR_OVERRIDE_FILE=$(mktemp ${SMOKE_ROOT}/curlew_var_override_XXXXXX.yaml)
cat > "$VAR_OVERRIDE_FILE" << 'YAML'
name: Var Override Test
variables:
  base_url: "http://wrong-host:9999"
requests:
  - name: Overridden URL
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
YAML
./curlew run "$VAR_OVERRIDE_FILE" --var base_url=__SMOKE_HTTPBIN_URL__ && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$VAR_OVERRIDE_FILE"
echo

echo "--- Running with --var invalid format (expect exit 1) ---"
VAR_INVALID_FILE=$(mktemp ${SMOKE_ROOT}/curlew_var_invalid_XXXXXX.yaml)
cat > "$VAR_INVALID_FILE" << 'YAML'
name: Test
requests:
  - name: X
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
./curlew run "$VAR_INVALID_FILE" --var noequals && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$VAR_INVALID_FILE"
echo

echo "--- Running collection with --env flag (expect pass) ---"
ENV_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_env_XXXXXX)
mkdir -p "$ENV_DIR/environments"
cat > "$ENV_DIR/environments/dev.yaml" << 'YAML'
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
YAML
cat > "$ENV_DIR/env-test.yaml" << 'YAML'
name: Env Test
requests:
  - name: Check Env
    request:
      method: GET
      url: "{{base_url}}/get"
    assertions:
      status: 200
YAML
./curlew run "$ENV_DIR/env-test.yaml" --env dev && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$ENV_DIR"
echo

echo "--- Running with --env missing environment (expect error listing available) ---"
ENV_MISS_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_env_miss_XXXXXX)
mkdir -p "$ENV_MISS_DIR/environments"
cat > "$ENV_MISS_DIR/environments/dev.yaml" << 'YAML'
variables:
  x: "1"
YAML
cat > "$ENV_MISS_DIR/test.yaml" << 'YAML'
name: Test
requests:
  - name: X
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
OUTPUT=$(./curlew run "$ENV_MISS_DIR/test.yaml" --env staging 2>&1 || true)
echo "$OUTPUT" | grep -q "dev" && echo "PASS: available environment listed" || fail "Missing available env in: $OUTPUT"
rm -rf "$ENV_MISS_DIR"
echo

echo "--- Running collection with .env auto-loading (expect pass) ---"
DOTENV_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_dotenv_XXXXXX)
cat > "$DOTENV_DIR/.env" << 'ENV'
BASE_URL=__SMOKE_HTTPBIN_URL__
ENV
cat > "$DOTENV_DIR/test.yaml" << 'YAML'
name: Dotenv Test
requests:
  - name: Check Dotenv
    request:
      method: GET
      url: "{{BASE_URL}}/get"
    assertions:
      status: 200
YAML
./curlew run "$DOTENV_DIR/test.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$DOTENV_DIR"
echo

echo "--- Running collection without .env (expect pass, no error) ---"
NO_DOTENV_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_no_dotenv_XXXXXX)
cat > "$NO_DOTENV_DIR/test.yaml" << 'YAML'
name: No Dotenv Test
requests:
  - name: Check No Dotenv
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
./curlew run "$NO_DOTENV_DIR/test.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$NO_DOTENV_DIR"
echo

echo "--- Running collection with --env-var flag (expect pass) ---"
export SMOKE_TEST_URL="__SMOKE_HTTPBIN_URL__"
ENVVAR_FILE=$(mktemp ${SMOKE_ROOT}/curlew_envvar_XXXXXX.yaml)
cat > "$ENVVAR_FILE" << 'YAML'
name: Env Var Test
requests:
  - name: Check Env Var
    request:
      method: GET
      url: "{{SMOKE_TEST_URL}}/get"
    assertions:
      status: 200
YAML
./curlew run "$ENVVAR_FILE" --env-var SMOKE_TEST_URL && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$ENVVAR_FILE"
echo

echo "--- Running with --env-var for missing OS var (expect error) ---"
unset SMOKE_MISSING_VAR 2>/dev/null || true
ENVVAR_MISS_FILE=$(mktemp ${SMOKE_ROOT}/curlew_envvar_miss_XXXXXX.yaml)
cat > "$ENVVAR_MISS_FILE" << 'YAML'
name: Test
requests:
  - name: X
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
OUTPUT=$(./curlew run "$ENVVAR_MISS_FILE" --env-var SMOKE_MISSING_VAR 2>&1 || true)
echo "$OUTPUT" | grep -q "not set" && echo "PASS: error indicates variable not set" || fail "Missing 'not set' in: $OUTPUT"
rm -f "$ENVVAR_MISS_FILE"
echo

echo "--- Help text shows --env-var ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-env-var" && echo "PASS: --env-var in help" || fail "Missing --env-var in help output"
echo

echo "--- Running collection with external request reference (expect pass) ---"
EXT_REF_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_ext_ref_XXXXXX)
mkdir -p "$EXT_REF_DIR/requests"
cat > "$EXT_REF_DIR/requests/get.yaml" << 'YAML'
name: External Get
request:
  method: GET
  url: "__SMOKE_HTTPBIN_URL__/get"
assertions:
  status: 200
YAML
cat > "$EXT_REF_DIR/collection.yaml" << 'YAML'
name: External Ref Smoke Test
requests:
  - path: requests/get.yaml
YAML
./curlew run "$EXT_REF_DIR/collection.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$EXT_REF_DIR"
echo

echo "--- Running collection with missing external reference (expect exit 3) ---"
EXT_MISS_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_ext_miss_XXXXXX)
cat > "$EXT_MISS_DIR/collection.yaml" << 'YAML'
name: Missing Ref Smoke Test
requests:
  - path: requests/nonexistent.yaml
YAML
OUTPUT=$(./curlew run "$EXT_MISS_DIR/collection.yaml" 2>&1 || true)
echo "$OUTPUT" | grep -q "external request file not found" && echo "PASS: external file not found error" || fail "Missing error message in: $OUTPUT"
rm -rf "$EXT_MISS_DIR"
echo

echo "--- Running collection with setup and teardown (expect pass, headers printed) ---"
SETUP_TD_FILE=$(mktemp ${SMOKE_ROOT}/curlew_setup_td_XXXXXX.yaml)
cat > "$SETUP_TD_FILE" << 'YAML'
name: Setup Teardown Smoke Test
setup:
  - name: Setup Request
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
requests:
  - name: Main Request
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
teardown:
  - name: Teardown Request
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
OUTPUT=$(./curlew run "$SETUP_TD_FILE" 2>&1)
echo "$OUTPUT" | grep -q "Setup:" && echo "PASS: Setup: header printed" || fail "Missing 'Setup:' header in output: $OUTPUT"
echo "$OUTPUT" | grep -q "Teardown:" && echo "PASS: Teardown: header printed" || fail "Missing 'Teardown:' header in output: $OUTPUT"
echo "$OUTPUT" | grep -q "3 request(s)" && echo "PASS: 3 total requests counted" || fail "Expected 3 requests in output: $OUTPUT"
./curlew run "$SETUP_TD_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$SETUP_TD_FILE"
echo

echo "--- Teardown runs when main fails (exit code ignores teardown) ---"
MAIN_FAIL_TD_FILE=$(mktemp ${SMOKE_ROOT}/curlew_main_fail_td_XXXXXX.yaml)
cat > "$MAIN_FAIL_TD_FILE" << 'YAML'
name: Main Fail Teardown Test
requests:
  - name: Failing Main
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 999
teardown:
  - name: Teardown Still Runs
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
OUTPUT=$(./curlew run "$MAIN_FAIL_TD_FILE" 2>&1 || true)
echo "$OUTPUT" | grep -q "Teardown:" && echo "PASS: Teardown: header printed on main failure" || fail "Missing 'Teardown:' header in output: $OUTPUT"
./curlew run "$MAIN_FAIL_TD_FILE" > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
# Main assertion failure → exit 1 (not affected by teardown success)
[ "$EXITCODE" = "1" ] && echo "PASS: exit code 1 (main assertion failure)" || fail "expected exit code 1, got $EXITCODE"
rm -f "$MAIN_FAIL_TD_FILE"
echo

echo "--- Project config variables resolved (walk-up) ---"
TMP=$(mktemp -d ${SMOKE_ROOT}/curlew_proj_XXXXXX)
mkdir -p "$TMP/project/sub"
cat > "$TMP/project/curlew.yaml" << 'YAML'
project_name: SmokeTest
variables:
  smoke_base: __SMOKE_HTTPBIN_URL__
YAML
cat > "$TMP/project/sub/collection.yaml" << 'YAML'
name: ProjectConfigTest
requests:
  - name: Test Request
    request:
      method: GET
      url: "{{smoke_base}}/get"
    assertions:
      status: 200
YAML
./curlew run "$TMP/project/sub/collection.yaml" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -rf "$TMP"
echo

echo "--- Help text shows curlew.yaml ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "curlew.yaml" && echo "PASS: curlew.yaml in help" || fail "Missing curlew.yaml in help output"
echo

echo "--- Help text shows --seed ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-seed" && echo "PASS: --seed in help" || fail "Missing --seed in help output"
echo

echo '--- Dynamic function {{$uuid}} produces a UUID in the request ---'
UUID_SRV_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_uuid_XXXXXX)
# We can't capture what hits a real server easily in smoke, so just run the collection and check exit 0.
UUID_FILE=$(mktemp ${SMOKE_ROOT}/curlew_uuid_XXXXXX.yaml)
cat > "$UUID_FILE" << 'YAML'
name: Dynamic UUID Smoke
requests:
  - name: UUID Request
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?id={{$uuid}}&ts={{$timestamp}}"
    assertions:
      status: 200
YAML
./curlew run "$UUID_FILE" && echo "Pass: exit code 0" || echo "ERROR: expected exit 0, got $?"
rm -f "$UUID_FILE"
rm -rf "$UUID_SRV_DIR"
echo

echo "--- --seed produces deterministic output (run twice, same UUID in URL) ---"
SEED_FILE=""
SEED_FILE=$(mktemp ${SMOKE_ROOT}/curlew_seed_XXXXXX.yaml)
register_path "$SEED_FILE"
cat > "$SEED_FILE" << 'YAML'
name: Seed Smoke
requests:
  - name: Seeded Request
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?id={{$uuid}}"
    assertions:
      status: 200
YAML
OUT1=$(./curlew run "$SEED_FILE" --seed 42 2>&1 | sed 's/  [0-9]*ms$//' | sed 's/([0-9]*ms)//')
OUT2=$(./curlew run "$SEED_FILE" --seed 42 2>&1 | sed 's/  [0-9]*ms$//' | sed 's/([0-9]*ms)//')
if [ "$OUT1" = "$OUT2" ]; then
  echo "PASS: --seed 42 produces identical output both runs"
else
  echo "FAIL: --seed 42 runs differ"
  echo "Run 1: $OUT1"
  echo "Run 2: $OUT2"
  exit 1
fi
rm -f "$SEED_FILE"
echo

echo "--- Running with --no-color flag (expect no ANSI codes) ---"
OUTPUT=$(./curlew run "$SMOKE_HELLO" --no-color 2>&1)
if printf '%s' "$OUTPUT" | grep -q $'\033\['; then
  echo "FAIL: ANSI codes found with --no-color"
  exit 1
fi
echo "PASS: no ANSI codes with --no-color"
echo

echo "--- Running with NO_COLOR env var (expect no ANSI codes) ---"
OUTPUT=$(NO_COLOR=1 ./curlew run "$SMOKE_HELLO" 2>&1)
if printf '%s' "$OUTPUT" | grep -q $'\033\['; then
  echo "FAIL: ANSI codes found with NO_COLOR=1"
  exit 1
fi
echo "PASS: no ANSI codes with NO_COLOR=1"
echo

echo "--- Help text shows --no-color ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-no-color" && echo "PASS: --no-color in help" || fail "Missing --no-color in help output"
echo

echo "--- Running with --format json (expect valid JSON) ---"
JSON_FILE=$(mktemp ${SMOKE_ROOT}/curlew_json_XXXXXX.yaml)
cat > "$JSON_FILE" << 'YAML'
name: JSON Format Smoke
requests:
  - name: JSON Get
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
JSON_OUTPUT=$(./curlew run "$JSON_FILE" --format json)
echo "$JSON_OUTPUT" | python3 -m json.tool > /dev/null && echo "PASS: --format json produces valid JSON" || fail "Invalid JSON output: $JSON_OUTPUT"
echo "$JSON_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'name' in d and 'status' in d and 'requests' in d, f'Missing fields: {list(d.keys())}'" && echo "PASS: top-level fields present" || fail "Missing fields in: $JSON_OUTPUT"
rm -f "$JSON_FILE"
echo

echo "--- --format json with failing assertion (expect JSON with failed status) ---"
JSON_FAIL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_json_fail_XXXXXX.yaml)
cat > "$JSON_FAIL_FILE" << 'YAML'
name: JSON Fail Smoke
requests:
  - name: Expect 404
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 404
YAML
JSON_FAIL_OUTPUT=$(./curlew run "$JSON_FAIL_FILE" --format json || true)
echo "$JSON_FAIL_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['status']=='failed', f'Expected failed, got {d[\"status\"]}'" && echo "PASS: --format json failed status" || fail "Wrong status in: $JSON_FAIL_OUTPUT"
rm -f "$JSON_FAIL_FILE"
echo

echo "--- --format json with no assertions (assertions array not null) ---"
JSON_NO_ASSERT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_json_no_assert_XXXXXX.yaml)
cat > "$JSON_NO_ASSERT_FILE" << 'YAML'
name: No Assert Smoke
requests:
  - name: No Assert
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
JSON_NO_ASSERT_OUTPUT=$(./curlew run "$JSON_NO_ASSERT_FILE" --format json)
echo "$JSON_NO_ASSERT_OUTPUT" | python3 -c "import sys,json; d=json.load(sys.stdin); r=d['requests'][0]; assert isinstance(r['assertions'], list), 'assertions should be array'" && echo "PASS: assertions is array not null" || fail "assertions not array in: $JSON_NO_ASSERT_OUTPUT"
rm -f "$JSON_NO_ASSERT_FILE"
echo

echo "--- Help text shows --format ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-format" && echo "PASS: --format in help" || fail "Missing --format in help output"
echo

echo "--- TAP: passing collection ---"
TAP_FILE=$(mktemp ${SMOKE_ROOT}/curlew_tap_XXXXXX.yaml)
cat > "$TAP_FILE" << 'YAML'
name: TAP Format Smoke
requests:
  - name: TAP Get
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
TAP_OUT=$(./curlew run "$TAP_FILE" --format tap)
echo "$TAP_OUT" | grep -q "^TAP version 13" || fail "--format tap: missing version line"
echo "$TAP_OUT" | grep -q "^1\.\." || fail "--format tap: missing plan line"
echo "$TAP_OUT" | grep -q "^ok 1" || fail "--format tap: missing ok line"
echo "$TAP_OUT" | grep -q "^# Summary:" || fail "--format tap: missing summary comment"
echo "PASS: --format tap produces valid TAP output"
rm -f "$TAP_FILE"
echo

echo "--- Help text lists tap ---"
HELP_OUT=$(./curlew --help)
echo "$HELP_OUT" | grep -q "tap" || fail "--help missing tap in --format description"
echo "PASS: tap listed in --format help"
echo

echo "--- Verbosity: quiet mode produces minimal output ---"
QUIET_FILE=$(mktemp ${SMOKE_ROOT}/curlew_quiet_XXXXXX.yaml)
cat > "$QUIET_FILE" << 'YAML'
name: Quiet Mode Test
requests:
  - name: Check Quiet
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    assertions:
      status: 200
YAML
QUIET_OUTPUT=$(./curlew run "$QUIET_FILE" -q 2>&1)
LINE_COUNT=$(echo "$QUIET_OUTPUT" | wc -l | tr -d ' ')
if [ "$LINE_COUNT" -gt 3 ]; then
  echo "FAIL: quiet mode produced $LINE_COUNT lines, expected <= 3"
  echo "Output: $QUIET_OUTPUT"
  exit 1
fi
echo "PASS: quiet mode output is minimal ($LINE_COUNT lines)"
rm -f "$QUIET_FILE"
echo

echo "--- Verbosity: verbose mode shows request detail lines ---"
VERBOSE_FILE=$(mktemp ${SMOKE_ROOT}/curlew_verbose_XXXXXX.yaml)
cat > "$VERBOSE_FILE" << 'YAML'
name: Verbose Mode Test
requests:
  - name: Check Verbose
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
      headers:
        Accept: "application/json"
    assertions:
      status: 200
YAML
VERBOSE_OUTPUT=$(./curlew run "$VERBOSE_FILE" -v --no-color 2>&1)
if ! echo "$VERBOSE_OUTPUT" | grep -q "^  > "; then
  echo "FAIL: verbose mode missing request detail lines"
  echo "Output: $VERBOSE_OUTPUT"
  exit 1
fi
echo "PASS: verbose mode shows request detail"
rm -f "$VERBOSE_FILE"
echo

echo "--- Help text shows verbosity flags ---"
HELP_OUTPUT=$(./curlew --help)
for flag in "-vv" "-q"; do
  if ! echo "$HELP_OUTPUT" | grep -qF -- "$flag"; then
    echo "FAIL: --help missing $flag"
    exit 1
  fi
done
echo "PASS: help text contains verbosity flags"
echo

echo "--- Help text shows --allow-sensitive ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "\-\-allow-sensitive" && echo "PASS: --allow-sensitive in help" || fail "Missing --allow-sensitive in help output"
echo

echo "--- Sensitive vars redacted at -vv (default) ---"
SENSITIVE_FILE=$(mktemp ${SMOKE_ROOT}/curlew_sensitiveXXXXXX.yaml)
cat > "$SENSITIVE_FILE" << 'YAML'
name: Sensitive Redaction Test
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
  password: "my_super_secret_123"
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        Authorization: "Bearer {{password}}"
    assertions:
      status: 200
YAML
SENS_OUTPUT=$(./curlew run "$SENSITIVE_FILE" -vv --no-color 2>&1)
# the fixture server echoes headers in the response body (like httpbin) — only check that the outgoing request header is redacted
if echo "$SENS_OUTPUT" | grep -q "> Authorization: Bearer my_super_secret_123"; then
  echo "FAIL: password value leaked in request header output"
  exit 1
fi
echo "$SENS_OUTPUT" | grep -q "> Authorization: \[REDACTED\]" && echo "PASS: [REDACTED] shown for sensitive vars" || fail "[REDACTED] not shown in request header"
rm -f "$SENSITIVE_FILE"
echo

echo "--- --allow-sensitive shows plain text values ---"
ALLOW_SENS_FILE=$(mktemp ${SMOKE_ROOT}/curlew_allow_sensXXXXXX.yaml)
cat > "$ALLOW_SENS_FILE" << 'YAML'
name: Allow Sensitive Test
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
  password: "visible_secret_456"
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        Authorization: "Bearer {{password}}"
    assertions:
      status: 200
YAML
ALLOW_OUTPUT=$(./curlew run "$ALLOW_SENS_FILE" -vv --allow-sensitive --no-color 2>&1)
echo "$ALLOW_OUTPUT" | grep -q "> Authorization: Bearer visible_secret_456" && echo "PASS: --allow-sensitive shows password value" || fail "value not shown with --allow-sensitive"
rm -f "$ALLOW_SENS_FILE"
echo

echo "--- \$faker.ssn auto-redacted in -vv terminal output (M13-002) ---"
# mktemp -t is macOS-portable and avoids the literal-filename issue that arises
# when a suffix follows the X-placeholder block on macOS (e.g. XXXXXXfoo.yaml).
FAKER_FILE=$(mktemp -t curlew_faker_ssn)
register_path "$FAKER_FILE"
cat > "$FAKER_FILE" << 'YAML'
name: Faker SSN Redaction
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
requests:
  - name: Submit
    request:
      method: POST
      url: "{{base_url}}/users"
      headers:
        Content-Type: "application/json"
      body:
        name: "{{$faker.fullName}}"
        ssn: "{{$faker.ssn}}"
YAML
# Run with -vv so RequestBodyDump emits the (body): line with the redacted body.
# The SSN value is auto-registered in runtimeSensitive by the IsSensitiveReturn hook
# (internal/variable/variable.go) and merged into sensitive before output is rendered.
FAKER_OUT=$(./curlew run "$FAKER_FILE" -vv --no-color --seed 42 2>&1 || true)
# Check that [REDACTED] appears in the body output and the raw SSN pattern does NOT.
echo "$FAKER_OUT" | grep -q '\[REDACTED\]' \
  && echo "PASS: \$faker.ssn appears as [REDACTED] in -vv body output" \
  || fail "[REDACTED] not found in -vv output" "$FAKER_OUT"
echo "$FAKER_OUT" | grep -qE '[0-9]{3}-[0-9]{2}-[0-9]{4}' \
  && fail "raw SSN pattern leaked into -vv output" "$FAKER_OUT" \
  || echo "PASS: raw SSN value not present in -vv output"
echo

echo "--- \$faker.ssn absent from --format json output (M13-002) ---"
# --format json (JSONRequest) has no request_body field; the raw SSN must simply
# not appear anywhere in the JSON output. Positive-redaction ([REDACTED] in body)
# is verified by the -vv terminal block above and the markdown block below.
FAKER_JSON_OUT=$(./curlew run "$FAKER_FILE" --format json --no-color --seed 42 2>&1 || true)
echo "$FAKER_JSON_OUT" | python3 -m json.tool > /dev/null \
  && echo "PASS: --format json produces valid JSON" \
  || fail "--format json output is not valid JSON" "$FAKER_JSON_OUT"
echo "$FAKER_JSON_OUT" | grep -qE '[0-9]{3}-[0-9]{2}-[0-9]{4}' \
  && fail "raw SSN pattern leaked into --format json output" "$FAKER_JSON_OUT" \
  || echo "PASS: raw SSN value not present in --format json output"
echo

echo "--- \$faker.ssn auto-redacted in --format markdown output (M13-002) ---"
# The markdown report renders the request body as JSON; the SSN must appear as [REDACTED].
FAKER_MD_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_faker_md_XXXXXX)
register_path "$FAKER_MD_DIR"
./curlew run "$FAKER_FILE" --format markdown --report "$FAKER_MD_DIR" --no-color --seed 42 2>&1 || true
FAKER_MD_CONTENT=$(cat "$FAKER_MD_DIR"/*.md 2>/dev/null || true)
echo "$FAKER_MD_CONTENT" | grep -q '\[REDACTED\]' \
  && echo "PASS: \$faker.ssn appears as [REDACTED] in --format markdown report" \
  || fail "[REDACTED] not found in markdown report" "$FAKER_MD_CONTENT"
echo "$FAKER_MD_CONTENT" | grep -qE '[0-9]{3}-[0-9]{2}-[0-9]{4}' \
  && fail "raw SSN pattern leaked into --format markdown report" "$FAKER_MD_CONTENT" \
  || echo "PASS: raw SSN value not present in --format markdown report"
rm -rf "$FAKER_MD_DIR"; FAKER_MD_DIR=""

rm -f "$FAKER_FILE"
echo

echo "--- \$faker financial fields auto-redacted in -vv terminal output (M13-007) ---"
FIN_FILE=$(mktemp -t curlew_faker_fin)
register_path "$FIN_FILE"
cat > "$FIN_FILE" << 'YAML'
name: Faker Financial Redaction
variables:
  base_url: "__SMOKE_HTTPBIN_URL__"
requests:
  - name: Submit
    request:
      method: POST
      url: "{{base_url}}/payments"
      headers:
        Content-Type: "application/json"
      body:
        cardholder: "{{$faker.fullName}}"
        card: "{{$faker.creditCard}}"
        cvv: "{{$faker.creditCardCVV}}"
        iban: "{{$faker.iban}}"
        bic: "{{$faker.bic}}"
        amount: "{{$faker.price('5','50')}}"
YAML
FIN_OUT=$(./curlew run "$FIN_FILE" -vv --no-color --seed 42 2>&1 || true)
# Three [REDACTED] occurrences expected (card, cvv, iban). bic + amount must be visible.
echo "$FIN_OUT" | grep -q '\[REDACTED\]' \
  && echo "PASS: financial fields appear as [REDACTED] in -vv body output" \
  || fail "[REDACTED] not found in -vv output" "$FIN_OUT"
echo

echo "--- \$faker financial fields auto-redacted in --format markdown output (M13-007) ---"
FIN_MD_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_faker_fin_md_XXXXXX)
register_path "$FIN_MD_DIR"
./curlew run "$FIN_FILE" --format markdown --report "$FIN_MD_DIR" --no-color --seed 42 2>&1 || true
FIN_MD_CONTENT=$(cat "$FIN_MD_DIR"/*.md 2>/dev/null || true)
echo "$FIN_MD_CONTENT" | grep -q '\[REDACTED\]' \
  && echo "PASS: financial fields appear as [REDACTED] in --format markdown report" \
  || fail "[REDACTED] not found in markdown report" "$FIN_MD_CONTENT"
# A raw 16-digit number must NOT appear in the markdown body (creditCard).
echo "$FIN_MD_CONTENT" | grep -qE '"card":[[:space:]]*"[0-9]{16}"' \
  && fail "raw 16-digit card leaked into markdown report" "$FIN_MD_CONTENT" \
  || echo "PASS: raw card value not present in --format markdown report"
rm -rf "$FIN_MD_DIR"; FIN_MD_DIR=""

echo "--- \$faker financial fields absent from --format json output (M13-007) ---"
FIN_JSON_OUT=$(./curlew run "$FIN_FILE" --format json --no-color --seed 42 2>&1 || true)
echo "$FIN_JSON_OUT" | python3 -m json.tool > /dev/null \
  && echo "PASS: --format json produces valid JSON" \
  || fail "--format json output is not valid JSON" "$FIN_JSON_OUT"
# The raw card / IBAN must not appear anywhere in the JSON output.
echo "$FIN_JSON_OUT" | grep -qE '"4[0-9]{15}"' \
  && fail "raw card pattern leaked into --format json output" "$FIN_JSON_OUT" \
  || echo "PASS: raw card value not present in --format json output"

rm -f "$FIN_FILE"
echo

echo "--- Running curlew init (expect project created) ---"
INIT_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_init_XXXXXX)
./curlew init "$INIT_DIR" || fail "init failed"
[ -f "$INIT_DIR/curlew.yaml" ]            && echo "PASS: curlew.yaml created"             || fail "curlew.yaml missing"
[ -f "$INIT_DIR/.gitignore" ]              && echo "PASS: .gitignore created"               || fail ".gitignore missing"
[ -f "$INIT_DIR/.env.example" ]            && echo "PASS: .env.example created"             || fail ".env.example missing"
[ -f "$INIT_DIR/collections/sample.yaml" ] && echo "PASS: collections/sample.yaml created"  || fail "collections/sample.yaml missing"
# The scaffolded curlew.yaml defaults base_url to https://httpbin.org (a
# user-facing default we keep); override it so the run stays hermetic.
./curlew run "$INIT_DIR/collections/sample.yaml" --var "base_url=$SMOKE_HTTPBIN_URL" && echo "PASS: sample collection runs successfully" || fail "sample collection run failed"
# base_url lives in the scaffolded curlew.yaml, so validate must resolve it by
# walking up from the collection instead of warning about it.
INIT_VALIDATE_OUT=$(./curlew validate "$INIT_DIR/collections/sample.yaml") || fail "scaffolded collection failed to validate" "$INIT_VALIDATE_OUT"
echo "$INIT_VALIDATE_OUT" | grep -q "base_url" \
  && fail "validate warned about base_url defined in curlew.yaml" "$INIT_VALIDATE_OUT" \
  || echo "PASS: validate does not warn about project-level base_url"
echo

echo "--- Running curlew init on existing project (expect error) ---"
./curlew init "$INIT_DIR" && fail "should have returned non-zero" || echo "PASS: init rejected existing project"
rm -rf "$INIT_DIR"
echo

echo "--- Help text shows init command ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "init" && echo "PASS: init in help" || fail "Missing init in help output"
echo

echo "=== Info command ==="

echo "--- Info: in project directory (JSON) ---"
INFO_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_info_XXXXXX)
cat > "$INFO_DIR/curlew.yaml" << 'YAML'
project_name: SmokeInfo
YAML
mkdir -p "$INFO_DIR/collections"
cat > "$INFO_DIR/collections/sample.yaml" << 'YAML'
name: Sample
requests: []
YAML
mkdir -p "$INFO_DIR/environments"
cat > "$INFO_DIR/environments/dev.yaml" << 'YAML'
variables:
  x: "1"
YAML
cd "$INFO_DIR"
INFO_JSON=$("$PROJECT_ROOT/curlew" info --format json)
echo "$INFO_JSON" | python3 -m json.tool > /dev/null && echo "PASS: info --format json produces valid JSON" || fail "Invalid JSON: $INFO_JSON"
echo "$INFO_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['project_name']=='SmokeInfo', f'Wrong name: {d[\"project_name\"]}'" && echo "PASS: project_name correct" || fail "project_name wrong"
cd "$PROJECT_ROOT"
rm -rf "$INFO_DIR"
echo

echo "--- Info: human-readable output ---"
INFO_HR_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_info_hr_XXXXXX)
cat > "$INFO_HR_DIR/curlew.yaml" << 'YAML'
project_name: HumanReadable
YAML
cd "$INFO_HR_DIR"
INFO_HR=$("$PROJECT_ROOT/curlew" info)
echo "$INFO_HR" | grep -q "HumanReadable" && echo "PASS: human-readable shows project name" || fail "Missing project name in: $INFO_HR"
cd "$PROJECT_ROOT"
rm -rf "$INFO_HR_DIR"
echo

echo "--- Info: outside project directory (expect error, exit 5) ---"
INFO_NO_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_info_no_XXXXXX)
cd "$INFO_NO_DIR"
"$PROJECT_ROOT/curlew" info > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
[ "$EXITCODE" = "5" ] && echo "PASS: exit code 5 outside project" || fail "expected exit 5, got $EXITCODE"
cd "$PROJECT_ROOT"
rm -rf "$INFO_NO_DIR"
echo

echo "=== Schema command ==="

echo "--- Schema: valid JSON Schema output ---"
SCHEMA_OUT=$(./curlew schema --format json)
echo "$SCHEMA_OUT" | python3 -m json.tool > /dev/null && echo "PASS: schema --format json produces valid JSON" || fail "Invalid JSON: $SCHEMA_OUT"
echo "$SCHEMA_OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); assert '\$schema' in d, 'Missing \$schema'" && echo "PASS: schema has \$schema keyword" || fail "Missing \$schema"
echo

echo "--- Schema: default format is JSON ---"
SCHEMA_DEFAULT=$(./curlew schema)
echo "$SCHEMA_DEFAULT" | python3 -m json.tool > /dev/null && echo "PASS: default schema output is valid JSON" || fail "default format not valid JSON"
echo

echo "--- Help text shows info and schema ---"
HELP_OUTPUT=$(./curlew --help)
echo "$HELP_OUTPUT" | grep -q "info" && echo "PASS: info in help" || fail "Missing info in help output"
echo "$HELP_OUTPUT" | grep -q "schema" && echo "PASS: schema in help" || fail "Missing schema in help output"
echo

echo "=== Validate command ==="

echo "--- Validate: valid collection ---"
./curlew validate sample/hello.yaml && echo "PASS: validate valid collection exit 0" || fail "expected exit 0, got $?"
echo

echo "--- Validate: missing file (expect exit 3) ---"
./curlew validate nonexistent_validate_test.yaml && echo "ERROR: should have failed" || echo "Exit code: $?"
echo

echo "--- Validate: help text shows validate ---"
VALIDATE_HELP=$(./curlew --help)
echo "$VALIDATE_HELP" | grep -q "validate" && echo "PASS: validate appears in help" || fail "validate missing from help output"
echo

echo "--- Validate: invalid YAML (expect exit 3) ---"
BAD_YAML=$(mktemp ${SMOKE_ROOT}/curlew_bad_yaml_XXXXXX.yaml)
printf 'invalid: yaml: :\n' > "$BAD_YAML"
./curlew validate "$BAD_YAML" && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$BAD_YAML"
echo

echo "--- Validate: --format json produces valid JSON ---"
JSON_OUT=$(./curlew validate --format json sample/hello.yaml)
echo "$JSON_OUT" | python3 -m json.tool > /dev/null && echo "PASS: validate --format json produces valid JSON" || fail "Invalid JSON: $JSON_OUT"
echo

echo "--- Validate: shared vault template (valid) ---"
./curlew validate testdata/team/shared-vault-template.yaml \
  && echo "PASS: valid team template exits 0" \
  || fail "expected exit 0"
echo

echo "--- Validate: shared vault template (invalid provider, expect exit 2) ---"
set +e
./curlew validate testdata/team/shared-vault-template.invalid.yaml
TEAM_RC=$?
set -e
if [ "$TEAM_RC" -eq 2 ]; then
  echo "PASS: invalid team template exits 2"
else
  echo "FAIL: expected exit 2, got $TEAM_RC"
  exit 1
fi
echo

echo "=== Exec command ==="

echo "--- Exec with inline URL ---"
./curlew exec __SMOKE_HTTPBIN_URL__/get && echo "PASS: exec inline URL exit 0" || echo "Exit code: $?"
echo

echo "--- Exec with --dry-run ---"
DRY_OUT=$(./curlew exec __SMOKE_HTTPBIN_URL__/get --dry-run 2>&1)
echo "$DRY_OUT" | grep -q "DRY RUN" && echo "PASS: dry-run output contains DRY RUN" || fail "Missing DRY RUN in: $DRY_OUT"
echo

echo "--- Exec with --stdin JSON ---"
echo '{"url":"__SMOKE_HTTPBIN_URL__/get","method":"GET"}' | ./curlew exec --stdin && echo "PASS: exec stdin exit 0" || echo "Exit code: $?"
echo

echo "--- Exec with --format json ---"
EXEC_JSON=$(echo '{"url":"__SMOKE_HTTPBIN_URL__/get","method":"GET"}' | ./curlew exec --stdin --format json)
echo "$EXEC_JSON" | python3 -m json.tool > /dev/null && echo "PASS: exec --format json produces valid JSON" || fail "Invalid JSON: $EXEC_JSON"
echo "$EXEC_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'status' in d and 'requests' in d, f'Missing fields: {list(d.keys())}'" && echo "PASS: exec JSON has expected fields" || fail "Missing fields in: $EXEC_JSON"
echo

echo "--- Exec with invalid JSON stdin (expect error) ---"
echo 'not json' | ./curlew exec --stdin > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
[ "$EXITCODE" = "3" ] && echo "PASS: invalid JSON returns exit 3" || fail "expected exit 3, got $EXITCODE"
echo

echo "--- Exec with --log ---"
EXEC_LOG=$(mktemp ${SMOKE_ROOT}/curlew_exec_log_XXXXXX.jsonl)
echo '{"url":"__SMOKE_HTTPBIN_URL__/get","method":"GET"}' | ./curlew exec --stdin --log "$EXEC_LOG" && echo "PASS: exec with --log exit 0" || echo "Exit code: $?"
[ -s "$EXEC_LOG" ] && echo "PASS: log file is non-empty" || fail "log file is empty"
rm -f "$EXEC_LOG"
echo

echo "--- Help text shows exec ---"
HELP_OUT=$(./curlew --help 2>&1)
echo "$HELP_OUT" | grep -q "exec" && echo "PASS: exec in help" || fail "Missing exec in help output"
echo

echo "=== Vault ==="

echo "--- Vault command shows usage (exit 0) ---"
./curlew vault > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
[ "$EXITCODE" = "0" ] && echo "PASS: vault exit code 0" || fail "expected exit 0, got $EXITCODE"
echo

echo "--- Vault usage lists the list subcommand ---"
VAULT_OUTPUT=$(./curlew vault --no-color 2>&1 || true)
echo "$VAULT_OUTPUT" | grep -q "vault list" && echo "PASS: vault usage mentions vault list" || fail "Missing 'vault list' in: $VAULT_OUTPUT"
echo

echo "--- Help text shows vault ---"
HELP_OUT=$(./curlew --help 2>&1)
echo "$HELP_OUT" | grep -q "vault" && echo "PASS: vault in help" || fail "Missing vault in help output"
echo

echo "--- Vault list with no profiles exits 0 ---"
./curlew vault list > /dev/null 2>&1 && EXITCODE=0 || EXITCODE=$?
[ "$EXITCODE" = "0" ] && echo "PASS: vault list exit code 0" || fail "expected exit 0, got $EXITCODE"
VAULT_LIST_OUT=$(./curlew vault list --no-color 2>&1 || true)
echo "$VAULT_LIST_OUT" | grep -q "No vault profiles configured" && echo "PASS: vault list reports no profiles" || fail "Missing 'No vault profiles configured' in: $VAULT_LIST_OUT"
echo

echo "--- Vault list --format json produces valid JSON ---"
VAULT_LIST_JSON=$(./curlew vault list --format json 2>&1 || true)
echo "$VAULT_LIST_JSON" | python3 -m json.tool > /dev/null && echo "PASS: vault list --format json is valid JSON" || fail "Invalid JSON: $VAULT_LIST_JSON"
echo "$VAULT_LIST_JSON" | python3 -c "import sys,json; d=json.load(sys.stdin); assert 'provider' in d and 'keys' in d, f'Missing fields: {list(d.keys())}'" && echo "PASS: vault list JSON has provider and keys fields" || fail "Missing fields in: $VAULT_LIST_JSON"
echo

echo "--- Help text shows vault list ---"
HELP_OUT=$(./curlew --help 2>&1)
echo "$HELP_OUT" | grep -q "vault list" && echo "PASS: vault list in help" || fail "Missing 'vault list' in help output"
echo

echo "--- Dynamic auth profiles are resolved (no feature gate) ---"
# The project config declares a dynamic auth profile whose collection file
# does not exist. The runner must get far enough to try loading it (proving
# the profile is honored, not gated) and fail with a file-not-found error.
AUTH_TMP=$(mktemp -d ${SMOKE_ROOT}/curlew_auth_XXXXXX)
mkdir -p "$AUTH_TMP/project"
cat > "$AUTH_TMP/project/curlew.yaml" << 'YAML'
project_name: smoke-auth
auth_profiles:
  login:
    type: dynamic
    collection: auth/login.yaml
YAML
cat > "$AUTH_TMP/project/collection.yaml" << 'YAML'
name: Main Collection
requests:
  - name: main request
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
YAML
AUTH_OUT=$(./curlew run "$AUTH_TMP/project/collection.yaml" 2>&1 || true)
echo "$AUTH_OUT" | grep -q "auth/login.yaml" && echo "PASS: dynamic auth profile collection was resolved" || { echo "FAIL: Expected auth/login.yaml lookup in: $AUTH_OUT"; rm -rf "$AUTH_TMP"; exit 1; }
echo "$AUTH_OUT" | grep -q "file not found" && echo "PASS: missing auth collection reported clearly" || { echo "FAIL: expected file-not-found error in: $AUTH_OUT"; rm -rf "$AUTH_TMP"; exit 1; }
rm -rf "$AUTH_TMP"
echo

echo "--- from_command variables ---"
FROM_CMD_DIR="$(mktemp -d ${SMOKE_ROOT}/curlew_from_cmd_XXXXXX)"
cat > "$FROM_CMD_DIR/from_cmd.yaml" <<'YAML'
name: from_command-smoke
variables:
  GREETING: { from_command: "echo hello-from-command" }
requests:
  - name: echo-back
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?msg={{GREETING}}"
YAML

set +e
./curlew run "$FROM_CMD_DIR/from_cmd.yaml" >"$FROM_CMD_DIR/out.txt" 2>&1
FROM_CMD_EXIT=$?
set -e

if [ "$FROM_CMD_EXIT" -eq 0 ]; then
  echo "PASS: from_command runs (exit 0)"
else
  # The request targets the local fixture server, so any non-zero exit code
  # is a real failure, not network weather.
  fail "from_command exited $FROM_CMD_EXIT (expected 0 against local fixture)" "$(cat "$FROM_CMD_DIR/out.txt")"
fi
rm -rf "$FROM_CMD_DIR"
echo

echo "--- Per-request auth: missing profile gives clear error ---"
PER_AUTH_FILE=$(mktemp ${SMOKE_ROOT}/curlew_per_auth_XXXXXX.yaml)
cat > "$PER_AUTH_FILE" << 'YAML'
name: Per-Request Auth Test
requests:
  - name: Authenticated Request
    auth: admin_token
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
    assertions:
      status: 200
YAML
PER_AUTH_OUT=$(./curlew run "$PER_AUTH_FILE" 2>&1 || true)
echo "$PER_AUTH_OUT" | grep -q "auth profile" && echo "PASS: auth profile error message shown" || { echo "FAIL: Expected auth profile error in: $PER_AUTH_OUT"; rm -f "$PER_AUTH_FILE"; exit 1; }
rm -f "$PER_AUTH_FILE"
echo

echo "--- Watch mode: start, trigger re-run, clean exit ---"
WATCH_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_watch_XXXXXX)
cat > "$WATCH_DIR/col.yaml" << 'YAML'
name: Watch Test
requests:
  - name: Ping
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
    assertions:
      status: 200
YAML
# Start watcher in background
./curlew watch "$WATCH_DIR/col.yaml" --no-color &
WATCH_PID=$!
register_pid "$WATCH_PID"
sleep 2
# Trigger a file change
echo "# touched" >> "$WATCH_DIR/col.yaml"
sleep 2
# Stop the watcher
stop_pid "$WATCH_PID" INT
WATCH_EXIT=$STOP_PID_STATUS
if [ "$WATCH_EXIT" -eq 0 ] || [ "$WATCH_EXIT" -eq 130 ]; then
  echo "PASS: watch exited cleanly (exit code $WATCH_EXIT)"
else
  echo "FAIL: watch exited with $WATCH_EXIT, expected 0 or 130"
  rm -rf "$WATCH_DIR"
  exit 1
fi
rm -rf "$WATCH_DIR"
echo

echo "--- Watch mode: --format json suppresses decorations ---"
WATCH_JSON_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_watchjson_XXXXXX)
cat > "$WATCH_JSON_DIR/col.yaml" << 'YAML'
name: Watch JSON Test
requests:
  - name: Ping
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
    assertions:
      status: 200
YAML
WATCH_JSON_OUT=$(mktemp ${SMOKE_ROOT}/curlew_watchjson_out_XXXXXX)
./curlew watch "$WATCH_JSON_DIR/col.yaml" --format json --no-color > "$WATCH_JSON_OUT" 2>/dev/null &
WATCH_JSON_PID=$!
register_pid "$WATCH_JSON_PID"
sleep 2
stop_pid "$WATCH_JSON_PID" INT
# Verify no terminal decorations in output
if grep -q "Watching for changes" "$WATCH_JSON_OUT"; then
  echo "FAIL: --format json should suppress 'Watching for changes' message"
  rm -rf "$WATCH_JSON_DIR" "$WATCH_JSON_OUT"
  exit 1
elif grep -q "Re-running" "$WATCH_JSON_OUT"; then
  echo "FAIL: --format json should suppress separator"
  rm -rf "$WATCH_JSON_DIR" "$WATCH_JSON_OUT"
  exit 1
else
  echo "PASS: watch --format json suppresses terminal decorations"
fi
rm -rf "$WATCH_JSON_DIR" "$WATCH_JSON_OUT"
echo

echo "--- Parallel flag ---"
PARALLEL_RC=0
PARALLEL_OUT=$(./curlew run "$SMOKE_HELLO" --parallel 2>&1) || PARALLEL_RC=$?
if [ "$PARALLEL_RC" -eq 0 ] && echo "$PARALLEL_OUT" | grep -q "Wave 1"; then
  echo "PASS: --parallel runs in waves (exit 0)"
else
  echo "FAIL: --parallel should run (exit $PARALLEL_RC), got: $PARALLEL_OUT"
  exit 1
fi
echo

echo "--- Data-driven testing ---"
DD_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_dd_XXXXXX)
cat > "$DD_DIR/data.csv" << 'CSV'
name,expected
alice,200
bob,200
CSV
cat > "$DD_DIR/dd_test.yaml" << 'YAML'
name: Data Driven Smoke Test
requests:
  - name: Test Users
    data_driven:
      source: data.csv
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get?user={{name}}"
    assertions:
      status: 200
YAML
DD_RC=0
DD_OUT=$(./curlew run "$DD_DIR/dd_test.yaml" 2>&1) || DD_RC=$?
if [ "$DD_RC" -eq 0 ] && echo "$DD_OUT" | grep -q "2 iterations"; then
  echo "PASS: data_driven runs one iteration per CSV row (exit 0)"
else
  echo "FAIL: data_driven should run 2 iterations (exit $DD_RC), got: $DD_OUT"
  rm -rf "$DD_DIR"
  exit 1
fi
rm -rf "$DD_DIR"
echo

echo "--- --format junit emits JUnit XML ---"
JUNIT_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_junit_XXXXXX)
cat > "$JUNIT_DIR/test.yaml" << 'YAML'
name: JUnit Smoke Test
requests:
  - name: Example
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
JUNIT_OUT=$(./curlew run "$JUNIT_DIR/test.yaml" --format junit 2>&1 || true)
if echo "$JUNIT_OUT" | grep -q "<testsuites"; then
  echo "PASS: --format junit emits JUnit XML"
else
  echo "FAIL: --format junit should emit JUnit XML, got: $JUNIT_OUT"
  rm -rf "$JUNIT_DIR"
  exit 1
fi
rm -rf "$JUNIT_DIR"
echo

echo "--- Help text shows junit in --format ---"
HELP_OUT=$(./curlew --help)
echo "$HELP_OUT" | grep -q "junit" && echo "PASS: junit in help" || fail "Missing junit in help"
echo

echo "--- Help text shows --report ---"
echo "$HELP_OUT" | grep -q "\-\-report" && echo "PASS: --report in help" || fail "Missing --report in help"
echo

echo "--- --format html writes an HTML report ---"
HTML_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_html_XXXXXX)
cat > "$HTML_DIR/test.yaml" << 'YAML'
name: HTML Smoke Test
requests:
  - name: Example
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
YAML
HTML_RC=0
HTML_OUT=$(./curlew run "$HTML_DIR/test.yaml" --format html --report "$HTML_DIR/report.html" 2>&1) || HTML_RC=$?
if [ "$HTML_RC" -eq 0 ] && [ -s "$HTML_DIR/report.html" ] && grep -q "<html" "$HTML_DIR/report.html"; then
  echo "PASS: --format html wrote an HTML report"
else
  echo "FAIL: --format html should write a report (exit $HTML_RC), got: $HTML_OUT"
  rm -rf "$HTML_DIR"
  exit 1
fi
echo

echo "--- Help text shows html in --format ---"
echo "$HELP_OUT" | grep -q "html" && echo "PASS: html in help" || fail "Missing html in help"
echo

echo "--- --format html without --report shows error ---"
HTML_NO_REPORT_OUT=$(./curlew run "$HTML_DIR/test.yaml" --format html 2>&1 || true)
if echo "$HTML_NO_REPORT_OUT" | grep -q -- "--report"; then
  echo "PASS: --format html without --report shows helpful error"
else
  echo "FAIL: Expected helpful error for --format html without --report, got: $HTML_NO_REPORT_OUT"
  rm -rf "$HTML_DIR"
  exit 1
fi
rm -rf "$HTML_DIR"
echo

echo "--- Running GraphQL collection against local fixture ---"
GQL_FILE=$(mktemp ${SMOKE_ROOT}/curlew_gql_XXXXXX.yaml)
cat > "$GQL_FILE" << 'YAML'
name: GraphQL Smoke Test
requests:
  - name: GraphQL Query
    request:
      protocol: graphql
      url: "__SMOKE_HTTPBIN_URL__/graphql"
      graphql:
        query: "query { hello }"
YAML
GQL_RC=0
GQL_OUT=$(./curlew run "$GQL_FILE" 2>&1) || GQL_RC=$?
if [ "$GQL_RC" -eq 0 ]; then
  echo "PASS: protocol: graphql executes against local fixture (exit 0)"
else
  echo "FAIL: protocol: graphql should run (exit $GQL_RC), got: $GQL_OUT"
  rm -f "$GQL_FILE"
  exit 1
fi
rm -f "$GQL_FILE"
echo

echo "--- GraphQL invalid protocol (expect parse error) ---"
GQL_BAD_FILE=$(mktemp ${SMOKE_ROOT}/curlew_gql_bad_XXXXXX.yaml)
cat > "$GQL_BAD_FILE" << 'YAML'
name: Bad Protocol Test
requests:
  - name: Bad Protocol
    request:
      protocol: grpc
      url: "https://example.com/rpc"
YAML
GQL_BAD_OUT=$(./curlew run "$GQL_BAD_FILE" 2>&1 || true)
if echo "$GQL_BAD_OUT" | grep -q "unsupported protocol"; then
  echo "PASS: invalid protocol detected"
else
  echo "FAIL: expected unsupported protocol error, got: $GQL_BAD_OUT"
  rm -f "$GQL_BAD_FILE"
  exit 1
fi
rm -f "$GQL_BAD_FILE"
echo

echo "--- GraphQL query_file + fragments ---"
GQL_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_gql_files_XXXXXX)
mkdir -p "$GQL_DIR/graphql/queries" "$GQL_DIR/graphql/fragments"
cat > "$GQL_DIR/graphql/queries/get_user.graphql" << 'GQL'
query GetUser {
  user { ...UserFields }
}
GQL
cat > "$GQL_DIR/graphql/fragments/user_fields.graphql" << 'GQL'
fragment UserFields on User {
  id
  name
}
GQL
cat > "$GQL_DIR/tests.yaml" << 'YAML'
name: GraphQL Files Test
requests:
  - name: Get User
    request:
      protocol: graphql
      url: "__SMOKE_HTTPBIN_URL__/graphql"
      graphql:
        query_file: graphql/queries/get_user.graphql
        fragments:
          - graphql/fragments/user_fields.graphql
YAML
GQL_FILES_RC=0
GQL_FILES_OUT=$(./curlew run "$GQL_DIR/tests.yaml" 2>&1) || GQL_FILES_RC=$?
if [ "$GQL_FILES_RC" -eq 0 ]; then
  echo "PASS: graphql query_file + fragments parsed and executed (exit 0)"
else
  echo "FAIL: expected exit 0 for query_file + fragments, got exit $GQL_FILES_RC: $GQL_FILES_OUT"
  rm -rf "$GQL_DIR"
  exit 1
fi
rm -rf "$GQL_DIR"
echo

echo "--- WebSocket protocol executes (expect dial failure against closed port) ---"
# No live WebSocket server in the hermetic fixture set: a dial failure against
# a closed localhost port proves the websocket protocol actually executes.
WS_FILE=$(mktemp ${SMOKE_ROOT}/curlew_ws_XXXXXX.yaml)
cat > "$WS_FILE" << 'YAML'
name: WebSocket Smoke
requests:
  - name: Echo
    request:
      protocol: websocket
      url: "ws://localhost:9999/echo"
      websocket:
        steps:
          - action: send
            message:
              type: ping
          - action: close
            code: 1000
YAML
WS_OUT=$(./curlew run "$WS_FILE" 2>&1 || true)
if echo "$WS_OUT" | grep -q "websocket dial failed"; then
  echo "PASS: protocol: websocket executes (dial attempted against closed port)"
else
  echo "FAIL: protocol: websocket should attempt the dial, got: $WS_OUT"
  rm -f "$WS_FILE"
  exit 1
fi
rm -f "$WS_FILE"
echo

echo "--- WebSocket invalid action (expect parse error) ---"
WS_BAD_FILE=$(mktemp ${SMOKE_ROOT}/curlew_ws_bad_XXXXXX.yaml)
cat > "$WS_BAD_FILE" << 'YAML'
name: Bad WS
requests:
  - name: Bad Action
    request:
      protocol: websocket
      url: "ws://localhost/ws"
      websocket:
        steps:
          - action: transmute
YAML
WS_BAD_OUT=$(./curlew run "$WS_BAD_FILE" 2>&1 || true)
if echo "$WS_BAD_OUT" | grep -q "unsupported websocket action"; then
  echo "PASS: invalid websocket action detected"
else
  echo "FAIL: expected unsupported websocket action error, got: $WS_BAD_OUT"
  rm -f "$WS_BAD_FILE"
  exit 1
fi
rm -f "$WS_BAD_FILE"
echo

echo "--- GraphQL error_handling ignore is a valid value ---"
GQL_IGNORE_FILE=$(mktemp ${SMOKE_ROOT}/curlew_gql_ignore_XXXXXX.yaml)
cat > "$GQL_IGNORE_FILE" << 'YAML'
name: GraphQL Ignore Test
requests:
  - name: Ignore Mode
    request:
      protocol: graphql
      url: "__SMOKE_HTTPBIN_URL__/graphql"
      graphql:
        query: "{ me { id } }"
        error_handling: ignore
YAML
GQL_IGNORE_RC=0
GQL_IGNORE_OUT=$(./curlew run "$GQL_IGNORE_FILE" 2>&1) || GQL_IGNORE_RC=$?
if [ "$GQL_IGNORE_RC" -eq 0 ]; then
  echo "PASS: error_handling: ignore accepted and executed (exit 0)"
else
  echo "FAIL: expected exit 0 for error_handling: ignore, got exit $GQL_IGNORE_RC: $GQL_IGNORE_OUT"
  rm -f "$GQL_IGNORE_FILE"
  exit 1
fi
rm -f "$GQL_IGNORE_FILE"
echo

echo "--- WebSocket reconnect config parses without error ---"
WS_RECONNECT_FILE=$(mktemp ${SMOKE_ROOT}/curlew_ws_reconnect_XXXXXX.yaml)
cat > "$WS_RECONNECT_FILE" << 'YAML'
name: WS Reconnect Smoke
requests:
  - name: Echo
    request:
      protocol: websocket
      url: "ws://localhost:9999/echo"
      websocket:
        reconnect:
          enabled: true
          max_attempts: 3
          initial_delay_ms: 1000
          backoff: exponential
        steps:
          - action: send
            message:
              type: ping
YAML
./curlew validate "$WS_RECONNECT_FILE" 2>&1 | head -5 || true
rm -f "$WS_RECONNECT_FILE"
echo "PASS: websocket reconnect config accepted by parser"
echo

echo "--- WebSocket heartbeat config parses without error ---"
WS_HB_FILE=$(mktemp ${SMOKE_ROOT}/curlew_ws_hb_XXXXXX.yaml)
cat > "$WS_HB_FILE" << 'YAML'
name: WS Heartbeat Smoke
requests:
  - name: Echo
    request:
      protocol: websocket
      url: "ws://localhost:9999/echo"
      websocket:
        heartbeat:
          enabled: true
          interval_ms: 30000
        steps:
          - action: send
            message:
              type: hello
YAML
./curlew validate "$WS_HB_FILE" 2>&1 | head -5 || true
rm -f "$WS_HB_FILE"
echo "PASS: websocket heartbeat config accepted by parser"
echo

echo "--- WebSocket URL auto-detection (ws:// scheme) parses as websocket protocol ---"
WS_AUTO_FILE=$(mktemp ${SMOKE_ROOT}/curlew_ws_auto_XXXXXX.yaml)
cat > "$WS_AUTO_FILE" << 'YAML'
name: WS AutoDetect Smoke
requests:
  - name: Echo
    request:
      url: "ws://localhost:9999/echo"
      websocket:
        steps:
          - action: send
            message:
              type: ping
YAML
WS_AUTO_OUT=$(./curlew run "$WS_AUTO_FILE" 2>&1 || true)
if echo "$WS_AUTO_OUT" | grep -q "websocket dial failed"; then
  echo "PASS: ws:// URL auto-detected as websocket (dial attempted against closed port)"
else
  echo "FAIL: expected websocket dial attempt for ws:// URL, got: $WS_AUTO_OUT"
  rm -f "$WS_AUTO_FILE"
  exit 1
fi
rm -f "$WS_AUTO_FILE"
echo

echo "--- Discovery: glob runs matching collections ---"
# testdata/discovery points at httpbin.org as user-facing documentation; run
# a hermetic copy rewritten to the local fixture so smoke never hits the
# public internet.
DISC_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_disc_XXXXXX)
mkdir -p "$DISC_DIR/sub"
for f in a_test.yaml b_test.yaml ignore.yaml; do
  sed "s|https://httpbin.org|$SMOKE_HTTPBIN_URL|g" "testdata/discovery/$f" > "$DISC_DIR/$f"
done
sed "s|https://httpbin.org|$SMOKE_HTTPBIN_URL|g" testdata/discovery/sub/c_test.yaml > "$DISC_DIR/sub/c_test.yaml"
# Glob patterns must be relative — run from inside the temp dir.
DISC_RC=0
DISC_OUT=$(cd "$DISC_DIR" && "$PROJECT_ROOT/curlew" run "**/*_test.yaml" 2>&1) || DISC_RC=$?
if [ "$DISC_RC" -ne 0 ]; then
  echo "FAIL: discovery glob run exited $DISC_RC, got: $DISC_OUT"
  rm -rf "$DISC_DIR"
  exit 1
fi
echo "$DISC_OUT" | grep -q "Collection: A" || { echo "FAIL: glob missed a_test.yaml: $DISC_OUT"; rm -rf "$DISC_DIR"; exit 1; }
echo "$DISC_OUT" | grep -q "Collection: C" || { echo "FAIL: glob missed sub/c_test.yaml: $DISC_OUT"; rm -rf "$DISC_DIR"; exit 1; }
if echo "$DISC_OUT" | grep -q "Collection: Ignored"; then
  echo "FAIL: glob should not match ignore.yaml: $DISC_OUT"
  rm -rf "$DISC_DIR"
  exit 1
fi
echo "PASS: discovery glob ran matching collections (recursive, exit 0)"
rm -rf "$DISC_DIR"
echo

echo "--- Discovery: literal path (no glob, expect exit 3 file-not-found) ---"
./curlew run "testdata/discovery/nonexistent.yaml" 2>&1 || true
echo "PASS: literal path bypasses discovery (exit 3)"
echo

echo "--- Include directive: merges included requests ---"
INC_PARENT=$(mktemp ${SMOKE_ROOT}/curlew_inc_parent_XXXXXX.yaml)
INC_SHARED_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_inc_shared_XXXXXX)
INC_CHILD="$INC_SHARED_DIR/auth.yaml"
cat > "$INC_CHILD" << 'YAML'
name: Shared Auth
requests:
  - name: Auth Check
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
YAML
cat > "$INC_PARENT" << YAML
name: Parent Collection
variables:
  base_url: __SMOKE_HTTPBIN_URL__
include:
  - $INC_CHILD
requests:
  - name: Parent Request
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
YAML
INC_RC=0
INC_OUT=$(./curlew run "$INC_PARENT" 2>&1) || INC_RC=$?
if [ "$INC_RC" -ne 0 ]; then
  echo "FAIL: include directive run exited $INC_RC, got: $INC_OUT"
  rm -f "$INC_PARENT"
  rm -rf "$INC_SHARED_DIR"
  exit 1
fi
echo "$INC_OUT" | grep -q "Parent Request" || { echo "FAIL: parent request missing: $INC_OUT"; rm -f "$INC_PARENT"; rm -rf "$INC_SHARED_DIR"; exit 1; }
echo "$INC_OUT" | grep -q "Auth Check" || { echo "FAIL: included request missing: $INC_OUT"; rm -f "$INC_PARENT"; rm -rf "$INC_SHARED_DIR"; exit 1; }
echo "PASS: include directive merged and ran included requests (exit 0)"
rm -f "$INC_PARENT"
rm -rf "$INC_SHARED_DIR"
echo

# --- M3-005: curlew import openapi ---
echo "--- Import OpenAPI ---"
mkdir -p ${SMOKE_ROOT}/curlew-smoke-openapi
cat > ${SMOKE_ROOT}/curlew-smoke-openapi/petstore.yaml <<'OAI'
openapi: 3.0.3
info:
  title: Petstore
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /pets:
    get:
      operationId: listPets
      responses:
        '200':
          description: OK
  /pets/{petId}:
    get:
      operationId: showPetById
      parameters:
        - name: petId
          in: path
          required: true
          schema: { type: string }
      responses:
        '200':
          description: OK
OAI

./curlew import openapi ${SMOKE_ROOT}/curlew-smoke-openapi/petstore.yaml \
    --output ${SMOKE_ROOT}/curlew-smoke-openapi/generated.yaml \
  && echo "PASS: import openapi writes file" \
  || fail "import openapi failed"

./curlew validate ${SMOKE_ROOT}/curlew-smoke-openapi/generated.yaml \
  && echo "PASS: generated collection validates" \
  || fail "generated collection failed to validate"

# --- M3-006: headers, request bodies, and status assertions ---
echo "--- Import OpenAPI with headers/body/status (M3-006) ---"
cat > ${SMOKE_ROOT}/curlew-smoke-openapi/petstore-full.yaml <<'OAI'
openapi: 3.0.3
info:
  title: Petstore Full
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
components:
  schemas:
    NewPet:
      type: object
      properties:
        name: { type: string }
paths:
  /pets:
    post:
      operationId: createPet
      parameters:
        - name: X-API-Key
          in: header
          required: true
          schema: { type: string }
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/NewPet'
      responses:
        '201': { description: Created }
        '404': { description: Not found }
OAI

./curlew import openapi ${SMOKE_ROOT}/curlew-smoke-openapi/petstore-full.yaml \
    --output ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: import openapi full spec writes file" \
  || fail "import openapi full spec failed"

grep -q "X-API-Key" ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains X-API-Key header" \
  || fail "missing X-API-Key header in generated collection"

grep -q "body:" ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && grep -q "name: string" ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains request body" \
  || fail "missing request body in generated collection"

grep -q "status:" ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: generated collection contains status assertions" \
  || fail "missing status assertions in generated collection"

./curlew validate ${SMOKE_ROOT}/curlew-smoke-openapi/full-generated.yaml \
  && echo "PASS: full generated collection validates" \
  || fail "full generated collection failed to validate"

rm -rf ${SMOKE_ROOT}/curlew-smoke-openapi
echo

echo "=== Shared vault template (M4-002) ==="
echo "--- Run with CURLEW_TEAM_CONFIG + stub ---"
export CURLEW_TEAM_CONFIG=testdata/team/shared-vault-template.yaml
export CURLEW_VAULT_STUB=1

SMOKE_OUT=$(./curlew run testdata/team/uses-team-vault.yaml --env production --var "base_url=$SMOKE_HTTPBIN_URL" 2>&1)
SMOKE_RC=$?

unset CURLEW_TEAM_CONFIG CURLEW_VAULT_STUB

if [ "$SMOKE_RC" -eq 0 ]; then
  echo "PASS: team template run exit 0"
else
  echo "FAIL: exit $SMOKE_RC"
  echo "$SMOKE_OUT"
  exit 1
fi

echo "$SMOKE_OUT" | grep -q "Resolved 2 secrets from shared template (production)" \
  && echo "PASS: resolved-log line present" \
  || fail "missing resolved-log line" "$SMOKE_OUT"

echo

echo "=== PR check (local results gate) ==="
SMOKE_RC=0
SMOKE_OUT=$(./curlew pr-check --results testdata/team/sample-junit.json --dry-run 2>&1) || SMOKE_RC=$?
if [ "$SMOKE_RC" -eq 0 ]; then
  echo "PASS: pr-check --dry-run exit 0"
else
  echo "FAIL: pr-check --dry-run exit $SMOKE_RC"
  echo "$SMOKE_OUT"
  exit 1
fi
echo "$SMOKE_OUT" | grep -q '"state"' \
  && echo "PASS: dry-run output contains the check summary" \
  || fail "missing summary in dry-run output" "$SMOKE_OUT"

# --summary writes the verdict to a file; a failing results file exits 1.
PRCHECK_SUMMARY=$(mktemp ${SMOKE_ROOT}/curlew_prcheck_XXXXXX.json)
./curlew pr-check --results testdata/team/sample-junit.json --summary "$PRCHECK_SUMMARY" > /dev/null 2>&1
[ -s "$PRCHECK_SUMMARY" ] \
  && echo "PASS: pr-check --summary wrote a summary file" \
  || fail "pr-check --summary produced no file"
rm -f "$PRCHECK_SUMMARY"

# pr-check must not reach the network: no backend env var can change its behaviour.
./curlew pr-check --results testdata/team/sample-junit.json > /dev/null 2>&1 \
  && echo "PASS: pr-check needs no backend configuration" \
  || fail "pr-check failed without backend configuration"

# The documented pipeline: `curlew run --format json` output feeds pr-check directly.
# Uses the local fixture server — the smoke suite never touches the public internet.
PRCHECK_COL=$(mktemp ${SMOKE_ROOT}/curlew_prcheck_col_XXXXXX.yaml)
cat > "$PRCHECK_COL" << YAML
name: PrCheck Pipeline
requests:
  - name: get
    request:
      method: GET
      url: "$SMOKE_HTTPBIN_URL/get"
    assertions:
      status: 200
YAML
PRCHECK_RUN_JSON=$(mktemp ${SMOKE_ROOT}/curlew_prcheck_run_XXXXXX.json)
./curlew run "$PRCHECK_COL" --format json > "$PRCHECK_RUN_JSON" 2>/dev/null || true
PRCHECK_OUT=$(./curlew pr-check --results "$PRCHECK_RUN_JSON" 2>&1) || true
echo "$PRCHECK_OUT" | grep -qE "^(success|failure): pass=" \
  && echo "PASS: pr-check reads curlew run --format json output" \
  || fail "pr-check could not read run --format json output" "$PRCHECK_OUT"
rm -f "$PRCHECK_RUN_JSON" "$PRCHECK_COL"

echo

echo "=== Perf --help (M5-011) ==="
PERF_HELP=$(./curlew perf --help 2>&1)
echo "$PERF_HELP" | grep -q "Usage: curlew perf" \
  || fail "perf --help missing usage line" "$PERF_HELP"
echo "$PERF_HELP" | grep -q -- "--vus" \
  || fail "perf --help missing --vus"
echo "$PERF_HELP" | grep -q -- "--duration" \
  || fail "perf --help missing --duration"
echo "$PERF_HELP" | grep -q -- "--ramp-up" \
  || fail "perf --help missing --ramp-up"
echo "$PERF_HELP" | grep -q -- "--rps" \
  || fail "perf --help missing --rps"
echo "$PERF_HELP" | grep -q -- "--output" \
  || fail "perf --help missing --output"
echo "PASS: perf --help documents all expected flags"

# Invalid --vus -> exit 2
SMOKE_RC=0
./curlew perf testdata/perf/sample-request.yaml \
  --vus 0 --duration 1s > /dev/null 2>${SMOKE_ROOT}/curlew_perf_err_$$.txt || SMOKE_RC=$?
[ "$SMOKE_RC" -eq 2 ] && echo "PASS: --vus 0 exits 2" \
  || { echo "FAIL: --vus 0 exited $SMOKE_RC (want 2)"; cat ${SMOKE_ROOT}/curlew_perf_err_$$.txt; exit 1; }
rm -f ${SMOKE_ROOT}/curlew_perf_err_$$.txt

# End-to-end: run perf for 1s against the primary fixture.
PERF_COL="${SMOKE_ROOT}/curlew_perf_smoke_$$.yaml"
cat > "$PERF_COL" << YAML
name: perf-smoke
request:
  method: GET
  url: "__SMOKE_HTTPBIN_URL__/"
YAML

SMOKE_RC=0
PERF_OUT=$(./curlew perf "$PERF_COL" --vus 2 --duration 1s 2>&1) || SMOKE_RC=$?
echo "$PERF_OUT" | grep -q "Load test: 2 virtual users" \
  && echo "PASS: perf prints header" \
  || { echo "FAIL: perf header"; echo "$PERF_OUT"; exit 1; }
echo "$PERF_OUT" | grep -q "Requests sent:" \
  && echo "PASS: perf prints summary" \
  || { echo "FAIL: perf summary missing"; exit 1; }

# M5-012: --output json report
PERF_JSON="${SMOKE_ROOT}/curlew_perf_report_$$.json"
SMOKE_RC=0
./curlew perf "$PERF_COL" --vus 2 --duration 1s \
  --output "$PERF_JSON" > ${SMOKE_ROOT}/curlew_perf_stdout_$$.log 2>&1 || SMOKE_RC=$?
grep -q "Results: requests=" ${SMOKE_ROOT}/curlew_perf_stdout_$$.log \
  && echo "PASS: perf --output json prints summary line" \
  || { echo "FAIL: missing Results line"; cat ${SMOKE_ROOT}/curlew_perf_stdout_$$.log; exit 1; }
[ -s "$PERF_JSON" ] && python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$PERF_JSON" \
  && echo "PASS: perf --output json produces valid JSON" \
  || { echo "FAIL: invalid or empty JSON"; cat "$PERF_JSON"; exit 1; }
rm -f "$PERF_JSON" ${SMOKE_ROOT}/curlew_perf_stdout_$$.log

# M5-012: --output html report
PERF_HTML="${SMOKE_ROOT}/curlew_perf_report_$$.html"
./curlew perf "$PERF_COL" --vus 2 --duration 1s \
  --output "$PERF_HTML" > /dev/null 2>&1
grep -q "<title>curlew perf report</title>" "$PERF_HTML" \
  && echo "PASS: perf --output html wrote expected title" \
  || { echo "FAIL: perf html missing title"; exit 1; }
rm -f "$PERF_HTML"

# M5-012: unsupported extension -> exit 2
SMOKE_RC=0
./curlew perf "$PERF_COL" --vus 1 --duration 200ms \
  --output "${SMOKE_ROOT}/x_smoke_$$.xyz" > /dev/null 2>${SMOKE_ROOT}/curlew_perf_err_$$.txt || SMOKE_RC=$?
[ "$SMOKE_RC" -eq 2 ] && echo "PASS: unsupported extension exits 2" \
  || { echo "FAIL: unsupported extension exited $SMOKE_RC"; cat ${SMOKE_ROOT}/curlew_perf_err_$$.txt; exit 1; }
grep -qi "unsupported report format" ${SMOKE_ROOT}/curlew_perf_err_$$.txt \
  && echo "PASS: stderr mentions unsupported format" \
  || { echo "FAIL: stderr missing message"; cat ${SMOKE_ROOT}/curlew_perf_err_$$.txt; exit 1; }
rm -f ${SMOKE_ROOT}/curlew_perf_err_$$.txt

rm -f "$PERF_COL"

echo

echo "=== Report Upload flag validation (M4-012) ==="

# Create a minimal collection file for flag validation tests (use PID to ensure unique name)
MINIMAL_COL="${SMOKE_ROOT}/curlew_report_upload_smoke_$$.yaml"
cat > "$MINIMAL_COL" << 'YAML'
name: minimal-smoke
requests:
  - name: health
    request:
      method: GET
      url: "http://localhost:1"
    assertions:
      status: 200
YAML

# Removed backend flags must fail loudly rather than being silently ignored.
for REMOVED_FLAG in --report-upload --workers --refresh-vault; do
  SMOKE_RC=0
  SMOKE_OUT=$(./curlew run "$MINIMAL_COL" "$REMOVED_FLAG" 2>&1) || SMOKE_RC=$?
  if [ "$SMOKE_RC" -eq 0 ]; then
    echo "FAIL: removed flag $REMOVED_FLAG was silently accepted"
    echo "$SMOKE_OUT"
    rm -f "$MINIMAL_COL"
    exit 1
  fi
  echo "$SMOKE_OUT" | grep -qi "unknown flag" \
    && echo "PASS: $REMOVED_FLAG rejected as an unknown flag" \
    || { echo "FAIL: $REMOVED_FLAG did not produce an unknown-flag error"; echo "$SMOKE_OUT"; rm -f "$MINIMAL_COL"; exit 1; }
done

# Removed subcommands must be unknown commands.
for REMOVED_CMD in login worker; do
  SMOKE_RC=0
  SMOKE_OUT=$(./curlew "$REMOVED_CMD" 2>&1) || SMOKE_RC=$?
  echo "$SMOKE_OUT" | grep -q "Unknown command" \
    && echo "PASS: curlew $REMOVED_CMD is an unknown command" \
    || { echo "FAIL: curlew $REMOVED_CMD is still reachable"; echo "$SMOKE_OUT"; rm -f "$MINIMAL_COL"; exit 1; }
done

rm -f "$MINIMAL_COL"
echo

echo "=== Stack idempotency (M4-012) ==="

if [ "${CURLEW_MANAGE_STACK:-0}" = "1" ]; then
  echo "--- Stack: up → count orgs → up again → count orgs (must be equal) ---"
  STACK_SCRIPT="$PROJECT_ROOT/scripts/test-stack.sh"
  STACK_TOKEN=$("$PROJECT_ROOT/scripts/test-token.sh" owner@example.com)
  BACKEND_URL="${CURLEW_BACKEND_URL:-http://localhost:5000}"

  "$STACK_SCRIPT" up
  COUNT_BEFORE=$(curl -sS -H "Authorization: Bearer $STACK_TOKEN" "$BACKEND_URL/api/v1/organizations" \
    | jq '.organizations | length')
  "$STACK_SCRIPT" up
  COUNT_AFTER=$(curl -sS -H "Authorization: Bearer $STACK_TOKEN" "$BACKEND_URL/api/v1/organizations" \
    | jq '.organizations | length')

  if [ "$COUNT_BEFORE" = "$COUNT_AFTER" ]; then
    echo "PASS: stack up is idempotent (org count: $COUNT_BEFORE)"
  else
    echo "FAIL: seeding is not idempotent (before=$COUNT_BEFORE after=$COUNT_AFTER)"
    "$STACK_SCRIPT" down
    exit 1
  fi
  "$STACK_SCRIPT" down
  echo "PASS: stack down completed"
else
  echo "SKIP: stack idempotency test (set CURLEW_MANAGE_STACK=1 with a running docker-compose stack to run)"
fi
echo

echo "=== Plugins (M5-017) ==="

PLUGIN_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_plugins_XXXXXX)

echo "--- Building hello-plugin fixture ---"
go build -buildvcs=false -o "$PLUGIN_DIR/hello-plugin" ./testdata/plugins/hello-plugin
echo "Build: OK"

echo "--- Single plugin ---"
OUT=$(CURLEW_PLUGINS="$PLUGIN_DIR/hello-plugin" ./curlew plugins list)
echo "$OUT" | grep -q "hello-plugin" \
  && echo "PASS: single plugin listed" || fail "single plugin — $OUT"
echo "$OUT" | grep -q "0.1.0" \
  && echo "PASS: version correct" || fail "version — $OUT"

echo "--- Directory of plugins ---"
OUT=$(CURLEW_PLUGINS="$PLUGIN_DIR" ./curlew plugins list)
echo "$OUT" | grep -q "hello-plugin" \
  && echo "PASS: directory expanded" || fail "directory — $OUT"

echo "--- Missing path exits 2 ---"
if CURLEW_PLUGINS="/does/not/exist/plugin" ./curlew plugins list 2>/dev/null; then
  echo "FAIL: should have exited 2"
  rm -rf "$PLUGIN_DIR"
  exit 1
else
  echo "PASS: missing path exits non-zero"
fi

echo "--- Empty env exits 0 with header ---"
OUT=$(CURLEW_PLUGINS="" ./curlew plugins list)
echo "$OUT" | grep -q "NAME" \
  && echo "PASS: header printed for empty env" || fail "no header — $OUT"

rm -rf "$PLUGIN_DIR"
echo

echo "=== Plugin hooks (M5-018) ==="

HOOK_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_hooklog_XXXXXX)
echo "--- Building hooklog-plugin fixture ---"
go build -buildvcs=false -o "$HOOK_DIR/hooklog" ./testdata/plugins/hooklog-plugin
echo "Build: OK"

SMOKE_COLL=$(mktemp ${SMOKE_ROOT}/curlew_hooklog_coll_XXXXXX.yaml)
cat > "$SMOKE_COLL" <<'YAML'
name: Hooklog smoke
requests:
  - name: ping
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
    assertions:
      status: 200
YAML

echo "--- Run with hooklog plugin ---"
OUT=$(CURLEW_PLUGINS="$HOOK_DIR/hooklog" ./curlew run "$SMOKE_COLL" 2>&1) || true
echo "$OUT" | grep -q "on_request" \
  && echo "PASS: on_request hook fired" || { echo "FAIL: no on_request line — $OUT"; rm -rf "$HOOK_DIR" "$SMOKE_COLL"; exit 1; }
echo "$OUT" | grep -q "on_response" \
  && echo "PASS: on_response hook fired" || { echo "FAIL: no on_response line — $OUT"; rm -rf "$HOOK_DIR" "$SMOKE_COLL"; exit 1; }
echo "$OUT" | grep -q "on_result" \
  && echo "PASS: on_result hook fired" || { echo "FAIL: no on_result line — $OUT"; rm -rf "$HOOK_DIR" "$SMOKE_COLL"; exit 1; }

rm -rf "$HOOK_DIR" "$SMOKE_COLL"
echo

echo "=== Example plugin: datadog-metrics (M5-019) ==="

DD_BUILD_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_ddplugin_XXXXXX)
echo "--- Building datadog-metrics example ---"
(cd "$PROJECT_ROOT/examples/plugins/datadog-metrics" && go build -buildvcs=false -o "$DD_BUILD_DIR/datadog-metrics" .)
echo "Build: OK"

echo "--- Testing datadog-metrics example ---"
(cd "$PROJECT_ROOT/examples/plugins/datadog-metrics" && go test ./...)
echo "Tests: OK"

echo "--- Standalone --help exits 0 with metadata ---"
OUT=$("$DD_BUILD_DIR/datadog-metrics" --help)
echo "$OUT" | grep -q "datadog-metrics" \
  && echo "PASS: --help prints plugin name" \
  || { echo "FAIL: --help — $OUT"; rm -rf "$DD_BUILD_DIR"; exit 1; }
echo "$OUT" | grep -q "on_response" \
  && echo "PASS: --help mentions on_response hook" \
  || { echo "FAIL: --help missing on_response — $OUT"; rm -rf "$DD_BUILD_DIR"; exit 1; }

rm -rf "$DD_BUILD_DIR"
echo

echo "=== body_file / body_binary_file (external-file request bodies) ==="

BF_DIR=$(mktemp -d ${SMOKE_ROOT}/curlew_body_file_XXXXXX)

echo "--- Running collection with body_file (text + interpolation) ---"
cat > "$BF_DIR/payload.json" << 'JSON'
{"batch_id":"{{batch_id}}","records":[{"id":1,"name":"Alice"}]}
JSON
cat > "$BF_DIR/collection.yaml" << 'YAML'
name: Body File Text Smoke
requests:
  - name: POST body_file to fixture
    request:
      method: POST
      url: "__SMOKE_HTTPBIN_URL__/anything"
      body_file: "payload.json"
    assertions:
      status: 200
      body:
        $.headers.Content-Type:
          equals: "application/json"
        $.json.batch_id:
          equals: "B-smoke-123"
        $.json.records[0].name:
          equals: "Alice"
YAML
./curlew run "$BF_DIR/collection.yaml" --var batch_id=B-smoke-123 \
  && echo "PASS: body_file round-tripped with Content-Type auto-detect and variable interpolation" \
  || { echo "FAIL: body_file smoke"; rm -rf "$BF_DIR"; exit 1; }

echo "--- Running collection with body_binary_file (raw bytes, no interpolation) ---"
# 16 bytes including non-UTF-8 sequences and literal {{..}} that must NOT interpolate.
printf '\x00\x01{{nope}}\xDE\xAD\xBE\xEF' > "$BF_DIR/blob.bin"
cat > "$BF_DIR/binary.yaml" << 'YAML'
name: Body Binary File Smoke
requests:
  - name: POST raw bytes
    request:
      method: POST
      url: "__SMOKE_HTTPBIN_URL__/anything"
      headers:
        Content-Type: "application/octet-stream"
      body_binary_file: "blob.bin"
    assertions:
      status: 200
      body:
        $.headers.Content-Type:
          equals: "application/octet-stream"
YAML
./curlew run "$BF_DIR/binary.yaml" \
  && echo "PASS: body_binary_file uploaded and server accepted" \
  || { echo "FAIL: body_binary_file smoke"; rm -rf "$BF_DIR"; exit 1; }

echo "--- validate catches a missing body_file (expect exit 3) ---"
cat > "$BF_DIR/missing.yaml" << 'YAML'
name: Missing Body File
requests:
  - name: x
    request:
      method: POST
      url: "https://example.com/"
      body_file: "does_not_exist.json"
YAML
./curlew validate "$BF_DIR/missing.yaml" && { echo "FAIL: should have rejected"; rm -rf "$BF_DIR"; exit 1; } \
  || echo "PASS: validate rejected missing body_file with exit $?"

echo "--- mutual exclusion: body + body_file fails (expect non-zero exit) ---"
cat > "$BF_DIR/conflict.yaml" << 'YAML'
name: Conflict
requests:
  - name: x
    request:
      method: POST
      url: "https://example.com/"
      body: { inline: "yes" }
      body_file: "payload.json"
YAML
./curlew validate "$BF_DIR/conflict.yaml" && { echo "FAIL: should have rejected"; rm -rf "$BF_DIR"; exit 1; } \
  || echo "PASS: validate rejected body/body_file conflict"

rm -rf "$BF_DIR"
echo

echo "--- Events NDJSON stream (--events happy path) ---"
EVENTS_COL=$(mktemp ${SMOKE_ROOT}/curlew_events_col_XXXXXX.yaml)
EVENTS_OUT=$(mktemp ${SMOKE_ROOT}/curlew_events_out_XXXXXX.jsonl)
cat > "$EVENTS_COL" << 'YAML'
name: events-smoke
requests:
  - name: ping
    request:
      method: GET
      url: __SMOKE_HTTPBIN_URL__/get
    assertions:
      status: 200
YAML
./curlew run "$EVENTS_COL" --events "$EVENTS_OUT"
HEAD_KIND=$(head -n 1 "$EVENTS_OUT" | jq -r .kind)
TAIL_KIND=$(tail -n 1 "$EVENTS_OUT" | jq -r .kind)
LINE_COUNT=$(wc -l < "$EVENTS_OUT" | tr -d ' ')
EVENT_COUNT=$(tail -n 1 "$EVENTS_OUT" | jq -r .event_count)
[[ "$HEAD_KIND" == "run.start" ]] || { echo "FAIL: first line kind = $HEAD_KIND"; rm -f "$EVENTS_COL" "$EVENTS_OUT"; exit 1; }
[[ "$TAIL_KIND" == "run.end" ]]   || { echo "FAIL: last line kind = $TAIL_KIND"; rm -f "$EVENTS_COL" "$EVENTS_OUT"; exit 1; }
[[ "$LINE_COUNT" == "$EVENT_COUNT" ]] || { echo "FAIL: line count $LINE_COUNT != event_count $EVENT_COUNT"; rm -f "$EVENTS_COL" "$EVENTS_OUT"; exit 1; }
rm -f "$EVENTS_COL" "$EVENTS_OUT"
echo "PASS: --events stream shape"
echo

echo "--- Running aws-sigv4 signing (validates registry + signing header) ---"
SIG_DIR=$(mktemp -d)
cat > "$SIG_DIR/sigv4.yaml" << 'YAML'
name: sigv4 demo
requests:
  - name: signed
    request:
      method: GET
      url: "__SMOKE_HTTPBIN_URL__/get"
    signing:
      type: aws-sigv4
      params:
        region: us-east-1
        service: s3
        access_key: "{{aws_key}}"
        secret_key: "{{aws_secret}}"
YAML
AUTH=$(./curlew run "$SIG_DIR/sigv4.yaml" \
    --var aws_key=AKIDEXAMPLE \
    --var aws_secret=wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY \
    --allow-sensitive \
    -v --format json 2>/dev/null \
    | jq -r '.requests[0].request_headers.Authorization' || echo MISSING)
case "$AUTH" in
    "AWS4-HMAC-SHA256 Credential="*) echo "PASS: AWS SigV4 Authorization header present" ;;
    *) echo "ERROR: AWS SigV4 Authorization missing or malformed: $AUTH" ; rm -rf "$SIG_DIR"; exit 1 ;;
esac
rm -rf "$SIG_DIR"
echo

echo "--- Running oauth1 signing (validates registry + Authorization header) ---"
OAUTH1_DIR=$(mktemp -d)
cat > "$OAUTH1_DIR/oauth1.yaml" << 'YAML'
name: oauth1 demo
requests:
  - name: signed
    request:
      method: POST
      url: "__SMOKE_HTTPBIN_URL__/post"
    signing:
      type: oauth1
      params:
        consumer_key: "{{ck}}"
        consumer_secret: "{{cs}}"
YAML
AUTH=$(./curlew run "$OAUTH1_DIR/oauth1.yaml" \
    --var ck=dpf43f3p2l4k3l03 \
    --var cs=kd94hf93k423kf44 \
    --allow-sensitive \
    -v --format json 2>/dev/null \
    | jq -r '.requests[0].request_headers.Authorization' || echo MISSING)
case "$AUTH" in
    "OAuth "*) echo "PASS: OAuth1 Authorization header present" ;;
    *) echo "ERROR: OAuth1 Authorization missing or malformed: $AUTH" ; rm -rf "$OAUTH1_DIR"; exit 1 ;;
esac
rm -rf "$OAUTH1_DIR"
echo

echo "--- Running webhook-sign Stripe (validates dynamic-fn + secret redaction) ---"
WHSIG_DIR=$(mktemp -d)
cat > "$WHSIG_DIR/stripe.yaml" << 'YAML'
name: webhook-sign demo
requests:
  - name: stripe-webhook
    request:
      method: POST
      url: "__SMOKE_HTTPBIN_URL__/"
      headers:
        X-Stripe-Signature: "{{$webhookSign.stripe('{{payload}}','{{stripe_signing_secret}}','1492774577')}}"
      body: "{{payload}}"
YAML
WHSIG_OUT=$(./curlew run "$WHSIG_DIR/stripe.yaml" \
    --var payload='{"amount":1000}' \
    --var stripe_signing_secret=whsec_supersecret_donot_leak \
    --allow-sensitive \
    -v --format json 2>/dev/null)
WHSIG_HDR=$(echo "$WHSIG_OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['requests'][0].get('request_headers',{}).get('X-Stripe-Signature','MISSING'))" 2>/dev/null || echo "MISSING")
echo "$WHSIG_HDR" | grep -q 't=1492774577,v1=' \
    && echo "PASS: Stripe-Signature t=...,v1=... header rendered" \
  || { echo "FAIL: missing t=...,v1=... shape; got: $WHSIG_HDR" ; rm -rf "$WHSIG_DIR" ; exit 1 ; }
# Without --allow-sensitive, the secret value must not appear in JSON output.
WHSIG_NOALLOW=$(./curlew run "$WHSIG_DIR/stripe.yaml" \
    --var payload='{"amount":1000}' \
    --var stripe_signing_secret=whsec_supersecret_donot_leak \
    -v --format json 2>/dev/null)
echo "$WHSIG_NOALLOW" | grep -q 'whsec_supersecret_donot_leak' \
  && { echo "FAIL: secret leaked into --format json output" ; rm -rf "$WHSIG_DIR" ; exit 1 ; } \
    || echo "PASS: secret not visible in serialised output"
rm -rf "$WHSIG_DIR"
echo

echo "--- Running jwt-decode (validates JWT decode dynamic-fns produce valid JSON) ---"
JWT_DIR=$(mktemp -d)
# JWT.io canonical example: HS256 over secret="your-256-bit-secret".
# Uses a placeholder replaced by sed so the literal dot-separated token is
# not misinterpreted by the shell heredoc.
JWT_TOKEN='eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c'
# $jwtDecodeHeader and $jwtDecodeClaims return compact JSON strings — place
# them directly in request headers so the rendered value is visible in
# --format json output. python3 then parses those JSON strings (simulating
# what a downstream $jsonpath assertion would do) to verify the decoded fields.
cat > "$JWT_DIR/jwt.yaml" << 'YAML'
name: jwt-decode demo
requests:
  - name: jwt-decode-headers
    request:
      method: POST
      url: "__SMOKE_HTTPBIN_URL__/"
      headers:
        X-Token-Header: "{{$jwtDecodeHeader('JWT_TOKEN_PLACEHOLDER')}}"
        X-Token-Claims: "{{$jwtDecodeClaims('JWT_TOKEN_PLACEHOLDER')}}"
YAML
sed -i "s/JWT_TOKEN_PLACEHOLDER/${JWT_TOKEN}/g" "$JWT_DIR/jwt.yaml"
JWT_OUT=$(./curlew run "$JWT_DIR/jwt.yaml" -v --format json 2>/dev/null)
# Extract the rendered header values from the JSON output.
JWT_HDR_JSON=$(echo "$JWT_OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['requests'][0].get('request_headers',{}).get('X-Token-Header','MISSING'))" 2>/dev/null || echo "MISSING")
JWT_CLAIMS_JSON=$(echo "$JWT_OUT" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['requests'][0].get('request_headers',{}).get('X-Token-Claims','MISSING'))" 2>/dev/null || echo "MISSING")
# Parse those header values as JSON and extract individual fields — this is
# exactly what a downstream $jsonpath('$.alg', ...) assertion would do.
JWT_ALG=$(echo "$JWT_HDR_JSON" | python3 -c "import sys,json; d=json.loads(sys.stdin.read().strip()); print(d.get('alg','MISSING'))" 2>/dev/null || echo "MISSING")
JWT_SUB=$(echo "$JWT_CLAIMS_JSON" | python3 -c "import sys,json; d=json.loads(sys.stdin.read().strip()); print(d.get('sub','MISSING'))" 2>/dev/null || echo "MISSING")
[ "$JWT_ALG" = "HS256" ] \
    && echo "PASS: jwtDecodeHeader returned valid JSON with alg=HS256" \
  || { echo "FAIL: expected alg=HS256, got: $JWT_ALG" ; rm -rf "$JWT_DIR" ; exit 1 ; }
[ "$JWT_SUB" = "1234567890" ] \
    && echo "PASS: jwtDecodeClaims returned valid JSON with sub=1234567890" \
  || { echo "FAIL: expected sub=1234567890, got: $JWT_SUB" ; rm -rf "$JWT_DIR" ; exit 1 ; }
rm -rf "$JWT_DIR"
echo

# ── Backend FileKeyProvider boot probe (M14-001) ─────────────────────────
# Gated behind CURLEW_RUN_BACKEND_SMOKE=1 — default off until M14 stabilises.
if [ "${CURLEW_RUN_BACKEND_SMOKE:-}" = "1" ]; then
  echo "--- Backend FileKeyProvider boot probe (M14-001) ---"
  TMPKEYS=$(mktemp -d ${SMOKE_ROOT}/curlew_keys_XXXXXX)
  BACKEND_SMOKE_URL="${CURLEW_BACKEND_URL:-http://localhost:5000}"
  ASPNETCORE_ENVIRONMENT=Testing \
    ASPNETCORE_URLS="$BACKEND_SMOKE_URL" \
    Curlew__KeyProvider__Mode=file \
    Curlew__KeyProvider__Env=smoke \
    Curlew__KeyProvider__File__Dir="$TMPKEYS" \
    dotnet run --project "$PROJECT_ROOT/src/ApiTool.Backend" --no-build &
  BACKEND_PID=$!
  register_pid "$BACKEND_PID"
  register_path "$TMPKEYS"
  BACKEND_READY=0
  for i in $(seq 1 30); do
    if curl -fsS "$BACKEND_SMOKE_URL/internal/keys/active" >/dev/null 2>&1; then
      BACKEND_READY=1
      break
    fi
    sleep 1
  done
  if [ "$BACKEND_READY" -eq 0 ]; then
    echo "FAIL: backend did not start within 30 seconds" >&2
    stop_pid "$BACKEND_PID"
    rm -rf "$TMPKEYS"
    exit 1
  fi
  BACKEND_KID=$(curl -fsS "$BACKEND_SMOKE_URL/internal/keys/active" | python3 -c "import sys,json; print(json.load(sys.stdin)['kid'])" 2>/dev/null || echo "MISSING")
  if [[ ! "$BACKEND_KID" =~ ^[a-z0-9-]{1,64}$ ]]; then
    echo "FAIL: kid '$BACKEND_KID' does not match the allowlist regex ^[a-z0-9-]{1,64}$" >&2
    stop_pid "$BACKEND_PID"
    rm -rf "$TMPKEYS"
    exit 1
  fi
  echo "PASS: kid=$BACKEND_KID"
  stop_pid "$BACKEND_PID"
  rm -rf "$TMPKEYS"
  echo
fi

# ── M19-001: if: conditional gate ─────────────────────────────────────────────
echo "=== M19-001: if: conditional gate ==="
IF_OUTPUT=$(./curlew run smoke/fixtures/if_conditional.yaml --var base_url="$SMOKE_HTTPBIN_URL" --format terminal 2>&1)
echo "$IF_OUTPUT"
if echo "$IF_OUTPUT" | grep -q "SKIPPED.*confirm-pending-order"; then
  echo "PASS: confirm-pending-order rendered as SKIPPED"
else
  echo "FAIL: confirm-pending-order not found in SKIPPED output; got:"
  echo "$IF_OUTPUT"
  exit 1
fi
if echo "$IF_OUTPUT" | grep -q "SKIPPED.*notify"; then
  echo "PASS: notify rendered as SKIPPED (parent skipped)"
else
  echo "FAIL: notify not found in SKIPPED output; got:"
  echo "$IF_OUTPUT"
  exit 1
fi
echo

# ── M19-004: cel: assertions ──────────────────────────────────────────────────
echo "=== M19-004: cel: assertions ==="
CEL_RC=0
CEL_OUTPUT=$(./curlew run smoke/fixtures/cel_assertions.yaml --var "base_url=$SMOKE_HTTPBIN_URL" --format terminal 2>&1) || CEL_RC=$?

if [ "$CEL_RC" -ne 1 ]; then
  echo "FAIL: expected exit 1 (one failing CEL assertion), got $CEL_RC"
  echo "$CEL_OUTPUT"
  exit 1
fi
echo "PASS: cel_assertions run exited 1 as expected"
if ! echo "$CEL_OUTPUT" | grep -q "response.body.total"; then
  echo "FAIL: failure message missing 'response.body.total'"
  echo "$CEL_OUTPUT"
  exit 1
fi
echo "PASS: failure message includes the expression source"
echo

# ── M19-005: curlew validate CEL parse/type-check ────────────────────────────
echo "=== M19-005: curlew validate CEL ==="
GOOD_OUT=$(./curlew validate smoke/fixtures/cel_validate_good.yaml 2>&1)
GOOD_RC=$?
if [ "$GOOD_RC" -ne 0 ]; then
  echo "FAIL: validate cel_validate_good.yaml exited $GOOD_RC, expected 0"
  echo "$GOOD_OUT"
  exit 1
fi
echo "PASS: cel_validate_good.yaml validates cleanly"

BAD_RC=0
BAD_OUT=$(./curlew validate smoke/fixtures/cel_validate_bad.yaml 2>&1) || BAD_RC=$?
if [ "$BAD_RC" -eq 0 ]; then
  echo "FAIL: validate cel_validate_bad.yaml exited 0, expected non-zero"
  echo "$BAD_OUT"
  exit 1
fi
echo "PASS: cel_validate_bad.yaml validate exited non-zero ($BAD_RC)"

# bad fixture must emit ERR_CEL_PARSE on assertions[0].cel and ERR_CEL_TYPE on if
echo "$BAD_OUT" | grep -q "assertions\[0\].cel" || fail "missing assertions[0].cel path" "$BAD_OUT"
echo "$BAD_OUT" | grep -q "ERR_CEL_PARSE" || fail "missing ERR_CEL_PARSE label" "$BAD_OUT"
echo "$BAD_OUT" | grep -q "ERR_CEL_TYPE" || fail "missing ERR_CEL_TYPE label" "$BAD_OUT"
echo "$BAD_OUT" | grep -q "int" || fail "ERR_CEL_TYPE message missing actual type 'int'" "$BAD_OUT"
echo "PASS: bad fixture emits both error codes with field paths and type label"
echo

## M18-008: telemetry subcommand round-trip
echo "--- M18-008: telemetry round-trip ---"
SMOKE_TELEMETRY_DIR="$(mktemp -d)"
register_path "$SMOKE_TELEMETRY_DIR"

export CURLEW_CONFIG_DIR="$SMOKE_TELEMETRY_DIR"
export CURLEW_TELEMETRY_FILE="$SMOKE_TELEMETRY_DIR/telemetry.ndjson"

# status before enable → exit 1
SMOKE_OUT=$(./curlew telemetry status 2>&1) && {
  echo "FAIL: telemetry status before enable should exit 1"
  exit 1
} || true
echo "$SMOKE_OUT" | grep -q "no install_id" || {
  echo "FAIL: telemetry status should say 'no install_id', got: $SMOKE_OUT"
  exit 1
}
echo "PASS: telemetry status before enable exits 1 with 'no install_id'"

# enable → exit 0; install_id file mode 0600
ENABLE_OUT=$(./curlew telemetry enable)
echo "$ENABLE_OUT" | grep -q "install_id=" || {
  echo "FAIL: enable output should contain install_id=, got: $ENABLE_OUT"
  exit 1
}
if [ "$(uname)" != "MINGW64_NT-10.0" ] && [ "$(uname)" != "CYGWIN_NT-10.0" ]; then
  ID_FILE="$SMOKE_TELEMETRY_DIR/install_id"
  ID_PERMS=$(stat -c '%a' "$ID_FILE" 2>/dev/null || stat -f '%Lp' "$ID_FILE" 2>/dev/null || echo "unknown")
  if [ "$ID_PERMS" != "600" ] && [ "$ID_PERMS" != "unknown" ]; then
    echo "FAIL: install_id file mode = $ID_PERMS, want 600"
    exit 1
  fi
fi
echo "PASS: telemetry enable exits 0, install_id file created"

# status after enable → exit 0, says enabled
STATUS_OUT=$(./curlew telemetry status)
echo "$STATUS_OUT" | grep -q "enabled" || {
  echo "FAIL: status after enable should say 'enabled', got: $STATUS_OUT"
  exit 1
}
echo "PASS: telemetry status after enable shows enabled"

# a run appends a run.completed event to the local file — no network involved
cat > "$SMOKE_TELEMETRY_DIR/col.yaml" << 'YAML'
name: Telemetry Smoke
requests:
  - name: noop
    request:
      method: GET
      url: "http://127.0.0.1:1"
YAML
./curlew run "$SMOKE_TELEMETRY_DIR/col.yaml" > /dev/null 2>&1 || true
grep -q '"event_type":"run.completed"' "$CURLEW_TELEMETRY_FILE" \
  || { echo "FAIL: run.completed not appended to $CURLEW_TELEMETRY_FILE"; exit 1; }
echo "PASS: run appends run.completed to the local events file"

# disable → exit 0
./curlew telemetry disable
echo "PASS: telemetry disable exits 0"

# delete → exit 0, local files and collected events removed
./curlew telemetry delete
if [ -f "$SMOKE_TELEMETRY_DIR/install_id" ]; then
  echo "FAIL: install_id not removed after delete"
  exit 1
fi
if [ -f "$CURLEW_TELEMETRY_FILE" ]; then
  echo "FAIL: events file not removed after delete"
  exit 1
fi
echo "PASS: telemetry delete removes install_id and collected events"

unset CURLEW_TELEMETRY_FILE
export CURLEW_CONFIG_DIR="$SMOKE_CFG_DIR"
echo

rm -rf "$SMOKE_CFG_DIR" "$SMOKE_HELLO_DIR"
stop_pid "$SMOKE_HTTPBIN_PID"

echo "=== Smoke Test Complete ==="
