# Improvement Report: M1-025

**Task:** Validate command (curlew validate)
**Date:** 2026-03-17
**Review:** management/reviews/M1-025-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `invalid_yaml_includes_line_number` uses `t.Logf` — line number never actually asserted | Changed to `t.Errorf` | ✓ tests pass |
| 2 | Medium | `invalid_yaml_shows_line_number` (CLI) uses `t.Logf` — spec behavior untested | Changed to `t.Errorf`, removed redundant `"3"` check | ✓ tests pass |
| 3 | Medium | `line_omitted_when_zero` asserts by comment (`_ = s`) rather than code | Added `bytes.Contains(data, []byte(`"line"`))` assertion | ✓ tests pass |
| 4 | Medium | `collectAllStringRefs` `[]interface{}` and `default` cases uncovered (63.6%) | Added `undefined_variable_in_body_array_returns_warning` test case | ✓ 100% coverage |
| 5 | Low | `Severity.String()` unknown default branch untested (75%) | Added `Severity(99)` case to `TestSeverityString` | ✓ 100% coverage |
| 6 | Low | `if json.Valid(data) == false` unconventional; duplicate call | Replaced with `if !json.Valid(data)` and removed duplicate | ✓ tests pass |

---

## Round 2 — 2026-03-17

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `validateCmd` unknown-format branch (lines 542–544) never exercised; function coverage 85.7% | Added `invalid_format_exit_1` test in `TestValidateCmd` | ✓ tests pass |
| 2 | Low | `collectRequestRefs` query-param loop body never executed; function coverage 87.5% | Added `undefined_variable_in_query_param_returns_warning` test in `TestValidate` | ✓ tests pass |

## Round 3 — 2026-03-17

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `parseValidateArgs` `--format` missing-value error path uncovered (77.8%) | Added `TestValidateCmd_format_flag_missing_value` | ✓ tests pass |
| 2 | Low | `printValidationResult` three `useColor=true` ANSI branches uncovered (85.7%) | Added `TestPrintValidationResult_color` for all 3 states | ✓ 100% coverage |

## Round 4 — 2026-03-17

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `parseValidateArgs` `--no-color` branch (`noColor = true`) uncovered (88.9%) | Added `no_color_flag_exit_0` subtest to `TestValidateCmd` passing `--no-color` and asserting exit 0 with no ANSI codes | ✓ 100% coverage |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate (Round 4)

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`internal/validator`) | 100.0% |
| Coverage (`internal/output`) | 91.6% |
| Coverage (`internal/variable`) | 96.1% |
| Coverage (`cmd/curlew`) | 88.6% |
| `parseValidateArgs` function | 100.0% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6d2e4c7 | fix(test): harden soft assertions and add missing coverage for M1-025 | Round 1: #1–#6 |
| 0c676c6 | test(validate): cover invalid format and query-param variable ref paths | Round 2: #1, #2 |
| 6f53473 | test(validate): cover missing --format value and color output branches | Round 3: #1, #2 |
| 547a3e5 | test(validate): cover --no-color branch in parseValidateArgs | Round 4: #1 |

## Summary
11/11 findings resolved across 4 review cycles. 0 deferred.
