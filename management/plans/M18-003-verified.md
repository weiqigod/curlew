# Verification Report: M18-003

**Task:** GDPR data inventory: 13-table decision matrix, `[GdprIncluded]`/`[GdprAnonymise]` attributes, assembly scanner
**Verified by:** AI
**Date:** 2026-05-18
**Branch:** feature/M18-003-gdpr-data-inventory
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/apitest` | PASS | Clean build |
| `go test ./...` | PASS | All Go suites green (`./scripts/ci-local.sh --go` clean end-to-end) |
| `go test -race ./...` | PASS | No races |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean (stale `/tmp/apitest_json_XXXXXX.yaml` removed pre-run, per review gate note) |
| `dotnet build ApiTool.Backend.sln` | PASS | 0 Warning(s), 0 Error(s) |
| `dotnet test src/ApiTool.Backend.Tests` (filter `~Gdpr`) | PASS | 80 passed, 1 skipped (M18-005 deferral) |
| `dotnet test src/ApiTool.Backend.Tests` (filter `~GdprInventory`) | PASS | 41 passed, 0 skipped (Observable #1) |
| `dotnet test src/ApiTool.Backend.Tests` (filter `~GdprAttributeScannerTests`) | PASS | 30 passed, 1 skipped (Observable #3) |
| Full backend suite (Stripe excluded) | PASS | 1900 passed, 3 skipped, 0 failed |
| Coverage on `GdprAttributeScanner.cs` | 98.55% line / 77.40% branch | >> 80% line DoD bar |
| Coverage on `GdprBundleManifest.cs` | 92.30% line / 100% branch | Above DoD bar |
| Coverage on `GdprManifestEntry.cs` (records) | 100% / 100% | — |

### Gate-note on `./scripts/ci-local.sh` auto-scope

The auto-scoped invocation (`run_backend=1, run_e2e=1` because `src/ApiTool.Backend/**` changed) fails at `=== test-stack up ===` because the `docker compose` plugin isn't available in this sandbox (`docker: unknown command: docker compose`). This is the same environmental gap the round-3 review flagged in its "Gate Note" — the Stripe integration tests and the docker-compose-orchestrated E2E suite are blocked by missing tooling, not by code changes. M18-003 ships pure C# + docs with **zero Go-file edits** (`git diff --name-only main...HEAD` shows none) and **zero backend-runtime wiring** (Plan Decision 10 — no DI registration, no EF migration, no Stripe-touching code). The Go gate runs clean end-to-end (`./scripts/ci-local.sh --go` → `=== ci-local PASS ===`); the .NET backend suite runs clean end-to-end excluding the Stripe-tagged tests; the GDPR-specific tests (the relevant gate for this slice) are fully green.

## Observable Output

From task YAML:

```
$ dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~GdprInventory"
Passed!  - Failed:     0, Passed:    41, Skipped:     0, Total:    41

$ test -f docs/security/data-inventory.md && grep -c '^| `' docs/security/data-inventory.md
13

$ dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~GdprAttributeScannerTests"
Passed!  - Failed:     0, Passed:    30, Skipped:     1, Total:    31
```

Expected: >= 10 GdprInventory tests pass; doc exists with >= 13 table rows; scanner manifest names all 13 tables.
Result: MATCH on all three. The scanner manifest (verified by `Manifest_contains_exactly_thirteen_tables` + `Manifest_records_expected_disposition` 13-row theory) names all 13 enumerated tables with the expected dispositions.

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Scanner emits one entry per `(Entity, Property, Disposition)` triple | `Manifest_contains_exactly_thirteen_tables`, `Entries_are_ordered_alphabetically_by_entity_name`, `Manifest_records_expected_disposition` | PASS |
| 2 | All 13 enumerated entities appear with a disposition drawn from the four-value set | `Manifest_records_expected_disposition` (13-row theory) | PASS |
| 3 | `data-inventory.md` carries 6 columns and >= 13 rows | `Doc_has_expected_column_headers`, `Doc_has_thirteen_data_rows` | PASS |
| 4 | New user-attributable entity without GDPR attributes fails CI naming the entity | `GdprInventoryCoverageTests.Every_user_attributable_entity_is_tagged` theory | PASS |
| 5 | `[GdprAnonymise]` on `Guid?` recorded as anonymisable-to-null | `OrganizationMember_InvitedBy_is_anonymisable`; deferred non-nullable-`Guid` columns tracked by the M18-005 skipped test | PASS |
| 6 | `[GdprAnonymise]` on `string` recorded as anonymisable-to-token | `Audit_log_actor_email_anonymises_to_deleted_user_token` | PASS |
| 7 | Export builder consumes `InExport` subset (no second list) | `InExport_subset_has_six_entries`, `InExport_does_not_contain_excluded_tables` (manifest is the single source — M18-004 will read it directly) | PASS |
| 8 | Doc-vs-code consistency: every doc row matches scanner disposition | `Doc_row_dispositions_match_scanner_manifest`, `Doc_InExport_column_matches_disposition`, `Doc_InDeletion_column_matches_disposition` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass (>=10 tests) | 80 GDPR tests passed, 1 skipped (tracked M18-005 deferral) | PASS |
| 2 | Observable command finds the inventory doc with >= 13 table rows | `grep -c '^\| \`'` → `13` | PASS |
| 3 | Test coverage >= 80% on GdprAttributeScanner.cs | 98.55% line | PASS |
| 4 | No build warnings or lint errors | `dotnet build` → 0 Warning(s), 0 Error(s); `golangci-lint run` clean | PASS |
| 5 | `docs/security/data-inventory.md` committed and cross-linked from `docs/COMPLIANCE.md` | Both files present; `ComplianceMd_exists` + `ComplianceMd_links_to_data_inventory` tests pin the cross-link | PASS |
| 6 | CHANGELOG.md entry references v4-4 | `### Added` entry in `## [Unreleased]` references "(M18-003, v4-4)" | PASS |
| 7 | CI guard `GdprInventoryCoverageTests` passes against the current entity set | Theory parameterised over every `DbSet<T>` on `AppDbContext`; all rows green | PASS |
| 8 | Doc-vs-code consistency test asserts every table row in `data-inventory.md` matches the scanner output disposition | `Doc_row_dispositions_match_scanner_manifest` (manifest ↔ markdown both directions) | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling (`%w`/exception wrapping with context) | PASS |
| Input validation (null assembly throws, doc parser validates header/cell count) | PASS |
| Naming conventions (`SetNull`/`DeletedUserToken`, `-Attribute` BCL suffix) | PASS |
| Code organisation (`Data/GdprAttributes/` vs `Compliance/Gdpr/` split) | PASS |
| Correctness (table-disposition = min-over-columns; reflection ordering deterministic; namespace bound to `typeof(User).Namespace`) | PASS |
| Test quality (specific assertions, table-driven theory across 13 dispositions, EF-model cross-check via `Table_names_match_AppDbContext_ToTable_calls`) | PASS |

Branch A path: `management/reviews/M18-003-review.md` (Round 3) recorded verdict PASS after two prior review-and-improve cycles (10 findings round 1, 5 findings round 2 — all resolved). Spot-check:
1. **Error wrapping** — `GdprInventoryDocConsistencyTests.ParseTableRows` `InvalidOperationException` carries doc path + offending row context; `GdprAttributeScanner.Scan(null)` throws `ArgumentNullException` with parameter name. PASS.
2. **Exported symbol doc comments** — `GdprDisposition` four enum values each carry an XML-doc `<summary>` describing the disposition's effect on export/deletion. PASS.
3. **Random test verifies what it claims** — `Audit_log_actor_email_anonymises_to_deleted_user_token` reads the manifest's `OrganizationAuditLogEntry` column entry for `ActorEmail` and asserts `AnonymiseAs.DeletedUserToken`; not a no-error smoke test. PASS.

## Commits

| Hash | Message |
|------|---------|
| `8d1719d3` | chore(task): mark M18-003 status as review |
| `1d9bd540` | docs(review): add passing review for M18-003 |
| `dc470811` | docs(review): add round-2 improvement report for M18-003 |
| `6ee29eca` | fix(gdpr): address round-2 review findings on M18-003 |
| `b31d5151` | docs(review): add second-round review with findings for M18-003 |
| `94a54925` | docs(review): add improvement report for M18-003 |
| `32c7a5f8` | fix(gdpr): tighten tests, validate input, sort inventory doc alphabetically |
| `8e04568b` | refactor(gdpr): rename AnonymiseAs.NullOrToken to SetNull and InDeletionAnonymise to TablesAnonymisedOnDeletion |
| `95816957` | docs(review): add review with findings for M18-003 |
| `901c6e90` | chore(task): mark M18-003 as review |
| `fc6e1dad` | docs(changelog): add M18-003 GDPR data inventory entry |
| `04f209e1` | docs(compliance): add GDPR data inventory (13-table decision matrix) and COMPLIANCE.md stub |
| `e51b34af` | test(compliance): add failing tests for GDPR inventory doc consistency |
| `1537a975` | feat(compliance): add GdprInventoryCoverageTests CI guard |
| `39bcdc1c` | feat(compliance): implement GdprAttributeScanner, GdprBundleManifest, GdprManifestEntry |
| `adca9e0f` | test(compliance): add failing tests for GdprAttributeScanner and GdprBundleManifest |
| `062d2831` | feat(compliance): apply GDPR attributes to 13 user-attributable entities |
| `80876462` | feat(compliance): implement GDPR attribute types and disposition enums |
| `6b604672` | test(compliance): add failing tests for GDPR attribute types |
| `98410caa` | chore(task): mark M18-003 as in_progress |
| `00621363` | chore(task): mark M18-003 as planned |
| `da5355db` | docs(plan): add implementation plan for M18-003 |

All commits carry `Refs: M18-003`. TDD pattern visible: `test(compliance): add failing tests for X` precedes `feat(compliance): implement X` for all three implementation steps.

## Files Changed

| File | Action |
|------|--------|
| `docs/COMPLIANCE.md` | created |
| `docs/security/data-inventory.md` | created |
| `CHANGELOG.md` | modified |
| `src/ApiTool.Backend/Compliance/Gdpr/GdprAttributeScanner.cs` | created |
| `src/ApiTool.Backend/Compliance/Gdpr/GdprBundleManifest.cs` | created |
| `src/ApiTool.Backend/Compliance/Gdpr/GdprManifestEntry.cs` | created |
| `src/ApiTool.Backend/Data/GdprAttributes/AnonymiseAs.cs` | created |
| `src/ApiTool.Backend/Data/GdprAttributes/GdprAnonymiseAttribute.cs` | created |
| `src/ApiTool.Backend/Data/GdprAttributes/GdprDisposition.cs` | created |
| `src/ApiTool.Backend/Data/GdprAttributes/GdprIncludedAttribute.cs` | created |
| `src/ApiTool.Backend/Data/GdprAttributes/GdprTableAttribute.cs` | created |
| `src/ApiTool.Backend/Data/Entities/{User,RefreshToken,OrganizationMember,EmailVerificationToken,PasswordResetToken,NotificationRule,CustomRole,Schedule,CoordinatorJob,TeamVault,OrganizationAuditLogEntry,GithubInstallation,GitLabInstallation}.cs` | modified — applied property/class-level GDPR attributes |
| `src/ApiTool.Backend/Data/Entities/{Organization,Result,OrganizationInvitation,Trial}.cs` | modified — class-level `[GdprTable(NotUserAttributable)]` opt-outs (Decision 6) |
| `src/ApiTool.Backend.Tests/Compliance/Gdpr/GdprAttributesTests.cs` | created |
| `src/ApiTool.Backend.Tests/Compliance/Gdpr/GdprAttributeScannerTests.cs` | created |
| `src/ApiTool.Backend.Tests/Compliance/Gdpr/GdprInventoryCoverageTests.cs` | created |
| `src/ApiTool.Backend.Tests/Compliance/Gdpr/GdprInventoryDocConsistencyTests.cs` | created |
| `management/{tasks/M18-003.yaml,backlog.yaml,plans/M18-003-plan.md,plans/M18-003-improved.md,plans/M18-003-improved-round2.md,reviews/M18-003-review.md}` | task-management bookkeeping |

## Issues Found
None. Round 3 review verdict was already PASS; spot-check did not surface new findings.

## Recommendation
PASS — ready for PR and merge.
