# Implementation Plan: M6-005

## Overview
Wire the NDJSON `events` emitter (M6-004) into the `run` subcommand behind a new `--events <path>` flag. This delivers a streaming `run.start` → per-request events → `run.end` NDJSON file alongside all existing `--format` outputs, with pre-run error coverage and clean rejection on non-run subcommands.

## Task Details
- **ID:** M6-005
- **Title:** Wire --events flag into the run subcommand end-to-end
- **Phase:** M6: AI Agent Integration
- **Priority:** 3
- **Complexity:** high
- **Estimated effort:** 2-3 days

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-003 | Body redaction for sensitive values | done |
| M6-004 | Events emitter package with NDJSON event types | done |

## Key Architectural Decisions

1. **Keep `internal/runner` agnostic of `internal/output/events`.** Introduce a narrow `runner.EventSink` interface (`RequestStart`, `RequestEnd`, `AssertionResult`) that `cmd/curlew/main.go` satisfies by adapting `*events.Emitter`. The runner never imports the events package, avoiding an import cycle and keeping the callback surface minimal and testable with a fake sink.
2. **`run.start` / `run.end` / `run.error` live entirely in `cmd/curlew/main.go`.** These events frame the run and encode CLI state (args, collection file, env name, exit code). The runner has no ambient knowledge of exit codes or CLI args, so emitting them from `main` is the natural seam.
3. **Early writability validation.** Open the events file with `os.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0o644)` *before* any HTTP execution, parser load, or runner dispatch so a bad path fails fast with a structured `[ERROR]` and exit 1 — satisfying "no partial file is created" and "exits with a clear error before executing any requests".
4. **Parallel wave emission.** Extend `parallel.Config` with an optional `EventSink` (interface mirror of the runner's). The parallel executor invokes it inside each goroutine around the `exec` call so request IDs, `wave_index`, and ordering survive parallel execution. The emitter's internal atomic counter keeps event `id` strictly monotonic across goroutines (already covered by `TestEmitter_ConcurrentEmitMonotonicIDs`).
5. **Request ID generation.** The adapter in `main.go` owns a `*atomic.Int64` and hands each request a stable string id (`req-NNN`) at `RequestStart`, stored alongside a closure that `RequestEnd` / `AssertionResult` dispatch to. For data-driven iterations, each iteration receives its own id.
6. **No changes to existing formatters or goldens.** All existing `--format` paths write to `os.Stdout`; events write only to the user-supplied path. When the path is `/dev/stdout`, the OS interleaves — we document this but take no special action beyond a sanity test.
7. **Rejection on non-run subcommands.** `workerCmd`, `perfCmd`, `prCheckCmd`, and other top-level subcommands explicitly reject `--events` with the exact message: `--events is supported only on run; use --format jsonl for streaming samples`. No events file is opened.

## Implementation Steps

### Step 1: Define `runner.EventSink` and thread it through `VarSources`
**Rationale:** This is the smallest, widest-reach change. Introducing the interface first lets every later step (parallel, data-driven, main wiring) target a stable surface. No emission logic yet — the interface exists, and all callers leave `OnEvent` nil so no behavior change is observable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `EventSink` interface and `VarSources.OnEvent EventSink` field |
| `internal/runner/runner_test.go` | modify | Add `TestRun_EventSink_Nil_NoOp` covering backward compatibility |

#### New Code
```go
// Added near top of runner.go (after Phase constants).

// EventSink receives per-request lifecycle notifications from the runner so
// callers (cmd/curlew) can translate them into an NDJSON event stream.
// Every method is non-blocking — implementations must not return errors that
// halt execution; emission failures are logged to stderr by the adapter.
//
// Contract:
//   - RequestStart fires exactly once per request, before the first exec call.
//   - AssertionResult fires once per individual assertion item, after Evaluate.
//   - RequestEnd fires exactly once per request, after assertion evaluation,
//     retries, and variable extraction — regardless of outcome.
type EventSink interface {
    RequestStart(ev RequestEvent)
    RequestEnd(ev RequestEndEvent)
    AssertionResult(ev AssertionEvent)
}

// RequestEvent describes a request lifecycle start for event emission.
type RequestEvent struct {
    RequestID  string
    Name       string
    Method     string
    URL        string
    Phase      string
    SourceFile string
    SourceLine int
}

// RequestEndEvent describes a completed request for event emission.
type RequestEndEvent struct {
    RequestID    string
    Outcome      string // "passed" | "failed" | "skipped" | "error"
    StatusCode   int
    Duration     time.Duration
    WaveIndex    int
    RequestBody  []byte
    ResponseBody []byte
    Err          error
}

// AssertionEvent describes one evaluated assertion for event emission.
type AssertionEvent struct {
    RequestID string
    Type      string
    Expected  string
    Actual    string
    Passed    bool
}
```

```go
// Added to VarSources (after Hooks field):

    // OnEvent receives per-request lifecycle callbacks. When nil, the runner
    // incurs zero overhead. M6-005 adds this to support the --events NDJSON
    // stream in cmd/curlew; no internal package imports output/events.
    OnEvent EventSink
```

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go — new test
type recordingSink struct {
    mu       sync.Mutex
    starts   []RequestEvent
    ends     []RequestEndEvent
    asserts  []AssertionEvent
}

func (r *recordingSink) RequestStart(e RequestEvent)        { r.mu.Lock(); r.starts = append(r.starts, e); r.mu.Unlock() }
func (r *recordingSink) RequestEnd(e RequestEndEvent)       { r.mu.Lock(); r.ends = append(r.ends, e); r.mu.Unlock() }
func (r *recordingSink) AssertionResult(e AssertionEvent)   { r.mu.Lock(); r.asserts = append(r.asserts, e); r.mu.Unlock() }

func TestRun_EventSink_Nil_NoOp(t *testing.T) {
    col := makeCollection([]string{"a", "b"}, false)
    results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
    if err != nil { t.Fatal(err) }
    if len(results) != 2 { t.Fatalf("want 2 results") }
    // Implicit assertion: nil sink is never invoked — no panics, no slice writes.
}
```

#### Impact on Existing Tests
- No existing test breaks — new field is zero-valued in all existing test call sites.

---

### Step 2: Emit RequestStart/End/AssertionResult from sequential `executePhase`
**Rationale:** Covers the default non-parallel path first. The sequential code is linear and easy to instrument without affecting retries/refresh-on-failure branches. Data-driven and parallel share structure that builds on this.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Call `vars.OnEvent.RequestStart/End/AssertionResult` at each per-request branch in `executePhase` |
| `internal/runner/runner_test.go` | modify | Add `TestRun_EventSink_SequentialPassingRequest`, `..._FailingAssertion`, `..._NetworkError`, `..._SkippedOnStopOnFailure` |

#### Current Code (excerpt, `executePhase` — representative HTTP branch)
```go
// Currently, after assertions:
rr := RequestResult{
    Name: item.Name, Phase: phase, ...
    Result: result, AssertionResults: ar, ...
}
if ar != nil && !ar.Passed {
    ...
    results = append(results, rr)
    continue
}
```

#### New Code (shape)
```go
// Just before exec (or retry wrap):
var reqID string
if vars.OnEvent != nil {
    reqID = vars.nextRequestID() // atomic-counter-backed helper
    vars.OnEvent.RequestStart(RequestEvent{
        RequestID: reqID, Name: item.Name, Method: req.Method, URL: req.URL,
        Phase: string(phase), SourceFile: item.SourceFile, SourceLine: item.SourceLine,
    })
}

// ... existing exec / assertion evaluation ...

// After assertions, before the final append:
if vars.OnEvent != nil {
    // Per-assertion events
    if ar != nil {
        for _, a := range ar.Items {
            vars.OnEvent.AssertionResult(AssertionEvent{
                RequestID: reqID, Type: a.Type, Expected: a.Expected, Actual: a.Actual, Passed: a.Passed,
            })
        }
    }
    // Terminal request event
    outcome := outcomeFor(rr) // helper mapping RequestResult → "passed"|"failed"|"error"|"skipped"
    vars.OnEvent.RequestEnd(RequestEndEvent{
        RequestID: reqID, Outcome: outcome,
        StatusCode: statusCodeFor(rr), Duration: durationFor(rr),
        WaveIndex: -1, RequestBody: requestBodyBytes(rr), ResponseBody: responseBodyBytes(rr),
        Err: rr.Err,
    })
}
```

An unexported helper `vars.nextRequestID()` works via a lazily-created counter living on a new (private) field pointer:
```go
// VarSources additions:
    // reqIDCounter is a per-Run atomic counter shared with parallel execution.
    // Lazily created inside Run so external callers never touch it.
    reqIDCounter *atomic.Int64
```
Initialised in `Run` before dispatch:
```go
var idc atomic.Int64
vars.reqIDCounter = &idc
```
Helper:
```go
func (v VarSources) nextRequestID() string {
    return fmt.Sprintf("req-%d", v.reqIDCounter.Add(1))
}
```

**Skipped items:** When `stopped=true` we already write `RequestResult{Skipped:true}`; the sink emission for skipped items goes directly to `RequestEnd` with `Outcome="skipped"` — no matching `RequestStart`. This mirrors how skipped requests never actually start. Alternative considered (and rejected): emit a `RequestStart`/`RequestEnd` pair even for skipped items — that would require emitting `source_file`/`source_line` twice and inflate event counts for large skipped suites.

#### Tests to Write FIRST (RED phase)

```go
func TestRun_EventSink_SequentialPassingRequest(t *testing.T) {
    sink := &recordingSink{}
    col := makeCollectionWithAssertions("ok", []int{200}, false) // expects 200
    _, _, err := Run(context.Background(), col, successExecutor, VarSources{OnEvent: sink})
    if err != nil { t.Fatal(err) }
    if len(sink.starts) != 1 || len(sink.ends) != 1 {
        t.Fatalf("want 1 start + 1 end, got %d/%d", len(sink.starts), len(sink.ends))
    }
    if sink.starts[0].RequestID != sink.ends[0].RequestID {
        t.Errorf("request_id mismatch")
    }
    if sink.ends[0].Outcome != "passed" {
        t.Errorf("outcome = %q, want passed", sink.ends[0].Outcome)
    }
    if len(sink.asserts) != 1 || !sink.asserts[0].Passed {
        t.Errorf("expected 1 passing assertion, got %+v", sink.asserts)
    }
}

func TestRun_EventSink_FailingAssertion(t *testing.T) { /* exec returns 500; expect outcome="failed" + failing assertion event */ }
func TestRun_EventSink_NetworkError(t *testing.T)    { /* exec returns err; expect outcome="error", Err non-nil, no assertion events */ }
func TestRun_EventSink_GuardRailSkip(t *testing.T)   { /* MaxRequests=1 with 3 items; expect 1 request.start+end and 2 skipped request.end with Outcome="skipped" */ }
func TestRun_EventSink_RequestIDsMonotonic(t *testing.T) { /* 5 items; IDs = req-1..req-5 in order */ }
```

#### Impact on Existing Tests
- `TestRun_WithHooksDispatcher_*` — unaffected (hooks ≠ EventSink; coexist).
- `TestRunner_RequestResultCarriesSourceLocation` — unaffected (source plumbing unchanged).
- All existing sequential tests continue to pass with `OnEvent:nil`.

---

### Step 3: Extend `parallel.Config` with an `EventSink` and emit in wave goroutines
**Rationale:** Parallel execution must emit per-request events with correct `wave_index`. The runner owns the adapter, so the cleanest approach is to pass a narrow sink through `parallel.Config` (mirroring the runner interface). Order: do this after Step 2 so the sequential tests already pin the emission contract.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add `Config.EventSink ParallelEventSink` field and emit inside `executeOneRequest` (or wrap before retry) |
| `internal/parallel/executor_test.go` | modify | Add wave-level emission test asserting monotonic IDs across goroutines and `WaveIndex` correctness |
| `internal/runner/runner.go` | modify | In `executeParallelMain`, synthesize a `parallel.EventSink` that delegates to `vars.OnEvent` with the shared counter |

#### New Code
```go
// internal/parallel/executor.go

// EventSink is the parallel executor's narrow callback for per-request events.
// Identical to runner.EventSink in shape (duplicated to avoid an import cycle
// from parallel → runner). The runner adapts between the two.
type EventSink interface {
    RequestStart(requestID, name, method, url, phase, sourceFile string, sourceLine int, waveIndex int)
    RequestEnd(requestID string, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error)
    AssertionResult(requestID, aType, expected, actual string, passed bool)
}

// Added to Config struct:
    EventSink EventSink // nil = no emission
    // NextRequestID is called once per request to allocate a stable ID. When
    // nil, EventSink is not invoked. The runner provides an atomic-counter-
    // backed implementation so IDs stay monotonic across goroutines.
    NextRequestID func() string
```

Inside `executeOneRequest`:
```go
var reqID string
if cfg.EventSink != nil && cfg.NextRequestID != nil {
    reqID = cfg.NextRequestID()
    cfg.EventSink.RequestStart(reqID, item.Name, req.Method, req.URL, "main",
        item.SourceFile, item.SourceLine, waveIdx)
}
// ... existing exec & assertion ...
if cfg.EventSink != nil && reqID != "" {
    if ar != nil {
        for _, a := range ar.Items {
            cfg.EventSink.AssertionResult(reqID, a.Type, a.Expected, a.Actual, a.Passed)
        }
    }
    cfg.EventSink.RequestEnd(reqID, outcomeStr, statusCode, duration, waveIdx, reqBody, respBody, execErr)
}
```

The runner synthesises a parallel-shaped sink from its own `vars.OnEvent`:
```go
// In runner.executeParallelMain:
var pSink parallel.EventSink
if vars.OnEvent != nil {
    pSink = &parallelSinkAdapter{inner: vars.OnEvent}
}
parallel.Config{
    ...
    EventSink:     pSink,
    NextRequestID: vars.nextRequestID,
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/parallel/executor_test.go
func TestExecuteWaves_EventSink_MonotonicIDsAcrossGoroutines(t *testing.T) {
    // 3 independent items execute in one wave; assert EventSink receives
    // 3 RequestStart + 3 RequestEnd with distinct, monotonic-from-caller IDs
    // and wave_index == 0.
}
func TestExecuteWaves_EventSink_WaveIndexPropagates(t *testing.T) {
    // Diamond: A → (B, C) → D. Assert B and C ends carry WaveIndex=1 and D carries WaveIndex=2.
}
```

#### Impact on Existing Tests
- `TestExecuteWaves_*` — unaffected (all set `EventSink: nil`).
- `executeParallelMain` in runner — receives two new Config fields, both nil for tests that don't opt in.

---

### Step 4: Build the events emitter adapter in `cmd/curlew/main.go`
**Rationale:** Central wiring point that converts `runner.EventSink` calls into concrete `events.Emitter` writes. Depends on Step 1–3 being in place so the method surface is stable. No flag parsing yet — this step builds the internal adapter type and a small helper that constructs it from a writer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `eventsAdapter` type (unexported) satisfying `runner.EventSink`; add `newEventsAdapter(em *events.Emitter) *eventsAdapter` |
| `cmd/curlew/main_test.go` | modify | Add `TestEventsAdapter_WiresToEmitter` covering RequestStart/End/Assertion |

#### New Code (shape)
```go
// cmd/curlew/main.go

// eventsAdapter implements runner.EventSink by forwarding to an events.Emitter.
// Emission errors are logged to stderr (non-fatal) — the run must still
// complete even if the events file writer stalls.
type eventsAdapter struct {
    em       *events.Emitter
    errOut   io.Writer // typically os.Stderr; injectable for tests
}

func newEventsAdapter(em *events.Emitter, errOut io.Writer) *eventsAdapter {
    return &eventsAdapter{em: em, errOut: errOut}
}

func (a *eventsAdapter) RequestStart(e runner.RequestEvent) {
    if err := a.em.EmitRequestStart(e.RequestID, e.Name, e.Method, e.URL, e.Phase, e.SourceFile, e.SourceLine); err != nil {
        fmt.Fprintf(a.errOut, "events: emit request.start: %v\n", err)
    }
}

func (a *eventsAdapter) RequestEnd(e runner.RequestEndEvent) {
    in := events.RequestEndInput{
        RequestID:    e.RequestID,
        Outcome:      events.Outcome(e.Outcome),
        StatusCode:   e.StatusCode,
        Duration:     e.Duration,
        WaveIndex:    e.WaveIndex,
        RequestBody:  e.RequestBody,
        ResponseBody: e.ResponseBody,
        Err:          e.Err,
    }
    if err := a.em.EmitRequestEnd(in); err != nil {
        fmt.Fprintf(a.errOut, "events: emit request.end: %v\n", err)
    }
}

func (a *eventsAdapter) AssertionResult(e runner.AssertionEvent) {
    if err := a.em.EmitAssertionResult(e.RequestID, e.Type, e.Expected, e.Actual, e.Passed); err != nil {
        fmt.Fprintf(a.errOut, "events: emit assertion.result: %v\n", err)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestEventsAdapter_WiresToEmitter(t *testing.T) {
    var buf bytes.Buffer
    em, err := events.NewEmitter(&buf, events.Options{CurlewVersion: "test", RunID: "r"})
    if err != nil { t.Fatal(err) }
    a := newEventsAdapter(em, io.Discard)
    a.RequestStart(runner.RequestEvent{RequestID: "req-1", Name: "ping", Method: "GET", URL: "https://x"})
    a.AssertionResult(runner.AssertionEvent{RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true})
    a.RequestEnd(runner.RequestEndEvent{RequestID: "req-1", Outcome: "passed", StatusCode: 200, Duration: 5 * time.Millisecond})
    // Parse the buffer as three NDJSON lines; verify kinds.
}
```

#### Impact on Existing Tests
- None — purely additive symbol.

---

### Step 5: Add `--events` flag parsing and pre-run validation
**Rationale:** With the adapter in place, the flag now has a concrete thing to wire up. Validation happens before parser/environment loading so bad paths fail in ≤ one syscall.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `events string` field to `runFlags`; parse `--events <path>` in `parseRunArgs` |
| `cmd/curlew/main_test.go` | modify | Extend `TestParseRunArgs` table with events cases; add `TestRunCmd_EventsFlag_UnwritablePath` |

#### New Code
```go
// runFlags additions:
    events string // --events <path>; empty = disabled

// parseRunArgs addition:
    case "--events":
        i++
        if i >= len(args) {
            return errorf("--events requires a file path (e.g. --events /tmp/events.jsonl)")
        }
        f.events = args[i]
```

#### Tests to Write FIRST (RED phase)

```go
// Add to TestParseRunArgs tests table:
{"events_flag", []string{"col.yaml", "--events", "/tmp/x.jsonl"}, ...},
{"events_without_value", []string{"col.yaml", "--events"}, ..., wantErr: true},

// New test driving the real runCmd path:
func TestRunCmd_EventsFlag_UnwritablePath(t *testing.T) {
    tmp := t.TempDir()
    // Write a collection file first.
    col := filepath.Join(tmp, "c.yaml")
    os.WriteFile(col, []byte("name: t\nrequests:\n  - name: p\n    request:\n      method: GET\n      url: https://example.com\n"), 0o600)
    // /does-not-exist/events.jsonl -> parent does not exist.
    stdout, stderr, code := captureRunCmd(t, col, "--events", "/does-not-exist/nope/events.jsonl")
    _ = stdout
    if code != 1 {
        t.Errorf("want exit 1, got %d (stderr: %s)", code, stderr)
    }
    // Ensure no requests were attempted (stderr mentions "events file"; runtime sanity)
}
```

#### Impact on Existing Tests
- `TestParseRunArgs` — new rows added; existing rows unchanged.
- Usage help string in `runCmdInner` updated to include `[--events <path>]` — no snapshot golden to update.

---

### Step 6: Wire emitter construction, `run.start` / `run.end` / `run.error` into `runCmdInner`
**Rationale:** With flag + adapter + runner sink in place, this step opens the file, constructs `events.NewEmitter`, plumbs the adapter into `runner.VarSources.OnEvent`, and emits the three frame events at the right spots. This completes the happy path end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Open events file early; build emitter; emit `run.start` before runner dispatch; emit `run.error` on every pre-run/runner-returned structured error branch; emit `run.end` at every exit path; close emitter via `defer` |
| `cmd/curlew/run_test.go` | modify | Add `TestRunCmd_Events_HappyPath` and `TestRunCmd_Events_UndefinedVariable_EmitsRunError` using `captureRunCmd` and parsing the NDJSON file |
| `cmd/curlew/main_test.go` | modify | Add a `runWithEventsBinary` e2e test that builds the real binary and asserts the events stream |
| `cmd/curlew/main.go` | modify | Usage help line adds `[--events <path>]` |

#### New Code (shape — inserted into `runCmdInner` early, after `parseRunArgs` succeeds)
```go
// --- Events emitter setup (M6-005) ---
var (
    eventsEmitter *events.Emitter
    eventsFile    io.Closer
    eventsSink    runner.EventSink
    runExitCode   int
)
if flags.events != "" {
    f, fErr := os.OpenFile(flags.events, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
    if fErr != nil {
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, flags.noColor))
        errOut.StructuredError(fmt.Errorf("cannot open --events file %q: %w", flags.events, fErr))
        return 1, nil
    }
    eventsFile = f
    em, emErr := events.NewEmitter(f, events.Options{CurlewVersion: version})
    if emErr != nil {
        _ = f.Close()
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, flags.noColor))
        errOut.StructuredError(fmt.Errorf("cannot initialise events emitter: %w", emErr))
        return 1, nil
    }
    eventsEmitter = em
    eventsSink = newEventsAdapter(em, os.Stderr)
    defer func() { _ = eventsFile.Close() }()
}

// Emit run.start as soon as possible (after we know we will attempt a run).
if eventsEmitter != nil {
    _ = eventsEmitter.EmitRunStart(redactedCLIArgs(args), flags.file, flags.envName)
}

// Defer the run.end emission until every exit path has been chosen.
// Summary/totals/exit_code are captured via a closure over named return value.
summary := (*runner.Summary)(nil)
defer func() {
    if eventsEmitter == nil {
        return
    }
    total, passed, failed, skipped := 0, 0, 0, 0
    if summary != nil {
        total = summary.Total
        passed = summary.Passed
        failed = summary.Failed
        skipped = summary.Skipped
    }
    _ = eventsEmitter.EmitRunEnd(total, passed, failed, skipped, runExitCode)
}()
```

For every existing `return N, ...` branch that carries a structured pre-run error (parser failure, env load failure, feature gate, vault config, team template), add:
```go
if eventsEmitter != nil {
    _ = eventsEmitter.EmitRunError(err) // classifyForEvent handles category/code/hint
}
```

And pass the sink into the runner:
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    ...
    OnEvent: eventsSink,
})
```

`redactedCLIArgs` produces a shallow copy with values of `--var`/`--env-var` replaced by `<redacted>` (arguments of these flags carry secrets). This is a simple loop — not table-driven.

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_Events_HappyPath(t *testing.T) {
    srv := httptest.NewServer(/* returns 200 */)
    defer srv.Close()
    tmp := t.TempDir()
    col := filepath.Join(tmp, "happy.yaml")
    events := filepath.Join(tmp, "events.jsonl")
    os.WriteFile(col, ..., 0o600)
    _, _, code := captureRunCmd(t, col, "--events", events)
    if code != 0 { t.Fatal(code) }
    lines := readJSONL(t, events)
    wantKinds := []string{"run.start", "request.start", "assertion.result", "request.end", "run.end"}
    for i, want := range wantKinds {
        if got, _ := lines[i]["kind"].(string); got != want {
            t.Errorf("line %d kind = %q, want %q", i, got, want)
        }
    }
    if lc, _ := lines[len(lines)-1]["event_count"].(float64); int(lc) != len(lines) {
        t.Errorf("event_count (%v) != line count (%d)", lc, len(lines))
    }
}

func TestRunCmd_Events_UndefinedVariable_EmitsRunError(t *testing.T) {
    tmp := t.TempDir()
    col := filepath.Join(tmp, "fail.yaml")
    events := filepath.Join(tmp, "events.jsonl")
    os.WriteFile(col, []byte(`name: u
requests:
  - name: needs-var
    request:
      method: GET
      url: "{{MISSING}}"
`), 0o600)
    _, _, code := captureRunCmd(t, col, "--events", events)
    if code == 0 { t.Fatal("want non-zero exit") }
    lines := readJSONL(t, events)
    // expect: run.start, run.error (with category/code/hint), run.end
    if lines[1]["kind"] != "run.error" {
        t.Errorf("want run.error at line 1, got %v", lines[1]["kind"])
    }
    errObj, _ := lines[1]["error"].(map[string]any)
    if errObj == nil || errObj["category"] == "" || errObj["code"] == "" {
        t.Errorf("run.error missing error fields: %v", errObj)
    }
    // run.end is final, carries exit_code matching the CLI exit.
    last := lines[len(lines)-1]
    if last["kind"] != "run.end" { t.Errorf("last kind = %v", last["kind"]) }
    if ec, _ := last["exit_code"].(float64); int(ec) != code {
        t.Errorf("run.end.exit_code %v != cli code %d", ec, code)
    }
}

// E2E binary test
func TestBinary_Run_EventsFlag(t *testing.T) {
    bin := buildBinary(t)
    srv := httptest.NewServer(/* status 200 */); defer srv.Close()
    dir := t.TempDir()
    col := filepath.Join(dir, "c.yaml"); os.WriteFile(col, ..., 0o600)
    ev := filepath.Join(dir, "events.jsonl")
    _, _, code := runBinary(t, bin, "run", col, "--events", ev)
    if code != 0 { t.Fatal(code) }
    data, _ := os.ReadFile(ev)
    // Assert line count > 0, last line is run.end, all lines parse as JSON.
}
```

#### Impact on Existing Tests
- No existing test provides `--events`; all existing goldens remain byte-identical.
- Existing structured-error emission paths (gate, parse, env-load, team-template) each gain a call to `eventsEmitter.EmitRunError(err)` guarded by `if eventsEmitter != nil` — zero-cost when the flag is absent.

---

### Step 7: Reject `--events` on non-run subcommands
**Rationale:** Smallest-blast-radius change of the lot; independent of steps 1–6. Done after the main wiring so tests can assert the rejection message stays stable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/worker.go` | modify | In `parseWorkerArgs`, match `--events` explicitly and return rejection error |
| `cmd/curlew/perf.go` | modify | In `parsePerfArgs`, match `--events` explicitly and return rejection error |
| `cmd/curlew/main.go` | modify | In `parsePrCheckArgs`, match `--events` explicitly and return rejection error |
| `cmd/curlew/worker_test.go`, `perf_test.go`, `main_test.go` (prcheck) | modify | Add `TestXxx_EventsFlagRejected` for each |

#### New Code (identical in each parser, slight message variation: mention the subcommand name)
```go
case "--events":
    return cfg, false, fmt.Errorf("--events is supported only on run; use --format jsonl for streaming samples")
```

For `perf` the natural message mentions `--output` as the streaming alternative. We settle on the exact message from the task yaml: `--events is supported only on run; use --format jsonl for streaming samples`.

#### Tests to Write FIRST (RED phase)

```go
func TestWorkerCmd_EventsFlagRejected(t *testing.T) {
    code := workerCmd([]string{"--events", "/tmp/x"})
    if code != 1 { t.Errorf("want exit 1, got %d", code) }
}
func TestPerfCmd_EventsFlagRejected(t *testing.T) { /* want exit 2 — perf returns 2 on parse error */ }
func TestPrCheckCmd_EventsFlagRejected(t *testing.T) { /* want exit 2 */ }
```

#### Impact on Existing Tests
- None — `--events` is a new flag on those subcommands, previously falling into the `default: unknown flag` branch. The new case has the same effect (non-zero exit) but with a specific message.

---

### Step 8: Smoke test + CHANGELOG + docs
**Rationale:** Cross-cutting finishing touches. Runs after all behavior is in place so the smoke test exercises the real binary.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | New section: run a happy collection with `--events /tmp/curlew_smoke_events.jsonl`; assert first line is `run.start`, last line is `run.end`, `event_count` matches line count |
| `CHANGELOG.md` | modify | Add M6-005 entry under Unreleased/Added |
| `docs/SPECIFICATION.md` | review only | No changes expected — M6 events surface already documented via `docs/events-schema/v0.1.json`. Confirm nothing in SPEC contradicts the new flag. |

#### Smoke test excerpt
```bash
echo "--- Events NDJSON stream (--events happy path) ---"
EVENTS_COL=$(mktemp /tmp/curlew_events_col_XXXXXX.yaml)
EVENTS_OUT=$(mktemp /tmp/curlew_events_out_XXXXXX.jsonl)
cat > "$EVENTS_COL" << 'YAML'
name: events-smoke
requests:
  - name: ping
    request:
      method: GET
      url: https://httpbin.org/get
    assertions:
      status: 200
YAML
./curlew run "$EVENTS_COL" --events "$EVENTS_OUT"
HEAD_KIND=$(head -n 1 "$EVENTS_OUT" | jq -r .kind)
TAIL_KIND=$(tail -n 1 "$EVENTS_OUT" | jq -r .kind)
LINE_COUNT=$(wc -l < "$EVENTS_OUT" | tr -d ' ')
EVENT_COUNT=$(tail -n 1 "$EVENTS_OUT" | jq -r .event_count)
[[ "$HEAD_KIND" == "run.start" ]] || { echo "FAIL: first line kind = $HEAD_KIND"; exit 1; }
[[ "$TAIL_KIND" == "run.end" ]]   || { echo "FAIL: last line kind = $TAIL_KIND"; exit 1; }
[[ "$LINE_COUNT" == "$EVENT_COUNT" ]] || { echo "FAIL: line count $LINE_COUNT != event_count $EVENT_COUNT"; exit 1; }
rm -f "$EVENTS_COL" "$EVENTS_OUT"
echo "PASS: --events stream shape"
```

(Requires `jq` — already used elsewhere in `smoke/run.sh`; confirm that before merge.)

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| `internal/runner/runner_test.go` | `TestRun_WithHooksDispatcher_*` | none | verify OnEvent:nil path still matches |
| `internal/runner/runner_test.go` | `TestRunner_RequestResultCarriesSourceLocation` | none | source plumbing untouched |
| `internal/runner/runner_test.go` | `TestRun` table | none | `VarSources{}` zero-value includes nil OnEvent |
| `internal/parallel/executor_test.go` | `TestExecuteWaves_*` | none | all existing cases leave EventSink nil |
| `cmd/curlew/main_test.go` | `TestParseRunArgs` | extend | add `--events` rows |
| `cmd/curlew/run_test.go` | existing run tests | none | no events flag, no events file created |
| all `internal/output/events` tests | all | none | emitter package unchanged |

## Risks and Edge Cases

- **Risk: Undefined-variable error's category is `config`, not `input`.**
  The task observable in `management/tasks/M6-005.yaml` hints at `category=input` for the `{{MISSING}}` example, but the actual classification (via `variable.Interpolate` → `apierrors.Structured{Category: CategoryConfig}`) is `config`. The `behaviors` block lists "file, line, category, code, hint" without locking a specific category.
  **Mitigation:** The plan follows the existing classification. The `run.error` tests assert *presence* of `category`/`code`/`hint`/`file`/`line`, not a specific category value. The observable command will still show a valid `run.error` event with `category=config` and `code=VAR_UNDEFINED`. This matches the schema (`config` is in the enum) and preserves the invariant that classification is centralised in `internal/errors`.

- **Risk: Events file interleaving when path is `/dev/stdout` on systems where existing output formatters also write to stdout.**
  **Mitigation:** The spec permits this ("NDJSON lines are written to stdout interleaved with any existing stdout output"). We add a smoke/test case confirming the file path form works; `/dev/stdout` is already tested implicitly by Unix conventions. No locking is introduced between the NDJSON writer and the format printers — interleaving is expected and documented.

- **Risk: Partial events file written then overwritten on retry.**
  `O_TRUNC` ensures a fresh file on each run; we never append. If the emitter fails mid-run (disk full), later `Emit*` calls still propagate errors to stderr but do not abort the run. The `run.end` line may be missing — consumers are already required to detect this condition per the schema.
  **Mitigation:** Documented; no code change.

- **Risk: High event volume for large collections with parallel execution creates bottleneck on emitter mutex.**
  **Mitigation:** The emitter already uses an atomic counter for `id` and a mutex only around the write syscall. `TestEmitter_ConcurrentEmitMonotonicIDs` covers 200 concurrent writes. No additional mitigation required for M6-005.

- **Edge case: Data-driven items produce one ID per iteration.**
  **Handling:** `executeDataDriven` allocates a fresh `reqID` per iteration via `vars.nextRequestID()`, same as the sequential path. The existing `TestRunner_DataDrivenIterationsCarrySourceLocation` is a template for a new test `TestRun_EventSink_DataDriven_EachIterationHasUniqueRequestID`.

- **Edge case: WebSocket items emit one pair of events (request.start / request.end) for the whole session.**
  **Handling:** The WS branch already produces a single `RequestResult`; treat it identically to HTTP. `outcome="passed"` or `"error"` based on `wsRes.Passed`. `status_code=101` is emitted as-is.

- **Edge case: Teardown phase items emit events with `phase="teardown"`.**
  **Handling:** `RequestEvent.Phase` is populated with `string(phase)` (`setup` / `main` / `teardown`). The schema already accepts any string for `phase`.

- **Edge case: Pre-run error AFTER run.start but BEFORE any request.start (e.g., feature gate in `buildScope`).**
  **Handling:** The deferred `run.end` emission still fires; the error path emits `run.error` in between. Expected stream: `run.start` → `run.error` → `run.end`. Verified by `TestRunCmd_Events_UndefinedVariable_EmitsRunError`.

- **Edge case: `CURLEW_VAULT_STUB=1` + shared template with `{{secrets.X}}` and `--events`.**
  **Handling:** Shared secrets are already redacted at the `RedactBody` stage; event bodies flow through the same redaction. Covered by passing the same redacted `results[i].RequestBody` / `results[i].Result.Body` to the emitter. The emission happens *before* redaction in the sequential path (redaction runs after `runner.Run`). **This is a bug seam.** Mitigation: emit the pre-redaction body is wrong for secrets; instead, run redaction on-the-fly in the adapter OR pipe the request/response body through the same sensitive-set redaction before handing to `EmitRequestEnd`. See Risk below.

- **Risk: Request/response bodies in events can leak sensitive values because emission happens during execution, BEFORE the redaction pass in `runCmdInner`.**
  **Mitigation:** Two options:
  1. (Chosen) Defer body emission until after redaction by having the sequential/parallel emission paths emit `RequestStart` immediately but defer `RequestEnd` body fields. This requires invasive refactoring and breaks the "streaming" contract.
  2. (Chosen) Perform redaction inside the adapter: the adapter holds a reference to the `*variable.SensitiveSet` (built once after parse, before runner.Run) plus `allowSensitive`. Before `EmitRequestEnd`, the adapter calls `variable.RedactBody` on request/response bytes. This preserves streaming and keeps redaction centralised.
  **Decision:** Option 2. The plan includes building the sensitive-set earlier (before `runner.Run`) so the adapter can hold a reference. A new test `TestRunCmd_Events_RedactsSensitiveBodyValues` asserts no sensitive value appears in the emitted events for a `--var API_KEY=...` flow.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (reproduces the task YAML):
```bash
cat > /tmp/events-happy.yaml <<'YAML'
name: happy
requests:
  - name: ping
    method: GET
    url: http://127.0.0.1:18080/ok
    assertions:
      status: 200
YAML
./curlew run /tmp/events-happy.yaml --events /tmp/events.jsonl
wc -l /tmp/events.jsonl

cat > /tmp/events-fail.yaml <<'YAML'
name: unresolved
requests:
  - name: needs-var
    method: GET
    url: "{{MISSING}}"
YAML
./curlew run /tmp/events-fail.yaml --events /tmp/events-fail.jsonl || true
```

Expected after Step 8:
- `/tmp/events.jsonl` — 5 lines: `run.start`, `request.start`, `assertion.result`, `request.end` (`outcome=passed`), `run.end` (with `event_count=5`).
- `/tmp/events-fail.jsonl` — 3 lines: `run.start`, `run.error` (`category=config`, `code=VAR_UNDEFINED`, `file=/tmp/events-fail.yaml`, `line` pointing at the request, non-empty `hint`), `run.end` (`exit_code != 0`).
