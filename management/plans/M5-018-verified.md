# Verification Report: M5-018

**Task:** go-cli: plugin hook registry (request/response/result lifecycle)
**Verified by:** AI
**Date:** 2026-04-21
**Branch:** feature/M5-018-plugin-hook-registry
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass (38+ packages) |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Plugin hooks section: on_request/on_response/on_result all pass |
| Coverage | 86.5% | Meets >= 80% threshold |

## Observable Output

```
Collection: hooklog smoke test
[plugin:hooklog] on_request GET https://httpbin.org/get
[plugin:hooklog] on_response 200 740ms
[plugin:hooklog] on_result pass_count=1 fail_count=0
  ✓ ping  200  740ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (741ms)
```

Expected: `[plugin:hooklog] on_request GET https://httpbin.org/get`, `[plugin:hooklog] on_response 200 <Xms>`, `[plugin:hooklog] on_result pass_count=1 fail_count=0`
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | on_request called before send, return mutates request | `TestDispatcher_OnRequest_SinglePluginMutatesHeaders`, `TestRun_WithHooksDispatcher_MutationAppliesToExec`, `TestRun_HookPlugin_OnRequestReceivesRequest` | PASS |
| 2 | Modified request used for execution | `TestRun_HookPlugin_TwoPluginsChainInOrder`, `TestRun_WithHooksDispatcher_MutationAppliesToExec` | PASS |
| 3 | on_response called with status/headers/body/duration_ms, can annotate | `TestDispatcher_OnResponse_AnnotationsAccumulate`, `TestDispatcher_OnResponse_StatusCodeNotReplaced`, `TestRun_HookPlugin_OnResponseReceivesResponse` | PASS |
| 4 | on_result called with counts and per-test rows | `TestDispatcher_OnResult_PassCountsForwarded`, `TestDispatcher_OnResult_PerTestRowsForwarded`, `TestRun_WithHooksDispatcher_OnResultFiresOnceWithSummary`, `TestRun_HookPlugin_OnResultReceivesSummary` | PASS |
| 5 | Plugin hook error aborts request as error (not fail) | `TestDispatcher_OnRequest_PluginRPCErrorAbortsWithErrHookAborted`, `TestRun_WithHooksDispatcher_AbortErrorBecomesRequestErr` | PASS |
| 6 | 10-second timeout terminates plugin, warning printed, run continues | `TestDispatcher_OnRequest_TimeoutDropsPlugin_WarningEmitted`, `TestRun_HookPlugin_TimeoutEmitsWarning` | PASS |
| 7 | Two plugins invoked in declared order, chained | `TestDispatcher_OnRequest_TwoPluginsChainInOrder`, `TestRun_HookPlugin_TwoPluginsChainInOrder` | PASS |
| 8 | --help includes Plugin hooks section with three lifecycle hooks | `TestRun_Help_IncludesPluginHooksSection` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all 38+ packages pass | PASS |
| 2 | Observable output works as specified | hooklog plugin outputs on_request/on_response/on_result lines as expected | PASS |
| 3 | Test coverage >= 80% | 86.5% total; internal/plugin/hooks 82.0%, internal/plugin 81.3% | PASS |
| 4 | No build warnings or lint errors | `go build` clean, `golangci-lint run` returns 0 issues | PASS |
| 5 | Help text for apitest run documents plugin hooks | `./apitest --help` shows "Plugin hooks (Enterprise tier)" section with on_request/on_response/on_result | PASS |
| 6 | Smoke test exercises a run with a hook-logging plugin | `smoke/run.sh` "Plugin hooks (M5-018)" section passes PASS/PASS/PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (management/reviews/M5-018-review-iter3.md verdict: PASS). Spot-checks:
- Error wrapping: `fmt.Errorf("%w: %w", ErrHookAborted, err)` — correct wrapping in hooks.go
- Doc comments: `WithHookTimeout`, `NewDispatcher`, exported types all have doc comments
- Test quality: `TestDispatcher_OnRequest_TwoPluginsChainInOrder` exercises chaining with real assertions on order — clean

## Commits

| Hash | Message |
|------|---------|
| 5723002 | chore(plugin): remove trailing newline in channel_test.go |
| 6b73bd1 | docs(review): add passing review for M5-018 (iteration 3) |
| af4fade | docs(review): add improvement report for M5-018 (iteration 2) |
| 63beaae | fix(plugin): forward plugin subprocess stderr to host writer |
| f44031f | docs(review): add review with findings for M5-018 |
| 1dd3848 | docs(review): add improvement report for M5-018 |
| 217060a | fix(cli): add hook dispatch integration tests and fix cleanup redundancy |
| 9570ba2 | fix(hooks): propagate caller context and fix removeReg memory leak |
| 9de014b | fix(plugin): decouple execSpawner context from process lifetime |
| 38249ed | docs(review): add review with findings for M5-018 |
| 8e8cc86 | chore(task): mark M5-018 as review |
| 3abd663 | docs(plugin): update smoke test, docs, and CHANGELOG for M5-018 |
| c9cbd2e | feat(plugin): add hooklog-plugin fixture and one-request.yaml test collection |
| 50bbb6a | feat(cli): wire plugin hook dispatcher into run command + help text |
| 2bff538 | feat(runner): wire plugin hook dispatcher into request/response/result lifecycle |
| 3fbe89b | feat(plugin): add hooks subpackage with Dispatcher for request/response/result lifecycle |
| dd92a3d | feat(plugin): add Channel type and LoadForRun for persistent hook channels |
| 354fe76 | test(plugin): add failing tests for on_result hook in knownHooks |
| 53d28df | chore(task): mark M5-018 as in_progress |
| 62da42e | chore(task): mark M5-018 as planned |
| ea6d728 | docs(plan): add implementation plan for M5-018 |

## Files Changed

| File | Action |
|------|--------|
| `internal/plugin/channel.go` | added — persistent JSON-RPC duplex channel |
| `internal/plugin/channel_test.go` | added — channel unit tests |
| `internal/plugin/hooks/hooks.go` | added — Dispatcher, Registry, payload types |
| `internal/plugin/hooks/hooks_test.go` | added — dispatcher unit tests (14 tests) |
| `internal/plugin/hooks/export_test.go` | added — test seam for timeout override |
| `internal/plugin/host.go` | modified — LoadForRun, execSpawner with stderr forwarding |
| `internal/plugin/host_test.go` | modified — tests for LoadForRun |
| `internal/plugin/plugin.go` | modified — HookName type, knownHooks set |
| `internal/runner/runner.go` | modified — HooksDispatcher interface, hook wiring |
| `internal/runner/runner_test.go` | modified — dispatcher integration tests |
| `cmd/apitest/main.go` | modified — help text Plugin hooks section |
| `cmd/apitest/plugins.go` | added — buildHookDispatcher helper |
| `cmd/apitest/plugins_test.go` | added — CLI integration tests |
| `smoke/run.sh` | modified — Plugin hooks (M5-018) section |
| `testdata/plugins/hooklog-plugin/main.go` | added — hooklog fixture |
| `testdata/plugins/one-request.yaml` | added — smoke test collection |
| `docs/plugins.md` | modified — hook invocation protocol documented |
| `CHANGELOG.md` | modified — M5-018 entry |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. All 8 behaviors verified, 6/6 DoD items complete, coverage 86.5%, CI passes, observable output matches specification exactly.
