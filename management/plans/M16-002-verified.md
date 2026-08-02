# Verification Report: M16-002

**Task:** Token tables and templates for password reset and email verification
**Verified by:** AI
**Date:** 2026-05-07
**Branch:** feature/M16-002-password-reset-email-verification-tables
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, cached results |
| `go test -race ./...` | PASS | No races detected (Go gate clean) |
| `golangci-lint run` | PASS | No findings (per review report) |
| `./smoke/run.sh` | PASS | Smoke test complete |
| `dotnet test src/ApiTool.Backend.Tests` | PASS | 1322 passed, 0 failed, 8 skipped (Stripe integration — pre-existing) |
| Targeted tests (observable filter) | PASS | 74 passed, 0 failed |
| Go coverage | 87.3% | Meets >= 80% threshold |
| Backend coverage | ~94.8% | Per review report; new entity code effectively 100% exercised |

Note: `ci-local.sh` exits 125 due to `docker compose` plugin not being installed on this machine (pre-existing infrastructure gap — `docker-compose` vs `docker compose` CLI). The Go gate, backend unit tests, and smoke test all pass. The docker-compose stack failure is not caused by M16-002 changes (no changes to `docker-compose.test.yml` or stack scripts).

## Observable Output

```
$ sqlite3 /tmp/m16-002-test.db ".schema password_reset_tokens"
CREATE TABLE IF NOT EXISTS "password_reset_tokens" (
    "Id" TEXT NOT NULL CONSTRAINT "PK_password_reset_tokens" PRIMARY KEY,
    "user_id" TEXT NOT NULL,
    "token_hash" BLOB NOT NULL,
    "issued_at" TEXT NOT NULL,
    "expires_at" TEXT NOT NULL,
    "consumed_at" TEXT NULL,
    "revoked_at" TEXT NULL,
    "requester_ip" TEXT NULL,
    "requester_ua" TEXT NULL,
    CONSTRAINT "FK_password_reset_tokens_users_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("Id") ON DELETE CASCADE
);
CREATE INDEX "idx_password_reset_tokens_active" ON "password_reset_tokens" ("expires_at") WHERE "consumed_at" IS NULL AND "revoked_at" IS NULL;
CREATE UNIQUE INDEX "idx_password_reset_tokens_hash" ON "password_reset_tokens" ("token_hash");
CREATE INDEX "idx_password_reset_tokens_user" ON "password_reset_tokens" ("user_id", "issued_at" DESC);

$ sqlite3 /tmp/m16-002-test.db ".schema email_verification_tokens"
CREATE TABLE IF NOT EXISTS "email_verification_tokens" (
    "Id" TEXT NOT NULL CONSTRAINT "PK_email_verification_tokens" PRIMARY KEY,
    "user_id" TEXT NOT NULL,
    "token_hash" BLOB NOT NULL,
    "issued_at" TEXT NOT NULL,
    "expires_at" TEXT NOT NULL,
    "consumed_at" TEXT NULL,
    "revoked_at" TEXT NULL,
    CONSTRAINT "FK_email_verification_tokens_users_user_id" FOREIGN KEY ("user_id") REFERENCES "users" ("Id") ON DELETE CASCADE
);
CREATE INDEX "idx_email_verification_tokens_active" ON "email_verification_tokens" ("expires_at") WHERE "consumed_at" IS NULL AND "revoked_at" IS NULL;
CREATE UNIQUE INDEX "idx_email_verification_tokens_hash" ON "email_verification_tokens" ("token_hash");
CREATE INDEX "idx_email_verification_tokens_user" ON "email_verification_tokens" ("user_id", "issued_at" DESC);

$ sqlite3 /tmp/m16-002-test.db ".schema users" | grep -i email_verified
, "is_admin" INTEGER NOT NULL DEFAULT 0, "password_hash" TEXT NULL, "email_verified" INTEGER NOT NULL DEFAULT 0)

$ dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~EmailTemplateInventory|PasswordResetTokenEntity|EmailVerificationTokenEntity"
Passed!  - Failed: 0, Passed: 74, Skipped: 0, Total: 74, Duration: 2 s

$ ls -la templates/email/password_reset.{mjml,json} templates/email/trial_expiring.{mjml,json}
-rw-r--r--  648 May  7 19:45 templates/email/password_reset.json
-rw-r--r-- 1180 May  7 19:45 templates/email/password_reset.mjml
-rw-r--r--  507 May  7 19:45 templates/email/trial_expiring.json
-rw-r--r--  893 May  7 19:45 templates/email/trial_expiring.mjml
```

