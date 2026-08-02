#!/usr/bin/env bash
# check-signing-keys_test.sh — Unit tests for check-signing-keys.sh (M18-009, v4-13).
#
# Tests the three exit-code paths of check-signing-keys.sh by intercepting the
# psql call with a PATH-prepended stub binary.  No real database is required.
#
# Usage:
#   ./scripts/check-signing-keys_test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TARGET="$SCRIPT_DIR/check-signing-keys.sh"

pass=0
fail=0

assert_exit() {
  local expected_exit="$1"
  local test_name="$2"
  shift 2
  local actual_exit=0
  "$@" >/dev/null 2>&1 || actual_exit=$?
  if [ "$actual_exit" -eq "$expected_exit" ]; then
    echo "PASS: $test_name"
    pass=$((pass+1))
  else
    echo "FAIL: $test_name (expected exit $expected_exit, got $actual_exit)"
    fail=$((fail+1))
  fi
}

# Create a temp dir with a psql stub script, run fn, remove dir.
make_psql_stub() {
  local count="$1"
  local stubdir
  stubdir="$(mktemp -d)"
  printf '#!/usr/bin/env bash\necho "%s"\n' "$count" > "$stubdir/psql"
  chmod +x "$stubdir/psql"
  echo "$stubdir"
}

make_psql_error_stub() {
  local stubdir
  stubdir="$(mktemp -d)"
  printf '#!/usr/bin/env bash\necho "psql: could not connect" >&2\nexit 1\n' > "$stubdir/psql"
  chmod +x "$stubdir/psql"
  echo "$stubdir"
}

# ── Test 1: file key mode → always skip (exit 0) ──────────────────────────

assert_exit 0 "file_mode_skips_regardless_of_profile" \
  env APITEST_SIGNING_KEY_MODE=file APITEST_BUILD_PROFILE=saas "$TARGET"

# ── Test 2: non-saas profile → skip (exit 0) ──────────────────────────────

assert_exit 0 "non_saas_profile_skips" \
  env APITEST_BUILD_PROFILE=self-hosted "$TARGET"

assert_exit 0 "unset_profile_skips" \
  env -u APITEST_BUILD_PROFILE -u APITEST_SIGNING_KEY_MODE "$TARGET"

# ── Test 3: saas profile, COUNT=0 → pass (exit 0) ─────────────────────────

stubdir="$(make_psql_stub 0)"
assert_exit 0 "saas_profile_all_keys_enrolled_passes" \
  env PATH="$stubdir:$PATH" APITEST_BUILD_PROFILE=saas DATABASE_URL="postgres://test/test" "$TARGET"
rm -rf "$stubdir"

# ── Test 4: saas profile, COUNT=2 → fail (exit 1) ─────────────────────────

stubdir="$(make_psql_stub 2)"
assert_exit 1 "saas_profile_null_kms_key_id_fails" \
  env PATH="$stubdir:$PATH" APITEST_BUILD_PROFILE=saas DATABASE_URL="postgres://test/test" "$TARGET"
rm -rf "$stubdir"

# ── Test 5: saas profile, psql fails → propagate failure (exit 1) ─────────

stubdir="$(make_psql_error_stub)"
assert_exit 1 "psql_connection_error_propagates" \
  env PATH="$stubdir:$PATH" APITEST_BUILD_PROFILE=saas DATABASE_URL="postgres://test/test" "$TARGET"
rm -rf "$stubdir"

# ── Summary ───────────────────────────────────────────────────────────────

echo
echo "Results: $pass passed, $fail failed"
[ $fail -eq 0 ] && exit 0 || exit 1
