# Improvement Report: M4-009

**Task:** Web: notification rule configuration UI
**Date:** 2026-04-17
**Review:** management/reviews/M4-009-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Orphaned JSDoc block in `guards.ts` — `requireTeamTier` doc was floating above `requireOrgAdmin`, and `requireTeamTier` had no doc. | Moved the JSDoc to its correct function. Gave `requireOrgAdmin` its own doc comment. Reordered so `requireOrgAdmin` precedes `requireTeamTier` in file order, matching documented "use AFTER requireTeamTier" caller guidance. | ✓ svelte-check 0 errors, 11 guard tests pass |
| 2 | Low | Misleading test name `Get_notification_rules_returns_200_with_rules_newest_first` — implementation uses `OrderBy(CreatedAt)` (oldest-first); test asserts oldest-first. | Renamed to `Get_notification_rules_returns_200_with_rules_oldest_first`. | ✓ 172 backend tests pass |
| 3 | Low | Non-idempotent seed: `failing-result.json` posted unconditionally on every re-run, accumulating delivery rows. | Added an idempotency guard: queries `GET /notification-deliveries?limit=1`, skips the failing-result post if deliveries already exist. | ✓ Manual review of seed script logic |
| 4 | Low | `confirm()` and `alert()` in `+page.svelte` — only places in the codebase using native browser dialogs; inconsistent with the existing `?toast=` pattern. | Replaced `confirm()` with an inline confirmation banner (`delete-confirm-banner`) showing Delete/Cancel buttons. Replaced `alert()` with an inline error banner (`delete-error-banner`). | ✓ svelte-check 0 errors, lint clean, 54 unit tests pass |
| 5 | Low | Race condition in "admin can delete a rule" E2E test: second `context.route` mock (empty list) registered after the DELETE click, could miss the re-fetch. | Replaced with a stateful counter-based handler registered before `page.goto()`. Counter-based handler returns one rule on first GET, empty list on subsequent GETs. Updated assertions to use new inline confirmation UI instead of `page.once('dialog')`. | ✓ svelte-check 0 errors, lint clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.csproj` | PASS |
| `dotnet test ApiTool.Backend.Tests.csproj` | PASS (172/172) |
| `npm run check` (svelte-check) | PASS (0 errors) |
| `npm run lint` (eslint) | PASS |
| `npm run test:unit` (vitest) | PASS (54/54) |
| Coverage | 54 unit tests; backend 34 notification tests + 138 other |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 698a7aa | fix(guards): move JSDoc comments to correct functions | #1 |
| 53b7ac5 | fix(tests): rename misleading test name to reflect actual sort order | #2 |
| 847ad8e | fix(seed): make failing-result post idempotent | #3 |
| 200d815 | fix(web): replace confirm/alert with inline confirmation and state mocks | #4, #5 |

## Summary

5/5 findings resolved. 0 deferred.
