# Verification Report: M1-008

**Task:** Structured error messages (parse, network, config)
**Verified by:** AI
**Date:** 2026-03-11
**Branch:** feature/M1-008-structured-errors
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 8 packages, ~13s |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 93.4% | Meets >= 80% threshold |

## Observable Output

```
$ ./curlew run /tmp/bad.yaml
[ERROR] /tmp/bad.yaml:2 — invalid YAML syntax: yaml: line 2: could not find expected ':'

$ ./curlew run /tmp/nourl.yaml
[ERROR] /tmp/nourl.yaml — request "no-url" is missing required field 'url'
  Hint: Every request must specify a url field

$ ./curlew run /tmp/connrefused.yaml
Collection: test
[ERROR] fail — Connection refused at 127.0.0.1:1
  Hint: Check that the server is running and listening on this port

$ ./curlew run /tmp/dnsfail.yaml
Collection: test
[ERROR] dns-fail — DNS resolution failed for nonexistent.invalid
  Hint: Check that the hostname is correct and DNS is configured

$ ./curlew run nonexistent.yaml
[ERROR] nonexistent.yaml — file not found: nonexistent.yaml
```

Expected: Structured `[ERROR]` format with file paths, line numbers, hints
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Invalid YAML includes file path and line number | `TestParseFile_StructuredErrors` | PASS |
| 2 | Missing required field indicates which field and where | `TestParseFile_MissingRequiredFields` | PASS |
| 3 | DNS failure distinguished from connection refused | `TestClassifyNetworkError`, `TestExecute_NetworkErrorClassification` | PASS |
| 4 | Connection refused suggests checking server | `TestNetworkErrorHints`, `TestExecute_NetworkErrorClassification` | PASS |
| 5 | TLS certificate error identified | `TestClassifyNetworkError`, `TestNetworkErrorHints` | PASS |
| 6 | Timeout includes duration and suggests increasing | `TestExecute_timeout_includes_duration` | PASS |
| 7 | Consistent `[ERROR] file:line — message` format | `TestFormat`, `TestPrintStructuredError`, `TestCLIIntegration_ErrorFormat` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 8 packages PASS | PASS |
| 2 | Observable output works | All 5 scenarios match expectations | PASS |
| 3 | Test coverage >= 80% | 93.4% overall | PASS |
| 4 | No build warnings or lint errors | `golangci-lint run` — 0 issues | PASS |
| 5 | Smoke test updated | Structured error checks added | PASS |

## Code Review

Review PASS trusted (management/reviews/M1-008-review.md), spot-check clean.

| Check | Status |
|-------|--------|
| Error handling | PASS — `%w` wrapping, `Unwrap()` chains, sentinels preserved |
| Naming conventions | PASS — no stuttering, `apierrors` alias, doc comments on exports |
| Code organization | PASS — clean package boundaries, narrow interfaces |
| Test quality | PASS — table-driven with subtests, behavior-covering |

## Commits

| Hash | Message |
|------|---------|
| 762c9c3 | docs(plan): add implementation plan for M1-008 |
| 46c43e7 | chore(task): mark M1-008 as planned |
| 4e7dc90 | chore(task): mark M1-008 as in_progress |
| ad12853 | test(errors): add failing tests for structured error types and formatting |
| 0d33161 | feat(errors): implement structured error types, network classification, and formatting |
| 3372d2f | refactor(errors): fix gofumpt formatting |
| 5bda14c | test(parser): add failing tests for structured errors and missing required field |
| 6e205fb | feat(parser): wrap errors in Structured with file path, line, and missing field validation |
| b20e971 | refactor(parser): fix gofumpt formatting |
| 8144dc7 | test(http): add failing tests for network error classification |
| f764add | feat(http): classify network errors with hints for DNS, connection refused, TLS, timeout |
| d33ffc2 | test(output): add failing tests for PrintStructuredError and PrintRequestError |
| 76d52cf | feat(output): add PrintStructuredError and PrintRequestError with [ERROR] format |
| 0d48bee | test(cli): add failing tests for [ERROR] format in CLI output |
| 937f868 | feat(cli): use PrintStructuredError and PrintRequestError for [ERROR] output |
| 0f4f684 | test(cli): add structured error format checks to smoke test |
| 75b49cb | chore(task): mark M1-008 as review |
| 5ad8c99 | docs(review): add review with findings for M1-008 |
| 1b4be1c | fix(httpexec): include timeout duration in error message and hint |
| 9334f0e | refactor(parser): replace custom contains with strings.Contains |
| d5d6efc | test(errors): add coverage for NetworkError.Error() |
| 73ba3ae | fix(parser): wrap unsupported method error in Structured |
| d1fdbc9 | docs(review): add improvement report for M1-008 |
| 4406989 | docs(review): add passing review for M1-008 |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/main.go` | modified | +2/-2 |
| `cmd/curlew/main_test.go` | modified | +57 |
| `internal/errors/errors.go` | created | +185 |
| `internal/errors/errors_test.go` | created | +156 |
| `internal/httpexec/executor.go` | modified | +8/-2 |
| `internal/httpexec/executor_test.go` | modified | +97/-6 |
| `internal/output/terminal.go` | modified | +21 |
| `internal/output/terminal_test.go` | modified | +63 |
| `internal/parser/errors.go` | modified | +7/-2 |
| `internal/parser/parser.go` | modified | +67/-7 |
| `internal/parser/parser_test.go` | modified | +96/-6 |
| `smoke/run.sh` | modified | +22 |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge
