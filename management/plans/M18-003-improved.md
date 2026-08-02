# Improvement Report: M18-003

**Task:** GDPR data inventory: 13-table decision matrix, `[GdprIncluded]`/`[GdprAnonymise]` attributes, assembly scanner
**Date:** 2026-05-18
**Review:** `management/reviews/M18-003-review.md`

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|-------------|----------|
| 1 | Medium | `AnonymiseAs.NullOrToken` is opaque (only sibling member emits a token) | Renamed enum member to `AnonymiseAs.SetNull`; updated all 13 usages (entities, attribute XML doc, tests) | ✓ build clean, 76 GDPR tests pass |
| 2 | Medium | Doc consistency test only checks `TableName → Disposition`; InExport/InDeletion column text never validated | Added `Doc_InExport_column_matches_disposition` and `Doc_InDeletion_column_matches_disposition` facts that derive the expected token from each row's disposition and assert the column text matches | ✓ new tests pass; flipping a `yes`→`no` in the doc would now fail |
| 3 | Medium | Behaviour B3 says doc carries columns `Table\|Owner\|InExport\|InDeletion\|Retention\|Notes` but column names were never asserted | Added `Doc_has_expected_column_headers` fact that locates the header row (first row whose first cell is `Table`) and asserts the six column names verbatim | ✓ new test passes |
| 4 | Medium | `GdprBundleManifest.InDeletionAnonymise` uses table-level disposition, so it silently omits column-level anonymisation cases like `OrganizationMember.InvitedBy` — misleading for M18-006 | Renamed property to `TablesAnonymisedOnDeletion` with a clearer XML doc that flags the table-vs-column distinction. Added parallel `ColumnsToAnonymise` helper that flattens (entry, column) pairs across the manifest. Two new tests assert that `OrganizationMember.InvitedBy` and the audit-log columns surface via the column-level view | ✓ tests pass |
| 5 | Low | Coverage test's `if (classAttr is not null) return` over-broadly exempts entities with any `GdprTableAttribute` rather than only the two opt-out `Kind` values | Switch is now `is { Kind: GdprTableKind.NotUserAttributable or GdprTableKind.ExcludedFromBoth }`; a future non-exempting `Kind` variant fails the coverage check by default | ✓ tests pass |
| 6 | Low | `ExcludedFromBoth.OnlyContain(e => e.Columns.Count == 0)` passes vacuously on empty | Replaced with `ExcludedFromBoth_contains_exactly_GithubInstallation_and_GitLabInstallation` asserting `HaveCount(2)`, the two specific entity types, and the empty-columns invariant | ✓ test passes |
| 7 | Low | `Table_names_match_AppDbContext_ToTable_calls` only spot-checked 3 of 13 names; a rename of any of the other 10 would silently desync | Test now constructs an in-memory `AppDbContext`, iterates every manifest entry, and compares the scanner's `TableName` against `ctx.Model.FindEntityType(t).GetTableName()` | ✓ test passes |
| 8 | Low | Doc-row parser used literal `"\| `"` prefix — fragile to a missing space after the pipe | Parser is now a compiled regex `^\|\s*` `([^` `]+)` `\s*\|` with helper `SplitMarkdownRow` that strips outer pipes and trims cells. Throws a clear `InvalidOperationException` if a matched row has fewer than 6 columns | ✓ all parsing tests pass |
| 9 | Low | Rows in `data-inventory.md` not alphabetised | Sorted all 13 rows alphabetically by table name. Added `Doc_rows_are_alphabetically_ordered_by_table_name` fact that locks the order | ✓ test passes |
| 10 | Low | `Scan(Assembly)` did not null-check the argument; NRE rather than `ArgumentNullException` | Added `ArgumentNullException.ThrowIfNull(assembly)` at the top of `Scan`. New test `Scan_throws_ArgumentNullException_when_assembly_is_null` asserts the parameter name | ✓ test passes |

## Out of Scope (Deferred)

No findings deferred. All ten findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS (0 warnings, 0 errors) |
| `dotnet test src/ApiTool.Backend.Tests --filter "FullyQualifiedName~Gdpr"` | PASS (76/76; was 69/69 — 7 new tests added) |
| `dotnet test src/ApiTool.Backend.Tests` (full backend suite) | PASS (2020 passed, 0 failed, 10 skipped — external deps) |
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `~/go/bin/golangci-lint run` | PASS (0 issues) |
| Coverage `GdprAttributeScanner.cs` | 98.41% line / 78.04% branch (≥ 80% line) |
| Coverage `GdprBundleManifest.cs` | 92.30% line / 100% branch |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `8e04568b` | refactor(gdpr): rename AnonymiseAs.NullOrToken to SetNull and InDeletionAnonymise to TablesAnonymisedOnDeletion | #1, #4 |
| `32c7a5f8` | fix(gdpr): tighten tests, validate input, sort inventory doc alphabetically | #2, #3, #5, #6, #7, #8, #9, #10 |

## Summary

10/10 findings resolved. No findings deferred. Quality gate green; 7 new tests added (`Scan_throws_ArgumentNullException_when_assembly_is_null`, `ColumnsToAnonymise_includes_OrganizationMember_InvitedBy_despite_hard_table_disposition`, `ColumnsToAnonymise_surfaces_audit_log_token_and_null_columns`, `ExcludedFromBoth_contains_exactly_GithubInstallation_and_GitLabInstallation`, `Doc_has_expected_column_headers`, `Doc_rows_are_alphabetically_ordered_by_table_name`, `Doc_InExport_column_matches_disposition`, `Doc_InDeletion_column_matches_disposition` — net +7 after replacing one vacuous assertion).
