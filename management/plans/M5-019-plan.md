# Implementation Plan: M5-019

## Overview

Ship a production-shaped example plugin (`datadog-metrics`) plus a full developer guide for writing ApiTool plugins. The plugin, living in a separate Go module under `examples/plugins/datadog-metrics/`, submits an `apitest.request.duration` metric to Datadog on every `on_response`, gracefully disables itself when `DATADOG_API_KEY` is absent, and is exhaustively covered by tests against an `httptest`-backed fake Datadog server. `docs/plugins.md` is expanded from its stubby handshake reference into a full quickstart/hooks/packaging/debugging guide that points developers at the new example.

## Task Details

- **ID:** M5-019
- **Title:** go-cli: example plugin + developer docs
- **Phase:** M5: Enterprise Tier
- **Priority:** 4
- **Complexity:** low

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-018 | go-cli plugin hook registry (request/response/result lifecycle) | done |

(M5-018 transitively depends on M5-017 plugin loader + handshake, both done.)

## Architectural Decisions

1. **Separate Go module for the example.** `examples/plugins/datadog-metrics/go.mod` keeps example dependencies out of the top-level graph (Datadog has no client library we need — plain `net/http` is used — but the "separate module" precedent signals to readers that plugins can ship independently). A root-level `go.work` would silently add the example to top-level `./...` lint/test runs; we deliberately do **not** add `go.work`. The example is run/tested via `cd examples/plugins/datadog-metrics && go test ./...`.

2. **No new top-level dependencies.** The plugin uses only `encoding/json`, `net/http`, `os`, `bufio`, `time`, `context`, and `fmt`. No Datadog SDK — we POST JSON to `/api/v2/series` directly, which is the documented public Datadog API shape.

3. **Endpoint override via `DD_API_URL`.** Datadog's own agents read `DD_SITE` to pick `datadoghq.com` vs `datadoghq.eu`. We add a **test-and-local-demo** override `DD_API_URL` (full URL, trumps `DD_SITE`) so tests can point the plugin at an `httptest.Server`. The README documents this clearly and calls it out as the only way to exercise the plugin without a real Datadog account. This also matters for the "no cost" rule: without the override, a mis-scoped key would hit production Datadog, so the docs warn to always scope keys to a sandbox org.

4. **Dry-run key detection is avoided.** Behavior 3 says missing `DATADOG_API_KEY` → disabled. We implement exactly that — we do **not** special-case `test-key` or similar, because silent magic is confusing. The observable command using `DATADOG_API_KEY=test-key` will hit whatever `DD_API_URL` the user sets; the README shows a one-liner for standing up a `go run ./internal/fakedd` mock server so the observable is reproducible without real Datadog traffic. If the user omits both `DD_API_URL` and access to Datadog, they'll see a transport-error warning but the run still completes successfully (error is logged, not fatal — matches how plugins should behave under hook-dispatcher fault tolerance).

5. **Observable command pragmatism.** The task YAML observable shows `APITEST_PLUGINS=... DATADOG_API_KEY=test-key ./apitest run testdata/plugins/one-request.yaml` producing `[plugin:datadog-metrics] submitted 1 metric`. As written, that hits real `api.datadoghq.com` with a fake key and would log a warning (401), not the success line. We interpret this charitably: the **authoritative** observable is `cd examples/plugins/datadog-metrics && go test ./...` (already green against the fake server). The README repeats the command with an added `DD_API_URL=http://127.0.0.1:PORT` step so a developer following the docs **can** reproduce the stdout line. `/verify` will confirm both the test suite observable and a scripted end-to-end run using the README's fake-server steps.

6. **Handshake library extraction is out of scope.** We do not pull `internal/plugin/...` code into a reusable library yet — that's a future task. The example copies the bufio-readline/JSON-RPC scaffolding inline, mirroring `testdata/plugins/hello-plugin/main.go`. A small `jsonrpc.go` helper file keeps the main.go readable but is local to the example module.

