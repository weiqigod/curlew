# Improvement Report: M14-018 (Iteration 3)

**Task:** Backend: outbound Checks API POST + pr_checks expansion + 6-state mapping
**Date:** 2026-05-06
**Review:** management/reviews/M14-018-review.md

## Resolved Findings (Iteration 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `PrChecksUploadEndpoint` had 0% line/branch coverage; `PrChecksUploadEndpointTests.cs` was never created | Extracted `ICheckRunPoster` interface; updated `CheckRunPoster` to implement it; changed `Program.cs` to register `AddScoped<ICheckRunPoster, CheckRunPoster>()`; created `FakeCheckRunPoster` test double; wrote `PrChecksUploadEndpointTests.cs` (11 cases) covering happy path, 401, 400 token-leak, 400 invalid state, 400 action_required, all 6 valid states, 400 missing/short head_sha, 404 no-installation, 403 repo-not-covered, 429 rate-limited | ✓ 11 tests pass |
| 2 | Critical | `GitHubInstallationsApi` had 0% line/branch coverage; `GitHubInstallationsApiTests.cs` was never created | Created `GitHubInstallationsApiTests.cs` (4 cases) verifying: Authorization header uses installation token not App JWT, full_name parsed into Owner/Name, malformed full_name (no slash) silently skipped, exactly one HTTP request per invocation | ✓ 4 tests pass |
| 3 | Medium | Multi-org user silently picks `memberships[0]` with no observable log signal for production misrouting | Added `log.LogWarning` in `PrChecksUploadEndpoint.HandleAsync` when `memberships.Count > 1` | ✓ build + lint pass |
| 4 | Low | `EscapeUserContent` declared `public` with no production callers | Changed to `internal static string EscapeUserContent(...)` — `InternalsVisibleTo("ApiTool.Backend.Tests")` already in `ApiTool.Backend.csproj` so existing `MarkdownSafetyTests` remain accessible | ✓ tests pass unchanged |

## Resolved Findings (Iterations 1 & 2 — all previously fixed)

All 12 findings from the first two review iterations were resolved. See
the prior iteration-2 report content below for details.

### Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | GitHub 422 mapped to `RepoNotCovered` (wrong) | Split handler: 404→`RepoNotCovered`, 422→`PermanentFailure` | ✓ |
| 2 | Medium | Idempotency path did not persist `row.Conclusion` | Moved conclusion computation before idempotency GET | ✓ |
| 3 | Medium | `HttpResponseMessage` objects not disposed | Added `using var _ = rsp` / `using var __ = rsp` | ✓ |
| 4 | Low | XML-doc mismatch on `PrChecksUploadEndpoint` | Updated doc comment to match actual behaviour | ✓ |
| 5 | Low | Duplicate `// Step 10:` labels in `CheckRunPoster` | Renumbered steps sequentially | ✓ |

## Out of Scope (Deferred)

No findings deferred. All 4 iteration-3 findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1128 passed, 0 failed, 8 skipped) |
| `dotnet test --filter CheckRunPoster\|PrChecksExpansion` | PASS (22 passed ≥ 14 floor) |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (line) | 93.9% |
| Coverage (branch) | 68.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `5017612a` | fix(pr-checks): extract ICheckRunPoster, internal EscapeUserContent, multi-org log warning | #3, #4 (+ structural enabler for #1) |
| `3c8f51c8` | test(pr-checks): add PrChecksUploadEndpointTests + GitHubInstallationsApiTests | #1, #2 |

## Summary

4/4 findings resolved. 0 deferred.

- Critical: 2 fixed
- Medium: 1 fixed
- Low: 1 fixed

Both previously 0%-coverage production classes now have meaningful test coverage.
The `ICheckRunPoster` interface extraction was the key structural enabler, allowing
endpoint tests to inject a `FakeCheckRunPoster` via `WithWebHostBuilder` without
any real GitHub HTTP traffic.
