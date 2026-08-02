# Code Review: M18-009

**Task:** Encryption-at-rest v2: envelope-encrypt team_vaults.template_jsonb and schedules.env_vars; signing-key CI lint
**Reviewer:** AI
**Date:** 2026-05-19
**Branch:** feature/M18-009-encryption-at-rest-v2
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All five findings from review iteration 1 have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `VaultConfigService.GetAsync` correctly returns `VaultConfigServiceError.DecryptionFailed` on `TeamVaultDecryptException`. `ScheduleExecutorService.ClaimNextAsync` correctly returns `ScheduleClaimError.DecryptionFailed` on `ScheduleEnvDecryptException`. Both endpoints map `DecryptionFailed → 500`. Pre-backfill plaintext fallback is confined to rows where `TemplateJsonCiphertext IS NULL`. All other error paths wrapped with context. |
| Input Validation | PASS | `EnvelopeCodec.Pack` validates nonce/tag/wrappedDek lengths. `FileTeamVaultKeyProvider` and `FileScheduleEnvKeyProvider` validate KEK size (exactly 32 bytes) and presence. Interface signatures accept empty-byte arrays without panic. |
| Naming | PASS | No stuttering. Doc comments on all exported types, interfaces, methods, and constants. `*DecryptException` and `*DecryptionFailed` enum variants follow the existing `GitLabPatDecryptException` pattern. |
| Code Organization | PASS | `EnvelopeCodec` promoted to `ApiTool.Backend.Crypto` namespace as a shared utility. Two independent options classes per subsystem. Fake providers in `TestInfrastructure`. `TeamVaultBackfillHost` tests use inline test doubles (`FailingTeamVaultKeyProvider`, `ThrowingOnDecryptTeamVaultKeyProvider`). |
| Correctness | PASS | DEK zeroing via `CryptographicOperations.ZeroMemory` in `EnvelopeCodec.DecryptWithDek` finally block. Migration 2 (`DropTeamVaultPlaintext`) guards against premature plaintext-column drop with an abort-raising SQL select. `TeamVaultBackfillHost` only processes rows with `TemplateJsonCiphertext IS NULL`. |
| Test Quality | PASS | 35 tests pass. Three `TeamVaultBackfillHostTests` cover: plaintext row encrypted on first tick, already-encrypted rows left untouched on second tick, abort on encrypt failure. `Decrypt_after_kid_change_fails_with_DecryptionFailed_error` covers the service-layer kid-mismatch path. `ScheduleEnvVarsRoundTripTests` covers null-env-vars empty dict, ciphertext opacity, and full round-trip. CI lint bash tests cover all five exit-code paths. |

## Test Coverage
- Tests specific to M18-009: 35 passing (up from 30 in iteration 1) — exceeds the DoD threshold of ≥ 16.
- `EnvelopeCodecTests`: 4 tests
- `FileTeamVaultKeyProviderTests`: 6 tests
- `GoogleKmsTeamVaultKeyProviderTests`: 4 tests
- `FileScheduleEnvKeyProviderTests`: 6 tests
- `GoogleKmsScheduleEnvKeyProviderTests`: 4 tests
- `TeamVaultEncryptionRoundTripTests`: 4 tests (3 original + `Decrypt_after_kid_change`)
- `ScheduleEnvVarsRoundTripTests`: 3 tests
- `TeamVaultBackfillHostTests`: 3 tests (added in improve phase)
- `check-signing-keys_test.sh`: 5 bash unit tests

## Summary

All five findings from iteration 1 have been resolved. The cryptographic core (EnvelopeCodec, both File and KMS providers for both subsystems), service-layer error propagation, backfill host logic, Migration 2 expand-contract drop, and CI lint script are all correct and well-tested. The code meets all project standards.
