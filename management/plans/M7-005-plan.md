# Implementation Plan: M7-005

## Overview

Replace the process-global `os.Stdout` hijack in `captureJSONCollection` with a clean writer-injection architecture: thread explicit `stdout, stderr io.Writer` parameters through `runCmdInner`, extend `watch.Config.RunFunc` to carry writers, and migrate all affected in-process tests to use `bytes.Buffer` injection instead of `os.Pipe` fd-swaps. Adds two anti-regression test gates: `TestConcurrentDiscovery` (proves two goroutines can run with separate writer buffers without interleaving) and `TestNoOsStdoutAssignment` (a `go/ast` scan that fails if any source file in `cmd/curlew/` ever reassigns `os.Stdout` or `os.Stderr` again).

## Task Details
- **ID:** M7-005
- **Title:** Thread stdout and stderr writers through runCmdInner (remove os.Stdout swap)
- **Phase:** M7: Output Discipline
- **Priority:** 1
- **Complexity:** high

## Dependencies

None. Task is explicitly orthogonal to M7-001/002/003/004 and ships independently.

| Task | Title | Status |
|------|-------|--------|
| — | — | — |

## Plan Scope Decisions

Two interpretive calls drive the plan. Both are recorded here so the execute phase and the reviewer can evaluate the tradeoffs:

1. **DoD grep scope broader than "Tests to migrate" list.** The scope names only three test files (`main_test.go`, `run_test.go`, `discovery_run_test.go`) as requiring migration, but the DoD demands `grep -rn 'os.Stdout = ' cmd/curlew/` return zero matches. An audit shows 66 total `os.Stdout = `/`os.Stderr = ` assignments spread across five files:
   - `main_test.go` (20), `run_test.go` (16), `discovery_run_test.go` (0 — relies on subprocess runs), `plugins_test.go` (4), `perf_test.go` (24), `discovery_run.go` (2, production code).
   - To satisfy the DoD literally we must also migrate `plugins_test.go` and `perf_test.go`. We honor this by extending `pluginsCmd`, `perfCmd`, `printHelp`, `printPerfHelp`, and `printPluginsHelp` with writer parameters — a pure mechanical change that mirrors the `runCmdInner` pattern and costs little beyond the primary refactor.
   - This interpretation matches the M7 milestone intent ("Stream discipline audit — stdout/stderr segregation"): the task is systemic; the named-file list in scope is a non-exhaustive starting point.

2. **`watch.Config.RunFunc` signature change vs. closure capture.** Scope suggests two options: "extend RunFunc to accept writers, or use a closure that captures them." Chosen: extend the signature to `func([]string, io.Writer, io.Writer) RunResult`. Rationale — (a) the closure approach works today but hides the requirement; downstream readers must reverse-engineer why the closure doesn't just call `runCmdInner(a)`; (b) the explicit signature mirrors the underlying change, makes the test helper `watch_test.go` straightforwardly adaptable, and protects against future watch callers reintroducing the hidden-global pattern; (c) `watch_test.go` call sites all pass `func(_ []string) RunResult{...}` — trivial mechanical update to `func(_ []string, _, _ io.Writer) RunResult{...}`.

3. **`TestConcurrentDiscovery` location.** Placed in `cmd/curlew/discovery_run_test.go` (co-located with the target it exercises). The test calls `runCmdInner` directly from two goroutines with separate buffers; no subprocess needed.

4. **`TestNoOsStdoutAssignment` location and scope.** Placed in `cmd/curlew/main_test.go` (alongside other process-wide gate tests). The AST scan walks every `.go` file in the same directory via `os.ReadDir(".")`, parses each with `parser.ParseFile`, and rejects any `*ast.AssignStmt` whose LHS is the selector expression `os.Stdout` or `os.Stderr`. Test files are included — the whole point is that tests are the current offender list.

## Implementation Steps

Steps are ordered by blast radius: tests/helpers first, then the core function, then dependent callers, then regression gates. Each step keeps the tree green (`go build ./cmd/curlew` and `go test ./cmd/curlew/...` must pass between steps).

### Step 1: Add `printHelpTo(w io.Writer)` helpers (low blast radius)

**Rationale:** Several tests capture `printHelp()`, `printPerfHelp()`, `printPluginsHelp()` by swapping `os.Stdout`. Introducing writer-parameterised variants lets those tests migrate without changing the enclosing command structure. Smallest possible first step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Split `printHelp()` into `printHelpTo(w io.Writer)` + a one-line `printHelp()` that calls it with `os.Stdout`. |
| `cmd/curlew/perf.go` | modify | Same split for `printPerfHelp()` → `printPerfHelpTo(w io.Writer)`. |
| `cmd/curlew/plugins.go` | modify | Same split for `printPluginsHelp()` → `printPluginsHelpTo(w io.Writer)`. |

#### Current Code
```go
// main.go
func printHelp() {
    fmt.Println("Usage: curlew <command> [args...]")
    // ...many lines of fmt.Println...
}
```

