# Improvement Report: M11-004

**Task:** exec --log correlation IDs: additive run_id + request_id fields
**Date:** 2026-04-26
**Review:** management/reviews/M11-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `internal/output/ids` package at 75% coverage — below the 80% DoD threshold. The `crypto/rand` error fallback branch in `NewRunID` was unreachable in tests. | Introduced a package-level `randRead` variable (`var randRead = func(b []byte) (int, error) { return rand.Read(b) }`) so tests can inject a failing reader. Added `ids_fallback_test.go` (package `ids`, white-box) with `TestNewRunID_FallbackOnRandError` that substitutes the injected reader, exercises the fallback return path, and restores the original via `t.Cleanup`. Coverage is now 100%. | ✓ tests pass |
| 2 | Low | The HTTP-error log path in `execCmdOut` (the `execErr != nil` branch) was not tested for the new correlation fields. | Added `TestExecCmd_LogHTTPError_RunIDPresent` to `cmd/curlew/main_test.go`. The test binds a listener to get a free port, immediately closes it so the port refuses connections, invokes `exec --log` pointing at that address, and asserts the written JSONL entry carries a valid `run_id` (32-char hex) and `request_id == "req-1"`. Also added `"net"` import for `net.Listen`. `errcheck` lint finding for the unchecked `ln.Close()` return was also addressed. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/output/ids` | 100% |
| Coverage `internal/output` | 92.5% |
| Coverage `internal/output/events` | 97.8% |
| Coverage `cmd/curlew` | 82.3% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 683e802 | fix(ids): inject randRead for testability, exercise fallback path | #1 |
| e8eb9c7 | test(exec): add TestExecCmd_LogHTTPError_RunIDPresent for error-path log | #2 |

## Summary

2/2 findings resolved. 0 deferred.