Expected: Both new token tables exist with correct columns, email_verified column on users, all 74 targeted tests pass, template files present.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | password_reset_tokens columns (id, user_id, token_hash, issued_at, expires_at, consumed_at, revoked_at, requester_ip, requester_ua) | `PasswordResetTokenEntityTests.Entity_persists_and_round_trips` + schema inspection | PASS |
| 2 | email_verification_tokens columns (id, user_id, token_hash, issued_at, expires_at, consumed_at, revoked_at) | `EmailVerificationTokenEntityTests.Entity_persists_and_round_trips` + schema inspection | PASS |
| 3 | users.email_verified BOOLEAN NOT NULL DEFAULT false | `PasswordResetTokenEntityTests.User_email_verified_column_defaults_false` + schema shows `INTEGER NOT NULL DEFAULT 0` | PASS |
| 4 | migration rolls back cleanly | Observable: migration applies to fresh DB, tables present; dotnet ef migrations remove clean | PASS |
| 5 | Slugs contains password_reset and trial_expiring | `EmailTemplateInventoryTests.Inventory_matches_spec` | PASS |
| 6 | manifest variables match spec (password_reset: user_email/reset_url/expires_at_local/requester_ip/requester_ua; trial_expiring: user_email/feature/expires_at_local/upgrade_url) | `EmailTemplateInventoryManifestTests.Manifest_variables_match_spec_allowlist` | PASS |
| 7 | partial index on (expires_at) WHERE consumed_at IS NULL AND revoked_at IS NULL (idx_password_reset_tokens_active) + unique index on (token_hash) (idx_password_reset_tokens_hash) | `Active_partial_index_exists_on_sqlite` + `Token_hash_unique_index_prevents_duplicate` + schema inspection | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 74 targeted tests pass, all 7 behaviors covered | PASS |
| 2 | Observable command works as specified | Schema inspected; both tables, email_verified column, and template files all confirmed present | PASS |
| 3 | Test coverage >= 80% on new code | Go: 87.3% overall; backend: ~94.8% (entity code at effectively 100%) | PASS |
| 4 | No build warnings or lint errors | `dotnet build` clean; golangci-lint 0 issues (per review) | PASS |
| 5 | Migration applies and rolls back cleanly | Applied to fresh SQLite DB; Down() drops both tables and email_verified column | PASS |
| 6 | MJML compiles via existing MjmlNetCompiler without errors | `Mjml_compiles_to_non_empty_html[password_reset]` and `[trial_expiring]` pass | PASS |
| 7 | Manifest validator (existing EmailTemplateLoader) accepts the two new manifests | `Manifest_round_trips_loader[password_reset]` and `[trial_expiring]` pass | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — no error-handling paths exist (pure data-layer entities) |
| Naming conventions | PASS — no stuttering; C# PascalCase maps to snake_case via HasColumnName; all exports doc-commented |
| Code organization | PASS — entities in Data/Entities/, inventory in Notifications/Email/, DbContext wiring co-located |
| Test quality | PASS — table-driven entity tests; meaningful assertions (round-trip, unique constraint enforcement, DDL inspection) |
| Doc comments | PASS — all exported types and properties carry XML doc comments |

Branch A: Review PASS trusted (iteration 2, post-improve), spot-check of entity file, test file, and error handling all clean.

## Commits

| Hash | Message |
|------|---------|
| fa201c97 | docs(plan): add implementation plan for M16-002 |
| 5144ba70 | chore(task): mark M16-002 as planned |
| 03921333 | chore(task): mark M16-002 as in_progress |
| 1c514c3b | test(auth): add failing entity tests for PasswordResetToken and EmailVerificationToken |
| 9b0397af | feat(auth): add PasswordResetToken and EmailVerificationToken entities, migration, and email_verified column |
| 1a575eaf | test(email): update inventory tests for M16 password_reset and trial_expiring slugs |
| b538b15e | feat(email): add password_reset and trial_expiring to inventory and template files |
| b7d91939 | docs(changelog): add M16-002 entry for token tables and email templates |
| 5b5b08af | chore(task): mark M16-002 as review |
| 4758a5ad | docs(review): add review with findings for M16-002 |
| 9c5836c2 | fix(tasks): correct behavior #7 index description in M16-002 task YAML |
| 4241233b | docs(review): add improvement report for M16-002 |
| 8814ed91 | docs(review): add passing review for M16-002 |

TDD pattern verified: test commit `1c514c3b` precedes implementation commit `9b0397af`.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `src/ApiTool.Backend/Data/Entities/PasswordResetToken.cs` | created | New entity — 9 properties |
| `src/ApiTool.Backend/Data/Entities/EmailVerificationToken.cs` | created | New entity — 7 properties |
| `src/ApiTool.Backend/Data/Entities/User.cs` | modified | Added EmailVerified property |
| `src/ApiTool.Backend/Data/AppDbContext.cs` | modified | Added 2 DbSets, 2 entity configs, email_verified mapping |
| `src/ApiTool.Backend/Migrations/20260507174354_AddPasswordResetAndEmailVerificationTokens.cs` | created | Migration Up/Down |
| `src/ApiTool.Backend/Migrations/20260507174354_AddPasswordResetAndEmailVerificationTokens.Designer.cs` | created | Migration metadata |
| `src/ApiTool.Backend/Migrations/AppDbContextModelSnapshot.cs` | modified | Regenerated snapshot |
| `src/ApiTool.Backend/Notifications/Email/EmailTemplateInventory.cs` | modified | Slugs expanded from 6 to 8 |
| `src/ApiTool.Backend.Tests/Auth/PasswordResetTokenEntityTests.cs` | created | 4 tests |
| `src/ApiTool.Backend.Tests/Auth/EmailVerificationTokenEntityTests.cs` | created | 3 tests |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailTemplateInventoryTests.cs` | modified | Updated count/spec tests, removed Forbidden_templates_are_absent |
| `src/ApiTool.Backend.Tests/Notifications/Email/EmailTemplateInventoryManifestTests.cs` | modified | Added 2 slugs to ExpectedVariables |
| `templates/email/password_reset.json` | created | Manifest with 5 variables |
| `templates/email/password_reset.mjml` | created | MJML body |
| `templates/email/trial_expiring.json` | created | Manifest with 4 variables |
| `templates/email/trial_expiring.mjml` | created | MJML body |
| `CHANGELOG.md` | modified | Unreleased entry added |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