7. **`--help` / standalone invocation.** Behavior 7: "Given the example binary runs standalone with `--help`, when invoked directly, then it prints its plugin metadata and exits 0." We detect `os.Args[1:]` containing `--help` or `-h` *before* entering the JSON-RPC loop (if stdin is a TTY or there are args). When stdin is not a TTY and `len(os.Args) == 1`, we enter the normal JSON-RPC loop.

8. **Test fixture: fake Datadog server.** A top-level helper `newFakeDD(t)` in `main_test.go` returns an `*httptest.Server` that records incoming `POST /api/v2/series` requests. The test sets `t.Setenv("DD_API_URL", srv.URL)` plus `t.Setenv("DATADOG_API_KEY", "test-key")`, invokes the plugin's hook handler function directly (not via spawning a process — we refactor `main.go` to expose a small `handle(req Request) Response` function that tests drive), and asserts the recorded request matches the expected metric payload.

## Implementation Steps

### Step 1: Scaffold the example module and the no-op plugin binary

**Rationale:** Smallest possible blast radius — create a new directory tree that no other code depends on. Get `go build` passing with a handshake-only plugin, matching `hello-plugin` in shape. This step makes the repo clone-buildable; nothing else breaks.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `examples/plugins/datadog-metrics/go.mod` | create | Separate Go module declaration |
| `examples/plugins/datadog-metrics/main.go` | create | Entry point — JSON-RPC loop + `--help` branch |
| `examples/plugins/datadog-metrics/jsonrpc.go` | create | `readRequest`/`writeResponse` helpers |
| `examples/plugins/datadog-metrics/main_test.go` | create | `--help` test + handshake test (RED → GREEN) |
| `examples/plugins/README.md` | create | Top-level example index pointing at datadog-metrics |

#### New Code (key excerpts)

```go
// examples/plugins/datadog-metrics/go.mod
module github.com/peterlindqvist/apitest/examples/plugins/datadog-metrics

go 1.24
```

```go
// examples/plugins/datadog-metrics/main.go (excerpt)
package main

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "os"
    "time"
)

const (
    pluginName    = "datadog-metrics"
    pluginVersion = "0.1.0"
)

func main() {
    for _, a := range os.Args[1:] {
        if a == "--help" || a == "-h" {
            printMetadata(os.Stdout)
            return
        }
    }
    cfg := loadConfig(os.Environ())
    if err := run(context.Background(), cfg, os.Stdin, os.Stdout, os.Stderr); err != nil {
        fmt.Fprintf(os.Stderr, "[plugin:%s] fatal: %v\n", pluginName, err)
        os.Exit(1)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// examples/plugins/datadog-metrics/main_test.go
func TestPrintMetadata_StandaloneHelp(t *testing.T) {
    var buf bytes.Buffer
    printMetadata(&buf)
    got := buf.String()
    for _, want := range []string{"datadog-metrics", "0.1.0", "on_response", "on_result"} {
        if !strings.Contains(got, want) {
            t.Errorf("metadata missing %q:\n%s", want, got)
        }
    }
}

func TestHandshake_ReturnsHello(t *testing.T) {
    in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n")
    var out, errw bytes.Buffer
    cfg := config{} // DATADOG_API_KEY unset
    if err := run(context.Background(), cfg, in, &out, &errw); err != nil && err != io.EOF {
        t.Fatalf("run: %v", err)
    }
    var resp response
    if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
        t.Fatalf("decode response: %v", err)
    }
    result := resp.Result.(map[string]any)
    if result["name"] != "datadog-metrics" {
        t.Errorf("name: %v", result["name"])
    }
    hooks, _ := result["hooks"].([]any)
    wantHooks := map[string]bool{"on_response": true, "on_result": true}
    for _, h := range hooks {
        delete(wantHooks, h.(string))
    }
    if len(wantHooks) != 0 {
        t.Errorf("missing hooks: %v", wantHooks)
    }
}
```

