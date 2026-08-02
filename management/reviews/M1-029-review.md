# Code Review: M1-029

**Task:** Guard rail (1,000-request limit)
**Reviewer:** AI
**Date:** 2026-03-19
**Branch:** feature/M1-029-guard-rail-request-limit

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Guard rail uses counter mechanism (no new error paths). Existing error wrapping with `%w` preserved. No swallowed errors. |
| Input Validation | PASS | Counter check `*counter >= maxRequests` correctly prevents execution beyond limit. Boundary condition (exactly 1000) correctly handled via `counter >= MaxRequests && total > counter`. |
| Naming | PASS | `MaxRequests`, `LimitExceeded`, `RequestsExecuted`, `GuardRailJSON` — clear, no stuttering. Doc comments on all exported symbols. |
| Code Organization | PASS | Guard rail logic scoped to `runner` package (counter + limit check). Output formatting in `output` package. CLI wiring in `cmd/curlew`. Clean separation. |
| Correctness | PASS | Counter shared across all phases (setup/main/teardown). Skipped requests don't increment counter. `LimitExceeded` only set when requests were actually blocked (not when exactly N requests fit the limit). Exit code 2 takes precedence over assertion/network failures. Race detector passes. |
| Test Quality | PASS | Table-driven tests with `t.Run()`. Boundary cases (999, 1000, 1001). Multi-phase counting. Small limit for easy testing. Integration tests across all 3 output formats (terminal, JSON, TAP). `MaxRequests` save/restore via `t.Cleanup()`. |

## Test Coverage
- `internal/runner`: 91.7%
- `internal/output`: 92.2%
- `cmd/curlew`: 85.3%
- `GuardRail()` method: 100%
- `Run()` function: 88.9%
- `executePhase()`: 92.3%
- Missing coverage: early-return paths in `Run()` where `RequestsExecuted` is not set (acceptable — field is only used when `LimitExceeded=true`, which requires full completion)

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| 999 requests run normally | `TestRun_GuardRail/"999 requests all succeed"` |
| >1000 stops with exit code 2 | `TestRun_GuardRail/"1001 requests stops at 1000"`, `TestRunCmd_GuardRail` (all 5 subtests) |
| Message suggests splitting | `TestPrinter_GuardRail/"includes hint"`, `TestRunCmd_GuardRail/"terminal hint message"` |
| All phases count toward limit | `TestRun_GuardRail/"limit hit across setup main teardown"` |
| Summary includes completed requests | `TestRun_GuardRail` (all cases verify `RequestsExecuted`, `Passed`, `Skipped`), `TestRunCmd_GuardRail` (verifies output) |

## Summary

Clean, well-structured implementation. The guard rail counter is shared across all execution phases via a pointer, correctly incremented only for actually-executed requests. The `LimitExceeded` condition includes a `total > counter` check that correctly distinguishes between "all requests fit" and "requests were blocked." All three output formats (terminal, JSON, TAP) handle the guard rail with exit code 2. Tests are comprehensive with good boundary coverage. No issues found.
