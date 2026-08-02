# Improvement Report: M5-005

**Task:** Web: audit log viewer page
**Date:** 2026-04-18
**Review:** management/reviews/M5-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Behavior 4 requires both `?from=` and `?to=` in URL when a date range is applied; `pickRange()` only set `?from=` and cleared `?to=`; E2E only asserted `/from=/`. | Updated `pickRange()` in `+page.svelte` to compute `to = now.toISOString()` and pass it to `applyFilter({ from, to })` for all presets except `all`. Updated E2E Behavior 4 test to also assert `toHaveURL(/to=/)`. | ✓ tests pass |
| 2 | Low | E2E "renders 10 newest-first rows" test verified row count and content but not column header count (risk of silently dropping columns). | Added `await expect(headers).toHaveCount(5)` assertion on the `thead th` locator in the E2E Behavior 1 test. | ✓ tests pass |
| 3 | Low | CSV `quote()` function checks for `\r` but no test case covered a carriage-return field, leaving that branch untested. | Added test case `{ name: 'quotes fields with carriage returns', entries: [entry({ ip_address: 'a\rb' })], expectSubstring: '"a\rb"' }` to `csv.test.ts`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `npm run build` | PASS |
| `npm run test:unit` | PASS |
| `npm run lint` | PASS |
| `npm run check` (svelte-check) | PASS |
| Tests | 137 passed (16 test files) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7c99209 | fix(audit-log): resolve review findings #1, #2, #3 | #1, #2, #3 |

## Summary
3/3 findings resolved. 0 deferred.
