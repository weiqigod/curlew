# Verification Report: M9-004

**Task:** markdown parallel + data-driven integration: wave grouping, per-iteration files
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M9-004-markdown-parallel-datadriven
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected (ci-local runs race detector) |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (markdown) | 92.3% | Meets >= 80% threshold |
| Coverage (runner) | 85.1% | Meets >= 80% threshold |
| Coverage (total) | 86.8% | Meets >= 80% threshold |

## Observable Output

The observable collections (`collections/parallel.yaml`, etc.) are not standalone files; the integration tests in `cmd/apitest/run_test.go` implement the observable scenarios against real HTTP servers and the real binary. All integration tests pass:

```
=== RUN   TestRun_MarkdownFormat_DataDriven
--- PASS: TestRun_MarkdownFormat_DataDriven (0.01s)
=== RUN   TestRun_MarkdownFormat_DataDrivenSplice
--- PASS: TestRun_MarkdownFormat_DataDrivenSplice (0.00s)
=== RUN   TestRun_MarkdownFormat_ParallelWaves
--- PASS: TestRun_MarkdownFormat_ParallelWaves (0.00s)
ok  	github.com/peterlindqvist/apitest/cmd/apitest	0.311s
```

Expected: parallel wave grouping, data-driven per-iteration files, splice safety
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | wave_index: N for parallel, wave_index: sequential for WaveIndex==-1 | `TestMarkdown_ParallelWaves`, `TestMarkdown_SequentialWaveIndex` | PASS |
| 2 | run.md renders parallel with `## Wave <N>` headers in ascending order; sequential keeps flat list | `TestMarkdown_RunMD_WaveGrouping`, `TestMarkdown_RunMD_WaveGrouping_MixedSequential` | PASS |
| 3 | Data-driven main requests produce one .md per iteration at `<report>/<slug>/iter-<n>.md` | `TestMarkdown_DataDriven_PerIteration`, `TestRun_MarkdownFormat_DataDriven` | PASS |
| 4 | `<report>/<slug>/index.md` has sentinel, run_id, summary counts, iteration table | `TestMarkdown_DataDriven_IndexSummary`, `TestMarkdown_RenderIndex` | PASS |
| 5 | Correlation IDs: `request_id=req-N-iter-M`, stable slug across iterations | `TestMarkdown_DataDriven_CorrelationIDs`, `TestMarkdown_RenderIteration` | PASS |
| 6 | run.md collapses data-driven to single bullet with aggregate counts | `TestMarkdown_DataDriven_IndexSummary`, `TestMarkdown_RunMD_DataDrivenEntry` | PASS |
| 7 | Iteration cap: truncation marker in index.md; iter files beyond cap not written | `TestMarkdown_DataDriven_IterCap`, `TestMarkdown_RenderIndex_Truncation` | PASS |
| 8 | Splice determinism: re-run rewrites sentinel region only; agent edits outside survive | `TestMarkdown_DataDriven_Splice`, `TestRun_MarkdownFormat_DataDrivenSplice` | PASS |
| 9 | Failed iteration renders full 10-section structure with `[ ]` markers, splice-safe | `TestMarkdown_DataDriven_FailedIteration` | PASS |
| 10 | ensureReportDir called with slug subpath | `TestMarkdown_DataDriven_PerIteration` (exercises dir creation) | PASS |
| 11 | Integration: 3-wave parallel collection produces 3 distinct wave_index values and 3 ## Wave headers | `TestRun_MarkdownFormat_ParallelWaves` | PASS |
| 12 | Integration: 5-iteration data-driven collection produces 5 iter-*.md + 1 index.md; re-run preserves agent edits | `TestRun_MarkdownFormat_DataDriven`, `TestRun_MarkdownFormat_DataDrivenSplice` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All listed tests pass | PASS |
| 2 | TestMarkdown_ParallelWaves | PASS (0.00s) | PASS |
| 3 | TestMarkdown_SequentialWaveIndex | PASS (0.00s) | PASS |
| 4 | TestMarkdown_RunMD_WaveGrouping | PASS (0.00s) | PASS |
| 5 | TestMarkdown_DataDriven_PerIteration | PASS (0.00s) | PASS |
| 6 | TestMarkdown_DataDriven_IndexSummary | PASS (0.00s) | PASS |
| 7 | TestMarkdown_DataDriven_CorrelationIDs | PASS (0.00s) | PASS |
| 8 | TestMarkdown_DataDriven_Splice | PASS (0.00s) | PASS |
| 9 | TestMarkdown_DataDriven_IterCap | PASS (0.15s) | PASS |
| 10 | TestMarkdown_DataDriven_FailedIteration | PASS (0.00s) | PASS |
| 11 | TestMarkdown_RunMD_DataDrivenEntry | PASS (0.00s) | PASS |
| 12 | Regression: M9-001/002/003 tests pass | `go test ./...` all pass | PASS |
| 13 | go test ./... passes | ci-local.sh PASS | PASS |
| 14 | coverage markdown >= 80% | 92.3% | PASS |
| 15 | golangci-lint run 0 issues | ci-local.sh output: "0 issues." | PASS |
| 16 | ./smoke/run.sh passes | ci-local.sh: Smoke Test Complete | PASS |
| 17 | ./scripts/ci-local.sh passes | ci-local PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: "Review PASS trusted (iteration 2), spot-check clean."
Spot-checked: `fmt.Errorf("write iter-%d.md: %w", it.Index, err)` correctly wraps errors; exported functions have doc comments; `TestMarkdown_DataDriven_Splice` exercises real agent-edit preservation end-to-end.

## Commits

| Hash | Message |
|------|---------|
| 40b8403 | docs(review): add passing review for M9-004 |
| e16fa59 | docs(review): add improvement report for M9-004 |
| 21c4a67 | fix(test): strengthen parallel wave + sequential branch coverage |
| 36737a6 | docs(review): add review with findings for M9-004 |
| 98aef4c | chore(task): mark M9-004 as review |
| e57c190 | refactor(output): apply gofumpt formatting, fix De Morgan's law lint |
| 95f5f2b | feat(output): wire data-driven dispatch in WriteReport, fix parallel slug fallback |
| 106ad91 | test(output): add failing tests for data-driven per-iteration files and integration |
| 6160e2c | test(output): add failing tests for run.md wave grouping |
| dd0b8f0 | feat(output): add IsParallel, Iterations, IterationEntry types to markdown formatter |
| e86d716 | chore(task): mark M9-004 as in_progress |
| 7dc6398 | chore(task): mark M9-004 as planned |
| 594e539 | docs(plan): add implementation plan for M9-004 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/output/markdown/datadriven.go` | added | Data-driven per-iteration file writer |
| `internal/output/markdown/datadriven_test.go` | added | 13 new tests for data-driven behaviors |
| `internal/output/markdown/formatter.go` | modified | IterationEntry, IsParallel, Iterations fields; dispatch to DD path |
| `internal/output/markdown/run_md.go` | modified | Wave grouping (renderRunMDByWave), DD aggregate bullet |
| `internal/output/markdown/run_md_test.go` | modified | Wave grouping and mixed-sequential tests |
| `cmd/apitest/main.go` | modified | buildMarkdownReport: DD grouping, IsParallel propagation |
| `cmd/apitest/run_test.go` | modified | Integration tests for parallel waves and data-driven layout |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
