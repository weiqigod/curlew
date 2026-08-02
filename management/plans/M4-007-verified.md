# Verification Report: M4-007

**Task:** CLI: pr-check subcommand posting status to backend
**Verified by:** AI
**Date:** 2026-04-16
**Branch:** feature/M4-007-pr-check-subcommand
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 29 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All smoke tests pass, including `PR check dry-run (M4-007)` |
| Coverage (`internal/prcheck`) | 91.6% | Meets >= 80% threshold |
| Coverage (total) | 87.2% | Meets >= 80% threshold |

## Observable Output

```
2026/04/16 10:10:22 mock-backend listening on :18080
2026/04/16 10:10:23 POST /api/v1/organizations/acme/results body={"collection_name":"smoke-tests","run_at":"2026-04-16T10:00:00Z","duration_ms":1234,"pass_count":3,"fail_count":0,"skipped_count":0,"triggered_by":"pr-check","git_sha":"abc1234","items":[...]}
2026/04/16 10:10:23 POST /api/v1/organizations/acme/pr-checks body={"repo":"acme/api","pr":42,"state":"success","result_id":"res_mock123"}
Uploaded result res_mock123 (pass=3 fail=0); status check posted
Exit code: 0
```

Expected: `Uploaded result res_... (pass=3 fail=0); status check posted` — exit 0. Mock log shows POST /api/v1/organizations/acme/results then POST /api/v1/organizations/acme/pr-checks with pr=42.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Valid results file + token → POSTs results, prints result_id | `TestRun_Success`, `TestPrCheckCmd_SuccessAllPass` | PASS |
| 2 | Results upload succeeds → POSTs pr-check payload (repo/pr/state/result_id) | `TestClient_PostPrCheck`, `TestRun_FailingTests_StateFailure` | PASS |
| 3 | Missing CURLEW_BACKEND_URL → exit 2, 'backend URL not configured' | `TestPrCheckCmd_MissingBackendURL`, `TestRun_MissingConfig_Error` | PASS |
| 4 | fail_count > 0 → state=failure, exit 1 | `TestPrCheckCmd_FailingTests_Exit1`, `TestRun_FailingTests_StateFailure` | PASS |
| 5 | Backend 401 → 'unauthorized: refresh CURLEW_BACKEND_TOKEN', exit 2 | `TestPrCheckCmd_Unauthorized`, `TestRun_Unauthorized_Error` | PASS |
| 6 | --dry-run → no HTTP traffic, stdout shows JSON payloads | `TestPrCheckCmd_DryRun_NoHTTP`, `TestRun_DryRun_NoHTTP` | PASS |
| 7 | --help documents all flags and env vars | `TestPrCheckCmd_Help` | PASS |
| 8 | Unreachable backend → 2 retries with 200ms backoff, exit 2 | `TestPrCheckCmd_ConnectionRefused_Exit2`, `TestClient_RetryBackoff`, `TestClient_RetryOnConnectionRefused` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `go test ./internal/prcheck/...` passes with >=7 tests | 36 tests pass in `internal/prcheck` | PASS |
| 2 | Real binary invocation against mock produces expected stdout and exit code | Observable scenario ran successfully (see above) | PASS |
| 3 | Help text documents every flag and env var | `TestPrCheckCmd_Help` verifies --org, --pr, --repo, --results, --dry-run, CURLEW_BACKEND_URL, CURLEW_BACKEND_TOKEN | PASS |
| 4 | testdata/team/sample-junit.json and mock-backend.sh checked in | Both files present in `testdata/team/` | PASS |
| 5 | smoke/run.sh invokes pr-check --dry-run against a fixture | `PR check dry-run (M4-007)` section in smoke/run.sh passes | PASS |
| 6 | Error paths (401, connection refused, missing env) asserted on | Tests: `TestPrCheckCmd_Unauthorized`, `TestPrCheckCmd_ConnectionRefused_Exit2`, `TestPrCheckCmd_MissingBackendURL` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — All errors wrapped with `%w`. Sentinel errors `ErrBackendURLMissing`, `ErrUnauthorized`, `ErrNetworkFailure` defined and used. |
| Naming conventions | PASS — No stuttering. All exported symbols have doc comments. |
| Code organization | PASS — Clean separation: types in `prcheck.go`, HTTP in `client.go`, orchestration in `run.go`. |
| Test quality | PASS — Table-driven tests, real timing assertions for retry, all 8 behaviors covered. |
| Context propagation | PASS — `context.Context` propagated through all call chains. Retry loop checks `ctx.Done()`. |

Branch A: Review PASS trusted, spot-check clean. Error wrapping with `%w` confirmed in `client.go`. Sentinel errors defined in `prcheck.go`. All exported symbols have doc comments.

## Commits

| Hash | Message |
|------|---------|
| 67e2b06 | docs(review): add passing review for M4-007 |
| 7a0e36f | docs(review): add improvement report for M4-007 |
| 0e0e210 | fix(prcheck): replace hollow RetryBackoff test with real timing assertion |
| 4ff0b96 | docs(review): add review with findings for M4-007 |
| 6b8f671 | chore(task): mark M4-007 as review |
| 1c77b19 | refactor(prcheck): simplify contains helper to use strings.Contains |
| 53a6b8e | refactor(prcheck): fix lint issues |
| a044a6f | feat(cli): add pr-check dry-run smoke test |
| 384d917 | feat(cli): add test fixtures and mock backend for pr-check |
| 153f44e | feat(cli): wire pr-check subcommand with flag parsing and help text |
| 7f84026 | test(cli): add failing tests for pr-check subcommand |
| dd23829 | feat(prcheck): implement Run orchestrator |
| 456ba7e | test(prcheck): add failing tests for Run orchestrator |
| c1c5f3f | feat(prcheck): implement HTTP client with retry and backoff |
| e2187de | test(prcheck): add failing tests for HTTP client with retry |
| 46952b4 | feat(prcheck): implement types, config validation, and results file loading |
| 138518e | test(prcheck): add failing tests for types, config, and file loading |

## Files Changed

| File | Action |
|------|--------|
| `cmd/curlew/main.go` | modified — added pr-check subcommand wiring |
| `cmd/curlew/main_test.go` | modified — added pr-check CLI tests |
| `internal/prcheck/client.go` | added — HTTP client with retry |
| `internal/prcheck/client_test.go` | added — client tests |
| `internal/prcheck/prcheck.go` | added — types, config, validation, file loading |
| `internal/prcheck/prcheck_test.go` | added — type/config/file tests |
| `internal/prcheck/run.go` | added — Run orchestrator |
| `internal/prcheck/run_test.go` | added — orchestrator tests |
| `smoke/run.sh` | modified — added pr-check dry-run smoke test |
| `testdata/team/mock-backend.sh` | added — mock backend script |
| `testdata/team/mock_server.go` | added — mock server Go source (go:build ignore) |
| `testdata/team/sample-junit.json` | added — sample passing results fixture |
| `testdata/team/sample-junit-fail.json` | added — sample failing results fixture |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
