# Improvement Report: M16-020

**Task:** Web /dashboard page with overview cards, trend chart, and failing endpoints
**Date:** 2026-05-12
**Review:** management/reviews/M16-020-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Behavior 6 row content (method, path, failure_count, last_seen relative time) was not asserted in the E2E test — only the container visibility was checked | Added `data-testid` attributes (`failing-endpoint-method`, `failing-endpoint-path`, `failing-endpoint-count`, `failing-endpoint-last-seen`) to each cell in `FailingEndpointsList.svelte`. Extended the first E2E test to assert row content: "GET", "/api/users/:id", "3", and `/\d+[smhd] ago/` regex match on the last-seen cell | ✓ tests pass |
| 2 | Medium | Dashboard page server loader had no unit tests; analogous results loader has 7 unit cases covering auth redirect, 404, tier-gate, and error handling | Added `vi.mock('$lib/api/dashboard', ...)` at the top of `page-server.test.ts` and a `describe('dashboard page loader', …)` block with 8 cases: auth redirect, 404, free-tier short-circuit (verified getStats not called), 402→tierGate, 400→windowError, generic error→loadError, happy path (stats+failures+recentRuns), and graceful recent-runs degradation | ✓ 241 tests pass |
| 3 | Low | Window picker condition `{#if !data.tierGate && !data.windowError}` rendered the picker above generic error states (5xx, 403, network failures), confusing UX | Added `&& !data.loadError` to the condition | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `npm run test:unit` | PASS — 241 tests across 24 files |
| `npm run check` (svelte-check) | PASS — 2 pre-existing errors in schedules files (verified on main), 0 new errors introduced |
| `npm run lint` | PASS — 2 pre-existing errors in schedules files (verified on main), 0 new errors introduced |
| Coverage | N/A (web — vitest, no coverage gate configured) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fc598a56 | fix(dashboard): hide window picker when loadError is set | #3 |
| 30d26cc7 | fix(dashboard): add row-content assertions for Behavior 6 failing endpoints | #1 |
| b4375e5b | test(dashboard): add unit tests for dashboard page server loader | #2 |

## Summary

3/3 findings resolved. 0 deferred.
