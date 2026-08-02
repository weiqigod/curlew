# Code Review: M2-031

**Task:** GraphQL error handling and partial success configuration
**Reviewer:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-031-graphql-error-handling

## Verdict: PASS

## Findings

No findings. All four issues from the previous review iteration have been resolved.

| Previous Finding | Status |
|-----------------|--------|
| #1 (High) — `checkErr` from `graphql.CheckResponse` silently swallowed | Fixed: early return with `fmt.Errorf("request %q: parsing graphql response: %w", ...)` at runner.go:924–925 |
| #2 (High) — `Warnings` not propagated in `executeParallelMain` | Fixed: `parallel.RequestOutcome` gained `Warnings []string`; mapped at runner.go:548 |
| #3 (Medium) — Dead assertion in `"error message mentions ignore"` test | Fixed: replaced duplicate `errors.Is` check with `strings.Contains(err.Error(), "ignore")` at graphql_test.go:307–309 |
| #4 (Medium) — `Warnings` not mapped in `buildDataDrivenFunc` | Fixed: `Warnings: rr.Warnings` added to `parallel.RequestOutcome` literal at runner.go:603 |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; `checkErr` no longer swallowed; sentinel errors used (`ErrInvalidErrorHandling`, `ErrInvalidProjectConfig`) |
| Input Validation | PASS | Three modes validated at parse time (parser.go); global config validated on load (project.go); `nil` input to `ClassifyOutcome` handled cleanly |
| Naming | PASS | No stuttering; doc comments on all exported types, functions, and constants; package names correct |
| Code Organization | PASS | Package boundaries respected; `internal/` enforced; no circular dependencies; `parallel.RequestOutcome` extended minimally |
| Correctness | PASS | Full-failure-always-fails semantics correct; per-request override over global correct; `Warnings` propagated through sequential, parallel, and data-driven paths |
| Test Quality | PASS | Mode matrix test covers all 10 combinations; assertion tests for `$.errors[0].extensions.code` and `$.errors[0].message`; malformed body test; all 6 task behaviors covered |

## Test Coverage

| Package | Coverage |
|---------|----------|
| `internal/graphql` | 97.7% |
| `internal/config` | 96.7% |
| `internal/output` | 92.3% |
| `internal/parser` | 89.8% |
| `internal/runner` | 86.7% |

All packages above the 80% threshold. All 6 task behaviors covered by at least one test.

## Behavior Coverage

| Behavior | Test |
|----------|------|
| Global `warn` mode: partial success → exit 0 + warning | `TestRun_graphql_mode_matrix` ("partial global warn"), `TestRun_graphql_warn_mode_updates_existing_test_behaviour` |
| Per-request `fail` beats global `warn` | `TestRun_graphql_mode_matrix` ("per-request fail beats global warn") |
| Full failure always fails regardless of mode | `TestRun_graphql_mode_matrix` (three "full failure" rows) |
| Assertion on `$.errors[0].extensions.code` | `TestRun_graphql_assertion_on_errors_extensions_code` |
| Assertion on `$.errors[0].message` | `TestRun_graphql_assertion_on_errors_message` |
| `ignore` mode: errors silently discarded | `TestRun_graphql_mode_matrix` ("partial global ignore", "per-request ignore beats global fail") |

## Summary

The implementation correctly delivers all six behaviors from the task definition. All four findings from the first review iteration have been properly resolved: the critical `checkErr` swallow is fixed, `Warnings` propagates through all three execution paths (sequential, parallel, data-driven), and the test assertion for the error message string is now live code. Build passes, all tests pass, linter reports zero issues.
