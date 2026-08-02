# Improvement Report: M2-031

**Task:** GraphQL error handling and partial success configuration
**Date:** 2026-04-10
**Review:** management/reviews/M2-031-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `checkErr` from `graphql.CheckResponse(result.Body)` silently swallowed — malformed JSON body undetected | Removed the `if checkErr == nil` wrapper; added early return `fmt.Errorf("request %q: parsing graphql response: %w", ...)` on non-nil `checkErr`. Added `TestRun_graphql_malformed_response_body_returns_error` to lock in behavior. | ✓ tests pass |
| 2 | High | `Warnings` field not propagated from `parallel.RequestOutcome` to `RequestResult` in `executeParallelMain` | Added `Warnings []string` to `parallel.RequestOutcome` struct; mapped `Warnings: outcome.Warnings` in the `executeParallelMain` conversion loop. | ✓ tests pass |
| 3 | Medium | Test case `"error message mentions ignore"` had dead assertion code — duplicate `errors.Is` check was always false | Replaced dead duplicate with `strings.Contains(err.Error(), "ignore")` check on the test case named "error message mentions ignore". Added `strings` import. | ✓ tests pass |
| 4 | Medium | `Warnings` field from `rr` not mapped into `parallel.RequestOutcome` in `buildDataDrivenFunc` | Added `Warnings: rr.Warnings` to the `parallel.RequestOutcome` struct literal in `buildDataDrivenFunc`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (internal/graphql) | 97.7% |
| Coverage (internal/runner) | 86.7% |
| Coverage (internal/parallel) | 90.2% |
| Overall total | 89.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| baa2fd0 | fix(graphql): add real assertion for 'ignore' in error message test | #3 |
| 162b779 | fix(runner): propagate graphql.CheckResponse error instead of swallowing | #1 |
| 6d59f5a | fix(runner): propagate Warnings through parallel execution paths | #2, #4 |

## Summary

4/4 findings resolved. 0 deferred.
