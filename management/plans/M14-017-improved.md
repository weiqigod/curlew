# Improvement Report: M14-017

**Task:** Backend: github_installations table + dashboard + webhook-first claim flows
**Date:** 2026-05-06
**Review:** management/reviews/M14-017-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Reconciler calls `ApplyRepoAddedAsync` (set-union only) instead of replacing `repo_set`; drift can only grow, never shrink | Added `ReplaceRepoSetAsync` method to `GithubInstallationsService` that computes removals (current − remote) and calls `ApplyRepoRemovedAsync` for them before calling `ApplyRepoAddedAsync` for additions. Updated reconciler to call `ReplaceRepoSetAsync`. | ✓ tests pass |
| 2 | Critical | `PRCHECK_REPO_NOT_COVERED` (24 chars) exceeds `HasMaxLength(20)` on `pr_checks.State`; crashes on Postgres at runtime | Shortened sentinel to `REPO_NOT_COVERED` (16 chars). Updated all three locations: service implementation and two test assertion strings. | ✓ tests pass |
| 3 | High | Silent fallback to hardcoded `"default-dev-state-signing-key-32ch"` when `StateSigningKey` is empty; anyone knowing the default can forge valid state tokens | Removed hardcoded fallback from both `GetInstallUrl` and `HandleCallback`. Both now return 500 if `StateSigningKey` is empty. Added `.Validate` rule to `GitHubAppOptions` in `Program.cs`: when `AppId != 0`, `StateSigningKey` must be set. | ✓ tests pass |
| 4 | Medium | `dynamic pgEx = ex.InnerException!` bypasses compile-time type safety; risks `RuntimeBinderException` if Npgsql assembly is not loaded | Replaced with reflection: `ex.InnerException.GetType().GetProperty("SqlState")?.GetValue(ex.InnerException) as string`. No dynamic dispatch, returns `null` on miss rather than throwing. | ✓ tests pass |
| 5 | Low | Reconciler test only seeds empty `repo_set` and exercises the "add" path; no test for removal drift | Added `Reconciler_removes_repos_absent_from_github_response` test that seeds `[A, B]`, sets fake API to return only `[A]`, runs the reconciler, and asserts `repo_set == [A]`. Written in RED first (caught finding #1), then GREEN after the fix. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet build src/ApiTool.Backend` | PASS (0 warnings) |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1051 passed, 8 skipped) |
| GitHub installation filter tests | PASS (33 passed) |
| Coverage | >80% (full suite 1051 tests pass) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 9be9ad6d | fix(github): reconciler replaces repo_set instead of only adding | #1, #5 |
| 05a155dd | fix(github): shorten pr_check state sentinel to REPO_NOT_COVERED | #2 |
| f95fa9be | fix(github): require StateSigningKey at startup; remove hardcoded fallback | #3 |
| 773c566c | fix(github): replace dynamic PostgresException dispatch with reflection | #4 |

## Summary

5/5 findings resolved. 0 deferred.
