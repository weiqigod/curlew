# Implementation Plan: M5-011

## Overview

Introduce a new `curlew perf` subcommand backed by a new `internal/loadgen`
package. The loadgen package provides a simple, dependency-free load generator:
a fixed virtual-user (VU) worker pool with optional linear ramp-up and a
constant-RPS throughput mode driven by `time.Ticker`. Every VU executes the
same request (parsed from a standalone request YAML file) against the target
server, reusing `internal/httpexec.Execute`. Emits a one-line summary on
completion; rich metrics/reports are deferred to M5-012.

## Task Details

- **ID:** M5-011
- **Title:** go-cli: load generation mode (virtual users, ramp profile)
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| — | (none declared) | — |

`management/backlog.yaml` lists no prerequisites for M5-011. The task uses
`internal/httpexec` which has been stable since M1.

## Architectural Decisions

These decisions resolve the ambiguities flagged in the task's `scope` field.

1. **Where does the request come from?** A standalone YAML file mirroring the
   parser's external-request format (`name: ...`, `request: { method, url,
   headers, body, query }`). We **do not** import `internal/parser` to avoid
   pulling in the entire collection pipeline (variables, retries, assertions,
   tier gates). Instead, `internal/loadgen` has its own tiny YAML loader that
   decodes just `{name, request}`. Rationale: perf never uses variables or
   setup/teardown per the scope; keep `loadgen` self-contained.

2. **VU model.** One goroutine per VU, each looping:
   `for !deadline && !ctx.Done() { httpexec.Execute(...); record() }`.
   No request-queue; VUs are self-scheduling. When `--rps` is set, all VUs pull
   tokens from a shared `time.Ticker(1s/rps)` channel before issuing a request
   (constant-rate throughput mode). VU count still applies as the max
   concurrency ceiling in RPS mode — scope doesn't require a dedicated
   open-model arrival process for this slice.

3. **Ramp-up.** Linear from 1 → VUs over `--ramp-up`. Implementation: a
   scheduler goroutine starts one VU at `t=0` and paces additional VUs at
   `rampUp / (VUs-1)` intervals until all VUs are running, then waits for the
   remaining steady-state time. Deadline is measured from `t=0` (ramp-up
   counts toward `--duration`).

4. **Cancellation / SIGINT.** The CLI wraps the run context with
   `signal.NotifyContext(ctx, os.Interrupt)`. Each VU's `httpexec.Execute`
   takes that context, so in-flight requests unblock naturally. The VUs
   finish their current request (context cancellation propagates to
   `http.Client.Do`) and then exit. The CLI reports exit code 130
   (128 + SIGINT). This matches Go conventions and the observable.

5. **Metric capture.** In this slice we capture only counters (sent, successes,
   failures) and the total elapsed time. Per-request latency samples and
   percentiles land in M5-012. `Runner.Run` returns a `Summary` struct that
   is friendly to M5-012's needs — per-request samples can be added later
   without breaking the existing signature.

6. **Tier gating.** Spec says "advanced performance testing" is Enterprise.
   We add a feature-gate call (`auth.CheckFeature(reg, "perf_loadgen", tier)`)
   before running, surfacing exit code 6 when the user is below Enterprise.
   Adding a new feature key to `auth.DefaultRegistry()` is a small additive
   change; existing tests remain green. Tests in `perf_test.go` override the
   tier via `CURLEW_TIER=enterprise`.

7. **Output format.** Stdout shows the four summary lines listed in
   `observable`. No file output (`--output`) in this slice — that's M5-012.
   We do, however, expose `--output` as an accepted (but reserved) flag in
   help text because the task's `behaviors` item 7 requires it to appear in
   `--help`. The flag value is parsed and stored but treated as "only stdout
   is supported in this slice; pass --output stdout or omit". Any other value
   produces `error: --output <name> not supported in this slice` and exit 2.
   This keeps the `--help` promise honest without gold-plating M5-012.

8. **No request-level assertions.** Requests are counted as successes when
   `httpexec.Execute` returns nil error AND status < 400. Status ≥ 400 is a
   failure (per behavior 5). Transport errors are failures.

9. **Test seam.** `loadgen.Run(ctx, cfg, opts)` accepts an injectable
   `ExecuteFunc` (same type signature the worker package uses) so tests drive
   a fake executor without real HTTP. This mirrors the worker pattern
   (`internal/worker/run.go:16`).

## Files to Create / Modify (Overview)

| File | Action | Purpose |
|------|--------|---------|
| `internal/loadgen/loadgen.go` | create | Config, Summary, sentinel errors |
| `internal/loadgen/loadgen_test.go` | create | Config validation + sentinel tests |
| `internal/loadgen/run.go` | create | Runner core: VU pool, ramp-up, ticker |
| `internal/loadgen/run_test.go` | create | Runner behaviour tests against httptest |
| `internal/loadgen/request.go` | create | YAML loader for request files |
| `internal/loadgen/request_test.go` | create | YAML-loading tests |
| `cmd/curlew/perf.go` | create | `perfCmd`, flag parser, help printer |
| `cmd/curlew/perf_test.go` | create | CLI-level flag parsing + exit code tests |
| `cmd/curlew/main.go` | modify | Dispatch `perf`, update `printHelp()` |
| `internal/auth/registry.go` | modify | Register `perf_loadgen` feature (Enterprise) |
| `internal/auth/registry_test.go` | modify | Assert new feature key exists |
| `testdata/perf/sample-request.yaml` | create | Sample request file for observable |
| `smoke/run.sh` | modify | New `=== Perf --help (M5-011) ===` and end-to-end blocks |
| `CHANGELOG.md` | modify | `## [Unreleased] Added: …` line |

