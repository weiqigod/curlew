# Improvement Report: M16-018

**Task:** CLI GetTeamVault with cache, --refresh-vault flag, and load-site enforcement
**Date:** 2026-05-12
**Review:** management/reviews/M16-018-review.md

## Iteration 1 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `schedCfg.refreshVault` parsed but never read in `runSchedulePull` — vault cache never force-refreshed on worker startup | Added vault refresh block in `runSchedulePull`: imports `teamtmpl`, parses JWT for `OrgID`, calls `vaultCache.Refresh` when `sc.refreshVault && cfgDir != ""` | ✓ tests pass |
| 2 | High | `LoadOptions.Claims` not set in `loadTeamTemplate()` — trial-active users blocked at the CLI call site despite passing unit tests | Captured `claims = &tok.Claims` alongside `orgID` in the license-store read path; passed as `Claims: claims` in `LoadOptions` | ✓ tests pass |
| 3 | Medium | `printWorkerHelpTo` missing `--refresh-vault` (DoD violation) | Added `--refresh-vault` line under "Schedule pull" section in `printWorkerHelpTo` | ✓ tests pass |
| 4 | Medium | `TestRunCmd_RefreshVault_BypassesTTL` planned but absent | Added integration test: seeds fresh cache, starts httptest backend, runs with/without `--refresh-vault`, asserts backend called only with flag | ✓ tests pass |
| 5 | Medium | Plan-specified path-traversal guard missing from `GetTeamVault` | Added `orgIDRE = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)` and validation before path construction | ✓ tests pass |
| 6 | Low | `itoa` helper needlessly complex (wrapping a closure in two no-op `strings.TrimRight` calls) | Replaced with `import "strconv"` and `strconv.Itoa(n)` | ✓ tests pass |
| 7 | Low | Empty-orgId guard returns bare `errors.New` rather than a matchable sentinel | Introduced `var ErrOrgIDRequired = errors.New("backend: orgId is required")` and registered it in `hints_init.go` | ✓ tests pass |

## Iteration 2 — Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Empty-orgId test used `tc.orgID == ""` short-circuit, so `ErrOrgIDRequired` was never verified via `errors.Is` | Changed `wantErr` to `backend.ErrOrgIDRequired`; removed the short-circuit; standard `errors.Is` path now exercises the sentinel | ✓ tests pass |
| 2 | Medium | `orgIDRE` path-traversal validation path had zero test coverage | Added test case `"invalid orgId format rejects path traversal"` with `orgID: "org/evil"`; error message checked via `strings.Contains` fallback | ✓ tests pass |
| 3 | Low | `vaultCache.Refresh(...)` in `licenseRefreshOut` passed `io.Discard` instead of `stderr` — vault cache refresh failures silently dropped | Replaced `io.Discard` with `stderr` on `cmd/curlew/license.go:412` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/backend` | 84.4% |
| Coverage `internal/vault/teamtemplate` | 88.3% |
| Coverage `internal/license` | 88.7% |
| Coverage `internal/runner` | 84.8% |
| Coverage `cmd/curlew` | 81.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 79453882 | fix(cli): resolve all review findings for M16-018 | iter-1 #1, #2, #3, #5, #6, #7 |
| f03f3454 | fix(cli): add missing sentinel registration and bypass test for M16-018 | iter-1 #4 |
| fe51164b | fix(backend): assert ErrOrgIDRequired sentinel and add invalid orgId test coverage | iter-2 #1, #2, #3 |

## Summary

Iteration 1: 7/7 findings resolved. 0 deferred.
Iteration 2: 3/3 findings resolved. 0 deferred.
