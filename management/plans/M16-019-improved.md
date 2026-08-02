# Improvement Report: M16-019 (Iteration 4)

**Task:** /results/stats and /results/failures aggregation endpoints
**Date:** 2026-05-12
**Review:** management/reviews/M16-019-review.md

## Resolved Findings (Iteration 1 — superseded)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `OrgId.TryParse` used in both handlers instead of `OrgResolver.ResolveAsync`, breaking slug-based routing | Replaced both `if (!OrgId.TryParse(...))` blocks with `await OrgResolver.ResolveAsync(orgId, db, ct)` returning null-checked `Guid?`, matching the pattern used in `ResultsEndpoints` | ✓ tests pass |
| 2 | High | Missing `Stats_returns_403_for_member_without_dashboard_view_permission` endpoint test | Added test with helper `CreateMemberWithoutDashboardViewAsync` that seeds a custom role excluding `dashboard.view` and asserts 403 | ✓ tests pass |
| 3 | High | Missing `Stats_accepts_org_slug_in_path` test | Added test that reads the org slug and calls the stats endpoint with it, asserting 200 | ✓ tests pass |
| 4 | Medium | No test for deterministic secondary sort (`failure_count DESC` tie-breaker by `PathTemplate`) | Added `Failures_sorted_by_failure_count_desc_then_path_template_asc` seeding two groups with equal `failure_count` and asserting alphabetical `PathTemplate` ordering | ✓ tests pass |
| 5 | Low | Loose p95 range assertion (`>= 280 && <= 300`) obscures whether the percentile helper is correct | Replaced with exact `Assert.Equal(290L, ...)` with inline derivation comment | ✓ tests pass |

## Resolved Findings (Iteration 2 — superseded)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `GetStatsAsync`/`GetFailuresAsync` materialised ALL rows via `ToListAsync` with no Postgres native-SQL path, violating the plan's provider-branching architecture and 800 ms latency SLA on 5M-row datasets (behavior #7) | Added `db.Database.IsNpgsql()` branch: Postgres path uses raw ADO.NET SQL with `PERCENTILE_CONT(0.5/0.95) WITHIN GROUP (ORDER BY duration_ms)`, `DATE_TRUNC('day', created_at AT TIME ZONE 'UTC')` for trend, and a `LATERAL` subquery for `sample_run_ids`. SQLite/test path retains the C# in-memory computation. Both paths produce identical wire shapes. | ✓ tests pass |
| 2 | Medium | Behavior #7 (800 ms latency SLA on 5M-row fixtures) had no test coverage | Added `DashboardLatencyBenchTests.cs` with two `[SkippableFact]` tests guarded by `APITOOL_RUN_DASHBOARD_LATENCY_BENCH=1` env var. Stats test also verifies `EXPLAIN ANALYZE` references `IX_Results_OrgId_CreatedAt`. | ✓ benchmark tests skip cleanly; non-benchmark tests still pass |
| 3 | Low | XML doc comment on `PathTemplateExtractor.Extract` said "case-insensitive on input" for `method` — misleading because `Extract` never uppercases the method | Replaced with accurate doc: "Must already be uppercased by the caller (e.g. via `ToUpperInvariant()`); `Extract` does not normalise case." | ✓ tests pass |

## Resolved Findings (Iteration 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | EXPLAIN ANALYZE SQL in `DashboardLatencyBenchTests` built with string interpolation (`$"... WHERE org_id = '{orgId}'..."`) — violates parameterized-query convention; any scanner flags it; bad precedent for copy-paste | Refactored `ExecuteExplainAsync` to accept `Guid orgId` parameter instead of a pre-built SQL string. Method now builds the SQL with `@orgId` placeholder and binds it via `cmd.CreateParameter()` / `cmd.Parameters.Add(p)`, matching the `AddParam` pattern used in production `DashboardResultsService` | ✓ tests pass |
| 2 | Low | `Failures_accepts_org_slug_in_path` test absent from `DashboardEndpointsTests` — failures endpoint uses identical `OrgResolver.ResolveAsync` code path as stats but had no slug-routing HTTP test | Added `Failures_accepts_org_slug_in_path` test mirroring the existing `Stats_accepts_org_slug_in_path` test: reads the org slug via GET /organizations/{id}, calls `/results/failures` with the slug, asserts 200 and presence of `items` field | ✓ tests pass |

## Resolved Findings (Iteration 4)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `Failures_returns_groups_sorted_by_count_desc` did not verify `first_seen_at`, `last_seen_at`, or `sample_run_ids` in the HTTP response JSON — behavior #5 fields confirmed correct at service layer but HTTP serialisation (JsonPropertyName annotations) was unverified | Extended the test to assert `first_seen_at` and `last_seen_at` are present via `TryGetProperty`, and `sample_run_ids` is present and non-empty | ✓ tests pass |
| 2 | Low | Failures response envelope fields `window` and `limit` were never asserted at the HTTP layer — unlike the stats full-schema test which checks every envelope field | Added `root.GetProperty("window")` and `root.GetProperty("limit")` assertions to the same test | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1868 passed, 10 skipped) |
| Dashboard-specific tests | PASS (78 passed, 2 benchmark tests skipped) |
| Coverage (SQLite fallback paths) | >80% on exercised paths; Postgres-native paths require live Postgres (expected) |

## Fix Commits (Iteration 4)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 1114f47b | fix(dashboard): extend failures endpoint test to assert all behavior #5 fields | #1, #2 |

## Summary

2/2 iteration-4 findings resolved. 0 deferred. All prior findings (iterations 1, 2, and 3) previously resolved.
