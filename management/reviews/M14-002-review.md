# Code Review: M14-002 (Iteration 2)

**Task:** Backend: POST /api/v1/auth/refresh unified mint endpoint
**Reviewer:** AI
**Date:** 2026-05-04
**Branch:** feature/M14-002-auth-refresh-unified-mint

## Verdict: PASS

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Low | Test Quality | `src/ApiTool.Backend.Tests/Licensing/Keys/FakeKeyProvider.cs` | 25–27, 42–44 | `FakeKeyProvider` mutates `_key`, `_kid`, and `_jwks` non-atomically in `SimulateKeyRotation()` without any synchronization (`lock`, `volatile`, or `Interlocked`). C# field writes are not guaranteed to be visible across threads without a memory barrier. In practice the single test that calls `SimulateKeyRotation()` uses it sequentially, so no actual race occurs today. However, if `BackendFactory` is ever shared across parallel test workers this becomes a real data race. | Add a `lock (_sync)` guard to all three field mutations in `SimulateKeyRotation()` and to the reads in `SignAsync`, `GetActiveKidAsync`, and `GetVerificationJwksAsync`. |

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error outcomes returned as discriminated results; no swallowed errors; `LogWarning` emitted on family-root missing anomaly; RFC 7807 problem-details with stable codes. |
| Input Validation | PASS | Handler validates null body, empty `refresh_token`, and `Guid.Empty` device_id before delegation to service. Service handles all null/expired/revoked/device-mismatch cases distinctly. |
| Naming | PASS | No stuttering; doc comments on all exported types and methods; package names correct; single-method interfaces use appropriate naming. |
| Code Organization | PASS | Handler orchestrates, service owns business logic, issuers mint tokens, `JwsBuilder` is internal. No circular dependencies. `using` / `await using` used correctly for transactions and disposable scopes. |
| Correctness | PASS | All 8 task-YAML behaviors implemented and tested. Missing-family-root fallback now logs a warning (finding #1 from iter-1). Null-user race in `RevokeEntireFamilyAsync` now tested (finding #6 from iter-1). `_` default arm in the switch is safe: all 5 non-success outcomes are explicitly handled, `Success` falls through. |
| Test Quality | PASS | All 6 findings from iteration 1 resolved: integration tests for behavior #2 (DB rotation state) and behavior #7 (kid-after-rotation), seed endpoint regression test, and null-user path. 40 new tests; all 8 behaviors covered. Remaining Low finding (#1 above) is test-infrastructure-only and non-blocking. |

## Test Coverage

- Gate: `./scripts/ci-local.sh --go` — PASS (all Go + smoke tests)
- Backend test count for M14-002 files: 40 new facts (13 integration, 9 unit-service, 7 unit-issuer, 3 LicenseIssuer, 2 AccessIssuer, 3 TierConfig, 3 EmailMessage)
- DoD requires >= 12 passing under `FullyQualifiedName~AuthRefresh` — `AuthRefreshEndpointsTests` alone has 13 facts
- All 8 task-YAML behaviors map to at least one test:
  - Behavior 1 (unified mint shape + claim shapes): `POST_auth_refresh_with_valid_token_returns_200_with_unified_mint_shape`, `License_jwt_carries_typ_license_jwt_with_17_claim_shape`, `Access_token_carries_typ_at_jwt_with_9_claim_shape`
  - Behavior 2 (rotation DB state + lifetime clamp): `RotateAsync_happy_path_marks_old_rotated_and_inserts_new_row_with_parent_id`, `RotateAsync_new_row_expires_at_clamped_to_365_days_from_family_root`, `Rotation_marks_old_rotated_and_inserts_new_row_with_correct_family_linkage`
  - Behavior 3 (reuse → family revocation + email): `RotateAsync_reuse_revokes_entire_family_and_enqueues_account_security_alert`, `Reuse_returns_401_AUTH_REFRESH_REUSED_with_problem_json`
  - Behavior 4 (device mismatch): `RotateAsync_with_mismatched_device_id_returns_DeviceMismatch_and_does_not_rotate`, `DeviceMismatch_returns_401_AUTH_DEVICE_MISMATCH_and_does_not_rotate`
  - Behavior 5 (absolute expiry): `RotateAsync_past_expires_at_returns_Expired`, `Expired_token_returns_401_AUTH_REFRESH_EXPIRED`
  - Behavior 6 (trial fields default): `Trial_fields_default_to_none_and_null`
  - Behavior 7 (kid after rotation): `IssueAsync_kid_header_points_at_current_signing_key`, `Active_kid_after_rotation_is_reflected_in_minted_jwt_headers`
  - Behavior 8 (no license/issue route): `Swagger_lists_auth_refresh_endpoint_and_NOT_license_issue`
- Missing coverage: `ResolveUserContextAsync` when membership exists but org has no subscription record (untested org-with-no-subscription path in `AuthRefreshEndpoints.cs`); this is a minor gap that does not affect any task-YAML behavior

## Summary

All 6 findings from iteration 1 have been addressed: the family-root missing path now logs a warning, a unit test guards the null-user race in `RevokeEntireFamilyAsync`, and the three missing integration tests (behavior #2 DB state, behavior #7 kid-after-rotation wiring, seed endpoint regression) have been added. The architecture is clean — handler/service/issuer separation is maintained, JWT shapes match the spec, RFC 7807 problem-details responses carry correct content-type and stable codes, and the `FakeKeyProvider` + `RecordingEmailQueue` test doubles are well-integrated. One Low finding remains in test infrastructure (`FakeKeyProvider` unsynchronized field mutation), acceptable for a test-only singleton used sequentially. The single remaining uncovered path (`ResolveUserContextAsync` with membership-but-no-subscription) does not correspond to any task-YAML behavior and does not block the task.
