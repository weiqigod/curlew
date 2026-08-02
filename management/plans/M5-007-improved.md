# Improvement Report: M5-007

**Task:** Web: custom role editor UI
**Date:** 2026-04-19
**Review:** management/reviews/M5-007-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Wrong `data-testid` in e2e spec for Billing category: `perm-section-billing---subscription` should be `perm-section-billing-subscription` (PermissionPicker's `/[^a-z]+/g` regex collapses `' & '` to a single `-`) | Changed assertion in `web/tests/e2e/org-roles.spec.ts` line 247 to `perm-section-billing-subscription` | ✓ tests pass |
| 2 | High | `countByRole` in `+page.server.ts` seeded counts by `role.id` (e.g. `builtin_owner`) but members with built-in roles contribute via `m.role` (e.g. `owner`) — mismatched keys gave 0 counts for all built-in roles | Added a `builtinNameToId` alias map from `role.name` → `role.id` for built-in roles; member iteration now resolves built-in members through the alias map to the canonical `role.id` key | ✓ tests pass |
| 3 | High | No unit tests for `countByRole` in `page-server.test.ts` despite SSO and audit-log loaders setting the precedent | Added `roles page loader` describe block to `web/src/routes-tests/page-server.test.ts` with 7 test cases: login redirect, professional-tier redirect, non-owner redirect, happy path, custom role member counts via `role_id`, built-in role member counts via role name, and error fallback | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `npm run build` | PASS |
| `npm run test:unit` | PASS (156 tests) |
| `npm run lint` (eslint) | PASS (0 errors, 0 warnings) |
| `npm run check` (svelte-check) | PASS (0 errors, 0 warnings) |
| Coverage | N/A (`@vitest/coverage-v8` not installed) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6c22f82 | fix(e2e): correct billing section testid in org-roles spec | #1 |
| 9292db8 | fix(roles): correct countByRole built-in id/name mismatch + add unit tests | #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
