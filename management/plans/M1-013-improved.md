# Improvement Report: M1-013 (Re-review)

**Task:** .env file loading for local secrets
**Date:** 2026-03-12
**Review:** management/reviews/M1-013-review.md (re-review)

## Resolved Findings

### From initial review (resolved in prior /improve cycle)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `os.IsNotExist(err)` deprecated | Replaced with `errors.Is(err, os.ErrNotExist)` | ✓ tests pass |
| 2 | Low | Missing `errors.Is(err, ErrInvalidDotenv)` assertion in LoadDotenv test | Added `wantSentinel` field to LoadDotenv table-driven test | ✓ tests pass |
| 3 | Low | Untested unreadable file branch in LoadDotenv | Added `TestLoadDotenv_unreadable_file` | ✓ tests pass |

### From re-review (resolved in this /improve cycle)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `TestParseDotenv` error cases don't verify `errors.Is(err, ErrInvalidDotenv)` | Added `wantSentinel` field to ParseDotenv table-driven test, matching LoadDotenv pattern | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (total) | 93.3% |
| Coverage (config package) | 94.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fcc24ff | refactor(config): replace deprecated os.IsNotExist with errors.Is | Initial #1 |
| eb4a6ae | test(config): strengthen LoadDotenv error assertions and branch coverage | Initial #2, #3 |
| 81b4849 | test(config): add sentinel error assertions to TestParseDotenv | Re-review #1 |

## Summary
4/4 findings resolved across two review cycles. 0 deferred.
