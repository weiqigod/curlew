# Improvement Report: M16-011

**Task:** apitest worker --schedule-pull mode with file: collection ref and pending-uploads queue
**Date:** 2026-05-11
**Review:** management/reviews/M16-011-review.md

## Iteration 3 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `buildResultRequest` dereferences nil `*ExecutionOutcome` and panics when `CollectionExecutor.Execute` returns `(nil, nil)` — a contract violation reachable through any test double or future implementation. Interface contract also undocumented. | Added a nil guard immediately before `buildResultRequest` in `RunOnce` that synthesises a single-item `error` `ExecutionOutcome` when `outcome` is nil after the execute block. Added interface-level doc comment stating "Execute must return a non-nil `*ExecutionOutcome` when err is nil." Added `TestRunner_NilOutcome_IsGuarded` to exercise the new path. | ✓ tests pass |
| 2 | Low | `Queue.Remove` errors silently discarded with `_ = r.Queue.Remove(...)` in both drain paths of `drainQueue`; I/O failures (e.g. permission denied) were invisible to operators. | Replaced both `_ = r.Queue.Remove(p.RunID)` calls with `if err := r.Queue.Remove(p.RunID); err != nil { fmt.Fprintf(r.Stderr, "worker: remove queued result %s: %v\n", ...) }`, matching the project's error-wrapping convention. | ✓ tests pass |

## Iteration 2 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Stale doc comment on `postWithRetry` described `backend.ErrNetworkFailure` as the return value on exhaustion, but the code already returns `ErrPostExhausted` after the first improve pass | Updated doc comment to read "Returns `ErrPostExhausted` if retries are exhausted and the backend remains unreachable." | ✓ tests pass |
| 2 | Low | Queue directory built with string concatenation `cfgDir + "/pending-uploads"` in `cmd/apitest/worker.go` — non-idiomatic; inconsistent with rest of codebase | Replaced with `filepath.Join(cfgDir, "pending-uploads")`; added `path/filepath` import | ✓ tests pass |
| 3 | Low | `time.Until(deadline)` used in `postWithRetry` sleep cap instead of `deadline.Sub(r.Now())`; inconsistency with injectable clock makes frozen-clock tests unable to control the cap | Replaced with `deadline.Sub(r.Now())` with an explanatory comment | ✓ tests pass |
| 4 | Low | Missing runner-level test for "unauthorized from poll is fatal" — plan's RED phase required this sub-case but it was absent | Added `TestRunner_Unauthorized_DuringPoll_IsFatal` asserting that a `*backend.ProblemDetails{Status: 401}` from `PollNextRun` propagates as a non-nil `RunOnce` error; uses `errors.As` to verify the concrete type | ✓ tests pass |

## Iteration 1 — Resolved Findings (carried forward)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `ErrPostExhausted` declared but never returned — `postWithRetry` returned `backend.ErrNetworkFailure` on exhaustion | `postWithRetry` now returns `ErrPostExhausted` when retries are exhausted | ✓ |
| 2 | Medium | `ScheduleHTTPClient` stutters as `schedule.ScheduleHTTPClient` | Renamed to `HTTPClient` | ✓ |
| 3 | Medium | Behavior 4 (claim reaped mid-run) had zero test coverage | Added `TestRunner_ClaimReaped_AbandonWithoutPosting` with `slowExecutor` | ✓ |
| 4 | Low | Doc comment on `Runner` mentioned `Sleep` field that did not exist | Added `Sleep` injectable field; updated doc comment | ✓ |
| 5 | Low | `time.After` goroutine leaks in `Run` and `postWithRetry` | Replaced with injectable `r.Sleep(ctx, d)` | ✓ |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate (Iteration 3)

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/worker/schedule` | 84.3% (up from 83.9% at review) |
| Coverage `internal/backend` | 83.9% |
| Coverage `cmd/apitest` | 81.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 89702a0d | fix(worker/schedule): resolve all review findings for M16-011 | iter1 #1–#5 |
| 4456cf08 | fix(worker/schedule): address all review findings from M16-011 second pass | iter2 #1, #2, #3, #4 |
| 159f1133 | fix(worker/schedule): nil outcome guard and Queue.Remove error logging | iter3 #1, #2 |

## Summary

Iteration 3: 2/2 findings resolved. 0 deferred.
All 11 findings across all three review iterations are resolved.