#### New Code
```go
// main.go
func printHelp() {
    printHelpTo(os.Stdout)
}

func printHelpTo(w io.Writer) {
    _, _ = fmt.Fprintln(w, "Usage: curlew <command> [args...]")
    // ...convert every fmt.Println(...) to fmt.Fprintln(w, ...)...
}
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/curlew/main_test.go — new test added before changing printHelp:
func TestPrintHelpTo_WritesToProvidedWriter(t *testing.T) {
    var buf bytes.Buffer
    printHelpTo(&buf)
    if !strings.Contains(buf.String(), "--no-color") {
        t.Errorf("printHelpTo did not write expected content; got %q", buf.String())
    }
}
```

#### Impact on Existing Tests
- `TestHelpText_noColor`, `TestHelpText_format`, `TestHelpText_VerbosityFlags`, `TestHelpText_ContainsExec`, `TestRunCmd_Help_MentionsTeamTemplate`, `TestRunCmd_HelpMentionsWorkers`, `TestPrintPerfHelp_MentionsAllFlags` — migrate to call `printHelpTo(&buf)` / `printPerfHelpTo(&buf)` and drop the pipe/swap machinery.

### Step 2: Add `perfCmdOut` and `pluginsCmdOut` writer-parameterised variants (low blast radius)

**Rationale:** `perf_test.go` (24 swaps) and `plugins_test.go` (4 swaps) swap `os.Stdout` around in-process calls to `perfCmd`/`pluginsCmd`/`run(plugins list)`. Add writer-parameterised variants; keep the existing one-arg versions as thin wrappers that call the new ones with `os.Stdout`/`os.Stderr`. Tests migrate to the writer-parameterised variant.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/perf.go` | modify | Add `perfCmdOut(args []string, stdout, stderr io.Writer) int`. `perfCmd(args)` becomes `return perfCmdOut(args, os.Stdout, os.Stderr)`. Replace every `os.Stdout`/`os.Stderr` inside the function with the parameters. |
| `cmd/curlew/plugins.go` | modify | Add `pluginsCmdOut(args []string, stdout, stderr io.Writer) int` and `pluginsListCmdOut(args []string, stdout, stderr io.Writer) int`. Existing `pluginsCmd` / `pluginsListCmd` become one-line wrappers. |
| `cmd/curlew/main.go` | modify | Dispatcher in `run()` continues calling `perfCmd(args[1:])` / `pluginsCmd(args[1:])` — no change needed. |

#### Current Code
```go
// perf.go
func perfCmd(args []string) int {
    // ...fmt.Fprintf(os.Stderr, ...) many times; writes summary to stdout indirectly...
}
```

#### New Code
```go
// perf.go
func perfCmd(args []string) int {
    return perfCmdOut(args, os.Stdout, os.Stderr)
}

func perfCmdOut(args []string, stdout, stderr io.Writer) int {
    // ...every os.Stdout → stdout; every os.Stderr → stderr...
}
```

#### Tests to Write FIRST (RED phase)

```go
// perf_test.go new test:
func TestPerfCmdOut_RoutesToProvidedWriters(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(200)
    }))
    defer srv.Close()
    t.Setenv("CURLEW_TIER", "enterprise")
    f := writePerfRequestFile(t, srv.URL)

    var stdout, stderr bytes.Buffer
    code := perfCmdOut([]string{f, "--vus", "1", "--duration", "100ms"}, &stdout, &stderr)
    if code != 0 {
        t.Fatalf("exit %d; stderr=%q", code, stderr.String())
    }
    if !strings.Contains(stdout.String(), "Load test:") {
        t.Errorf("stdout missing summary; got %q", stdout.String())
    }
}
```

#### Impact on Existing Tests
- `perf_test.go`: all 24 `os.Stdout =`/`os.Stderr =` swaps replaced by calls to `perfCmdOut(args, &stdout, &stderr)`. Assertions unchanged.
- `plugins_test.go`: all 4 swaps in `capturePluginsOutput` replaced; helper rewritten to take `fn func(stdout, stderr io.Writer) int` and allocate the buffers internally. 11 call sites updated to the new closure signature.

### Step 3: `captureRunCmd`, `captureRun`, `captureExecCmd` helpers take buffers directly

**Rationale:** These helpers are the largest test offenders (20 + 16 + embedded = bulk of the swaps). Before changing `runCmdInner`, we first need the helper signatures to match the incoming writer-based API. Rewrite the helpers to allocate buffers and invoke the *new* signatures that come from Step 4. Because Step 3 depends on Step 4's signatures, these two steps land as one atomic commit (but are planned separately for clarity).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main_test.go` | modify | `captureRunCmd(t, args...)` — drop pipe machinery; allocate two `bytes.Buffer`s; call a new helper `runCmdWithWriters(args, stdout, stderr)` (see Step 4). |
| `cmd/curlew/main_test.go` | modify | `captureRun(t, args...)` — same treatment but for `run(args)`. This is trickier because `run` dispatches to many subcommands. See Step 5. |
| `cmd/curlew/main_test.go` | modify | `captureExecCmd(t, stdin, args...)` — rewrite to call `execCmdOut(args, stdout, stderr, stdinReader)` (new wrapper in Step 2 style). |

#### Current Code
```go
// main_test.go line 32:
func captureRunCmd(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
    t.Helper()
    oldOut := os.Stdout
    oldErr := os.Stderr
    rOut, wOut, _ := os.Pipe()
    rErr, wErr, _ := os.Pipe()
    os.Stdout = wOut
    os.Stderr = wErr
    exitCode = runCmd(args)
    // ...close/read pipes, restore os.Stdout/os.Stderr...
    return outBuf.String(), errBuf.String(), exitCode
}
```

