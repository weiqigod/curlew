# Improvement Report: M20-002

**Task:** Latin-script European locale pools (en-GB, fr-FR, es-ES, it-IT, pt-BR, nl-NL, pl-PL, sv-SE, tr-TR)
**Date:** 2026-06-12
**Review:** management/reviews/M20-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Behavior 6 (lazy sync.Once init per SPEC:1110) unimplemented and untested — no test verified the conscious deviation to an eager package-level map | Added `TestLocalePools_Behaviour6_EagerRegistrationEquivalent` in `locale_pools_test.go`. The test asserts all nine pools are registered and fully populated at package init time, documents the architectural decision (KAD-1: tiny in-source slices make lazy loading moot; observable contract is identical), and explicitly maps to Behavior 6. | ✓ tests pass |
| 2 | Low | `"Hart"` in `it-IT` lastNames pool — English/Germanic surname with no Italian origin | Replaced `"Hart"` with `"Ferraro"` (authentic Italian surname) in `locale_pools.go` line 117 | ✓ tests pass |
| 3 | Low | `"Eixample"` in `es-ES` cities pool — a Barcelona district, not a Spanish city | Replaced `"Eixample"` with `"Pamplona"` (proper Spanish city) in `locale_pools.go` line 91 | ✓ tests pass |
| 4 | Low | Redundant `code := code` loop-variable re-declarations in three test loop bodies (`locale_pools_test.go:32`, `dynamic_test.go:5138`, `dynamic_test.go:5246`) — unnecessary since Go 1.22 changed loop-variable semantics; project targets Go 1.24 | Removed all three `code := code` lines | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`go test -cover ./internal/variable/...`) | 97.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 276afe2c | fix(variable): resolve all M20-002 review findings | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.
