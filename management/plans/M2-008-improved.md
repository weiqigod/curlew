# Improvement Report: M2-008

**Task:** Auth profile configuration and execution
**Date:** 2026-03-27
**Review:** management/reviews/M2-008-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `RunForExtraction` had 0% test coverage — scope-diff logic, assertion-failure detection, recursive prevention, and file-parsing path all untested. | Added `TestRunForExtraction` in `internal/runner/runner_test.go` with 5 subtests: file-not-found, scope-diff extraction, network failure, assertion failure, recursive prevention. Coverage: 0% → 92%. | ✓ all subtests pass |
| 2 | Medium | `ErrProfileNotFound` exported but never returned by any code, dead API surface. | Removed the sentinel from `internal/auth/profile.go`. No callers existed. | ✓ build and lint clean |
| 3 | Low | Behavior #3 (auth profile fails → exit code 5) had no smoke/integration test. | Changed `runCmd` (main.go:278) to use `currentTier()` instead of hardcoded `auth.TierFree` (consistent with `vaultCmd`). Added `TestRunCmd_AuthProfileFailure_exitCode5` in `main_test.go` that overrides tier to Solo and verifies exit code 5 when the auth collection file does not exist. | ✓ test passes |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (overall) | 91.5% (up from 90.6%) |
| Coverage (`internal/runner`) | 90.2% (RunForExtraction: 92%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 87abdf6 | fix(auth): remove unused ErrProfileNotFound sentinel | #2 |
| 6b3149e | test(runner): add TestRunForExtraction covering all execution paths | #1 |
| dd4af65 | fix(cli): wire currentTier() in runCmd + test auth profile failure exit 5 | #3 |

## Summary

3/3 findings resolved. 0 deferred.
