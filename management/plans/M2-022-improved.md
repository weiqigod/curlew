# Improvement Report: M2-022 (Iteration 2)

**Task:** Data-driven integration with parallel execution and large datasets
**Date:** 2026-04-08
**Review:** management/reviews/M2-022-review.md (iteration 2)

## Prior Iteration

Iteration 1 resolved 9/9 findings from the first review. See commit history for details.

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Chunk-relative indexing bug: `ExecuteParallel` iterates each chunk with 0-based indices, producing duplicate names and incorrect `_index`/`_total`/`_row_number` scope variables for chunks after the first | Added `ChunkOffset` and `TotalRows` fields to `ParallelConfig`. `ExecuteParallel` now computes `globalIdx = chunkOffset + localIdx` for `InjectIterationVars` and `ExecFn`, while using `localIdx` for the results array. The runner passes the running chunk offset and full dataset total when calling `ExecuteParallel` for each chunk. Added `TestExecuteParallel_ChunkOffset` test. | Tests pass |
| 2 | High | Missing Method, URL, RequestHeaders, RequestBody, and RetryCount in parallel data-driven results | Added `Method`, `URL`, `RequestHeaders`, `RequestBody`, and `RetryCount` fields to `IterationResult`. Populated in the `execFn` closure (both error and success paths). Copied into `RequestResult` during conversion. Added assertions to `TestRun_DataDriven_ParallelExecution`. | Tests pass |
| 3 | Medium | `FilterResults` is dead code operating on unexported `iterationResult` type, never called from production | Removed `results.go` and `results_test.go` entirely. The actual filtering is handled by `filterDataDrivenResults` in the runner package. | Tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (datadriven + runner) | 88.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| c317e4a | fix(datadriven): use global indices in chunked parallel execution | #1 |
| f11dd86 | fix(datadriven): populate Method/URL/Headers/Body/RetryCount in parallel results | #2 |
| 647af83 | refactor(datadriven): remove dead FilterResults code | #3 |

## Summary
3/3 findings resolved. 0 deferred.
