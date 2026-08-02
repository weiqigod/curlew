# Implementation Plan: M3-001

## Overview
Introduce a collection-level `rate_limit_rps` throttle that shares a single
token-bucket limiter across every HTTP dispatch (setup, main, teardown,
data-driven iterations, `--parallel` workers) and gate the capability behind
the new Professional-tier feature `rate_limit_global`.

## Task Details
- **ID:** M3-001
- **Title:** Global rate_limit_rps throttle across all requests
- **Phase:** M3: Professional Tier
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-013 | Basic retry logic / retry config precedence | done |

## Architectural Decisions

1. **Extract `internal/ratelimit` package** (rather than promoting the
   private `datadriven.rateLimiter`). A shared package is reusable by the
   runner and data-driven paths, keeps the token-bucket implementation in
   one place, and is independently unit-testable. `internal/datadriven`
   will delegate to `ratelimit.Limiter` instead of its private type.

2. **Wrap `exec` at the top of `runner.Run` with a rate-limited adapter**
   so every `requtil.ExecuteFunc` invocation transparently waits on the
   shared limiter. This captures:
   - Sequential HTTP in `executePhase`
   - Parallel wave HTTP in `parallel.ExecuteWaves`
   - Data-driven sequential in `executeDataDriven`
   - Data-driven parallel in `executeDataDrivenParallel`
   All four paths call the wrapped `ExecuteFunc` (directly or via
   `retry.ExecuteWithRetry`), so a single wrap point covers them without
   touching the inner execution code. Retries are naturally throttled too
   because each retry attempt goes through the wrapped `exec`.

3. **Stack the global and data-driven limiters.** The data-driven per-item
   `rate_limit_rps` remains intact inside `datadriven.ExecuteParallel`.
   The global limiter is applied inside the wrapped `exec`, so both token
   buckets must yield a token before a request fires. This matches the
   behaviour required by the "both throttles stack" behavior statement.

4. **Gate `rate_limit_global` inside `runner.Run`, before any phase
   executes.** The parser cannot evaluate tiers (it has no `auth.Tier`
   input), so "parse-time" in the task scope is interpreted as
   "before any request dispatches" — i.e., immediately after `ParseFile`
   succeeds, at the entry of `runner.Run`. This matches the existing
   pattern used by `data_driven`, `dynamic_auth_profiles`, `vault`, etc.
   The CLI observable (exit code 6) is produced by `cmd/curlew/main.go`
   already mapping `*auth.GateError` from `runner.Run` → exit 6.

5. **Parse-time validation of negative `rate_limit_rps`.** The parser
   rejects `rate_limit_rps: -1` with a structured error carrying the
   YAML line number (via the existing `apierrors.Structured{Line: ...}`
   pattern). `0` and "unset" are valid and mean "unlimited".

6. **WebSocket requests dial through the limiter.** The task scope says
   "every HTTP dispatch". WebSocket uses `websocket.Execute` rather than
   `exec`, but the connection upgrade is a single HTTP dispatch. The
   sequential WebSocket branch in `executePhase` and the parallel
   `buildWebSocketFunc` will call `limiter.Wait(ctx)` once before
   `websocket.Execute`. Data-frame traffic inside a WebSocket session is
   not throttled; only the initial upgrade counts as a request.

## Implementation Steps

### Step 1: Create `internal/ratelimit` package

**Rationale:** Smallest blast radius — new package with no consumers yet.
Everything else depends on the shared `Limiter` type existing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/ratelimit/limiter.go` | create | Exported `Limiter` type with `New(rps int) *Limiter` and `(l *Limiter) Wait(ctx context.Context) error` |
| `internal/ratelimit/limiter_test.go` | create | Table-driven behavior tests |

#### New Code

```go
// Package ratelimit provides a shared token-bucket rate limiter used to
// throttle HTTP dispatch across all request phases of a collection run.
package ratelimit

import (
    "context"
    "sync"
    "time"
)

// Limiter is a simple single-token bucket limiter. Construct with New; a nil
// *Limiter is safe to Wait on and is treated as "no throttling".
type Limiter struct {
    mu       sync.Mutex
    interval time.Duration
    last     time.Time
}

