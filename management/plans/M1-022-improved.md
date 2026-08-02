# Improvement Report: M1-022

**Task:** Verbosity levels (-v, -vv, -q)
**Date:** 2026-03-17
**Review:** management/reviews/M1-022-review.md

## Resolved Findings (Round 1 — 228df5d)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `ResponseDetail` 0% test coverage | Added `TestPrinterVerbosityVerbose_ShowsResponseDetail` and `TestPrinterVerbosityDefault_NoResponseDetail` to `terminal_test.go` | ✓ `ResponseDetail` now 100% |
| 2 | Medium | `RequestBodyDump` 0% test coverage | Added `TestPrinterVerbosityDebug_ShowsRequestBodyDump`, `TestPrinterVerbosityVerbose_NoRequestBodyDump`, and `TestPrinterVerbosityDebug_RequestBodyDump_NilBody` | ✓ `RequestBodyDump` now 100% |
| 3 | Low | `Verbosity.String()` "unknown" default case untested | Added `{"unknown value", Verbosity(99), "unknown"}` entry to `TestVerbosity` table | ✓ `String()` now 100% |
| 4 | Low | `-v` help check fragile (substring match against `-vv`) | Changed check to `strings.Contains(buf.String(), "  -v ")` to require leading spaces and trailing space | ✓ test passes, unambiguous |
| 5 | Low | Missing `TestRunCmdDirect_QuietMode_OnlySummary` and `TestRunCmdDirect_VerboseMode_ShowsHeaders` integration tests | Added both tests using `captureRunCmd` + `httptest.NewServer` pattern; quiet verifies no collection header + summary present; verbose verifies `> GET`, `< 200`, and custom response header appear | ✓ both tests pass |

## Resolved Findings (Round 2 — cb4637a)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `ResponseBodyDump` truncation branch (`len(body) > maxBytes`) uncovered — function at 83.3% | Added `TestPrinterVerbosityDebug_ResponseBodyDump_Truncation` in `terminal_test.go` with a 10 KB+1 body; asserts `(body, truncated)` prefix and `[N bytes total]` byte-count line appear | ✓ `ResponseBodyDump` now 100% |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/output` | 91.4% |
| Coverage `cmd/curlew` | 85.9% |
| Coverage `internal/output/terminal.go:ResponseBodyDump` | 100% |
| Coverage `internal/output/terminal.go:ResponseDetail` | 100% |
| Coverage `internal/output/terminal.go:RequestBodyDump` | 100% |
| Coverage `internal/output/verbosity.go:String` | 100% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 228df5d | test(output): add missing tests from M1-022 review findings | Round 1: #1–5 |
| cb4637a | test(output): add ResponseBodyDump truncation test | Round 2: #1 |

## Summary

6/6 total findings resolved across 2 review cycles. 0 deferred.
