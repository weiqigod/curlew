# Improvement Report: M18-003 (Round 2)

**Task:** GDPR data inventory: 13-table decision matrix, `[GdprIncluded]`/`[GdprAnonymise]` attributes, assembly scanner
**Date:** 2026-05-18
**Review:** management/reviews/M18-003-review.md (round 2)

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|-------------|----------|
| 1 | Medium | `AnonymiseAs.SetNull` documented as "for nullable Guid?" but 7 of 8 SetNull-tagged columns are non-nullable `Guid` (`OrganizationAuditLogEntry.ActorId`, `NotificationRule.CreatedBy`, `CustomRole.CreatedBy`, `Schedule.CreatedBy`, `CoordinatorJob.CreatedBy`, `TeamVault.CreatedBy`, `TeamVault.UpdatedBy`) | Reviewer-option (c) selected over (b) because the original plan's Decision 10 explicitly defers schema migrations to M18-005. `AnonymiseAs.SetNull` XML-doc relaxed to acknowledge "when the schema allows it"; each of the 7 properties gets a per-property `NOTE: SetNull declared here; schema widening to Guid? is owed by M18-005`; new `[Fact(Skip = "Tracked deferral: M18-005 widens these columns to Guid? before M18-006 can NULL them.")]` test `SetNull_on_nonnullable_Guid_columns_are_deferred_to_M18_005` enumerates the 7 (Entity, Property) pairs by name so M18-005 inherits a checklist that goes green by removing `Skip=`. | ✓ build clean, all 80 GDPR tests pass + 1 deferral skip |
| 2 | Low | Scanner's `[GdprAnonymise]`-only fallback default disposition uncovered | Added inline doc on `GdprAttributeScanner.cs:67-71` pointing at the pinning test, plus new fact `Audit_log_actor_email_column_disposition_is_InExportInDeletionAnonymise` asserting `ActorEmail.Disposition == InExportInDeletionAnonymise` | ✓ test passes |
| 3 | Low | `docs/COMPLIANCE.md` link to `security/data-inventory.md` uncovered | Added two facts in `GdprInventoryDocConsistencyTests`: `ComplianceMd_exists` and `ComplianceMd_links_to_data_inventory` (`Should().Contain("security/data-inventory.md")`) | ✓ tests pass |
| 4 | Low | Namespace match was a string literal `"ApiTool.Backend.Data.Entities"` | Replaced with `private static readonly string EntityNamespace = typeof(User).Namespace ?? throw …;` — a User-namespace rename now forces recompilation of the scanner | ✓ build clean, existing 13-table cardinality test confirms no regression |
| 5 | Low | Lazy `GdprAttributeScanner.Manifest` static surface uncovered | Added one-line fact `Lazy_Manifest_static_returns_thirteen_entries` asserting `GdprAttributeScanner.Manifest.Entries.Should().HaveCount(13)` | ✓ test passes |

## Out of Scope (Deferred)

No findings deferred. All five round-2 findings resolved. Finding #1's *underlying schema migration* is intentionally deferred to M18-005 per the original plan; the deferral is now explicit, tracked by a skipped test, and documented in three places (the enum XML-doc, the 7 property NOTEs, the test).

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS (0 warnings, 0 errors) |
| `dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~Gdpr"` | PASS (80 passed, 1 skipped — the tracked deferral, 0 failed) |
| `dotnet test src/ApiTool.Backend.Tests` (full backend suite) | PASS (2024 passed, 11 skipped — external deps, 0 failed) |
| Scanner coverage | 98.55% line / 77.4% branch (well above 80% line DoD bar) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `6ee29eca` | fix(gdpr): address round-2 review findings on M18-003 | #1, #2, #3, #4, #5 |

## Notes on judgment calls

- **Finding #1 — chose option (c) over reviewer-recommended (b).** The reviewer's option (b) — a precondition check in `Scan` that throws when `[GdprAnonymise(SetNull)]` is on a non-nullable `Guid` — would have caused the scanner to throw immediately against the current entity set, requiring either (a) a schema migration (out of scope per Plan Decision 10) or a hand-maintained grandfather list in the scanner (the exact "second list to drift" the plan tries to avoid). Option (c) preserves the original slice's "pure-CSharp + docs" scope while making the deferred work visible in three coupled places: the enum doc, the 7 property NOTEs, and a skipped test that M18-005 turns green by removing `Skip=`.
- **Finding #5 — chose the targeted-fact route over redirecting the class field at `GdprAttributeScanner.Manifest`.** Redirecting would have coupled every other test in the class to the lazy static and broken `Scan_is_deterministic`, which needs two independent `Scan` calls. The one-line fact gives equivalent surface coverage without that cost.

## Summary
5/5 findings resolved. 0 deferred. Backend suite green (2024 passed, 0 failed). Scanner coverage 98.55% line. Ready for `/review M18-003` round 3.
