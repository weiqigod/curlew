# Improvement Report: M1-014

**Task:** Request-level variables, --env-var, full precedence
**Date:** 2026-03-12
**Review:** management/reviews/M1-014-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `ParseEnvVarFlag` does not validate empty `varName` in mapped form (e.g., `=$OS_VAR`) | Added `if varName == ""` check returning `ErrInvalidEnvVarFlag` | ✓ tests pass |
| 2 | Low | `ParseEnvVarFlag` does not validate empty `envName` after split (e.g., `KEY=`, `KEY=$`) | Added `if envName == ""` check returning `ErrInvalidEnvVarFlag` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 93.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 74fc69b | fix(variable): validate empty varName and envName in ParseEnvVarFlag | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
