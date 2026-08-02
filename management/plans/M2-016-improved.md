# Improvement Report: M2-016

**Task:** Parallel request execution with wave-based scheduling
**Date:** 2026-04-07
**Review:** management/reviews/M2-016-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Zero-valued RequestOutcome appended when scope creation fails, producing duplicate spurious entries | Restructured wave pre-check into a single pass: dependency check, guard rail, and scope creation merged. Only requests with successful scope creation are added to the `ready` slice and launched as goroutines. Zero-valued entries can no longer leak into outcomes. | Tests pass (new test: TestExecuteWaves_ScopeCreationFailure_NoZeroValuedOutcomes) |
| 4 | Medium | Guard rail counter incremented before execution; overcounts on scope failure | Counter is now incremented only after successful scope creation, not in the pre-check loop. Scope creation failures do not consume guard rail slots. | Tests pass (new test: TestExecuteWaves_GuardRailNotOvercounted_OnScopeFailure) |
| 2 | Medium | Three functions duplicated between parallel/executor.go and runner/runner.go (interpolateRequest, toHeaderInputs, toBodyInputs) | Extracted into new `internal/requtil/` package with exported functions: InterpolateRequest, ToHeaderInputs, ToBodyInputs, ToHTTPRequest. Both runner and parallel now import from requtil. | Tests pass, lint clean |
| 5 | Low | parallel.ExecuteFunc identical to runner.ExecuteFunc, forcing wrapper function | ExecuteFunc type moved to requtil. runner.ExecuteFunc is now a type alias (`= requtil.ExecuteFunc`). Wrapper function in executeParallelMain removed; exec func passed directly. | Tests pass, lint clean |
| 3 | Medium | Snapshot() has 0% unit test coverage in variable package | Added 6 unit tests in variable_test.go covering: basic copy, independence from parent mutations, dynamic registry preservation, nil resolved/vars maps, and funcCache independence. Snapshot now at 100% coverage. | Tests pass |
| 6 | Low | callOrder slice in test handler appended without synchronization (potential data race) | Added sync.Mutex to protect callOrder in TestRunCmd_parallel_setup_sequential_main_parallel. Reads also protected via mutex lock + copy. Removed unused atomic counter. | Tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage | 90.2% total; parallel 93.3%; runner 90.2%; variable 96.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fc1cf53 | fix(parallel): eliminate zero-valued outcomes on scope failure and correct guard rail counter | #1, #4 |
| 0bef795 | refactor(requtil): extract shared request helpers into internal/requtil package | #2, #5 |
| bf105c8 | test(variable): add unit tests for Scope.Snapshot() | #3 |
| 75cbeae | fix(test): protect callOrder slice with mutex in parallel test | #6 |

## Summary
6/6 findings resolved. 0 deferred.
