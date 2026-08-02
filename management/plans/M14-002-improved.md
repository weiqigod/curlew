# Improvement Report: M14-002

**Task:** Backend: POST /api/v1/auth/refresh unified mint endpoint
**Date:** 2026-05-04
**Review:** management/reviews/M14-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `RotateHappyPathAsync` silently falls back to `row.IssuedAt` when family root is null; no logging, no test | Added `LogWarning` when `root is null` in `RefreshTokenService.cs` lines 97-104; added unit test `RotateAsync_with_missing_family_root_falls_back_gracefully_and_succeeds` in `RefreshTokenServiceTests.cs` | ✓ tests pass |
| 2 | Medium | Behavior #2 (DB state after rotation) missing integration-test counterpart in `AuthRefreshEndpointsTests` | Added `Rotation_marks_old_rotated_and_inserts_new_row_with_correct_family_linkage` integration test that verifies `RotatedAt` and new row family/parent linkage via `_factory.Services.CreateScope()` | ✓ tests pass |
| 3 | Medium | Behavior #7 (kid in JWS after key rotation) not covered by an integration test exercising the HTTP handler's DI wiring | Extended `FakeKeyProvider` with `SimulateKeyRotation()` method; exposed `GetFakeKeyProvider()` on `BackendFactory`; added `Active_kid_after_rotation_is_reflected_in_minted_jwt_headers` integration test | ✓ tests pass |
| 4 | Low | Test name `IssueAsync_emits_at_jwt_with_10_claim_shape_and_aud_cli_api` conflicts with spec's "9-claim" terminology — reader confusion | Renamed to `IssueAsync_emits_at_jwt_with_correct_claim_shape_and_aud_cli_api` and added a clarifying comment explaining the count discrepancy | ✓ tests pass |
| 5 | Low | `InternalRefreshSeedEndpoint` at 46% line coverage; no automated integration test | Added `Seed_endpoint_returns_plaintext_and_device_id_usable_by_refresh` integration test that calls `POST /internal/test/seed-refresh` and immediately exercises the returned plaintext via `POST /api/v1/auth/refresh` | ✓ tests pass |
| 6 | Low | User-deleted race in `RevokeEntireFamilyAsync` (null user → empty `To` field) not tested | Added `RevokeEntireFamilyAsync_when_user_row_deleted_does_not_throw_and_enqueues_email` unit test using raw SQL to delete the user row (disabling FK checks) then asserting the service does not throw and email is enqueued with `relogin_url` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Total test count | 706 (up from 698 before improvement) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `0bbd09a1` | fix(auth): log warning on missing family root; test null-user email path | #1, #6 |
| `70745e80` | fix(test): clarify AT claim count ambiguity in AccessTokenIssuerTests | #4 |
| `a80ce705` | fix(test): add missing integration tests for behaviors #2, #7 and seed endpoint | #2, #3, #5 |

## Summary

6/6 findings resolved. 0 deferred.

The three most impactful fixes:
- **Finding #1**: `RefreshTokenService` now emits a `LogWarning` when the family root row is missing, making the data-integrity anomaly observable in production rather than silently computing a wrong lifetime.
- **Findings #2 & #7**: Two task-YAML behaviors that lacked integration-test counterparts now have them: DB-state verification after rotation and the kid-after-rotation wiring test via the new `FakeKeyProvider.SimulateKeyRotation()` capability.
- **Finding #6**: The null-user race in `RevokeEntireFamilyAsync` is now regression-guarded; the service correctly handles a deleted user without throwing.
