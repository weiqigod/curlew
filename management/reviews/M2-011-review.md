# Code Review: M2-011

**Task:** Watch mode with file system monitoring
**Reviewer:** AI
**Date:** 2026-04-01
**Branch:** feature/M2-011-watch-mode
**Round:** 4

## Verdict: PASS

## Findings

No findings. All issues from rounds 1–3 have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All `fmt.Errorf` calls use `%w` for wrapping. Stderr user-facing messages use `%v` correctly. Context describes WHERE (e.g., "resolving collection path", "parsing collection"). |
| Input Validation | PASS | External inputs validated via `parser.ParseFile` and `config` package. CLI args validated by `parseRunArgs`. Missing file and missing env produce clear errors. |
| Naming | PASS | `Paths` avoids stutter (`watch.Paths`). All exported types/functions have doc comments. `Config.RunFunc` has inline comment. Package name `watch` is lowercase single-word. |
| Code Organization | PASS | Clean `internal/watch` boundary. Narrow exported surface (`Paths`, `Config`, `Run`, `CollectPaths`). `defer` for watcher close, debouncer stop, and context cancel. No circular dependencies. |
| Correctness | PASS | No race conditions (verified with `-race`). Context propagated via `ctx.Done()` in select loop. No goroutine leaks — debouncer uses `time.AfterFunc` with buffered channel. `syncWatchDirs` properly adds new and removes stale directories. `isWriteEvent` covers Write, Create, and Rename for editor compatibility. |
| Test Quality | PASS | All 8 behaviors covered. Error paths tested (missing file, missing env, RunFunc failure). Assertions are specific (counts, basenames, absolute paths, individual arg elements). Table-driven tests throughout. |
| Dependencies | PASS | `fsnotify` correctly listed as direct dependency in go.mod. `golang.org/x/sys` correctly marked indirect. |

## Test Coverage
- Coverage: 86.0% (watch package)
- `Paths.All`: 100%, `Paths.Dirs`: 100%, `CollectPaths`: 90%
- `Run`: ~71% — uncovered branches are rare error paths (filepath.Abs failure in event handler, watcher channel close)
- All packages pass with race detector
- Lint: 0 issues

## Behavior Coverage

| # | Behavior | Test |
|---|----------|------|
| 1 | Collection file modified -> re-runs | `TestRun/rerun_on_collection_file_change` |
| 2 | External request file modified -> re-runs | `TestRun/rerun_on_external_file_change` |
| 3 | Environment file modified -> re-runs | `TestRun/rerun_on_env_file_change` |
| 4 | .env file modified -> re-runs | `TestRun/rerun_on_dotenv_change` |
| 5 | Non-related file -> no re-run | `TestRun/ignores_unrelated_file_changes` |
| 6 | Ctrl+C -> clean exit code 0 | `TestRun/clean_shutdown_on_context_cancel` |
| 7 | Env flags preserved across re-runs | `TestRun/preserves_args_across_reruns` |
| 8 | Debounce rapid saves -> one re-run | `TestRun/debounces_rapid_saves` |

## Previous Review Findings (Rounds 1–3)

All findings from prior rounds resolved:
- Round 1 (6 findings): naming stutter, missing behavior tests, silent error discard, reimplemented `strings.Contains`
- Round 2 (2 findings): fsnotify indirect dependency, stale directory leak in watcher
- Round 3 (3 findings): missing RunFunc field comment, missing env-not-found test case, weak arg assertion

## Summary

Clean implementation with well-structured code across three files (`paths.go`, `watch.go`, `main.go`). The debouncer design is sound — channel-based with no shared mutable state. Path collection via `CollectPaths` is refreshed after each re-run, ensuring new external files are detected. The `syncWatchDirs` function properly adds and removes directories. All 8 specified behaviors are tested. Coverage exceeds 80%. No issues found.
