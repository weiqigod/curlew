# Improvement Report: M16-004

**Task:** Web pages for password reset and email verification
**Date:** 2026-05-10
**Review:** management/reviews/M16-004-review.md
**Iteration:** 2

## Resolved Findings

### Iteration 1 Findings (all fixed in iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `use:enhance` applied to client-side-only form in `EmailVerifiedRequiredModal.svelte` caused a spurious 405 POST to the billing page (no server actions) alongside the correct `handleResend` fetch | Removed `import { enhance }` and `use:enhance` directive; `on:submit\|preventDefault={handleResend}` is the sole and correct handler | ✓ tests pass, lint clean, svelte-check 0 errors |
| 2 | Medium | `fail(500, { error: 'server_error' })` branch in password-reset confirm action (`+page.server.ts:43`) was untested | Added test: mock `authApi.confirmPasswordReset` to reject with `new Error('network failure')`, assert result matches `{ status: 500, data: { error: 'server_error' } }` | ✓ tests pass |
| 3 | Medium | `{ error: 'server_error' }` branch in email-verification confirm load (`+page.server.ts:24`) was untested | Added test: mock `authApi.confirmEmailVerification` to reject with `new Error('network failure')`, assert result matches `{ error: 'server_error' }` | ✓ tests pass |

### Iteration 2 Findings (fixed in this iteration)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `client.ts` was extended with RFC 7807 `problemType` extraction (line 71), `detail` field parsing (line 75), and `title` fallback (line 76), but `client.test.ts` had zero direct tests for any of these three new parsing paths | Added three test cases in `client.test.ts` under new `describe('RFC 7807 problem-detail parsing')`: (a) 422 with type/detail/score asserts `problemType` and `detail` becomes description; (b) 400 with type/title (no description/detail) asserts `title` becomes description; (c) 400 with only `type` asserts `problemType` set and description falls back to statusText | ✓ all 3 new tests pass |
| 2 | Low | Empty-password path of the `!token \|\| !password` guard in password-reset confirm action was untested (only empty-token was covered) | Added test case with `token: 'prst_xxx', new_password: ''` asserting `{ status: 400, data: { error: 'missing_field' } }` | ✓ test passes |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `npm run lint` | PASS |
| `npm run check` (svelte-check) | PASS (0 errors, 0 warnings) |
| `npm run test:unit` | PASS — 182 tests across 18 files |
| Coverage | ~90% on new code (all client parsing paths, all action branches, both guard arms now covered) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `543b8b42` | fix(billing): remove spurious use:enhance from EmailVerifiedRequiredModal | iter 1 #1 |
| `9ec67527` | test(auth): cover server_error fallback branches in confirm page tests | iter 1 #2, #3 |
| `ab8c3393` | test(auth): add direct client RFC 7807 tests and empty-password guard coverage | iter 2 #1, #2 |

## Summary

5/5 findings resolved across 2 iterations. 0 deferred. Tests increased from 176 to 182; all 18 test files pass.
