# Improvement Report: M12-001

**Task:** Argument-parsing foundation for dynamic functions
**Date:** 2026-04-28
**Review:** management/reviews/M12-001-review.md

## Iteration 1 Resolved Findings (all carried forward as resolved)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `arityError("echo", 0, len(args))` in cache sub-test used wrong expected-arity (0 instead of 1) | Changed to `arityError("echo", 1, len(args))` in `dynamic_test.go` | ✓ |
| 2 | Medium | Behavior 6 (N-arg arity mismatch) had no dedicated test | Added `TestInterpolate_ArityMismatch_NArgFunction` | ✓ |
| 3 | Low | `TestRegistry_available_sorted` used soft `>= 15` bound | Changed to exact `!= 15` equality | ✓ |
| 4 | Low | Task YAML status inconsistent with backlog | Synced `status: review` | ✓ |

## Iteration 2 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Double-prefixed arity error message: `arityError` embedded `"$funcName:"` in `Message`, then `Evaluate` wrapped with `fmt.Errorf("$%s: %w", name, err)`, producing `"$timestamp: $timestamp: expected 0 arguments, got 1"` in terminal/JSON output | Removed `"$funcName:"` prefix from `arityError.Message` (now `"expected %d arguments, got %d"`); `Evaluate`'s wrapper supplies the sole `$name:` context. Updated `TestRegistry_Evaluate_AcceptsArgs/zero-arg` to assert `strings.Count(err.Error(), "$timestamp") == 1`. | ✓ tests pass |
| 2 | Low | MANUAL.md §3.7 documents `{{$fn()}}` as equivalent to `{{$fn}}`, but no `Interpolate`-level test verified this end-to-end equivalence | Added `TestInterpolate_EmptyParensEquivalence`: asserts `{{$timestamp()}}` and `{{$uuid()}}` resolve to non-empty values, and that `{{$timestamp()}}` returns the same memoized value as `{{$timestamp}}` within one request. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/variable`) | 96.0% |

## Fix Commits (Iteration 2)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `291f06a` | fix(variable): remove doubled function-name prefix from arity error | #1 |
| `884a5ec` | test(variable): add empty-parens equivalence test | #2 |

## Summary
2/2 iteration-2 findings resolved. 0 deferred.
