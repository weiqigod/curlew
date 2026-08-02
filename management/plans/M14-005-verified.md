# Verification Report: M14-005

**Task:** CLI: apitest login (RFC 8628 device-code flow)
**Verified by:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-005-cli-login-device-code
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All 50 packages, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All smoke blocks pass, M14-005 login help block PASS |
| Coverage (`cmd/apitest`) | 81.7% | Meets >= 80% threshold |
| Coverage (`internal/backend`) | 84.1% | Meets >= 80% threshold |
| Coverage (`internal/backend/device`) | 84.2% | Meets >= 80% threshold |
| Total coverage | 87.4% | Well above threshold |

## Observable Output

```
$ APITEST_BACKEND_URL=http://127.0.0.1:18080 APITEST_CONFIG_DIR=/tmp/apitest-m14-005-demo ./apitest login --no-browser
First, copy your one-time code: ABCD-EFGH
Then visit: https://app.apitool.dev/device
Authentication complete. Welcome, smoke@example.com.
```

```
$ cat /tmp/apitest-m14-005-demo/device.json
{
  "device_id": "dev_stub_001",
  "issued_at": "2026-05-05T05:58:20.586228Z"
}
```

Expected: stdout with "First, copy your one-time code: ABCD-EFGH", "Then visit: https://app.apitool.dev/device", "Authentication complete. Welcome, smoke@example.com." Exit 0; refresh token and device.json persisted.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | TTY without --no-browser → browser opened, code+URI printed as fallback | `TestLogin_TTY_OpensBrowserAtVerificationComplete` | PASS |
| 2 | --no-browser → no browser invoked, only print path | `TestLogin_NoBrowserFlag_SuppressesAutoOpen` | PASS |
| 3 | authorization_pending → sleep interval, retry until success | `TestLogin_Polling_PendingThenSuccess` | PASS |
| 4 | slow_down → +5s to polling interval per RFC 8628 §3.5 | `TestLogin_Polling_SlowDownAddsFiveSeconds` | PASS |
| 5 | expired_token → exit 4 + "Code expired; run apitest login again" | `TestLogin_Polling_ExpiredToken_ExitFour` | PASS |
| 6 | license_jwt, refresh_token, device_id persisted; access_token in-memory only | `TestLogin_PersistsRefreshAndDeviceAndLicense` | PASS |
| 7 | Second login does not call /revoke — old family left intact server-side | `TestLogin_TwoConsecutiveLogins_LeavesOldFamily` | PASS |
| 8 | --help documents --no-browser and device-code UX | `TestLogin_Help_DocumentsNoBrowser` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `go test ./cmd/apitest/... -run TestLogin` passes with >=8 tests | 14 tests pass | PASS |
| 2 | Real binary invocation against stub produces documented stdout, exits 0 | Exact match: ABCD-EFGH, verification_uri, Welcome message | PASS |
| 3 | Help text registers `apitest login --no-browser` and explains device-code UX | Smoke: "PASS: login help mentions --no-browser", "PASS: login help describes device-code UX" | PASS |
| 4 | `testdata/m14/stub-backend.sh` checked in and reusable | Present; supports STUB_BEHAVIOR, --bg mode | PASS |
| 5 | `smoke/run.sh` extended with login help smoke block | M14-005 block at end of smoke/run.sh, all 3 checks pass | PASS |
| 6 | `docs/SPECIFICATION.md:8205-8222` cited in `cmd/apitest/login.go` header | Line 3 of login.go references spec anchor | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — all errors returned, never panicked; `fmt.Errorf("context: %w", err)` used throughout |
| Naming conventions | PASS — no stuttering; doc comments on all exports |
| Code organization | PASS — clean vertical slice, `internal/backend/device` isolated, login.go matches existing structural pattern |
| Test quality | PASS — 14 TestLogin_* tests, 5 device_test subtests, 6 sentinel tests; test seams injected via t.Cleanup |
| Context propagation | PASS — `context.Context` threaded through all HTTP calls |
| Race detector | PASS — no races |

Review was PASS (iteration 3, post-improve). Spot-check: error wrapping uses `%w`, doc comments present on all exports, tests exercise behaviors not just absence of error.

## Commits

| Hash | Message |
|------|---------|
| 306c2213 | docs(review): add passing review for M14-005 |
| 7d9c040e | docs(review): update improvement report for M14-005 iteration 2 |
| e2ee628e | fix(login): add TestTierFromLicenseJWT + clarify access_token persistence in task YAML |
| e881fd60 | docs(review): add review with findings for M14-005 (iteration 2) |
| e9c08f91 | docs(review): add improvement report for M14-005 |
| ac887533 | test(login): resolve review findings #1-#7 — coverage + correctness |
| 1214491e | docs(review): add review with findings for M14-005 |
| 4a7f15d5 | chore(task): mark M14-005 as review |
| 4fd7c971 | feat(cli): extend smoke/run.sh with apitest login --help check |
| 64b719b1 | feat(cli): add testdata/m14/stub-backend.sh and stub_server.go |
| d36d9b37 | feat(cli): wire login subcommand into runWithWriters and top-level help |
| 2577717a | feat(cli): implement loginCmdOut with RFC 8628 device-code flow |
| fc9c95fc | test(cli): add failing tests for apitest login device-code flow |
| 9b28dca8 | feat(backend): implement StartDevice and PollDevice with RFC 8628 sentinel mapping |
| 01b361ff | test(backend): add failing tests for StartDevice and PollDevice sentinel mapping |
| 92c5d478 | feat(backend): implement internal/backend/device package |
| a4d91c5f | test(backend): add failing tests for device.json read/write |

TDD pattern visible: test commits appear before feat commits for each package.

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/login.go` | created |
| `cmd/apitest/login_test.go` | created |
| `cmd/apitest/main.go` | modified (login subcommand wiring) |
| `internal/backend/devicecode.go` | created |
| `internal/backend/devicecode_test.go` | created |
| `internal/backend/device/device.go` | created |
| `internal/backend/device/device_test.go` | created |
| `smoke/run.sh` | modified (M14-005 login help block) |
| `testdata/m14/stub-backend.sh` | created |
| `testdata/m14/stub_server.go` | created |
| `management/tasks/M14-005.yaml` | modified (behavior #6 clarified) |
| `management/backlog.yaml` | modified |
| `management/plans/M14-005-plan.md` | created |
| `management/plans/M14-005-improved.md` | created |
| `management/reviews/M14-005-review.md` | created |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
