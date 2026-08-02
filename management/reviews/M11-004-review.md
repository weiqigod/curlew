# Code Review: M11-004

**Task:** exec --log correlation IDs: additive run_id + request_id fields
**Reviewer:** AI
**Date:** 2026-04-26
**Branch:** feature/M11-004-exec-log-correlation-ids

## Verdict: PASS

## Findings

No findings. All previous review findings (iteration 1) have been resolved:
- Finding #1 (Medium): `ids` package coverage below 80% — fixed by injecting `randRead` variable and adding `ids_fallback_test.go` with `TestNewRunID_FallbackOnRandError`. Coverage is now 100%.
- Finding #2 (Low): missing HTTP-error log path test — fixed by adding `TestExecCmd_LogHTTPError_RunIDPresent` that exercises the `execErr != nil` log branch via a refused-port connection. `ln.Close()` error properly checked.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned, wrapped with `%w`, `AppendJSONL` errors printed to stderr as designed; `ln.Close()` error checked in test |
| Input Validation | PASS | `omitempty` fields handle empty/zero correctly; no nil-dereference risk; `randRead` injection is safe via `t.Cleanup` restore |
| Naming | PASS | `ids.NewRunID` is clean; `execRequestID` is a well-named constant; `randRead` is unexported; exported symbols have doc comments |
| Code Organization | PASS | `internal/output/ids` sub-package correctly placed; no circular imports; `runner` and `events` cleanly delegate; `cmd/curlew/` entry-point role maintained |
| Correctness | PASS | `runID` minted once per invocation before any log-write branches (satisfies "every invocation has a run_id" invariant); all three log-write paths (dry-run, exec-error, exec-success) populate both fields; `omitempty` preserves backward-compat schema; unused `crypto/rand`/`encoding/hex` imports removed from `runner.go` and `emitter.go` |
| Test Quality | PASS | All three exec log paths covered (success, dry-run, HTTP-error); `ids` package at 100% including fallback path via injected reader; `jsonl_test.go` extended with round-trip and omitempty assertions for both new fields; table-driven tests throughout |

## Spec / Behavior Compliance

All 10 behaviors from the task YAML are covered by tests and implementation:

| Behavior | Test(s) | Implementation |
|----------|---------|---------------|
| JSONLEntry gains RunID/RequestID with omitempty tags | `TestAppendJSONL` (two new cases) | `internal/output/jsonl.go` |
| Shared 32-char hex generator via `internal/output/ids` | `TestNewRunID`, `TestNewRunID_DistinctAcrossCalls`, `TestNewRunID_FallbackOnRandError` | `ids.go`; `runner.go` and `emitter.go` delegate |
| RequestID == "req-1" for exec | `TestExecCmd_LogContainsRunIDAndRequestID`, `TestExecCmd_LogDryRun_RunIDPresent`, `TestExecCmd_LogHTTPError_RunIDPresent` | `const execRequestID = "req-1"` in `execCmdOut` |
| Live exec emits run_id + request_id | `TestExecCmd_LogContainsRunIDAndRequestID` | `execCmdOut` success path |
| Dry-run emits run_id | `TestExecCmd_LogDryRun_RunIDPresent` | `execCmdOut` dry-run path |
| MANUAL.md §4.5 example updated | observable grep | `docs/MANUAL.md` line 1389 |
| SPECIFICATION.md expanded | observable grep | `docs/SPECIFICATION.md` line 9252 |
| CHANGELOG.md Added entry | observable grep | `CHANGELOG.md` line 29 |
| IMPROVEMENT.md §6 W5 bonus annotated | observable grep | `IMPROVEMENT.md` line 355 |
| IMPROVEMENT.md line 3 status header updated | observable grep | `IMPROVEMENT.md` line 3 |

## Test Coverage

- `internal/output/ids`: **100.0%** ✓
- `internal/output`: **92.5%** ✓
- `internal/output/events`: **97.8%** ✓
- `cmd/curlew`: **82.3%** ✓
- All packages above DoD threshold (>= 80%)

## Summary

The implementation is complete, correct, and fully tested. The two findings from iteration 1 are cleanly resolved: the `ids` package now has 100% coverage via a testability seam (`randRead` injectable variable) with a white-box fallback test, and the HTTP-error exec log path has its own integration test. All 10 task behaviors are verified by tests, the shared run-id generator is properly factored, and documentation updates (MANUAL.md, SPECIFICATION.md, CHANGELOG.md, IMPROVEMENT.md) are all in place.
