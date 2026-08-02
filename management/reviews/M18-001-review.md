# Code Review: M18-001

**Task:** Audit-log bulk export: JSONL+CSV chunked streaming, Enterprise tier gate, cap removal
**Reviewer:** AI
**Date:** 2026-05-17
**Branch:** feature/M18-001-audit-log-bulk-export
**Iteration:** 2 (re-review after improve phase)

## Verdict: PASS

## Findings

No findings. All five findings from iteration 1 are genuinely resolved.

## Prior Findings — Resolution Verification

| # | Severity | Finding | Resolution | Verified |
|---|----------|---------|-----------|---------|
| 1 | Medium | Swagger surface test did not verify `application/x-ndjson` and `text/csv` as response content types in OpenAPI spec | Two-part fix: (a) collapsed separate `Produces(200, contentType: ...)` calls into a single `Produces<ListAuditLogResponse>(200, "application/json", "application/x-ndjson", "text/csv")` — Swashbuckle now correctly emits three content entries under the 200 object; (b) test now enumerates `ok200.GetProperty("content")` keys and asserts both media types are present. Test would fail if `Produces` metadata were absent. | RESOLVED |
| 2 | Low | Missing integration test for unknown-org on the export code branch | `Get_audit_log_export_unknown_org_returns_403` added — uses unrecognised orgId with `?format=jsonl`, asserts 403 + `permission_denied` body, confirming RBAC fires before tier gate on the export path. | RESOLVED |
| 3 | Low | `WriteCsvAsync` had no cancellation test | `WriteCsvAsync_propagates_cancellation_mid_stream` added, mirrors the JSONL counterpart using `CancelAfterNAsync(5, cts)`. | RESOLVED |
| 4 | Low | No empty result set edge case tests | `WriteJsonlAsync_empty_source_writes_headers_only_with_empty_body` and `WriteCsvAsync_empty_source_writes_header_row_only` added; both verify headers are set and body contains only what the format mandates (empty / header-only respectively). | RESOLVED |
| 5 | Low | Redundant `WithCancellation(ct)` in streamer | Removed from both `WriteJsonlAsync` and `WriteCsvAsync`; both now use `await foreach (var dto in rows)` directly. `ct` is correctly bound to the underlying EF enumerator via `[EnumeratorCancellation]` at the call site in `AuditLogEndpoints.cs:110`. The streamer's own `ct.ThrowIfCancellationRequested()` inside the loop remains as a defensive fast-path. | RESOLVED |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No swallowed errors. `AuditLogExportTierGate` uses `UnreachableException` for the exhaustive switch arm — correct. `EnsureExporterAuthorisedAsync` returns typed error values; no throws on expected paths. Streaming exceptions propagate naturally to ASP.NET's error pipeline. |
| Input Validation | PASS | `OrgId.TryParse` guards orgId. `Guid.TryParseExact(..., "N")` guards user_id. `from > to` validated before the export branch is entered. Unknown `format` values fall through to paginated path without a 400 regression. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `AuditLogExportError`, `AuditLogExportTierGate`, `AuditLogExportStreamer`, `StreamForExportAsync`, `EnsureExporterAuthorisedAsync` all follow Effective Go conventions (mutatis mutandis for C#). `ExportFormat` private enum is correctly scoped. |
| Code Organization | PASS | `Internal/TierGates/` package boundary respected. `AuditLogCsvFormatter` internal helpers (`HeaderRow`, `FormatRow`) correctly scoped `internal`. Public `Format(IReadOnlyList<>)` API preserved. No circular dependencies. |
| Correctness | PASS | All eight task behaviors implemented. Cap removal confirmed (no `.Take()` in `StreamForExportAsync`). Filter equivalence confirmed via `BuildFilteredQuery` shared helper. Content-Disposition filename matches spec (`yyyyMMddTHHmmss`). `HttpResults.Empty` correctly no-ops after the streamer writes the response. `X-Accel-Buffering: no` defensively set. Cancellation propagation: `[EnumeratorCancellation]` binds `ct` at call site; streamer's own `ct.ThrowIfCancellationRequested()` and `WriteAsync(bytes, ct)` provide additional cancellation points. No goroutine/task leaks. |
| Test Quality | PASS | All eight behaviors from the task YAML are covered. Empty-result-set edge cases covered. CSV cancellation covered. Unknown-org export branch covered. Swagger surface test now asserts both streaming media types appear in the OpenAPI spec. |

## Test Coverage

- **New test files:** `AuditLogExportTierGateTests.cs` (5 cases), `AuditLogExportStreamerTests.cs` (14 tests after improvements), `AuditLogQueryServiceStreamTests.cs` (12 tests), `AdapterDelegationTests.cs` (3 new cases for the new gate adapter)
- **Modified test files:** `AuditLogEndpointsTests.cs` (9 new integration tests including regression guard + unknown-org export), `AuditLogSwaggerSurfaceTests.cs` (assertions now verify ndjson and csv content types in spec)
- **Total tests after improve:** 1924 passing (up from 1920 before the improve phase)
- **Coverage on `AuditLogExportStreamer.cs`:** 100% line rate — all paths (JSONL write, CSV write, empty-source, cancellation, header configuration) exercised
- **Coverage on `AuditLogExportTierGate.cs`:** 85.7% line rate — above the ≥80% DoD threshold
- **Pre-audit gate:** `./scripts/ci-local.sh --go` PASS

## Summary

All five iteration-1 findings are genuinely resolved with no regressions introduced. The Swagger surface fix is architecturally correct — collapsing multiple `Produces` calls into one is the documented Swashbuckle pattern for multi-content-type responses. The `WithCancellation` removal is safe because `ct` is already bound via `[EnumeratorCancellation]` at the call site, and the streamer's per-row `ct.ThrowIfCancellationRequested()` and per-write `ct` argument provide cancellation detection regardless. The implementation meets all eight specified behaviors, exceeds the DoD test count floor (≥10), and the quality gate passes cleanly.
