# Improvement Report: M5-004

**Task:** Backend: audit log capture middleware
**Date:** 2026-04-18
**Review:** management/reviews/M5-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `Get_audit_log_csv_rfc4180_quotes_fields_with_commas` test missing; `Quote()` double-quote escape branch (line 42) and injection-prefix branch (line 39) in `AuditLogCsvFormatter` had 0 coverage | Created `AuditLogCsvFormatterTests.cs` with 8 unit tests covering: comma wrapping, RFC 4180 double-quote doubling, formula-injection prefix (=, +, @), newline wrapping, header row, and plain-field no-quote case | ✓ 8/8 tests pass |
| 2 | High | `Get_audit_log_with_user_id_filter_returns_only_matching_rows` test missing; `user_id` query param parse path and invalid-GUID 400 return in `AuditLogEndpoints` had 0 coverage | Added `Get_audit_log_with_user_id_filter_returns_only_matching_rows` and `Get_audit_log_with_invalid_user_id_format_returns_400` integration tests to `AuditLogEndpointsTests.cs` | ✓ both tests pass |
| 3 | Medium | `AuditLogError.OrganizationNotFound` defined but never returned or handled — latent correctness hazard (silent fall-through to CSV/200 branch if ever triggered) | Removed `OrganizationNotFound` from `AuditLogError.cs`; `PermissionDenied` already covers the unknown-org-as-not-a-member → 403 path | ✓ build clean, no references |
| 4 | Low | `userId is null` path in `GetAuditLog` (401 return) never hit in tests | Added `Get_audit_log_unauthenticated_returns_401` integration test that uses an anonymous client with no `Authorization` header | ✓ test passes |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend/ApiTool.Backend.csproj` | PASS |
| `dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj` | PASS |
| All tests | PASS |
| Audit-scoped tests | 45 passed (up from 34) |
| Total test count | 395 (up from 384) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c5b954c | fix(audit): remove dead AuditLogError.OrganizationNotFound enum value | #3 |
| 86bcc1d | test(audit): add RFC 4180 quoting and CSV injection protection tests | #1 |
| 5067466 | test(audit): add user_id filter, invalid user_id, and 401 unauthenticated tests | #2, #4 |

## Summary
4/4 findings resolved. 0 deferred.
