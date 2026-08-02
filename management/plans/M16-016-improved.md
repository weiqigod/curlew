# Improvement Report: M16-016 (Iteration 6)

**Task:** Web /integrations/gitlab dashboard page with PAT submission and project lookup
**Date:** 2026-05-11
**Review:** management/reviews/M16-016-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `UriFormatException` in `GitLabProjectLookup.LookupAsync` when `gitlabBaseUrl` is not a valid absolute URI; exception escaped the `catch` block causing 500 instead of 400 | Added `Uri.TryCreate` pre-validation guard before the `new Uri(...)` construction. Added `InvalidBaseUrl` to `GitLabProjectLookupStatus` enum. Added `InvalidBaseUrl` case in `CreateInstallation` switch returning `BadRequest400("invalid_base_url", ...)`. Added unit tests covering 3 invalid-URL cases and an endpoint integration test `POST_lookup_InvalidBaseUrl_returns_400_invalid_base_url` | ✓ 1734 backend tests pass |
| 2 | Low | 403 → `Unauthorized` branch in `GitLabProjectLookup` had no unit test | Added `Maps_403_to_Unauthorized` unit test asserting `GitLabProjectLookupStatus.Unauthorized` on `HttpStatusCode.Forbidden` response | ✓ tests pass |
| 3 | Low | 5xx → `Unreachable` branch in `GitLabProjectLookup` had no unit test | Added `Maps_5xx_to_Unreachable` unit test asserting `GitLabProjectLookupStatus.Unreachable` on `HttpStatusCode.ServiceUnavailable` (503) response | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| `dotnet build ApiTool.Backend` | PASS (0 errors, 0 warnings) |
| `dotnet test ApiTool.Backend.Tests` | PASS (1734 passed, 0 failed, 8 skipped) |
| Go coverage (total) | 87.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `1b3b0e5b` | fix(gitlab): guard invalid base URL and add missing lookup branch tests | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
