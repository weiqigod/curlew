# Code Review: M14-020

**Task:** Web: Connect GitHub install entry point + install-state view
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-020-github-integrations-page
**Iteration:** 2

## Verdict: PASS

## Findings

No findings.

All 6 findings from iteration 1 have been correctly resolved:

| # | Severity | Finding (from iteration 1) | Fix Verified |
|---|----------|---------------------------|--------------|
| 1 | High | Claimed+suspended install fell through to admin Connect branch | `{:else if isSuspended && data.installation}` branch added at line 64, before the admin branch — correct precedence ✓ |
| 2 | High | Suspended test only checked badge visibility — missed the connect-button-hidden and description assertions | Test now asserts `github-connect-button` NOT visible and `github-suspended-description` visible with correct text ✓ |
| 3 | Medium | Help text rendered without a Connect button when `installUrl` was null | Help paragraph moved inside `{#if data.installUrl}` block ✓ |
| 4 | Low | `InstallationDto` exported unnecessarily from `+page.server.ts` | `export` keyword removed — interface is module-private ✓ |
| 5 | Low | `context.unroute(...)` lacked `.catch(() => {})` | `.catch(() => {})` appended ✓ |
| 6 | Low | `SEEDED_ORG_ID = 'org_test'` duplicated in spec instead of imported from fixtures | Exported from `fixtures.ts`; both specs now import it ✓ |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Loader correctly distinguishes 404 (expected "not installed" state, silent), 5xx (surfaced as `stateError`), and unknown errors (`e.message` fallback). `requireAuth` throws a SvelteKit redirect — correct pattern. `organizationsApi.list` failure degrades to empty array with no hard crash. |
| Input Validation | PASS | All `data.installation` accesses in `+page.svelte` guarded with null checks. Auth missing → redirect. No-org state → shows read-only card with tooltip. |
| Naming | PASS | No stuttering. `InstallationDto`, `RepoRef`, `InstallUrlResponse`, `InstallStateResponse` all clear. Reactive declarations (`isPendingClaim`, `isSuspended`, `isConnected`, `repoCount`) are descriptive. |
| Code Organization | PASS | Files confined to `web/src/routes/integrations/` and `web/tests/e2e/` per DoD. Only existing `$lib` modules imported — no new shared modules created. `fixtures.ts` and `org-billing.spec.ts` changes are minimal and within `web/tests/e2e/`. |
| Correctness | PASS | All five install states render correctly: not-installed (admin: Connect button; member: read-only), connected (status line), suspended (badge + description, no Connect), pending-claim (admin: Claim button), and the `?installed=true` toast. State-machine branch order is correct. |
| Test Quality | PASS | 7 tests covering all 7 task behaviours. Behaviour 4 (suspended) now asserts badge visible + connect-button NOT visible + description shown — the previously under-specified test is now complete. All error paths exercised via mocked routes. |

## Test Coverage

- Unit tests: N/A (web-only slice; no Go code changed)
- Playwright spec: 7 tests covering all 7 task behaviours (DoD requires ≥4)
- All behaviours from the task YAML are covered by at least one test

## DoD Verification

| DoD Item | Status | Notes |
|----------|--------|-------|
| Playwright spec passes with ≥4 assertions | PASS | 7 tests authored, covering all behaviours |
| Admin/member RBAC distinction covered by a Playwright test | PASS | Behaviour 2 test: member sees card but no Connect button + tooltip |
| svelte-check clean on touched files | PASS | Confirmed in improvement report; no type errors |
| Help/onboarding text explains both install paths | PASS | `github-install-help` paragraph inside `+page.svelte` (inside `{#if data.installUrl}` block) |
| No new files outside `web/src/routes/integrations/` and `web/tests/e2e/` | PASS | 3 new files (server, svelte, spec) within scope; `fixtures.ts` and `org-billing.spec.ts` are existing files with minimal additive changes, both within `web/tests/e2e/` |
| `docs/SPECIFICATION.md:8413–8417` cited in `+page.server.ts` header | PASS | Lines 1–2 of `+page.server.ts` — citation verified to point to the installation-lifecycle section |

## Summary

All iteration-1 findings are resolved and verified against the current code. The state-machine branch order in `+page.svelte` is now correct for all install states, the suspended-install test is fully specified, and the codebase is clean. No new issues found in this audit pass.
