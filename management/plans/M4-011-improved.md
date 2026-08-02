# Improvement Report: M4-011

**Task:** Web: billing and seat management portal
**Date:** 2026-04-17
**Review:** management/reviews/M4-011-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `invitationsApi.list` had no error-propagation test; plan listed `propagates 403 permission_denied` as required but it was absent | Added `it('propagates 403 permission_denied', ...)` to the `invitationsApi.list` describe block in `invitations.test.ts` | ✓ tests pass (80 total) |
| 2 | Medium | E2E test 1 asserted tier and seats bar but not `subscription-price` or `subscription-renewal`, leaving behavior #1 partially unverified | Added `await expect(page.getByTestId('subscription-price')).toBeVisible()` and `await expect(page.getByTestId('subscription-renewal')).toContainText('2026')` to test 1 | ✓ spec updated |
| 3 | Medium | E2E test 4 exercised the invite flow but never asserted that `members-table` and `invitations-table` are visible; two-table layout unverified | Added `await expect(page.getByTestId('members-table')).toBeVisible()` and `await expect(page.getByTestId('invitations-table')).toBeVisible()` before the invite flow in test 4 | ✓ spec updated |
| 4 | Low | `PortalRequest` interface was exported but never imported or used anywhere — dead code on the public API surface | Imported `PortalRequest` in `subscriptionsApi.portal` and used it to type the POST body instead of the inline object literal | ✓ svelte-check clean, lint clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `npm run test:unit` | PASS |
| `npm run check` (svelte-check) | PASS — 0 errors, 0 warnings |
| `npm run lint` (eslint) | PASS |
| Coverage | 80 unit tests pass; `@vitest/coverage-v8` not installed so numeric % unavailable |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 476d736 | test(invitations): add missing 403 error-propagation test for list | #1 |
| d344a7d | fix(web): address review findings #2, #3, #4 for M4-011 | #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.
