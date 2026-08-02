# Code Review: M5-017 (iteration 2)

**Task:** go-cli: plugin interface + loader (external-process model)
**Reviewer:** AI
**Date:** 2026-04-21
**Branch:** feature/M5-017-plugin-interface

## Verdict: PASS

## Findings

No findings.

## Previous Findings — All Resolved

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Medium | `TestPluginsList_HappyPath` called `run()` twice; second call was uncaptured | Fixed: one `run()` call inside `capturePluginsOutput`; `var code int` declared outside closure and captured inside |
| 2 | Medium | Behavior 2 (handshake timeout) had no CLI-level test | Fixed: `TestPluginsList_TimeoutWarning` added with `stall: true` spawner and `pluginsCtxFactory` seam (10ms timeout); asserts exit 0 and `warning:` on stderr |
| 3 | Medium | Behavior 3 (directory) had no CLI-level test | Fixed: `TestPluginsList_Directory` added; asserts both `aplug` and `zplug` appear in table output with exit 0 |
| 4 | Low | Spec deviation (non-executable files in directories silently skipped) not explicitly cross-referenced | Fixed: multi-line comment added in `discover.go` at the skip site explicitly cross-referencing behavior 6 and stating the UX rationale |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `fmt.Errorf("context: %w", err)`; sentinel errors (`ErrHandshakeTimeout`, `ErrDuplicateName`, `ErrNotExecutable`, `ErrHandshakeProtocol`) used; no swallowed errors; `errors.Is` used throughout |
| Input Validation | PASS | `nil` stderr guarded in `Host.Load`; empty env returns nil immediately; all failure modes (missing file, not-executable, malformed JSON, EOF, timeout, JSON-RPC error response) return errors not panics |
| Naming | PASS | No stuttering; all exported symbols have doc comments; `spawner` interface uses `-er` suffix; `plugin.Plugin` and `plugin.Host` are clean; package name `plugin` is lowercase single-word |
| Code Organization | PASS | `internal/plugin` boundary respected; single responsibility per file (jsonrpc, handshake, discover, host, signal); `defer` used for all cleanup; no circular dependencies |
| Correctness | PASS | Context propagated correctly; buffered channel (`make(chan readResult, 1)`) prevents goroutine leak on timeout; `kill()` in fakeSpawner waits on `done` ensuring goroutine cleanup; 1 MiB buffer prevents large-response truncation issues; race detector passes (`go test -race`) |
| Test Quality | PASS | All 8 behaviors covered; all previous gap tests added; `pluginsListCmd` coverage 91.3%; `internal/plugin` coverage 87.9%; table-driven tests with `t.Run`; one integration test builds real fixture binary |

## Test Coverage

- `internal/plugin`: **87.9%** (exceeds 80% threshold)
- `cmd/curlew` overall: **81.6%** (exceeds 80% threshold)
- `cmd/curlew/plugins.go` breakdown:
  - `pluginsCmd`: 100%
  - `pluginsListCmd`: 91.3%
  - `renderPluginsTable`: 91.7%
  - `printPluginsHelp`: 100%

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| B1: single plugin handshake + table output | `TestPluginsList_HappyPath`, `TestHost_Load/single_plugin_happy_path`, `TestHost_Load_WithRealFixture` | PASS |
| B2: handshake timeout → non-fatal warning, exit 0 | `TestPluginsList_TimeoutWarning`, `TestHost_Load_HandshakeTimeout` | PASS |
| B3: CURLEW_PLUGINS is a directory → all executables loaded | `TestPluginsList_Directory`, `TestDiscover/directory_expands_to_sorted_executables` | PASS |
| B4: unknown hook → warning + continue | `TestPluginsList_UnknownHookWarning`, `TestHost_Load/unknown_hook_ignored_with_warning` | PASS |
| B5: duplicate name → error + exit 2 | `TestPluginsList_DuplicateNameExits2`, `TestHost_Load/duplicate_plugin_name_is_fatal` | PASS |
| B6: not executable → error + exit 2 | `TestPluginsList_MissingPathExits2`, `TestDiscover/non-executable_file_produces_fatal_error` | PASS |
| B7: `--help` documents CURLEW_PLUGINS | `TestPluginsList_Help`, `TestPluginsList_HelpArg` | PASS |
| B8: docs/plugins.md documents handshake schema | `docs/plugins.md` reviewed; wire format, env var, exit codes, Go example, security note all present | PASS |

## Observable Verification

```
NAME          VERSION  HOOKS
hello-plugin  0.1.0    on_request,on_response
Exit: 0
```
Column widths auto-pad to longest value with two-space minimum gap — matches the plan spec exactly.

## Summary

All four findings from the first review were correctly resolved. The duplicate `run()` call is gone, CLI-level timeout and directory tests are in place, `pluginsListCmd` coverage improved from 69.6% to 91.3%, and the spec-deviation comment is explicit. The core implementation remains solid: JSON-RPC codec, handshake validation, discovery, and `Host.Load` orchestration are all well-structured. Documentation and smoke test are complete. Zero findings in this iteration.
