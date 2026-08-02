# Code Review: M16-004

**Task:** Web pages for password reset and email verification
**Reviewer:** AI
**Date:** 2026-05-10
**Branch:** feature/M16-004-web-password-reset-email-verification
**Iteration:** 3 (post-improve, second pass)

## Verdict: PASS

## Findings

No findings. All findings from iterations 1 and 2 have been resolved.

## Previous Findings Status

| # | Iteration | Finding | Status |
|---|-----------|---------|--------|
| 1 (iter 1) | 1 | `use:enhance` on client-side-only form in `EmailVerifiedRequiredModal` | FIXED |
| 2 (iter 1) | 1 | `fail(500, { error: 'server_error' })` branch untested in password-reset confirm action | FIXED |
| 3 (iter 1) | 1 | `{ error: 'server_error' }` branch untested in email-verification confirm load | FIXED |
| 4 (iter 2) | 2 | `client.test.ts` lacked direct tests for new RFC 7807 parsing paths (`problemType`, `detail` → description, `title` fallback) | FIXED — three new test cases added at lines 110–178 of `client.test.ts`; `client.test.ts` now has 11 tests |
| 5 (iter 2) | 2 | Empty-password path through `missing_field` guard in password-reset confirm action untested | FIXED — test case added at line 729 of `page-server.test.ts` with `{ token: 'prst_xxx', new_password: '' }` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return wrapped errors or `fail()` with structured data. Enumeration-defense catch-alls in request actions are documented and intentional. No swallowed errors that affect user-visible state. |
| Input Validation | PASS | Missing token → `fail(400, missing_field)`. Missing password → `fail(400, missing_field)`. Empty email covered by enumeration defense. Backend is authoritative for all other constraints. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `AuthOkResponse`, `authApi`, `EmailVerifiedRequiredModal`, `isEmailNotVerified` all follow conventions. Interface names appropriate. |
| Code Organization | PASS | Auth routes live outside `(app)` group (correct for unauthenticated flows). `$lib/api/auth.ts` is a narrow, well-scoped module. `ApiError.problemType` extension is additive and non-breaking. Package boundaries respected. |
| Correctness | PASS | Token extraction, server-side auto-confirm on email-verification load, redirect targets, and error branching on `problemType` are all correct. `score` field flows through `details` correctly from RFC 7807 body. Modal state reset on `close()` is correct. `portalLoading` left `true` on success is intentional (browser navigates away). |
| Test Quality | PASS | All three RFC 7807 parsing paths now have direct tests in `client.test.ts`. Both guard branches (`!token`, `!password`) in password-reset confirm action are tested. All 8 task behaviors have test coverage. |

## Test Coverage

- **Unit (vitest):** 18 test files, 182 tests — all pass.
  - `auth.test.ts`: 7 tests (requestPasswordReset, confirmPasswordReset 200/422/400, resendEmailVerification, confirmEmailVerification 200/400)
  - `client.test.ts`: 11 tests including 3 new RFC 7807 parsing tests
  - `page-server.test.ts`: 53 tests including 18 auth page server tests (all action/load branches covered)
- **E2E (Playwright):** 5 password-reset specs + 4 email-verification specs + 1 billing modal spec — cover happy paths, error paths, and missing-token edge cases.
- **Estimated unit coverage on new code:** ≥90% — all action branches, load branches, and client parsing paths are directly tested.

## Spec Compliance Check

All 8 behaviors from the task YAML are covered:

| Behavior | Coverage |
|----------|----------|
| 1. password-reset/request POSTs and shows generic confirmation | `auth-password-reset.spec.ts` test 1 + `page-server.test.ts` |
| 2. password-reset/confirm weak password shows 422 score inline | `auth-password-reset.spec.ts` test 3 + `page-server.test.ts` |
| 3. password-reset/confirm strong password redirects to /login | `auth-password-reset.spec.ts` test 2 + `page-server.test.ts` |
| 4. password-reset/confirm invalid token shows 400 + request-new-link | `auth-password-reset.spec.ts` test 4 + `page-server.test.ts` |
| 5. email-verification/request POSTs and shows generic confirmation | `auth-email-verification.spec.ts` test 1 + `page-server.test.ts` |
| 6. email-verification/confirm valid token auto-redirects | `auth-email-verification.spec.ts` test 2 + `page-server.test.ts` |
| 7. billing 403 email-not-verified opens resend modal | `org-billing.spec.ts` test 8 |
| 8. new auth pages reachable (no broken links) | E2e navigation in all 9 new route specs |

## Summary

All findings from iterations 1 and 2 have been correctly resolved. The RFC 7807 parsing paths in `client.ts` are now directly tested in `client.test.ts` (3 new test cases covering `detail`-as-description, `title`-fallback, and type-only bodies). The empty-password guard branch is covered. All 182 unit tests pass; the Go CI gate passes cleanly. The implementation is complete, correct, and well-tested.