#### New Code
```go
// main_test.go:
func captureRunCmd(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
    t.Helper()
    var outBuf, errBuf bytes.Buffer
    exitCode = runCmdWithWriters(args, &outBuf, &errBuf)
    return outBuf.String(), errBuf.String(), exitCode
}
```

#### Impact on Existing Tests
- Every caller of `captureRunCmd`/`captureRun`/`captureExecCmd` (dozens across `main_test.go` and `run_test.go`) continues to work because the helper preserves its `(stdout, stderr, exitCode)` signature. Internal plumbing only.
- Inline swap patterns (e.g. `run_test.go:684-710` that swaps `os.Stderr` for a team-template log capture, `run_test.go:840-918` for the workers cluster) become: allocate `var errBuf bytes.Buffer`; call a new `runCmdWithWriters([...], nil, &errBuf)` (nil stdout OK — runner writes nothing relevant for these tests) or pass a discard for the unused side.

### Step 4: Change `runCmdInner` to accept `stdout, stderr io.Writer` (the core change)

**Rationale:** The architectural heart of the task. All other steps depend on the new signature being in place. Do this in one commit; prior steps have already updated helpers and dependents to be ready.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Change `runCmdInner(args []string) (int, *runner.Summary)` → `runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary)`. |
| `cmd/curlew/main.go` | modify | Inside `runCmdInner`: replace every bare `os.Stdout` with `stdout`; every bare `os.Stderr` with `stderr`. Audit: ~50+ call sites (not "20+" as the scope estimates — recount confirms more). |
| `cmd/curlew/main.go` | modify | `runCmd(args)` becomes `return runCmdWithWriters(args, os.Stdout, os.Stderr)` where `runCmdWithWriters` is a thin wrapper used by both production entry and tests. (Alternative: have `runCmd` pass `os.Stdout`/`os.Stderr` directly. We add `runCmdWithWriters` as the test-friendly seam.) |
| `cmd/curlew/main.go` | modify | `newStderrPrinter(noColor)` is bound to `os.Stderr` today. Add `newStderrPrinterTo(w io.Writer, noColor bool)` that takes a writer; call it as `newStderrPrinterTo(stderr, noColor)` inside `runCmdInner`. Preserve the old helper for M7-001 compatibility — it remains the call site of last resort for `os.Stderr`-derived color. |
| `cmd/curlew/main.go` | modify | `stdoutUseColor := shouldUseColor(os.Stdout, noColor)` → `shouldUseColor(stdout, noColor)`. The `shouldUseColor` helper already accepts a writer. |
| `cmd/curlew/main.go` | modify | `newEventsAdapter(em, os.Stderr, ...)` at `main.go:497` and `:1050` → `newEventsAdapter(em, stderr, ...)`. |
| `cmd/curlew/main.go` | modify | `buildHookDispatcher(ctx, os.Stderr)` at `main.go:1005` → `buildHookDispatcher(ctx, stderr)`. (No signature change to `buildHookDispatcher` — it already takes `io.Writer`.) |
| `cmd/curlew/main.go` | modify | `distributed.Run(ctx, distributed.Config{..., Stdout: os.Stdout}, ...)` at `main.go:982` → `Stdout: stdout`. |
| `cmd/curlew/main.go` | modify | Inside `runCmdInner`, the `_, _ = fmt.Fprintln(os.Stderr, "Usage: curlew run …")` at `main.go:443` → `fmt.Fprintln(stderr, "Usage: …")`. Parse-error pre-flag case: writer is always the passed `stderr`. |
| `cmd/curlew/main.go` | modify | Terminal summary path: `output.NewPrinter(os.Stdout, stdoutUseColor, verbosity)` at `:994` and `:1056` → `output.NewPrinter(stdout, stdoutUseColor, verbosity)`. |
| `cmd/curlew/main.go` | modify | All `writeGateForFormat(os.Stdout, os.Stderr, ...)` calls → `writeGateForFormat(stdout, stderr, ...)`. |

#### Current Code
```go
// main.go:438
func runCmdInner(args []string) (int, *runner.Summary) {
    flags, parseErr := parseRunArgs(args)
    if parseErr != nil {
        _, _ = fmt.Fprintln(os.Stderr, "Usage: curlew run ...")
        errOut := newStderrPrinter(flags.noColor)
        errOut.StructuredError(parseErr)
        return 1, nil
    }
    // ...50+ references to os.Stdout / os.Stderr...
}

// main.go:1525
func runCmd(args []string) int {
    if code := checkGraceExpired(); code != 0 {
        return code
    }
    code, _ := runCmdInner(args)
    return code
}
```

#### New Code
```go
// main.go:438
func runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary) {
    flags, parseErr := parseRunArgs(args)
    if parseErr != nil {
        _, _ = fmt.Fprintln(stderr, "Usage: curlew run ...")
        errOut := newStderrPrinterTo(stderr, flags.noColor)
        errOut.StructuredError(parseErr)
        return 1, nil
    }
    // ...all os.Stdout → stdout; os.Stderr → stderr...
}

// main.go:1525
func runCmd(args []string) int {
    if code := checkGraceExpired(); code != 0 {
        return code
    }
    return runCmdWithWriters(args, os.Stdout, os.Stderr)
}

// new helper — test seam + production entry for runCmdInner-with-summary callers:
func runCmdWithWriters(args []string, stdout, stderr io.Writer) int {
    code, _ := runCmdInner(args, stdout, stderr)
    return code
}
```

