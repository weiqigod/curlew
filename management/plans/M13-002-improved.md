# Improvement Report: M13-002

**Task:** $faker personal data — 10 functions including auto-sensitive $faker.ssn
**Date:** 2026-04-29
**Review:** management/reviews/M13-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `smoke/run.sh` lines 873-875: assertion `grep -q '"ssn".*"\[REDACTED\]"'` against `--format json` output can never pass — `JSONRequest` has no `request_body` field, so no `ssn` field ever appears in the JSON output | Removed the broken positive-redaction assertion from the `--format json` smoke block. Retained the negative check (SSN must not appear at all in JSON output). The positive-redaction contract (`[REDACTED]` in the body) is proved by the `-vv` terminal block and the `--format markdown` block, both of which already passed. | ✓ `./smoke/run.sh` exits 0; all three FAKER blocks pass |
| 2 | Critical | `mktemp /tmp/apitest_faker_ssnXXXXXX.yaml` creates a literal filename on macOS (suffix after X-placeholder block is not supported), causing `mkstemp failed: File exists` on the second CI run when a prior failed run left the file behind | Changed to `mktemp -t apitest_faker_ssn` (macOS-portable; produces a randomly suffixed file in `$TMPDIR`). Added `trap 'rm -f "$FAKER_FILE"; ...' EXIT` so the temp file is always cleaned up even when the script exits early via `exit 1`. Removed redundant inline `kill` calls from each assertion branch (trap handles it). | ✓ `./smoke/run.sh` exits 0; confirmed cleanup on all exit paths |
| 3 | High | `docs/MANUAL.md` lines 1088-1099 stated SSN appears as `[REDACTED]` in "the captured request-body field" of `--format json`. This is factually incorrect — `JSONRequest` has no `request_body` field | Corrected the documentation to state that the SSN is redacted in the `-vv` terminal body dump, the markdown report body section, the events stream, and `--log` output. Explicitly notes that `--format json` has no `request_body` field so the SSN simply does not appear there. | ✓ Documentation now matches implementation |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `./smoke/run.sh` | PASS |
| Coverage (`internal/variable`) | 96.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1417f70 | fix(smoke): resolve M13-002 review findings #1 #2 #3 | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.

All three findings were in the integration layer (`smoke/run.sh` and `docs/MANUAL.md`). The Go source implementation was already correct (unit tests pass at 96.9% coverage, all 13 behaviors covered). The root cause was that a previous improve iteration added a smoke assertion for a JSON field (`request_body`) that does not exist in the `JSONRequest` output schema, along with a macOS-incompatible `mktemp` template that caused the temp file to be left behind on early exit. Both issues are now resolved.
