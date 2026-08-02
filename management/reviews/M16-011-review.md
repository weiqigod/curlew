# Code Review: M16-011

**Task:** curlew worker --schedule-pull mode with file: collection ref and pending-uploads queue
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-011-schedule-pull-worker

## Verdict: FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Medium | Correctness | `internal/worker/schedule/runner.go` | 193 | If `CollectionExecutor.Execute` returns `(nil, nil)`, `buildResultRequest` dereferences the nil `*ExecutionOutcome` pointer and panics. The `runnerExecutor` implementation never does this, but the interface contract does not document the requirement that callers must never return `(nil, nil)`, making the panic reachable via any test double or future implementation. | Add a nil guard before `buildResultRequest`: if `outcome` is nil after the execute block, synthesize a fail `ExecutionOutcome`. Also document in the `CollectionExecutor` interface: "Execute must return a non-nil `*ExecutionOutcome` when err is nil." |
| 2 | Low | Error Handling | `internal/worker/schedule/runner.go` | 233, 239 | `Queue.Remove` errors are silently discarded with `_ = r.Queue.Remove(...)` in `drainQueue`. `Queue.Remove` is idempotent for missing files but returns a real error for I/O failures (e.g., permission denied). A silent drop means an operator can never diagnose why queued results persist on disk despite a supposedly successful result post. | Replace with `if err := r.Queue.Remove(p.RunID); err != nil { _, _ = fmt.Fprintf(r.Stderr, "worker: remove queued result %s: %v\n", p.RunID, err) }`. |

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Previous Review Findings — All Resolved

All four findings from iteration 2 (Low severity) are confirmed fixed:

1. **Doc comment on `postWithRetry`** — Updated to "Returns `ErrPostExhausted` if retries are exhausted." (runner.go:277)
2. **`filepath.Join` for pending-uploads dir** — Now uses `filepath.Join(cfgDir, "pending-uploads")`. (worker.go:122)
3. **Clock inconsistency in sleep cap** — Now uses `deadline.Sub(r.Now())`. (runner.go:302)
4. **Missing runner-level 401-fatal test** — `TestRunner_Unauthorized_DuringPoll_IsFatal` added. (runner_test.go:438)

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | FAIL | Finding 2: `Queue.Remove` errors silently discarded in two drain paths. All other errors wrapped with `%w`, sentinel errors cover all caller-inspectable cases, no other swallowed errors. |
| Input Validation | PASS | `ResolveCollection` handles empty path, unknown scheme, `git:` refs with clear errors. `parseWorkerArgs` validates all flag values with descriptive errors. `Queue.Enqueue` guards dir creation. `Runner.defaults()` guards all zero fields. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `HTTPClient` interface naming correct. Package names are lowercase single-word. |
| Code Organization | PASS | Package boundary clean. `internal/worker/schedule/` is self-contained. `backend.Client` extended additively with `GetJSONOptional`. No circular dependencies. `defer` used correctly for cleanup (response bodies, timers, heartbeat goroutine). |
| Correctness | FAIL | Finding 1: nil `*ExecutionOutcome` from `Execute` causes panic in `buildResultRequest` — no nil guard and interface contract undocumented. All other correctness properties verified: happy path, drain, network-failure enqueue, `git:` rejection, claim-reap abandonment, 401-fatal, `--once` integration. |
| Test Quality | PASS | All 9 task behaviors have test coverage. Behavior 8 (`--perf-pull` concurrency) is explicitly deferred per plan Open Decision 2 and documented in help text and package doc. Coverage: 84.9% on `internal/worker/schedule`, 81.5% on `cmd/curlew`, 83.9% on `internal/backend` — all exceed the 80% threshold. |

## Test Coverage

- `internal/worker/schedule`: **84.9%** (exceeds ≥80% threshold)
- `internal/backend`: **83.9%** (exceeds threshold)
- `cmd/curlew`: **81.5%** (exceeds threshold)

## Summary

This is the third iteration. All four Low-severity findings from iteration 2 are cleanly resolved. Two new findings remain: a Medium-severity nil panic in `buildResultRequest` when `CollectionExecutor.Execute` returns `(nil, nil)` (production executor never does this, but the interface lacks a documented contract and the runner has no defensive nil guard); and a Low-severity silent discard of `Queue.Remove` I/O errors in `drainQueue`. Per the all-or-nothing rule both must be fixed before the task can be marked done.
