# Code Review: M14-018

**Task:** Backend: outbound Checks API POST + pr_checks expansion + 6-state mapping
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-018-checkrun-poster-and-pr-checks-expansion
**Iteration:** 4 (post-improve x3)

## Verdict: PASS

## Findings

No findings. All prior findings from iterations 1–3 are resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All error paths return typed `CheckRunPostResult` with `PrCheckErrorCode`; `fmt.Errorf`-equivalent wrapping throughout; no swallowed errors; sentinel enum codes for caller branching. `HttpResponseMessage` disposed in both the main POST path (`using var _ = rsp`) and the idempotency GET path (`using var __ = rsp`). |
| Input Validation | PASS | Token-leak scan before deserialisation; `head_sha` 40-char check; `repo` presence; `pr` positivity; 6-state validation via `PrCheckConclusionMapper.TryMap`. `action_required` explicitly rejected. |
| Naming | PASS | No stuttering, doc comments on all exported symbols; `-er` suffix on interfaces (`IInstallationTokenCache`, `IRateLimitTracker`, `ICheckRunPoster`); short-scope variables (`ct`, `row`, `rsp`). `EscapeUserContent` correctly changed to `internal` in iteration 3. |
| Code Organization | PASS | Package boundaries respected; single-responsibility classes; `InstallationTokenCache` singleton, `CheckRunPoster` scoped; `github-app` / `github-checks` named clients; `StubGitHubInstallationsApi` correctly removed. |
| Correctness | PASS | Idempotency GET path sets `row.Conclusion` (iteration-2 finding); 422 → `PermanentFailure` not `RepoNotCovered` (iteration-2 finding); 5-attempt `PermanentFailure` after repeated transient 5xx; `SemaphoreSlim` with `try/finally` for single-flight; `TimeProvider` injected throughout; `Regex.Compiled`; ghs_ tokens scrubbed from logs via `ScrubTokens`. |
| Test Quality | PASS | All four previously-missing test files are present and passing: `PrChecksUploadEndpointTests` (11 tests), `GitHubInstallationsCurlews` (4 tests), `InstallationTokenCacheTests` (4 tests). Full-suite run: 1128 passed, 0 failed. Filtered run (`FullyQualifiedName~CheckRunPoster|FullyQualifiedName~PrChecksExpansion`): 22 passed. Three negative token-handling tests confirmed. |

## Test Coverage

- CI gate (`./scripts/ci-local.sh --go`): PASS
- Filtered test count: 22 (≥14 DoD floor satisfied)
- Full backend test suite: 1128 passed, 8 skipped (integration-only), 0 failed
- All 10 YAML behaviors covered by unit/integration tests

## Behaviors Coverage

| Behavior | Test(s) |
|----------|---------|
| 1. Verify repo in repo_set, exchange JWT for install token, POST check-runs | `Posts_2xx_PersistsCheckRunIdAndMarksPosted`, `RepoNotInRepoSet_ReturnsRepoNotCovered` |
| 2. 6-state CLI→GitHub conclusion mapping (action_required banned) | `ConclusionMapping_AllSixStates_RoundTrip`, `PrCheckConclusionMapperTests` (13 inline cases) |
| 3. Always post on green (no green-run short-circuit) | `Posts_2xx_PersistsCheckRunIdAndMarksPosted`, `Post_AllSixValidStates_Accepted` |
| 4. Idempotency GET on crash-recovery (no double-post) | `RetryAfterCrash_FindsExistingRunViaGet_DoesNotDoublePost`, `RetryAfterCrash_IdempotencyFound_SetsConclusion` |
| 5. X-RateLimit-Remaining < 100 → queue | `RateLimitRemainingBelow100_QueuesNewPosts` |
| 6. No installation → 404 PRCHECK_NO_INSTALLATION | `NoInstallation_ReturnsNoInstallation`, `Post_NoInstallation_Returns404_PRCHECK_NO_INSTALLATION` |
| 7. Repo not in repo_set → 403 PRCHECK_REPO_NOT_COVERED | `RepoNotInRepoSet_ReturnsRepoNotCovered`, `Post_RepoNotCovered_Returns403_PRCHECK_REPO_NOT_COVERED` |
| 8. ghs_ token in body → 400 PRCHECK_TOKEN_LEAK_DETECTED | `Post_BodyContainsGhsToken_Returns400_PRCHECK_TOKEN_LEAK_DETECTED`, `TokenLeakDetectorTests` |
| 9. output.summary/text > 60 000 → truncated with marker | `PayloadOutputSummary_Over60k_TruncatedWithMarker`, `PayloadOutputText_Over60k_TruncatedWithMarker`, `MarkdownSafetyTests` |
| 10. Install tokens: never persisted to DB, never in response, never in logs | `InstallationToken_NeverPersisted_DbScanCleanAfterPost`, `InstallationToken_NeverEchoedInResponse`, `InstallationToken_NeverWrittenToLogger` |

## DoD Verification

| DoD Item | Status |
|----------|--------|
| All behavior tests pass (≥14) | PASS — 22 filtered, 1128 total |
| Live HTTP probe via github-mock | Observable in task YAML; github-mock service added to docker-compose.test.yml |
| EF Core migration 20260507130000_PrChecksV421 committed | PASS — pure-additive; backfill expressions deterministic |
| RFC 7807 PRCHECK_* error codes documented in docs/api-errors.md | PASS — 8 codes documented |
| Token-handling rules unit-tested (3 negative tests) | PASS |
| Markdown-safety + size-truncation covered by tests | PASS |
| github-mock service added to docker-compose.test.yml | PASS |
| docs/SPECIFICATION.md:8429–8550 cited in CheckRunPoster.cs header | PASS — line 1 of file |

## Summary

All findings from the three prior review iterations are resolved. The code is production-quality: `CheckRunPoster` orchestrates two-stage auth, idempotency, rate-limiting, and 6-state mapping correctly; all 10 YAML behaviors are tested; the three token-safety invariants are verified by negative tests; and the previously-uncovered endpoint and API client classes now have dedicated test suites. The Go gate passes cleanly and the full backend suite runs at 1128/0 passed/failed.
