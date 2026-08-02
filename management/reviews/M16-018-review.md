# Code Review: M16-018

**Task:** CLI GetTeamVault with cache, --refresh-vault flag, and load-site enforcement
**Reviewer:** AI
**Date:** 2026-05-12
**Branch:** feature/M16-018-cli-team-vault-cache
**Iteration:** 3 (post-improve, iteration 2)

## Verdict: PASS

## Findings

No findings. All three findings from iteration 2 have been resolved:

1. **Finding #1 (Medium) — FIXED**: `ErrOrgIDRequired` sentinel is now verified via `errors.Is(gotErr, backend.ErrOrgIDRequired)`. The old short-circuit `if tc.orgID == "" && gotErr != nil { return }` has been removed. The sentinel is now properly matched on the standard path.

2. **Finding #2 (Medium) — FIXED**: `orgIDRE` path-traversal guard is now tested via the `"invalid orgId format rejects path traversal"` table row with `orgID: "org/evil"`. The test verifies the error message contains `"invalid orgId"` via a substring check, which is appropriate for a non-sentinel fmt.Errorf-produced error.

3. **Finding #3 (Low) — FIXED**: `vaultCache.Refresh(...)` in `cmd/curlew/license.go:412` now passes `stderr` as the warn writer instead of `io.Discard`, so failures during the team vault cache refresh are surfaced to the user via stderr.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. Sentinels `ErrTeamVaultNotFound` and `ErrOrgIDRequired` defined and registered in `hints_init.go`. Non-fatal vault fetch paths (stale cache, license --refresh) handled correctly. |
| Input Validation | PASS | `orgIDRE` guard present, tested, and correct. Empty-orgId guard uses the `ErrOrgIDRequired` sentinel. `Fetcher` nil-safe for TTL-hit path. |
| Naming | PASS | No stuttering. Exported symbols have doc comments. `Fetcher` interface uses `-er` suffix. `backendVaultFetcher` adapter is unexported as appropriate. |
| Code Organization | PASS | `internal/` boundaries respected. `backendVaultFetcher` adapter in `cmd/curlew` keeps `teamtemplate` decoupled from `internal/backend`. Loader, cache, and transport are cleanly separated. |
| Correctness | PASS | `--refresh-vault` wired in both `run` and `worker`. `Claims` threaded into `LoadOptions`. Runner gate upgraded to `CheckFeatureWithClaims`. Flock serializes concurrent cache writes. `fetchCtx` 2s timeout applied only to the network fetch, not the lock acquisition (correct). |
| Test Quality | PASS | All 10 task behaviors covered by at least one test. Table-driven tests throughout. Concurrent flock test mirrors the JWKS cache pattern. File-mode test for 0600. Integration test verifies `team_vault.json` shape + mode. |

## Test Coverage

- `internal/backend`: 84.4% ✓
- `internal/vault/teamtemplate`: 88.3% ✓
- `internal/license`: 88.7% ✓
- `internal/runner`: 84.8% ✓
- `cmd/curlew`: 81.4% ✓
- All changed packages above 80% floor ✓

## Behavior Coverage

All 10 behaviors from the task YAML are covered:

1. `GetTeamVault` transport → `TestGetTeamVault` + `TestGetTeamVault_PathContainsOrgID`
2. Cache writes `{fetched_at, version, template}` mode 0600 → `TestCache_Load_FetchedFileMode0600` + `TestLicenseRefresh_TeamTier_WritesTeamVaultCache`
3. TTL hit → no network call → `TestCache_Load/ttl_hit_no_network`
4. Stale + reachable → foreground fetch → `TestCache_Load/ttl_miss_fetch_succeeds` + `TestCache_FetchTimeout_FallsBackToStale`
5. Stale + unreachable → use cached + warning → `TestCache_Load/stale_offline_returns_cached_with_warning`
6. `--refresh-vault` bypasses TTL → `TestRunCmd_RefreshVault_BypassesTTL`
7. Free tier gate fires on backend cache → `TestRunCmd_TeamSecrets_FreeTierBackendOnlyBlocked` + `TestLoad_Gate_FreeTierBlocked`
8. Team/Enterprise tier passes gate → `TestLoad_Gate_TeamTierPasses` + `TestLoad_Gate_EnterpriseTierPasses`
9. `license --refresh` fetches team vault → `TestLicenseRefresh_TeamTier_WritesTeamVaultCache`
10. Concurrent flock serialization → `TestCache_Concurrent_OneNetworkCallUnderFlock`

## Summary

All three findings from the previous review iteration have been cleanly resolved. The implementation is complete, well-tested, and meets all standards. The feature gate, cache TTL logic, flock concurrency control, and loader precedence chain all work correctly. Coverage exceeds 80% in all changed packages.
