# Verification Report: M2-009

**Task:** Auth profile token caching and refresh
**Verified by:** AI
**Date:** 2026-03-29
**Branch:** feature/M2-009-auth-token-caching
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | 15 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All scenarios pass including auth gate |
| Coverage (total) | 91.4% | Meets >= 80% threshold |
| Coverage (internal/auth) | 87.7% | Meets >= 80% threshold |
| Coverage (internal/runner) | 91.8% | Meets >= 80% threshold |
| Coverage (internal/config) | 96.5% | Meets >= 80% threshold |

## Observable Output

Auth profiles are gated at Solo tier. The smoke test confirms correct gating behaviour (exit 6 with `auth profile gate message shown`). Caching and refresh behaviour is fully verified via unit and integration tests:

```
go test -v -run "TestExecuteProfiles_Caching|TestExecutePhase_RefreshOnFailure|TestRun_AuthProfileCaching" ./internal/auth/... ./internal/runner/...
--- PASS: TestExecuteProfiles_Caching (0.00s)
--- PASS: TestRun_AuthProfileCaching (0.00s)
--- PASS: TestExecutePhase_RefreshOnFailure (0.00s)
ok  github.com/weiqigod/curlew/internal/auth   0.170s
ok  github.com/weiqigod/curlew/internal/runner 0.288s
```

Expected: auth profile caching and refresh verified via unit tests
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `cache_ttl: 3600` prevents re-execution within TTL | `TestExecuteProfiles_Caching/cache_hit_skips_execution`, `TestRun_AuthProfileCaching` | PASS |
| 2 | Expired cache causes re-execution | `TestExecuteProfiles_Caching/expired_cache_re-executes` | PASS |
| 3 | `refresh_on_failure: true` retries on 401 | `TestExecutePhase_RefreshOnFailure/401_with_refresh_on_failure_retries_after_re-auth` | PASS |
| 4 | Cached credentials shared across sequential runs | `TestRun_AuthProfileCaching` (uses real `FileCacheStore` in `t.TempDir()` across two `Run` calls) | PASS |
| 5 | Sensitive values obfuscated in cache file | `TestObfuscate` (roundtrip + distinctness), `TestFileCacheStore/save_and_load_roundtrip` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 15 packages pass | PASS |
| 2 | Observable output works as specified | Unit tests verify caching/refresh; smoke confirms gate | PASS |
| 3 | Test coverage >= 80% | Total 91.4%; auth 87.7%; runner 91.8%; config 96.5% | PASS |
| 4 | No build warnings or lint errors | `go build` clean; `golangci-lint` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | No new user-facing commands added — N/A | PASS |
| 6 | Smoke test updated (if new capability) | Auth gate already covered; caching is internal — N/A | PASS |

## Code Review

Branch A: Review PASS trusted, spot-check clean.

| Check | Status |
|-------|--------|
| Error handling (`%w` wrapping) | PASS — `fmt.Errorf("%w: %w", ...)` throughout `cache.go`; best-effort operations silenced with `_` |
| Doc comments on exports | PASS — all exported types, functions, and vars have doc comments |
| Test quality | PASS — `cache_hit_skips_execution` correctly asserts `wantExecCalls: 0` and `wantToken: "cached_tok"` |

## Commits

| Hash | Message |
|------|---------|
| 344b2d8 | docs(review): add passing review for M2-009 |
| 50f18b9 | docs(review): add improvement report for M2-009 |
| 2d814a0 | test(auth): add Save MkdirAll failure test case |
| 837cac6 | docs(review): add improvement report for M2-009 |
| 9d9322e | docs(review): add improvement report for M2-009 |
| dfc4735 | test(runner): add missing refresh-failure test case and rename cache test |
| e21caab | test(auth): rename misleading cache test to reflect actual behavior |
| 205b722 | fix(auth): wrap underlying errors with %w for full error chain |
| 093ed15 | docs(review): add review with findings for M2-009 |
| 4f40d71 | chore(task): mark M2-009 as review |
| 30574bf | style: fix gofumpt formatting in test files |
| 472e10a | chore: add .curlew/cache/ to .gitignore |
| 52cca05 | feat(runner): implement refresh-on-failure 401 retry with cache invalidation |
| b01a87e | test(runner): add failing tests for refresh-on-failure 401 retry |
| 6f8d498 | feat(runner): add CacheStore to VarSources and wire into auth profile execution |
| b6fecc1 | test(runner): add failing test for auth profile caching across runs |
| 8eed788 | feat(auth): integrate cache lookup and save into ExecuteProfiles |
| 9578985 | test(auth): add failing tests for ExecuteProfiles caching behavior |
| bf376bc | feat(auth): implement CacheStore interface, FileCacheStore, NopCacheStore, and obfuscation |
| c5baa40 | test(auth): add failing tests for CacheStore, FileCacheStore, and obfuscation |
| 4080fb1 | feat(auth): add CacheTTL and RefreshOnFailure fields to Profile struct |
| edd575b | test(config): add failing tests for cache_ttl and refresh_on_failure fields |
| 1e3b677 | chore(task): mark M2-009 as in_progress |
| b451271 | chore(task): mark M2-009 as planned |
| 0716668 | docs(plan): add implementation plan for M2-009 |

All 25 commits reference `Refs: M2-009`. TDD pattern visible: `test(...)` commits precede each `feat(...)` commit.

## Files Changed

| File | Action |
|------|--------|
| `internal/auth/cache.go` | created |
| `internal/auth/cache_test.go` | created |
| `internal/auth/profile.go` | modified — `CacheTTL`, `RefreshOnFailure` fields; `ExecuteProfiles` cache integration |
| `internal/auth/profile_test.go` | modified — caching tests added |
| `internal/config/project.go` | modified — `CacheTTL`, `RefreshOnFailure` parsed from YAML |
| `internal/config/project_test.go` | modified — new field tests |
| `internal/runner/runner.go` | modified — `CacheStore` in `VarSources`; refresh-on-failure retry |
| `internal/runner/runner_test.go` | modified — caching and refresh integration tests |
| `.gitignore` | modified — `.curlew/cache/` excluded |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
