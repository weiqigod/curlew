# Improvement Report: M12-006

**Task:** $dateAdd and $dateSubtract relative-time arithmetic
**Date:** 2026-04-28
**Review:** management/reviews/M12-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestRegistry_DateAdd_AddDateMonthRollover` used `amount='13'` but task YAML behavior 6 explicitly specifies `amount='12' unit='month' on date 2026-01-31`. The test demonstrated rollover but did not match the literal spec. | Converted the test to a table-driven sub-test with two cases: `amount='12'` → `2027-01-31T00:00:00Z` (no rollover — the spec behaviour 6 literal) and `amount='13'` → `2027-03-03T00:00:00Z` (rollover via Feb 31). Both cases pass with clear names and comments. | ✓ tests pass |
| 2 | Medium | `TestRegistry_DateSubtract` used `('30', 'minute')` but task behavior 2 specifies `$dateSubtract('7', 'day')`. The mandated behavior was not directly exercised. | Changed `TestRegistry_DateSubtract` to use `('7', 'day')` expecting `2026-04-21T12:00:00Z` (base `2026-04-28T12:00:00Z` minus 7 days). Added new `TestRegistry_DateSubtract_AllUnits` table test covering minute/hour/day/week/month/year for `$dateSubtract`, which preserves the 30-minute scenario. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/variable`) | 96.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| d8e8e91 | test(variable): fix spec-alignment for behaviors 2 and 6 | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
