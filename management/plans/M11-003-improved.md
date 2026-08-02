# Improvement Report: M11-003 (iteration 2)

**Task:** TAP output completeness: per-iteration data-driven test points + parallel speedup diagnostic
**Date:** 2026-04-26
**Review:** management/reviews/M11-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 (iter 1) | Medium | `TestTAPOutput_ParallelSpeedup` only verified the TAP `speedup_factor` was a parseable float; it did not compare it against the JSON formatter's value for the same run as required by the DoD parity contract. | Added a second invocation with `--format json --parallel`, parsed `parallel_execution.speedup_factor` from the JSON output, extracted the TAP value via regex, and asserted both match within ≤ 0.15 tolerance (one rounding step + half-step — accounts for independent timing across two separate runs). Also preserved the format check (`%.1f`). | tests pass |
| 1 (iter 2) | Low | Comments at three locations in `TestTAPOutput_ParallelSpeedup` said "≤ 0.1 difference (one rounding step)" but the actual guard was `if diff > 0.15`, creating a comment/code mismatch that would mislead readers. | Updated all three comment occurrences to say "≤ 0.15 (one rounding step + half-step tolerance)" to accurately reflect the intentional wider margin. The code was correct; only the comments were wrong. | tests pass, lint clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/output`) | 92.5% |
| Coverage (`cmd/apitest`) | 82.0% |
| Coverage (total, scoped) | 84.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 64b45f1 | fix(test): implement TAP/JSON speedup_factor parity assertion | iter 1 #1 |
| 3d2fb29 | fix(test): reconcile speedup_factor parity tolerance comment with code | iter 2 #1 |

## Summary

2/2 findings resolved across 2 review iterations. 0 deferred.

**Iteration 1** resolved the substantive parity-assertion gap: the test now
performs a real cross-formatter comparison (two separate invocations,
`--format tap --parallel` vs `--format json --parallel`), parses
`speedup_factor` from both outputs, and asserts they agree within ≤ 0.15
(one rounding step + half-step). The tolerance is necessary because two
separate in-process invocations have independent timing; the formula is
shared via `buildParallelMetadata` so any future code divergence would be
caught by a larger delta.

**Iteration 2** resolved a Low-severity comment/code mismatch: three comment
occurrences said "≤ 0.1" while the guard used `> 0.15`. Comments updated to
match the intentional code behaviour.
