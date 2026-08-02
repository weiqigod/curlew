# Verification Report: M7-005

**Task:** Thread stdout and stderr writers through runCmdInner (remove os.Stdout swap)
**Verified by:** AI
**Date:** 2026-04-22
**Branch:** feature/M7-005-thread-writers-runcmdinner
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected (via ci-local.sh) |
| `golangci-lint run` | PASS | No findings (via ci-local.sh) |
| `./smoke/run.sh` | PASS | All smoke checks pass |
| Coverage | 86.2% | Meets >= 80% threshold |

## Observable Output

```
grep -rn "os.Stdout = " cmd/apitest/   → ZERO MATCHES
grep -n "^func runCmdInner" cmd/apitest/main.go → func runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary)
go test -run TestConcurrentDiscovery ./cmd/apitest/...  → PASS
go test -run TestNoOsStdoutAssignment ./cmd/apitest/... → PASS
```

Expected: zero `os.Stdout = ` matches; `runCmdInner` with explicit writer parameters; both anti-regression gates pass.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | runCmdInner with bytes.Buffer stdout/stderr routes all output to provided writers | `TestRunCmdInner_RoutesOutputToInjectedWriters`, `TestRunCmdInner_StdoutUntouched_OnParseError` | PASS |
| 2 | Concurrent goroutines with separate writer buffers produce no interleaving | `TestConcurrentDiscovery` | PASS |
| 3 | captureJSONCollection does NOT assign to os.Stdout | `TestCaptureJSONCollection_DoesNotTouchOsStdout`, `TestNoOsStdoutAssignment` | PASS |
| 4 | watchCmd passes watch.Config.Stdout/Stderr through RunFunc | `TestRun_PassesConfigWritersToRunFunc` | PASS |
| 5 | runCmd passes os.Stdout/os.Stderr as writers (backward-compatible CLI) | `TestRunCmd_*` suite | PASS |
| 6 | Existing tests use writer injection (bytes.Buffer) instead of os.Stdout swap | `captureRunCmd`, `capturePluginsOutput`, run_test.go workers tests | PASS |
| 7 | TestNoOsStdoutAssignment AST scan blocks any future reassignment of os.Stdout/os.Stderr | `TestNoOsStdoutAssignment` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | go test ./... — all PASS | PASS |
| 2 | runCmdInner signature is (args []string, stdout, stderr io.Writer) (int, *runner.Summary) | grep confirmed exact match at main.go:468 | PASS |
| 3 | grep -rn 'os.Stdout = ' cmd/apitest/ returns zero matches | grep output: ZERO MATCHES | PASS |
| 4 | grep -rn 'os.Stderr = ' cmd/apitest/ returns zero matches | grep output: ZERO MATCHES | PASS |
| 5 | TestConcurrentDiscovery verifies no interleaving | go test -run TestConcurrentDiscovery: PASS | PASS |
| 6 | TestNoOsStdoutAssignment uses go/ast to reject future reassignment | go test -run TestNoOsStdoutAssignment: PASS | PASS |
| 7 | All tests that hijacked os.Stdout migrated to writer injection | grep -rn "os.Stdout = " cmd/apitest/ → zero results | PASS |
| 8 | captureJSONCollection simplified to direct buffer injection | function uses bytes.Buffer + runCmdInner, no fd-swap | PASS |
| 9 | go test ./... passes with no regressions | All packages PASS | PASS |
| 10 | Test coverage does not regress below 80% | 86.2% total | PASS |
| 11 | golangci-lint run passes with 0 issues | ci-local.sh lint gate PASS | PASS |
| 12 | ./smoke/run.sh passes | ci-local.sh smoke gate PASS | PASS |
| 13 | ./scripts/ci-local.sh passes | ci-local PASS | PASS |
| 14 | CHANGELOG.md updated | Entry added under [Unreleased] | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 2, verdict PASS), spot-check clean:
- Error wrapping: `fmt.Errorf("...: %w", err)` pattern confirmed in main.go
- Doc comments: `runCmdInner`, `runCmdWithWriters`, `runWithWriters`, `captureJSONCollection` all documented
- `TestConcurrentDiscovery` per-goroutine buffers are non-shared; race detector passes

## Commits

| Hash | Message |
|------|---------|
| a886bc3 | docs(review): add passing review for M7-005 |
| 334bc1d | docs(review): add improvement report for M7-005 |
| 367c435 | fix(cmd): correct stale doc comment on runWithWriters |
| 2e0122f | docs(review): add review with findings for M7-005 |
| 5b8ea6a | chore(task): mark M7-005 as review |
| ec68931 | feat(cli): thread writers through runCmdInner and all subcommands (M7-005) |
| e5bad44 | fix(cli): remove os.Stdout fd-swap from captureJSONCollection |
| 7a0f989 | feat(cli): add writer-injectable variants for license and worker subcommands |
| 83804a9 | feat(cli): change RunFunc signature to accept io.Writer parameters |
| a3996ea | feat(cli): add perfCmdOut and pluginsCmdOut writer-injectable variants |
| c31df19 | chore(task): mark M7-005 as in_progress |
| 004bba2 | chore(task): mark M7-005 as planned |
| 865b131 | docs(plan): add implementation plan for M7-005 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/main.go` | modified — runCmdInner/runCmdWithWriters/runWithWriters writer injection |
| `cmd/apitest/discovery_run.go` | modified — captureJSONCollection fd-swap removed |
| `cmd/apitest/main_test.go` | modified — captureRunCmd/captureRun migrated; TestConcurrentDiscovery, TestNoOsStdoutAssignment added |
| `cmd/apitest/discovery_run_test.go` | modified — TestCaptureJSONCollection_DoesNotTouchOsStdout added |
| `cmd/apitest/run_test.go` | modified — os.Stdout swaps migrated |
| `cmd/apitest/perf.go` | modified — perfCmdOut writer-injectable variant |
| `cmd/apitest/perf_test.go` | modified — all os.Stdout/os.Stderr swaps migrated |
| `cmd/apitest/plugins.go` | modified — pluginsCmdOut writer-injectable variant |
| `cmd/apitest/plugins_test.go` | modified — capturePluginsOutput rewritten with writer injection |
| `cmd/apitest/license.go` | modified — licenseCmdOut writer-injectable variant |
| `cmd/apitest/worker.go` | modified — workerCmdOut writer-injectable variant |
| `internal/watch/watch.go` | modified — RunFunc type extended to carry io.Writer params |
| `internal/watch/watch_test.go` | modified — RunFunc closures updated; TestRun_PassesConfigWritersToRunFunc added |
| `CHANGELOG.md` | modified — entry added under Unreleased |
| `management/backlog.yaml` | modified — task status |
| `management/tasks/M7-005.yaml` | modified — task YAML |
| `management/plans/M7-005-plan.md` | added |
| `management/reviews/M7-005-review.md` | added |
| `management/plans/M7-005-improved.md` | added |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
