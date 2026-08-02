# Code Review: M16-014

**Task:** Outbound IGitLabCheckPoster and pr_checks provider discriminator
**Reviewer:** AI
**Date:** 2026-05-11
**Branch:** feature/M16-014-gitlab-check-poster
**Iteration:** 3 (post-improve pass 2)

## Verdict: PASS

## Findings

No findings. All three findings from iteration 2 have been resolved:

| # | Prior Finding | Status |
|---|---------------|--------|
| 1 | `PostsSuccess_PrivateTokenHeader_IsDecryptedPat` was hollow — no header assertion | `FakeHttpMessageHandler.LastRequestHeaders` (OrdinalIgnoreCase dict) now captures all non-standard request headers. Test now asserts `LastRequestHeaders["Private-Token"] == FakePat`. **RESOLVED.** |
| 2 | Missing `InstallationSoftDeleted_ReturnsFailed_GitLabNoInstallation` test | Test added: seeds `GitLabInstallation` with `DeletedAt` set, asserts `Failed + GitLabNoInstallation + zero HTTP calls`. **RESOLVED.** |
| 3 | 401 test used `NullAuditWriter`; `audit.Append("gitlab.pat.revoked")` unverified | `SpyAuditWriter` added; 401 test replaced `NullAuditWriter` with spy and asserts `ContainSingle(e => e.EventType == "gitlab.pat.revoked")`. **RESOLVED.** |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `catch (JsonException)` for optional status-id parse is narrow and correct. `catch (Exception)` in `SendAsync` path is intentional (HttpClient throws `HttpRequestException`, `TaskCanceledException`, etc.) and logs+queues rather than swallowing. Endpoint-level `catch (Exception)` is pre-existing (M14-018 scope). All `MarkFailed`/`MarkStatus` helpers save changes and return without swallowing. |
| Input Validation | PASS | Provider enum gated at endpoint; head_sha 40-char required; target_url HTTPS-only with drop-and-warn for non-HTTPS; HTTP base URL rejected unless `AllowHttp=true`. |
| Naming | PASS | No stuttering; doc comments on all exported types and methods; `GitLabStateMapper`, `IGitLabRateLimitTracker`, `GitLabCheckPoster` are clear and appropriately scoped. |
| Code Organization | PASS | Package boundaries respected; state-mapping in its own pure-function type; rate-limit tracker isolated; poster implementation clean. |
| Correctness | PASS | State-mapping table matches spec :9233-9242; description truncation to 255 chars with `…(truncated)` is correct; PAT revoked race condition is safe (idempotent write); `TrimEnd('/')` on base URL prevents double-slash URL construction. |
| Test Quality | PASS | All nine behaviors covered; happy path, error paths, edge cases tested; table-driven theory tests for state mapping; spy audit writer for security-incident path; retry ceiling (5 attempts) tested explicitly. |

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Migration applies with `provider TEXT NOT NULL DEFAULT 'github'`, nullable FK/bigint columns | `PrCheckEntitySchemaTests`, `PrCheckProviderMigrationTests` |
| 2 | Existing rows default to `provider='github'` | `ExistingRow_WithoutProvider_DefaultsToGithub` (entity + migration variants) |
| 3 | `success → success`, name=Curlew, description≤255, target_url HTTPS | `PostsSuccess_BodyContains_State_Name_Description_Context`, `StateMapping_TableDriven` |
| 4 | `timed_out → failed` with `[timed out]` prefix | `StateMapping_TableDriven`, `Description_LongerThan255_TruncatedWithMarker` |
| 5 | `neutral/skipped → success` with `neutral:` / `skipped:` prefix | `StateMapping_TableDriven` |
| 6 | 401 → `access_token_revoked_at` set, audit `gitlab.pat.revoked`, `status=gitlab_token_revoked` | `Returns401_MarksAccessTokenRevoked_AndSetsStatus_GitLabTokenRevoked` (with `SpyAuditWriter`) |
| 7 | RateLimit-Remaining=0 + Reset → backs off | `Returns429_QueuesRow_AndUpdatesRateLimitTracker`, `GitLabRateLimitTrackerTests` |
| 8 | Idempotency relies on GitLab side; no dedup logic needed | Noted in plan Decision G; GitLab's own idempotency on `(commit_sha, name, context)` |
| 9 | HTTP base URL rejected unless `GITLAB__ALLOW_HTTP=true` | `HttpBaseUrl_WithoutAllowHttp_RejectsBeforeNetworkCall`, `HttpBaseUrl_WithAllowHttpTrue_AllowsHttp` |

## Test Coverage

- `GitLabStateMapper`: All 6 CLI states, invalid inputs, description prefix markers, truncation at boundary (255), exactly-at-limit case, null original description — 100% line coverage.
- `GitLabRateLimitTracker`: Not-blocked default, retry-after backoff, reset-time backoff, above-watermark (no block), after-reset, clear, two-installation independence — 100% line coverage.
- `GitLabCheckPoster`: Happy path, Private-Token header, state-mapping (6 states), description truncation, 401 with audit, 429 rate-limit, 404/422 permanent failure, 5xx first-attempt queue, 5xx retry ceiling (5 attempts), HTTP insecure URL (allow and deny), HTTPS base URL, non-HTTPS target_url dropped, HTTPS target_url included, pre-revoked short-circuit, soft-deleted installation, no installation FK, pre-blocked installation — ~95%+ branch coverage with no known untested paths.
- `PrChecksProblem` GitLab methods: 5/5 new methods tested for status code, RFC-7807 `code`, and `type` fields.
- `PrCheckEntitySchemaTests`: Provider default + required, CHECK constraint SQL, FK nullable, FK on-delete-set-null, GitLabStatusId nullable, back-compat default on insert, cascade-on-hard-delete.
- `PrCheckProviderMigrationTests`: Default github on insert after MigrateAsync, gitlab round-trip, FK on-delete-set-null.
- `PrChecksUploadEndpointTests`: Provider=github routing, missing provider defaults to github, invalid provider 400, gitlab no-installation 404, gitlab row-persistence with FK.

## Summary

All three iteration-2 findings are resolved cleanly: the Private-Token header is now verifiably asserted via the extended `FakeHttpMessageHandler.LastRequestHeaders` dictionary; the soft-deleted installation path has a dedicated test; and the 401 audit-event path is verified with a `SpyAuditWriter`. The implementation is correct, well-structured, and thoroughly tested. No new findings were identified in this pass. The task is ready for `/verify`.
