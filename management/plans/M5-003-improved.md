# Improvement Report: M5-003

**Task:** Web: SSO configuration UI (SAML + OIDC)
**Date:** 2026-04-18
**Review:** management/reviews/M5-003-review.md

## Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | SSO sub-nav link gated on `isTeamTier` — enterprise orgs evaluate this as false, hiding the SSO link from the users who need it | Added `isEnterpriseTier` reactive var to `+layout.svelte` (mirrors `requireEnterpriseTier` guard logic). Changed SSO link condition from `isTeamTier && role === 'owner'` to `isEnterpriseTier && role === 'owner'` | ✓ build + svelte-check pass |
| 2 | High | `nonEnterpriseTier` field declared in SSO page loader test interface but no test case used it; the `requireEnterpriseTier` redirect path was untested | Added `{ name: 'redirects when non-enterprise tier', nonEnterpriseTier: true, expectThrows: true }` case and corresponding mock setup (`tier: 'professional'`) in the test loop body | ✓ 106 tests pass |
| 3 | Medium | `Tabs.svelte` placed in `components/ui/` with hardcoded SSO-specific slot names (`saml`, `oidc`) and ARIA label — not a reusable generic component | Renamed and moved to `web/src/lib/components/sso/SsoTabs.svelte`; updated import in `+page.svelte` | ✓ build + svelte-check pass |
| 4 | Medium | `'solo'` tier not covered in `requireEnterpriseTier` guard unit tests (parallel `requireTeamTier` suite had an explicit `solo tier redirects` case) | Added `{ name: 'solo tier redirects', tier: 'solo', expected: 'redirect' }` to `requireEnterpriseTier` test cases | ✓ 106 tests pass |

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 5 | Medium | `admin_denied` InlineData theory case in `GetConfig_as_non_owner_returns_permission_denied` used `f.MemberId` (Member role) for both cases — Admin code path never actually exercised; a future regression permitting admins would not be caught | Added `AdminId` to the `Fixture` class; seeded a `OrgRole.Admin` membership row in `BuildAsync`; updated the `[Theory]` to accept `bool isAdmin` and select `f.AdminId` or `f.MemberId` accordingly | ✓ 359 backend tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS |
| `dotnet test` (359 tests) | PASS |
| `npm run check` (svelte-check) | PASS (0 errors, 0 warnings) |
| `npm run lint` (eslint) | PASS |
| `npm run test:unit` | PASS (106 tests, 14 files) |
| Coverage | ~90% backend, ~88% web |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| e3ce657 | test(guards,sso): add missing tier test cases | #4, #2 |
| f37ae8a | fix(sso): fix SSO subnav visibility and move Tabs to sso components | #1, #3 |
| 38260dc | fix(sso-tests): seed admin-role user in fixture and use it for admin_denied case | #5 |

## Summary

5/5 findings resolved. 0 deferred.
