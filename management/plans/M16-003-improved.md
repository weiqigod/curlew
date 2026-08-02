# Improvement Report: M16-003

**Task:** Password reset and email verification endpoints with rate limits and [RequireVerifiedEmail] filter
**Date:** 2026-05-10
**Review:** management/reviews/M16-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `PasswordResetService.ConfirmAsync` committed `consumed_at` + `password_hash` in a transaction, then called `RevokeAllFamiliesForUserAsync` outside it — non-atomic per spec | Moved `RevokeAllFamiliesForUserAsync` inside the `BeginTransactionAsync` block (and the InMemory else-branch) so all three operations are one unit | ✓ tests pass |
| 2 | Medium | `EmailVerificationEndpoints` used anonymous response objects (`new { ok, message }`), producing no `.Produces<T>()` OpenAPI metadata for the 200 paths — violating Definition of Done | Introduced `EmailVerificationResponse` typed record; replaced both anonymous objects; added `.Produces<EmailVerificationResponse>()` to both endpoints | ✓ tests pass |
| 3 | Low | `AuthTokenCleanupServiceTests` used `Task.Delay(50ms)` as the sole synchronisation — fragile on a loaded CI machine | Replaced with a `WaitForConditionAsync` polling helper (10 ms poll, 5 s timeout) that waits for the DB assertion condition to be satisfied before asserting; for "keep" tests uses a 100 ms polling window | ✓ tests pass |
| 4 | Low | `EmailVerificationServiceTests` had no explicit test for spec behavior "Given a verified user, when resend is called, all non-consumed tokens are revoked and a new evtk_ token is issued" | Added `ResendAsync_already_verified_user_still_issues_new_token_and_revokes_prior` seeding a user with `emailVerified: true` and an existing unconsumed token | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet build src/ApiTool.Backend.Tests` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1406 passed, 8 skipped — Stripe live-key integration tests) |
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | ≥80% on new code (all new service/filter/cleanup code exercised by unit + integration tests) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `16b27ab3` | fix(auth): make PasswordResetService.ConfirmAsync fully atomic | #1 |
| `dcf45767` | fix(auth): replace anonymous response objects in EmailVerificationEndpoints with typed DTO | #2 |
| `47934fe6` | fix(test): replace fragile Task.Delay(50ms) in AuthTokenCleanupServiceTests with polling | #3 |
| `1443421a` | test(auth): add missing verified-user resend test to EmailVerificationServiceTests | #4 |

## Summary
4/4 findings resolved. 0 deferred.