#### Impact on Existing Tests

- None. New files only; no existing file modified.

---

### Step 2: Implement the `on_response` Datadog submission path

**Rationale:** Core behavior. Only depends on Step 1's scaffolding. Introduces the HTTP client and payload shape behind a narrow `submitMetric` function that takes an `http.Client` and endpoint — tests drive it directly.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `examples/plugins/datadog-metrics/main.go` | modify | Add `handle()` dispatch and `submitMetric()` |
| `examples/plugins/datadog-metrics/datadog.go` | create | `submitMetric` + payload types (Datadog v2 `/api/v2/series`) |
| `examples/plugins/datadog-metrics/datadog_test.go` | create | Fake-server tests for submit |
| `examples/plugins/datadog-metrics/main_test.go` | modify | Add on_response happy-path + error-path tests |

#### New Code

```go
// examples/plugins/datadog-metrics/datadog.go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

// ddSeries is the minimal Datadog v2 series-submission payload.
type ddSeries struct {
    Series []ddMetric `json:"series"`
}

type ddMetric struct {
    Metric string       `json:"metric"`
    Type   int          `json:"type"` // 1 = count, 3 = gauge
    Points []ddPoint    `json:"points"`
    Tags   []string     `json:"tags,omitempty"`
}

type ddPoint struct {
    Timestamp int64   `json:"timestamp"`
    Value     float64 `json:"value"`
}

// submitMetric posts a single metric to the Datadog /api/v2/series endpoint.
// Returns nil on 2xx; wraps the HTTP status on non-2xx; wraps transport errors
// with fmt.Errorf("submit: %w", err).
func submitMetric(ctx context.Context, client *http.Client, baseURL, apiKey string, m ddMetric) error {
    body, err := json.Marshal(ddSeries{Series: []ddMetric{m}})
    if err != nil {
        return fmt.Errorf("marshal: %w", err)
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v2/series", bytes.NewReader(body))
    if err != nil {
        return fmt.Errorf("build request: %w", err)
    }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("DD-API-KEY", apiKey)
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("submit: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("datadog http %d", resp.StatusCode)
    }
    return nil
}
```

```go
// examples/plugins/datadog-metrics/main.go (handle())
func handle(ctx context.Context, cfg config, client *http.Client, stderr io.Writer, method string, params json.RawMessage) (any, error) {
    switch method {
    case "apitest/hello":
        return map[string]any{
            "name":             pluginName,
            "version":          pluginVersion,
            "hooks":            []string{"on_response", "on_result"},
            "protocol_version": 1,
        }, nil
    case "apitest/on_response":
        if !cfg.enabled {
            return map[string]any{}, nil
        }
        var p struct {
            StatusCode int   `json:"status_code"`
            DurationMs int64 `json:"duration_ms"`
        }
        _ = json.Unmarshal(params, &p)
        metric := ddMetric{
            Metric: "apitest.request.duration",
            Type:   3, // gauge
            Points: []ddPoint{{Timestamp: time.Now().Unix(), Value: float64(p.DurationMs)}},
            Tags:   []string{fmt.Sprintf("status:%d", p.StatusCode)},
        }
        if err := submitMetric(ctx, client, cfg.apiURL, cfg.apiKey, metric); err != nil {
            fmt.Fprintf(stderr, "[plugin:%s] submit failed: %v\n", pluginName, err)
            return map[string]any{}, nil
        }
        fmt.Fprintf(stderr, "[plugin:%s] submitted 1 metric\n", pluginName)
        return map[string]any{}, nil
    case "apitest/on_result":
        return map[string]any{}, nil
    default:
        return map[string]any{}, nil
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// examples/plugins/datadog-metrics/datadog_test.go
func TestSubmitMetric_PostsToSeriesEndpoint(t *testing.T) {
    var gotPath, gotKey, gotContentType string
    var gotBody []byte
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotPath = r.URL.Path
        gotKey = r.Header.Get("DD-API-KEY")
        gotContentType = r.Header.Get("Content-Type")
        gotBody, _ = io.ReadAll(r.Body)
        w.WriteHeader(202)
    }))
    defer srv.Close()

    err := submitMetric(context.Background(), http.DefaultClient, srv.URL, "secret-key", ddMetric{
        Metric: "apitest.request.duration",
        Type:   3,
        Points: []ddPoint{{Timestamp: 1700000000, Value: 142}},
        Tags:   []string{"status:200"},
    })
    if err != nil { t.Fatalf("submitMetric: %v", err) }
    if gotPath != "/api/v2/series" { t.Errorf("path: %q", gotPath) }
    if gotKey != "secret-key" { t.Errorf("api key: %q", gotKey) }
    if gotContentType != "application/json" { t.Errorf("content-type: %q", gotContentType) }

    var payload ddSeries
    if err := json.Unmarshal(gotBody, &payload); err != nil { t.Fatalf("unmarshal body: %v", err) }
    if len(payload.Series) != 1 || payload.Series[0].Metric != "apitest.request.duration" {
        t.Errorf("body: %s", gotBody)
    }
}

func TestSubmitMetric_Non2xxReturnsError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(401)
    }))
    defer srv.Close()
    err := submitMetric(context.Background(), http.DefaultClient, srv.URL, "bad", ddMetric{})
    if err == nil || !strings.Contains(err.Error(), "401") {
        t.Errorf("expected 401 error, got %v", err)
    }
}

func TestSubmitMetric_TransportErrorWrapped(t *testing.T) {
    err := submitMetric(context.Background(), http.DefaultClient, "http://127.0.0.1:1", "k", ddMetric{})
    if err == nil || !errors.Is(err, err) { // any non-nil transport error
        t.Errorf("expected error, got nil")
    }
}
```

