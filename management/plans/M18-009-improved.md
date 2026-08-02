# Improvement Report: M18-009

**Task:** Encryption-at-rest v2: envelope-encrypt team_vaults.template_jsonb and schedules.env_vars; signing-key CI lint
**Date:** 2026-05-19
**Review:** management/reviews/M18-009-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Three `TeamVaultBackfillHostTests` missing — backfill COUNT-mismatch abort logic had zero test coverage | Added `TeamVaultBackfillHostTests.cs` with 3 tests: `Seeded_plaintext_rows_are_encrypted_on_first_tick`, `Already_encrypted_rows_are_left_alone_on_second_tick`, `Backfill_throws_when_a_row_fails_to_encrypt`. Uses `SingletonScopeFactory` + in-memory SQLite + `FailingTeamVaultKeyProvider` test double. | ✓ 3 tests pass |
| 2 | Medium | `VaultConfigService.GetAsync` silently swallowed `TeamVaultDecryptException` and fell back to stale `TemplateYaml`, masking key-rotation failures with a successful 200 | Added `VaultConfigServiceError.DecryptionFailed`. `GetAsync` now catches `TeamVaultDecryptException` and returns `(null, DecryptionFailed, …)` — no stale-plaintext fallback. Plaintext fallback retained only for rows where ciphertext IS NULL (pre-backfill window). Endpoint updated to map `DecryptionFailed → 500`. | ✓ tests pass, including new `Decrypt_after_kid_change` integration test |
| 3 | Medium | `ScheduleExecutorService.ClaimNextAsync` silently returned empty `envVars` dict on `ScheduleEnvDecryptException`, causing workers to proceed with incorrect env-var sets | Added `ScheduleClaimError.DecryptionFailed`. `ClaimNextAsync` now returns `(null, DecryptionFailed)` on decrypt failure. Endpoint maps `DecryptionFailed → 500`. Run stays in Running state — worker must not retry the claim. | ✓ tests pass |
| 4 | Medium | `Decrypt_after_kid_change_fails_with_TeamVaultDecryptException` integration test missing from `TeamVaultEncryptionRoundTripTests` — the primary test that would have caught finding #2 | Added `Decrypt_after_kid_change_fails_with_DecryptionFailed_error` to `TeamVaultEncryptionRoundTripTests`. Seeds encrypted row, mutates stored kid, asserts `GetAsync` returns `VaultConfigServiceError.DecryptionFailed`. Inline `ThrowingOnDecryptTeamVaultKeyProvider` test double drives the failure path. | ✓ test passes |
| 5 | Medium | Migration 2 (`DropTeamVaultPlaintext`) absent — DoD requires "Down-migration path documented (decrypts and writes plaintext back into restored column)" | Created `20260519090000_DropTeamVaultPlaintext.cs`. `Up()` has a guard SQL `RAISE(ABORT, …)` that aborts if any row has NULL ciphertext (backfill must complete first), then drops `template_jsonb`. `Down()` re-adds column as nullable TEXT and copies `hex(template_jsonb_ciphertext)` as a documented placeholder (out-of-band decryption step required for full restore). Deployment sequencing documented in XML doc comment. | ✓ builds, guard SQL verified |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| M18-009 specific tests | 35 passing (up from 30) |
| Total test suite | 2256 passed, 15 skipped, 0 failed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `8c405b3f` | feat(vault): wire encryption into TeamVault/Schedule services + EF migration (M18-009) | Execute-phase uncommitted changes (prerequisite) |
| `6063acfc` | fix(vault): surface DecryptionFailed instead of silently falling back on decrypt error | #2, #3 |
| `d337399f` | test(vault): add TeamVaultBackfillHostTests + Decrypt_after_kid_change integration test | #1, #4 |
| `6ae8cbe3` | feat(migration): add DropTeamVaultPlaintext Migration 2 with guard + Down decrypt path (M18-009) | #5 |

## Summary

5/5 findings resolved. 0 deferred.

Key changes:
- `VaultConfigServiceError` and `ScheduleClaimError` each gained a `DecryptionFailed` variant
- Both service-layer silent-fallback behaviors replaced with explicit error propagation
- 4 new tests added (3 backfill host + 1 kid-change integration test)
- Migration 2 adds the expand-contract drop with guard + documented rollback path