## Implementation Steps

Ordering rationale: packages with the smallest blast radius first. Build the
loadgen primitives (pure Go, testable with fakes), then the YAML loader
(pure Go + os), then wire the CLI, then smoke + CHANGELOG. Feature gating is
additive and lands with the CLI wire-up (not earlier) so intermediate commits
still build.

### Step 1: `internal/loadgen` skeleton — Config, Summary, sentinels

**Rationale:** Start with the narrow public surface so subsequent tests have
a stable target. No concurrency yet — this step is pure types + validation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/loadgen/loadgen.go` | create | Package doc, Config, Summary, sentinel errors, Validate |
| `internal/loadgen/loadgen_test.go` | create | Table-driven Validate tests |

#### New Code

```go
// Package loadgen implements a file-driven HTTP load generator used by the
// curlew perf subcommand. A fixed pool of virtual users (VUs) executes a
// single parsed request in a loop against a target server. Optional
// linear ramp-up paces VU activation; optional constant-RPS throughput
// mode paces requests via a shared time.Ticker.
package loadgen

import (
    "errors"
    "time"
)

// Sentinel errors for well-known configuration failures.
var (
    ErrInvalidVUs      = errors.New("--vus must be >= 1")
    ErrInvalidDuration = errors.New("--duration must be > 0")
    ErrInvalidRampUp   = errors.New("--ramp-up must be >= 0 and <= duration")
    ErrInvalidRPS      = errors.New("--rps must be >= 0")
)

// Config captures the run configuration. VUs and Duration are required.
type Config struct {
    VUs      int           // --vus
    Duration time.Duration // --duration
    RampUp   time.Duration // --ramp-up (0 = no ramp)
    RPS      int           // --rps (0 = unbounded throughput)
}

// Validate returns an error if the Config is unusable.
func (c Config) Validate() error {
    if c.VUs < 1 {
        return ErrInvalidVUs
    }
    if c.Duration <= 0 {
        return ErrInvalidDuration
    }
    if c.RampUp < 0 || c.RampUp > c.Duration {
        return ErrInvalidRampUp
    }
    if c.RPS < 0 {
        return ErrInvalidRPS
    }
    return nil
}

// Summary captures the outcome of a perf run.
type Summary struct {
    Requests  int           // total issued
    Successes int           // 2xx/3xx responses
    Failures  int           // transport errors or >=4xx
    Elapsed   time.Duration // from first VU start to final return
    Aborted   bool          // set true when run ended due to ctx cancel
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestConfig_Validate(t *testing.T) {
    tests := []struct {
        name string
        cfg  Config
        want error
    }{
        {"zero VUs", Config{VUs: 0, Duration: time.Second}, ErrInvalidVUs},
        {"negative VUs", Config{VUs: -1, Duration: time.Second}, ErrInvalidVUs},
        {"zero duration", Config{VUs: 1, Duration: 0}, ErrInvalidDuration},
        {"negative duration", Config{VUs: 1, Duration: -time.Second}, ErrInvalidDuration},
        {"ramp-up negative", Config{VUs: 1, Duration: time.Second, RampUp: -time.Millisecond}, ErrInvalidRampUp},
        {"ramp-up exceeds duration", Config{VUs: 1, Duration: time.Second, RampUp: 2 * time.Second}, ErrInvalidRampUp},
        {"negative RPS", Config{VUs: 1, Duration: time.Second, RPS: -1}, ErrInvalidRPS},
        {"minimum valid config", Config{VUs: 1, Duration: time.Millisecond}, nil},
        {"ramp equal to duration", Config{VUs: 5, Duration: time.Second, RampUp: time.Second}, nil},
        {"rps > 0", Config{VUs: 2, Duration: time.Second, RPS: 100}, nil},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := tc.cfg.Validate()
            if !errors.Is(got, tc.want) {
                t.Fatalf("Validate = %v, want %v", got, tc.want)
            }
        })
    }
}
```

#### Impact on Existing Tests

None — this is a brand-new package.

### Step 2: `internal/loadgen/run.go` — core runner with fake executor

**Rationale:** Implement and test the concurrency logic against a fake
`ExecuteFunc` before wiring real HTTP. This is where most test coverage
lives; building against a fake keeps runtime < 50 ms per test case.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/loadgen/run.go` | create | Run, VU pool, ramp scheduler, RPS ticker |
| `internal/loadgen/run_test.go` | create | 8+ table-driven behaviour tests with httptest + fake executor |

#### New Code

