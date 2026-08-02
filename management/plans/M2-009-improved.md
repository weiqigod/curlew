# Improvement Report: M2-009

**Task:** Auth profile token caching and refresh
**Date:** 2026-03-29
**Review:** management/reviews/M2-009-review.md

## Resolved Findings (Round 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `%s` instead of `%w` for underlying error in `FileCacheStore.Load` (lines 62, 68) | Changed to `fmt.Errorf("%w: %w", ErrCacheCorrupted, err)` for full error chain | ✓ tests pass |
| 2 | High | `%s` instead of `%w` in `Deobfuscate` (line 151) | Changed to `fmt.Errorf("%w: base64 decode: %w", ErrCacheCorrupted, err)` | ✓ tests pass |
| 3 | Medium | `Invalidate` returns raw OS error without context | Changed to `fmt.Errorf("remove cache entry %q: %w", profileName, err)` with nil guard | ✓ tests pass |
| 4 | Medium | Missing test for "401 refresh failure falls through to 401" | Added `authExecFails` field to test struct; authExec fails on refresh call (call > 1); new case asserts status=401, httpCalls=1 | ✓ tests pass |
| 5 | Low | Misleading test name `"collection mismatch treated as cache miss"` | Renamed to `"load preserves collection field for caller comparison"` | ✓ tests pass |

## Resolved Findings (Round 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 6 | Low | `FileCacheStore.Save` at 47.4% function coverage — `MkdirAll` failure path untested | Added test `"save fails when cache dir is a file"`: creates a regular file at the cache dir path to trigger `MkdirAll` failure; `Save` function coverage → 52.6% | ✓ tests pass |

## Out of Scope (Deferred)

| # | Severity | Finding | Rationale |
|---|----------|---------|-----------|
| — | Low | Remaining `Save` error paths: `CreateTemp`, `Write` (with cleanup), `Close` (with cleanup), `Rename` (with cleanup) | Require OS-level fault injection. Explicitly noted as deferrable per review recommendation. Package coverage remains above 80% threshold at 87.7%. |

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/auth`) | 87.7% |
| Coverage (`internal/runner`) | 91.8% |
| Coverage (total) | 91.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 205b722 | fix(auth): wrap underlying errors with %w for full error chain | #1, #2, #3 |
| e21caab | test(auth): rename misleading cache test to reflect actual behavior | #5 |
| dfc4735 | test(runner): add missing refresh-failure test case and rename cache test | #4 |
| 2d814a0 | test(auth): add Save MkdirAll failure test case | #6 |

## Notes

Finding #3 (`Invalidate` wrapping) required an additional nil guard: the original code returned `err` which could be `nil` on success. The replacement `fmt.Errorf(...)` always returns non-nil, so `if err == nil || errors.Is(err, os.ErrNotExist)` was used to early-return `nil` before wrapping.

Finding #4 required distinguishing initial auth execution (which must succeed) from the refresh execution (which the test exercises failing). The `authExec` lambda checks `authExecCalls > 1` before failing, so the initial `ExecuteProfiles` call in `Run()` succeeds and the refresh call fails as intended.

## Summary

6/6 findings resolved. 0 deferred (OS-injection paths noted but not required by review).
