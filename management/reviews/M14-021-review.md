# Code Review: M14-021

**Task:** E2E: login → run --report-upload → check-run posted → receipt email queued
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-021-e2e-revenue-loop
**Iteration:** 3

## Verdict: PASS

## Findings

No findings. All previous findings resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Go errors wrapped with `%w`; sentinel `ErrNetworkFailure` used for caller-matchable network failure. C# uses typed error enum returns (`PrCheckError`). `EmailQueueProcessor` resolves `IRecentlySentEmailLog` via nullable `GetService<>()?.Record()` — no-op when unregistered in production. No swallowed errors. |
| Input Validation | PASS | `SeedM14Request`: `body is null` → 400, `OrgId == Guid.Empty` → 400, subscription not found → 404. `handleReportUpload`: missing `CURLEW_BACKEND_URL` → exit 2; missing `--org` → exit 1 at parse time; `--pr` without `--repo` (and vice versa) → error at parse time. `InMemoryRecentlySentEmailLog`: capacity ≤ 0 throws `ArgumentOutOfRangeException`; `Recent(0)` returns empty. |
| Naming | PASS | No stuttering. Doc comments on all exported Go functions and all C# public types. `IRecentlySentEmailLog` follows the `-er` interface-naming convention. Package names are lowercase single-word. `SeedM14Request` is `internal sealed record` — correctly scoped to the assembly. |
| Code Organization | PASS | `internal/` package boundaries respected. New web routes in correct SvelteKit route-tree location. `IRecentlySentEmailLog` separated from production `IEmailQueue`. Dev/Testing env-gating is consistent with the `seed-refresh` precedent. `InternalAccessFilter` (defined in `ApiTool.Backend.Licensing.Keys`) is a second-layer security guard on both new internal endpoints. |
| Correctness | PASS | `{{ GITHUB_MOCK_URL }}` (plain identifier) in `testdata/m14/e2e-collection.yaml` resolves correctly via `--env-var GITHUB_MOCK_URL` which reads from the process environment set by the Playwright spec. `InMemoryRecentlySentEmailLog` ring-buffer eviction under `lock(_lock)` is race-safe. `JsonSerializer.Serialize` used for `RepoSetJson` (repo names with special chars handled). `handleReportUpload` stdout conditioned on `flags.pr > 0 && flags.repo != ""`. `ci-local.sh --down` is idempotent via `|| true` on both stack-down calls. |
| Test Quality | PASS | All planned test cases present: `RecentlySentEmailLogTests` (4 cases incl. concurrency with 1000 parallel goroutines), `InternalEmailAuditEndpointTests` (3 cases: Testing=200, round-trip billing_receipt, Production=404), `InternalSeedM14EndpointTests` (4 cases: upgrade, noop/idempotency, row-insert, Production=404), `PrChecksServiceTests` (2 additive cases for `PostedAt`/`CheckRunId` round-trip), `TestCiLocalDownIdempotent` (Go smoke test, two sequential invocations), `TestRunWithReportUpload` (6 cases covering pass/fail/no-PR/missing-org/unreachable/missing-URL), Playwright spec (5 assertions, all guarded via `test.skip(!BACKEND_TOKEN, ...)`). |

## Resolved Findings (prior iterations)

| Iteration | Severity | Finding | Resolution |
|-----------|----------|---------|------------|
| 1 | Medium | `InternalSeedM14EndpointTests` missing Production=404 test | Added `SeedM14_in_Production_returns_404` via isolated `WebApplicationFactory<Program>` with `UseEnvironment("Production")` |
| 1 | Medium | `InternalEmailAuditEndpointTests` missing Production=404 test | Added `Get_in_Production_returns_404` via same isolated factory pattern |
| 1 | Low | `InternalSeedM14Endpoint.cs` manual JSON string interpolation for `RepoSetJson` | Replaced with `JsonSerializer.Serialize(body.Repos ?? [])` |
| 2 | High | `testdata/m14/e2e-collection.yaml` used `{{ env.GITHUB_MOCK_URL }}` (dotted namespace unsupported by variable resolver) | Changed to `{{ GITHUB_MOCK_URL }}`; spec passes `--env-var GITHUB_MOCK_URL` to CLI; subprocess env includes `GITHUB_MOCK_URL` |

## Test Coverage
- Go coverage: **87.3%** (project total); `handleReportUpload` path 100%.
- C# (M14-021 scope): 23/23 tests pass — `RecentlySentEmailLogTests` (4), `InternalEmailAuditEndpointTests` (3), `InternalSeedM14EndpointTests` (4), `PrChecksServiceTests` additive cases (2), plus pre-existing `PrChecksServiceTests` (10).
- Playwright spec: 5 assertions present, guarded by `CURLEW_BACKEND_TOKEN` skip-guard for non-E2E runs.

## Summary

All four prior-iteration findings have been correctly resolved. The iteration-2 High finding (`{{ env.GITHUB_MOCK_URL }}` dotted namespace) is fixed: the collection uses `{{ GITHUB_MOCK_URL }}`, the Playwright spec passes `--env-var GITHUB_MOCK_URL` to the CLI invocation, and the subprocess environment contains `GITHUB_MOCK_URL` so the import resolves at runtime. The codebase is clean across all six audit categories. No new findings.
