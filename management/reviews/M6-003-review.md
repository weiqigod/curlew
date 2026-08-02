# Code Review: M6-003

**Task:** Redact sensitive values from request and response bodies
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-001-error-taxonomy
**Iteration:** 2 (re-review after /improve)

## Verdict: PASS

## Findings

No findings. All four issues from the previous review have been resolved:

1. ~~Missing `junit_format` subtest~~ — `t.Run("junit_format", ...)` added to `TestRedact_BodyAcrossFormats` (main_test.go ~line 7426). Subtest correctly gates behind `TierProfessional` and asserts the secret is absent.
2. ~~Zero-value `SensitiveSet` defensive guards untested~~ — `TestSensitiveSet_ZeroValue` added to `sensitive_test.go` (lines 323–347), covering both `AddValue` and `Merge` on an uninitialized struct.
3. ~~`Names()` nil receiver path untested~~ — `TestSensitiveSet_NilSafe` extended to assert `s.Names() == nil` for a nil receiver (sensitive_test.go line 318–320).
4. ~~Dead `enc.Encode` error branch unexplained~~ — Explanatory comment added (redact.go lines 75–78) documenting why the branch is retained for future safety; the coverage gap at 87.5% is intentional and documented.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No errors returned from pure transformation functions; no swallowed errors; nil guards present on all public receivers. |
| Input Validation | PASS | `nil` body, empty string, empty `[]byte`, nil `SensitiveSet`, empty `SensitiveSet`, zero-value `SensitiveSet{}`, and empty sensitive values all handled correctly. |
| Naming | PASS | No stuttering; exported symbols have doc comments (`RedactBody`, `AddValue`, `Values`); private helpers use short descriptive names; package name unchanged. |
| Code Organization | PASS | `internal/variable/redact.go` is a coherent single-purpose file; `addSensitiveValues` is unexported and local to main.go; no cross-package boundary violations. |
| Correctness | PASS | Longest-first ordering in `Values()` prevents prefix-collision bugs. JSON structure preserved via round-trip through `encoding/json`. `--allow-sensitive` opt-out preserved. Response body mutated before all formatters consume it. |
| Test Quality | PASS | All 6 task behaviors covered; `junit_format` subtest added; nil/zero-value guard paths exercised; table-driven tests for `RedactBody` and `SensitiveSet` value tracking. |

## Test Coverage
- `internal/variable` package overall: **96.7%** (requirement: ≥ 80% — PASS)
- `internal/variable/redact.go`: 97.5% (`redactJSONString` encode-error path is 87.5% — intentional defensive branch, documented)
- `cmd/apitest` package overall: **81.7%** (PASS)
- `addSensitiveValues` helper: 100%

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| Request body: sensitive value replaced with redaction marker | `TestRedact_BodyAcrossFormats/terminal_vv` (-vv shows request body dump with [REDACTED]) | PASS |
| Response body: echoed sensitive value replaced | `TestRedact_BodyAcrossFormats/{terminal_vv,json_format,tap_format,junit_format}` | PASS |
| `--allow-sensitive` skips redaction | `TestRedact_BodyAcrossFormats/allow_sensitive_shows_secret` | PASS |
| JSON structure preserved, only string leaves rewritten | `TestRedactBody/{json_string_object,json_string_nested,map_string_any}` | PASS |
| Non-JSON text: substring replaced; partial-token not replaced | `TestRedactBody/{plain_text_substring,plain_text_partial_not_replaced}` | PASS |
| Existing header-redaction unchanged | `TestRunCmd_SensitiveRedaction` (pre-existing, still passes) | PASS |

## Summary

All four findings from the first review have been fully resolved. The implementation is correct and complete: `RedactBody` handles JSON and plain-text bodies with longest-first value matching, `SensitiveSet` now tracks concrete sensitive values alongside names, the main.go wiring mutates results before any formatter consumes them, and the end-to-end `TestRedact_BodyAcrossFormats` test covers all five output formats including JUnit. Coverage is 96.7% in `internal/variable` and 81.7% in `cmd/apitest`, both above the 80% threshold.