```go
package loadgen

import (
    "context"
    "fmt"
    "io"
    "os"
    "sync"
    "sync/atomic"
    "time"

    "github.com/weiqigod/curlew/internal/httpexec"
)

// ExecuteFunc is the injectable request-execution seam (tests swap this for
// a fake; production wires httpexec.Execute).
type ExecuteFunc func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error)

// RunOptions injects test seams. Zero value uses production defaults.
type RunOptions struct {
    Execute ExecuteFunc // nil = httpexec.Execute
    Stdout  io.Writer   // nil = os.Stdout
    Now     func() time.Time
}

// Run executes the load generation loop and returns a summary. The caller
// owns ctx; ctx cancellation ends the run early with Aborted=true.
func Run(ctx context.Context, cfg Config, req *httpexec.Request, opts RunOptions) (*Summary, error) {
    if err := cfg.Validate(); err != nil {
        return nil, err
    }
    if req == nil || req.URL == "" {
        return nil, fmt.Errorf("loadgen: request is nil or URL empty")
    }

    execute := opts.Execute
    if execute == nil {
        execute = httpexec.Execute
    }
    stdout := opts.Stdout
    if stdout == nil {
        stdout = os.Stdout
    }
    now := opts.Now
    if now == nil {
        now = time.Now
    }

    runCtx, cancel := context.WithDeadline(ctx, now().Add(cfg.Duration))
    defer cancel()

    var requests, successes, failures int64
    var ticker *time.Ticker
    var tickC <-chan time.Time
    if cfg.RPS > 0 {
        ticker = time.NewTicker(time.Second / time.Duration(cfg.RPS))
        defer ticker.Stop()
        tickC = ticker.C
    }

    start := now()
    var wg sync.WaitGroup
    for i := 0; i < cfg.VUs; i++ {
        wg.Add(1)
        vuIdx := i
        go func() {
            defer wg.Done()
            // Wait for this VU's ramp-in tick (linear).
            if cfg.RampUp > 0 && cfg.VUs > 1 {
                per := cfg.RampUp / time.Duration(cfg.VUs-1)
                delay := time.Duration(vuIdx) * per
                select {
                case <-time.After(delay):
                case <-runCtx.Done():
                    return
                }
            }
            runVU(runCtx, execute, req, tickC, &requests, &successes, &failures)
        }()
    }
    wg.Wait()

    aborted := ctx.Err() != nil
    return &Summary{
        Requests:  int(atomic.LoadInt64(&requests)),
        Successes: int(atomic.LoadInt64(&successes)),
        Failures:  int(atomic.LoadInt64(&failures)),
        Elapsed:   now().Sub(start),
        Aborted:   aborted,
    }, nil
}

// runVU loops issuing requests until ctx is done.
func runVU(ctx context.Context, execute ExecuteFunc, req *httpexec.Request,
    tickC <-chan time.Time, reqs, succ, fail *int64) {
    for {
        if ctx.Err() != nil {
            return
        }
        if tickC != nil {
            select {
            case <-ctx.Done():
                return
            case <-tickC:
            }
        }
        atomic.AddInt64(reqs, 1)
        res, err := execute(ctx, req)
        switch {
        case err != nil:
            atomic.AddInt64(fail, 1)
        case res.StatusCode >= 400:
            atomic.AddInt64(fail, 1)
        default:
            atomic.AddInt64(succ, 1)
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_VUsExecuteUntilDeadline(t *testing.T) {
    var count int64
    fake := func(ctx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
        atomic.AddInt64(&count, 1)
        return &httpexec.Result{StatusCode: 200}, nil
    }
    sum, err := loadgen.Run(
        context.Background(),
        loadgen.Config{VUs: 4, Duration: 200 * time.Millisecond},
        &httpexec.Request{Method: "GET", URL: "http://stub"},
        loadgen.RunOptions{Execute: fake},
    )
    // asserts: err nil, sum.Requests > 4, sum.Successes == sum.Requests,
    //          sum.Failures == 0, sum.Elapsed ~ 200ms
}

func TestRun_RejectsInvalidConfig(t *testing.T) {
    tests := []struct {
        cfg  Config
        want error
    }{
        {Config{VUs: 0, Duration: time.Second}, ErrInvalidVUs},
        {Config{VUs: 1, Duration: 0}, ErrInvalidDuration},
    }
    // asserts errors.Is behaviour
}

func TestRun_FailuresOn500(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.WriteHeader(500)
    }))
    defer srv.Close()
    sum, err := loadgen.Run(ctx, Config{VUs: 2, Duration: 100 * time.Millisecond},
        &httpexec.Request{Method: "GET", URL: srv.URL}, RunOptions{})
    // asserts: err nil, sum.Failures == sum.Requests, Successes == 0
}

func TestRun_ContextCancelStopsVUs(t *testing.T) {
    slow := func(ctx context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(50 * time.Millisecond):
            return &httpexec.Result{StatusCode: 200}, nil
        }
    }
    ctx, cancel := context.WithCancel(context.Background())
    go func() { time.Sleep(20 * time.Millisecond); cancel() }()
    sum, err := loadgen.Run(ctx, Config{VUs: 3, Duration: 5 * time.Second},
        &httpexec.Request{Method: "GET", URL: "http://stub"}, RunOptions{Execute: slow})
    // asserts: err nil (ctx.Err swallowed into Summary), sum.Aborted true,
    //          returned within ~100ms (not 5s)
}

func TestRun_RampUp_LinearActivation(t *testing.T) {
    // Track timestamp of first request per VU by using a fake that records
    // goroutine ID via a counter. Assert spread across ramp-up window.
    // Use Config{VUs: 4, Duration: 300ms, RampUp: 200ms}.
    // Assert: first request timestamps form 4 roughly equal buckets in [0, 200ms].
}

func TestRun_RPSMode_LimitsThroughput(t *testing.T) {
    fake := instantFakeExecutor(t)
    sum, _ := loadgen.Run(ctx,
        Config{VUs: 10, Duration: 500 * time.Millisecond, RPS: 20},
        &httpexec.Request{Method: "GET", URL: "http://stub"},
        RunOptions{Execute: fake},
    )
    // assert: Requests approx 10 (20 rps * 0.5s), within +/- 4 tolerance
}

func TestRun_NilRequestReturnsError(t *testing.T) {
    _, err := loadgen.Run(ctx, Config{VUs: 1, Duration: time.Second}, nil, RunOptions{})
    // asserts: err not nil, contains "request"
}

func TestRun_ZeroVUsError(t *testing.T) {
    _, err := loadgen.Run(ctx, Config{VUs: 0, Duration: time.Second},
        &httpexec.Request{Method: "GET", URL: "x"}, RunOptions{})
    // asserts: errors.Is(err, ErrInvalidVUs)
}
```