#### Tests to Write FIRST (RED phase)

```go
// main_test.go — new table-driven test exercising writer injection:
func TestRunCmdInner_RoutesOutputToInjectedWriters(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(200)
    }))
    defer srv.Close()
    tmpDir := t.TempDir()
    f := writeSimpleCollection(t, tmpDir, srv.URL)

    cases := []struct {
        name           string
        args           []string
        wantStdoutSub  string
        wantStderrSub  string
        wantExitCode   int
    }{
        {"json success routes to stdout", []string{f, "--format", "json"}, `"status":`, "", 0},
        {"tap success routes to stdout", []string{f, "--format", "tap"}, "TAP version 13", "", 0},
        {"parse error routes to stderr", []string{"/nonexistent.yaml"}, "", "", 3},
        {"usage error routes to stderr", []string{"--bogus-flag"}, "", "Usage:", 1},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            var stdout, stderr bytes.Buffer
            code, _ := runCmdInner(tc.args, &stdout, &stderr)
            if code != tc.wantExitCode {
                t.Errorf("exit=%d want=%d", code, tc.wantExitCode)
            }
            if tc.wantStdoutSub != "" && !strings.Contains(stdout.String(), tc.wantStdoutSub) {
                t.Errorf("stdout missing %q; got %q", tc.wantStdoutSub, stdout.String())
            }
            if tc.wantStderrSub != "" && !strings.Contains(stderr.String(), tc.wantStderrSub) {
                t.Errorf("stderr missing %q; got %q", tc.wantStderrSub, stderr.String())
            }
        })
    }
}

// main_test.go — new: stdout is never touched when the path is error-only
func TestRunCmdInner_StdoutUntouched_OnParseError(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code, _ := runCmdInner([]string{"--bogus-flag"}, &stdout, &stderr)
    if code == 0 {
        t.Fatalf("expected non-zero exit")
    }
    if stdout.Len() != 0 {
        t.Errorf("stdout wrote %d bytes on parse-error path; should be zero (stderr-only): %q",
            stdout.Len(), stdout.String())
    }
}
```

#### Impact on Existing Tests
- Every test that calls `runCmdInner(args)` directly (watch plumbing tests, discovery runs via `captureJSONCollection`) — signature change. Mitigated because Step 3 already routed the top-level `captureRunCmd` / `runCmd` through the new signature. Only direct callers of `runCmdInner` need an update — audit confirms only one direct caller outside `main.go`: `captureJSONCollection` (Step 5 rewrites it) and `watchCmd`'s `RunFunc` closure (Step 6).
- `TestNewStderrPrinter_NoColorEnv` (main_test.go:720) — unaffected; `newStderrPrinter` retained.

### Step 5: Simplify `captureJSONCollection` — direct buffer injection (kills the fd-swap)

**Rationale:** The canonical bug site. After Step 4, `runCmdInner` accepts a writer; the function shrinks to a one-liner around it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/discovery_run.go` | modify | Rewrite `captureJSONCollection`: construct a `bytes.Buffer`, pass it to `runCmdInner`, parse the buffer bytes. Drop `os.Pipe`, `os.Stdout = w`, `os.Stdout = origStdout`. Drop `io.Copy(&buf, r)` — redundant. |
| `cmd/curlew/discovery_run.go` | modify | Update `runDiscoveredCollections` signature to accept `stdout, stderr io.Writer`; thread through. For JSON branch, pass per-iteration buffers. For non-JSON branch, pass the discovery-wide `stdout`/`stderr`. |

#### Current Code
```go
// discovery_run.go:303
func captureJSONCollection(args []string) (*output.JSONOutput, *runner.Summary, int) {
    origStdout := os.Stdout
    r, w, pipeErr := os.Pipe()
    if pipeErr != nil {
        return nil, nil, 3
    }
    defer func() { _ = r.Close() }()
    os.Stdout = w

    code, summary := runCmdInner(args)

    _ = w.Close()
    os.Stdout = origStdout

    var buf strings.Builder
    _, _ = io.Copy(&buf, r)

    var jsonOut output.JSONOutput
    if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &jsonOut); jsonErr != nil {
        return nil, summary, code
    }
    return &jsonOut, summary, code
}

// discovery_run.go:159 — runDiscoveredCollections (no writer params today)
func runDiscoveredCollections(matches []string, envName, format, report string, ...) (int, *runner.Summary) {
    // ...captures JSON via the os.Stdout hijack above; for terminal mode uses os.Stdout directly
    // at line 218: output.WriteMultiJSON(os.Stdout, multiOut)
    // at line 223: output.NewPrinter(os.Stdout, ...)
}
```

#### New Code
```go
// discovery_run.go:303
// captureJSONCollection runs one collection in JSON format into an in-memory
// buffer and returns the parsed JSON document, summary, and exit code. No
// process-global file-descriptor manipulation — safe to invoke concurrently.
func captureJSONCollection(args []string, stderr io.Writer) (*output.JSONOutput, *runner.Summary, int) {
    var buf bytes.Buffer
    code, summary := runCmdInner(args, &buf, stderr)

    var jsonOut output.JSONOutput
    if jsonErr := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &jsonOut); jsonErr != nil {
        return nil, summary, code
    }
    return &jsonOut, summary, code
}

