# Code Review: M1-023

**Task:** Sensitive variable detection and redaction
**Reviewer:** AI
**Date:** 2026-03-17
**Branch:** feature/M1-023-sensitive-redaction

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel `ErrInvalidDotenv` used; no swallowed errors |
| Input Validation | PASS | Nil/empty inputs handled throughout; `UnmarshalYAML` validates mapping node kind before iteration |
| Naming | PASS | No stuttering; exported symbols have doc comments; `SensitiveSet`, `AddHeuristicNames` are clear |
| Code Organization | PASS | `sensitive.go` is a pure new file with no cross-package reach-in; runner and parser updated mechanically |
| Correctness | PASS | Heuristic name detection now covers all six variable sources: collection, dotenv, env-file, `--env-var`, `--var`, project config |
| Test Quality | PASS | All seven behaviors have integration coverage; all four previously-missing tests added |

## Spec Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| 1. Heuristic name redaction (password, token, …) | `heuristic_password_var_redacted`, `heuristic_token_var_redacted`, `heuristic_detects_sensitive_name_in_dotenv` |
| 2. `!sensitive` YAML tag → redacted | `sensitive_var_redacted_in_verbose_output` |
| 3. `--allow-sensitive` shows plain text | `allow_sensitive_shows_plaintext_value` |
| 4. JSON format redacts the same way | `sensitive_redacted_in_json_format` |
| 5. Authorization header redacted in verbose | `authorization_header_redacted_in_verbose` |
| 6. `!sensitive` prefix in `.env` | `TestParseDotenvSensitive` (5 subtests) |
| 7. Sensitive value not leaked in error messages | `error_message_does_not_leak_sensitive_value` |

## Test Coverage

| Package | Coverage |
|---------|----------|
| `internal/variable` | 96.1% |
| `internal/config` | 96.4% |
| `internal/parser` | 91.5% |
| `internal/runner` | 91.2% |
| `cmd/apitest` | 86.1% |
| **Total** | **92.1%** |

All packages exceed the 80% threshold.

## Summary

All four findings from the first review have been resolved. The heuristic sensitivity detection now covers all variable sources (`col.Variables`, `.env`, `--env`, `--env-var`, `--var`, project config) via the clean `AddHeuristicNames` helper. All seven spec behaviors are covered by unit and integration tests. Lint is clean and build succeeds.

→ Run `/verify M1-023` to complete the task.