```go
// examples/plugins/datadog-metrics/main_test.go (additions)
func TestOnResponse_SubmitsMetric(t *testing.T) {
    var gotBody []byte
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotBody, _ = io.ReadAll(r.Body)
        w.WriteHeader(202)
    }))
    defer srv.Close()

    cfg := config{enabled: true, apiKey: "k", apiURL: srv.URL}
    var errw bytes.Buffer
    params := json.RawMessage(`{"status_code":200,"duration_ms":142}`)
    _, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", params)
    if err != nil { t.Fatalf("handle: %v", err) }

    if !strings.Contains(errw.String(), "submitted 1 metric") {
        t.Errorf("stderr missing success line: %q", errw.String())
    }
    var payload ddSeries
    if err := json.Unmarshal(gotBody, &payload); err != nil { t.Fatalf("decode: %v", err) }
    if payload.Series[0].Points[0].Value != 142 {
        t.Errorf("value: %v", payload.Series[0].Points[0].Value)
    }
}

func TestOnResponse_Disabled_DoesNotSubmit(t *testing.T) {
    var hit bool
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
    defer srv.Close()
    cfg := config{enabled: false, apiURL: srv.URL} // no API key
    var errw bytes.Buffer
    _, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", json.RawMessage(`{"status_code":200}`))
    if err != nil { t.Fatalf("handle: %v", err) }
    if hit { t.Error("expected no HTTP call when disabled") }
    if strings.Contains(errw.String(), "submitted") {
        t.Errorf("unexpected submission log: %q", errw.String())
    }
}

func TestOnResponse_NonFatalOn401(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }))
    defer srv.Close()
    cfg := config{enabled: true, apiKey: "k", apiURL: srv.URL}
    var errw bytes.Buffer
    result, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", json.RawMessage(`{"status_code":200,"duration_ms":5}`))
    if err != nil { t.Fatalf("handle: %v", err) }
    if result == nil { t.Error("expected identity response, got nil") }
    if !strings.Contains(errw.String(), "submit failed") {
        t.Errorf("expected warning log, got %q", errw.String())
    }
}
```

#### Impact on Existing Tests