// New returns a Limiter that paces requests to at most rps per second. When
// rps <= 0 it returns nil, which represents "unlimited" (Wait on a nil
// receiver is a no-op).
func New(rps int) *Limiter {
    if rps <= 0 {
        return nil
    }
    return &Limiter{interval: time.Second / time.Duration(rps)}
}

// Wait blocks until the next token is available, or until ctx is cancelled.
// It is safe to call from multiple goroutines concurrently; tokens are
// awarded in the order goroutines acquire the mutex.
func (l *Limiter) Wait(ctx context.Context) error {
    if l == nil {
        return nil
    }
    l.mu.Lock()
    now := time.Now()
    next := l.last.Add(l.interval)
    if next.After(now) {
        delay := next.Sub(now)
        l.last = next
        l.mu.Unlock()
        t := time.NewTimer(delay)
        defer t.Stop()
        select {
        case <-t.C:
            return nil
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    l.last = now
    l.mu.Unlock()
    return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestNew(t *testing.T) {
    tests := []struct {
        name    string
        rps     int
        wantNil bool
    }{
        {"positive rps returns limiter", 10, false},
        {"zero rps returns nil (unlimited)", 0, true},
        {"negative rps returns nil (unlimited)", -1, true},
    }
    // ...
}

func TestLimiter_Wait_Throttles(t *testing.T) {
    // 100 rps → 10ms interval; 5 Waits should take >= ~30ms
}

func TestLimiter_Wait_NilReceiverIsNoop(t *testing.T) {
    var l *Limiter
    if err := l.Wait(context.Background()); err != nil {
        t.Fatalf("nil Wait error: %v", err)
    }
}

func TestLimiter_Wait_ContextCancellation(t *testing.T) {
    // 1 rps; after first Wait, cancel ctx; second Wait must return ctx.Err()
}

func TestLimiter_Wait_ConcurrentSharesBucket(t *testing.T) {
    // 10 rps across 4 goroutines doing 5 waits each (20 total):
    // wall-clock must be >= ~1.8s (19 intervals × 100ms × 80% tolerance).
}
```

#### Impact on Existing Tests
- No existing tests affected (new package).

---

### Step 2: Delegate `datadriven.rateLimiter` to `ratelimit.Limiter`

**Rationale:** Small refactor, internal to one file. Keeps data-driven
behavior identical while eliminating duplicate logic, and sets up the
"stacking" semantics (Step 6) by letting the global limiter sit on top of
the datadriven one without code duplication.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/datadriven/parallel.go` | modify | Remove private `rateLimiter`/`newRateLimiter`; use `ratelimit.New` and `*ratelimit.Limiter.Wait` |
| `internal/datadriven/parallel_test.go` | modify | Rewrite `TestRateLimiter_*` tests to cover the shared package via the datadriven integration point only (the limiter-specific tests live in `internal/ratelimit/limiter_test.go`) |

#### Current Code
```go
// internal/datadriven/parallel.go
type rateLimiter struct { ... }
func newRateLimiter(rps int) *rateLimiter { ... }
func (rl *rateLimiter) wait(ctx context.Context) error { ... }

// usage inside ExecuteParallel
rl := newRateLimiter(cfg.RateLimitRPS)
...
if err := rl.wait(runCtx); err != nil { break }
```

#### New Code
```go
// internal/datadriven/parallel.go
import "github.com/weiqigod/curlew/internal/ratelimit"

// usage
rl := ratelimit.New(cfg.RateLimitRPS)
...
if err := rl.Wait(runCtx); err != nil { break }
```

#### Tests to Write FIRST (RED phase)
- Keep `TestExecuteParallel_RateLimit` — still exercises the throttle via
  `cfg.RateLimitRPS`. Should pass unchanged because behavior is preserved.
- Delete the three `TestRateLimiter_*` tests in `parallel_test.go` — they
  become redundant with the new `internal/ratelimit/limiter_test.go`.

#### Impact on Existing Tests
- `TestRateLimiter_NilWhenDisabled`, `TestRateLimiter_ThrottlesRequests`,
  `TestRateLimiter_ContextCancellation` (`parallel_test.go`) — delete;
  equivalent coverage moves to `internal/ratelimit/limiter_test.go`.
- `TestExecuteParallel_RateLimit` — must keep passing with the new
  delegation; no changes to the test body.

---

### Step 3: Add `RateLimitRPS` to `parser.Collection` with validation

**Rationale:** Add the YAML field and parse-time validation before any
runner changes so the downstream code has something real to read.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `RateLimitRPS int` with tag `rate_limit_rps` to `Collection` |
| `internal/parser/parser.go` | modify | Validate non-negative `rate_limit_rps` with a structured error that includes the YAML line number |
| `internal/parser/parser_test.go` | modify | Add parsing + validation tests |
| `internal/parser/testdata/rate_limit_global.yaml` | create | Fixture: `rate_limit_rps: 5` |
| `internal/parser/testdata/rate_limit_negative.yaml` | create | Fixture: `rate_limit_rps: -1` |

#### Current Code
```go
// internal/parser/collection.go
type Collection struct {
    Name          string            `yaml:"name"`
    ...
    Options       Options           `yaml:"options,omitempty"`
    ExternalFiles []string          `yaml:"-"`
}
```

#### New Code
```go
type Collection struct {
    Name          string            `yaml:"name"`
    ...
    Options       Options           `yaml:"options,omitempty"`
    RateLimitRPS  int               `yaml:"rate_limit_rps,omitempty"` // 0 = unlimited; >0 = token-bucket throttle (Professional tier)
    ExternalFiles []string          `yaml:"-"`
}
```

Validation is added in `parser.ParseFile` after the existing
`validateRequests` loop but before `return &col, nil`. It needs access to
the raw YAML node for the line number, so we thread a secondary pass that
walks the top-level document node when `col.RateLimitRPS < 0`:

```go
// In ParseFile, after yaml.Unmarshal and before returning:
if col.RateLimitRPS < 0 {
    line := findTopLevelKeyLine(data, "rate_limit_rps")
    return nil, &apierrors.Structured{
        Category: apierrors.CategoryParse,
        FilePath: path,
        Line:     line,
        Message:  fmt.Sprintf("rate_limit_rps must be >= 0, got %d", col.RateLimitRPS),
        Hint:     "Use 0 (or omit the field) to disable throttling",
        Inner:    ErrInvalidFieldValue,
    }
}
```

`findTopLevelKeyLine` is a small helper that re-parses `data` as a
`yaml.Node` document and scans the top-level mapping for the given key,
returning its line number. It lives in `parser.go` alongside
`extractYAMLLine`. (The alternative — decoding into a `yaml.Node` up-front
and walking it for every top-level field — is rejected because it
touches the hot path for every parse.)

#### Tests to Write FIRST (RED phase)
```go
func TestParseFile_rateLimitRPS(t *testing.T) {
    tests := []struct {
        name     string
        fixture  string
        wantRPS  int
        wantErr  bool
        wantLine int
    }{
        {"parses positive rate_limit_rps", "testdata/rate_limit_global.yaml", 5, false, 0},
        {"unset defaults to 0", "testdata/minimal.yaml", 0, false, 0},
        {"negative rejected with line number", "testdata/rate_limit_negative.yaml", 0, true, 2},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            col, err := ParseFile(tt.fixture)
            if tt.wantErr {
                if err == nil {
                    t.Fatal("expected error, got nil")
                }
                var se *apierrors.Structured
                if !errors.As(err, &se) {
                    t.Fatalf("expected *apierrors.Structured, got %T", err)
                }
                if se.Line != tt.wantLine {
                    t.Errorf("Line = %d, want %d", se.Line, tt.wantLine)
                }
                if !errors.Is(err, ErrInvalidFieldValue) {
                    t.Errorf("expected ErrInvalidFieldValue, got %v", err)
                }
                return
            }
            if err != nil {
                t.Fatalf("ParseFile: %v", err)
            }
            if col.RateLimitRPS != tt.wantRPS {
                t.Errorf("RateLimitRPS = %d, want %d", col.RateLimitRPS, tt.wantRPS)
            }
        })
    }
}
```

Fixture `rate_limit_global.yaml`:
```yaml
name: Rate Limited
rate_limit_rps: 5
requests:
  - name: A
    request:
      method: GET
      url: https://example.com
```

Fixture `rate_limit_negative.yaml`:
```yaml
name: Bad
rate_limit_rps: -1
requests:
  - name: A
    request:
      method: GET
      url: https://example.com
```

#### Impact on Existing Tests
- None. The new field has a zero-value default that leaves existing
  fixtures untouched.

---

### Step 4: Register `rate_limit_global` as a Professional feature

**Rationale:** Isolated registry addition; needed before the runner can
call `CheckFeature("rate_limit_global", ...)`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Register `rate_limit_global` at Professional tier |
| `internal/auth/registry_test.go` | modify | Extend `TestDefaultRegistry` with the new feature |

#### Current Code
```go
// internal/auth/registry.go — inside DefaultRegistry()
r.Register(FeatureDefinition{
    Name:         "protocol_websocket",
    ...
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "protocol_websocket",
    ...
})
r.Register(FeatureDefinition{
    Name:         "rate_limit_global",
    RequiredTier: TierProfessional,
    Description:  "Global rate_limit_rps requires Professional tier ($19/month)",
    Workaround:   "Use per-request pacing in your own scripts, or the data-driven rate_limit_rps for single requests",
})
return r
```

#### Tests to Write FIRST (RED phase)
Add `{"contains rate_limit_global", "rate_limit_global", true}` to the
`TestDefaultRegistry` table.

#### Impact on Existing Tests
- `TestDefaultRegistry` — extended, still passes.

---

### Step 5: Gate and apply the limiter in `runner.Run`

**Rationale:** This is the central wiring step. It depends on the three
preceding steps (package, collection field, registry entry). Blast radius
is contained to `runner.go` and its test file.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Gate `rate_limit_global`, build shared limiter, wrap `exec` |
| `internal/runner/runner_test.go` | modify | Add behavior tests |

#### New Code (runner.go, inside `Run`)

Insert after `buildScope(...)` succeeds and before auth profile execution:

```go
// Feature gate for rate_limit_global (Professional tier).
if col.RateLimitRPS > 0 {
    reg := vars.Registry
    if reg == nil {
        reg = auth.DefaultRegistry()
    }
    tier := vars.Tier
    if tier == "" {
        tier = auth.TierFree
    }
    if gateErr := auth.CheckFeature(reg, "rate_limit_global", tier); gateErr != nil {
        return nil, emptySummary, gateErr
    }
}

// Build the shared global limiter and wrap exec so every HTTP dispatch
// — sequential, parallel, and data-driven — waits on the same token bucket.
globalLimiter := ratelimit.New(col.RateLimitRPS)
wrappedExec := exec
if globalLimiter != nil {
    wrappedExec = func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
        if err := globalLimiter.Wait(ctx); err != nil {
            return nil, err
        }
        return exec(ctx, req)
    }
}
```

All subsequent calls in `Run` (auth profile execution path, `runPhases`,
`RunForExtraction` recursive calls) use `wrappedExec` in place of `exec`.
`RunForExtraction` (which is called by auth profile execution) inherits
the wrapper naturally because the wrapped func is what we pass down.

For the WebSocket branches in `executePhase` and `buildWebSocketFunc`,
add a `ctx` check via the limiter immediately before `websocket.Execute`.
The simplest way to share the limiter with those branches is to add a
`globalLimiter *ratelimit.Limiter` field to `VarSources` (populated by
`Run` before calling `runPhases`). This keeps the plumbing minimal — no
new parameters on `runPhases`, `executePhase`, `executeParallelMain`,
etc. — and preserves the invariant that `VarSources` carries per-run
state.

```go
// VarSources addition:
// globalLimiter is internal state populated by Run; callers should leave it nil.
globalLimiter *ratelimit.Limiter
```

Then inside `executePhase`'s WebSocket branch:
```go
if vars.globalLimiter != nil {
    if err := vars.globalLimiter.Wait(ctx); err != nil {
        // surface as a context-cancelled skip
        results = append(results, RequestResult{Name: item.Name, Phase: phase, Skipped: true, WaveIndex: -1})
        stopped = true
        continue
    }
}
*counter++
...
wsRes := websocket.Execute(ctx, &req, reqScope, dialer)
```

and mirror the same in `buildWebSocketFunc`.

#### Tests to Write FIRST (RED phase)

```go
// Behavior: "rate_limit_rps: 5" with 10 sequential requests takes >= 1.8s
func TestRun_GlobalRateLimit_ThrottlesSequential(t *testing.T) {
    calls := 0
    exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
        calls++
        return &httpexec.Result{StatusCode: 200}, nil
    }
    col := &parser.Collection{
        Name: "Throttled", RateLimitRPS: 5,
        Requests: parser.Section{Items: /* 10 GET requests */},
    }
    start := time.Now()
    _, summary, err := Run(context.Background(), col, exec, VarSources{Tier: auth.TierProfessional})
    elapsed := time.Since(start)
    // 10 requests @ 5 rps = 9 intervals × 200ms = 1.8s; allow 80% tolerance → 1.44s
    if elapsed < 1440*time.Millisecond { t.Errorf(...) }
    if summary.Passed != 10 { t.Errorf(...) }
}

// Behavior: "rate_limit_rps: 0 or unset" applies no throttle
func TestRun_GlobalRateLimit_ZeroIsUnlimited(t *testing.T) {
    // 100 requests must complete in well under 100ms (no sleeps).
}

// Behavior: combined with --parallel, workers share the same bucket
func TestRun_GlobalRateLimit_ParallelSharesBucket(t *testing.T) {
    // 10 independent requests, parallel: true, rate_limit_rps: 5 → >= 1.44s
}

// Behavior: combined with data_driven rate_limit, both stack
func TestRun_GlobalRateLimit_StacksWithDataDriven(t *testing.T) {
    // collection rate_limit_rps: 5, data_driven rate_limit_rps: 20 over 5 rows
    // → wall clock dominated by the slower (global) limiter: >= 4 × 200ms × 0.8 = 640ms
}

// Behavior: Free tier triggers exit-code-6 gate
func TestRun_GlobalRateLimit_FreeTierGated(t *testing.T) {
    col := &parser.Collection{Name: "X", RateLimitRPS: 100, Requests: /*...*/}
    _, _, err := Run(context.Background(), col, exec, VarSources{Tier: auth.TierFree})
    var ge *auth.GateError
    if !errors.As(err, &ge) { t.Fatalf(...) }
    if ge.Result.Feature != "rate_limit_global" { t.Errorf(...) }
}

// Behavior: rate_limit_rps = 0 does NOT trigger gate
func TestRun_GlobalRateLimit_ZeroDoesNotGate(t *testing.T) {
    // Free tier + rate_limit_rps: 0 → no gate, request runs.
}

// Behavior: context cancellation while sleeping returns promptly
func TestRun_GlobalRateLimit_ContextCancellation(t *testing.T) {
    // rate_limit_rps: 1; fire 3 requests in goroutine; cancel after first completes;
    // expect Run to return within ~100ms (not wait for the next 1-second token).
}
```

Test fixtures reuse an inline stub exec that returns 200 OK immediately.

#### Impact on Existing Tests
- `TestRun_DataDriven_ParallelRateLimit` — unchanged; uses the
  data-driven field, not the collection field. Still passes.
- No other existing `runner_test.go` test sets `col.RateLimitRPS`, so no
  regressions expected. Existing tests with unset `RateLimitRPS` fall
  through the `globalLimiter != nil` guard and behave identically.
- `TestRun_DataDriven_FeatureGate_Free` and similar gate tests — unaffected.

---

### Step 6: Update JSON schema

**Rationale:** Last. A schema update has the smallest impact: it affects
only the `validator` and `curlew validate` paths and would otherwise
reject the new field.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/schema/collection.json` | modify | Add top-level `rate_limit_rps` property |
| `internal/schema/schema_test.go` | modify | Add assertion that a collection with `rate_limit_rps: 5` validates |

#### New Code
```json
"options": { ... },
"rate_limit_rps": {
  "type": "integer",
  "minimum": 0,
  "description": "Global token-bucket rate limit in requests per second (Professional tier). 0 = unlimited."
}
```

#### Tests to Write FIRST (RED phase)
Add a schema_test case that validates a minimal collection with
`rate_limit_rps: 5` and a negative case where `rate_limit_rps: -1` is
rejected by the JSON schema validator.

#### Impact on Existing Tests
- None. `additionalProperties: false` at the top level means the new
  property must be declared explicitly — otherwise existing tests with
  `rate_limit_rps` would fail. Tests without the field are unaffected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action |
|-----------|--------------|--------|--------|
| `internal/ratelimit/limiter_test.go` | all new | add | write first (RED) |
| `internal/datadriven/parallel_test.go` | `TestRateLimiter_NilWhenDisabled` | remove | equivalent coverage in ratelimit package |
| `internal/datadriven/parallel_test.go` | `TestRateLimiter_ThrottlesRequests` | remove | equivalent coverage in ratelimit package |
| `internal/datadriven/parallel_test.go` | `TestRateLimiter_ContextCancellation` | remove | equivalent coverage in ratelimit package |
| `internal/datadriven/parallel_test.go` | `TestExecuteParallel_RateLimit` | none | still passes after delegation |
| `internal/parser/parser_test.go` | `TestParseFile_rateLimitRPS` | add | new (parsing + validation) |
| `internal/parser/parser_test.go` | `TestParseFile_dataDrivenParallelConfig` | none | orthogonal (per-item field) |
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | extend | add feature name assertion |
| `internal/runner/runner_test.go` | `TestRun_GlobalRateLimit_*` | add (7 new) | behavior tests |
| `internal/runner/runner_test.go` | `TestRun_DataDriven_ParallelRateLimit` | none | orthogonal |
| `internal/schema/schema_test.go` | schema test case | add | accepts `rate_limit_rps: 5`; rejects `-1` |

## Risks and Edge Cases

- **Risk:** Timing-based tests are flaky on slow CI runners. **Mitigation:**
  Use the same `× 80/100` tolerance factor already applied by
  `TestRun_DataDriven_ParallelRateLimit`, and keep the RPS values high
  enough (≥5) that intervals stay in the 100ms–200ms range rather than
  multi-second waits.

- **Risk:** Context cancellation inside `Limiter.Wait` may leave
  `l.last` set to a future time, slightly penalising the next request
  after a cancelled context. **Mitigation:** Acceptable — after ctx.Err
  the caller aborts the run. If the limiter is reused across runs (not
  our case), this would need resetting.

- **Edge case:** `rate_limit_rps: 0` must NOT trigger the feature gate
  (no throttle requested). **Handling:** Gate check is inside
  `if col.RateLimitRPS > 0 { ... }`.

- **Edge case:** `rate_limit_rps` combined with `stop_on_failure` or
  `required` — when a failing request stops the run mid-way, no extra
  tokens should be consumed. **Handling:** The wrapped `exec` only waits
  when a request is actually dispatched; skipped requests bypass it.

- **Edge case:** Auth-profile execution (`auth.ExecuteProfiles`) runs its
  own child collection via `RunForExtraction`. Auth profile HTTP requests
  count against the limiter too, because `authExec` ultimately calls
  `exec` which we've wrapped. **Handling:** Intentional and correct —
  auth requests are real HTTP dispatches.

- **Edge case:** `--parallel` with max parallelism > global rps.
  Multiple workers will serialise on the limiter's mutex; throughput
  remains bounded by the bucket. **Handling:** Covered by
  `TestLimiter_Wait_ConcurrentSharesBucket` and
  `TestRun_GlobalRateLimit_ParallelSharesBucket`.

- **Edge case:** Refresh-on-failure retry in `executePhase` calls `exec`
  directly (outside `retry.ExecuteWithRetry`). Because we wrap `exec`
  itself, the refresh dispatch is still throttled. **Handling:** Covered
  automatically by the wrap point.

- **Edge case:** Data-driven parallel inside global-parallel wave — the
  data-driven per-iteration limiter runs inside `datadriven.ExecuteParallel`,
  and the global limiter inside the wrapped `exec`. Worker parallelism is
  bounded by both. **Handling:** Stacking is validated by
  `TestRun_GlobalRateLimit_StacksWithDataDriven`.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/ratelimit/... ./internal/parser/... ./internal/datadriven/... \
        ./internal/auth/... ./internal/runner/... ./internal/schema/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# Professional tier: 10 requests at 5 rps should take >= ~1.8s wall-clock
go build ./cmd/curlew
CURLEW_TIER=professional ./curlew run smoke/rate_limit_global.yaml  # >= ~1.8s

# Free tier: same collection exits 6 with "rate_limit_global requires Professional tier"
CURLEW_TIER=free ./curlew run smoke/rate_limit_global.yaml          # exit code 6
echo $?

go test ./internal/runner/... ./internal/parser/...
```

A minimal `smoke/rate_limit_global.yaml` fixture pointing at a local
test server is the observable artefact. If the smoke runner doesn't ship
a local throttle test server, the observable can be verified with a
Go test that uses `httptest.NewServer`.
