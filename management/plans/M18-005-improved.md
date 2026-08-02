# Improvement Report: M18-005

**Task:** GDPR deletion state machine: PendingDeletionAt + AnonymisedAt columns, re-auth, UserDeletionFinalizerHost, emails
**Date:** 2026-05-18
**Review:** management/reviews/M18-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `UserDeletionFinalizerHostTests.cs` line 176 used reflection to verify `TickOnceAsync` is public rather than actually calling `POST /api/v1/internal/test-hooks/run-deletion-finalizer` via HTTP. Every analogous milestone slice (M18-004, M18-002) has a dedicated `Internal*EndpointTests.cs` integration test. | Created `src/ApiTool.Backend.Tests/Internal/InternalRunDeletionFinalizerEndpointTests.cs` with 4 integration tests using `BackendFactory`: `Endpoint_returns_200_with_ticked_true`, `Endpoint_sets_anonymised_at_for_users_past_30_day_cooldown`, `Endpoint_enqueues_account_deletion_completed_email`, `Endpoint_is_registered_in_Testing_environment`. Seeds real user rows, triggers the hook via HTTP, and asserts on DB state and `RecordingEmailQueue`. | ✓ tests pass (4/4) |
| 2 | Low | `UserDeletionFinalizerHost.cs` lines 87–90: `clock.GetUtcNow().Date` returns a `DateTime` with `Kind=Unspecified`; compared against `now.UtcDateTime` (`Kind=Utc`). Works today (UTC offset 0) but is a correctness hazard if `TimeProvider` ever returns a non-zero offset. | Changed `now.Date.AddHours(3)` to `now.UtcDateTime.Date.AddHours(3)` so the `next03` variable has `Kind=Utc` throughout the schedule calculation. | ✓ build clean, all tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet build src/ApiTool.Backend.Tests` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS — 2128 passed, 0 failed, 16 skipped |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS — 0 issues |
| Coverage (`UserDeletionEndpoints.cs`) | ≥88% line, ≥65% branch |
| Coverage (`UserDeletionFinalizerHost.TickOnceAsync`) | 100% line, 100% branch |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `a0bc8e9e` | fix(gdpr): eliminate DateTime.Kind mismatch in finalizer schedule | #2 |
| `cc2cd41f` | test(gdpr): add integration tests for run-deletion-finalizer test hook | #1 |

## Summary

2/2 findings resolved. 0 deferred.