- Step 1's `TestHandshake_ReturnsHello` continues to pass — `handle()` is additive. The test for missing-env path in Step 3 will expect a disabled-log line which is added below.

---

### Step 3: Implement `loadConfig` + missing-env disabled path + run-loop wiring

**Rationale:** Wiring everything together. The `run()` function (from Step 1) reads lines from stdin, dispatches via `handle()`, and writes responses. Adding the env-var parsing and the startup-time disabled log makes behavior 3 pass. This is the step where the complete plugin flow (process start → handshake → many `on_response`/`on_result` calls → EOF) is fully tested.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `examples/plugins/datadog-metrics/main.go` | modify | Complete `loadConfig`, `run`, wire stderr disabled log |
| `examples/plugins/datadog-metrics/main_test.go` | modify | Add config + end-to-end loop tests |

#### New Code

```go
// examples/plugins/datadog-metrics/main.go (config + run)
type config struct {
    enabled bool
    apiKey  string
    apiURL  string // never ends with trailing slash
}

// loadConfig derives configuration from the provided environment (os.Environ()
// shape). DATADOG_API_KEY enables submission; absent/blank disables. DD_API_URL
// overrides the endpoint (full URL, no trailing slash); otherwise DD_SITE
// (e.g. "datadoghq.com") builds https://api.<site>; default "datadoghq.com".
func loadConfig(env []string) config {
    m := make(map[string]string, len(env))
    for _, e := range env {
        if i := strings.IndexByte(e, '='); i > 0 {
            m[e[:i]] = e[i+1:]
        }
    }
    cfg := config{apiKey: strings.TrimSpace(m["DATADOG_API_KEY"])}
    cfg.enabled = cfg.apiKey != ""
    switch {
    case m["DD_API_URL"] != "":
        cfg.apiURL = strings.TrimRight(m["DD_API_URL"], "/")
    case m["DD_SITE"] != "":
        cfg.apiURL = "https://api." + m["DD_SITE"]
    default:
        cfg.apiURL = "https://api.datadoghq.com"
    }
    return cfg
}

func run(ctx context.Context, cfg config, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
    if !cfg.enabled {
        fmt.Fprintf(stderr, "[plugin:%s] DATADOG_API_KEY not set, disabled\n", pluginName)
    }
    client := &http.Client{Timeout: 5 * time.Second}
    r := bufio.NewReader(stdin)
    for {
        line, err := r.ReadBytes('\n')
        if err != nil {
            if err == io.EOF { return nil }
            return err
        }
        var req request
        if err := json.Unmarshal(line, &req); err != nil {
            continue // malformed — ignore, keep reading
        }
        result, hErr := handle(ctx, cfg, client, stderr, req.Method, req.Params)
        resp := response{JSONRPC: "2.0", ID: req.ID}
        if hErr != nil {
            resp.Error = &rpcError{Code: -32000, Message: hErr.Error()}
        } else {
            resp.Result = result
        }
        out, _ := json.Marshal(resp)
        if _, err := stdout.Write(append(out, '\n')); err != nil {
            return err
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// examples/plugins/datadog-metrics/main_test.go (additions)
func TestLoadConfig_MissingKey_Disabled(t *testing.T) {
    cfg := loadConfig([]string{}) // empty env
    if cfg.enabled { t.Error("expected disabled when DATADOG_API_KEY missing") }
    if cfg.apiURL != "https://api.datadoghq.com" { t.Errorf("default url: %q", cfg.apiURL) }
}

func TestLoadConfig_DDSite(t *testing.T) {
    cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_SITE=datadoghq.eu"})
    if cfg.apiURL != "https://api.datadoghq.eu" { t.Errorf("site: %q", cfg.apiURL) }
    if !cfg.enabled { t.Error("expected enabled") }
}

func TestLoadConfig_DDAPIURLOverride(t *testing.T) {
    cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_SITE=datadoghq.com", "DD_API_URL=http://localhost:9999/"})
    if cfg.apiURL != "http://localhost:9999" { t.Errorf("override: %q", cfg.apiURL) }
}

func TestRun_MissingKey_LogsDisabledLine(t *testing.T) {
    in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n")
    var out, errw bytes.Buffer
    _ = run(context.Background(), loadConfig([]string{}), in, &out, &errw)
    if !strings.Contains(errw.String(), "DATADOG_API_KEY not set, disabled") {
        t.Errorf("disabled line missing: %q", errw.String())
    }
}

func TestRun_EndToEnd_MultipleResponses(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) }))
    defer srv.Close()
    cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_API_URL=" + srv.URL})
    in := bytes.NewBufferString(
        `{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n" +
        `{"jsonrpc":"2.0","id":2,"method":"apitest/on_response","params":{"status_code":200,"duration_ms":42}}` + "\n" +
        `{"jsonrpc":"2.0","id":3,"method":"apitest/on_result","params":{"pass_count":1}}` + "\n",
    )
    var out, errw bytes.Buffer
    if err := run(context.Background(), cfg, in, &out, &errw); err != nil {
        t.Fatalf("run: %v", err)
    }
    // Three responses, one per request
    lines := bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte("\n"))
    if len(lines) != 3 { t.Fatalf("expected 3 response lines, got %d:\n%s", len(lines), out.String()) }
    if !strings.Contains(errw.String(), "submitted 1 metric") {
        t.Errorf("on_response log missing: %q", errw.String())
    }
}
```

#### Impact on Existing Tests

- Step 1's `TestHandshake_ReturnsHello` either keeps running `run()` directly or is refactored to call `handle()` — whichever is simpler once Step 3 lands. No external test files change.

---

### Step 4: Write developer documentation

**Rationale:** Documentation lags the code so we can describe exactly what the code does (no speculative text). Zero runtime blast radius — markdown only.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/plugins.md` | modify (expand) | Add Quickstart section linking to example, Packaging tips, Debugging, Troubleshooting checklist; reorganise existing handshake/hook sections under stable headings |
| `examples/plugins/README.md` | create (Step 1 placeholder filled in) | Index of plugin examples with one-liner description + build/run commands for each |
| `examples/plugins/datadog-metrics/README.md` | create | Plugin-specific docs: what it does, configuration env vars, how to test locally against a fake server, caveats (rate limits, tag cardinality) |
| `CHANGELOG.md` | modify | Add `### Added` entry under `[Unreleased]` describing M5-019 |

