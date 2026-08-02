# Code Review: M4-011

**Task:** Web: billing and seat management portal
**Reviewer:** AI
**Date:** 2026-04-17
**Branch:** feature/M4-011-web-billing-members
**Iteration:** 2 (post-improvement)

## Verdict: PASS

## Findings

No findings. All four issues from the first review (iteration 1) were resolved by commits `476d736` and `d344a7d`.

Previously resolved findings (for traceability):

| # | Severity | Category | File | Finding | Resolution |
|---|----------|----------|------|---------|------------|
| 1 | Medium | Test Quality | `web/src/lib/api/invitations.test.ts` | `invitationsApi.list` had no error-propagation test | `it('propagates 403 permission_denied', ...)` added — 9 tests in describe block now |
| 2 | Medium | Test Quality | `web/tests/e2e/org-billing.spec.ts` | E2E test 1 did not assert `subscription-price` or `subscription-renewal` | Both assertions added to test 1 |
| 3 | Medium | Test Quality | `web/tests/e2e/org-billing.spec.ts` | E2E test 4 did not assert two-table members layout | `members-table` and `invitations-table` visibility asserted before invite flow |
| 4 | Low | Code Quality | `web/src/lib/types/subscriptions.ts` | `PortalRequest` exported but unused | Now imported and used in `subscriptionsApi.portal` to type the POST body |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Server loaders catch API errors and return graceful error state; API client propagates `ApiError` with `code` and `status`; no swallowed errors. |
| Input Validation | PASS | `AddSeatsModal` validates `addCount >= 1` with `Number.isInteger`; `InviteMemberModal` validates email with `EMAIL_RE` before dispatch. Both show inline validation errors. |
| Naming | PASS | No stuttering; all exported symbols have JSDoc comments; component names follow SvelteKit conventions. |
| Code Organization | PASS | Guards in `$lib/server/guards`; types separated from API clients; components scoped under `billing/` and `members/` subdirectories; `PortalRequest` is now used rather than dead code. |
| Correctness | PASS | Guard chain correct (auth → team tier → owner/admin); `Promise.all` for parallel member+invitation loading; `invalidateAll()` after all mutations; Escape key and backdrop click both invoke `handleClose`; `rel="noopener noreferrer"` on portal anchor; `aria-valuenow/min/max` on seats bar; all form inputs have associated `<label>` elements. |
| Test Quality | PASS | 80 unit tests pass; all three new API modules (subscriptions, invitations, members) and the new `requireOrgOwner` guard have full error-path coverage; 7 E2E assertions covering all 8 specified behaviors; `svelte-check` 0 errors/0 warnings; `eslint` clean. |

## Test Coverage

- Unit tests: 80 pass, 0 fail
- E2E assertions: 7 tests covering all 8 task behaviors
- `svelte-check`: 0 errors, 0 warnings
- `npm run lint`: clean
- `@vitest/coverage-v8` not installed; numeric % unavailable

## Behavior Coverage

| Behavior | Test | Status |
|----------|------|--------|
| Billing page renders tier, price, renewal, seats bar | E2E test 1 | COVERED |
| Add seats sends PATCH, seats bar updates | E2E test 2 | COVERED |
| Manage billing POSTs to /portal, redirects | E2E test 6 | COVERED |
| Members and invitations listed in separate tables | E2E test 4 | COVERED |
| Invite sends POST, invitation row appears | E2E test 4 | COVERED |
| Cancel invitation sends DELETE, row removed | E2E test 5 | COVERED |
| Non-owner on /billing sees owner_required toast | E2E test 3 | COVERED |
| svelte-check reports no type errors | `npm run check` | COVERED |

## Definition of Done

| Item | Status |
|------|--------|
| Playwright spec passes with >=6 assertions | PASS — 7 tests |
| svelte-check and npm run lint pass | PASS |
| Billing and Members appear in org sub-nav with correct role guards | PASS — owner sees Billing, admin+owner see Members |
| Accessibility: forms have labels, portal redirect uses `rel=noopener` | PASS |
| Types in `web/src/lib/types/subscriptions.ts` mirror backend OpenAPI | PASS |
| README under `web/` documents seeding a team-tier org | PASS — section added at line 85 |

## Summary

All four findings from iteration 1 are fully resolved. The implementation is complete, well-structured, and accessible: guard chain is correct, component decomposition follows established conventions, all form inputs have labels, the portal anchor carries `rel="noopener noreferrer"`, the seats bar has ARIA progressbar attributes, and all 8 specified behaviors are covered by automated tests. Quality gates (unit tests, svelte-check, lint) are clean.
