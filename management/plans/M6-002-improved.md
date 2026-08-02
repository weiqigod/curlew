# Improvement Report: M6-002

**Task:** Source-location plumbing: file and line onto parsed items and results
**Date:** 2026-04-21
**Review:** management/reviews/M6-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `buildWSOutcome` constructed a `parallel.RequestOutcome` without setting `SourceFile` or `SourceLine`, silently dropping source location for every parallel WebSocket request | Added `SourceFile: item.SourceFile, SourceLine: item.SourceLine` to the `RequestOutcome` literal in `buildWSOutcome` | ✓ tests pass |
| 2 | High | Three early-return paths in `buildWebSocketFunc` (gate-error, interpolation-error, rate-limit cancellation) returned `parallel.RequestOutcome` literals omitting `SourceFile` and `SourceLine` | Added `SourceFile: item.SourceFile, SourceLine: item.SourceLine` to each of the three early-return `RequestOutcome` literals | ✓ tests pass |
| 3 | High | `buildDataDrivenFunc` conversion loop (runner.RequestResult → parallel.RequestOutcome) omitted `SourceFile` and `SourceLine`, causing location loss when data-driven requests run via the parallel wave path | Added `SourceFile: rr.SourceFile, SourceLine: rr.SourceLine` to the `parallel.RequestOutcome` literal in the conversion loop | ✓ tests pass |
| 4 | Medium | No tests for source-location threading in parallel WebSocket or parallel data-driven paths; only sequential execution was covered | Added `TestRunner_ParallelWebSocketCarriesSourceLocation` and `TestRunner_ParallelDataDrivenCarriesSourceLocation` exercising the parallel code paths via `VarSources{Parallel: true}` and `VarSources{WebSocketDialer: dialer, Parallel: true}` | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage `internal/parser` | 89.8% |
| Coverage `internal/runner` | 85.6% |
| Coverage `internal/parallel` | 90.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ecd7c7e | fix(runner): thread SourceFile/SourceLine through parallel WebSocket and data-driven paths | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred. All parallel execution paths (WebSocket and data-driven via wave mode) now correctly preserve `SourceFile` and `SourceLine` through the `parallel.RequestOutcome` intermediate struct into the final `RequestResult`. Two new tests lock in the parallel coverage for these paths.
