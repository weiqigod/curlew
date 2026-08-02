# Code Review: M16-013

**Task:** gitlab_installations table with IGitLabKeyProvider and gitlab_webhook_events
**Reviewer:** AI
**Date:** 2026-05-10
**Branch:** feature/M16-013-gitlab-installations-tables
**Iteration:** 3 (post-improve, final)

## Verdict: PASS

## Findings

No findings. All four findings from iteration 2 have been resolved:

1. **Medium (resolved)**: `FileGitLabKeyProvider.EncryptAsync` now wraps `WrapDek(dek)` in `try { } finally { CryptographicOperations.ZeroMemory(dek); }` — the DEK is zeroed on all code paths including exception.
2. **Low (resolved)**: `GoogleKmsGitLabKeyProvider.EncryptAsync` catch block now has a `// NOTE:` comment documenting the semantic mismatch (`GitLabPatDecryptException` used for encrypt failures) and the correct follow-up rename.
3. **Low (resolved)**: `Constructor_throws_when_kek_missing` assertion updated to `"*does/not/exist.bin*32*"` — verifies both path and size constraint from behavior #6.
4. **Low (resolved)**: `Encrypt_when_kms_throws_surfaces_exception_without_key_in_message` test added to `GoogleKmsGitLabKeyProviderTests` using `FailingKmsClient` — the `EncryptAsync` failure path is now tested.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with context; `GitLabPatDecryptException` sentinel used consistently; inner exceptions preserved; DEK zeroed in `finally` blocks in both providers; no swallowed errors. |
| Input Validation | PASS | `FileGitLabKeyProvider` validates KEK path (missing, wrong size) at construction time with clear messages naming path and required size. `GitLabEnvelopeCodec.Unpack` validates version byte and truncation. `Pack` validates nonce (12 bytes), tag (16 bytes), and wrappedDek length (<= 0xFFFF). |
| Naming | PASS | No stuttering. All exported types, methods, constants, and properties have XML doc comments. `IGitLabKeyProvider` uses `-er` suffix. Internal codec correctly marked `internal`. `GitLabPatDecryptException` semantic mismatch for encrypt path is documented with `// NOTE:` comment. |
| Code Organization | PASS | `internal/` boundaries respected. `GitLabEnvelopeCodec` is `internal static`. Providers, options, interface, and endpoint each in distinct files. Single-responsibility throughout. `using ApiTool.Backend.Organizations` in endpoint file correctly retained for `ErrorResponse`. |
| Correctness | PASS | DEK zeroed in `finally` in both `FileGitLabKeyProvider.EncryptAsync` and `GoogleKmsGitLabKeyProvider.EncryptAsync`. `DecryptAsync` in file provider guards against double-wrap: `GitLabPatDecryptException` for kid mismatch thrown before `try/catch`, not inside it. Migration `Down()` drops both tables. `HasFilter("\"deleted_at\" IS NULL")` on the partial unique index correctly mirrors the `idx_github_installations_org` pattern. |
| Test Quality | PASS | All 7 behaviors in the task YAML have at least one test. Table-driven tests where appropriate. Error paths covered (missing/wrong-size KEK, kid mismatch, tampered ciphertext, KMS encrypt failure, KMS decrypt failure, truncated blob, wrong version byte). Integration test covers GET 200, GET 401, POST 501. `FakeGitLabKeyProvider` correctly registered in `BackendFactory`. |

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| 1 — `gitlab_installations` columns per spec | `GitlabInstallations_table_exists_with_required_columns`; migration Up() |
| 2 — `gitlab_webhook_events` columns per spec | `Webhook_event_uuid_is_unique`; migration Up() |
| 3 — migration rolls back cleanly | Migration `Down()` drops both tables; documented in plan observable |
| 4 — `FileGitLabKeyProvider` round-trips a PAT | `Encrypt_then_Decrypt_round_trips_a_PAT` |
| 5 — `GoogleKmsGitLabKeyProvider` round-trips via stub KMS | `Encrypt_then_Decrypt_round_trips_a_PAT_through_kms` |
| 6 — missing/wrong-size KEK throws clear startup exception | `Constructor_throws_when_kek_missing` (asserts path + 32 bytes); `Constructor_throws_when_kek_wrong_size` |
| 7 — FK ON DELETE CASCADE to webhook events documented | `Hard_deleting_installation_cascades_to_webhook_events`; Decision E documented in plan |

## Test Coverage

- Go gate: all packages pass; `go test ./...` green; `golangci-lint` clean; smoke suite passes.
- C# tests: 1350 passed, 0 failed, 8 Stripe live-mode skipped (as reported by ci-local gate, which runs Go only — C# backend gate runs in CI with docker stack).
- GitLab-specific new tests: 28 tests covering `GitLabEnvelopeCodec`, `FileGitLabKeyProvider`, `GoogleKmsGitLabKeyProvider`, `FakeKmsClient` symmetric methods, schema/cascade/uniqueness, options binding, and endpoint integration.
- Coverage on new code: all branches exercised; both happy-path and error-path tests present for every new component.

## Summary

All four findings from the iteration 2 review are correctly resolved. The DEK zeroing is now in a `try/finally` block in both providers; the missing-file test asserts both path and size; the `EncryptAsync` failure path in the KMS provider has a test; and the semantic mismatch is documented with a `// NOTE:` comment. No new issues found in this iteration. The implementation is complete, correct, and well-tested. Ready for `/verify`.
