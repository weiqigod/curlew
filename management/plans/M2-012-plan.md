# Implementation Plan: M2-012

## Overview
Enhance watch mode with terminal UX: run separators with timestamps, running totals across re-runs, `--format json` per-run output, `--clear` flag, "Watching for changes..." status message, and graceful parse error handling.

## Task Details
- **ID:** M2-012
- **Title:** Watch mode terminal UX and incremental feedback
- **Phase:** M2: Watch Mode
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-011 | Watch mode with file system monitoring | done |

## Implementation Steps

### Step 1: Add `RunResult` type and update `Config.RunFunc` signature

**Rationale:** Smallest blast radius — changes a type and its call sites. All subsequent steps depend on structured run results.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Add `RunResult` type, change `RunFunc` signature |
| `internal/watch/watch_test.go` | modify | Update all `RunFunc` lambdas to return `RunResult` |
| `cmd/apitest/main.go` | modify | Add `runCmdResult` wrapper, update `watchCmd` |

#### Current Code

`internal/watch/watch.go` Config:
```go
type Config struct {
	CollectionPath string             // path to collection file
	Args           []string           // full CLI args to pass to RunFunc on each re-run
	EnvName        string             // --env flag value (for path collection)
	Debounce       time.Duration      // debounce interval (default 500ms if zero)
	Stdout         io.Writer          // watch status messages
	Stderr         io.Writer          // error output
	UseColor       bool               // ANSI color codes
	RunFunc        func([]string) int // function called on each run; receives Args, returns exit code
}
```

`cmd/apitest/main.go` watchCmd (line 493):
```go
RunFunc: runCmd,
```

#### New Code

`internal/watch/watch.go`:
```go
// RunResult holds structured feedback from a single test run.
type RunResult struct {
	ExitCode int
	Passed   int
	Failed   int
	Skipped  int
	Total    int
}

type Config struct {
	CollectionPath string
	Args           []string
	EnvName        string
	Debounce       time.Duration
	Stdout         io.Writer
	Stderr         io.Writer
	UseColor       bool
	Format         string // "json" or "" (terminal)
	ClearScreen    bool   // --clear flag
	RunFunc        func([]string) RunResult
}
```

`cmd/apitest/main.go` — new wrapper function:
```go
// runCmdResult wraps runCmd to return structured results for watch mode.
// It re-parses args and invokes runner.Run directly to capture the Summary.
func runCmdResult(args []string) watch.RunResult {
	exitCode := runCmd(args)
	// The exit code already reflects pass/fail. We need the summary for totals.
	// Re-parse and re-run would be wasteful, so we use a shared summary capture.
	return watch.RunResult{ExitCode: exitCode}
}
```

**Better approach** — capture summary via closure in `watchCmd`:

In `watchCmd`, we'll extract the summary from `runCmd` by introducing a small refactor: a `runCmdWithSummary` function that returns `(int, *runner.Summary)`. The existing `runCmd` calls this and discards the summary. The watch wrapper captures it.

```go
// runCmdInner is the shared implementation for runCmd and watch mode.
// Returns exit code and the runner summary (nil if execution never reached runner.Run).
func runCmdInner(args []string) (int, *runner.Summary) {
	// ... existing runCmd body, but returns (exitCode, summary) ...
}

func runCmd(args []string) int {
	code, _ := runCmdInner(args)
	return code
}
```

`watchCmd` wrapper:
```go
RunFunc: func(args []string) watch.RunResult {
	exitCode, summary := runCmdInner(args)
	r := watch.RunResult{ExitCode: exitCode}
	if summary != nil {
		r.Total = summary.Total
		r.Passed = summary.Passed
		r.Failed = summary.Failed
		r.Skipped = summary.Skipped
	}
	return r
},
```

#### Tests to Write FIRST (RED phase)

No new test functions — existing tests updated mechanically:
```go
// All RunFunc lambdas change from:
RunFunc: func(_ []string) int { count.Add(1); return 0 },
// To:
RunFunc: func(_ []string) RunResult { count.Add(1); return RunResult{} },
```

Verify all 11 existing tests still pass with the new signature.

#### Impact on Existing Tests
- All 11 `TestRun` subtests — update `RunFunc` return type (mechanical, no logic change)
- `TestSyncWatchDirs` — no change
- `TestDebouncer` — no change

---

### Step 2: Add "Watching for changes..." message with file list

**Rationale:** Pure additive output, no state tracking required. Foundation for behavior 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Add `printWatchStatus`, call after initial run |
| `internal/watch/watch_test.go` | modify | Add test for watch status message |

#### Current Code

`watch.go` Run function, after initial run (line 33-41):
```go
// Initial run.
cfg.RunFunc(cfg.Args)

// Collect paths to watch.
wp, err := CollectPaths(cfg.CollectionPath, cfg.EnvName)
```

