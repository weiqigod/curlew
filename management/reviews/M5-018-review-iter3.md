# Code Review: M5-018 (Iteration 3)

**Task:** go-cli: plugin hook registry (request/response/result lifecycle)
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M5-018-plugin-hook-registry

## Verdict: PASS

## Findings

No findings. All previous findings from iterations 1 and 2 have been resolved.

Previous findings addressed:
- **Finding #1 (Critical, iteration 2):** `execSpawner.Spawn` now correctly sets `cmd.Stderr = s.stderr`, forwarding plugin subprocess stderr to the parent process. Smoke test now correctly captures the hooklog output lines.
- **Finding #2 (Medium, iteration 2):** `SetHookTimeoutForTest` removed from production code. `hookTimeout` is now a `Dispatcher` field set via `WithHookTimeout` option. `export_test.go` provides `SetHookTimeoutForTesting` for test use. `plugins_test.go` uses the `hookTimeoutOverride` seam instead.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors defined (`ErrCallTimeout`, `ErrCallPluginError`, `ErrChannelClosed`, `ErrHookAborted`); no swallowed errors; no panics on expected failures |
| Input Validation | PASS | Nil/empty inputs handled; malformed hook responses handled gracefully; closed-channel EOF handled; unknown hooks in hello filtered with warning |
| Naming | PASS | No stuttering; doc comments on all exported symbols; package names are clean; interface names correct |
| Code Organization | PASS | `internal/` package boundaries respected; `hooks` subpackage correctly imports `plugin` but not vice versa; no circular deps; test helpers confined to `export_test.go` and `*_test.go` files |
| Correctness | PASS | `cmd.Stderr = s.stderr` forwards plugin subprocess stderr; goroutines drain properly via buffered channels (capacity 1); `Channel.mu` serialises concurrent calls; `sync.Once` ensures idempotent kill; `dropPlugin` removes timed-out plugins from all hook lists globally; smoke test passes |
| Test Quality | PASS | All 8 task behaviors covered by tests; table-driven tests used; integration tests exercise real binary via `captureRun`; error paths and timeouts covered |

## Test Coverage

- `internal/plugin`: 81.4%
- `internal/plugin/hooks`: 82.1%
- `internal/runner`: 85.6%
- `cmd/apitest`: 81.6%

All packages at or above the 80% threshold.

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| on_request called before send, return mutates request | `TestDispatcher_OnRequest_SinglePluginMutatesHeaders`, `TestRun_WithHooksDispatcher_MutationAppliesToExec`, `TestRun_HookPlugin_OnRequestReceivesRequest` |
| Modified request used for execution | `TestRun_HookPlugin_TwoPluginsChainInOrder`, `TestRun_WithHooksDispatcher_MutationAppliesToExec` |
| on_response called with status/headers/body/duration_ms, can annotate | `TestDispatcher_OnResponse_AnnotationsAccumulate`, `TestDispatcher_OnResponse_StatusCodeNotReplaced`, `TestRun_HookPlugin_OnResponseReceivesResponse` |
| on_result called with counts and per-test rows | `TestDispatcher_OnResult_PassCountsForwarded`, `TestDispatcher_OnResult_PerTestRowsForwarded`, `TestRun_WithHooksDispatcher_OnResultFiresOnceWithSummary`, `TestRun_HookPlugin_OnResultReceivesSummary` |
| Plugin hook error aborts request as error (not fail) | `TestDispatcher_OnRequest_PluginRPCErrorAbortsWithErrHookAborted`, `TestRun_WithHooksDispatcher_AbortErrorBecomesRequestErr` |
| 10-second timeout terminates plugin, warning printed, run continues | `TestDispatcher_OnRequest_TimeoutDropsPlugin_WarningEmitted`, `TestRun_HookPlugin_TimeoutEmitsWarning` |
| Two plugins invoked in declared order, chained | `TestDispatcher_OnRequest_TwoPluginsChainInOrder`, `TestRun_HookPlugin_TwoPluginsChainInOrder` |
| --help includes Plugin hooks section with three hooks | `TestRun_Help_IncludesPluginHooksSection` |

## Summary

All findings from iterations 1 and 2 are resolved. The architecture is clean: `internal/plugin` handles discovery and handshake, `internal/plugin/hooks` owns the dispatch logic, `internal/runner` wires hooks around each HTTP exec call, and the CLI wiring in `cmd/apitest` is minimal and testable. Error handling, input validation, naming, code organisation, correctness, and test quality all meet project standards. The smoke test passes end-to-end with the hooklog fixture.
