# Improvement Report: M14-004

**Task:** CLI: internal/backend HTTP client foundation with flock + hybrid keychain
**Date:** 2026-05-04
**Review:** management/reviews/M14-004-review.md

## Resolved Findings (Iteration 1 — commit 5670182a)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `BuildRefreshRequest` silently discards `json.Encode` error with `_ =` | Replaced `_ = json.NewEncoder(&buf).Encode(payload)` with a proper `if err := ...; err != nil { return nil, fmt.Errorf("encode refresh payload: %w", err) }` guard | ✓ tests pass |
| 2 | Medium | `EncryptedFile`/`NewEncryptedFile` exported unnecessarily; plan specified lowercase; tests in `package backend_test` required export | Renamed to `encryptedFile`/`newEncryptedFile` (unexported). Moved `encfile_test.go` from `package backend_test` to `package backend` (internal test file) so tests access the unexported type directly. Updated `storage.go` to use `newEncryptedFile`. | ✓ tests pass |
| 3 | Medium | Concurrency test only checked call count and token values; no ordering assertion proving second goroutine blocked | Added `starts [2]time.Time` and `handlerExitTime` tracking. After `wg.Wait()`, asserts that at least one goroutine started before the handler finished — proving the goroutines were concurrent rather than sequentially executed. | ✓ tests pass |
| 4 | Low | `machineIDForTest` dead var in production code (`encfile.go`), never assigned by any test | Removed `machineIDForTest` variable and the `if machineIDForTest != nil` delegation block entirely from `encfile.go` | ✓ tests pass |
| 5 | Low | Empty `case "darwin":` no-op branch in `machineID()` switch | Removed the `case "darwin":` block; the comment about ioreg non-implementation moved to the fallback block's comment | ✓ tests pass |
| 6 | Low | `keychainStorage.DeleteRefreshToken` had 0% statement coverage | Added `DeleteRefreshToken()` calls (first + idempotent second) to `TestStorage_KeychainAvailable_Darwin`; coverage for that function is now 100% | ✓ tests pass |
| 7 | Low | `errors.Is(tc.wantErr, backend.ErrNotProblem)` semantically backwards in `problem_test.go` | Replaced with `tc.wantErr == backend.ErrNotProblem` for the sentinel equality check; `errors.Is` is reserved for checking the returned error (the function under test) | ✓ tests pass |

## Resolved Findings (Iteration 2 — commit 39d06942)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 8 | Low | Behavior #6 ("Bearer header is set on any request") only tested for `PostJSON`, not `GetJSON`; the `if accessToken != ""` branch in `GetJSON` (client.go:73–75) was uncovered | Added `TestClient_GetJSON_setsBearerHeader`: httptest server records the `Authorization` header; `GetJSON` called with token `"abc123"`; asserts `got == "Bearer abc123"` | ✓ tests pass |

## Resolved Findings (Iteration 3 — commits cd67f5bc, eff41087)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 9 | Medium | `keyring.Set` returned bare error without `fmt.Errorf` wrapping in `keychainStorage.SetRefreshToken` (line 109) | Replaced `return keyring.Set(...)` with `if err := keyring.Set(...); err != nil { return fmt.Errorf("keyring set: %w", err) }` | ✓ tests pass |
| 10 | Medium | `keyring.Delete` non-`ErrNotFound` path returned bare error without wrapping in `keychainStorage.DeleteRefreshToken` (line 118) | Added `if err != nil { return fmt.Errorf("keyring delete: %w", err) }` before `return nil` | ✓ tests pass |
| 11 | Low | `SetRefreshToken("")` empty-token rejection untested for both backends — guard branches at 0% coverage | Added `TestStorage_SetEmptyTokenRejected` table-driven test covering both `ForceFile: true` (file backend) and keychain backend (darwin-only sub-test) | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (internal/backend) | 83.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 5670182a | fix(backend): resolve all M14-004 review findings | #1–#7 |
| 39d06942 | test(backend): add TestClient_GetJSON_setsBearerHeader | #8 |
| cd67f5bc | fix(backend): wrap keyring.Set and keyring.Delete errors with context | #9, #10 |
| eff41087 | test(backend): add TestStorage_SetEmptyTokenRejected for both backends | #11 |

## Summary
11/11 findings resolved across 3 iterations. 0 deferred. Coverage improved from 81.5% to 83.8%.