#### New Code

```go
// Initial run.
cfg.RunFunc(cfg.Args)

// Collect paths to watch.
wp, err := CollectPaths(cfg.CollectionPath, cfg.EnvName)
if err != nil {
	_, _ = fmt.Fprintf(cfg.Stderr, "watch: failed to collect paths: %v\n", err)
	return 1
}

// Show watch status (terminal mode only).
if cfg.Format != "json" {
	printWatchStatus(cfg.Stdout, cfg.UseColor, wp)
}
```

New function:
```go
func printWatchStatus(w io.Writer, useColor bool, wp *Paths) {
	msg := "Watching for changes..."
	if useColor {
		msg = "\033[36m" + msg + "\033[0m"
	}
	_, _ = fmt.Fprintln(w, msg)
	for _, f := range wp.All() {
		_, _ = fmt.Fprintf(w, "  %s\n", filepath.Base(f))
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrintWatchStatus(t *testing.T) {
	tests := []struct {
		name     string
		useColor bool
		wantMsg  string
		wantFile string
	}{
		{"basic output", false, "Watching for changes...", "col.yaml"},
		{"with color", true, "\033[36mWatching for changes...\033[0m", "col.yaml"},
	}
	// ...
}

// In TestRun:
t.Run("output shows watching message after initial run", func(t *testing.T) {
	// ... set up, trigger change, cancel ...
	// Assert stdout contains "Watching for changes..."
})

t.Run("watching message lists watched files", func(t *testing.T) {
	// ... verify file names appear in output ...
})
```

#### Impact on Existing Tests
- `TestRun/output includes rerun separator` — stdout now also contains "Watching for changes...", assertion checks `strings.Contains` so still passes
- No other tests affected

---

### Step 3: Add running totals tracking

**Rationale:** Depends on `RunResult` from Step 1. Adds state tracking across runs.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Add `RunningTotals`, track and print after each run |
| `internal/watch/watch_test.go` | modify | Add totals tests |

#### New Code

```go
// RunningTotals accumulates pass/fail counts across multiple watch runs.
type RunningTotals struct {
	Runs    int
	Passed  int
	Failed  int
	Skipped int
	Total   int
}

// Add incorporates a single run's results into the running totals.
func (rt *RunningTotals) Add(r RunResult) {
	rt.Runs++
	rt.Passed += r.Passed
	rt.Failed += r.Failed
	rt.Skipped += r.Skipped
	rt.Total += r.Total
}
```

In `Run()`, declare `var totals RunningTotals` before the loop. After each `RunFunc` call:
```go
result := cfg.RunFunc(cfg.Args)
totals.Add(result)
if cfg.Format != "json" {
	printRunningTotals(cfg.Stdout, cfg.UseColor, &totals)
}
```

```go
func printRunningTotals(w io.Writer, useColor bool, rt *RunningTotals) {
	msg := fmt.Sprintf("Totals (%d runs): %d passed, %d failed, %d skipped",
		rt.Runs, rt.Passed, rt.Failed, rt.Skipped)
	if useColor {
		msg = "\033[90m" + msg + "\033[0m"
	}
	_, _ = fmt.Fprintln(w, msg)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunningTotals(t *testing.T) {
	tests := []struct {
		name    string
		results []RunResult
		want    RunningTotals
	}{
		{"zero value is valid", nil, RunningTotals{}},
		{"single add", []RunResult{{Passed: 3, Failed: 1, Total: 4}},
			RunningTotals{Runs: 1, Passed: 3, Failed: 1, Total: 4}},
		{"multiple adds accumulate", []RunResult{
			{Passed: 2, Failed: 1, Total: 3},
			{Passed: 3, Failed: 0, Total: 3},
		}, RunningTotals{Runs: 2, Passed: 5, Failed: 1, Total: 6}},
	}
	// ...
}

func TestPrintRunningTotals(t *testing.T) {
	tests := []struct {
		name     string
		useColor bool
		totals   RunningTotals
		want     string
	}{
		{"basic output", false, RunningTotals{Runs: 2, Passed: 5, Failed: 1}, "Totals (2 runs): 5 passed, 1 failed"},
		{"with color", true, RunningTotals{Runs: 1, Passed: 3, Failed: 0}, "\033[90m"},
	}
	// ...
}

// In TestRun:
t.Run("running totals accumulate across reruns", func(t *testing.T) {
	// RunFunc returns different counts per run.
	// After 2 runs, verify accumulated totals in stdout.
})

t.Run("running totals include initial run", func(t *testing.T) {
	// Cancel before any file change. Verify stdout shows totals from initial run.
})
```

