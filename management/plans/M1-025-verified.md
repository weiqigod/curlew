# Verification Report: M1-025

**Task:** Validate command (curlew validate)
**Verified by:** AI
**Date:** 2026-03-17
**Branch:** feature/M1-025-validate-command
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 12 packages, all green |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All validate scenarios clean |
| Coverage — `internal/validator` | 100.0% | Meets ≥80% threshold |
| Coverage — `internal/output` | 91.6% | Meets ≥80% threshold |
| Coverage — `internal/variable` | 96.1% | Meets ≥80% threshold |
| Coverage — `cmd/curlew` | 88.6% | Meets ≥80% threshold |
| Coverage — total | 92.4% | Meets ≥80% threshold |

## Observable Output

```
$ ./curlew validate cmd/curlew/testdata/minimal.yaml
OK cmd/curlew/testdata/minimal.yaml is valid
$ echo $?
0

$ ./curlew validate cmd/curlew/testdata/invalid.yaml
FAIL cmd/curlew/testdata/invalid.yaml is invalid
  [ERROR]   line 1: invalid YAML syntax: yaml: line 1: did not find expected ',' or ']'
$ echo $?
3
```

Expected: pass/fail output without HTTP requests, exit 0 for valid / exit 3 for invalid
Result: MATCH

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Valid collection → exit 0 + valid message | `valid_file_exit_0`, `valid_file_prints_valid_message` | PASS |
| 2 | Invalid YAML → exit 3 + parse error + line number | `invalid_yaml_exit_3`, `invalid_yaml_shows_line_number` | PASS |
| 3 | Missing required fields → exit 3 listing missing fields | `missing_required_field_exit_3` | PASS |
| 4 | Undefined variable refs → warning, exit 0 | `undefined_var_warns_exit_0` | PASS |
| 5 | Missing external file ref → exit 3 | `missing_external_file_exit_3` | PASS |
| 6 | `--format json` → JSON validation output | `format_json_valid_file_exit_0`, `format_json_invalid_file_exit_3`, `format_json_output_is_valid_json` | PASS |
| 7 | Multiple files / glob → all validated | `multiple_files_all_valid_exit_0`, `multiple_files_one_invalid_exit_3`, `glob_pattern_matches_files`, `glob_pattern_no_matches_exit_3` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 12 packages green | PASS |
| 2 | Observable output works as specified | Valid → exit 0 + OK; Invalid → exit 3 + FAIL + error | PASS |
| 3 | Test coverage ≥ 80% | 92.4% total; `internal/validator` 100% | PASS |
| 4 | No build warnings or lint errors | `go build` clean; `golangci-lint` 0 issues | PASS |
| 5 | Help text updated | `validate <file>` shown in `curlew --help` | PASS |
| 6 | Smoke test updated | `=== Validate command ===` section in `smoke/run.sh` passes | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Doc comments on exports | PASS |
| Test quality (table-driven) | PASS |
| Code organization | PASS |

Branch A: Review PASS (Round 5) trusted; spot-check clean:
- Error handling: validator returns `*Result` accumulating issues — no error swallowing; sentinel errors used in `parser` layer with `%w` wrapping
- Exported symbols: all have doc comments (verified in `internal/validator/validator.go`)
- Tests: all use `t.Run` subtests; assertions are hard (`t.Errorf`), not soft (`t.Logf`)

## Commits

| Hash | Message |
|------|---------|
| 98c9296 | docs(review): add passing review for M1-025 |
| a553143 | docs(review): add improvement report for M1-025 |
| 547a3e5 | test(validate): cover --no-color branch in parseValidateArgs |
| e4af5d0 | docs(review): add review with findings for M1-025 |
| cc814ce | docs(review): add improvement report for M1-025 |
| 6f53473 | test(validate): cover missing --format value and color output branches |
| 2ea57a0 | docs(review): add review with findings for M1-025 |
| a998486 | docs(review): add improvement report for M1-025 |
| 0c676c6 | test(validate): cover invalid format and query-param variable ref paths |
| 20338f1 | docs(review): add review with findings for M1-025 |
| 15cefd3 | docs(review): add improvement report for M1-025 |
| 6d2e4c7 | fix(test): harden soft assertions and add missing coverage for M1-025 |
| 1546df5 | docs(review): add review with findings for M1-025 |
| 2183d69 | chore(task): mark M1-025 as review |
| ab54524 | chore(smoke): fix SIGPIPE in validate help text check |
| a68fa30 | test(validator): add coverage tests for Severity.String and body map refs |
| e3186da | feat(cli): add validateCmd + smoke test cases |
| 53c6710 | feat(cli): add validate command with glob support and JSON output |
| 38bac16 | test(cli): add failing tests for validateCmd |
| 1d9dfbf | feat(output): add ValidationJSONOutput types and WriteValidationJSON |
| 55491a0 | test(output): add failing tests for WriteValidationJSON |
| f3e5456 | feat(validator): create validator package with Validate function |
| bba09c0 | test(validator): add failing tests for Validate |
| aa37268 | feat(variable): export FindReferences wrapper |
| a825d8d | test(variable): add failing tests for FindReferences |
| a89d6d5 | chore(task): mark M1-025 as in_progress |
| b81a1d4 | chore(task): mark M1-025 as planned |
| 9d1acaf | docs(plan): add implementation plan for M1-025 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/curlew/main.go` | modified — added `validateCmd`, `parseValidateArgs`, `expandGlobs`, `buildValidationJSONOutput`, `printValidationResult` |
| `cmd/curlew/main_test.go` | modified — added `TestValidateCmd`, `TestValidateCmd_format_flag_missing_value`, `TestPrintValidationResult_color`, `TestValidateHelp` |
| `internal/output/json.go` | modified — added `ValidationJSONOutput` types and `WriteValidationJSON` |
| `internal/output/json_test.go` | modified — added `TestWriteValidationJSON` |
| `internal/validator/validator.go` | created — new `validator` package with `Validate`, `Severity`, `Issue`, `Result` |
| `internal/validator/validator_test.go` | created — full test suite for `Validate` |
| `internal/variable/variable.go` | modified — exported `FindReferences` wrapper |
| `internal/variable/variable_test.go` | modified — added `TestFindReferences` |
| `smoke/run.sh` | modified — added validate smoke tests |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
