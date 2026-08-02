# Improvement Report: M14-021

**Task:** E2E: login → run --report-upload → check-run posted → receipt email queued
**Date:** 2026-05-06
**Review:** management/reviews/M14-021-review.md

## Resolved Findings

### Iteration 1 (review 1 → review 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `InternalSeedM14EndpointTests` missing `SeedM14_in_Production_returns_404` test — env-gate security invariant unexercised | Added `SeedM14_in_Production_returns_404` test using an isolated `WebApplicationFactory<Program>` with `UseEnvironment("Production")` and in-memory SQLite DB override; asserts `POST /internal/test/seed-m14` returns 404 | ✓ tests pass |
| 2 | Medium | `InternalEmailAuditEndpointTests` missing `Get_in_Production_returns_404` test — env-gate security invariant unexercised | Added `Get_in_Production_returns_404` test using same isolated Production-env factory pattern; asserts `GET /internal/test/email-audit` returns 404 | ✓ tests pass |
| 3 | Low | `InternalSeedM14Endpoint.cs` line 60: manual JSON string interpolation for `RepoSetJson` could produce malformed JSON for repo names containing `"` or `\` | Replaced `"[" + string.Join(",", body.Repos.Select(r => $"\"{r}\"")) + "]"` with `JsonSerializer.Serialize(body.Repos ?? [])` — also added `using System.Text.Json;` import | ✓ tests pass |

### Iteration 2 (review 2 → review 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 4 | High | `testdata/m14/e2e-collection.yaml` line 11 uses `{{ env.GITHUB_MOCK_URL }}` — a dotted namespace the CLI variable resolver (`varPattern = [a-zA-Z_][a-zA-Z0-9_]*`) cannot match. The placeholder passed through unresolved, making the collection URL a literal string and causing exit 1 at E2E runtime. | Changed collection URL to `{{ GITHUB_MOCK_URL }}` (plain identifier). Added `'--env-var', 'GITHUB_MOCK_URL'` to the Playwright spec's CLI flags so the env var already in the process environment is imported into the variable scope. Removed the non-functional `APITEST_ENV_GITHUB_MOCK_URL` process env key. | ✓ build + tests + lint pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 87.3% |
| `dotnet test` (M14-021 scope) | PASS (23/23) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 4915e31e | fix(internal): add Production-env gate tests + safe JSON serialization | #1, #2, #3 |
| 487d0e22 | fix(e2e): resolve {{ GITHUB_MOCK_URL }} using plain identifier syntax | #4 |

## Summary

4/4 findings resolved. 0 deferred.

Iteration 1 fixed three findings from the first review: two missing Production-env tests for the internal endpoint env-gates, and a JSON serialization correctness issue. Iteration 2 fixed the single High finding from the second review: `{{ env.GITHUB_MOCK_URL }}` used a dotted namespace syntax that the CLI variable resolver doesn't support. The fix uses the plain `{{ GITHUB_MOCK_URL }}` identifier and imports it into scope via `--env-var GITHUB_MOCK_URL`, which reads from the process environment already set by the Playwright runner.
