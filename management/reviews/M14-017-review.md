# Code Review: M14-017

**Task:** Backend: github_installations table + dashboard + webhook-first claim flows
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-017-github-installations
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All 5 findings from iteration 1 have been resolved.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|----------------|
| — | — | — | — | — | No findings | — |

## Previous Findings — Verified Resolved

| # | Severity | Finding (Iteration 1) | Resolution Verified |
|---|----------|-----------------------|---------------------|
| 1 | Critical | Reconciler only grew `repo_set` (add-only); drift could never shrink | `ReplaceRepoSetAsync` added to `GithubInstallationsService`; reconciler calls it; computes removals via `ApplyRepoRemovedAsync` before additions. ✓ |
| 2 | Critical | `PRCHECK_REPO_NOT_COVERED` (24 chars) exceeded `HasMaxLength(20)` on `pr_checks.State` | Shortened to `REPO_NOT_COVERED` (16 chars) in `ApplyRepoRemovedAsync` and both test assertions. ✓ |
| 3 | High | Hardcoded fallback `"default-dev-state-signing-key-32ch"` silently used when `StateSigningKey` was empty | Fallback removed from `GetInstallUrl` and `HandleCallback`; both return 500 if key is empty; `Program.cs` `.Validate` rule enforces non-empty key when `AppId != 0` at startup. ✓ |
| 4 | Medium | `dynamic` dispatch to access `PostgresException.SqlState`; risk of `RuntimeBinderException` | Replaced with reflection (`GetType().GetProperty("SqlState")?.GetValue(ex.InnerException) as string`); no dynamic, returns `null` on miss. ✓ |
| 5 | Low | Reconciler test exercised only the add-only path; no removal drift test | `Reconciler_removes_repos_absent_from_github_response` seeds `[A, B]`, fakes API returning `[A]`, asserts final `repo_set == [A]`. ✓ |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Errors wrapped and returned; sentinel enum `InstallClaimError` for well-known failures; `DbUpdateException` caught and translated at claim paths; no swallowed errors. |
| Input Validation | PASS | Missing `state`/`installation_id` → 401; invalid `org_id` format → 400; unauthenticated → 401; empty `StateSigningKey` → 500 with startup validation. |
| Naming | PASS | No stuttering; all exported types, methods, and enum members carry doc comments; `IGitHubInstallationsApi` follows `-er` convention. |
| Code Organization | PASS | Clean layering: entity → service (business logic) → handlers (dispatch) → endpoints (HTTP); background service scoped correctly; stub registered outside Testing only. |
| Correctness | PASS | `ReplaceRepoSetAsync` correctly computes set-difference and set-union; `REPO_NOT_COVERED` fits within MaxLength(20); state token HMAC uses constant-time comparison. |
| Test Quality | PASS | All 8 behaviors covered by tests; 33 dedicated tests; migration constraint tests use real SQLite (not InMemory); reconciler tests use shared SQLite connection; FakeGitHubInstallationsApi with `CallCount` assertion. |

## Test Coverage
- Coverage: >80% (backend suite: 1051 tests pass; 33 new M14-017 tests)
- All behaviors from task YAML verified by at least one test
- DoD items satisfied: ≥12 filter tests pass (33 actual), cross-tenant UNIQUE-index test, reconciler fake-API test, runbook section in README, spec refs in handler file headers

## Summary

All five findings from the first review have been cleanly resolved. The implementation correctly replaces `repo_set` with the authoritative GitHub list during reconciliation, enforces a column-safe sentinel for pr_check state, requires `StateSigningKey` at startup and rejects requests if unconfigured, and uses safe reflection for Npgsql exception inspection. The test suite is comprehensive — 33 tests covering all 8 specified behaviors, with real SQLite for constraint enforcement and a dedicated fake API client for the reconciler.
