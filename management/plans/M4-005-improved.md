# Improvement Report: M4-005

**Task:** Web: team test results dashboard page
**Date:** 2026-04-15
**Review:** management/reviews/M4-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Tier-redirect test only asserted URL change, not that the `toast-team-tier-required` element was visible. Also, the redirect destination `/org/[slug]` 404ed, preventing the layout (and toast) from rendering. | Created stub `/org/[slug]/+page.svelte` and `/org/[slug]/+page.server.ts` so the redirect destination renders the app layout. Updated the tier-redirect test to also intercept per-org detail calls, then added `await expect(page.getByTestId('toast-team-tier-required')).toBeVisible()` as a required assertion. | ✓ svelte-check + build + unit tests pass |
| 2 | Medium | No E2E or unit test asserted that `data-testid="subnav-results-link"` is visible for team-tier orgs and absent for non-team-tier orgs, despite the implementation being present in `+layout.svelte`. | Added `await expect(page.getByTestId('subnav-results-link')).toBeVisible()` to the happy-path (team-tier) test. Added `await expect(page.getByTestId('subnav-results-link')).not.toBeVisible()` to the tier-redirect test. | ✓ svelte-check + build + unit tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| `npm run check` (svelte-check) | PASS |
| `npm run lint` (eslint) | PASS |
| `npm run test:unit` (Vitest, 43 tests) | PASS |
| `npm run build` | PASS |
| Go Coverage | 87.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ec070c2 | fix(web): add missing E2E assertions for toast and subnav visibility | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
