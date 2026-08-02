# Verification Report: M2-011

**Task:** Watch mode with file system monitoring
**Verified by:** AI
**Date:** 2026-04-01
**Branch:** feature/M2-011-watch-mode
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 16 packages, all cached/pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean, including watch scenario |
| Coverage | 91.1% total, 86.0% watch | Meets >= 80% threshold |

## Observable Output

```
$ curlew watch col.yaml
Collection: Watch Test
  ✓ Ping  200  708ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (708ms)
--- Re-running (changed: col.yaml) at 08:57:39 ---
Collection: Watch Test
  ✓ Ping  200  139ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (139ms)
```

Expected: Collection re-runs on file change, clean exit on Ctrl+C
Result: MATCH (verified via smoke test)

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Collection file modified -> re-runs | `TestRun/rerun_on_collection_file_change` | PASS |
| 2 | External request file modified -> re-runs | `TestRun/rerun_on_external_file_change` | PASS |
| 3 | Environment file modified -> re-runs | `TestRun/rerun_on_env_file_change` | PASS |
| 4 | .env file modified -> re-runs | `TestRun/rerun_on_dotenv_change` | PASS |
| 5 | Non-related file -> no re-run | `TestRun/ignores_unrelated_file_changes` | PASS |
| 6 | Ctrl+C -> clean exit code 0 | `TestRun/clean_shutdown_on_context_cancel` | PASS |
| 7 | Env flags preserved across re-runs | `TestRun/preserves_args_across_reruns` | PASS |
| 8 | Debounce rapid saves -> one re-run | `TestRun/debounces_rapid_saves` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | 8/8 tests pass | PASS |
| 2 | Observable output works | Smoke test verified | PASS |
| 3 | Test coverage >= 80% | 86.0% (watch), 91.1% (total) | PASS |
| 4 | No build warnings or lint errors | `golangci-lint run`: 0 issues | PASS |
| 5 | Help text updated | `watch <file>` in help output | PASS |
| 6 | Smoke test updated | Watch scenario added to `smoke/run.sh` | PASS |

## Code Review

Review PASS trusted (round 4), spot-check clean.

| Check | Status |
|-------|--------|
| Error handling (`%w` wrapping) | PASS |
| Naming conventions (no stutter) | PASS |
| Doc comments on exports | PASS |
| Code organization (`internal/watch`) | PASS |
| Test quality (specific assertions) | PASS |
| Race detector | PASS |

## Commits

| Hash | Message |
|------|---------|
| 79f8385 | docs(plan): add implementation plan for M2-011 |
| 96fea3c | chore(task): mark M2-011 as planned |
| 31f75ae | chore(task): mark M2-011 as in_progress |
| 3c9d567 | test(parser): add failing tests for ExternalFiles population |
| 04ec025 | feat(parser): add ExternalFiles field to Collection |
| 9009c89 | chore: add fsnotify dependency for watch mode |
| 139468f | test(watch): add failing tests for WatchPaths and CollectPaths |
| 1db26e7 | feat(watch): implement WatchPaths and CollectPaths |
| b856fa1 | refactor(watch): fix lint issues in watch paths |
| 6e9a5ee | test(watch): add failing tests for debouncer and Run |
| c84d7a2 | feat(watch): implement debouncer and watch Run loop |
| 2200830 | refactor(watch): check watcher.Close error return |
| fe6964d | test(cli): add failing tests for watch command |
| 35eca58 | feat(cli): wire up watch command |
| 700a97a | test(smoke): add watch mode smoke scenario |
| 8981b10 | chore(task): mark M2-011 as review |
| 012021d | docs(review): add review with findings for M2-011 |
| 30163b5 | refactor(watch): rename WatchPaths to Paths to avoid stutter |
| c44dd3f | fix(watch): replace custom substring helpers with strings.Contains |
| 87a844a | fix(watch): log errors when adding directories during refresh |
| a9e6f3a | test(watch): add missing tests for external, env, and dotenv file re-runs |
| fec51b0 | docs(review): add improvement report for M2-011 |
| 902ae3e | docs(review): add review with findings for M2-011 |
| f48bce7 | fix(deps): promote fsnotify to direct dependency in go.mod |
| e76d4a7 | fix(watch): remove stale directories from watcher on path refresh |
| 39b9409 | docs(review): update improvement report for M2-011 (round 2) |
| 009207a | docs(review): add review with findings for M2-011 (round 3) |
| 545d207 | fix(watch): resolve round 3 review findings |
| ec776ff | docs(review): update improvement report for M2-011 (round 3) |
| 7b901c6 | docs(review): add passing review for M2-011 (round 4) |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/main.go` | modified | +35 |
| `cmd/curlew/main_test.go` | modified | +31 |
| `go.mod` | modified | +7/-1 |
| `go.sum` | modified | +4 |
| `internal/parser/collection.go` | modified | +15/-1 |
| `internal/parser/external.go` | modified | +16/-1 |
| `internal/parser/parser.go` | modified | +4/-1 |
| `internal/parser/parser_test.go` | modified | +66 |
| `internal/watch/paths.go` | created | +124 |
| `internal/watch/paths_test.go` | created | +353 |
| `internal/watch/watch.go` | created | +202 |
| `internal/watch/watch_test.go` | created | +525 |
| `smoke/run.sh` | modified | +31 |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