Plus one integration test using `net/http/httptest` to validate the real
`httpexec.Execute` path:

```go
func TestRun_Integration_HTTPTestServer(t *testing.T) {
    srv := httptest.NewServer(...)
    defer srv.Close()
    sum, err := loadgen.Run(ctx, Config{VUs: 3, Duration: 150 * time.Millisecond},
        &httpexec.Request{Method: "GET", URL: srv.URL}, RunOptions{})
    // asserts: Requests > 0, Successes == Requests
}
```

That's **9 tests** in `run_test.go` — above the observable's "≥8" threshold.

#### Impact on Existing Tests

None — new package.

### Step 3: `internal/loadgen/request.go` — YAML request loader

**Rationale:** Decoupled from the parser package; minimal YAML surface. Land
this after the runner so the CLI wiring step has a single dependency edge.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/loadgen/request.go` | create | LoadRequestFile(path) → *httpexec.Request |
| `internal/loadgen/request_test.go` | create | Table-driven YAML-parsing tests |

#### New Code

```go
package loadgen

import (
    "errors"
    "fmt"
    "os"

    "github.com/weiqigod/curlew/internal/httpexec"
    "gopkg.in/yaml.v3"
)

// ErrRequestFileNotFound is returned when the --file path does not exist.
var ErrRequestFileNotFound = errors.New("request file not found")

// requestFile mirrors the external-request YAML format but is intentionally
// scoped down (no assertions, no extract, no retry).
type requestFile struct {
    Name    string `yaml:"name"`
    Request struct {
        Method      string            `yaml:"method"`
        URL         string            `yaml:"url"`
        Headers     map[string]string `yaml:"headers,omitempty"`
        QueryParams map[string]string `yaml:"query,omitempty"`
        Body        any               `yaml:"body,omitempty"`
    } `yaml:"request"`
}

// LoadRequestFile reads a YAML request file and returns an httpexec.Request.
func LoadRequestFile(path string) (*httpexec.Request, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil, fmt.Errorf("%w: %s", ErrRequestFileNotFound, path)
        }
        return nil, fmt.Errorf("reading request file %s: %w", path, err)
    }
    var rf requestFile
    if err := yaml.Unmarshal(data, &rf); err != nil {
        return nil, fmt.Errorf("parsing request file %s: %w", path, err)
    }
    if rf.Request.URL == "" {
        return nil, fmt.Errorf("request file %s: missing required field 'request.url'", path)
    }
    method := rf.Request.Method
    if method == "" {
        method = "GET"
    }
    return &httpexec.Request{
        Method:      method,
        URL:         rf.Request.URL,
        Headers:     rf.Request.Headers,
        QueryParams: rf.Request.QueryParams,
        Body:        rf.Request.Body,
    }, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestLoadRequestFile(t *testing.T) {
    tests := []struct {
        name    string
        yaml    string
        wantErr error
        check   func(*testing.T, *httpexec.Request)
    }{
        {"valid GET", `name: t\nrequest:\n  method: GET\n  url: http://x\n`, nil, func(t *testing.T, r *httpexec.Request) { ... }},
        {"default method is GET", `name: t\nrequest:\n  url: http://x\n`, nil, func(...) { /* Method == "GET" */ }},
        {"missing url rejected", `name: t\nrequest:\n  method: GET\n`, errMissingURL, nil},
        {"invalid yaml rejected", `not: valid: yaml:::`, errInvalidYAML, nil},
        {"nonexistent file rejected", "", ErrRequestFileNotFound, nil},
        {"POST with body and headers", ..., nil, func(...) { /* headers + body preserved */ }},
    }
}
```

#### Impact on Existing Tests

None.

### Step 4: Register `perf_loadgen` feature (Enterprise tier)

**Rationale:** Feature gate plumbing is additive — register it before the CLI
step so the CLI can `auth.CheckFeature` without further edits.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `perf_loadgen` → Enterprise entry |
| `internal/auth/registry_test.go` | modify | Assert the feature is registered |

#### Current Code

(Inspect `internal/auth/registry.go` for the existing entry format. Follow the
same pattern used for `distributed_execution` (M5-008/M5-010) — that feature
is the closest precedent for an Enterprise-gated capability.)

#### New Code

```go
// inside DefaultRegistry() init block
features["perf_loadgen"] = Feature{
    Key:           "perf_loadgen",
    RequiredTier:  TierEnterprise,
    Message:       "Performance load generation requires the Enterprise tier.",
    UpgradeURL:    "https://curlew.dev/pricing",
    TrialAvailable: true,
    RegisterURL:   "https://curlew.dev/register",
    Workaround:    "Use a dedicated load-testing tool (e.g., k6, Vegeta) for local performance runs below Enterprise.",
}
```

(Exact field set must match the existing Feature struct — verified during
execute by reading the file.)

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry_PerfLoadgen(t *testing.T) {
    reg := auth.DefaultRegistry()
    f, ok := reg.Lookup("perf_loadgen")
    if !ok {
        t.Fatal("feature perf_loadgen not registered")
    }
    if f.RequiredTier != auth.TierEnterprise {
        t.Errorf("perf_loadgen tier = %v, want Enterprise", f.RequiredTier)
    }
}
```