// discovery_run.go:159
func runDiscoveredCollections(
    matches []string,
    envName, format, report string,
    cliVars, envVarVars map[string]string,
    seed *int64,
    noColor bool,
    verbosity output.Verbosity,
    allowSensitive, showDeps, dryRun, runParallel, confirmLargeDS bool,
    stdout, stderr io.Writer,
) (int, *runner.Summary) {
    // ...for JSON branch: captureJSONCollection(args, stderr).
    // ...for non-JSON branch: runCmdInner(args, stdout, stderr).
    // ...aggregate output writes: output.WriteMultiJSON(stdout, multiOut); output.NewPrinter(stdout, ...).
    // ...error messages: fmt.Fprintf(stderr, "json encode error: %v\n", err).
}
```

#### Tests to Write FIRST (RED phase)

```go
// discovery_run_test.go — new: proves the fd-swap is gone
func TestCaptureJSONCollection_DoesNotTouchOsStdout(t *testing.T) {
    origStdout := os.Stdout
    origStderr := os.Stderr
    t.Cleanup(func() {
        if os.Stdout != origStdout {
            t.Errorf("os.Stdout was replaced during test run")
        }
        if os.Stderr != origStderr {
            t.Errorf("os.Stderr was replaced during test run")
        }
    })

    tmpDir := t.TempDir()
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(200)
    }))
    defer srv.Close()
    f := writeSimpleCollection(t, tmpDir, srv.URL)

    args := buildArgsForCollection(f, "", "json", "", nil, nil, nil, true, output.VerbosityDefault,
        false, false, false, false, false)
    var stderr bytes.Buffer
    doc, _, code := captureJSONCollection(args, &stderr)
    if code != 0 {
        t.Fatalf("exit=%d stderr=%q", code, stderr.String())
    }
    if doc == nil || doc.Status != "passed" {
        t.Errorf("expected passed status; got %+v", doc)
    }
}

// TestConcurrentDiscovery — primary regression gate for the task.
func TestConcurrentDiscovery(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(200)
    }))
    defer srv.Close()

    tmpDir := t.TempDir()
    colA := writeNamedCollection(t, tmpDir, "a.yaml", "collection_A", srv.URL)
    colB := writeNamedCollection(t, tmpDir, "b.yaml", "collection_B", srv.URL)

    var (
        wg           sync.WaitGroup
        stdoutA      bytes.Buffer
        stdoutB      bytes.Buffer
        stderrA      bytes.Buffer
        stderrB      bytes.Buffer
        codeA, codeB int
    )
    wg.Add(2)
    go func() {
        defer wg.Done()
        codeA, _ = runCmdInner([]string{colA, "--format", "json"}, &stdoutA, &stderrA)
    }()
    go func() {
        defer wg.Done()
        codeB, _ = runCmdInner([]string{colB, "--format", "json"}, &stdoutB, &stderrB)
    }()
    wg.Wait()

    if codeA != 0 || codeB != 0 {
        t.Fatalf("exit codes codeA=%d codeB=%d stderrA=%q stderrB=%q", codeA, codeB, stderrA.String(), stderrB.String())
    }

    // Each buffer contains a single well-formed JSON document referencing its own collection.
    var docA, docB output.JSONOutput
    if err := json.Unmarshal(bytes.TrimSpace(stdoutA.Bytes()), &docA); err != nil {
        t.Fatalf("buffer A not valid JSON (interleaved?): %v\nraw: %q", err, stdoutA.String())
    }
    if err := json.Unmarshal(bytes.TrimSpace(stdoutB.Bytes()), &docB); err != nil {
        t.Fatalf("buffer B not valid JSON (interleaved?): %v\nraw: %q", err, stdoutB.String())
    }
    if docA.Name != "collection_A" {
        t.Errorf("buffer A cross-contaminated — got collection %q, want collection_A", docA.Name)
    }
    if docB.Name != "collection_B" {
        t.Errorf("buffer B cross-contaminated — got collection %q, want collection_B", docB.Name)
    }
    // Buffer A must not contain B's collection name and vice versa.
    if strings.Contains(stdoutA.String(), "collection_B") {
        t.Errorf("buffer A contains content from buffer B — interleaving detected")
    }
    if strings.Contains(stdoutB.String(), "collection_A") {
        t.Errorf("buffer B contains content from buffer A — interleaving detected")
    }
}
```

#### Impact on Existing Tests
- `TestBuildArgsForCollection`, `TestAggregateSummaries`, `TestWorseExitCode`, `TestAggregateExitCodes` — unaffected (no I/O).
- Any existing test that called `captureJSONCollection(args)` — signature changes, add a `bytes.Buffer` stderr argument.
- `runDiscoveredCollections` callers (exactly one: `runCmdInner`) — update call site to pass `stdout, stderr`.

### Step 6: Update `watchCmd` RunFunc signature and closure

**Rationale:** Independent surface; do after the core is stable. Extends `watch.Config.RunFunc` signature and is the last call-site touch before the regression gate.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/watch/watch.go` | modify | Change `RunFunc` type to `func([]string, io.Writer, io.Writer) RunResult`. Update call sites `cfg.RunFunc(cfg.Args)` → `cfg.RunFunc(cfg.Args, cfg.Stdout, cfg.Stderr)` at `watch.go:67` and `:139`. |
| `internal/watch/watch_test.go` | modify | All 20+ `RunFunc: func(_ []string) RunResult{...}` → `RunFunc: func(_ []string, _, _ io.Writer) RunResult{...}`. |
| `cmd/curlew/main.go` | modify | Inside `watchCmd`, update `RunFunc: func(a []string) watch.RunResult { exitCode, summary := runCmdInner(a); ... }` → `RunFunc: func(a []string, stdout, stderr io.Writer) watch.RunResult { exitCode, summary := runCmdInner(a, stdout, stderr); ... }`. |

