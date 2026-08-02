# Improvement Report: M1-017

**Task:** Global project config (curlew.yaml)
**Date:** 2026-03-14
**Review:** management/reviews/M1-017-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `TestRun_ProjectVariables` missing `dotenv overrides project` case (precedence 4 > 2) | Added `dotEnv` field to test struct; added new case; wired `DotEnv: tc.dotEnv` into `VarSources` | ✓ tests pass |
| 2 | Low | `TestFindProjectRoot` grandparent case only asserts `found == true`, not the returned path | Added `wantRootFn func(startDir string) string` field; grandparent case asserts `root == filepath.Dir(filepath.Dir(startDir))`; changed `_, found :=` to `root, found :=` in loop | ✓ tests pass |
| 3 | Low | `TestLoadProjectConfig` asserts `root != ""` but not actual path value | Added `wantRootFn func(startDir string) string` field; `loads from project root` and `yml extension supported` cases assert `root == startDir` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 92.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 668fdfc | test(runner,config): strengthen assertions in project variable tests | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
