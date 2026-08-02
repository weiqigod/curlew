# Code Review: M1-025

**Task:** Validate command (curlew validate)
**Reviewer:** AI
**Date:** 2026-03-17
**Branch:** feature/M1-025-validate-command
**Round:** 5 (post-improvement round 4)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors use `%w` wrapping; sentinel errors used correctly; no swallowed errors |
| Input Validation | PASS | Empty args, unknown format, missing glob matches, non-existent files all handled |
| Naming | PASS | No stuttering; all exported symbols have doc comments; package name is lowercase single-word |
| Code Organization | PASS | `internal/validator` is a clean package; no circular deps; no resource leaks |
| Correctness | PASS | Extract-variable forward-propagation correct; `joinSections` covers setup/teardown; all 7 spec behaviors wired up end-to-end |
| Test Quality | PASS | `parseValidateArgs` now at 100% — `--no-color` branch covered by `no_color_flag_exit_0` subtest added in commit `547a3e5` |

## Test Coverage

| Package | Coverage | Status |
|---------|----------|--------|
| `internal/validator` | 100.0% | PASS (≥80%) |
| `internal/output` | 91.6% | PASS (≥80%) |
| `internal/variable` | 96.1% | PASS (≥80%) |
| `cmd/curlew` | 88.6% | PASS (≥80%) |

**Function-level detail (validate-related):**
- `parseValidateArgs`: 100.0% ✓
- `validateCmd`: 94.3% ✓ (only uncovered path is `WriteValidationJSON` I/O error — untriggerable in unit tests)
- `expandGlobs`: 90.9% ✓ (only uncovered path is `filepath.Glob` ErrBadPattern — extremely rare)
- `buildValidationJSONOutput`: 100.0% ✓
- `printValidationResult`: 100.0% ✓

## Spec Behavior Coverage

| Behavior | Tests |
|----------|-------|
| Valid collection → exit 0 + valid message | `valid_file_exit_0`, `valid_file_prints_valid_message` |
| Invalid YAML → exit 3 + line number | `invalid_yaml_exit_3`, `invalid_yaml_shows_line_number` |
| Missing required fields → exit 3 | `missing_required_field_exit_3` |
| Undefined variable refs → warning, exit 0 | `undefined_var_warns_exit_0` |
| Missing external file ref → exit 3 | `missing_external_file_exit_3` |
| `--format json` → valid JSON output | `format_json_valid_file_exit_0`, `format_json_invalid_file_exit_3`, `format_json_output_is_valid_json` |
| Multiple files / glob → all validated | `multiple_files_all_valid_exit_0`, `multiple_files_one_invalid_exit_3`, `glob_pattern_matches_files`, `glob_pattern_no_matches_exit_3` |

All 7 spec behaviors are fully covered. ✓

## Summary

All previous findings have been resolved. The `--no-color` branch in `parseValidateArgs` is now fully exercised. All packages exceed the 80% coverage threshold, lint is clean, and every task behavior has at least one test. The implementation is correct, well-structured, and complete.
