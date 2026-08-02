# Code Review: M16-020

**Task:** Web /dashboard page with overview cards, trend chart, and failing endpoints
**Reviewer:** AI
**Date:** 2026-05-12
**Branch:** feature/M16-020-web-dashboard-page
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Server loader uses `Promise.allSettled` for graceful degradation; 402, 400, and generic errors are each caught and routed to appropriate page state; `instanceof ApiError` checks are correct. |
| Input Validation | PASS | Raw `?window=` param forwarded verbatim to the backend per plan decision 6 (backend owns the allow-list); 400 response caught and rendered as `windowError`; free-tier `org.tier` uses `?? 'team'` defensive fallback matching the existing convention. |
| Naming | PASS | No stuttering; `window` variable renamed to `currentWindow` in the page component to avoid shadowing the `window` global. All exported symbols have JSDoc comments. |
| Code Organization | PASS | New components placed under `lib/components/dashboard/`; format helpers isolated in `lib/dashboard/format.ts`; server loader imports only what it needs; no circular dependencies. |
| Correctness | PASS | Single-element trend chart handled via `Math.max(1, trend.length - 1)`; empty trend renders no polyline; `limit_clamped` flows through correctly; `resultsApi.list` failure degrades gracefully; window picker is now hidden when `loadError` is set (finding #3 from iteration 1 resolved). |
| Test Quality | PASS | All three medium/low findings from iteration 1 resolved: Behavior 6 row content is now fully asserted in E2E; dashboard server loader has 8 unit test cases in `page-server.test.ts`; window picker condition guards all error states. |

## Test Coverage

- **`web/src/lib/api/dashboard.test.ts`**: 11 unit tests — envelope parsing, query-param forwarding, 402/400 surface, omit-when-absent, orgId in URL, `limit_clamped` pass-through.
- **`web/src/lib/dashboard/format.test.ts`**: 14 unit tests — full branch coverage of `formatRelativeTime` (zero, seconds, minutes, hours, days, future, invalid), `formatPassRate`, and `formatDuration`.
- **`web/tests/e2e/org-dashboard.spec.ts`**: 8 E2E test cases covering all 8 behaviors, including Behavior 6 row-content assertions (method, path, failure_count, relative time).
- **`+page.server.ts`**: 8 unit test cases in `page-server.test.ts` covering auth redirect, 404, free-tier short-circuit, 402→tierGate, 400→windowError, generic loadError, happy path, and recent-runs graceful degradation.
- Pre-existing lint/check errors in `schedules/+page.svelte` and `page-server.test.ts` exist on `main`; none introduced by this task.

## Summary

All three findings from the first review iteration have been cleanly resolved. The implementation is structurally sound: tier-gate UX is in-place, window-param forwarding delegates validation to the backend, the parallel fetch fan-out degrades gracefully, format helpers are fully unit-tested, and the server loader now has unit test coverage matching the precedent set by the results-page loader. The CI gate passes cleanly.
