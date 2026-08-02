# Improvement Report: M5-010

**Task:** go-cli: --workers flag for distributed run execution
**Date:** 2026-04-20
**Review:** management/reviews/M5-010-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `ErrCoordinatorURLMissing` and `ErrTokenMissing` exported but never returned by `Run()` — misleading API | Removed both sentinels; validation remains in `cmd/curlew/main.go` where it belongs | ✓ tests pass |
| 2 | High | `TestRun_ShardReassignment` missing `summary.Total/Passed/Failed` assertions for behaviour 4 | Added assertions: `Total=6`, `Passed=6`, `Failed=0` covering reassigned-shard outcome inclusion | ✓ tests pass |
| 3 | Medium | Missing `--org` exits with code `1` instead of `2` (inconsistent with other pre-condition failures) | Changed exit code to `2` in `cmd/curlew/main.go` | ✓ tests pass |
| 4 | Medium | No test for `--workers N` with missing `--org` | Added `TestRunCmd_WorkersMissingOrg` verifying exit 2 and `--org` in error message | ✓ tests pass |
| 5 | Medium | `TestRun_AggregatedFailurePropagates` missing `summary.Passed == 3` assertion | Added `if summary.Passed != 3 { t.Errorf(...) }` | ✓ tests pass |
| 6 | Medium | `Summary.Duration` always zero — `SummaryWithDuration` prints `(0ms)` | Added `start := nowFn()` before job creation; `aggregate` now accepts `elapsed time.Duration` and sets `summary.Duration = elapsed` | ✓ tests pass |
| 7 | Low | `fmt.Errorf("%s", item.Message)` does not wrap an error — should use `errors.New` | Replaced with `errors.New(item.Message)` | ✓ tests pass |
| 8 | Low | Dead variable `var callCount int` / `_ = callCount` left over from development | Removed both lines | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/runner/distributed`) | 85.8% |
| Coverage (`cmd/curlew`) | 81.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a4da1f6 | fix(distributed): resolve all M5-010 review findings | #1, #2, #3, #4, #5, #6, #7, #8 |

## Summary

8/8 findings resolved. 0 deferred.
