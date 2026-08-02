# Improvement Report: M1-020

**Task:** JSON output format (--format json)
**Date:** 2026-03-15
**Review:** management/reviews/M1-020-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 2 | Medium | `status_code omitempty` caused field to be absent for error requests, violating Behavior 3 | Removed `omitempty` from `StatusCode` tag; `status_code: 0` now always emitted; added unit test in `json_test.go`; enhanced `TestRunCmdDirect_JSONNetworkError` to assert field presence | ✓ tests pass |
| 1 | Medium | `--format <unknown>` (e.g. `--format xml`) silently fell through to terminal mode | Added validation block in `runCmd` after successful parse; unknown format values are rejected with exit 1 and structured error on stderr; added `TestRunCmdDirect_UnknownFormat` | ✓ tests pass |
| 3 | Low | `runCmd` function coverage was 68.6%; terminal exit 4 and exit 5 paths untested in-process | Added `TestRunCmdDirect_TerminalNetworkError` (exit 4), `TestRunCmdDirect_TerminalVarError` (exit 5), and `TestRunCmdDirect_JSONMissingEnv` (exit 3); `runCmd` coverage 77.4% | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`cmd/apitest`) | 87.9% |
| Coverage (`internal/output`) | 100.0% |
| Coverage (total) | 93.0% |
| `runCmd` function coverage | 77.4% (was 68.6%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 879bae6 | fix(output): always emit status_code field, even for error requests | #2 |
| cfebc07 | fix(cli): reject unknown --format values with error | #1 |
| 7539046 | test(cli): add in-process runCmd tests for coverage gaps | #3 |

## Summary

3/3 findings resolved. 0 deferred.
