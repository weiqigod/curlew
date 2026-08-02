# Code Review: M5-007

**Task:** Web: custom role editor UI
**Reviewer:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-007-web-custom-role-editor
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Previous Findings — All Resolved

| # | Severity | Finding | Resolution Status |
|---|----------|---------|-------------------|
| 1 | Critical | Wrong `data-testid` in e2e spec for Billing category (`perm-section-billing---subscription` vs `perm-section-billing-subscription`) | Fixed: assertion now matches computed value |
| 2 | High | `countByRole` used `role.id` keys but built-in members contribute via `role.name` — always 0 for built-in roles | Fixed: `builtinNameToId` alias map resolves name → id correctly |
| 3 | High | No unit tests for `countByRole` despite SSO and audit-log loaders setting the unit-test precedent | Fixed: 7 unit tests added to `page-server.test.ts` |

## Findings

No new findings.

## Verification of Fixes

### Fix 1 — Billing testid
`/[^a-z]+/g` collapses `' & '` to a single `-`, producing `perm-section-billing-subscription`.
E2e spec line 247 now asserts `'perm-section-billing-subscription'`. All 6 category testids
verified by running the transform in Node against the full `PERMISSION_CATEGORIES` list.

### Fix 2 — `countByRole` correctness
Manual simulation confirms: with 3 roles (`builtin_owner`, `builtin_member`, `role_qa_lead`)
and 3 members (u1=owner built-in, u2/u3=member with role_id=role_qa_lead),
counts are `{ builtin_owner: 1, builtin_member: 0, role_qa_lead: 2 }` — correct.

### Fix 3 — Unit tests
`page-server.test.ts` now has a `roles page loader` describe block with 7 test cases:
- login redirect, professional-tier redirect, non-owner (admin) redirect
- happy path returns roles and org
- custom role member counts via `role_id`
- built-in role member counts via role name-to-id alias
- error fallback returns empty roles + populated error string

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `ApiError` propagated with `details` captured for 409; catch blocks in page.svelte and page.server.ts are appropriate; no swallowed errors |
| Input Validation | PASS | `orgId`/`roleId` are `encodeURIComponent`-encoded; form name trimmed before submit; empty permissions allowed per spec |
| Naming | PASS | No stuttering; all exported symbols have doc comments; no underscores in package/module names |
| Code Organization | PASS | types → api → component → page.server → page.svelte layering respected; single responsibility per file |
| Correctness | PASS | `countByRole` alias fix verified; all 6 category testids now match PermissionPicker output; `invalidateAll()` used after mutations to avoid stale-cache |
| Test Quality | PASS | 9 unit tests for `rolesApi` (all error paths), 7 unit tests for `roles page loader`, 8 e2e tests covering all 7 behaviors + subnav DoD; 156 total unit tests passing |

## Test Coverage

- `npm run test:unit`: 156 tests, 17 test files, all PASS
- `npm run lint` (eslint): 0 errors, 0 warnings
- `npm run check` (svelte-check): 0 errors, 0 warnings
- `npm run build`: clean build, 0 warnings
- E2e: 8 tests in `org-roles.spec.ts` covering all 7 task behaviors
- Unit: `roles.test.ts` (9), `api-error.test.ts` (7 including `details` round-trip),
  `client.test.ts` (8 including `details` capture), `page-server.test.ts` (7 for roles loader)
- `@vitest/coverage-v8` not installed; numeric % unavailable

## Summary

All three findings from the first review are correctly resolved. The `countByRole` fix
is verified both by the new unit tests and by manual simulation. The e2e billing testid
now matches the PermissionPicker's `/[^a-z]+/g` output. The code is structurally sound,
well-typed, and all quality gates pass. No new issues found in this review iteration.
