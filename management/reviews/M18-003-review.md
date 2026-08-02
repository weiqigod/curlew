# Code Review: M18-003 (Round 3)

**Task:** GDPR data inventory: 13-table decision matrix, `[GdprIncluded]`/`[GdprAnonymise]` attributes, assembly scanner
**Reviewer:** AI
**Date:** 2026-05-18
**Branch:** feature/M18-003-gdpr-data-inventory

## Verdict: PASS ✓

No findings. After two prior review rounds (10 findings round 1, 5 findings round 2 — all resolved per `M18-003-improved.md` and `M18-003-improved-round2.md`), the code is in shippable shape.

The round-2 findings were addressed cleanly:
- **#1 (`SetNull` precondition).** The deferral to M18-005 is now visible in three coupled places: the `AnonymiseAs.SetNull` XML-doc, a `NOTE` on each of the seven non-nullable `Guid` properties, and a skipped `[Fact(Skip = "Tracked deferral: M18-005 widens these columns to Guid? before M18-006 can NULL them.")]` test enumerating the seven `(Entity, Property)` pairs — M18-005 will close it by removing `Skip=`.
- **#2 (silent fallback default).** Pinned by `Audit_log_actor_email_column_disposition_is_InExportInDeletionAnonymise` plus an inline scanner doc.
- **#3 (`COMPLIANCE.md` link uncovered).** `ComplianceMd_exists` and `ComplianceMd_links_to_data_inventory` enforce the DoD line.
- **#4 (namespace string literal).** Replaced by `private static readonly string EntityNamespace = typeof(User).Namespace ?? throw …` — a future rename of the entity namespace now forces recompilation of the scanner rather than silently emptying the manifest.
- **#5 (lazy `Manifest` static surface uncovered).** Pinned by `Lazy_Manifest_static_returns_thirteen_entries`.

Observable verification from the task YAML:
- `dotnet test … --filter "FullyQualifiedName~GdprInventory"` → 41 passed, 0 failed, 0 skipped.
- `dotnet test … --filter "FullyQualifiedName~GdprAttributeScannerTests"` → 30 passed, 0 failed, 1 skipped (tracked M18-005 deferral). Manifest names all 13 tables with the expected dispositions.
- `grep -c '^| \`' docs/security/data-inventory.md` → `13` (matches the expected cardinality).

Spec coverage against the eight YAML behaviors:
| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Scanner emits one entry per `(Entity, Property, Disposition)` triple | `Manifest_contains_exactly_thirteen_tables`, `Entries_are_ordered_alphabetically_by_entity_name`, `Manifest_records_expected_disposition` |
| 2 | All 13 enumerated entities appear with a disposition drawn from the four-value set | `Manifest_records_expected_disposition` (theory across all 13 entities) |
| 3 | `data-inventory.md` carries 6 columns and ≥13 rows | `Doc_has_expected_column_headers`, `Doc_has_thirteen_data_rows` |
| 4 | New user-attributable entity without GDPR attributes fails CI naming the entity | `GdprInventoryCoverageTests.Every_user_attributable_entity_is_tagged` theory |
| 5 | `[GdprAnonymise]` on `Guid?` recorded as anonymisable-to-null | `OrganizationMember_InvitedBy_is_anonymisable` (`Guid?` — schema honours it now); deferred-Guid columns tracked by the M18-005 skipped test |
| 6 | `[GdprAnonymise]` on `string?` recorded as anonymisable-to-token | `Audit_log_actor_email_anonymises_to_deleted_user_token` |
| 7 | Export builder consumes the `InExport` subset (no second list) | `InExport_subset_has_six_entries`, `InExport_does_not_contain_excluded_tables` (manifest is the single source — M18-004 will read it directly) |
| 8 | Doc-vs-code consistency: every doc row matches scanner disposition | `Doc_row_dispositions_match_scanner_manifest`, `Doc_InExport_column_matches_disposition`, `Doc_InDeletion_column_matches_disposition` |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `Scan(null)` throws `ArgumentNullException` with parameter name; doc-consistency parser throws `InvalidOperationException` with file path and offending row context; no swallowed exceptions. |
| Input Validation | PASS | Round-1 #10 (null assembly) resolved; doc parser validates header row presence, cell count ≥ 6, and disposition substring recognisability. |
| Naming | PASS | `SetNull`/`DeletedUserToken` self-describe (round-1 #1); `-Attribute` BCL suffix; namespace + filename consistent. |
| Code Organization | PASS | Clean split between `Data/GdprAttributes/` (annotations) and `Compliance/Gdpr/` (consumer). No DI wiring, no migrations — pure CSharp + docs per Plan Decision 10. |
| Correctness | PASS | Table-level disposition uses min-over-columns (lower enum value = stronger inclusion right) — pinned by `OrganizationMember_InvitedBy_is_anonymisable` (table hard-delete + column anonymise) and `TablesAnonymisedOnDeletion_includes_audit_log_and_creators`. `ColumnsToAnonymise` column-level flattener correctly surfaces `OrganizationMember.InvitedBy` despite the parent table being `InExportInDeletionHard`. Reflection ordering deterministic (alphabetical, `StringComparer.Ordinal`). |
| Test Quality | PASS | Specific assertions (`.Should().Be(...)`/`.BeEquivalentTo(...)`), table-driven theory for the 13-table disposition matrix, deterministic ordering checks, parser-error paths covered, EF-model cross-check (`Table_names_match_AppDbContext_ToTable_calls`) catches `TableNameOf` drift against the live `OnModelCreating`. |

## Test Coverage
- 80 / 80 GDPR-related tests pass; 1 skipped (`SetNull_on_nonnullable_Guid_columns_are_deferred_to_M18_005`, explicit tracked deferral).
- `GdprAttributeScanner.cs` line coverage 98.55 % / branch 77.4 % per round-2 improvement report (DoD bar: 80 % line).
- No uncovered surfaces remain after round 2.

## Gate Note

`./scripts/ci-local.sh --go` exits 1 in the local sandbox because of a pre-existing `/tmp/apitest_json_XXXXXX.yaml` file blocking `mktemp` in the smoke script's `--format json` step. **The failure is unrelated to M18-003** (this slice is pure C#; `git diff --name-only main...HEAD` shows zero Go files changed) and pre-dates this branch's work. Removing the stale tmp file restores the gate; doing so requires explicit permission outside this audit's scope. The relevant gate for this task — `dotnet test src/ApiTool.Backend.Tests` — is fully green. `/verify` should clean the stale tmp file before running the full suite.

## Summary
Two rounds of review-and-improve have driven the code to a clean state. Every YAML behavior is covered by at least one test, the doc-vs-code consistency loop is enforced both directions (manifest ↔ markdown table, scanner ↔ EF model), the future-drift gap is closed by the coverage theory, and the deferred schema migration (the `SetNull`-on-`Guid` precondition) is tracked explicitly by a skipped test that M18-005 inherits as a checklist. Ready to verify.

→ Run `/verify M18-003` to complete the task.
