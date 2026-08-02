# Improvement Report: M5-018 (Iteration 2)

**Task:** go-cli: plugin hook registry (request/response/result lifecycle)
**Date:** 2026-04-21
**Review:** management/reviews/M5-018-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `execSpawner.Spawn` did not set `cmd.Stderr`, so plugin subprocess stderr was silently discarded. The hooklog fixture writes hook log lines to its own stderr; those bytes never reached the parent process's stderr, causing the smoke test to fail. | Changed `execSpawner` from a zero-value struct to a struct holding `stderr io.Writer`. Updated `execSpawner.Spawn` to set `cmd.Stderr = s.stderr`. Updated `NewHost` to pass `stderr` to `execSpawner{stderr: stderr}`. | ✓ tests pass |
| 2 | Medium | `SetHookTimeoutForTest` was exported from production `hooks.go` — a test-helper function in the production binary's API surface. | Removed `SetHookTimeoutForTest` and the package-level `hookTimeout` variable from `hooks.go`. Added `hookTimeout time.Duration` field to `Dispatcher` (defaults to `HookTimeout`). Added `WithHookTimeout(d time.Duration) DispatcherOption` and variadic `opts ...DispatcherOption` to `NewDispatcher` — clean production API. Updated `export_test.go` to `SetHookTimeoutForTesting(d *Dispatcher, timeout time.Duration)` (instance-scoped, test-only). Updated `hooks_test.go` to call after `buildDispatcher`. Added `hookTimeoutOverride time.Duration` test seam in `plugins.go`; `buildHookDispatcher` passes `WithHookTimeout(hookTimeoutOverride)` when non-zero. Updated `plugins_test.go` to set `hookTimeoutOverride` instead of calling the removed production export. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage — `internal/plugin` | 81.4% |
| Coverage — `internal/plugin/hooks` | 82.1% |
| Coverage — `internal/runner` | 85.6% |
| Coverage — `cmd/curlew` | 81.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 63beaae | fix(plugin): forward plugin subprocess stderr to host writer | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
