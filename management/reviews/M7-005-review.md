# Code Review: M7-005

**Task:** Thread stdout and stderr writers through runCmdInner (remove os.Stdout swap)
**Reviewer:** AI
**Date:** 2026-04-22
**Branch:** feature/M7-005-thread-writers-runcmdinner
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors used appropriately; no swallowed errors in changed code. The `_ = output.WriteXxx(stdout, ...)` discards are a pre-existing codebase convention for best-effort display writes and are not new regressions. |
| Input Validation | PASS | `nil` writer arguments would cause a panic in `fmt.Fprintf`, but callers always provide non-nil writers; consistent with existing codebase convention. |
| Naming | PASS | No stuttering; all exported symbols have doc comments; `newStderrPrinterTo` follows the `*To` writer-injectable helper convention. `runWithWriters` doc comment corrected in iteration 2 — now accurately states all subcommands thread writers through `*CmdOut` variants. |
| Code Organization | PASS | `internal/` boundaries respected; `captureJSONCollection` reduced from ~25 lines (with fd-swap) to ~10 lines (direct buffer injection); `runCmdInner`/`runCmdWithWriters`/`runCmd` responsibilities are cleanly separated. |
| Correctness | PASS | `bytes.Buffer` instances in `TestConcurrentDiscovery` are per-goroutine (not shared); `go test -race` passes. `TestNoOsStdoutAssignment` AST gate correctly targets `*ast.AssignStmt` LHS — function-argument uses of `os.Stderr` in `plugins_test.go` are not assignments and correctly pass the gate. |
| Test Quality | PASS | All 7 behaviors covered by at least one test; `TestConcurrentDiscovery` and `TestNoOsStdoutAssignment` are anti-regression gates; table-driven tests used where appropriate; `t.Run` subtests used; error paths tested. |

## Test Coverage

- Overall coverage: **86.2%** (above 80% requirement)
- `cmd/apitest` package: **80.3%**
- `internal/watch` package: **89.7%**
- Missing coverage: acceptable — no coverage regressions; key paths exercised by new and existing tests.

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | runCmdInner stdout/stderr injection | `TestRunCmdInner_RoutesOutputToInjectedWriters`, `TestRunCmdInner_StdoutUntouched_OnParseError` |
| 2 | Concurrent goroutines no interleaving | `TestConcurrentDiscovery` |
| 3 | captureJSONCollection no os.Stdout assign | `TestCaptureJSONCollection_DoesNotTouchOsStdout`, `TestNoOsStdoutAssignment` |
| 4 | watchCmd passes Config.Stdout/Stderr | `TestRun_PassesConfigWritersToRunFunc` |
| 5 | runCmd passes os.Stdout/os.Stderr | `TestRunCmd_*` suite (calls `runCmd` which routes to `os.Stdout`/`os.Stderr`) |
| 6 | Tests use writer injection | `captureRunCmd` uses `bytes.Buffer`; `capturePluginsOutput` uses `io.Writer` closures; run_test.go workers tests use `runCmdWithWriters` |
| 7 | TestNoOsStdoutAssignment AST scan | `TestNoOsStdoutAssignment` |

## Observable Verification

```
grep -rn "os.Stdout = " cmd/apitest/  → zero matches ✓
grep -rn "os.Stderr = " cmd/apitest/  → zero matches ✓
grep -n "^func runCmdInner" cmd/apitest/main.go → func runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary) ✓
go test -run TestConcurrentDiscovery ./cmd/apitest/...  → PASS ✓
go test -run TestNoOsStdoutAssignment ./cmd/apitest/... → PASS ✓
go test -race ./cmd/apitest/...                         → PASS ✓
./scripts/ci-local.sh --go                              → PASS ✓
Coverage                                                → 86.2% ✓
```

## Summary

All iteration-1 findings resolved. The implementation is architecturally complete: `runCmdInner` accepts explicit `stdout, stderr io.Writer` parameters; the `os.Stdout` fd-swap in `captureJSONCollection` is eliminated; all 14 subcommands have writer-injectable `*CmdOut` variants; `watch.Config.RunFunc` carries writers through its signature; and two anti-regression gates (`TestConcurrentDiscovery`, `TestNoOsStdoutAssignment`) enforce the new invariants. Overall coverage is 86.2%. The codebase is ready for `/verify`.
