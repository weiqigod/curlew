# Code Review: M16-002

**Task:** Token tables and templates for password reset and email verification
**Reviewer:** AI
**Date:** 2026-05-07
**Branch:** feature/M16-002-password-reset-email-verification-tables
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No error-handling paths exist in this slice — pure data-layer entities, migration, and static inventory. No swallowed errors. |
| Input Validation | PASS | No external input consumed; validation is deferred to M16-003/M16-008 (endpoint slices). EF Core constraints enforce nullability and required columns at the data layer. |
| Naming | PASS | All exported types, properties, and DbSet accessors carry XML doc comments. No stuttering. C# PascalCase properties map to snake_case column names via `HasColumnName`. Package names are correct. |
| Code Organization | PASS | Entities are leaf types in `Data/Entities/`. DbContext wiring is co-located in `AppDbContext.cs`. Inventory is in `Notifications/Email/EmailTemplateInventory.cs`. No circular dependencies. No reaching into package internals. Single responsibility per class. |
| Correctness | PASS | Migration `Up`/`Down` is atomic (two tables + one column in `Up`, dropped cleanly in `Down`). Partial index filter uses double-quoted column names consistent with `idx_signing_keys_current` and `idx_github_installations_org` precedents. `Database.IsNpgsql()` guard on `inet` type follows the established pattern from `PrCheck.AnnotationsJson`. FK cascade deletes are appropriate. `IsDescending(false, true)` on composite index is supported by EF Core 9.0.4. |
| Test Quality | PASS | 7 entity tests (4 for `PasswordResetToken`, 3 for `EmailVerificationToken`) cover persist/round-trip, unique index enforcement, and partial index DDL existence. `User_email_verified_column_defaults_false` covers behavior #3. 7 parameterized theories × 8 slugs in `EmailTemplateInventoryManifestTests` cover manifest loading, MJML compilation, variable allowlists, test_data key alignment, unknown variable rejection, placeholder-copy header, and full variable usage in MJML. All 7 task behaviors are covered. |

## Test Coverage
- Go gate: all packages >= 80% (lowest: 81.4% on `cmd/apitest`)
- Backend gate: 1322 passed, 0 failed, 8 skipped (Stripe integration tests — pre-existing skip)
- New entity tests: 74 targeted tests pass
- Coverage on new code: entity classes and inventory at effectively 100% (all properties and configuration paths exercised); `AppDbContext.cs` new config blocks exercised by migration tests

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| #1: password_reset_tokens columns | `PasswordResetTokenEntityTests.Entity_persists_and_round_trips` | PASS |
| #2: email_verification_tokens columns | `EmailVerificationTokenEntityTests.Entity_persists_and_round_trips` | PASS |
| #3: users.email_verified DEFAULT false | `PasswordResetTokenEntityTests.User_email_verified_column_defaults_false` | PASS |
| #4: migration rollback | Observable-only (consistent with all prior migrations) | PASS |
| #5: Slugs contains password_reset and trial_expiring | `EmailTemplateInventoryTests.Inventory_matches_spec` + `Manifest_variables_match_spec_allowlist` | PASS |
| #6: manifest variables match spec | `EmailTemplateInventoryManifestTests.Manifest_variables_match_spec_allowlist` | PASS |
| #7: partial index on expires_at WHERE consumed_at IS NULL AND revoked_at IS NULL; unique index on token_hash | `Active_partial_index_exists_on_sqlite` + `Token_hash_unique_index_prevents_duplicate` | PASS |

## Summary

The implementation is technically correct and faithfully follows the spec. All 1322 backend tests pass with no failures. The single Low finding from iteration 1 (behavior #7 incorrectly describing the partial index column as `token_hash` instead of `expires_at`) was correctly resolved in the `/improve` pass by updating `management/tasks/M16-002.yaml`. The code, tests, templates, and task documentation now all agree with each other and with the spec.
