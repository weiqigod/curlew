# Code Review: M2-014

**Task:** Retry configuration precedence (global, collection, section, request)
**Reviewer:** AI
**Date:** 2026-04-02
**Branch:** feature/M2-014-retry-config-precedence
**Review Round:** 2 (re-review after /improve)

## Verdict: PASS

## Findings

No findings. Previous finding (#1: `%s` instead of `%w` in defaults error wrapping) was resolved in commit `25ce3cc`.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; context describes WHERE; sentinel errors used correctly |
| Input Validation | PASS | Section.UnmarshalYAML handles sequence, mapping, and default cases; defaults parsing rejects invalid YAML |
| Naming | PASS | No stuttering; all exported symbols have doc comments; short names in tight scopes |
| Code Organization | PASS | httpexec decoupled from parser (own Request type); internal boundaries respected; old types/functions removed cleanly |
| Correctness | PASS | Merge semantics correct (scalars replaced, arrays replaced entirely, objects deep-merged); precedence chain wired correctly through all phases; shallow copies safe given configs are immutable after creation |
| Test Quality | PASS | All 6 behaviors covered at unit and integration levels; table-driven tests; edge cases tested (nil, empty, both-nil, partial overlap) |

## Test Coverage
- Coverage: 90.8% total
  - `internal/retry`: 85.5%
  - `internal/config`: 96.6%
  - `internal/runner`: 92.1%
  - `internal/parser`: 90.6%
  - `internal/validator`: 100.0%
  - `cmd/curlew`: 84.3%
- All changed packages above 80% threshold.

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Global false + collection true = enabled | `TestMergeAll/global false + collection true`, `TestResolveRetryConfig_precedence/collection overrides global` |
| 2 | Collection max 5 + request max 10 = 10 | `TestMergeAll/collection max 5 + request max 10`, `TestResolveRetryConfig_precedence/request overrides collection` |
| 3 | Section setup max_attempts 5 retries 5 times | `TestResolveRetryConfig_precedence/section setup with max_attempts 5`, `TestRun_sectionRetry` |
| 4 | Teardown disabled = no retries | `TestResolveRetryConfig_precedence/teardown disabled overrides`, `TestRun_teardownRetryDisabled` |
| 5 | Collection status_codes replaces global array | `TestMergeAll/global status_codes replaced by collection`, `TestMergeConfigs/array field replaces entirely` |
| 6 | Global network_errors inherited when collection omits | `TestMergeAll/global network_errors inherited`, `TestMergeConfigs/object deep merge - retry_on sub-fields` |

## Summary

Clean implementation. The retry precedence chain (builtin < global < collection < section < request) is correctly wired through all packages. The `Section` type with dual YAML form support is well-designed. The `httpexec` decoupling from `parser` is a clean architectural improvement. All 6 task behaviors have both unit and integration test coverage. The previous review's finding has been resolved.