#### Current Code
```go
// internal/watch/watch.go:52
RunFunc func([]string) RunResult

// internal/watch/watch.go:67
result := cfg.RunFunc(cfg.Args)

// cmd/curlew/main.go:1570
RunFunc: func(a []string) watch.RunResult {
    exitCode, summary := runCmdInner(a)
    ...
}
```

#### New Code
```go
// internal/watch/watch.go:52
RunFunc func([]string, io.Writer, io.Writer) RunResult

// internal/watch/watch.go:67
result := cfg.RunFunc(cfg.Args, cfg.Stdout, cfg.Stderr)

// cmd/curlew/main.go:1570
RunFunc: func(a []string, stdout, stderr io.Writer) watch.RunResult {
    exitCode, summary := runCmdInner(a, stdout, stderr)
    ...
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/watch/watch_test.go — new: RunFunc receives the Config writers verbatim
func TestRun_PassesConfigWritersToRunFunc(t *testing.T) {
    var capturedStdout, capturedStderr io.Writer
    cfg := Config{
        CollectionPath: filepath.Join(t.TempDir(), "x.yaml"),
        Args:           []string{"x.yaml"},
        Stdout:         &bytes.Buffer{},
        Stderr:         &bytes.Buffer{},
        Debounce:       50 * time.Millisecond,
        RunFunc: func(_ []string, stdout, stderr io.Writer) RunResult {
            capturedStdout = stdout
            capturedStderr = stderr
            return RunResult{}
        },
    }
    // ...write a dummy collection; cancel ctx after 200ms...
    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()
    _ = Run(ctx, cfg)
    if capturedStdout != cfg.Stdout {
        t.Errorf("RunFunc got stdout %p; want cfg.Stdout %p", capturedStdout, cfg.Stdout)
    }
    if capturedStderr != cfg.Stderr {
        t.Errorf("RunFunc got stderr %p; want cfg.Stderr %p", capturedStderr, cfg.Stderr)
    }
}
```

#### Impact on Existing Tests
- `internal/watch/watch_test.go`: 20+ closures updated mechanically — add two unused `io.Writer` params to each.
- The primary assertion in each existing test is on counters (`count.Add`) — no change to behaviour.

### Step 7: Anti-regression AST gate — `TestNoOsStdoutAssignment`

**Rationale:** The final ratchet. Parses every `.go` file in `cmd/curlew/` and fails if any `*ast.AssignStmt` reassigns `os.Stdout` or `os.Stderr`. Prevents future reintroduction of the fd-swap pattern.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main_test.go` | modify | Add `TestNoOsStdoutAssignment`. |

#### New Code
```go
// main_test.go — new test
import (
    "go/ast"
    "go/parser"
    "go/token"
    // ...existing imports...
)

// TestNoOsStdoutAssignment is an anti-regression gate: it walks every .go file
// in the current (cmd/curlew) directory and fails if any source assigns to
// os.Stdout or os.Stderr. Reassigning these process globals corrupts streams
// shared by concurrent goroutines (M7-005 removed the last offender in
// discovery_run.go). Keep this test green — if you need to redirect output,
// pass a writer through the function's signature instead.
func TestNoOsStdoutAssignment(t *testing.T) {
    entries, err := os.ReadDir(".")
    if err != nil {
        t.Fatalf("ReadDir: %v", err)
    }
    fset := token.NewFileSet()
    var offenders []string
    for _, e := range entries {
        if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
            continue
        }
        file, err := parser.ParseFile(fset, e.Name(), nil, parser.SkipObjectResolution)
        if err != nil {
            t.Fatalf("parse %s: %v", e.Name(), err)
        }
        ast.Inspect(file, func(n ast.Node) bool {
            as, ok := n.(*ast.AssignStmt)
            if !ok {
                return true
            }
            for _, lhs := range as.Lhs {
                sel, ok := lhs.(*ast.SelectorExpr)
                if !ok {
                    continue
                }
                ident, ok := sel.X.(*ast.Ident)
                if !ok {
                    continue
                }
                if ident.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr") {
                    pos := fset.Position(as.Pos())
                    offenders = append(offenders, fmt.Sprintf("%s:%d: reassignment of os.%s", pos.Filename, pos.Line, sel.Sel.Name))
                }
            }
            return true
        })
    }
    if len(offenders) > 0 {
        t.Fatalf("os.Stdout/os.Stderr reassignment detected (M7-005 prohibits this):\n  %s", strings.Join(offenders, "\n  "))
    }
}
```

#### Tests to Write FIRST (RED phase)

This test itself is the RED test — it will initially fail (because tests still swap `os.Stdout`). It goes GREEN after Steps 1–6 complete. We add it in Step 1 (so the whole pipeline is driven by its failure) OR as the final step that verifies completion. **Decision: add it at the end of Step 1, keep it in a `t.Skip` state with a "TODO: unskip in Step 7" comment, then unskip it as part of Step 7.** Rationale — if we add it green at the end it can't catch mid-stream regressions in the execute phase; the skip/unskip gate tracks progress in the harness itself.

Alternative considered and rejected: add the test at Step 1 without skip — then every intermediate commit leaves tests red, which violates the "always runnable" principle. Skip-then-unskip preserves it.

#### Impact on Existing Tests
- None — pure anti-regression addition.

### Step 8: Update `CHANGELOG.md`

**Rationale:** Required by DoD.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add entry under a new Unreleased section (or next pending section). |

#### New Code
```markdown
### Changed
- `runCmdInner` now takes explicit `stdout, stderr io.Writer` parameters (M7-005). The
  previous process-global `os.Stdout`-swap hijack in `captureJSONCollection` has been
  removed; glob-discovered collections and future parallel runs can now execute in
  separate goroutines with independent output buffers. `watch.Config.RunFunc`
  signature changed to `func([]string, io.Writer, io.Writer) watch.RunResult`.
