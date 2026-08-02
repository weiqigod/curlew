# Improvement Report: M3-001

**Task:** Global rate_limit_rps throttle across all requests
**Date:** 2026-04-10
**Review:** management/reviews/M3-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Missing `TestRun_GlobalRateLimit_StacksWithDataDriven` (behavior #7: "both throttles stack") | Added test in `internal/runner/runner_test.go`: collection `rate_limit_rps: 5` + data-driven `rate_limit_rps: 20` over 5 CSV rows; asserts elapsed >= 640ms (global limiter dominates) | ✓ tests pass |
| 2 | Medium | Solo tier not tested for `rate_limit_global` gate (behavior #6 says "Free or Solo tier") | Added `TestRun_GlobalRateLimit_SoloTierGated` in `internal/runner/runner_test.go`; asserts `TierSolo` returns `*auth.GateError` for `rate_limit_global` | ✓ tests pass |
| 3 | Medium | `TestDefaultRegistry` did not assert `RequiredTier` for `rate_limit_global` | Added `TestDefaultRegistry_RateLimitGlobal` in `internal/auth/registry_test.go`, mirroring the existing `TestDefaultRegistry_ParallelExecution` pattern; asserts `TierProfessional` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/runner`) | 86.2% |
| Coverage (`internal/auth`) | 88.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 54c7ece | test(runner,auth): add missing tests for M3-001 review findings | #1, #2, #3 |

## Summary
3/3 findings resolved. 0 deferred.
