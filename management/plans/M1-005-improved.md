# Improvement Report: M1-005 (Re-review)

**Task:** Assert on response body with JSONPath
**Date:** 2026-03-10
**Review:** management/reviews/M1-005-review.md (re-review)

## Previous Improvements

All 3 findings from the initial review were resolved in the prior improvement cycle:
- #1 (High): `evalBodyAssertion` handles `ErrInvalidPath` — fixed in 595348f
- #2 (Medium): Renamed `BodyAssertionInput` → `BodyInput` — fixed in 5e29d1e
- #3 (Low): Negative array index returns `ErrInvalidPath` — fixed in 037188e

## Resolved Findings (Re-review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `valuesEqual` Sprintf fallback causes cross-type false positives (string "1" == number 1, string "true" == boolean true) | Added type guard before Sprintf fallback: different non-numeric types return false immediately. Added 3 test cases for cross-type rejection. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 93.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 3b7fcb6 | fix(assertion): reject cross-type equality in valuesEqual | #1 |

## Summary
1/1 findings resolved. 0 deferred. Coverage at 93.4%.
