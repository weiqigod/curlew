# Code Review: M5-003

**Task:** Web: SSO configuration UI (SAML + OIDC)
**Reviewer:** AI
**Date:** 2026-04-18
**Iteration:** 3 (after /improve iteration 2)
**Branch:** feature/M5-003-web-sso-config-ui

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Resolved in Previous Iterations

| Iteration | # | Severity | Finding | Resolution |
|-----------|---|----------|---------|------------|
| 1 | 1 | High | SSO sub-nav link gated on `isTeamTier` — enterprise orgs never see the link | Fixed: `isEnterpriseTier` reactive var added; SSO link now uses `isEnterpriseTier && role === 'owner'` |
| 1 | 2 | High | `nonEnterpriseTier` field declared but never used in SSO loader test cases | Fixed: added `redirects when non-enterprise tier` case |
| 1 | 3 | Medium | `Tabs.svelte` in `components/ui/` had hardcoded SSO-specific slot names | Fixed: moved/renamed to `web/src/lib/components/sso/SsoTabs.svelte` |
| 1 | 4 | Medium | `'solo'` tier missing from `requireEnterpriseTier` test cases | Fixed: `solo tier redirects` case added |
| 2 | 5 | Medium | `admin_denied` theory case in `GetConfig_as_non_owner_returns_permission_denied` used `f.MemberId` (Member role) for both cases — admin code path never actually exercised | Fixed: `AdminId` seeded with `OrgRole.Admin`; theory uses `bool isAdmin` to select correct user |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | C# uses consistent `(result, err)` tuple pattern. TypeScript surfaces errors via `ApiError` with optional `field`. All `SsoError` variants handled in `GetSsoConfig` handler. No swallowed errors. SvelteKit redirect/error throws from guards are correctly placed before the try block in the loader. |
| Input Validation | PASS | SAML upsert validates all required fields before DB calls. OIDC form validates `client_secret` client-side. Backend guards against invalid org ID format (returns 404). `fieldError` helper correctly merges client-side and server-side errors without double-displaying. |
| Naming | PASS | No stuttering. C# records use PascalCase. TypeScript interfaces use snake_case matching wire format. JSDoc/XML-doc present on all public exported symbols. `SsoTabs.svelte` correctly co-located in `components/sso/`. |
| Code Organization | PASS | SSO components co-located in `components/sso/`. Package/module boundaries respected. Single responsibility per component. `SsoConfigView` correctly strips secrets (no `idp_cert_pem`, no `client_secret` fields). |
| Correctness | PASS | `isEnterpriseTier` layout predicate matches `requireEnterpriseTier` guard logic exactly. `GetConfigAsync` checks membership before org existence, preventing org-existence probing. Client secret not pre-populated in OIDC form (correct: `OidcConfigView` has no secret field). Admin-role code path now verified independently from member-role path. |
| Test Quality | PASS | All code paths covered. Admin-seeding fix confirmed: `AdminId` seeded with `OrgRole.Admin`, theory uses `bool isAdmin` to select correct user ID. 114 backend tests pass. 106 web unit tests pass. E2E spec has 7 assertions covering all 6 task behaviors plus sub-nav visibility. |

## Test Coverage

- **Backend (C# unit + integration):** 114 tests pass. `GetConfigAsync` covered for: owner-no-SSO, owner-saml, owner-oidc, cert-not-exposed, saml-fields-non-null-oidc-null, admin-denied, member-denied, unknown-org. Endpoint integration tests cover GET success, GET 403, GET 401, GET invalid-format. Coverage ~90%.
- **Web unit tests:** 106 tests pass. `api-error.test.ts` covers `field` round-trip (with and without field). `client.test.ts` covers `api.put` and field propagation on 400. `sso.test.ts` covers all 6 planned cases. `guards.test.ts` covers `requireEnterpriseTier` with all 7 tier values (enterprise, team, absent, professional, solo, free, null-org). Page loader tests: 9 cases covering auth, 404, admin redirect, member redirect, enterprise-tier redirect, happy-path, saml provider, oidc provider, and api-throws error. Coverage ~88%.
- **E2E (Playwright):** 7 assertions — two-tabs render, SAML save + toast, OIDC save + toast, inline field error on 400, test-login button href/target, non-owner redirect with toast, sub-nav SSO link visibility.

## Behavior Coverage (Task YAML)

| Behavior | Test(s) |
|----------|---------|
| Page renders SAML/OIDC tabs and sso_provider badge | E2E test 1; page-server loader tests |
| SAML submit → PUT /sso/saml → "SSO enabled" toast | E2E test 2; `sso.test.ts` upsertSaml cases |
| OIDC submit → PUT /sso/oidc → toast | E2E test 3; `sso.test.ts` upsertOidc cases |
| 400 invalid_sso_config → inline field error | E2E test 4; `sso.test.ts` field propagation; `client.test.ts` |
| Test SSO login button → new tab with correct href | E2E test 5 (href, target=_blank, rel=noopener verified) |
| Non-owner → redirect + owner_required toast | E2E test 6; page-server loader (admin-redirect, member-redirect cases) |
| svelte-check clean | Verified by build pass |

## Summary

All five findings from iterations 1 and 2 are confirmed correctly resolved. The iteration-2 finding (admin code path untested in `GetConfigAsync`) is fixed: `AdminId` is now seeded with `OrgRole.Admin` and the theory test selects between `AdminId` and `MemberId` via `bool isAdmin`, making the admin and member rejection paths independently verified. All 114 backend tests and 106 web unit tests pass. All 7 task behaviors are covered by tests. The implementation is correct, well-structured, and meets all project standards.
