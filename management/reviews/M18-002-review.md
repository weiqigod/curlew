# Code Review: M18-002

**Task:** Audit-log RBAC permissions + per-org retention + AuditLogCleanupHost
**Reviewer:** AI
**Date:** 2026-05-17
**Iteration:** 2 (post-improve)
**Branch:** feature/M18-002-audit-rbac-retention (committed to main)

## Verdict: PASS

## Findings

No findings. Both issues raised in iteration 1 have been resolved:

- **Finding 1 (Medium):** `UpdateOrganization` endpoint now correctly declares `.Produces<ErrorResponse>(StatusCodes.Status400BadRequest)` at `MembersEndpoints.cs:50`. Confirmed.
- **Finding 2 (Medium):** `MembersService.UpdateOrgAsync` now captures `prevRetentionDays = org.AuditLogRetentionDays` before mutation and includes `audit_log_retention_days` in both `PreviousState` and `NewState` of the `org.settings.updated` audit event (`MembersService.cs:233, 250-251`). Confirmed.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned (never thrown/swallowed). `AuditLogError` and `MemberError` discriminated unions used correctly throughout. `OperationCanceledException` caught specifically in `ExecuteAsync` loop, not swallowed on cancel path. No unguarded exceptions. |
| Input Validation | PASS | `retention_days` validated as > 0 (returns `RetentionDaysInvalid`) and > 365 requires Enterprise (returns `RetentionDaysExceedsCap`). OrgId string parse validated before use. UserId filter validated as GUID-N format. All error paths map to 400 with named error codes. |
| Naming | PASS | No stuttering. Doc comments on all exported types and public methods. `AuditLogMetrics` is `internal`. `AuditLogCleanupOptions.Section` constant follows established config-section pattern. |
| Code Organization | PASS | `AuditLogCleanupHost`, `AuditLogMetrics`, `AuditLogCleanupOptions`, `InternalAuditCleanupTickEndpoint` cleanly separated into focused files. `RoleResolver` injected via constructor (no unnecessary abstraction layer). Service scope correctly managed per-tick via `IServiceScopeFactory`. |
| Correctness | PASS | `ExecuteDeleteAsync` used for bulk delete (avoids materializing rows). `TimeProvider` injected for testability. Per-org iteration ensures independent retention. `TickOnceAsync` is a public seam callable from the test-hook. `InternalAuditCleanupTickEndpoint` guards against non-Testing/Development environments. Both previous medium findings resolved. |
| Test Quality | PASS | 5 `AuditLogCleanupHostTests` unit tests cover deletion, per-org independence, false-positive guard, structured log, and metrics counter. 3 new `AuditLogEndpointsTests` cover Security-Auditor 200, Security-Auditor export 403 naming `audit_log.export`, and plain-Member 403 on both paths. 4 `MembersUpdateOrgRetentionTests` cover valid set, non-Enterprise > 365 rejection, Enterprise > 365 allowed, zero rejection. 1 `OrganizationAuditLogSchemaTests` verifies column default. 2 `InternalAuditCleanupTickEndpointTests` verify end-to-end cleanup and hook reachability. Total: 15 new tests (exceeds ≥12 DoD). |

## Behavior Coverage

| # | Behavior | Covered By | Status |
|---|----------|-----------|--------|
| 1 | `audit_log_retention_days` column, INTEGER NOT NULL DEFAULT 365 | `OrganizationAuditLogSchemaTests.Organizations_table_has_audit_log_retention_days_column_with_default_365` | COVERED |
| 2 | Non-Enterprise > 365 → 400 `retention_days_exceeds_cap` | `MembersUpdateOrgRetentionTests.Update_org_rejects_retention_days_above_365_for_non_enterprise` | COVERED |
| 3 | Daily cleanup deletes expired rows per-org, retains within-window rows | `AuditLogCleanupHostTests.TickOnceAsync_deletes_rows_older_than_retention_for_each_org`, `TickOnceAsync_honours_per_org_retention_independently`, `TickOnceAsync_keeps_rows_within_retention_window` | COVERED |
| 4 | `audit_log.view` and `audit_log.export` in `Permissions.All`; Owner and Admin carry both | `PermissionsTests.All_contains_audit_log_keys`, `BuiltInAdmin_contains_audit_log_view_and_export`, `BuiltInOwner_contains_both_audit_log_permissions` | COVERED |
| 5 | Security Auditor (audit_log.view only) GET /audit-log → 200 | `AuditLogEndpointsTests.Get_audit_log_as_security_auditor_member_returns_200` | COVERED |
| 6 | Security Auditor GET ?format=jsonl → 403, body names `audit_log.export` | `AuditLogEndpointsTests.Get_audit_log_jsonl_as_security_auditor_returns_403_with_export_permission_name` | COVERED |
| 7 | Plain Member without custom role → 403 on both paginated and export paths | `AuditLogEndpointsTests.Get_audit_log_as_plain_member_without_custom_role_returns_403` | COVERED |
| 8 | Cleanup logs per-org deletion count + emits Prometheus counter | `AuditLogCleanupHostTests.TickOnceAsync_logs_per_org_deletion_count`, `TickOnceAsync_increments_prometheus_counter` | COVERED |

All 8 behaviors covered.

## Test Coverage

- Go gate: all packages ≥ 80% (all cached from prior run; gate: PASS)
- Backend: 15+ new tests; all pass per improved plan report (1944 passed, 0 failed after improvements)
- `AuditLogCleanupHost.cs`: ~92% line coverage (unit + integration via hook endpoint)
- `AuditLogQueryService.cs` modified paths: covered by existing + 3 new endpoint integration tests
- DoD requirement (≥ 80%) satisfied

## Summary

The iteration 2 code is clean. Both medium findings from iteration 1 have been correctly resolved: the `UpdateOrganization` endpoint now declares its 400 response shape in OpenAPI metadata, and the `org.settings.updated` audit event correctly captures `audit_log_retention_days` in both previous and new state snapshots. All 8 task behaviors are covered by tests, the CI gate passes, and all standards categories pass.
