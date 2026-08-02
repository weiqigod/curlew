# Improvement Report: M16-014

**Task:** Outbound IGitLabCheckPoster and pr_checks provider discriminator
**Date:** 2026-05-11
**Review:** management/reviews/M16-014-review.md
**Iteration:** 2 (post second review pass)

## Previously Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Bare `catch {}` in `GitLabCheckPoster.cs` silently swallowed all exceptions | Changed to `catch (JsonException)` | ✓ tests pass |
| 2 | Medium | `Returns5xx_QueuesForRetry` assertion too weak; missing retry-ceiling escalation test | Added `Returns5xx_MarksFailed_AfterFiveAttempts`; tightened assertion | ✓ tests pass |
| 3 | Medium | Missing `Post_ProviderGitlab_PersistsRowWithProviderGitlab_AndInstallationFk` endpoint test | Added full integration test asserting `Provider="gitlab"` and `GitLabInstallationId` | ✓ tests pass |
| 4 | Low | `FakeGitLabCheckPoster.Posted()` used `null` Code instead of `PrCheckErrorCode.None` | Fixed to `PrCheckErrorCode.None` | ✓ tests pass |
| 5 | Low | `CheckRunPostResult.CheckRunId` doc inaccurate for GitLab rows | Updated doc comment | ✓ tests pass |
| 6 | Low | `PrCheckUploadResponse.GithubCheckRunId` doc lacked GitLab guidance | Updated doc comment | ✓ tests pass |

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `PostsSuccess_PrivateTokenHeader_IsDecryptedPat` was hollow — only asserted `LastRequestBody.NotBeNullOrEmpty()`, not the `Private-Token` header | Extended `FakeHttpMessageHandler` with `LastRequestHeaders` (OrdinalIgnoreCase dictionary) capturing all non-standard request headers; rewrote test to assert `LastRequestHeaders["Private-Token"] == FakePat` | ✓ tests pass |
| 2 | Low | Missing `InstallationSoftDeleted_ReturnsFailed_GitLabNoInstallation` test (code path existed on line 49 of `GitLabCheckPoster.cs`) | Added test seeding `GitLabInstallation` with `DeletedAt` set; asserts `Failed` + `GitLabNoInstallation` + zero HTTP calls | ✓ tests pass |
| 3 | Low | `Returns401` test used `NullAuditWriter`; audit log write-path (`event_type = "gitlab.pat.revoked"`) unverified | Added `SpyAuditWriter` recording stub; updated 401 test to use spy and assert `ContainSingle(e => e.EventType == "gitlab.pat.revoked")` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1637 passed, 8 skipped) |
| `golangci-lint run` | PASS (0 issues) |
| `GitLabCheckPoster` targeted filter (24 tests) | PASS |
| Full filter `~GitLabCheckPoster\|PrCheckProvider\|…` (83 tests) | PASS |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `24fbf4e0` | fix(pr-checks): narrow bare catch; fix FakeGitLabCheckPoster Code; update doc comments | iter1 #1, #4, #5, #6 |
| `f08a0d0d` | test(pr-checks): add retry-ceiling test and endpoint row-persistence test | iter1 #2, #3 |
| `4af1c079` | fix(tests): resolve all three review findings for M16-014 | iter2 #1, #2, #3 |

## Summary

Iteration 2: 3/3 findings resolved. 0 deferred.
Cumulative: 9/9 total findings across both review iterations resolved.
