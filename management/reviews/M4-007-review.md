# Code Review: M4-007

**Task:** CLI: pr-check subcommand posting status to backend
**Reviewer:** AI
**Date:** 2026-04-16
**Branch:** feature/M4-007-pr-check-subcommand

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. Sentinel errors (`ErrBackendURLMissing`, `ErrUnauthorized`, `ErrNetworkFailure`) defined and used correctly. `resp.Body.Close()` called correctly after `io.ReadAll`. Network failure wrapped via `fmt.Errorf("%w: %v", ErrNetworkFailure, lastErr)` for `errors.Is` compatibility. |
| Input Validation | PASS | `Validate()` checks all required fields with clear error messages. Empty string, zero PR value (`PR <= 0`), and missing file path all produce specific, actionable errors. |
| Naming | PASS | No stuttering. All exported types, functions, methods, and constants have doc comments. Package name `prcheck` is lowercase single-word. `countingRoundTripper` follows `-er` suffix convention. |
| Code Organization | PASS | Clean separation: types/config in `prcheck.go`, HTTP in `client.go`, orchestration in `run.go`. No circular dependencies. `defer resp.Body.Close()` replaced with explicit `resp.Body.Close()` after `ReadAll` (idiomatic for retry loops). `//go:build ignore` tag on `mock_server.go` prevents inclusion in `./...` builds. |
| Correctness | PASS | Response body fully read then closed (correct order). Context propagated through all call chains. Retry loop checks `ctx.Done()` before each sleep. `bytes.NewReader` used per-attempt so the request body is re-readable on retry. 401 short-circuits retries immediately (correct — no point retrying auth errors). |
| Test Quality | PASS | `TestClient_RetryBackoff` (previously hollow) now uses a closed-server + `countingRoundTripper` to assert both attempt count (`maxRetries+1 = 3`) and total elapsed time (>= `maxRetries * retryInterval = 400ms`). All eight task behaviors covered by tests. Table-driven tests with descriptive `t.Run` names. Sentinel errors checked via `errors.Is`. Integration-style tests use `httptest.Server`. |

## Test Coverage
- Coverage: 91.6% (`internal/prcheck`)
- All 36 tests in `internal/prcheck` pass (including 4 retry tests, 7 config validation tests, 5 file-loading tests, 7 run orchestration tests).
- All 8 pr-check CLI tests in `cmd/curlew` pass.
- Total: 44 tests, well above the >=7 requirement from the definition of done.
- Uncovered lines are limited to edge branches in `doWithRetry` that are impractical to trigger deterministically (e.g. read error during body read from a live connection). 91.6% is above the 80% quality gate.

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| POSTs results payload, prints returned result_id | `TestRun_Success`, `TestPrCheckCmd_SuccessAllPass` | PASS |
| POSTs pr-check status payload with repo/pr/state/result_id | `TestRun_FailingTests_StateFailure`, `TestClient_PostPrCheck` | PASS |
| Missing CURLEW_BACKEND_URL → exit 2, 'backend URL not configured' | `TestPrCheckCmd_MissingBackendURL`, `TestRun_MissingConfig_Error` | PASS |
| fail_count > 0 → state=failure, exit 1 | `TestPrCheckCmd_FailingTests_Exit1`, `TestRun_FailingTests_StateFailure` | PASS |
| Backend 401 → 'unauthorized: refresh CURLEW_BACKEND_TOKEN', exit 2 | `TestPrCheckCmd_Unauthorized`, `TestRun_Unauthorized_Error` | PASS |
| --dry-run → no HTTP, stdout shows JSON payloads | `TestPrCheckCmd_DryRun_NoHTTP`, `TestRun_DryRun_NoHTTP` | PASS |
| --help documents all flags and env vars | `TestPrCheckCmd_Help` | PASS |
| Unreachable backend → 2 retries with 200ms backoff, exit 2 | `TestPrCheckCmd_ConnectionRefused_Exit2`, `TestClient_RetryBackoff`, `TestClient_RetryOnConnectionRefused` | PASS |

## Summary

The implementation is correct, well-structured, and complete. The single finding from the first review (hollow `TestClient_RetryBackoff`) was resolved: the test now asserts both attempt count and minimum elapsed time, providing genuine confidence in backoff behavior. All eight task behaviors are covered by tests, all definition-of-done items are satisfied, golangci-lint reports 0 issues, and coverage is 91.6% — above the 80% quality gate.
