# Improvement Report: M14-020

**Task:** Web: Connect GitHub install entry point + install-state view
**Date:** 2026-05-06
**Review:** management/reviews/M14-020-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Claimed+suspended install fell through to admin "Connect GitHub" branch, showing the Connect button instead of a "unsuspend on GitHub" message. | Added a `{:else if isSuspended && data.installation}` branch in `+page.svelte` before the admin-connect branch. Renders `github-suspended-description` element with correct guidance; Connect button is not shown. | ✓ tests pass |
| 2 | High | Behaviour 4 Playwright test only asserted badge visibility — did not assert Connect button was hidden or suspension description was shown; bug in #1 was undetected. | Extended the suspended test: mocks install-url endpoint, asserts `github-connect-button` is NOT visible, asserts `github-suspended-description` is visible with correct text. | ✓ tests pass |
| 3 | Medium | Help text ("Two ways to connect…") rendered inside the admin branch even when `installUrl` was null, referencing "this button" when no button existed. | Moved the help paragraph inside the `{#if data.installUrl}…{/if}` block so it only renders alongside the Connect button. | ✓ tests pass |
| 4 | Low | `InstallationDto` was unnecessarily exported from `+page.server.ts`; the only public contract needed is the `load` return type via `./$types`. | Removed the `export` keyword — interface is now module-private. | ✓ tests pass |
| 5 | Low | `context.unroute('**/api/v1/organizations')` in the member test lacked `.catch(() => {})`, inconsistent with project pattern in `org-roles.spec.ts:261`. | Added `.catch(() => {})` to the `unroute` call. | ✓ tests pass |
| 6 | Low | `SEEDED_ORG_ID = 'org_test'` was a duplicated local constant in both `github-connect.spec.ts` and `org-billing.spec.ts` instead of being shared from fixtures. | Exported `SEEDED_ORG_ID` from `web/tests/e2e/helpers/fixtures.ts`; updated both spec files to import it and removed the local declarations. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `npm run check` (svelte-check) | PASS (0 errors, 0 warnings) |
| `npm run lint` (eslint) | PASS |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| cf5f293e | fix(integrations): resolve suspended-install render gap and test gaps | #1, #2, #3, #4, #5, #6 |

## Summary

6/6 findings resolved. 0 deferred.
