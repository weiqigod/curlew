# Code Review: M9-004

**Task:** markdown parallel + data-driven integration: wave grouping, per-iteration files
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-004-markdown-parallel-datadriven
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. Both issues from iteration 1 are fully resolved:

1. `TestRun_MarkdownFormat_ParallelWaves` now uses a chained A→B→C collection via `extract:` variables to force three distinct execution waves. The test asserts `## Wave 0`, `## Wave 1`, `## Wave 2` headers in `run.md` in ascending order and verifies each per-request file (`a.md`, `b.md`, `c.md`) carries the matching `wave_index: N` value. Behavior 11 and the corresponding DoD item are fully satisfied.

2. `TestMarkdown_RunMD_WaveGrouping_MixedSequential` covers the `## Sequential` fallback section in `renderRunMDByWave`. The test uses `IsParallel=true` with a `WaveIndex=-1` data-driven aggregate entry alongside waved entries and verifies the `## Sequential` section appears after the wave headers with the data-driven entry beneath it. `renderRunMDByWave` now reports 100% statement coverage.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; `return err` for `EnsureReportDir` is appropriate (already has context); no swallowed errors in implementation code; `baseSlug, _ = parser.Slug(groupName)` silent discard is safe because parser validates request names at load time |
| Input Validation | PASS | Nil slices, empty iterations, and zero `IterationTotal` handled gracefully; `len(Iterations) == 0` path preserves M9-002/M9-003 behaviour unchanged |
| Naming | PASS | All exported symbols have doc comments; no stuttering; `IterationEntry`/`RequestEntry`/`WriteOptions` are correctly namespaced; all new functions in `datadriven.go` and `run_md.go` are unexported |
| Code Organization | PASS | `internal/output/markdown` does not import `internal/runner`; package boundaries respected; clean imports; single-responsibility per file (`datadriven.go`, `run_md.go`, `formatter.go`, `writer.go`) |
| Correctness | PASS | Inner-loop `i--` adjustment in `buildMarkdownReport` is correct; splice discipline extended to iter files and `index.md`; `IterationTotal > len(Iterations)` truncation logic correct; `WaveIndex=-1` for DD aggregates in run.md is explicit and intentional; empty `RequestID` for parallel runner results (pre-existing limitation, no regression) |
| Test Quality | PASS | All 11 DoD-named tests present and passing; integration test exercises real 3-wave parallel collection with real HTTP server; `TestMarkdown_RunMD_WaveGrouping_MixedSequential` covers previously-uncovered `## Sequential` branch; `renderRunMDByWave` now 100% covered |

## Test Coverage
- Coverage: 92.3% (`internal/output/markdown` — exceeds 80% threshold)
- `renderRunMDByWave`: 100% (was missing `## Sequential` branch in iteration 1; now fully covered)
- `renderDataDrivenRequest`: 78.6% — uncovered lines are error-injection paths (EnsureReportDir failure, writeFile failure for iter/index files); overall package coverage is 92.3% which satisfies the threshold
- Runner package: 85.1% (no regressions; all existing tests pass)
- Total across all packages: 86.8%

## Summary

The iteration-1 improvements are correct and complete. Both medium and low severity findings from the first review are resolved without introducing new issues. The implementation correctly extends M9-002/M9-003 markdown output with parallel wave grouping and data-driven per-iteration file layout; all 11 DoD-named tests pass; coverage is 92.3% for the markdown package; golangci-lint reports 0 issues; `./scripts/ci-local.sh --go` passes cleanly.
