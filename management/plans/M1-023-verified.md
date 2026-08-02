# Verification Report: M1-023

**Task:** Sensitive variable detection and redaction
**Verified by:** AI
**Date:** 2026-03-17
**Branch:** feature/M1-023-sensitive-redaction
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All checks pass (smoke test bugs fixed) |
| Coverage | 92.1% | Exceeds >= 80% threshold |

## Observable Output

```
=== -vv output (should show [REDACTED]) ===
  > Authorization: [REDACTED]
  > X-API-Key: [REDACTED]

=== --allow-sensitive output (should show plain text) ===
  > Authorization: Bearer secret123
  > X-API-Key: abc
```

Expected: sensitive headers redacted by default, shown plain with `--allow-sensitive`
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given a variable named password, token, secret, api_key, or authorization, when displayed in output, then value shows [REDACTED] | `heuristic_password_var_redacted`, `heuristic_token_var_redacted` | PASS |
| 2 | Given a variable tagged with !sensitive, when displayed in output, then value shows [REDACTED] | `sensitive_var_redacted_in_verbose_output` | PASS |
| 3 | Given --allow-sensitive flag, when executed, then sensitive values are shown in plain text | `allow_sensitive_shows_plaintext_value` | PASS |
| 4 | Given sensitive values in JSON output format, when present, then they are redacted the same way | `sensitive_redacted_in_json_format` | PASS |
| 5 | Given sensitive values in request headers (Authorization), when shown in verbose output, then they are redacted | `authorization_header_redacted_in_verbose` | PASS |
| 6 | Given a variable with !sensitive tag in .env file, when loaded, then it is marked sensitive | `TestParseDotenvSensitive` (5 subtests) + `heuristic_detects_sensitive_name_in_dotenv` | PASS |
| 7 | Given redaction in error messages, when a sensitive variable fails to resolve, then the value is not leaked | `error_message_does_not_leak_sensitive_value` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages pass | PASS |
| 2 | Observable output works as specified | `[REDACTED]` shown with -vv, plain text with --allow-sensitive | PASS |
| 3 | Test coverage >= 80% | 92.1% total; all packages exceed threshold | PASS |
| 4 | No build warnings or lint errors | `go build` clean, `golangci-lint` 0 issues | PASS |
| 5 | Help text updated | `--allow-sensitive` visible in `curlew --help` | PASS |
| 6 | Smoke test updated | Sensitive redaction smoke tests added and passing | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (from `management/reviews/M1-023-review.md`), spot-check clean.

Spot-check results:
- `sensitive.go:102` — `RedactValue` returns error from caller, no panics; `%w`-wrapped errors in config/dotenv
- `sensitive.go:51` — `NewSensitiveSet` has doc comment; all exports documented
- `TestRunCmd_SensitiveRedaction` — table-driven subtests, each exercises a distinct behavior

## Smoke Test Fix

The smoke test added in M1-023 had two bugs fixed during verification:
1. The test collection did not use the `password` variable in any request, so nothing was interpolated and `[REDACTED]` could never appear. Fixed by adding `Authorization: "Bearer {{password}}"` to the request headers.
2. The `mktemp` template patterns included a `.yaml` suffix after `XXXXXX`, which macOS BSD `mktemp` does not support (X's must be at the end). Fixed by removing the suffix from the two new templates.

These were bugs in the test code, not in the implementation.

## Commits

| Hash | Message |
|------|---------|
| 4bb766c | fix(smoke): fix sensitive redaction smoke tests |
| 09f23ee | docs(review): add passing review for M1-023 |
| 894a2ab | docs(review): add improvement report for M1-023 |
| 0f9a2da | feat(output): expand heuristic sensitivity detection to all variable sources |
| bb093ed | test(variable): add failing test for dotenv heuristic sensitivity detection |
| 22de86e | docs(review): add review with findings for M1-023 |
| 76fe56f | chore(task): mark M1-023 as review |
| 5516fc6 | feat(output): add sensitive redaction smoke tests and fix lint |
| 8eb6bf6 | feat(cli): add --allow-sensitive flag and sensitive redaction wiring |
| f2d8a34 | test(cli): add failing tests for --allow-sensitive flag and redaction |
| aa4a8b2 | feat(config): add !sensitive prefix support to dotenv parsing |
| 19d2f4a | test(config): add failing tests for dotenv !sensitive prefix |
| bca7162 | feat(parser): introduce SensitiveVars with !sensitive YAML tag support |
| dc7a668 | test(parser): add failing tests for SensitiveVars and !sensitive tag |
| 0fdbc52 | feat(variable): implement SensitiveSet, redaction functions |
| 744dabc | test(variable): add failing tests for sensitive redaction |

## Files Changed

| File | Action |
|------|--------|
| `internal/variable/sensitive.go` | added — SensitiveSet, IsSensitiveName, IsSensitiveHeaderName, RedactValue, RedactHeaders |
| `internal/variable/sensitive_test.go` | added |
| `internal/parser/collection.go` | modified — SensitiveVars field, !sensitive YAML tag |
| `internal/parser/collection_test.go` | modified |
| `internal/config/dotenv.go` | modified — !sensitive prefix support |
| `internal/config/dotenv_test.go` | modified |
| `cmd/curlew/main.go` | modified — --allow-sensitive flag, SensitiveSet wiring across all variable sources |
| `cmd/curlew/main_test.go` | modified — TestRunCmd_SensitiveRedaction |
| `smoke/run.sh` | modified — sensitive redaction smoke tests |

## Issues Found

One pre-existing bug in the smoke test (not implementation code): `password` variable was not used in any request header, so `[REDACTED]` would never appear. Fixed in this verification pass.

## Recommendation

PASS — ready for PR and merge.
