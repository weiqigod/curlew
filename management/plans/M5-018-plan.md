# Implementation Plan: M5-018

## Overview

Wire the external-process plugin host (M5-017) into the request/response/result
lifecycle: extend the `internal/plugin` package with a persistent hook-calling
channel, add a new `internal/plugin/hooks` registry/dispatcher that serializes
hook invocations over JSON-RPC, and invoke it from `internal/runner` around
each request and at run completion. Plugins can mutate the outgoing request,
annotate the response, and observe the summary with a hard per-hook timeout
and plugin-ordered chaining.

## Task Details

- **ID:** M5-018
- **Title:** go-cli: plugin hook registry (request/response/result lifecycle)
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high
- **Estimated effort:** 8-12 hours

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-017 | go-cli plugin interface + loader (external-process model) | done |

## Key Architectural Decisions

1. **Persistent plugin process per run.** M5-017's `Host.Load` spawned plugins
   only long enough to do the handshake. M5-018 needs plugins alive for the
   entire run so they can receive hook invocations. We extend `Host` with a
   new entry point `LoadForRun` (or extend `Load`) that, after the successful
   handshake, keeps the plugin process alive, tracks (stdin, stdout-reader,
   kill-func) in `h.running`, and returns a `*Dispatcher`. `Close()` (already
   present as no-op) iterates `h.running` to stop them.

2. **New `on_result` hook.** The task introduces a third hook. We extend
   `knownHooks` in `plugin.go` to include `on_result` and document it in
   `docs/plugins.md`. This is forward-compatible: plugins built against M5-017
   continue to work; plugins that declare `on_result` are new.

3. **Hook dispatcher as a separate subpackage.** Per the task scope, the
   invocation machinery lives in `internal/plugin/hooks`. This keeps the
   loader (`internal/plugin`) narrowly focused on discovery/handshake and lets
   `hooks` own the runtime wiring. `hooks` imports `plugin` (for `Plugin`
   type) but not vice versa, avoiding cycles. The `hooks` subpackage exports
   a `Registry` and `Dispatcher`.

4. **Blocking dispatch with per-call goroutine + channel for timeout.** Each
   plugin hook invocation runs on a goroutine so the 10-second timeout can be
   enforced by `select` on a `context.WithTimeout` deadline. On timeout we
   kill the plugin process (the host's stored `kill` func) and remove it from
   the live registry so it is treated as if no hook were registered.

5. **JSON-RPC `call` method names.** The wire protocol uses JSON-RPC 2.0
   requests (not notifications — the task's wording "notifications" is at the
   conceptual level; we need a return value to support mutation). Method
   names: `apitest/on_request`, `apitest/on_response`, `apitest/on_result`.
   Each uses a monotonically increasing ID per plugin. This mirrors the
   M5-017 handshake pattern.

6. **Request mutation via return value on `on_request`.** The plugin returns
   a JSON object with optional `method`, `url`, `headers`, `body`. Only
   present fields overwrite the request; absent fields keep the original
   value. `headers` is a full replacement (not merge) — matches the way
   Postman-equivalents typically handle pre-request scripts. We document
   this explicitly.

7. **Response annotation (not replacement) for `on_response`.** The return
   value is permitted to carry an `annotations` map that attaches arbitrary
   k/v pairs to `RequestResult.Warnings` (or a new `RequestResult.Annotations`
   slot if cleaner — we use `Warnings` for simplicity since it is already
   printed). Plugins cannot rewrite status/body/headers. This enforces
   "annotate not replace".

8. **Execution-ordered chaining.** Plugins are invoked in the order that
   `Host.LoadForRun` returns, which is the order they appear in
   `APITEST_PLUGINS` after directory expansion — matching the existing
   loader's behaviour. The chained value (post-hook request for
   `on_request`, annotations for `on_response`) is passed into the next
   plugin's hook.