```

## Test Impact Summary

| Test File | Test / Helper | Impact | Action Required |
|-----------|---------------|--------|-----------------|
| `cmd/curlew/main_test.go` | `captureRunCmd`, `captureRun`, `captureExecCmd` | breaks | Rewrite to allocate `bytes.Buffer`s; invoke `runCmdWithWriters` / `execCmdOut`. |
| `cmd/curlew/main_test.go` | `TestHelpText_noColor`, `TestHelpText_format`, `TestHelpText_VerbosityFlags`, `TestHelpText_ContainsExec` | breaks | Switch to `printHelpTo(&buf)`. |
| `cmd/curlew/main_test.go` | `TestNewStderrPrinter_NoColorEnv` | none | `newStderrPrinter` retained. |
| `cmd/curlew/main_test.go` | `TestRunCmdInner_RoutesOutputToInjectedWriters` (new) | adds | RED→GREEN gate for Step 4. |
| `cmd/curlew/main_test.go` | `TestRunCmdInner_StdoutUntouched_OnParseError` (new) | adds | RED→GREEN gate for stderr discipline. |
| `cmd/curlew/main_test.go` | `TestNoOsStdoutAssignment` (new) | adds | Added-then-skipped in Step 1; unskipped in Step 7. |
| `cmd/curlew/run_test.go` | `TestRunCmd_*_TeamTemplate_*`, `TestRunCmd_Workers*` | breaks | Replace `os.Stderr = w` swaps with `runCmdWithWriters(args, &stdout, &stderr)`. |
| `cmd/curlew/run_test.go` | `TestRunCmd_Help_MentionsTeamTemplate`, `TestRunCmd_HelpMentionsWorkers`, `TestRunCmd_WorkersObservableE2E` | breaks | Use `printHelpTo(&buf)` / `runCmdWithWriters`. |
| `cmd/curlew/discovery_run_test.go` | `TestCaptureJSONCollection_DoesNotTouchOsStdout` (new) | adds | Verifies the fd-swap is gone. |
| `cmd/curlew/discovery_run_test.go` | `TestConcurrentDiscovery` (new) | adds | The primary observable regression gate. |
| `cmd/curlew/perf_test.go` | `TestPrintPerfHelp_MentionsAllFlags`, `TestPerfCmd_Run_HTTPTestServer_Success`, `TestPerfCmd_RPSHeaderInStdout`, `TestPerfCmd_OutputJSON_WritesFile`, `TestPerfCmd_Run_HTTPTestServer_AllFailures`, `TestPerfCmd_ContextCancelExitCode130`, `TestPerfCmd_TierGate`, and all other swap sites | breaks | Replace 24 swap sites with `perfCmdOut(args, &stdout, &stderr)` / `printPerfHelpTo(&buf)`. |
| `cmd/curlew/plugins_test.go` | `capturePluginsOutput` | breaks | Rewrite helper to take `fn func(stdout, stderr io.Writer) int` and allocate buffers internally. 11 call sites updated. |
| `internal/watch/watch_test.go` | all `RunFunc:` closures (20+) | breaks | Add two unused `io.Writer` parameters. |
| `internal/watch/watch_test.go` | `TestRun_PassesConfigWritersToRunFunc` (new) | adds | Verifies RunFunc receives Config writers. |

Total: ~90 test migrations, 5 new test functions, zero test deletions.

## Risks and Edge Cases

- **Risk:** Some `os.Stdout`/`os.Stderr` references inside `runCmdInner` are deep-nested (e.g. feature-gate branches at 527, 541; distributed path at 975–1000; hookDispatcher at 1005; events adapter construction at 497 and 1050). Missing one leaves a split-brain state where part of the output goes to the buffer and part goes to the real stdout.
  → **Mitigation:** Step 4 includes `TestRunCmdInner_StdoutUntouched_OnParseError` which asserts `stdout.Len() == 0` on error paths. Add three more coverage tests (feature-gate, events-file-error, hook-error) as micro-cases under the same parent Test — ensures every exit-path writer is correctly parameterised. After the bulk replace, run `grep -n "os\\.Stdout\\|os\\.Stderr" cmd/curlew/main.go | grep -v "_test\\.go"` and audit every remaining hit — only `runCmd`, `watchCmd`, the dispatcher in `run()`, and out-of-scope commands (execCmd, validateCmd, etc.) may legitimately reference `os.Stdout`/`os.Stderr`.

- **Risk:** `captureJSONCollection`'s behaviour change (fd-swap → buffer) might alter byte ordering if previous runs relied on OS pipe buffering to serialise interleaved writes. Today the function returns the full JSON doc only after the run finishes — no streaming. Safe.
  → **Mitigation:** golden-file tests already cover multi-collection JSON output. If any goldens shift, the bytes should be byte-identical (same writer, same formatter) — a diff implies a bug to fix, not an acceptable goldens update.

- **Risk:** `TestConcurrentDiscovery` touches the real filesystem and spawns an `httptest.NewServer` — flaky if the goroutines deadlock on a shared resource. Neither `runCmdInner` nor `httpexec.Execute` owns shared mutable state today (after the removal of the fd-swap), so this should be clean. Race detector must pass (`go test -race`).
  → **Mitigation:** Run `go test -race -run TestConcurrentDiscovery ./cmd/curlew/...` locally as part of verification. `ci-local.sh` already runs with `-race`.

- **Risk:** The `--events` emitter at `main.go:478` opens a file and writes NDJSON to it. If an instance of `runCmdInner` is called with the same `--events` path from two goroutines (concurrent events file writes), the file will corrupt. This risk exists today and is not introduced by the task; note it here so the review phase can confirm that concurrent usage is not a supported scenario and the `TestConcurrentDiscovery` test does not use `--events`.
  → **Mitigation:** The new test avoids `--events`. Add a doc comment on `runCmdInner` noting that the `--events` path is not safe for concurrent invocation with the same file.

- **Edge case:** `runCmdInner` calls `os.Getenv("CURLEW_TEAM_CONFIG")` and `os.Getenv("CURLEW_VAULT_STUB")` — those are process globals, not writer globals. The plan does not change them.
  → **Handling:** `TestConcurrentDiscovery` uses a temp dir with no `CURLEW_TEAM_CONFIG` set; no risk.

- **Edge case:** `os.Stdin` is swapped in `captureExecCmd`. The task is about stdout/stderr only. `os.Stdin` swaps remain (they do not match the grep `os.Stdout = ` / `os.Stderr = `).
  → **Handling:** Out of scope. No action.

- **Edge case:** `main.go:564` sets `stdoutUseColor := shouldUseColor(os.Stdout, noColor)`. With writer injection, this becomes `shouldUseColor(stdout, noColor)`. When tests pass a `*bytes.Buffer`, `output.IsTerminal` returns false → `stdoutUseColor` is false → ANSI escapes not emitted. Matches current test behaviour because the pipe-swap also hides the TTY from `isatty` checks. No behaviour change.
  → **Handling:** Verified via existing golden output tests; if any diff, it's a bug.

- **Edge case:** Unknown args without a command printed `os.Stderr` from the top-level `run()`. Not in `runCmdInner`. Unchanged.

- **Edge case:** Two-phase migration for tests (Step 3 + Step 4): during the inter-commit interval, tests may temporarily reference a helper that does not exist yet. Keep both steps in a single commit to avoid a broken intermediate state, OR land Step 3 with a stub `runCmdWithWriters` that delegates to the old `runCmdInner` before Step 4 flips the signature.
  → **Mitigation:** Execute phase lands Steps 3 and 4 in one atomic commit.

- **Risk:** `TestNoOsStdoutAssignment` uses `os.ReadDir(".")` which depends on the `go test` invocation being run from `cmd/curlew/`. `go test ./...` invokes tests with the package directory as CWD, so this works. Document the dependency.
  → **Mitigation:** Add comment on the test.

## Verification

```bash
go build ./cmd/curlew
go test ./...
go test -race ./cmd/curlew/...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh --go
```

Observable verification (matches task YAML):

```bash
# 1. captureJSONCollection no longer hijacks os.Stdout at the process level.
grep -rn "os.Stdout = " cmd/curlew/
# Expected: zero matches.

grep -rn "os.Stderr = " cmd/curlew/
# Expected: zero matches.

# 2. runCmdInner takes explicit writer parameters.
grep -n "^func runCmdInner" cmd/curlew/main.go
# Expected: func runCmdInner(args []string, stdout, stderr io.Writer) (int, *runner.Summary)

# 3. Concurrent glob discovery works without stream corruption.
go test -run TestConcurrentDiscovery ./cmd/curlew/...
# Expected: PASS.

# 4. Static-analysis gate rejects future reassignment of os.Stdout.
go test -run TestNoOsStdoutAssignment ./cmd/curlew/...
# Expected: PASS.

# 5. Existing test suite is green.
go test ./...
# Expected: all PASS.

# 6. Coverage does not regress.
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1
# Expected: total coverage ≥ 80%.
```
