# Code Review: M1-001

**Task:** Run a single GET request from a collection file
**Reviewer:** AI
**Date:** 2026-03-10
**Branch:** feature/M1-001-run-single-get

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`, sentinel errors for well-known failures, Go 1.20+ multi-wrap preserving inner errors, no swallowed errors, no panics for expected failures |
| Input Validation | PASS | Edge cases handled: missing file, invalid YAML, empty collection name, empty args, nil headers map (no-op range), empty requests array (exit 0), 0-byte file (ErrEmptyCollection) |
| Naming | PASS | No stuttering, short names in tight scopes, all exported symbols have doc comments, package names lowercase single-word |
| Code Organization | PASS | `internal/` boundaries respected, no circular deps (parser ← httpexec ← output), minimal exports, `go.mod` correct (no stale `// indirect`) |
| Correctness | PASS | Body drained then closed, context propagated to HTTP request, defer placed after nil-resp check, empty collection handled gracefully |
| Test Quality | PASS | Table-driven with `t.Run()`, error paths and edge cases covered, integration tests with real binary via `os/exec`, httptest servers, testdata fixtures, multi-wrap verification tests |

## Test Coverage
- Coverage: 94.8% overall (cmd/curlew 95.8%, internal/httpexec 92.9%, internal/output 100%, internal/parser 90.9%)
- Missing coverage: `main()` (untestable `os.Exit` wrapper — acceptable), one branch in `ParseFile` (non-`ErrNotExist` file read error), one branch in `Execute` (request build error — only reachable with malformed URL)

## Behavior Coverage

All 10 behaviors from the task YAML are covered by at least one test:
1. Valid GET request → `TestCLIIntegration_successful_run`, `TestRunCmd_successful_request`
2. Output shows name/status/duration → `TestCLIIntegration_successful_run`, `TestPrintResult`
3. Missing file → exit 3 → `TestCLIIntegration`, `TestRunCmd_missing_file`
4. Invalid YAML → exit 3 → `TestCLIIntegration`, `TestRunCmd_invalid_yaml`
5. Network failure → exit 4 → `TestCLIIntegration_network_error`, `TestRunCmd_network_error`
6. `run` no args → exit 1 → `TestCLIIntegration`, `TestRunCmd_no_args`
7. `--help` → `TestCLIIntegration`, `TestRun_help`
8. `--version` → `TestCLIIntegration`, `TestRun_version`
9. Successful → exit 0 → `TestCLIIntegration_successful_run`, `TestRunCmd_successful_request`
10. Sample file exists → `sample/hello.yaml` present, exercised by `smoke/run.sh`

## Summary

Clean, well-structured first vertical slice. Four packages (parser, httpexec, output, cmd) with clear separation of concerns and no circular dependencies. Error handling follows Go 1.20+ best practices with multi-error wrapping preserving full error chains. Test coverage at 94.8% with both unit and integration tests exercising all 10 specified behaviors. All prior review findings (error wrapping, package rename, dead code, body drain, go.mod) have been resolved.
