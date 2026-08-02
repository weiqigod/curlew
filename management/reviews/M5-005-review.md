# Code Review: M5-005

**Task:** Web: audit log viewer page
**Reviewer:** AI
**Date:** 2026-04-18
**Branch:** feature/M5-005-web-audit-log-viewer
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All three issues from Review 1 were fully resolved:

| Prior # | Severity | Finding | Resolution |
|---------|----------|---------|------------|
| 1 | Medium | Behavior 4: `pickRange()` only set `?from=`, did not set `?to=`; E2E test only asserted `/from=/`. | `pickRange()` now sets `to = now.toISOString()` for all non-`all` presets and passes it to `applyFilter({ from, to })`. E2E Behavior 4 test now asserts both `toHaveURL(/from=/)` and `toHaveURL(/to=/)`. |
| 2 | Low | E2E "renders 10 newest-first rows" test did not assert all five column headers. | Added `await expect(page.getByTestId('audit-log-table').locator('thead th')).toHaveCount(5)` to the E2E Behavior 1 test. |
| 3 | Low | CSV `quote()` function checked for `\r` but no test covered a carriage-return-containing field. | Added test case `{ name: 'quotes fields with carriage returns', entries: [entry({ ip_address: 'a\rb' })], expectSubstring: '"a\rb"' }` to `csv.test.ts`. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `+page.server.ts` catches API errors and surfaces them via the `error` field. Export endpoint converts API errors to 502. Guards throw appropriate SvelteKit redirects. No errors swallowed. |
| Input Validation | PASS | `?page=` guarded by `Math.max(1, Number(...) \|\| 1)` and clamped to `totalPages`. Null-coalescing before every `quote()` call. Filter params forwarded verbatim to backend (intentional — backend validates ISO dates). Negative and NaN page values all correctly resolve to 1. |
| Naming | PASS | No stuttering. `auditLogApi`, `formatAuditLogCsv`, `AUDIT_LOG_PAGE_SIZE`, `AUDIT_EVENT_TYPES` are clear and consistent with existing module conventions. `quote()` private helper is un-exported. Doc comments on all exported symbols. |
| Code Organization | PASS | Types in `$lib/types/`, CSV formatter in `$lib/audit-log/`, API client in `$lib/api/`, server guards reused from `$lib/server/guards`. Clean separation. No circular dependencies. |
| Correctness | PASS | Pagination clamping correct. CSV RFC 4180 quoting logic verified (formula-injection prefix, double-quote doubling, newline and carriage-return wrapping). `pickRange()` now correctly sets both `from` and `to`. Layout `isEnterpriseTier && isAdmin` gate correctly wrapped in `{#if currentOrg}` block. |
| Test Quality | PASS | 137 unit tests pass across 16 test files. svelte-check: 0 errors. ESLint: clean. Build: succeeds with no warnings. All 7 behaviors from the task YAML have test coverage. Carriage-return CSV quoting branch now exercised by a dedicated test case. |

## Spec Compliance (Behavior Coverage)

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Table renders with columns: event_type, user, target, timestamp, IP | E2E: "renders 10 newest-first rows" + `toHaveCount(5)` on thead th | PASS |
| 2 | 10 rows per page with pagination controls | E2E: "pagination navigates between pages"; unit: loader page=2/clamped cases | PASS |
| 3 | event_type filter → URL gains `?event_type=member.invited` | E2E: "event_type filter updates URL and narrows rows" | PASS |
| 4 | Date range → URL includes `?from=...` and `?to=...` | E2E: "range 7 days adds ?from= and ?to= to URL" (both assertions present) | PASS |
| 5 | CSV export → header row `event_type,user,target,timestamp,ip` | E2E: "Export CSV triggers a download with correct header row"; unit: csv formatter + export endpoint | PASS |
| 6 | Non-admin → redirected with 'Admin access required' toast | E2E: "non-admin is redirected with admin_required toast"; unit: loader memberRole case | PASS |
| 7 | svelte-check reports no type errors | `npm run check` = 0 errors 0 warnings | PASS |

## Test Coverage
- Unit tests: 137 passed (16 test files)
- ESLint: 0 errors
- svelte-check: 0 errors, 0 warnings
- Production build: succeeds with no warnings
- E2E spec: 7 Playwright tests covering all 6 observable behaviors + DoD subnav link assertion

## Summary

The implementation is complete and correct. All three findings from Review 1 were resolved cleanly: the date-range picker now sets both `?from=` and `?to=` query params (satisfying Behavior 4 as written), the E2E column-header assertion guards against silent regressions, and the CSV carriage-return branch is now tested. All 137 unit tests pass, ESLint is clean, svelte-check reports zero errors, and the production build succeeds without warnings. No new findings were identified in this review.
