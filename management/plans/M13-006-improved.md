# Improvement Report: M13-006

**Task:** $faker content data — 5 functions (4 argument-bearing)
**Date:** 2026-04-29
**Review:** management/reviews/M13-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `isApproxText` tolerances in `TestRegistry_FakerContent` were 130/150 — far too loose to validate the length contract from behaviors 8 and 9 | Tightened to `isApproxText(200, 80)` and `isApproxText(50, 80)`. 80 is calibrated from empirical measurement (max overshoot ~74 chars over 100 seeds due to sentence-boundary stop); a comment explains the rationale. Review suggested 60, but 60 would cause flaky CI failures given the measured max overshoot. | ✓ tests pass |
| 2 | Low | `loremWords` pool had 66 duplicate entries (267 raw, 176 unique), reducing effective entropy. No test guarded against duplicates | Replaced duplicate second-half repetitions with distinct classical Latin vocabulary. Pool now has 247 unique entries (0 duplicates). Added `TestFakerContent_PoolNoDuplicates` test to guard against future regressions. | ✓ tests pass |
| 3 | Low | `generateSentence(rng, 0)` would panic on empty slice index at `words[0]` — no defensive guard despite comment "Caller must pass n >= 1" | Added `if len(words) == 0 { return "." }` guard before the capitalisation step. Returns a safe value rather than panicking, per CLAUDE.md policy. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/variable/...`) | 97.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `17c6f76` | fix(variable): guard generateSentence against n=0 to avoid panic | #3 |
| `3b49226` | fix(variable): deduplicate loremWords pool and add no-duplicates test | #2 |
| `055298f` | fix(variable): tighten isApproxText tolerances in TestRegistry_FakerContent | #1 |

## Summary

3/3 findings resolved. 0 deferred.