#### Impact on Existing Tests

`registry_test.go` may iterate known features — if it asserts total feature
count, bump that count. Verified when the RED test first fails.

### Step 5: `cmd/curlew/perf.go` — CLI subcommand

**Rationale:** Wires parser → loadgen.Run → summary. Last functional step
because it depends on everything above.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/perf.go` | create | perfCmd, parsePerfArgs, printPerfHelp |
| `cmd/curlew/perf_test.go` | create | Flag-parsing and exit-code tests |
| `cmd/curlew/main.go` | modify | Dispatch `perf` in `run()`; update `printHelp()` |

#### Current Code (in main.go:84)

```go
switch args[0] {
case "--version":
    ...
case "worker":
    return workerCmd(args[1:])
default:
    _, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
    printHelp()
    return 1
}
```

#### New Code

```go
switch args[0] {
case "--version":
    ...
case "worker":
    return workerCmd(args[1:])
case "perf":
    return perfCmd(args[1:])
default:
    ...
}
```

And in `printHelp()` (around `main.go:2718`):

```go
fmt.Println("  worker          Run as a distributed worker (Enterprise tier)")
fmt.Println("  perf <file>     Run a load test against a single request (Enterprise tier)")
```

Plus a new `Perf Options:` block:

```go
fmt.Println("Perf Options (Enterprise tier):")
fmt.Println("  --vus <n>           Number of virtual users (concurrent workers)")
fmt.Println("  --duration <d>      Total run duration (e.g. 30s, 2m)")
fmt.Println("  --ramp-up <d>       Linearly ramp VU count from 1 to --vus over this window")
fmt.Println("  --rps <n>           Target throughput in requests/sec (0 = unbounded)")
fmt.Println("  --output <dest>     Output destination (stdout|json|html). Only 'stdout' supported in this slice.")
fmt.Println("  --help, -h          Show perf-specific help")
```

New `perf.go`:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "os"
    "os/signal"
    "strconv"
    "time"

    "github.com/weiqigod/curlew/internal/auth"
    "github.com/weiqigod/curlew/internal/httpexec"
    "github.com/weiqigod/curlew/internal/loadgen"
    "github.com/weiqigod/curlew/internal/output"
)

type perfFlags struct {
    file     string
    vus      int
    duration time.Duration
    rampUp   time.Duration
    rps      int
    output   string // "stdout" (default) — json/html deferred to M5-012
}

func perfCmd(args []string) int {
    // Grace-expiry check (mirrors runCmd).
    if code := checkGraceExpired(); code != 0 {
        return code
    }

    flags, showHelp, err := parsePerfArgs(args)
    if showHelp {
        printPerfHelp()
        return 0
    }
    if err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
        printPerfHelp()
        return 2
    }

    // Tier gate — perf_loadgen requires Enterprise.
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "perf_loadgen", currentTier()); gateErr != nil {
        var ge *auth.GateError
        if errors.As(gateErr, &ge) {
            printer := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, false))
            printer.FeatureGate(&ge.Result)
            return 6
        }
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", gateErr)
        return 1
    }

    // Output destination validation (stdout only in this slice).
    if flags.output != "" && flags.output != "stdout" {
        _, _ = fmt.Fprintf(os.Stderr,
            "error: --output %q not supported in this slice; only 'stdout' (see M5-012 for json/html)\n",
            flags.output)
        return 2
    }

    req, loadErr := loadgen.LoadRequestFile(flags.file)
    if loadErr != nil {
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", loadErr)
        return 3
    }

    cfg := loadgen.Config{
        VUs:      flags.vus,
        Duration: flags.duration,
        RampUp:   flags.rampUp,
        RPS:      flags.rps,
    }

    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    // Header line before run.
    fmt.Printf("Load test: %d virtual users, %s duration, %s ramp-up\n",
        flags.vus, flags.duration, flags.rampUp)
    if flags.rps > 0 {
        fmt.Printf("Target rate: %d req/s\n", flags.rps)
    }
    fmt.Printf("VUs: 1 ... %d (ramped in %s)\n", flags.vus, flags.rampUp)
    fmt.Println("Running...")

    sum, runErr := loadgen.Run(ctx, cfg, req, loadgen.RunOptions{})
    if runErr != nil {
        // ctx-cancellation errors are returned in Summary.Aborted; runErr
        // here means config validation or setup failed.
        if errors.Is(runErr, loadgen.ErrInvalidVUs) ||
            errors.Is(runErr, loadgen.ErrInvalidDuration) ||
            errors.Is(runErr, loadgen.ErrInvalidRampUp) ||
            errors.Is(runErr, loadgen.ErrInvalidRPS) {
            _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
            return 2
        }
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
        return 1
    }

    fmt.Printf("Requests sent: %d; successes: %d; failures: %d\n",
        sum.Requests, sum.Successes, sum.Failures)

    switch {
    case sum.Aborted:
        return 130 // SIGINT
    case sum.Failures > 0:
        return 1
    default:
        return 0
    }
}

func parsePerfArgs(args []string) (perfFlags, bool, error) {
    f := perfFlags{}
    var positional []string
    for i := 0; i < len(args); i++ {
        switch args[i] {
        case "--help", "-h":
            return f, true, nil
        case "--vus":
            i++
            if i >= len(args) {
                return f, false, fmt.Errorf("--vus requires a value")
            }
            n, err := strconv.Atoi(args[i])
            if err != nil {
                return f, false, fmt.Errorf("--vus must be an integer: %w", err)
            }
            f.vus = n
        case "--duration":
            i++
            if i >= len(args) {
                return f, false, fmt.Errorf("--duration requires a value (e.g. 30s)")
            }
            d, err := time.ParseDuration(args[i])
            if err != nil {
                return f, false, fmt.Errorf("--duration invalid: %w", err)
            }
            f.duration = d
        case "--ramp-up":
            i++
            if i >= len(args) {
                return f, false, fmt.Errorf("--ramp-up requires a value (e.g. 5s)")
            }
            d, err := time.ParseDuration(args[i])
            if err != nil {
                return f, false, fmt.Errorf("--ramp-up invalid: %w", err)
            }
            f.rampUp = d
        case "--rps":
            i++
            if i >= len(args) {
                return f, false, fmt.Errorf("--rps requires a value")
            }
            n, err := strconv.Atoi(args[i])
            if err != nil {
                return f, false, fmt.Errorf("--rps must be an integer: %w", err)
            }
            f.rps = n
        case "--output":
            i++
            if i >= len(args) {
                return f, false, fmt.Errorf("--output requires a value")
            }
            f.output = args[i]
        default:
            if len(args[i]) > 0 && args[i][0] == '-' {
                return f, false, fmt.Errorf("unknown flag: %s", args[i])
            }
            positional = append(positional, args[i])
        }
    }
    if len(positional) != 1 {
        return f, false, fmt.Errorf("perf requires exactly one positional argument: <request-file>")
    }
    f.file = positional[0]
    // Validate early for friendly exit-code-2 messages before entering loadgen.
    if f.vus < 1 {
        return f, false, fmt.Errorf("--vus must be >= 1")
    }
    if f.duration <= 0 {
        return f, false, fmt.Errorf("--duration must be > 0")
    }
    return f, false, nil
}

func printPerfHelp() {
    fmt.Println("Usage: curlew perf <request-file> [options]")
    fmt.Println()
    fmt.Println("Run a load test against a single HTTP request (Enterprise tier).")
    fmt.Println()
    fmt.Println("Required:")
    fmt.Println("  <request-file>     Path to a YAML file defining { name, request }")
    fmt.Println("  --vus <n>          Virtual users (>= 1)")
    fmt.Println("  --duration <d>     Run duration (> 0, e.g. 30s, 2m)")
    fmt.Println()
    fmt.Println("Options:")
    fmt.Println("  --ramp-up <d>      Linear ramp from 1 → <vus> over this window (default 0)")
    fmt.Println("  --rps <n>          Target throughput in requests/sec (default 0 = unbounded)")
    fmt.Println("  --output <dest>    Output destination — 'stdout' only (json/html land in M5-012)")
    fmt.Println("  --help, -h         Show this help message")
    fmt.Println()
    fmt.Println("Exit codes: 0=ok, 1=failures, 2=usage, 3=request-file error, 6=tier-gated, 130=SIGINT")
}
```