#### Impact on Existing Tests
- Stdout output now contains totals lines — existing `strings.Contains` assertions unaffected
- Tests with `RunFunc` returning `RunResult{}` (zero value) produce "0 passed, 0 failed" — no issue

---

### Step 4: Add `--format json` support

**Rationale:** Depends on `Format` field from Step 1. Suppresses terminal decorations; lets `runCmd`'s JSON output pass through cleanly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Conditional output based on `cfg.Format` |
| `internal/watch/watch_test.go` | modify | Add JSON mode tests |
| `cmd/apitest/main.go` | modify | Pass `format` through to `watch.Config` |

#### Current Code

`watchCmd` line 472:
```go
file, envName, _, _, _, _, noColor, _, _, err := parseRunArgs(args)
```

The `_` at position 3 is the `format` return value — currently discarded.

#### New Code

`watchCmd`:
```go
file, envName, format, _, _, _, noColor, _, _, err := parseRunArgs(args)
// ...
return watch.Run(ctx, watch.Config{
	// ... existing fields ...
	Format:  format,
	RunFunc: func(args []string) watch.RunResult { /* ... */ },
})
```

In `watch.go`, all terminal-specific output is gated:
```go
if cfg.Format != "json" {
	printSeparator(cfg.Stdout, cfg.UseColor, lastChanged)
}
result := cfg.RunFunc(cfg.Args)
totals.Add(result)
if cfg.Format != "json" {
	printRunningTotals(cfg.Stdout, cfg.UseColor, &totals)
}
```

When `Format == "json"`, each `RunFunc` call outputs a complete JSON object to stdout (handled by `runCmdInner`), and the watch layer adds no decoration. Runs are separated by the natural boundary of complete JSON objects (one per run).

#### Tests to Write FIRST (RED phase)

```go
// In TestRun:
t.Run("json format suppresses separator", func(t *testing.T) {
	// Set Format: "json", trigger rerun, verify no "Re-running" in stdout
})

t.Run("json format suppresses watching message", func(t *testing.T) {
	// Set Format: "json", verify no "Watching for changes..." in stdout
})

t.Run("json format suppresses running totals", func(t *testing.T) {
	// Set Format: "json", trigger rerun, verify no "Totals" in stdout
})
```

#### Impact on Existing Tests
- No existing tests set `Format`, so they default to `""` (terminal mode) — no change

---

### Step 5: Add `--clear` flag support

**Rationale:** Simple additive feature. Depends on `ClearScreen` field from Step 1.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Add `clearScreen` call before re-run when flag set |
| `internal/watch/watch_test.go` | modify | Add clear screen tests |
| `cmd/apitest/main.go` | modify | Parse `--clear` flag in `watchCmd` |

#### New Code

`watch.go`:
```go
func clearScreen(w io.Writer) {
	_, _ = fmt.Fprint(w, "\033[2J\033[H")
}
```

In the re-run block:
```go
case <-db.C:
	if cfg.ClearScreen && cfg.Format != "json" {
		clearScreen(cfg.Stdout)
	}
	if cfg.Format != "json" {
		printSeparator(cfg.Stdout, cfg.UseColor, lastChanged)
	}
	// ...
```

`watchCmd` in `main.go` — parse `--clear` before calling `parseRunArgs`:
```go
func watchCmd(args []string) int {
	// Extract watch-specific flags before parseRunArgs.
	clearScreen := false
	var filteredArgs []string
	for _, a := range args {
		if a == "--clear" {
			clearScreen = true
		} else {
			filteredArgs = append(filteredArgs, a)
		}
	}
	file, envName, format, _, _, _, noColor, _, _, err := parseRunArgs(filteredArgs)
	// ...
	return watch.Run(ctx, watch.Config{
		// ...
		ClearScreen: clearScreen,
		// ...
	})
}
```

#### Tests to Write FIRST (RED phase)

```go
// In TestRun:
t.Run("clear flag emits ANSI clear before rerun", func(t *testing.T) {
	// Set ClearScreen: true, trigger rerun.
	// Verify stdout contains "\033[2J\033[H"
})

t.Run("clear flag does not clear on initial run", func(t *testing.T) {
	// Set ClearScreen: true, cancel before any rerun.
	// Verify stdout does NOT contain "\033[2J"
})
```

#### Impact on Existing Tests
- No existing tests set `ClearScreen` — no change

---

### Step 6: Graceful parse error handling (verify + test)

