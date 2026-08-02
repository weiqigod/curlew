# Improvement Report: M5-017

**Task:** go-cli: plugin interface + loader (external-process model)
**Date:** 2026-04-21
**Review:** management/reviews/M5-017-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestPluginsList_HappyPath` called `run()` twice — second call uncaptured, exit-code check read `stderr` from the first run | Removed the second `run()` call; declared `var code int` outside the closure and captured it inside `capturePluginsOutput`, matching the pattern used by all other test functions | ✓ tests pass |
| 2 | Medium | Behavior 2 (handshake timeout) had no CLI-level test; `testPluginDef.stall` was declared but unused | Added `TestPluginsList_TimeoutWarning` using a `stall: true` spawner; added `pluginsCtxFactory` test seam to `plugins.go` so tests can inject a 10ms context without touching the internal 5s constant; asserts exit 0 and warning on stderr | ✓ tests pass |
| 3 | Medium | Behavior 3 (directory) had no CLI-level test; table rendering for multiple plugins from a dir was untested at CLI level | Added `TestPluginsList_Directory` with two fake plugins in a temp dir; asserts both appear in table output with exit 0 | ✓ tests pass |
| 4 | Low | Spec deviation: non-executable files inside a directory silently skipped (task YAML behavior 6 is unambiguous); deviation not explicitly cross-referenced | Added explicit multi-line comment in `discover.go` at the skip site that cross-references behavior 6, states the rationale (shared directories commonly contain non-executable files), and points to the test that covers the in-scope case (explicitly-named non-executable file) | ✓ verified |

## Additional Improvements (not in review, raised coverage)

Two extra tests added to bring `pluginsListCmd` from 69.6% to 91.3%:
- `TestPluginsList_HelpArg` — exercises `plugins list --help` arg path
- `TestPluginsList_UnknownFlag` — exercises `plugins list --unknown-flag` path (exits 2)

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| `internal/plugin` coverage | 87.9% |
| `cmd/curlew` overall coverage | 81.3% |
| `pluginsListCmd` coverage | 91.3% (was 69.6%) |
| Total coverage | 86.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 50781b4 | fix(plugin): resolve all review findings for M5-017 | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred. `pluginsListCmd` coverage improved from 69.6% to 91.3% (well above 80% threshold). All quality gates pass.
