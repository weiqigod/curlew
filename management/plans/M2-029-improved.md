# Improvement Report: M2-029

**Task:** GraphQL protocol adapter (queries and mutations)
**Date:** 2026-04-09
**Review:** management/reviews/M2-029-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Shallow copy of `*req` shares Headers map; `BuildRequest` mutates caller's map when adding Content-Type | Deep-copy the headers map before modification using a manual copy loop | ✓ tests pass (new test `TestBuildRequest_does_not_mutate_caller_headers`) |
| 2 | High | `mode, _ = graphql.ParseErrorHandling(...)` silently discards the error in runner | Handle the error: return it as a request-scoped error. Defense-in-depth since parser now validates too | ✓ tests pass |
| 3 | Medium | Parser does not validate `graphql.error_handling` values; invalid values silently accepted | Added validation in parser: only `""`, `"fail"`, `"warn"` allowed. New sentinel `ErrInvalidFieldValue` | ✓ tests pass (new test `graphql_invalid_error_handling_rejected`) |
| 4 | Medium | `ErrGraphQLErrors` sentinel defined but never used in production code | Removed the unused sentinel. Can be re-added if needed for M2-031 | ✓ tests pass, lint clean |
| 5 | Medium | `internal/requtil/` has no test files; GraphQL interpolation code only tested indirectly | Created `requtil_test.go` with 8 unit tests covering: nil GraphQL, query interpolation, variable interpolation, empty variables, error cases, non-mutation, and error_handling preservation | ✓ tests pass |

## Out of Scope (Deferred)

| # | Severity | Finding | Rationale |
|---|----------|---------|-----------|
| 6 | Low | Branch name `feature/M2-029-dashboard-export-sharing` is misleading | Renaming branches mid-development is disruptive and the review itself notes this. The branch name does not affect functionality. |

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS (all 21 packages) |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 89.4% total; graphql 97.3%; parser 90.7%; runner 86.5%; requtil 58.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `8187a30` | fix(parser): validate graphql error_handling values at parse time | #3 |
| `b5d08ce` | fix(graphql): deep-copy headers map in BuildRequest to prevent caller mutation | #1 |
| `8ce0e95` | fix(runner): handle ParseErrorHandling error instead of discarding it | #2 |
| `00c5e7b` | fix(graphql): remove unused ErrGraphQLErrors sentinel | #4 |
| `e03c8cd` | test(requtil): add unit tests for GraphQL field interpolation | #5 |

## Summary
5/6 findings resolved. 1 deferred (Low severity: branch name mismatch).