#### Tests to Write FIRST (RED phase)

`cmd/curlew/perf_test.go` — mirror the style of `cmd/curlew/worker_test.go`:

```go
func TestParsePerfArgs(t *testing.T) {
    tests := []struct {
        name    string
        args    []string
        want    perfFlags
        wantErr bool
        wantHelp bool
    }{
        {"basic vus+duration", []string{"req.yaml", "--vus", "5", "--duration", "30s"},
            perfFlags{file: "req.yaml", vus: 5, duration: 30 * time.Second}, false, false},
        {"ramp-up parsed", []string{"req.yaml", "--vus", "10", "--duration", "1m", "--ramp-up", "10s"},
            perfFlags{..., rampUp: 10 * time.Second}, false, false},
        {"rps parsed", []string{"req.yaml", "--vus", "5", "--duration", "5s", "--rps", "100"}, ..., false, false},
        {"--help", []string{"--help"}, perfFlags{}, false, true},
        {"zero vus errors", []string{"req.yaml", "--vus", "0", "--duration", "5s"}, perfFlags{}, true, false},
        {"zero duration errors", []string{"req.yaml", "--vus", "5", "--duration", "0s"}, perfFlags{}, true, false},
        {"unknown flag errors", []string{"--bogus"}, perfFlags{}, true, false},
        {"missing positional errors", []string{"--vus", "5", "--duration", "5s"}, perfFlags{}, true, false},
        {"non-integer --vus errors", []string{"req.yaml", "--vus", "abc", "--duration", "5s"}, perfFlags{}, true, false},
        {"invalid duration errors", []string{"req.yaml", "--vus", "5", "--duration", "not-a-dur"}, perfFlags{}, true, false},
    }
}

func TestPerfCmd_TierGate(t *testing.T) {
    // Set CURLEW_TIER=free, run with valid args against an httptest server,
    // assert exit code 6 and stderr contains upgrade URL.
}

func TestPerfCmd_InvalidVUsExitCode2(t *testing.T) { /* behavior 3 */ }
func TestPerfCmd_InvalidDurationExitCode2(t *testing.T) { /* behavior 4 */ }
func TestPerfCmd_Run_HTTPTestServer_Success(t *testing.T) {
    // httptest server returns 200, CURLEW_TIER=enterprise, --vus 2 --duration 200ms
    // assert: exit 0, stdout contains "Requests sent:" and "successes:"
}
func TestPerfCmd_Run_HTTPTestServer_AllFailures(t *testing.T) {
    // httptest server returns 500 → exit 1, "failures: N" in stdout
}
func TestPerfCmd_UnsupportedOutputExitCode2(t *testing.T) { /* --output html */ }
func TestPerfCmd_NonExistentFileExitCode3(t *testing.T) { /* missing file */ }
func TestPrintPerfHelp_MentionsAllFlags(t *testing.T) {
    out := captureStdout(func() { printPerfHelp() })
    for _, want := range []string{"--vus", "--duration", "--ramp-up", "--rps", "--output"} {
        if !strings.Contains(out, want) { t.Errorf(...) }
    }
}
```

