# Code Review: M3-001

**Task:** Global rate_limit_rps throttle across all requests
**Reviewer:** AI
**Date:** 2026-04-10
**Branch:** feature/M3-001-global-rate-limit-rps
**Iteration:** 2 (after `/improve`)

## Verdict: PASS

## Findings

No findings. All three issues from the iteration-1 review have been resolved.

## Resolved from Iteration 1

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | High | Missing `TestRun_GlobalRateLimit_StacksWithDataDriven` (behavior #7) | Added in `internal/runner/runner_test.go` at line 7999; verifies global 5 rps dominates data-driven 20 rps, asserts elapsed >= 640ms |
| 2 | Medium | Solo tier not tested for `rate_limit_global` gate (behavior #6 partial) | Added `TestRun_GlobalRateLimit_SoloTierGated` at line 7979; asserts `TierSolo` returns `*auth.GateError` for `rate_limit_global` |
| 3 | Medium | `TestDefaultRegistry` did not assert `RequiredTier` for `rate_limit_global` | Added `TestDefaultRegistry_RateLimitGlobal` at line 73 in `registry_test.go`; mirrors existing `TestDefaultRegistry_ParallelExecution` pattern |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; `ErrInvalidFieldValue` sentinel used correctly; no panics for expected failures; `ctx.Err()` propagated from limiter through all code paths. |
| Input Validation | PASS | Negative `rate_limit_rps` rejected with `apierrors.Structured` including line number; zero treated as unlimited (nil `*Limiter`); nil `*Limiter` receiver is a safe no-op on `Wait`. |
| Naming | PASS | No stuttering; all exported types/functions/methods have doc comments (`Limiter`, `New`, `Wait`, `FeatureDefinition`); package `ratelimit` is lowercase single-word. |
| Code Organization | PASS | New `internal/ratelimit` package has single responsibility; `internal/` boundaries respected; `datadriven` correctly delegates to shared package; no circular imports; `globalLimiter` is unexported in `VarSources` to prevent caller mutation. |
| Correctness | PASS | Token bucket mutex scoped correctly; `defer t.Stop()` used for timer cleanup; context cancellation handled in sequential and WebSocket code paths; parallel workers share one `exec` closure which holds the limiter. Race detector passes across all packages. |
| Test Quality | PASS | All 8 behaviors from the task YAML have at least one test; table-driven tests used throughout; `t.Run()` with descriptive names; error paths covered; timing tolerance (80%) applied consistently. |

## Test Coverage

| Package | Coverage | Threshold |
|---------|----------|-----------|
| `internal/ratelimit` | 100% | >= 80% ✓ |
| `internal/runner` | 86.2% | >= 80% ✓ |
| `internal/auth` | 88.6% | >= 80% ✓ |
| `internal/parser` | 88.8% | >= 80% ✓ |
| `internal/datadriven` | 90.4% | >= 80% ✓ |

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| `rate_limit_rps: 10` parsed at Professional tier → `Collection.RateLimitRPS = 10` | `TestParseFile_rateLimitRPS / parses positive rate_limit_rps` |
| `rate_limit_rps: 0` or unset → no throttling | `TestRun_GlobalRateLimit_ZeroIsUnlimited`, `TestRun_GlobalRateLimit_ZeroDoesNotGate` |
| 10 sequential requests at 5 rps → >= 1.8s wall-clock | `TestRun_GlobalRateLimit_ThrottlesSequential` |
| `--parallel` + 5 rps → workers share bucket, throughput stays <= 5 rps | `TestRun_GlobalRateLimit_ParallelSharesBucket` |
| `rate_limit_rps: -1` → structured error with line number | `TestParseFile_rateLimitRPS / negative rejected with line number` |
| `rate_limit_rps: 100` at Free or Solo tier → exit code 6 + gate error | `TestRun_GlobalRateLimit_FreeTierGated`, `TestRun_GlobalRateLimit_SoloTierGated` |
| Global + data-driven rate limits stack | `TestRun_GlobalRateLimit_StacksWithDataDriven` |
| Context cancellation while sleeping → promptly returns | `TestRun_GlobalRateLimit_ContextCancellation` |

## Quality Gates

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage >= 80% in all changed packages | PASS |

## Summary

All three iteration-1 findings have been fully resolved. The implementation is architecturally correct: the `internal/ratelimit` package provides a nil-safe, mutex-protected token-bucket used consistently across sequential, parallel, WebSocket, and data-driven execution paths. The feature gate correctly blocks Free and Solo tiers, and all 8 task-specified behaviors are covered by tests. No new issues were found in this iteration.
