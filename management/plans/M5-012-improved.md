# Improvement Report: M5-012

**Task:** go-cli: perf metrics and HTML/JSON report
**Date:** 2026-04-20
**Review:** management/reviews/M5-012-review.md

## Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `writeReportFile` returned the `write(f)` error bare with no context — callers could not identify which file caused the failure | Wrapped with `fmt.Errorf("writing %s: %w", path, err)` | ✓ tests pass |
| 2 | Low | `f.Close()` error on success path returned bare with no context | Wrapped with `fmt.Errorf("closing %s: %w", path, err)` | ✓ tests pass |
| 3 | Low | Exported constants `FormatStdout`, `FormatJSON`, `FormatHTML` used inline comments instead of standalone doc comments as required by CLAUDE.md | Replaced inline `// comment` with standalone `// FormatX is ...` doc-comment lines above each constant | ✓ tests pass |
| 4 | Low | `percentile` had two dead defensive branches (`rank < 0`, `rank >= len(sorted)`) that were unreachable for all valid inputs, suppressing branch coverage | Removed dead branches; replaced with a doc comment proving the rank invariant (rank always in [0, n-1] for valid p and non-empty slice) | ✓ tests pass |
| 5 | Low | `nowFn` package-level variable was declared as a test seam but never used in any test file, making it dead non-exported surface | Added `TestWriteJSON_GeneratedAtUsesNowFn` which overrides `nowFn` to a fixed timestamp and asserts `generated_at` is deterministic | ✓ tests pass |

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 6 | Medium | `SummaryLine()` omitted throughput (req/s) from the stdout summary line. Behavior 1 explicitly requires throughput in the summary: *"the summary line includes requests, p50, p95, p99 latencies (ms), throughput (req/s), and error_rate (%)"* | Added `throughput=%.1freq/s` field to `SummaryLine` format string between p99 and error_rate; updated `TestSummaryLine_MatchesObservable` and added `TestSummaryLine_IncludesThroughput` | ✓ tests pass |

## Resolved Findings (Iteration 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 7 | Low | `printPerfHelp()` lacked an `Examples:` block required by Definition of Done ("Help text documents --output, --format and example invocations"). No test verified that examples appeared. | Added `Examples:` block with three concrete invocations to `printPerfHelp()`; updated `TestPrintPerfHelp_MentionsAllFlags` to assert `Examples:` block and `--output report.json` example are present | ✓ tests pass |

## Resolved Findings (Iteration 4)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 8 | Medium | Logical bug in `TestPrintPerfHelp_MentionsAllFlags` (line 156): `&&` instead of `\|\|` made the `--output report.json` examples assertion permanently dead — since `"curlew perf"` always appears in the usage line, the first operand was always `false`, making the entire `&&` condition always `false`. | Changed `&&` to `\|\|` so the assertion fires if either string is absent | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/loadgen/report`) | 94.9% |
| Coverage (`internal/loadgen`) | 96.2% |
| Coverage (`cmd/curlew`) | 81.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `4d7f950` | fix(perf): wrap write and close errors with context in writeReportFile | #1, #2 |
| `f1ae1aa` | fix(report): replace inline comments with standalone doc comments on exported constants | #3 |
| `a9cafa2` | fix(report): remove dead defensive branches from percentile and document invariants | #4 |
| `4c248e4` | test(report): exercise nowFn seam in TestWriteJSON_GeneratedAtUsesNowFn | #5 |
| `e071102` | fix(report): add throughput to SummaryLine output | #6 |
| `42493f6` | fix(perf): add Examples block to printPerfHelp and assert in test | #7 |
| `b81cac3` | fix(test): correct && to \|\| in TestPrintPerfHelp_MentionsAllFlags assertion | #8 |

## Summary

8/8 findings resolved. 0 deferred. Iteration 1 resolved 5 Low findings; iteration 2 resolved 1 Medium finding (throughput missing from stdout summary); iteration 3 resolved 1 Low finding (missing Examples block in help text); iteration 4 resolved 1 Medium finding (logical operator bug making test assertion permanently dead).
