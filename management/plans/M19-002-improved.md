# Improvement Report: M19-002

**Task:** internal/cel/ foundation: Evaluator + standard activation
**Date:** 2026-05-16
**Review:** management/reviews/M19-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | No CHANGELOG.md entry for M19-002 | Added entry under `[Unreleased] → Added` describing the new `internal/cel/` package: Evaluator interface, StandardActivation, ErrCelParse/ErrCelType sentinels, time-of-day rejection, and sensitive observer contract | ✓ tests pass |
| 2 | Low | `TestEvaluator_RejectsZeroArgTimestamp` did not assert error message names "timestamp" | Added `errors.As` assertion and `strings.Contains(ce.Error(), "timestamp")` check, matching the pattern used in `TestEvaluator_RejectsTimeOfDayNow` | ✓ tests pass |
| 3 | Low | No test exercising `response.headers` CEL field access | Added `response.headers access` sub-case to `TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv` that compiles and evaluates `response.headers.Authorization == "Bearer x"` | ✓ tests pass |
| 4 | Low | Redundant `CelError.Is(target error) bool` method — `Unwrap` already provides errors.Is traversal | Removed the `Is` method entirely; `Unwrap` remains and is sufficient | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/cel/`) | 92.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2809515c | fix(cel): remove redundant CelError.Is method | #4 |
| 6f5a5c9a | test(cel): strengthen timestamp rejection and add headers access test | #2, #3 |
| 7810c747 | docs(changelog): add M19-002 entry for internal/cel/ foundation | #1 |

## Summary

4/4 findings resolved. 0 deferred.
