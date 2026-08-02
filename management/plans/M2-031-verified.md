# Verification Report: M2-031

**Task:** GraphQL error handling and partial success configuration
**Verified by:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-031-graphql-error-handling
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/apitest` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All 23 packages pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Exit code 0 |
| Coverage (total) | 89.2% | Meets >= 80% threshold |
| Coverage `internal/graphql` | 97.7% | Well above threshold |
| Coverage `internal/runner` | 86.7% | Above threshold |
| Coverage `internal/config` | 96.7% | Above threshold |
| Coverage `internal/output` | 92.3% | Above threshold |
| Coverage `internal/parser` | 89.8% | Above threshold |

## Observable Output

```
go test ./internal/graphql/...
ok  	github.com/peterlindqvist/apitest/internal/graphql	97.7% of statements
ok  	github.com/peterlindqvist/apitest/internal/graphql/files	94.8% of statements

go build -o ./apitest ./cmd/apitest
(clean build, no output)
```

Expected: `graphql.error_handling: warn` globally accepted, partial success logs warning with exit 0; per-request `fail` override causes failure; `go test ./internal/graphql/...` passes.
Result: MATCH — all graphql tests pass at 97.7% coverage, build succeeds, smoke test passes.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Global `warn` mode: partial success → exit 0 + warning | `TestRun_graphql_mode_matrix/partial_global_warn`, `TestRun_graphql_warn_mode_updates_existing_test_behaviour` | PASS |
| 2 | Per-request `fail` beats global `warn` | `TestRun_graphql_mode_matrix/per-request_fail_beats_global_warn` | PASS |
| 3 | Full failure always fails regardless of mode | `TestRun_graphql_mode_matrix/full_failure_fail_default`, `/full_failure_warn_still_fails`, `/full_failure_ignore_still_fails` | PASS |
| 4 | Assertion on `$.errors[0].extensions.code` | `TestRun_graphql_assertion_on_errors_extensions_code` | PASS |
| 5 | Assertion on `$.errors[0].message` | `TestRun_graphql_assertion_on_errors_message` | PASS |
| 6 | `ignore` mode: errors silently discarded | `TestRun_graphql_mode_matrix/partial_global_ignore`, `/per-request_ignore_beats_global_fail` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 23 packages, 0 failures | PASS |
| 2 | Observable output works | Binary builds; graphql tests pass; smoke test passes | PASS |
| 3 | Test coverage >= 80% | Total 89.2%, all key packages above 80% | PASS |
| 4 | No build warnings or lint errors | Clean `go build` + `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | N/A — no new user-facing flags | PASS |
| 6 | Smoke test updated | Added `error_handling: ignore` parser check to `smoke/run.sh` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (management/reviews/M2-031-review.md verdict PASS), spot-check clean:
- Error wrapping: `fmt.Errorf("%w: %q ...")` in `graphql.go:77`, `fmt.Errorf("parsing graphql response: %w", ...)` at line 131
- Exported symbols: All exports in `internal/graphql/graphql.go` have doc comments (`ErrorHandlingMode`, `ParseErrorHandling`, `Outcome`, `ClassifyOutcome`, `CheckResponse`, etc.)
- Test quality: `TestRun_graphql_warn_mode_updates_existing_test_behaviour` actually checks `results[0].Warnings` is non-empty — not just absence of error

## Commits

| Hash | Message |
|------|---------|
| 00f513c | docs(review): add passing review for M2-031 |
| 51c657b | docs(review): add improvement report for M2-031 |
| 6d59f5a | fix(runner): propagate Warnings through parallel execution paths |
| 162b779 | fix(runner): propagate graphql.CheckResponse error instead of swallowing |
| baa2fd0 | fix(graphql): add real assertion for 'ignore' in error message test |
| e68651a | docs(review): add review with findings for M2-031 |
| a5eff80 | chore(task): mark M2-031 as review |
| e91704a | refactor(runner): fix gofumpt formatting |
| 31c19ea | feat(smoke): add GraphQL error_handling ignore parser smoke test |
| 0b37f4c | feat(output): add warnings field to JSON output and wire GlobalGraphQL in CLI |
| ef592a9 | test(output): add failing tests for warnings in JSON output |
| 80040ea | feat(runner): implement graphql error handling modes with global/per-request precedence |
| 07f307e | test(runner): add failing tests for graphql error handling modes and warnings |
| 7ccb59c | feat(config): add GraphQLDefaults type to project config with validation |
| 82c9e4e | test(config): add failing tests for GraphQLDefaults in project config |
| 595af55 | feat(parser): accept ignore as valid graphql error_handling value |
| 5a99c39 | test(parser): add failing test for graphql error_handling ignore |
| 6ffc160 | feat(graphql): add ignore mode and ClassifyOutcome helper |
| aaa7269 | test(graphql): add failing tests for ignore mode and ClassifyOutcome |
| 884c46e | chore(task): mark M2-031 as in_progress |
| e15ee9a | chore(task): mark M2-031 as planned |
| f7c3e39 | docs(plan): add implementation plan for M2-031 |

TDD pattern visible: `test(...)` commits precede corresponding `feat(...)` commits throughout. All commits reference `Refs: M2-031`.

## Files Changed

| File | Action |
|------|--------|
| `internal/graphql/graphql.go` | modified — added `ErrorHandlingIgnore`, `Outcome` type, `ClassifyOutcome` |
| `internal/graphql/graphql_test.go` | modified — added `ignore` cases, `TestClassifyOutcome` |
| `internal/config/project.go` | modified — added `GraphQLDefaults`, `GraphQLErrorHandling` types |
| `internal/config/project_test.go` | modified — added `TestParseProjectConfig_GraphQLDefaults` |
| `internal/parser/parser.go` | modified — extended allow-list to include `ignore` |
| `internal/parser/parser_test.go` | modified — added `ignore` validation test |
| `internal/parser/testdata/graphql_error_handling_ignore.yaml` | added — fixture |
| `internal/runner/runner.go` | modified — added `GlobalGraphQL` to `VarSources`, `Warnings` to `RequestResult`, full mode resolution logic |
| `internal/runner/runner_test.go` | modified — added mode matrix, assertion, warning propagation tests |
| `internal/parallel/executor.go` | modified — added `Warnings []string` to `RequestOutcome` |
| `internal/output/json.go` | modified — added `Warnings []string` to `JSONRequest` |
| `internal/output/json_test.go` | modified — added warnings marshalling tests |
| `cmd/apitest/main.go` | modified — wired `GlobalGraphQL`, renders warnings, populates JSON warnings |
| `smoke/run.sh` | modified — added `error_handling: ignore` parser smoke test |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge
