# Improvement Report: M18-002

**Task:** Audit-log RBAC permissions + per-org retention + AuditLogCleanupHost
**Date:** 2026-05-17
**Review:** management/reviews/M18-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `UpdateOrganization` endpoint registration missing `Produces<ErrorResponse>(400)` for `retention_days_exceeds_cap` and `retention_days_invalid`, inconsistent with sibling endpoints | Added `.Produces<ErrorResponse>(StatusCodes.Status400BadRequest)` to the `UpdateOrganization` `.MapPatch` chain in `MembersEndpoints.cs` | ✓ tests pass |
| 2 | Medium | `org.settings.updated` audit event omitted `audit_log_retention_days` from `PreviousState`/`NewState`, producing misleading audit records when only retention was changed | Captured `prevRetentionDays = org.AuditLogRetentionDays` before mutation; included `audit_log_retention_days` in both state snapshots in `MembersService.UpdateOrgAsync` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build ApiTool.Backend.sln` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| Coverage (AuditLogCleanupHost) | ~92% line |
| Coverage (AuditLogQueryService) | ~92% line |

Full test run: 1944 passed, 0 failed, 10 skipped.

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| bb2d596e | fix(organizations): declare 400 responses on UpdateOrganization endpoint | #1 |
| 2871b70d | fix(organizations): include audit_log_retention_days in org.settings.updated payload | #2 |

## Summary

2/2 findings resolved. 0 deferred.
