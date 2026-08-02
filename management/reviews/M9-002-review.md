# Code Review: M9-002

**Task:** markdown formatter: --format markdown with sentinel splice, JSON body, run.md index
**Reviewer:** AI (iteration 8)
**Date:** 2026-04-25
**Branch:** feature/M9-002-markdown-formatter

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` **passes** (build, test, race, coverage 87.7% markdown / 80.9% cmd / 85.1% runner, lint 0 issues, smoke).

## Previous Findings Status

Iteration 7 had 1 finding:

- Finding #1 (Medium): Parallel data-driven `RequestResult` literals did not populate `RequestID` or `RequestSlug`, contradicting the documented contract → **FIXED**. `datadriven.IterationResult` now carries `RequestID string` and `RequestSlug string` fields; the parallel execFn sets them at lines 2367–2368 and 2409–2410; the conversion loop propagates them at lines 2526–2527. `TestRun_DataDriven_ParallelExecution` asserts both fields are non-empty for all parallel data-driven results.

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`. Sentinel error `errMalformedSentinel` defined. No swallowed errors. `panic` at unreachable switch branch (unhandled `spliceAction`) is appropriate as a programming-error guard. |
| Input Validation | PASS | Nil guards on `entry.Assertions`, `h == nil`, empty body. `parseSentinels` / `parseAllSentinelPairs` handle all malformed sub-cases (BEGIN without END, END without BEGIN, nested BEGIN). Skipped requests correctly carry `RequestSlug: item.Slug` at all skip sites. |
| Naming | PASS | No stuttering. All exported symbols (`WriteReport`, `EnsureReportDir`, `Report`, `WriteOptions`, `SentinelBeginPrefix`, `SentinelEndPrefix`, `SentinelSuffix`, `RunMDSentinelSlug`, `NewRunID`) have doc comments. Package name `markdown` is lowercase single-word. |
| Code Organization | PASS | `internal/output/markdown/` cleanly isolated; cmd layer imports it, no reverse dependency. Formatter, splice, writer, run_md split into clearly scoped files. `datadriven.IterationResult` additive fields are self-contained. |
| Correctness | PASS | Sentinel splice matrix covers all six cases. Parallel data-driven `RequestID`/`RequestSlug` now propagated (fixing iteration 7 finding). Runner always mints `reqID` regardless of `--events`. `run.md` uses same `writeFile` dispatch as per-request files (splice on rerun works by construction). Atomic write via `os.CreateTemp` + `os.Rename` prevents byte-level corruption under concurrency. |
| Test Quality | PASS | All 14 DoD-named tests present and passing. Additional tests added: `TestMarkdown_Splice_SecondRunAfterOrphanAppend`, `TestMarkdown_RunMD_WithEnvName`, `TestRunner_SkippedRequestsCarrySlug`, `TestRun_DataDriven_ParallelExecution` (RequestID/RequestSlug assertions). Coverage 87.7% markdown (above 80% threshold). `writeAtomic` at 56.2% — `f.Write` / `f.Close` error paths require OS-level fault injection and are intentionally left uncovered; the rename path is exercised by every atomic write call. |

## Test Coverage

- Coverage `internal/output/markdown`: **87.7%** (above 80% threshold)
- Coverage `internal/runner`: **85.1%** (above 80% threshold)
- Coverage `cmd/apitest`: **80.9%** (at 80% threshold)
- All 14 DoD-named tests present and passing
- `writeAtomic`: 56.2% — OS fault-injection paths (`f.Write` failure, `f.Close` failure) are not covered; acceptable given difficulty of triggering kernel-level write failures in unit tests
- `renderResponse`: 68.8% — JSON-parse-failure fallthrough not directly tested (requires malformed JSON response body); non-JSON empty-body path not tested; both deferred by design (M9-003 owns the full content-type matrix)

## Summary

All findings from prior iterations are resolved. The parallel data-driven `RequestID`/`RequestSlug` gap (iteration 7's only finding) is fixed in `datadriven/parallel.go` and `runner.go`, with a regression assertion added to `TestRun_DataDriven_ParallelExecution`. The markdown formatter package, splice logic, atomic writer, run.md index, CLI dispatch, schema enum, config validation, and runner plumbing all meet the project standards. Coverage is above threshold for all affected packages.
