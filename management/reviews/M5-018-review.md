# Code Review: M5-018

**Task:** go-cli: plugin hook registry (request/response/result lifecycle)
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M5-018-plugin-hook-registry

## Verdict: FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Critical | Correctness | `internal/plugin/host.go` | 31–58 | `execSpawner.Spawn` sets up `cmd.StdinPipe()` and `cmd.StdoutPipe()` but never sets `cmd.Stderr`. Per Go `os/exec` semantics, when `cmd.Stderr` is nil the subprocess's stderr is discarded (connected to the null device). The hooklog fixture writes `[plugin:hooklog] on_request ...` to `os.Stderr`, but those bytes are silently dropped. The smoke test therefore finds no `on_request` line and fails. The first-iteration review appeared to pass because the old bug (process killed on context cancel) caused the parent `apitest` to emit `warning: plugin hooklog error on on_request: plugin channel closed...`, and `grep -q "on_request"` matched that warning string — a false positive. After fixing finding #1 there are no more such warnings, so the false positive disappears and the real failure surfaces. | Set `cmd.Stderr = h.stderr` in `execSpawner.Spawn` so the plugin subprocess's stderr is forwarded to the host's stderr writer (which is `os.Stderr` in production). This makes hook notifications visible in the terminal and in the smoke-test capture (`2>&1`). |
| 2 | Medium | Code Organization | `internal/plugin/hooks/hooks.go` | 22–36 | `SetHookTimeoutForTest` is exported from non-test production code. Test-helper functions that mutate global state should not be part of the production binary's exported API surface. The idiomatic Go approach is the `export_test.go` pattern, which is already used by `hooks_test.go` via `export_test.go:SetHookTimeoutForTesting`. The external-package test `cmd/apitest/plugins_test.go` (package `main`) should use `hooks.SetHookTimeoutForTest`, but the function it calls is the production export; this conflates test and production concerns. | Move `SetHookTimeoutForTest` out of `hooks.go` into an `export_test.go` inside `internal/plugin/hooks/` (the existing file already has `SetHookTimeoutForTesting` — unify to one function). For `cmd/apitest/plugins_test.go`, replace the direct call to `hooks.SetHookTimeoutForTest` with a local helper that uses the exported test-only function, or access it via the `export_test.go` bridge. |

## Gate Output

```
=== Plugin hooks (M5-018) ===
--- Building hooklog-plugin fixture ---
Build: OK
--- Run with hooklog plugin ---
FAIL: no on_request line — Collection: Hooklog smoke
  ✓ ping  200  623ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (624ms)
```

**Root cause of finding #1:** `execSpawner.Spawn` does not forward the subprocess's stderr. The hooklog plugin writes `[plugin:hooklog] on_request GET ...` to its `os.Stderr`, but because `cmd.Stderr` is nil in `exec.Command`, those bytes are discarded by Go's runtime. The smoke test captures stdout+stderr of the parent `apitest` process (`2>&1`), but the plugin's stderr never reaches the parent. In iteration 1 of this review the smoke test falsely passed because the parent's own error warning contained "on_request" as a substring.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Errors wrapped with `%w`; sentinels defined for all known failure modes; no swallowed errors |
| Input Validation | PASS | Nil/empty inputs handled; malformed hook responses handled gracefully; channel-closed EOS handled |
| Naming | PASS | No stuttering; doc comments on all exported symbols; package names are clean |
| Code Organization | FAIL | `SetHookTimeoutForTest` is a test helper exported from production code (finding #2) |
| Correctness | FAIL | Plugin subprocess stderr discarded — smoke test fails (finding #1) |
| Test Quality | PASS | All five planned CLI-level integration tests implemented; table-driven tests used; all task behaviors covered |

## Test Coverage

- `internal/plugin`: 81.3%
- `internal/plugin/hooks`: 82.0%
- `internal/runner`: 85.6%
- `cmd/apitest`: 81.6%

All packages exceed the 80% threshold. The unit tests are well-structured. The gap is the end-to-end real-subprocess path: unit tests use in-process spawners so they do not expose the `cmd.Stderr` omission.

## Summary

The architecture is sound and all unit tests pass. Two issues remain from the improve phase: the critical omission of `cmd.Stderr` forwarding in `execSpawner.Spawn` (which causes the smoke test to fail because plugin stderr output is discarded), and a medium-severity concern that `SetHookTimeoutForTest` is exported from production code rather than confined to a `_test.go` file. Both must be fixed before this task can be verified.
