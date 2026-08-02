# Improvement Report: M2-001

**Task:** from_command variable source
**Date:** 2026-03-20
**Review:** management/reviews/M2-001-review.md

## Resolved Findings

### Round 1 (previous review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | CommandCache created fresh per Run(), never serves a hit | Added design-intent code comment and documentation test explaining per-run scope | ✓ tests pass |
| 2 | Medium | No integration test for from_command sensitive redaction | Added runner-level test (sensitive set tracking) and CLI-level test (parser populates sensitive set from YAML) | ✓ tests pass |
| 3 | Low | Command executes even when collection value will overwrite result | Added skip check for `col.Variables.Values[name]` before command execution | ✓ tests pass |
| 4 | Low | No test for negative TTL in CommandCache | Added `cache_negative_ttl_no_caching` test case | ✓ tests pass |

### Round 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `sensitive` field uses raw string comparison `fieldVal.Value == "true"` instead of YAML boolean decoding | Changed to `fieldVal.Decode(&sensitive)` with error handling, consistent with `cache` field decoding. Added TDD test with `sensitive: yes` fixture. | ✓ tests pass |

### Round 3 (current review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Inner error wrapped with `%v` instead of `%w` in non-ExitError path of `ExecuteCommand` | Changed `%v` to `%w` for Go 1.20+ multi-`%w` support, making underlying OS/exec error unwrappable | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 90.9% |

## Fix Commits

### Round 1

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| bc3b591 | test(variable): add negative TTL test for CommandCache | #4 |
| b4cb4cb | fix(runner): skip command execution when collection value overrides | #3 |
| cb091b6 | fix(runner): document cache as single-run-scoped with design comment | #1 |
| 2baa94a | test(cli): add integration tests for from_command sensitive redaction | #2 |

### Round 2

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 0c68461 | fix(parser): decode sensitive field as YAML boolean instead of string comparison | #1 |

### Round 3

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2884e6a | fix(variable): use %w for inner error wrapping in ExecuteCommand | #1 |

## Summary
6/6 findings resolved across 3 review rounds. 0 deferred.