**Rationale:** Mostly already working. Need to verify and add explicit tests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch_test.go` | modify | Add parse error tests |

#### Analysis

Current behavior when collection becomes invalid after edit:
1. `RunFunc` (which calls `runCmdInner`) tries to parse the file, fails, prints the error, returns exit code 3
2. Watch loop calls `CollectPaths` to refresh — this also fails (parse error), prints "failed to refresh paths" to stderr, and `continue`s the loop
3. Watch continues with old file set

This is correct behavior. The parse error is shown (by `RunFunc` to stdout and by `CollectPaths` to stderr), and watching continues. We just need tests to lock this down.

#### Tests to Write FIRST (RED phase)

```go
// In TestRun:
t.Run("parse error after edit shows error and continues watching", func(t *testing.T) {
	// Initial valid collection → RunFunc succeeds
	// Edit to invalid YAML → RunFunc returns non-zero
	// Edit back to valid → RunFunc succeeds again
	// Verify watch continued through the error (run count >= 3)
})
```

#### Impact on Existing Tests
- None — purely additive test

---

### Step 7: Refactor `runCmd` → `runCmdInner` + `runCmd`

**Rationale:** This is the actual refactor in `main.go` that enables Step 1's wrapper. Listed separately because it's the highest-risk change (modifying a large function).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Extract `runCmdInner`, keep `runCmd` as thin wrapper |

#### Current Code

```go
func runCmd(args []string) int {
	// ~310 lines of logic
	// Returns various exit codes
}
```

#### New Code

```go
func runCmdInner(args []string) (int, *runner.Summary) {
	// Same body, but instead of bare `return N`, track summary and return (N, summary)
	// The summary variable is already in scope (line 275)
}

func runCmd(args []string) int {
	code, _ := runCmdInner(args)
	return code
}
```

The key change is that every `return N` in `runCmd` becomes `return N, summary` (or `return N, nil` for early returns before `runner.Run`). The `summary` variable is already declared at line 275.

#### Tests to Write FIRST (RED phase)

No new tests needed — `runCmd` behavior is unchanged. Existing integration tests and smoke tests cover this.

#### Impact on Existing Tests
- `cmd/apitest/main.go` tests call `runCmd` which delegates to `runCmdInner` — transparent

---

### Step 8: Update help text and smoke test

**Rationale:** Last step — documentation and validation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | Update watch help text |
| `smoke/run.sh` | modify | Add JSON format watch smoke test |

#### New Help Text

In the watch usage string:
```
Usage: apitest watch <collection-file> [--env <name>] [--var key=value ...] [--format <type>] [--clear] [--no-color] [-v] [-vv] [-q]
```

#### Smoke Test Addition

```bash
# Watch with --format json (brief test — start, verify initial JSON output, stop)
```

#### Impact on Existing Tests
- None

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/watch/watch_test.go` | All 11 `TestRun/*` subtests | `RunFunc` signature change | Update return type to `RunResult` |
| `internal/watch/watch_test.go` | `TestSyncWatchDirs` | none | — |
| `internal/watch/watch_test.go` | `TestDebouncer` | none | — |
| `internal/watch/paths_test.go` | all | none | — |
| `cmd/apitest/*_test.go` | all | none (runCmd unchanged) | — |

## Risks and Edge Cases

- **Risk:** `runCmdInner` refactor introduces bugs in early-return paths where `summary` is nil
  - **Mitigation:** Every early return explicitly returns `nil` for summary. Existing tests + smoke test cover all paths.

- **Risk:** Running totals show "0 passed, 0 failed" when `RunFunc` returns zero-value `RunResult` (e.g., parse error path where runner never executes)
  - **Mitigation:** This is acceptable — a parse error run contributes 0 to all counters. The totals still accurately reflect successful runs.

- **Risk:** `--format json` and `--clear` combined
  - **Mitigation:** JSON mode ignores `--clear` (clearing terminal makes no sense for machine consumers). Explicit check in code.

- **Risk:** `--clear` erases running totals from previous runs
  - **Mitigation:** Expected behavior — clear wipes screen, then current run outputs its results plus cumulative totals.

- **Edge case:** Collection becomes invalid, then valid again
  - **Handling:** Already works. Invalid edit → `RunFunc` shows error, `CollectPaths` fails to refresh (old paths stay watched). Valid edit → everything resumes. New external files added during invalid state aren't watched until parse succeeds again. Acceptable.

- **Edge case:** First run has parse error
  - **Handling:** `RunFunc` handles the error. `CollectPaths` fails → `Run` returns 1 (watch never starts). This is correct — the user should fix the file before starting watch mode.

- **Edge case:** Very many re-runs accumulate large totals
  - **Handling:** Go `int` is 64-bit on modern systems. Overflow impossible in practice.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Terminal mode: start watch, edit file, observe separator + timestamp + running totals
apitest watch tests.yaml

# JSON mode: start watch, edit file, observe complete JSON objects
apitest watch tests.yaml --format json

# Clear mode: start watch, edit file, observe terminal clears between runs
apitest watch tests.yaml --clear

# Unit tests
go test ./internal/watch/... -v
```
