# Improvement Report: M2-015

**Task:** Dependency analysis algorithm (variable analysis and graph building)
**Date:** 2026-04-07
**Review:** management/reviews/M2-015-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `ScanRequestFields` panics on nil `*parser.RequestItem` | Added nil guard `if item == nil { return result }` at function entry | tests pass |
| 2 | Medium | No binary integration test for `--show-dependencies` (completeness contract) | Added `TestCLIIntegration_ShowDependencies_DOTOutput` using `buildBinary`/`runBinary` | tests pass |
| 3 | Low | `scanBody` missing coverage for `[]any` and `map[string]string` branches | Added test cases `body array of objects` and `body map string string` to `TestScanRequestFields` | tests pass, scanBody 100% |
| 4 | Low | `addEdge` merge path (append variable to existing edge) never tested | Added `multiVarEdgeItems` helper and `TestAnalyze_MultiVarEdgeMerge` verifying single edge with two variables | tests pass, addEdge 100% |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 92.7% (parallel package, up from 90.0%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| a4443a1 | fix(parallel): add nil guard to ScanRequestFields and improve scanBody test coverage | #1, #3 |
| a98bd53 | test(parallel): add multi-variable edge merge coverage for addEdge | #4 |
| 666fc50 | test(cli): add binary integration test for --show-dependencies | #2 |

## Summary
4/4 findings resolved. 0 deferred.
