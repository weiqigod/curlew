# Code Review: M2-012

**Task:** Watch mode terminal UX and incremental feedback
**Reviewer:** AI
**Date:** 2026-04-01
**Branch:** feature/M2-012-watch-terminal-ux

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All returned errors use `%w` wrapping (`addWatchDirs`). Stderr logging uses `%v` correctly. Early returns in `runCmdInner` return `nil` summary; post-runner returns include `summary`. No swallowed errors. |
| Input Validation | PASS | `--clear` flag parsed and filtered before `parseRunArgs`. `Format` validated by `runCmdInner`. `ClearScreen` correctly suppressed in JSON mode. |
| Naming | PASS | `RunResult`, `RunningTotals`, `runCmdInner` — clear, no stuttering. Doc comments on all exported symbols. |
| Code Organization | PASS | Clean boundary between `internal/watch/` and `cmd/curlew/`. `runCmdInner` extraction is minimal-surface refactor. Exported API is narrow. |
| Correctness | PASS | Single-goroutine event loop — no races (`go test -race` passes). `summary` nil-checked in `watchCmd` closure. `ClearScreen` suppressed in JSON mode. `lastChanged` always set before `printSeparator`. Singular/plural handled correctly in totals. |
| Test Quality | PASS | 26 subtests covering all 5 behaviors. Table-driven tests for `RunningTotals`, `PrintRunningTotals`, `PrintWatchStatus`. Integration tests for JSON suppression, clear flag, parse error recovery. Error paths and edge cases covered. |

## Test Coverage
- Coverage: 89.7%
- Missing coverage: `addWatchDirs` error path (75%), `syncWatchDirs` error paths (85.7%) — pre-existing, not introduced by this task

## Summary

Code is clean after the two improvement fixes (singular/plural totals, separator file name assertion). The `runCmdInner` refactor is mechanical and correct, all five behaviors are implemented and tested, JSON mode properly suppresses all terminal decorations, `--clear` and parse error recovery work as specified. Race detector passes. No issues found.
