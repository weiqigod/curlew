# Code Review: M14-001

**Task:** Backend: signing_keys table + IKeyProvider with File and GoogleKMS providers
**Reviewer:** AI
**Date:** 2026-05-04
**Branch:** feature/M14-001-signing-keys-key-provider
**Iteration:** 5 (verifying iteration-4 fix)

## Verdict: PASS

## Findings

None.

## Iteration 4 Finding — Verified Status

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `ISigningKeyRotator.cs` class-level `<summary>` still contained the false claim "and provisions a new `<c>next</c>` key." | Removed in commit `b7075585` | ✓ Confirmed: `ISigningKeyRotator.cs` class summary now reads "Orchestrates the key rotation lifecycle: promotes `<c>next</c>` to `<c>current</c>`, demotes the old `<c>current</c>` to `<c>verifying</c>` (or `<c>revoked</c>` in emergency)." — no false claim remains |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Errors wrapped with context; sentinel `InvalidKidFormatException` used correctly; `DbUpdateException` caught in bootstrap paths with retry-read; `CryptographicOperations.FixedTimeEquals` for timing-safe secret comparison; explicit null guard for null `KmsKeyId` with clear `InvalidOperationException` message |
| Input Validation | PASS | Kid format validated via `KeyId.IsValid` allowlist regex before any DB or filesystem access; all nil/empty/malformed inputs handled; path traversal characters rejected; `isRetry` guard prevents infinite PEM re-bootstrap loop |
| Naming | PASS | No stuttering; all exported symbols have doc comments; package/class names follow conventions; `IKeyProvider` has exactly 3 methods per spec; `GetKeyForSigningAsync` retained on concrete types only |
| Code Organization | PASS | `internal/` package boundaries respected; single responsibility per class (`IKeyProvider` / `ISigningKeyStore` / `ISigningKeyRotator` separated cleanly); `BouncyCastleEs256` encapsulates crypto; no circular dependencies |
| Correctness | PASS | Emergency rotation with no staged key: revokes current and bootstraps fresh (new kid distinct from compromised one); rotation order (demote current first, promote next second) avoids partial-index collision; `InternalAccessFilter` returns 404 (not 403) to prevent endpoint fingerprinting |
| Test Quality | PASS | All 7 task behaviors covered by tests; table-driven theory for kid validation (8 cases); integration tests exercise real HTTP endpoints; `InternalAccessFilterTests` covers all access-control branches; `SigningKeyRotatorTests` covers the "no next key + emergency" edge case added in iteration 3; 662 tests all passing |

## Test Coverage

- **Go coverage:** Not applicable (backend-only task). Go gate: PASS (87.7% total, all packages ≥ 76.8%).
- **Backend task coverage:** All 7 behaviors from the task YAML are covered by at least one test.
- **Test count (new in M14-001):** ~39 test cases across `KeyIdTests` (10), `EfSigningKeyStoreTests` (5), `FileKeyProviderTests` (8), `SigningKeyRotatorTests` (5), `GoogleKmsKeyProviderTests` (4), `InternalKeysEndpointsTests` (2), `InternalAccessFilterTests` (5) — well above the DoD minimum of 10.
- **Missing coverage areas:** None that map to task behaviors.

## Summary

The pre-audit gate passes cleanly (662/662 tests, `ci-local.sh --go` PASS, 0 lint issues). The single Low finding from iteration 4 — a false doc claim ("provisions a new next key") in the `ISigningKeyRotator` class summary — was removed in commit `b7075585`. The class summary now accurately describes the rotation semantics. No findings remain across all six review dimensions. The implementation is complete and correct.
