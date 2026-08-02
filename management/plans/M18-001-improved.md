# Improvement Report: M18-001

**Task:** Audit-log bulk export: JSONL+CSV chunked streaming, Enterprise tier gate, cap removal
**Date:** 2026-05-17
**Review:** management/reviews/M18-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Swagger surface test does not verify `application/x-ndjson` and `text/csv` appear as response content types in the OpenAPI spec | Two-part fix: (a) restructured `AuditLogEndpoints.cs` to use a single `Produces<ListAuditLogResponse>(200, "application/json", "application/x-ndjson", "text/csv")` call (previous separate `Produces(200, contentType: ...)` calls were collapsed by Swashbuckle into an empty 200 response without content); (b) extended `AuditLogSwaggerSurfaceTests.cs` to enumerate the `200` response content keys and assert both media types appear | tests pass |
| 2 | Low | Missing `Get_audit_log_export_unknown_org_returns_403` integration test for the export code branch | Added `Get_audit_log_export_unknown_org_returns_403` test to `AuditLogEndpointsTests.cs` exercising an unknown orgId with `?format=jsonl`; asserts 403 with `permission_denied` body (RBAC check fires before tier gate, consistent with anti-enumeration shape of the paginated path) | tests pass |
| 3 | Low | `WriteCsvAsync` has no cancellation test | Added `WriteCsvAsync_propagates_cancellation_mid_stream` to `AuditLogExportStreamerTests.cs` mirroring the existing JSONL counterpart using `CancelAfterNAsync` helper | tests pass |
| 4 | Low | No empty result set edge case tests | Added `WriteJsonlAsync_empty_source_writes_headers_only_with_empty_body` and `WriteCsvAsync_empty_source_writes_header_row_only` to `AuditLogExportStreamerTests.cs` | tests pass |
| 5 | Low | Redundant `WithCancellation(ct)` on `rows` in both `WriteJsonlAsync` and `WriteCsvAsync` | Removed `.WithCancellation(ct)` calls; both methods now iterate `await foreach (var dto in rows)` directly, since `ct` is already bound via the `[EnumeratorCancellation]` attribute on `StreamForExportAsync` | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `golangci-lint run` | N/A (backend track, Go linter not applicable) |
| Total tests | 1924 passed, 0 failed, 10 skipped |
| Coverage `AuditLogExportStreamer.cs` | 100% line rate, 100% branch rate |
| Coverage `AuditLogExportTierGate.cs` | 85.7% line rate (≥80% DoD) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ce1222a1 | fix(audit): remove redundant WithCancellation in streamer | #5 |
| 817d1e0d | test(audit): add WriteCsvAsync cancellation + empty-set edge case tests | #3, #4 |
| 46026925 | test(audit): add unknown-org export integration test | #2 |
| 9ae9338e | fix(audit): assert ndjson+csv content types in OpenAPI spec + test | #1 |

## Summary

5/5 findings resolved. 0 deferred.

New tests added: 4 (1 integration, 3 unit) bringing total from 1920 to 1924 passing.
All previously passing tests continue to pass — no regressions.