Use the pattern from `cmd/curlew/worker_test.go` for `captureStdout`.

#### Impact on Existing Tests

- `cmd/curlew/main_test.go` — add/update any test that snapshots the help
  text so the new `perf` command line is expected. Check during execute
  by running the existing tests and updating snapshots as needed.
- If `run()`'s default case is covered by a "unknown command" test, no change
  needed — `perf` is now explicitly routed, so the default case still fires
  on truly unknown input.

### Step 6: Sample request file and smoke test

**Rationale:** The observable in the task YAML references
`testdata/perf/sample-request.yaml`. Deliver both pieces so the observable
and `./smoke/run.sh` run end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/perf/sample-request.yaml` | create | Minimal GET-to-httpbin.org request |
| `smoke/run.sh` | modify | Add `=== Perf --help (M5-011) ===` and end-to-end block against an ephemeral Python httptest server |

#### New testdata

```yaml
name: perf-sample
request:
  method: GET
  url: "http://127.0.0.1:8080/"
```

(Smoke spawns `python3 -m http.server 8080 &` or a Go-based httptest binary.
Other smoke blocks in `smoke/run.sh` already use this pattern — follow that.)

#### New smoke block (append near line 1748 after the M5-010 worker block)

```bash
echo "=== Perf --help (M5-011) ==="
PERF_HELP=$(./curlew perf --help 2>&1)
echo "$PERF_HELP" | grep -q "Usage: curlew perf" \
  || { echo "FAIL: perf --help missing usage line"; echo "$PERF_HELP"; exit 1; }
echo "$PERF_HELP" | grep -q -- "--vus" \
  || { echo "FAIL: perf --help missing --vus"; exit 1; }
echo "$PERF_HELP" | grep -q -- "--duration" \
  || { echo "FAIL: perf --help missing --duration"; exit 1; }
echo "$PERF_HELP" | grep -q -- "--ramp-up" \
  || { echo "FAIL: perf --help missing --ramp-up"; exit 1; }
echo "$PERF_HELP" | grep -q -- "--rps" \
  || { echo "FAIL: perf --help missing --rps"; exit 1; }
echo "$PERF_HELP" | grep -q -- "--output" \
  || { echo "FAIL: perf --help missing --output"; exit 1; }
echo "PASS: perf --help documents all expected flags"

# Invalid --vus → exit 2
SMOKE_RC=0
CURLEW_TIER=enterprise ./curlew perf testdata/perf/sample-request.yaml \
  --vus 0 --duration 1s > /dev/null 2>/tmp/curlew_perf_err_$$.txt || SMOKE_RC=$?
[ "$SMOKE_RC" -eq 2 ] && echo "PASS: --vus 0 exits 2" \
  || { echo "FAIL: --vus 0 exited $SMOKE_RC (want 2)"; cat /tmp/curlew_perf_err_$$.txt; exit 1; }
rm -f /tmp/curlew_perf_err_$$.txt

# End-to-end: start a Python http.server, run perf 2s against it.
python3 -m http.server 0 --bind 127.0.0.1 > /tmp/curlew_perf_http_$$.log 2>&1 &
PERF_PID=$!
sleep 1
PERF_PORT=$(awk '/Serving/ {print $NF}' /tmp/curlew_perf_http_$$.log | tr -d '()' | awk -F: '{print $2}')
# Edit sample to use the chosen port inline
PERF_COL="/tmp/curlew_perf_smoke_$$.yaml"
cat > "$PERF_COL" << YAML
name: perf-smoke
request:
  method: GET
  url: "http://127.0.0.1:${PERF_PORT}/"