9. **Error vs. timeout distinction.** A plugin returning a JSON-RPC error
   object for `on_request` aborts the request with `RequestResult.Err` set
   and the run marks the request as error (categorised via a new sentinel
   `ErrPluginHookAborted` wrapping the plugin's error message). A timeout
   does NOT abort — it warns, unregisters the hook, and continues. This
   matches the acceptance criteria precisely.

10. **Wiring point: `ExecuteFunc` wrapper.** We wrap `exec` inside
    `runner.Run` (same pattern as the rate-limiter wrap at line 237) with
    a hook-aware function: before calling `exec`, run `OnRequest`; after it
    returns, run `OnResponse`. This avoids modifying the `executePhase` /
    parallel / data-driven / websocket paths. We decline to run hooks around
    WebSocket (since its result is synthesised) — documented as a known
    limitation for this task. `on_result` fires once from `runner.Run` after
    `runPhases` returns, with the finished Summary.

11. **Dispatcher passed through `VarSources`.** We add a new field
    `vars.Hooks *hooks.Dispatcher`. When non-nil, `Run` wraps `exec` and
    calls `OnResult`. When nil (all existing tests, and runs without
    plugins), behaviour is unchanged. This is a minimal-blast-radius wiring
    change.

12. **Timeout: 10 seconds hard-coded.** The task fixes this value. We expose
    it as an exported constant for testability and keep the default.

13. **No new help flags.** The task's help-text behaviour is a new "Plugin
    hooks" section in `apitest run --help` that enumerates the three hooks
    and the APITEST_PLUGINS env var. We extend the existing `printHelp()`
    block in `cmd/apitest/main.go` (currently "Plugins (Enterprise tier):").

14. **New smoke-test plugin fixture.** We add
    `testdata/plugins/hooklog-plugin/main.go` — a long-running plugin that
    responds to hello + three hook methods by printing one line to stderr
    per hook and returning an identity response. Smoke test builds it and
    runs the observable command from the task YAML.

## Implementation Steps

Ordered by blast radius (smallest first):

### Step 1: Extend `internal/plugin` to support `on_result` in the known-hook set

**Rationale:** Safe, isolated change. Adds a new entry to `knownHooks`; no
existing test relies on `on_result` being filtered out. Unblocks plugins to
declare it without warning.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/plugin.go` | modify | Add `"on_result": {}` to `knownHooks` |
| `internal/plugin/host_test.go` | modify | Extend existing hook-filtering test to cover `on_result` |

#### Current Code

```go
// internal/plugin/plugin.go
var knownHooks = map[string]struct{}{
    "on_request":  {},
    "on_response": {},
}
```

#### New Code

```go
var knownHooks = map[string]struct{}{
    "on_request":  {},
    "on_response": {},
    "on_result":   {},
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestHost_Load_FiltersKnownHooks(t *testing.T) {
    tests := []struct {
        name      string
        hooks     []string
        wantHooks []string
        wantWarn  bool
    }{
        {"all three known", []string{"on_request", "on_response", "on_result"},
            []string{"on_request", "on_response", "on_result"}, false},
        {"on_result alone", []string{"on_result"}, []string{"on_result"}, false},
        {"unknown mixed with on_result", []string{"on_result", "on_magic"},
            []string{"on_result"}, true},
    }
    // ... drive through host.Load via fakeSpawner and assert p.Hooks + warnings
}
```

#### Impact on Existing Tests

- `TestHost_Load_UnknownHook` in `host_test.go`: still passes (validates the
  warning for unknown hooks; `on_result` is known).
- `fakeplugin_test.go::echoHello` default response still only lists
  `on_request`, `on_response`; no change needed.

---

### Step 2: Add persistent-channel support to `internal/plugin.Host`

**Rationale:** The host currently spawns-and-drops each plugin. To dispatch
hooks we need a live channel back to each plugin. This step extends `Host`
with a `LoadForRun` method (or adds a `Channel` return from the existing
path) — we pick `LoadForRun` so the list command's simpler semantics are
preserved and `plugins list` keeps exiting right after the handshake.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/host.go` | modify | Add `LoadForRun` method; keep stdin + stdout reader alive and populate `h.running` |
| `internal/plugin/channel.go` | create | New `Channel` type wrapping one plugin's (stdin, stdout reader, kill, mu, seqID); `Call(ctx, method, params)` method |
| `internal/plugin/channel_test.go` | create | Unit tests for `Channel.Call` (success, plugin-error, timeout, process exited) |

#### New `Channel` sketch

```go
// internal/plugin/channel.go
package plugin

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "sync"
)

// Channel is a live JSON-RPC duplex to a single running plugin.
// Call serializes requests (one at a time; the wire protocol is
// request-response and plugins handle one call at a time).
type Channel struct {
    name   string // plugin name (for diagnostics)
    stdin  io.WriteCloser
    stdout *bufio.Reader
    kill   func()
    mu     sync.Mutex // serialises Call
    seq    int        // next request id
    closed bool
}

// Call sends a JSON-RPC request and blocks until the response or ctx.Done.
// If ctx expires first the channel is closed (kill() invoked) and
// ErrCallTimeout is returned. When the plugin returns a JSON-RPC error the
// error message is wrapped in ErrCallPluginError.
func (c *Channel) Call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error)

// Close terminates the plugin process. Safe to call multiple times.
func (c *Channel) Close() error
```

#### New `Host.LoadForRun` sketch

```go
// LoadForRun performs the same handshake as Load but keeps each
// loaded plugin's stdio channel alive. The returned channels parallel
// the returned plugins slice (same index, same order).
func (h *Host) LoadForRun(ctx context.Context, pluginsEnv string) ([]Plugin, []*Channel, []LoadError, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestChannel_Call_RoundTrips(t *testing.T) { /* echo plugin returning params */ }
func TestChannel_Call_TimesOut(t *testing.T) { /* ctx deadline; kill invoked; ErrCallTimeout */ }
func TestChannel_Call_PluginErrorResponse(t *testing.T) { /* rpcError -> ErrCallPluginError */ }
func TestChannel_Call_ProcessExited(t *testing.T) { /* stdout EOF -> wrapped ErrChannelClosed */ }
func TestChannel_Close_Idempotent(t *testing.T) { /* second Close() doesn't panic */ }

func TestHost_LoadForRun_KeepsChannelsAlive(t *testing.T) {
    // Use fakeSpawner with a plugin that answers hello and then
    // a second method; assert the same channel can make a second call.
}

func TestHost_LoadForRun_ReturnsChannelsParallelToPlugins(t *testing.T) {
    // Two plugins: asserts channels[i] matches plugins[i] by name.
}

func TestHost_Close_TerminatesRunningChannels(t *testing.T) {
    // After LoadForRun + Close, spawner's kill func was invoked.
}
```

#### Impact on Existing Tests

- No existing test references `LoadForRun`. `Load` remains behaviourally
  unchanged (still spawns-and-kills per handshake via `defer kill()`).
- `fakePlugin.handler` currently answers exactly one request then exits.
  For channel tests we extend `fakeplugin_test.go` with a new
  `fakePlugin.loopHandler` that reads requests in a loop until EOF.

---

### Step 3: Create `internal/plugin/hooks` subpackage with `Registry` and `Dispatcher`

**Rationale:** This is the new API surface the runner wires into. Built on
top of Step 2's `Channel`, it has no coupling to `runner` or `httpexec`, so
it can be fully tested in isolation first.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/plugin/hooks/hooks.go` | create | `Registry`, `Dispatcher`, payload types, sentinels |
| `internal/plugin/hooks/hooks_test.go` | create | Table-driven tests for all behaviours |
| `internal/plugin/hooks/dispatch_test.go` | create | Ordering, timeout, error-propagation tests |

#### Proposed API

```go
// Package hooks dispatches lifecycle events to loaded plugins.
package hooks

// HookTimeout is the hard per-hook wall-clock limit (10s). Exposed for tests.
const HookTimeout = 10 * time.Second

// Sentinel errors.
var (
    // ErrHookAborted is returned from OnRequest when a plugin's hook
    // returns a JSON-RPC error. The caller should mark the request as
    // error (not fail). Wrapped with fmt.Errorf so errors.Is works.
    ErrHookAborted = errors.New("plugin hook aborted request")
)

// Registration associates a plugin with its live channel and the hooks it declared.
type Registration struct {
    Plugin  plugin.Plugin
    Channel *plugin.Channel
}

// Dispatcher runs the registered hooks for each lifecycle event.
// Construct via NewDispatcher. Safe for concurrent use across goroutines.
type Dispatcher struct {
    stderr io.Writer
    regs   []Registration

    // Per-hook live list — plugins that timed out are dropped.
    onRequest  []*Registration
    onResponse []*Registration
    onResult   []*Registration
    mu         sync.Mutex
}

// NewDispatcher builds a Dispatcher from the successfully loaded plugins.
// Plugins are invoked in slice order; stderr receives timeout warnings.
func NewDispatcher(stderr io.Writer, regs []Registration) *Dispatcher

// RequestPayload is the serialised input to on_request / output that may
// mutate the next stage. Fields use JSON-friendly types.
type RequestPayload struct {
    Method      string            `json:"method"`
    URL         string            `json:"url"`
    Headers     map[string]string `json:"headers,omitempty"`
    Body        any               `json:"body,omitempty"`
    QueryParams map[string]string `json:"query_params,omitempty"`
}

// OnRequest is called before Execute. Each plugin may mutate the request;
// the returned RequestPayload is chained into the next plugin. A JSON-RPC
// error from any plugin is returned with ErrHookAborted in the error chain;
// timeouts drop the plugin and continue.
func (d *Dispatcher) OnRequest(ctx context.Context, req RequestPayload) (RequestPayload, error)

// ResponsePayload is the serialised input to on_response. Status/Headers/Body
// are read-only; annotations accumulate as the payload flows between plugins.
type ResponsePayload struct {
    StatusCode  int               `json:"status_code"`
    Headers     map[string]string `json:"headers,omitempty"`
    Body        json.RawMessage   `json:"body,omitempty"`
    DurationMs  int64             `json:"duration_ms"`
    Annotations []string          `json:"annotations,omitempty"`
}

// OnResponse is called after Execute. Plugins may append to Annotations;
// status/body/headers are never replaced. Timeouts drop the plugin.
// Never returns an abort error (matches acceptance: annotate not replace).
func (d *Dispatcher) OnResponse(ctx context.Context, resp ResponsePayload) (ResponsePayload, error)

// ResultPayload is the serialised input to on_result (fired once at run end).
type ResultPayload struct {
    PassCount  int              `json:"pass_count"`
    FailCount  int              `json:"fail_count"`
    SkipCount  int              `json:"skip_count"`
    DurationMs int64            `json:"duration_ms"`
    Tests      []ResultTestRow  `json:"tests,omitempty"`
}

type ResultTestRow struct {
    Name       string `json:"name"`
    Status     string `json:"status"` // "pass" | "fail" | "skip" | "error"
    DurationMs int64  `json:"duration_ms"`
    Error      string `json:"error,omitempty"`
}

// OnResult fires once at run completion. Errors are best-effort (logged to
// stderr); the run's exit code is unaffected.
func (d *Dispatcher) OnResult(ctx context.Context, res ResultPayload) error

// Close terminates all plugin processes. Safe to call multiple times.
func (d *Dispatcher) Close() error
```

#### Tests to Write FIRST (RED phase)

```go
func TestDispatcher_OnRequest(t *testing.T) {
    tests := []struct{
        name     string
        plugins  []fakeHookPlugin
        in       RequestPayload
        want     RequestPayload
        wantErr  error
    }{
        {"no plugins registered passes through", ...},
        {"single plugin mutates headers", ...},
        {"two plugins chain in declared order", ...},
        {"plugin that does not declare on_request is skipped", ...},
        {"plugin returns JSON-RPC error -> ErrHookAborted", ...},
        {"plugin exceeds 10s -> dropped, warning, continues", ...},
        {"second plugin runs after first times out", ...},
    }
}

func TestDispatcher_OnResponse(t *testing.T) {
    tests := []struct{
        name          string
        plugins       []fakeHookPlugin
        in            ResponsePayload
        wantAnnot     []string
    }{
        {"annotations accumulate across plugins", ...},
        {"plugin tries to rewrite status_code: ignored", ...},
        {"timeout drops plugin for subsequent hooks too", ...},
    }
}

func TestDispatcher_OnResult(t *testing.T) {
    tests := []struct{
        name    string
        plugins []fakeHookPlugin
        in      ResultPayload
    }{
        {"pass counts forwarded", ...},
        {"per-test rows forwarded", ...},
        {"plugin error does not propagate (best-effort)", ...},
    }
}

func TestDispatcher_HookTimeout_DropsPluginGlobally(t *testing.T) {
    // on_request times out -> subsequent on_response for same plugin is skipped.
}

func TestDispatcher_Close_KillsAllChannels(t *testing.T) { ... }
```

#### Impact on Existing Tests

- None — this is a new subpackage.

---

### Step 4: Extend runner wiring to invoke hooks around exec and after runPhases

**Rationale:** Now that the dispatcher is tested in isolation, wire it into
the runner. The change is surgical: add a field to `VarSources`, wrap
`exec` inside `Run`, and call `OnResult` after `runPhases` returns.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `Hooks *hooks.Dispatcher` to `VarSources`; wrap `exec`; call `OnResult` |
| `internal/runner/runner_test.go` | modify | New tests covering: hook wraps executor, mutation applies, abort, no-op when Hooks is nil |

#### Current Code

```go
// internal/runner/runner.go:237 (rate-limit wrap)
globalLimiter := ratelimit.New(col.RateLimitRPS)
if globalLimiter != nil {
    unwrapped := exec
    exec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
        if waitErr := globalLimiter.Wait(ctx); waitErr != nil {
            return nil, waitErr
        }
        return unwrapped(ctx, req)
    }
    vars.globalLimiter = globalLimiter
}
```

#### New Code

Add after the rate-limit wrap (so rate-limit fires first, then hooks, then
exec):

```go
// Hook wrap: on_request before exec, on_response after.
// When vars.Hooks is nil (no plugins) this is a no-op by fast path.
if vars.Hooks != nil {
    unwrapped := exec
    exec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
        inPayload := hooks.RequestPayload{
            Method: req.Method, URL: req.URL,
            Headers: req.Headers, Body: req.Body, QueryParams: req.QueryParams,
        }
        outPayload, hErr := vars.Hooks.OnRequest(ctx, inPayload)
        if hErr != nil {
            // Plugin aborted the request: surface as an error.
            return nil, hErr
        }
        // Apply mutations back to the request before sending.
        req.Method = outPayload.Method
        req.URL = outPayload.URL
        req.Headers = outPayload.Headers
        req.Body = outPayload.Body
        req.QueryParams = outPayload.QueryParams

        start := time.Now()
        resp, execErr := unwrapped(ctx, req)
        if execErr != nil {
            return resp, execErr
        }
        respPayload := hooks.ResponsePayload{
            StatusCode: resp.StatusCode,
            Headers:    flattenHeader(resp.Headers),
            Body:       resp.Body,
            DurationMs: time.Since(start).Milliseconds(),
        }
        // We drop the annotation return value here: the annotations
        // are printed by the plugin itself to its own stderr (acceptance
        // test expectation). A follow-up task can thread them into
        // RequestResult.Warnings if needed.
        _, _ = vars.Hooks.OnResponse(ctx, respPayload)
        return resp, nil
    }
}
```

And after `runPhases` returns, before `return`:

```go
if vars.Hooks != nil && summary != nil {
    _ = vars.Hooks.OnResult(ctx, buildResultPayload(results, summary))
}
```

Plus a small helper:

```go
func buildResultPayload(results []RequestResult, summary *Summary) hooks.ResultPayload {
    rows := make([]hooks.ResultTestRow, 0, len(results))
    for _, r := range results {
        status := "pass"
        switch {
        case r.Skipped:
            status = "skip"
        case r.Err != nil:
            status = "error"
        case r.AssertionResults != nil && !r.AssertionResults.Passed:
            status = "fail"
        }
        var dur int64
        if r.Result != nil {
            dur = r.Result.Duration.Milliseconds()
        }
        errStr := ""
        if r.Err != nil {
            errStr = r.Err.Error()
        }
        rows = append(rows, hooks.ResultTestRow{
            Name: r.Name, Status: status, DurationMs: dur, Error: errStr,
        })
    }
    return hooks.ResultPayload{
        PassCount:  summary.Passed,
        FailCount:  summary.Failed,
        SkipCount:  summary.Skipped,
        DurationMs: summary.Duration.Milliseconds(),
        Tests:      rows,
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go

func TestRun_WithHooksDispatcher_InvokesOnRequestBeforeExec(t *testing.T) {
    // Fake dispatcher counts calls; assert order: OnRequest, exec, OnResponse.
}

func TestRun_WithHooksDispatcher_MutationAppliesToExec(t *testing.T) {
    // Dispatcher's OnRequest adds X-Trace header; exec sees it in req.Headers.
}

func TestRun_WithHooksDispatcher_AbortErrorBecomesRequestErr(t *testing.T) {
    // ErrHookAborted propagates; RequestResult.Err wraps it.
}

func TestRun_WithoutHooks_NoWrapOverhead(t *testing.T) {
    // vars.Hooks nil -> exec is called exactly as the input. (behaviour test)
}

func TestRun_WithHooksDispatcher_OnResultFiresOnceWithSummary(t *testing.T) {
    // Fake dispatcher records the last ResultPayload; assert PassCount etc.
}
```

#### Impact on Existing Tests

- All existing runner_test.go tests pass `VarSources{...}` without the
  `Hooks` field — they use the zero value (nil), so the wrapper is skipped.
- No existing test will break.

---

### Step 5: Build the dispatcher in `cmd/apitest` and wire into `run` command

**Rationale:** Wire the real plugin host into the CLI. This is the smallest
change at the highest-impact integration point: where `run` calls
`runner.Run`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/plugins.go` | modify | Add `buildHookDispatcher(ctx context.Context) (*hooks.Dispatcher, func(), error)` helper |
| `cmd/apitest/main.go` | modify | Build dispatcher in `runCmd`, pass to `runner.Run`, defer Close(), update help text |
| `cmd/apitest/main.go` (help) | modify | Add "Plugin hooks" section to `printHelp` listing the three lifecycle hooks |
| `cmd/apitest/plugins_test.go` | modify | Integration test: plugin declaring on_request sees request through the fake spawner |

#### New helper sketch

```go
// buildHookDispatcher loads plugins via pluginsHostFactory, builds a
// hooks.Dispatcher over the live channels, and returns it with a cleanup
// func that terminates all plugin processes. Returns nil dispatcher
// (not an error) when APITEST_PLUGINS is empty.
func buildHookDispatcher(ctx context.Context, stderr io.Writer) (*hooks.Dispatcher, func(), error)
```

#### Help-text addition

```go
fmt.Println("Plugin hooks (Enterprise tier):")
fmt.Println("  Plugins may register three lifecycle hooks, called in declared order:")
fmt.Println("    on_request   — called before each HTTP request is sent; may mutate")
fmt.Println("                    the method/url/headers/body/query_params.")
fmt.Println("    on_response  — called after each response; may attach annotations")
fmt.Println("                    (status/headers/body are not replaceable).")
fmt.Println("    on_result    — called once at run completion with pass/fail counts")
fmt.Println("                    and per-test rows.")
fmt.Println("  Per-hook timeout: 10 seconds. A timed-out plugin is terminated and")
fmt.Println("  the run continues as if the hook were not registered.")
fmt.Println("  Set APITEST_PLUGINS=/path/to/plugin[:...] to enable.")
```

#### Wiring point in `runCmd`

```go
// cmd/apitest/main.go around line 746
hookDispatcher, closeHooks, hookErr := buildHookDispatcher(ctx, os.Stderr)
if hookErr != nil {
    _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", hookErr)
    return 2, nil
}
defer closeHooks()

results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    // ... existing fields ...
    Hooks: hookDispatcher,
})
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/apitest/plugins_test.go

func TestRun_HookPlugin_OnRequestReceivesRequest(t *testing.T) {
    // Fake spawner with hook-aware handler; fake HTTP exec to skip network.
    // Assert handler saw on_request call with correct params.
}

func TestRun_HookPlugin_OnResponseReceivesResponse(t *testing.T) { ... }

func TestRun_HookPlugin_OnResultReceivesSummary(t *testing.T) { ... }

func TestRun_HookPlugin_TimeoutEmitsWarning(t *testing.T) {
    // Stall plugin; assert stderr contains "plugin <name> timed out on <hook>"
    // and run completes successfully.
}

func TestRun_HookPlugin_TwoPluginsChainInOrder(t *testing.T) {
    // Two plugins both on_request; second sees mutation from first.
}

func TestRun_Help_IncludesPluginHooksSection(t *testing.T) {
    stdout, _ := captureOutput(func() int { return run([]string{"--help"}) })
    require.Contains(t, stdout, "Plugin hooks")
    require.Contains(t, stdout, "on_request")
    require.Contains(t, stdout, "on_response")
    require.Contains(t, stdout, "on_result")
}
```

#### Impact on Existing Tests

- No existing `run` test sets `APITEST_PLUGINS`, so `buildHookDispatcher`
  returns `(nil, noop, nil)` and `runner.VarSources.Hooks` is nil — existing
  tests behave unchanged.
- The help-text test on existing commands will need the new section to not
  remove earlier sections; simply appending new text is safe.

---

### Step 6: Add `testdata/plugins/hooklog-plugin` fixture

**Rationale:** The observable command in the task YAML references a
"hooklog" plugin. This fixture is the canonical long-running plugin that
demonstrates all three hooks end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/plugins/hooklog-plugin/main.go` | create | Plugin binary that handshakes then responds to on_request, on_response, on_result, logging one line per hook to stderr |
| `testdata/plugins/one-request.yaml` | create | Minimal collection with one GET request used by smoke test |

#### Fixture sketch

```go
// testdata/plugins/hooklog-plugin/main.go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    r := bufio.NewReader(os.Stdin)
    for {
        line, err := r.ReadBytes('\n')
        if err != nil {
            return
        }
        var req struct {
            JSONRPC string          `json:"jsonrpc"`
            ID      int             `json:"id"`
            Method  string          `json:"method"`
            Params  json.RawMessage `json:"params,omitempty"`
        }
        if err := json.Unmarshal(line, &req); err != nil {
            return
        }
        var result any
        switch req.Method {
        case "apitest/hello":
            result = map[string]any{
                "name":             "hooklog",
                "version":          "0.1.0",
                "hooks":            []string{"on_request", "on_response", "on_result"},
                "protocol_version": 1,
            }
        case "apitest/on_request":
            var p struct {
                Method string `json:"method"`
                URL    string `json:"url"`
            }
            _ = json.Unmarshal(req.Params, &p)
            fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_request %s %s\n", p.Method, p.URL)
            result = map[string]any{} // no mutation
        case "apitest/on_response":
            var p struct {
                StatusCode int   `json:"status_code"`
                DurationMs int64 `json:"duration_ms"`
            }
            _ = json.Unmarshal(req.Params, &p)
            fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_response %d %dms\n", p.StatusCode, p.DurationMs)
            result = map[string]any{}
        case "apitest/on_result":
            var p struct {
                PassCount int `json:"pass_count"`
                FailCount int `json:"fail_count"`
            }
            _ = json.Unmarshal(req.Params, &p)
            fmt.Fprintf(os.Stderr, "[plugin:hooklog] on_result pass_count=%d fail_count=%d\n", p.PassCount, p.FailCount)
            result = map[string]any{}
        }
        resp := map[string]any{
            "jsonrpc": "2.0", "id": req.ID, "result": result,
        }
        out, _ := json.Marshal(resp)
        _, _ = os.Stdout.Write(append(out, '\n'))
    }
}
```

#### Tests to Write FIRST (RED phase)

- `internal/plugin/hooks/integration_test.go` (`//go:build !short`) —
  builds the hooklog fixture and exercises one request end-to-end against
  an `httptest.Server`, asserts stderr contains the three expected lines.

#### Impact on Existing Tests

- None; new fixture.

---

### Step 7: Extend smoke test + docs

**Rationale:** Completeness contract. The observable command must run via
smoke/run.sh; the help text must mention plugin hooks; `docs/plugins.md`
must document the runtime hook protocol.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | New "Plugin hooks (M5-018)" section that builds hooklog, runs one-request.yaml, greps stderr for the three hook lines |
| `docs/plugins.md` | modify | New "Hook invocation protocol" section describing on_request/on_response/on_result wire format, timeout semantics, mutation rules |
| `CHANGELOG.md` | modify | Add entry under `[Unreleased] / Added` |

#### Smoke-test addition

```bash
echo "=== Plugin hooks (M5-018) ==="
HOOK_DIR=$(mktemp -d /tmp/apitest_hooklog_XXXXXX)
go build -o "$HOOK_DIR/hooklog" ./testdata/plugins/hooklog-plugin
SMOKE_COLL=$(mktemp /tmp/apitest_hooklog_coll_XXXXXX.yaml)
cat > "$SMOKE_COLL" <<'YAML'
name: Hooklog smoke
requests:
  - name: ping
    request:
      method: GET
      url: https://httpbin.org/get
    assertions:
      status: 200
YAML
OUT=$(APITEST_PLUGINS="$HOOK_DIR/hooklog" ./apitest run "$SMOKE_COLL" 2>&1)
echo "$OUT" | grep -q "on_request" || { echo "FAIL: no on_request line — $OUT"; exit 1; }
echo "$OUT" | grep -q "on_response" || { echo "FAIL: no on_response line — $OUT"; exit 1; }
echo "$OUT" | grep -q "on_result" || { echo "FAIL: no on_result line — $OUT"; exit 1; }
echo "PASS: hooklog all three lifecycle hooks fired"
rm -rf "$HOOK_DIR" "$SMOKE_COLL"
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| `internal/plugin/host_test.go` | `TestHost_Load_UnknownHook` | none | passes; `on_result` becomes known |
| `internal/plugin/host_test.go` | `TestHost_Load_FiltersKnownHooks` | new | write in Step 1 |
| `internal/plugin/channel_test.go` | (new file) | new | write in Step 2 |
| `internal/plugin/host_test.go` | `TestHost_LoadForRun_*` | new | write in Step 2 |
| `internal/plugin/hooks/hooks_test.go` | (new file) | new | write in Step 3 |
| `internal/plugin/hooks/dispatch_test.go` | (new file) | new | write in Step 3 |
| `internal/plugin/hooks/integration_test.go` | (new file, `!short`) | new | write in Step 6 |
| `internal/runner/runner_test.go` | `TestRun_WithHooksDispatcher_*` | new | write in Step 4 |
| `internal/runner/runner_test.go` | existing tests | none | VarSources.Hooks defaults to nil |
| `cmd/apitest/plugins_test.go` | `TestRun_HookPlugin_*` | new | write in Step 5 |
| `cmd/apitest/main_test.go` | existing help-text tests | update | assert the new "Plugin hooks" section is present |

## Risks and Edge Cases

- **Risk: Plugin process deadlock on blocked stdout.** A plugin that
  writes huge responses could exceed pipe buffers and block.
  **Mitigation:** `Channel.Call` uses a goroutine to read; the 10s
  timeout triggers kill + abandon on any blocked read.

- **Risk: Concurrent calls to the same plugin across parallel-mode
  requests.** Go's parallel execution could invoke the hook wrapper
  concurrently. **Mitigation:** `Channel.mu` serialises every `Call`.
  Plugins are guaranteed to see requests one at a time; parallel runs
  simply queue at the channel boundary. Trade-off: plugins can serialise
  throughput. Documented as known limitation.

- **Risk: Plugin crashes mid-run.** stdout EOF means the next `Call`
  will return `ErrChannelClosed`. **Mitigation:** treat like a timeout —
  print a warning and drop the plugin from the live registries; the run
  continues.

- **Risk: Mutations produce an invalid request.** A plugin that returns
  a malformed `method` field could crash the HTTP layer.
  **Mitigation:** `ToHTTPRequest` is permissive; an empty method yields a
  network error wrapped by `httpexec.ErrNetwork`, which the runner
  already handles. Documented.

- **Risk: `on_response` payload size for large bodies.** Plugins receive
  the whole body as `json.RawMessage`. **Mitigation:** documented as an
  intentional limit; follow-up tasks can cap body size if needed.

- **Risk: Timeout warning race condition.** If a plugin replies at the
  same instant as `ctx.Done()`, the warning may print while the response
  is being processed. **Mitigation:** `select` with priority on `ctx.Done`
  after receiving; plus `sync.Once` on the kill func so duplicate kill is
  idempotent.

- **Edge case: `--parallel` with plugins.** Parallel runs wrap `exec` the
  same way, so hooks fire per request. Ordering across requests is
  non-deterministic (by design of parallel mode); per-request ordering is
  still setup → on_request → exec → on_response → assertions. Confirmed
  safe.

- **Edge case: WebSocket requests.** WebSocket path does not call `exec`
  (it has its own `websocket.Execute`). Hooks will NOT fire around
  WebSocket steps in this task. Documented.

- **Edge case: Data-driven requests.** `executeDataDriven` ultimately
  calls the same wrapped `exec`, so hooks fire per iteration. Confirmed.

- **Edge case: Retry attempts.** Retry uses the wrapped exec, so hooks
  fire per attempt. This is intentional — plugins see every attempt.
  Documented in the help text and docs/plugins.md.

- **Edge case: `on_result` fires even on `setupFailed`.** The
  `runPhases` path always returns a summary (even when setup failed and
  main was skipped). `OnResult` fires once for the whole run.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/plugin/...
go test ./internal/plugin/hooks/...
go test ./internal/runner/...
go test ./cmd/apitest/...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
go build -o apitest ./cmd/apitest
go build -o /tmp/apitest-hooklog-plugin ./testdata/plugins/hooklog-plugin
APITEST_PLUGINS=/tmp/apitest-hooklog-plugin \
  ./apitest run testdata/plugins/one-request.yaml
# Expected stderr (tail) includes:
#   [plugin:hooklog] on_request GET https://httpbin.org/get
#   [plugin:hooklog] on_response 200 142ms
#   [plugin:hooklog] on_result pass_count=1 fail_count=0
# Expected exit: 0
```
