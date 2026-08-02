# Code Review: M2-009

**Task:** Auth profile token caching and refresh
**Reviewer:** AI
**Date:** 2026-03-29
**Branch:** feature/M2-009-auth-token-caching

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; best-effort cache operations correctly silenced with `_`; sentinel errors used for all cache states |
| Input Validation | PASS | `nil` cache → `NopCacheStore`; empty `projectRoot` → `NopCacheStore`; collection-field mismatch checked by caller; negative TTL not possible via YAML int |
| Naming | PASS | No stuttering; short names in tight scopes; all exported symbols have doc comments; `-er` suffix on `CacheStore` interface |
| Code Organization | PASS | `internal/` boundaries respected; atomic write via `CreateTemp`+`Rename`; no goroutine leaks; `defer` used for scope/request lifecycle |
| Correctness | PASS | Cache-hit deobfuscation partial-failure correctly invalidates and falls through; refresh-on-failure at-most-once retry enforced; teardown always runs; context propagated throughout |
| Test Quality | PASS | All behaviors covered; `MkdirAll` failure test added; overall coverage auth 87.7%, runner 91.8%, config 96.5% — all above 80% threshold |

## Test Coverage
- `internal/auth`: 87.7%
- `internal/runner`: 91.8%
- `internal/config`: 96.5%
- Missing coverage: `FileCacheStore.Save` OS-injection paths (CreateTemp/Write/Close/Rename failures) — require OS-level injection, explicitly deferred in previous review cycle

## Previous Findings Status

| # | Previous Finding | Status |
|---|-----------------|--------|
| 1 | `MkdirAll` failure path untested in `FileCacheStore.Save` | ✅ Fixed — `"save fails when cache dir is a file"` test added; coverage improved from 47.4% → 52.6% for `Save` |

## Spec Behavior Coverage

| Behavior | Covered By | Status |
|----------|-----------|--------|
| `cache_ttl: 3600` prevents re-execution within TTL | `TestExecuteProfiles_Caching/"cache hit skips execution"`, `TestRun_AuthProfileCaching` | ✅ |
| Expired cache causes re-execution | `TestExecuteProfiles_Caching/"expired cache re-executes"` | ✅ |
| `refresh_on_failure: true` retries on 401 | `TestExecutePhase_RefreshOnFailure` (6 cases) | ✅ |
| Shared cache across sequential runs | `TestRun_AuthProfileCaching` (second run hits file cache) | ✅ |
| Sensitive values obfuscated in cache file | `TestObfuscate` (roundtrip + distinctness), `TestFileCacheStore/"save and load roundtrip"` | ✅ |

## Summary

All findings from the previous two review cycles are resolved. The implementation is production-quality: atomic cache writes, correct TTL/expiry handling, full error wrapping throughout, refresh-on-failure at-most-once retry, NopCacheStore fallback, and `MkdirAll` failure now tested. Package coverage is well above the 80% threshold across all three changed packages. No new issues found.
