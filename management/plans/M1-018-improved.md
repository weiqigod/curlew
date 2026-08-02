# Improvement Report: M1-018

**Task:** Dynamic variable functions with seed reproducibility
**Date:** 2026-03-14
**Review:** management/reviews/M1-018-review.md

## Resolved Findings (Round 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `WithOverrides` drops `registry` field; requests with per-request `variables:` blocks silently keep `{{$func}}` literals | Added `child.registry = s.registry` in `WithOverrides` after `NewScope(merged)` | ✓ tests pass |
| 2 | Medium | `Registry.Evaluate()` panics on nil cache (both read and write on nil map) | Wrapped cache read and write with `if cache != nil` guards | ✓ tests pass |
| 3 | Medium | `float64InRange` crypto/rand path (rng==nil) untested; 40% function coverage | Added `TestRegistry_no_seed_randomFloat_valid`; `float64InRange` now at 100% | ✓ tests pass |
| 4 | Low | No runner-level test covering per-request `variables:` combined with a dynamic function URL | Added `TestRun_dynamic_with_request_variables`; would have caught Bug #1 | ✓ tests pass |

## Resolved Findings (Round 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 5 | Low | `intn` crypto/rand path (nil rng) had 0% coverage — only untested branch remaining | Added `TestRegistry_no_seed_randomInt_valid`; `intn` now at 100% | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/variable` | 95.8% (up from 94.7%) |
| Coverage `internal/runner` | 91.2% |
| Coverage `cmd/apitest` | 85.5% |
| Coverage total | 92.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a08068a | fix(variable): propagate registry in WithOverrides and guard nil cache | #1, #2, #3, #4 |
| 30f0991 | fix(variable): add test for intn crypto/rand path (nil rng branch) | #5 |

## Summary

5/5 findings resolved across 2 review rounds. 0 deferred.
