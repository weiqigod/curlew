# Code Review: M11-003 (iteration 3)

**Task:** TAP output completeness: per-iteration data-driven test points + parallel speedup diagnostic
**Reviewer:** AI
**Date:** 2026-04-26
**Branch:** feature/M11-003-tap-data-driven-iterations-parallel-speedup

## Verdict: PASS

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All `fmt.Fprintf`/`fmt.Fprintln` errors propagated immediately via `return err` in `writeTAPParallelDiagnostic`. No swallowed errors. `buildParallelMetadata` handles nil summary defensively. `isParallel && summary.WaveCount > 1` guard in `main.go` prevents nil dereference on `summary` (isParallel implies `summary != nil`). |
| Input Validation | PASS | Nil guard on `parallel` in `WriteTAP` (`parallel != nil && parallel.WaveCount > 1`). `buildParallelMetadata` returns zero values for nil input. Both call sites correctly handle nil/non-nil `parallelInfo`. Wave-count guard (`> 1`) prevents single-wave diagnostic. |
| Naming | PASS | `ParallelTAP` has no stutter. `writeTAPParallelDiagnostic` is unexported and descriptive. `buildParallelMetadata` is unexported. All exported symbols (`ParallelTAP`, `WriteTAP`) have doc comments. |
| Code Organization | PASS | `ParallelTAP` lives in `internal/output/tap.go`. `buildParallelMetadata` in `cmd/apitest/main.go` is shared between JSON and TAP paths — no duplication. No circular dependencies introduced. All call sites updated (lines 815, 1488 in `main.go`; test call sites pass `nil` or the new arg correctly). |
| Correctness | PASS | `speedup_factor` formula matches JSON path (both use `buildParallelMetadata`). `sanitizeTAPName` preserves `[i/N]` suffixes (no `#`, no whitespace stripped). `buildParallelMetadata` returns `speedup = 0` when `Duration == 0` (defensive). Tolerance comment (≤ 0.15) and guard code (`> 0.15`) are consistent (iteration 2 finding resolved). |
| Test Quality | PASS | `TestTAPOutput_ParallelSpeedup` performs real cross-formatter parity comparison: two separate invocations, both speedup values extracted and compared within a stated ≤ 0.15 tolerance; comments and code agree. `TestWriteTAP_ParallelSpeedup` has 6 subtests covering nil, wave_count==1, wave_count>1, coexistence with wave comments, formatting, and ordering. `TestTAPOutput_DataDrivenIterations` checks group marker, per-iteration suffix, and plan count. `TestRunCmdDirect_DataDriven_TAPFormat` extended with [i/N] suffix assertion. All tests use `httptest.NewServer` (no real network in unit/integration tests). |

## Test Coverage

- Coverage: `internal/output` 92.5% (above 80%), `cmd/apitest` 82.0% (above 80%)
- `writeTAPParallelDiagnostic` i/o error branches are not covered (require a broken writer); acceptable — same pattern as `writeTAPDiagnostics` and all other `WriteTAP` error paths.
- All 9 behaviors from the task YAML are covered by at least one test.

## Behavior Coverage

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | One test point per data-driven iteration with [i/N] suffix, group marker preserved | `TestTAPOutput_DataDrivenIterations`, `TestWriteTAP_DataDrivenAnnotations` | PASS |
| 2 | Plan count 1..N matches per-iteration test-point count | `TestTAPOutput_DataDrivenIterations` plan check (assertion 3) | PASS |
| 3 | YAML diagnostic block emitted when IsParallel && WaveCount > 1 | `TestTAPOutput_ParallelSpeedup`, `TestWriteTAP_ParallelSpeedup` | PASS |
| 4 | TestTAPOutput_DataDrivenIterations exists and asserts iteration suffix + plan count | test exists and passes | PASS |
| 5 | TestTAPOutput_ParallelSpeedup asserts speedup_factor matches JSON formatter's value | parity comparison present; comment and code agree (≤ 0.15 tolerance) | PASS |
| 6 | SPECIFICATION.md TAP section documents per-iteration test points and parallel YAML diagnostic | data-driven and parallel diagnostic examples added at line 3148 | PASS |
| 7 | MANUAL.md TAP-format section gains data-driven TAP example and parallel-run TAP example | both examples added with APITEST_TIER note | PASS |
| 8 | CHANGELOG.md [Unreleased] Added entry | entry added under ### Added | PASS |
| 9 | IMPROVEMENT.md §2.4 bullets 1 (TAP half) + 2 annotated with 'Shipped (M11-003)' | both bullets annotated | PASS |

## Summary

The iteration-3 implementation is clean. The only finding from iteration 2 (comment/code threshold mismatch at ≤ 0.1 vs > 0.15) has been resolved — all three comment occurrences now read "≤ 0.15" consistently with the guard `if diff > 0.15`. All core implementation aspects — `WriteTAP` signature extension, `ParallelTAP` struct, `writeTAPParallelDiagnostic`, and `buildParallelMetadata` shared helper — are correct and fully tested. All 9 task behaviors are covered by at least one test. The Go gate (`./scripts/ci-local.sh --go`) passes.