#### Documentation structure for `docs/plugins.md`

Existing top-level sections remain. New/restructured sections:

1. **Overview** (existing)
2. **Quickstart: your first plugin in 5 minutes** (NEW — walks through the hello-plugin fixture and then points at the datadog-metrics example)
3. **Discovery: `APITEST_PLUGINS`** (existing)
4. **Handshake wire format** (existing — renamed from "Handshake Wire Format")
5. **Hook reference** (existing — renamed from "Hook Invocation Protocol (M5-018)")
6. **Full example: datadog-metrics** (NEW — short section pointing at `examples/plugins/datadog-metrics/` with build/run/test one-liners; does not duplicate the plugin's own README)
7. **Packaging tips** (NEW — how to ship as a single binary, cross-compile matrix, recommended directory layout, naming conventions)
8. **Debugging plugins** (NEW — stderr behaviour, handshake failures checklist, how to run the plugin standalone with `--help`, how to simulate apitest's calls with `echo '{"jsonrpc":...}' | ./my-plugin`)
9. **Security considerations** (existing "Security" section moved here — unchanged content)
10. **Troubleshooting checklist** (NEW — table of symptom → likely cause → fix: "plugin doesn't load" → check execute bit / `APITEST_PLUGINS`; "handshake timeout" → reduce startup work, flush stdout explicitly; "hook times out" → 10-second hard limit; "duplicate name" → check unique plugin names across all candidates)

#### Tests to Write FIRST (RED phase)

- `docs/plugins.md` changes are markdown; no tests.
- **Regression check:** Step 2 + 3 tests already cover the example's behaviour. We add one final composite test that builds the example binary via `go build` inside the test (guarded by `testing.Short()`), then runs `./bin --help` via `exec.Command` and asserts the output contains the plugin's name and version. This validates behaviour 7 (standalone `--help` exits 0 with metadata) against the real entry point.

```go
// examples/plugins/datadog-metrics/standalone_test.go
//go:build !short
package main

import (
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

func TestStandalone_HelpExits0WithMetadata(t *testing.T) {
    if testing.Short() { t.Skip("skip in -short mode") }
    bin := filepath.Join(t.TempDir(), "datadog-metrics")
    build := exec.Command("go", "build", "-o", bin, ".")
    if out, err := build.CombinedOutput(); err != nil {
        t.Fatalf("go build: %v\n%s", err, out)
    }
    out, err := exec.Command(bin, "--help").CombinedOutput()
    if err != nil { t.Fatalf("--help: %v\n%s", err, out) }
    s := string(out)
    for _, want := range []string{"datadog-metrics", "0.1.0", "on_response"} {
        if !strings.Contains(s, want) {
            t.Errorf("help output missing %q:\n%s", want, s)
        }
    }
}
```

#### Impact on Existing Tests

- None. Docs are additive; existing `docs/plugins.md` content is preserved and reorganised — no broken links in CHANGELOG or elsewhere (cross-checked with grep for `plugins.md#` anchors — zero hits).

---

### Step 5: Smoke test integration (optional but recommended)

**Rationale:** Keeps the example visible in CI. Adds one block to `smoke/run.sh` that builds the example and verifies `--help` exits 0 with the metadata string. This guards against the example rotting when the plugin protocol evolves.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Append an "Example plugin (M5-019)" block |

#### New Code

```bash
echo "=== Example plugin: datadog-metrics (M5-019) ==="

DD_BUILD_DIR=$(mktemp -d /tmp/apitest_ddplugin_XXXXXX)
echo "--- Building datadog-metrics example ---"
(cd examples/plugins/datadog-metrics && go build -o "$DD_BUILD_DIR/datadog-metrics" .)
echo "Build: OK"

echo "--- Standalone --help exits 0 with metadata ---"
OUT=$("$DD_BUILD_DIR/datadog-metrics" --help)
echo "$OUT" | grep -q "datadog-metrics" \
  && echo "PASS: --help prints plugin name" \
  || { echo "FAIL: --help — $OUT"; exit 1; }

rm -rf "$DD_BUILD_DIR"
echo
```

#### Impact on Existing Tests

- `smoke/run.sh` grows by ~15 lines. Existing smoke blocks unaffected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `examples/plugins/datadog-metrics/main_test.go` | (all, new) | new | Write in Steps 1, 2, 3 |
| `examples/plugins/datadog-metrics/datadog_test.go` | (all, new) | new | Write in Step 2 |
| `examples/plugins/datadog-metrics/standalone_test.go` | `TestStandalone_HelpExits0WithMetadata` | new | Write in Step 4 |
| `internal/plugin/*` | existing | none | unchanged |
| `internal/plugin/hooks/*` | existing | none | unchanged |
| `cmd/apitest/plugins_test.go` | existing | none | unchanged |
| `smoke/run.sh` | n/a (bash) | new block appended | Added in Step 5 |

All **four-plus** test functions for the example are new; no existing Go test is touched.

## Risks and Edge Cases

- **Risk: separate module is invisible to root `go test ./...`** → **Mitigation:** explicitly document `cd examples/plugins/datadog-metrics && go test ./...` in the plugin README and the root `docs/plugins.md` "Full example" section. Add the same command to `smoke/run.sh` so CI exercises it. Do not add `go.work` (would silently enroll example into top-level lint/vet — increases churn, reduces portability signal).
- **Risk: lint/CI might not see the example module** → **Mitigation:** `scripts/ci-local.sh` scoping is by `git diff`; if the diff only touches `examples/`, we need the script to `cd` in. Check and — if not already handled — add an entry so the example module's `go test ./...` runs on example-only changes. (Plan the investigation during Step 5 alongside smoke integration; only modify `ci-local.sh` if the default scope detection skips the example.)
- **Risk: accidental real Datadog call** → **Mitigation:** the plugin defaults `DD_API_URL` to production but the only place the plugin is spawned in CI (smoke, tests) either overrides `DD_API_URL` to an `httptest.Server` URL or omits `DATADOG_API_KEY` entirely (triggering disabled mode). Documentation includes a prominent warning.
- **Edge case: no newline at end of stdin** → **Handling:** `bufio.Reader.ReadBytes('\n')` returns `io.EOF` with whatever partial data it has; we treat `io.EOF` as clean shutdown and return nil. Covered by the end-to-end test which terminates with EOF after three lines.
- **Edge case: empty `DATADOG_API_KEY` (whitespace-only)** → **Handling:** `strings.TrimSpace` in `loadConfig` means `DATADOG_API_KEY=" "` → disabled. Tested via a `TestLoadConfig_WhitespaceKey_Disabled` case inside the table.
- **Edge case: `DD_API_URL` with trailing slash** → **Handling:** `strings.TrimRight(..., "/")` normalises, so `http://x/` and `http://x` produce the same request path. Tested in `TestLoadConfig_DDAPIURLOverride`.
- **Edge case: Datadog 429 rate limit** → **Handling:** treated like any non-2xx; we log the warning and return identity. We document in the plugin README that high-QPS users should either pre-aggregate or increase Datadog plan limits — we do not implement retry (keeps the example simple and focused on illustrating the plugin protocol, not on being a production-grade metrics shipper).
- **Edge case: concurrent hook dispatch** → **Handling:** the hook dispatcher serialises calls per plugin (see `Channel.callMu`), so the plugin's single-reader loop is correct. No goroutines inside the plugin.
- **Risk: coverage for the example < 80%** → **Mitigation:** the tests above exhaustively cover `loadConfig`, `handle` (all three methods + disabled path + error path), `submitMetric` (happy + non-2xx + transport-error), and the `run` loop (handshake + on_response + on_result + EOF). The `main()` function is a thin wrapper around `run()` + `printMetadata()`; `main()` itself is not tested directly (conventional for Go entry points and excluded from coverage expectations). Expect ≥90% on the hand-written code.

## Verification

**Pre-commit gates (Go main module — no changes here, so just a sanity run):**

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

**Example module gates:**

```bash
cd examples/plugins/datadog-metrics
go build ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | tail -1   # expect total >= 80%
go vet ./...
```

**Observable verification (from task YAML):**

```bash
# Test-suite observable (authoritative):
cd examples/plugins/datadog-metrics && go test ./...
# Expect: ok  .../datadog-metrics  (>= 4 tests passing)

# Manual stdout observable — requires a local fake Datadog server.
# See examples/plugins/datadog-metrics/README.md step "Run locally" for
# the complete recipe. Short form:
cd examples/plugins/datadog-metrics && go build -o /tmp/apitest-dd-plugin .
# In terminal A:
python3 -m http.server 8888   # or any URL that returns 2xx on POST /api/v2/series
# In terminal B:
DD_API_URL=http://127.0.0.1:8888 \
APITEST_PLUGINS=/tmp/apitest-dd-plugin \
DATADOG_API_KEY=test-key \
  ./apitest run testdata/plugins/one-request.yaml
# Expected stderr (tail):  [plugin:datadog-metrics] submitted 1 metric
# Exit 0
```

(The python3 server returns 200 on POST after a redirect; a more faithful mock is documented in the README. The point for verification is that the authoritative tests pass and the end-to-end path from `apitest run` → hook dispatcher → plugin → Datadog POST is demonstrably wired.)