YAML

CURLEW_TIER=enterprise PERF_OUT=$(./curlew perf "$PERF_COL" --vus 2 --duration 1s 2>&1) || SMOKE_RC=$?
echo "$PERF_OUT" | grep -q "Load test: 2 virtual users" \
  && echo "PASS: perf prints header" \
  || { echo "FAIL: perf header"; echo "$PERF_OUT"; kill $PERF_PID; exit 1; }
echo "$PERF_OUT" | grep -q "Requests sent:" \
  && echo "PASS: perf prints summary" \
  || { echo "FAIL: perf summary missing"; kill $PERF_PID; exit 1; }

kill $PERF_PID 2>/dev/null || true
rm -f "$PERF_COL" /tmp/curlew_perf_http_$$.log
```

(If `python3` is not guaranteed on CI, fall back to a small Go helper or
reuse an existing httpbin-like fixture. Verified during execute.)

#### Impact on Existing Tests

None — pure addition.

### Step 7: CHANGELOG

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | `## [Unreleased]` block — add perf line |

Add under Unreleased → Added:

```markdown
- `curlew perf <file>` load-generation subcommand (Enterprise tier) — fixed
  virtual-user pool with linear ramp-up and optional constant-RPS throughput
  mode (M5-011).
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/loadgen/loadgen_test.go` | `TestConfig_Validate` | new | add (10 cases) |
| `internal/loadgen/run_test.go` | 9 new functions | new | add |
| `internal/loadgen/request_test.go` | `TestLoadRequestFile` | new | add (6 cases) |
| `internal/auth/registry_test.go` | existing feature-count assertion | may break | bump expected count; add perf_loadgen case |
| `cmd/curlew/perf_test.go` | 10 new functions | new | add |
| `cmd/curlew/main_test.go` | any `printHelp` snapshot test | may break | update to include `perf` line |
| `smoke/run.sh` | new `=== Perf --help (M5-011) ===` block | new | add |

## Risks and Edge Cases

- **Risk: goroutine leaks on context cancel.** → **Mitigation:** every VU
  selects on `runCtx.Done()` both before and inside the RPS wait; the
  `httpexec.Execute` context is derived from `runCtx`, so `http.Client.Do`
  will return when the deadline/cancel fires. Test
  `TestRun_ContextCancelStopsVUs` asserts bounded return time.
- **Risk: flaky timing tests.** → **Mitigation:** use wide tolerance windows
  (e.g., "Requests ≥ VUs" rather than "Requests == N"; ramp-up test asserts
  bucket ordering, not exact timestamps). Each test runs in ≤ 300 ms.
- **Risk: SIGINT test pollution.** → **Mitigation:** use in-process
  `ctx.Cancel()` rather than actually signalling the process in Go tests.
  Reserve real SIGINT behaviour for smoke only.
- **Risk: `time.Ticker` resolution at high RPS.** → **Mitigation:** scope
  limits us to test cases ≤ 200 RPS. At that rate, `time.Second/200` = 5ms
  is well above Go's scheduler granularity.
- **Risk: RPS fairness across VUs.** → **Mitigation:** a shared unbuffered
  ticker channel means VUs race for each tick — that's acceptable for this
  slice; scope doesn't require work distribution guarantees.
- **Edge case: VUs = 1 with ramp-up > 0.** → **Handling:** `cfg.VUs-1` is 0,
  so ramp-up is treated as a no-op; the lone VU starts at t=0. Tested
  implicitly by `TestRun_VUsExecuteUntilDeadline` with VUs=1.
- **Edge case: ramp-up ≥ duration.** → **Handling:** Validate rejects
  `RampUp > Duration`; `RampUp == Duration` is allowed (all VUs barely make
  one request at the end, still honest behaviour).
- **Edge case: the request file path is a directory.** → **Handling:**
  `os.ReadFile` returns an error that surfaces through exit 3.
- **Edge case: request file uses `!sensitive` tags or variables.** →
  **Handling:** unsupported in this slice — unknown YAML fields are silently
  ignored by our narrow struct. Documented in help text ("{ name, request }").
- **Edge case: zero successes + zero failures.** → **Handling:** possible
  if duration < httpexec startup overhead. Exit 0 with Requests=0 (not a
  hard failure — behaviour 1 says exit 0 when no failures).

## Verification

```bash
go build ./cmd/curlew
go test ./internal/loadgen/... ./cmd/curlew/... ./internal/auth/...
go test ./...                                     # full regression
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from the task YAML):

```bash
go build ./cmd/curlew
go test ./internal/loadgen/...
# Expected: ok  internal/loadgen  (>=8 tests passing)

CURLEW_TIER=enterprise ./curlew perf testdata/perf/sample-request.yaml \
  --vus 10 --duration 5s --ramp-up 2s
# Expected stdout:
#   "Load test: 10 virtual users, 5s duration, 2s ramp-up"
#   "VUs: 1 ... 10 (ramped in 2s)"
#   "Running..."
#   "Requests sent: N; successes: N; failures: 0"
# exit 0
```

(Observable requires an HTTP server at the URL in `testdata/perf/sample-request.yaml`.
The smoke test launches `python3 -m http.server` for this; developers running
the observable locally either run the same, point the file at a real URL, or
rely on the smoke script as the canonical reproduction.)
