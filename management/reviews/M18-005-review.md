# Code Review: M18-005

**Task:** GDPR deletion state machine: PendingDeletionAt + AnonymisedAt columns, re-auth, UserDeletionFinalizerHost, emails
**Reviewer:** AI
**Date:** 2026-05-18
**Branch:** feature/M18-005-gdpr-deletion-state-machine
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. Both issues from iteration 1 have been resolved.

## Resolved Since Iteration 1

| # | Severity | Finding | Fix Verified |
|---|----------|---------|-------------|
| 1 | Medium | `UserDeletionFinalizerHostTests.cs` line 176 used reflection to verify `TickOnceAsync` is public rather than calling the endpoint via HTTP. | `src/ApiTool.Backend.Tests/Internal/InternalRunDeletionFinalizerEndpointTests.cs` added with 4 integration tests (`Endpoint_returns_200_with_ticked_true`, `Endpoint_sets_anonymised_at_for_users_past_30_day_cooldown`, `Endpoint_enqueues_account_deletion_completed_email`, `Endpoint_is_registered_in_Testing_environment`). Each calls `POST /api/v1/internal/test-hooks/run-deletion-finalizer` via `HttpClient`, seeds real user rows, and asserts DB state and `RecordingEmailQueue`. Matches the M18-002/M18-004 pattern. ✓ |
| 2 | Low | `UserDeletionFinalizerHost.cs`: `clock.GetUtcNow().Date` (Kind=Unspecified) mixed with `now.UtcDateTime` (Kind=Utc) in schedule calculation. | Line 87 changed to `now.UtcDateTime.Date.AddHours(3)` — `next03` is now `Kind=Utc` throughout. ✓ |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return typed problem-detail results with machine-readable `code` extensions. `ConsumeAsync` returns `ReauthError` enum — no exceptions for expected failures. `TickOnceAsync` catches per-user exceptions and continues (with `OperationCanceledException` correctly not swallowed). |
| Input Validation | PASS | `ConsumeAsync` guards blank/null/wrong-prefix tokens. `RequestDeletion` checks for missing `X-Reauth-Token`. `IssueAsync` treats null `PasswordHash` (SSO-only users) as `WrongPassword`. `AlreadyPending` guard prevents duplicate deletion requests. |
| Naming | PASS | No stuttering. All exported types and interfaces have XML doc comments. `DeletionProblems` is correctly `internal static`. `drto_` prefix mnemonic documented. DTOs (`UserDeletionRequestDto`, `UserDeletionStatusDto`) are unambiguous. |
| Code Organization | PASS | `DeletionProblems` is `internal` to the Gdpr namespace. Service, host, and endpoint each own their domain. Stub correctly separated from interface. All three endpoints registered in `Program.cs`. Test-hook endpoint correctly guarded by `!env.IsDevelopment() && !env.IsEnvironment("Testing")`. |
| Correctness | PASS | Single-use token semantics enforced atomically (`ConsumedAt = now`). `pending_deletion_at IS NULL` guard prevents duplicate requests. Email snapshotted before anonymisation. Background service guards with env check. `next03` DateTime.Kind mismatch fixed. `OperationCanceledException` not swallowed. |
| Test Quality | PASS | 38 tests across M18-005 scope (well above ≥14 DoD floor). All 9 spec behaviors covered. New `InternalRunDeletionFinalizerEndpointTests.cs` provides the missing HTTP-level integration coverage for the test-hook endpoint. Reflection test retained as an additional contract guard. |

## Test Coverage

- **Total M18-005 tests:** ~38 passing
- **DeletionReauthServiceTests:** 7 unit tests — all `ReauthError` return values, cross-user token isolation, clock-based expiry
- **ReauthEndpointsTests:** 3 integration tests
- **UserDeletionEndpointsTests:** 13 integration tests — all 3 endpoints and all reauth error codes
- **UserDeletionFinalizerHostTests:** 7 unit tests for `TickOnceAsync`
- **InternalRunDeletionFinalizerEndpointTests:** 4 integration tests (new in iteration 2)
- **AppDbContextSchemaTests:** 2 new tests for schema columns
- **GdprAttributeScannerTests:** M18-005 additions — manifest count, disposition, user-id column
- **EmailTemplateInventoryTests/ManifestTests:** covered by parameterised suite

## Behavior Coverage

| Behavior | Tests |
|----------|-------|
| Fresh re-auth token → 202, `pending_deletion_at` set | `POST_with_fresh_token_returns_202_and_sets_pending_deletion_at`, `POST_response_carries_finalizes_at_30_days_in_future` |
| No `X-Reauth-Token` → 401 `reauth_required` | `POST_without_reauth_token_returns_401_reauth_required` |
| Expired token → 401 `reauth_expired` | `POST_with_expired_reauth_token_returns_401_reauth_expired` |
| Already-consumed token → 401 `reauth_consumed` | `POST_with_consumed_reauth_token_returns_401_reauth_consumed` |
| Cancel pending deletion → clears `pending_deletion_at`, audit event | `POST_cancel_clears_pending_deletion_at_and_emits_audit_event` |
| Finalizer skips users within 30-day cooldown | `Tick_skips_users_whose_pending_deletion_at_is_less_than_30_days_old` |
| Finalizer processes users past 30-day cooldown → sets `anonymised_at` | `Tick_sets_anonymised_at_for_processed_users`, `Endpoint_sets_anonymised_at_for_users_past_30_day_cooldown` |
| Deletion initiated → `account_deletion_initiated` email | `POST_enqueues_account_deletion_initiated_email_with_cancel_url_variable` |
| Deletion finalized → `account_deletion_completed` email | `Tick_enqueues_account_deletion_completed_email`, `Endpoint_enqueues_account_deletion_completed_email` |

## Summary

The implementation is correct, complete, and well-structured. Both findings from iteration 1 are resolved: the internal test-hook endpoint now has a dedicated `InternalRunDeletionFinalizerEndpointTests.cs` integration test suite that exercises the real HTTP path (matching the M18-002/M18-004 pattern), and the `DateTime.Kind` mismatch in the finalizer schedule calculation is corrected. All 9 spec behaviors are covered by tests, the migration is reversible, email templates pass the inventory manifest suite, and the full dotnet test suite passes. No new issues found.
