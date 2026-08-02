# Verification Report: M14-004

**Task:** CLI: internal/backend HTTP client foundation with flock + hybrid keychain
**Verified by:** AI
**Date:** 2026-05-04
**Branch:** feature/M14-004-internal-backend-client
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (`internal/backend`) | 83.8% | Exceeds >= 80% threshold |
| Test count (`internal/backend/...`) | 23 tests | Exceeds >= 14 DoD threshold |

## Observable Output

```
OK: client built; lock OK; keychain available=true; rfc7807 mapping OK
```

Command: `CURLEW_INTERNAL=1 ./curlew internal backend-probe --base http://127.0.0.1:0 --self-test`
Expected: `OK: client built; lock OK; keychain available=<bool>; rfc7807 mapping OK`
Result: MATCH (exit 0)

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | 200 response parsed into typed result, nil error | `TestClient_GetJSON/200_application/json_parses_into_typed_result` | PASS |
| 2 | 401 problem+json AUTH_REFRESH_REUSED → typed *ProblemDetails with Code/Title/Type/RequestID | `TestClient_GetJSON/401_problem+json_AUTH_REFRESH_REUSED_yields_*ProblemDetails`, `TestDecodeProblem/AUTH_REFRESH_REUSED_canonical_body` | PASS |
| 3 | No keychain → AES-256-GCM encrypted-file fallback at mode 0600 | `TestStorage_NoKeychainFallsBackToFile`, `TestEncryptedFile_FileMode0600` | PASS |
| 4 | Two concurrent RefreshTokens calls → exactly one acquires flock, other blocks | `TestRefreshTokens_Concurrent_OnlyOneNetworkCall` | PASS |
| 5 | Stale lock + 5s timeout → fallback to cache, no second refresh | `TestRefreshTokens_HoldStaleLockTimesOutAndReadsCache` | PASS |
| 6 | Bearer token in Authorization header; refresh token in JSON body only | `TestClient_PostJSON_setsBearerHeader`, `TestClient_GetJSON_setsBearerHeader`, `TestClient_BuildRefreshRequest_neverSetsAuthorizationHeader` | PASS |
| 7 | OS keychain available → tokens go to keychain not encrypted file | `TestStorage_KeychainAvailable_Darwin` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `go test ./internal/backend/... passes with >=14 tests` | 23 tests pass | PASS |
| 2 | `curlew internal backend-probe --self-test` exits 0 with documented stdout | `OK: client built; lock OK; keychain available=true; rfc7807 mapping OK` | PASS |
| 3 | `ProblemDetails` exported with stable JSON tags; godoc explains Code-not-Status branching rule | `problem.go` doc comments + `doc.go` | PASS |
| 4 | Storage fallback covered by test simulating "no keychain" on Linux | `TestStorage_NoKeychainFallsBackToFile` uses `keyring.MockInitWithError` | PASS |
| 5 | flock contention covered by 2-goroutine test asserting ordering | `TestRefreshTokens_Concurrent_OnlyOneNetworkCall` with `handlerExitTime` ordering guard | PASS |
| 6 | Help text adds `internal backend-probe` as hidden (gated behind `CURLEW_INTERNAL=1`) | `TestPrintHelp_DoesNotMentionInternal` passes | PASS |
| 7 | `doc.go` cites `SPECIFICATION.md:7938–7951` + `:8195–8202` | All four spec line refs present in `doc.go` | PASS |

## Code Review

Review verdict from `management/reviews/M14-004-review.md`: **PASS** (Iteration 4, post-improvement × 3)

| Check | Status |
|-------|--------|
| Error handling (all wrapped with `%w`) | PASS |
| Naming conventions (no stutter, doc comments on all exports) | PASS |
| Code organization (internal/ boundaries, single responsibility) | PASS |
| Test quality (table-driven, error paths, binary E2E) | PASS |
| Input validation (empty BaseURL, ConfigDir, refresh token) | PASS |
| Sentinel errors for well-known failure modes | PASS |
| Bearer-only auth header policy enforced | PASS |

Spot-checks:
- `storage.go` error wrapping: `keyring.Set`, `keyring.Delete`, `machine-id` all use `fmt.Errorf("context: %w", err)` ✓
- `ProblemDetails`, `ErrNotProblem`, `DecodeProblem`, `ErrTokenNotFound` all have doc comments ✓
- `TestRefreshTokens_Concurrent_OnlyOneNetworkCall` exercises real concurrency with ordering assertions ✓

(Branch A: Review PASS trusted, spot-check clean)

## Commits

| Hash | Message |
|------|---------|
| `549817f1` | docs(review): add passing review for M14-004 |
| `b31f2c19` | docs(review): update improvement report for M14-004 (iteration 3) |
| `eff41087` | test(backend): add TestStorage_SetEmptyTokenRejected for both backends |
| `cd67f5bc` | fix(backend): wrap keyring.Set and keyring.Delete errors with context |
| `06c26e89` | docs(review): add review with findings for M14-004 |
| `279c4662` | docs(review): update improvement report for M14-004 (iteration 2) |
| `39d06942` | test(backend): add TestClient_GetJSON_setsBearerHeader |
| `46f13315` | docs(review): add review with findings for M14-004 |
| `7979ba8d` | docs(review): add improvement report for M14-004 |
| `5670182a` | fix(backend): resolve all M14-004 review findings |
| `e87b5583` | docs(review): add review with findings for M14-004 |
| `161af6f3` | chore(task): mark M14-004 as review |
| `b3874128` | refactor(errors): fix gofumpt ordering of backend import in coverage_test |
| `cd203968` | refactor(backend): fix errcheck and gofumpt lint issues |
| `5bd7837f` | feat(cli): implement internal backend-probe subcommand (M14-004) |
| `8737a2d8` | test(cli): add failing tests for internal backend-probe subcommand |
| `8e2f5dba` | test(backend): add failing tests for client, encfile, storage, lock |
| `e15223f3` | test(backend): add failing tests for ProblemDetails decoding |
| `028411be` | feat(backend): add gofrs/flock and zalando/go-keyring dependencies |
| `32f120b4` | chore(task): mark M14-004 as in_progress |
| `b9478bfc` | chore(task): mark M14-004 as planned |
| `7c60669d` | docs(plan): add implementation plan for M14-004 |

TDD pattern visible: `test(...)` commits appear before `feat(...)` commits ✓

## Files Changed

| File | Action |
|------|--------|
| `internal/backend/doc.go` | created |
| `internal/backend/problem.go` | created |
| `internal/backend/problem_test.go` | created |
| `internal/backend/client.go` | created |
| `internal/backend/client_test.go` | created |
| `internal/backend/encfile.go` | created |
| `internal/backend/encfile_test.go` | created |
| `internal/backend/storage.go` | created |
| `internal/backend/storage_test.go` | created |
| `internal/backend/lock.go` | created |
| `internal/backend/lock_test.go` | created |
| `internal/backend/hints_init.go` | created |
| `cmd/curlew/internal_cmd.go` | created |
| `cmd/curlew/internal_cmd_test.go` | created |
| `cmd/curlew/main.go` | modified |
| `internal/errors/coverage_test.go` | modified |
| `go.mod` | modified |
| `go.sum` | modified |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
