# Improvement Report: M5-009

**Task:** go-cli: worker agent protocol
**Date:** 2026-04-19
**Review:** management/reviews/M5-009-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `defer hcancel()` inside `for` loop accumulates N deferred cancels across shard iterations (memory concern for long-running workers) | Removed `defer hcancel()` on line 107; retained the existing explicit `hcancel()` call after `executeShard` returns for correct per-shard cancellation | ✓ tests pass |
| 2 | Medium | Behavior 5 requires "logs a warning on each retry" — `doWithRetry` retried silently with no per-attempt log | Added `fmt.Fprintf(c.logWriter(), "warning: retrying %s (attempt %d/%d): %v\n", ...)` at the start of each retry iteration; added `LogWriter io.Writer` field to `Client` (nil = `os.Stderr`) so tests can capture or suppress the output | ✓ tests pass |
| 3 | Medium | No test exercises multi-shard iteration — the primary worker loop (claim → complete → claim → complete → 204) was completely untested | Added `TestRun/two_shards_then_204` sub-test: 2 shards (3 requests + 2 requests), asserts `ShardsCompleted==2`, `TotalPass==5`, both "Completed" lines in stdout, and 2 `SubmitResult` calls | ✓ tests pass |
| 4 | Low | Dead code: redundant `errors.Is(err, ErrUnauthorized)` inner branch in `Run` — both branches returned `(summary, err)` identically | Collapsed to a single `return summary, err` without the inner `errors.Is` check | ✓ tests pass |
| 5 | Low | `_ []int` (expected status codes) parameter in `doWithRetry` silently discarded — misleading API | Removed the parameter entirely and updated all three call sites (`Claim`, `SubmitResult`, `Heartbeat`) | ✓ tests pass |
| 6 | Low | `containsStr` in `internal/worker/worker_test.go` and `containsSubstr` in `cmd/curlew/worker_test.go` are manual reimplementations of `strings.Contains` | Replaced both helpers with `strings.Contains` (added `"strings"` import to both files, removed helper functions) | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/worker`) | 91.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 4fe99a2 | fix(worker): resolve all review findings for M5-009 | #1, #2, #3, #4, #5, #6 |

## Summary

6/6 findings resolved. 0 deferred.
