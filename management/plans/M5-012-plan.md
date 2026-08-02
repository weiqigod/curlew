# Implementation Plan: M5-012

## Overview

Turn the `apitest perf` subcommand into a real observability tool: per-request
latency samples flowing from `internal/loadgen` into a new
`internal/loadgen/report` subpackage that computes p50/p95/p99, error rate,
throughput, and a one-second-resolution time series, then emits the result as a
summary line on stdout plus an optional JSON or self-contained HTML report
(Chart.js via CDN script tag).

## Task Details

- **ID:** M5-012
- **Title:** go-cli: perf metrics and HTML/JSON report
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task   | Title                                                      | Status |
|--------|------------------------------------------------------------|--------|
| M5-011 | go-cli: load generation mode (virtual users, ramp profile) | done   |

## Architectural Decisions

These decisions resolve ambiguities in the task's `scope` field and close the
open questions the Plan phase surfaced.

1. **Metric stream shape.** `loadgen.Run` already records counters in-place.
   This slice adds a single optional `OnSample func(Sample)` hook on
   `loadgen.RunOptions` that is invoked once per completed request with the
   request's start offset (relative to run start), latency, and outcome. A
   callback beats a channel here because (a) it needs no extra goroutine or
   backpressure handling, (b) the hook is called from the same VU goroutine so
   the report aggregator can either lock-protect its state or (our choice) run
   behind a mutex-guarded `Aggregator` that the CLI wires up once. `Sample`
   records integer nanoseconds (`StartOffsetNs`, `LatencyNs`) to keep the zero
   value inert and avoid allocating a `time.Time` per request. Tests can pass
   their own `OnSample` for deterministic sample sets.

2. **Percentile algorithm.** Simple in-memory buffer of `[]time.Duration`
   followed by `sort.Slice` and nearest-rank percentile
   (`samples[ceil(p*N)-1]`). No external dependency (rejecting the
   `influxdata/tdigest` suggestion in the scope) — `minimise external
   dependencies` is a hard rule in `CLAUDE.md`. Memory cost for a 10-minute run
   at 1000 RPS is 10ms × 600k samples × 16 B ≈ 9.6 MB; acceptable for M5.
   Comment documents this bound and the future swap point if we ever need
   streaming quantiles.

3. **Time-series resolution.** Buckets of 1 second indexed by
   `floor(startOffset / 1s)`. Each bucket tracks `{count, success, sumLatency,
   minLatency, maxLatency, sortedSamples []time.Duration}`. At render time the
   bucket's p95 is computed from its local samples (or skipped when the bucket
   is empty). This keeps memory bounded at one tiny struct per run-second.

4. **Output destination.** `--output` is promoted from "stdout only" to a
   first-class flag. Routing decided by the **file extension** of the flag
   value (after stripping leading `./`): `.json` → JSON, `.html` → HTML,
   otherwise the literal string `"stdout"` (default) means "print summary only;
   write nothing to disk". Anything else is rejected with the exact text
   `error: unsupported report format .xyz` on stderr and exit 2 (matches
   behavior 5). File writes truncate-and-overwrite via `os.WriteFile` (matches
   behavior 8). `--format` is **not** added this slice — the spec uses
   `--format` for the run-level JSON/TAP/JUnit output, and overloading it on
   perf would be confusing. The task YAML's `definition_of_done` mentions
   `--format` but the behaviors only require `--output`; we satisfy
   `definition_of_done` by documenting `--output` (which controls format via
   extension) in help text.

5. **HTML report shape.** Standalone HTML file; embedded CSS; a single
   `<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1"></script>` CDN
   tag (scope explicitly allows CDN; `--offline` bundling is deferred). One
   latency-vs-time line chart (p50 + p95 per second) and an error-rate overlay.
   Summary metric cards (total, p50, p95, p99, error rate, throughput) mirror
   the existing `internal/output/html.go` card pattern but are rendered from a
   new self-contained template in `internal/loadgen/report/html.go` — the perf
   report is structurally very different from the run report (time-series
   centric, no assertions) and sharing the existing template would force awkward
   optional-every-field contortions.

6. **Small-sample warning.** Behavior 6 pins a specific failure-mode: when
   `len(samples) < 100`, p99 is not well-defined under nearest-rank, so we
   fall back to max and append
   ` (p99 may be imprecise for small samples)` to the stdout summary line and
   set a `SmallSampleWarning bool` in the JSON/HTML data so the UI can show a
   note card.

7. **Unsupported-extension error path.** Detected **before** the run starts
   (fail fast), from `cmd/apitest/perf.go`. `cmd/apitest` stays the sole owner
   of CLI exit codes; the report subpackage has no knowledge of `os.Exit`.
   The existing exit-code-2 path in `perf.go` is rewritten to call a new
   `reportFormatFromOutput(flag string) (report.Format, error)` helper so the
   format dispatch is unit-testable.

8. **Stdout summary wording.** Task observable says:
   ```
   Results: requests=N, p50=Xms, p95=Yms, p99=Zms, error_rate=0%
   ```
   We emit exactly this line plus a second line
   `Wrote perf-report.json` (or `.html`) when `--output` is a file path. The
   existing `Requests sent:` line from M5-011 is preserved above the new
   `Results:` line so older smoke assertions keep passing.

9. **Package boundary.** New `internal/loadgen/report` package. Exports:
   - `type Sample struct { StartOffsetNs, LatencyNs int64; Success bool }`
   - `type Aggregator` with `Observe(Sample)` (thread-safe), `Metrics() Metrics`
   - `type Metrics struct { Requests int; P50, P95, P99 time.Duration; ErrorRate float64; Throughput float64; Elapsed time.Duration; SmallSampleWarning bool; Buckets []Bucket }`
   - `type Bucket struct { SecondOffset int; Requests int; Errors int; P50, P95 time.Duration }`
   - `type Format int` with `FormatStdout`, `FormatJSON`, `FormatHTML`
   - `DetectFormat(outputFlag string) (Format, string, error)` — returns the format, the cleaned path, and a sentinel `ErrUnsupportedFormat` on `.xyz` etc.
   - `WriteJSON(w io.Writer, m Metrics, title string) error`
   - `WriteHTML(w io.Writer, m Metrics, title string) error`
   - `SummaryLine(m Metrics) string` — the one-liner for stdout.
   `Sample` also lives in `internal/loadgen` (as `loadgen.Sample`) to avoid an
   import cycle: `loadgen` emits samples but does not import `report`.
   `report.Observe` accepts the local `report.Sample` type and the caller
   converts; since both are plain structs of int64s this costs nothing.

10. **Sentinel errors.**
    - `loadgen/report.ErrUnsupportedFormat`
    - `loadgen/report.ErrNilMetrics`
    Wrapped via `fmt.Errorf("...: %w", err)` at call sites.

## Implementation Steps

Steps are ordered **smallest blast radius first**: leaf utilities, then the
aggregator, then loadgen plumbing, then CLI wiring, finally the HTML template.
Every step starts with a failing test.

### Step 1: `Sample` type and format detection

**Rationale:** pure functions with no external state; fastest path to green
tests and the foundation every later step builds on.

#### Files to Modify

| File                                       | Action | Description                                                            |
|--------------------------------------------|--------|------------------------------------------------------------------------|
| `internal/loadgen/sample.go`               | create | `Sample{StartOffsetNs, LatencyNs int64; Success bool}` struct          |
| `internal/loadgen/report/format.go`        | create | `Format` enum, `DetectFormat`, `ErrUnsupportedFormat`                  |
| `internal/loadgen/report/format_test.go`   | create | Table-driven tests for DetectFormat                                    |

#### Current Code

`internal/loadgen/sample.go` does not exist.
`internal/loadgen/report/` does not exist.

#### New Code

`internal/loadgen/sample.go`:
```go
package loadgen

// Sample is one completed request in a perf run. StartOffsetNs is measured
// from the start of Run (not wall time). Zero value is a valid "not yet set"
// sentinel used only inside tests.
type Sample struct {
    StartOffsetNs int64
    LatencyNs     int64
    Success       bool
}
```

`internal/loadgen/report/format.go`:
```go
package report

import (
    "errors"
    "fmt"
    "path/filepath"
    "strings"
)

// Format identifies the report output format selected by the user.
type Format int

const (
    FormatStdout Format = iota // default — summary line only
    FormatJSON                 // .json file
    FormatHTML                 // .html file
)

// ErrUnsupportedFormat is returned by DetectFormat for extensions other than
// .json, .html, or the literal string "stdout".
var ErrUnsupportedFormat = errors.New("unsupported report format")

// DetectFormat parses --output and returns the format plus the cleaned file
// path (empty for FormatStdout). An empty flag value defaults to stdout.
func DetectFormat(flag string) (Format, string, error) {
    if flag == "" || flag == "stdout" {
        return FormatStdout, "", nil
    }
    ext := strings.ToLower(filepath.Ext(flag))
    switch ext {
    case ".json":
        return FormatJSON, flag, nil
    case ".html":
        return FormatHTML, flag, nil
    default:
        return FormatStdout, "", fmt.Errorf("%w %s", ErrUnsupportedFormat, ext)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestDetectFormat(t *testing.T) {
    tests := []struct {
        name    string
        flag    string
        want    Format
        wantPath string
        wantErr bool
    }{
        {"empty flag defaults to stdout", "", FormatStdout, "", false},
        {"literal stdout string", "stdout", FormatStdout, "", false},
        {"json extension", "report.json", FormatJSON, "report.json", false},
        {"html extension", "out/perf.html", FormatHTML, "out/perf.html", false},
        {"uppercase extension treated as case-insensitive", "R.JSON", FormatJSON, "R.JSON", false},
        {"unsupported extension xyz", "report.xyz", FormatStdout, "", true},
        {"no extension", "perf-report", FormatStdout, "", true},
        {"hidden file without extension", ".perf", FormatStdout, "", true},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            f, p, err := DetectFormat(tc.flag)
            if tc.wantErr {
                if !errors.Is(err, ErrUnsupportedFormat) {
                    t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected err: %v", err)
            }
            if f != tc.want {
                t.Errorf("format = %v, want %v", f, tc.want)
            }
            if p != tc.wantPath {
                t.Errorf("path = %q, want %q", p, tc.wantPath)
            }
        })
    }
}
```

#### Impact on Existing Tests

None — new package, new type.

---

### Step 2: `Aggregator` + `Metrics` with percentile math

**Rationale:** pure aggregation logic, no I/O; unit-testable with synthetic
samples; all later steps read from its output.

#### Files to Modify

| File                                         | Action | Description                                                                        |
|----------------------------------------------|--------|------------------------------------------------------------------------------------|
| `internal/loadgen/report/metrics.go`         | create | `Sample`, `Bucket`, `Metrics`, `Aggregator` with `Observe` / `Metrics()`            |
| `internal/loadgen/report/metrics_test.go`    | create | Percentile math, bucketing, error-rate, small-sample warning                        |

#### New Code

```go
package report

import (
    "math"
    "sort"
    "sync"
    "time"
)

// Sample is the report-facing view of one completed request.
type Sample struct {
    StartOffsetNs int64
    LatencyNs     int64
    Success       bool
}

// Bucket is the per-second time-series entry.
type Bucket struct {
    SecondOffset int
    Requests     int
    Errors       int
    P50          time.Duration
    P95          time.Duration
}

// Metrics is the aggregated result of a perf run.
type Metrics struct {
    Requests           int
    Successes          int
    Failures           int
    Elapsed            time.Duration
    P50, P95, P99      time.Duration
    Min, Max, Mean     time.Duration
    ErrorRate          float64 // 0..1
    Throughput         float64 // req/s
    SmallSampleWarning bool
    Buckets            []Bucket
}

// Aggregator collects samples from a running perf load test. It is safe for
// concurrent use by multiple VU goroutines.
type Aggregator struct {
    mu       sync.Mutex
    samples  []time.Duration       // all latencies, for global percentiles
    buckets  map[int]*bucketState  // keyed by second offset
    requests int
    successes int
    failures  int
    lastOffset int64 // ns; used to compute Elapsed if callers do not supply it
}

type bucketState struct {
    requests  int
    errors    int
    latencies []time.Duration
}

// New returns a ready-to-use Aggregator.
func New() *Aggregator {
    return &Aggregator{buckets: make(map[int]*bucketState)}
}

// Observe records one sample. Thread-safe; VU goroutines may call concurrently.
func (a *Aggregator) Observe(s Sample) {
    a.mu.Lock()
    defer a.mu.Unlock()

    lat := time.Duration(s.LatencyNs)
    a.samples = append(a.samples, lat)
    a.requests++
    if s.Success {
        a.successes++
    } else {
        a.failures++
    }
    if s.StartOffsetNs > a.lastOffset {
        a.lastOffset = s.StartOffsetNs
    }

    sec := int(s.StartOffsetNs / int64(time.Second))
    b, ok := a.buckets[sec]
    if !ok {
        b = &bucketState{}
        a.buckets[sec] = b
    }
    b.requests++
    if !s.Success {
        b.errors++
    }
    b.latencies = append(b.latencies, lat)
}

// Metrics returns an immutable snapshot. Elapsed is derived from the largest
// observed start offset plus the matching sample's latency, which is a good
// proxy when the caller does not pass a separate elapsed.
func (a *Aggregator) Metrics(elapsed time.Duration) Metrics {
    a.mu.Lock()
    defer a.mu.Unlock()

    m := Metrics{
        Requests:  a.requests,
        Successes: a.successes,
        Failures:  a.failures,
        Elapsed:   elapsed,
    }
    if a.requests == 0 {
        return m
    }

    sorted := append([]time.Duration(nil), a.samples...)
    sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

    m.Min = sorted[0]
    m.Max = sorted[len(sorted)-1]
    m.P50 = percentile(sorted, 0.50)
    m.P95 = percentile(sorted, 0.95)
    if len(sorted) < 100 {
        m.P99 = m.Max
        m.SmallSampleWarning = true
    } else {
        m.P99 = percentile(sorted, 0.99)
    }

    var total time.Duration
    for _, d := range sorted {
        total += d
    }
    m.Mean = total / time.Duration(len(sorted))

    m.ErrorRate = float64(a.failures) / float64(a.requests)
    if elapsed > 0 {
        m.Throughput = float64(a.requests) / elapsed.Seconds()
    }

    // Buckets: sorted by SecondOffset for deterministic JSON / HTML.
    keys := make([]int, 0, len(a.buckets))
    for k := range a.buckets {
        keys = append(keys, k)
    }
    sort.Ints(keys)
    m.Buckets = make([]Bucket, 0, len(keys))
    for _, k := range keys {
        bs := a.buckets[k]
        local := append([]time.Duration(nil), bs.latencies...)
        sort.Slice(local, func(i, j int) bool { return local[i] < local[j] })
        m.Buckets = append(m.Buckets, Bucket{
            SecondOffset: k,
            Requests:     bs.requests,
            Errors:       bs.errors,
            P50:          percentile(local, 0.50),
            P95:          percentile(local, 0.95),
        })
    }
    return m
}

// percentile uses nearest-rank on a pre-sorted slice. 0 <= p <= 1. For empty
// input returns 0.
func percentile(sorted []time.Duration, p float64) time.Duration {
    if len(sorted) == 0 {
        return 0
    }
    rank := int(math.Ceil(p*float64(len(sorted)))) - 1
    if rank < 0 {
        rank = 0
    }
    if rank >= len(sorted) {
        rank = len(sorted) - 1
    }
    return sorted[rank]
}
```

#### Tests to Write FIRST

```go
func TestAggregator_EmptyReturnsZeroMetrics(t *testing.T) { /* ... */ }

func TestAggregator_PercentilesNearestRank(t *testing.T) {
    // 100 samples 1..100ms; p50 -> 50ms; p95 -> 95ms; p99 -> 99ms
}

func TestAggregator_SmallSampleFallback(t *testing.T) {
    // 10 samples -> P99 == Max and SmallSampleWarning == true
}

func TestAggregator_ErrorRate(t *testing.T) {
    // 10 samples, 3 failures -> ErrorRate == 0.3
}

func TestAggregator_Throughput(t *testing.T) {
    // 100 requests across 2s elapsed -> 50 req/s
}

func TestAggregator_BucketsByStartOffset(t *testing.T) {
    // Samples at 0ms,500ms,999ms -> bucket 0 has 3 requests
    // Samples at 1_000ms, 1_500ms -> bucket 1 has 2 requests
    // Verify SecondOffset ordering is 0 then 1.
}

func TestAggregator_ConcurrentObserve(t *testing.T) {
    // 8 goroutines each calling Observe 100x; assert total count after wg.Wait.
    // Run under go test -race.
}
```

| Case name                          | Samples                               | Assertion                                 |
|------------------------------------|---------------------------------------|-------------------------------------------|
| empty returns zero metrics         | none                                  | Requests=0, P50=0                         |
| 100 samples nearest-rank           | 1ms..100ms                            | P50=50ms, P95=95ms, P99=99ms, warn=false  |
| small sample fallback              | 10 samples 1ms..10ms                  | P99=Max=10ms, warn=true                   |
| error rate 30%                     | 10 samples, 3 failures                | ErrorRate=0.3                             |
| throughput 50 req/s                | 100 requests, elapsed=2s              | Throughput=50.0                           |
| bucket assignment                  | offsets 0, 500ms, 999ms, 1s, 1.5s     | Buckets[0].Requests=3, [1].Requests=2     |
| concurrent Observe race-free       | 8 goroutines × 100 obs                | Requests=800, no race                     |

#### Impact on Existing Tests

None — new package.

---

### Step 3: `SummaryLine` + `WriteJSON`

**Rationale:** these are pure `io.Writer` functions that consume `Metrics`;
they lock the text contract for behavior 1 and behavior 2 without touching
the CLI yet.

#### Files to Modify

| File                                     | Action | Description                                         |
|------------------------------------------|--------|-----------------------------------------------------|
| `internal/loadgen/report/summary.go`     | create | `SummaryLine(m Metrics) string`                     |
| `internal/loadgen/report/json.go`        | create | `WriteJSON`, JSON struct definitions                |
| `internal/loadgen/report/json_test.go`   | create | Round-trip + exact-field assertions                 |
| `internal/loadgen/report/summary_test.go`| create | SummaryLine format + small-sample warning suffix    |

#### New Code

```go
// summary.go
package report

import (
    "fmt"
    "strings"
)

// SummaryLine returns the stdout one-liner. Exactly matches the observable
// contract in M5-012:
//   "Results: requests=N, p50=Xms, p95=Yms, p99=Zms, error_rate=0%"
// With the small-sample note appended when Metrics.SmallSampleWarning is true.
func SummaryLine(m Metrics) string {
    var b strings.Builder
    fmt.Fprintf(&b, "Results: requests=%d, p50=%dms, p95=%dms, p99=%dms, error_rate=%d%%",
        m.Requests,
        m.P50.Milliseconds(),
        m.P95.Milliseconds(),
        m.P99.Milliseconds(),
        int(m.ErrorRate*100+0.5),
    )
    if m.SmallSampleWarning {
        b.WriteString(" (p99 may be imprecise for small samples)")
    }
    return b.String()
}
```

```go
// json.go
package report

import (
    "encoding/json"
    "fmt"
    "io"
    "time"
)

type jsonReport struct {
    Schema             string         `json:"schema"`
    GeneratedAt        string         `json:"generated_at"`
    Title              string         `json:"title"`
    Metrics            jsonMetrics    `json:"metrics"`
    Buckets            []jsonBucket   `json:"buckets"`
    SmallSampleWarning bool           `json:"small_sample_warning"`
}

type jsonMetrics struct {
    Requests     int     `json:"requests"`
    Successes    int     `json:"successes"`
    Failures     int     `json:"failures"`
    ElapsedMs    int64   `json:"elapsed_ms"`
    P50Ms        int64   `json:"p50_ms"`
    P95Ms        int64   `json:"p95_ms"`
    P99Ms        int64   `json:"p99_ms"`
    MinMs        int64   `json:"min_ms"`
    MaxMs        int64   `json:"max_ms"`
    MeanMs       int64   `json:"mean_ms"`
    ErrorRate    float64 `json:"error_rate"`
    Throughput   float64 `json:"throughput_rps"`
}

type jsonBucket struct {
    Second   int   `json:"second"`
    Requests int   `json:"requests"`
    Errors   int   `json:"errors"`
    P50Ms    int64 `json:"p50_ms"`
    P95Ms    int64 `json:"p95_ms"`
}

// WriteJSON encodes the metrics as the apitest perf JSON report. Truncates w
// through the caller's os.WriteFile; this function is I/O-only.
func WriteJSON(w io.Writer, m Metrics, title string) error {
    if m.Requests == 0 {
        // Still emit a valid (empty) JSON doc so downstream tools do not choke.
    }
    r := jsonReport{
        Schema:             "apitest.perf.v1",
        GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
        Title:              title,
        SmallSampleWarning: m.SmallSampleWarning,
        Metrics: jsonMetrics{
            Requests:  m.Requests,
            Successes: m.Successes,
            Failures:  m.Failures,
            ElapsedMs: m.Elapsed.Milliseconds(),
            P50Ms:     m.P50.Milliseconds(),
            P95Ms:     m.P95.Milliseconds(),
            P99Ms:     m.P99.Milliseconds(),
            MinMs:     m.Min.Milliseconds(),
            MaxMs:     m.Max.Milliseconds(),
            MeanMs:    m.Mean.Milliseconds(),
            ErrorRate: m.ErrorRate,
            Throughput: m.Throughput,
        },
        Buckets: make([]jsonBucket, 0, len(m.Buckets)),
    }
    for _, b := range m.Buckets {
        r.Buckets = append(r.Buckets, jsonBucket{
            Second: b.SecondOffset, Requests: b.Requests, Errors: b.Errors,
            P50Ms: b.P50.Milliseconds(), P95Ms: b.P95.Milliseconds(),
        })
    }
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    if err := enc.Encode(r); err != nil {
        return fmt.Errorf("encode perf report: %w", err)
    }
    return nil
}
```

The time-source is overridable for golden-file tests via a package-level
`var nowFn = time.Now` hooked in `report_test.go` — avoids making tests depend
on wall clock.

#### Tests to Write FIRST

```go
func TestSummaryLine_MatchesObservable(t *testing.T) {
    got := SummaryLine(Metrics{Requests: 42, P50: 11*time.Millisecond, P95: 22*time.Millisecond, P99: 33*time.Millisecond, ErrorRate: 0})
    want := "Results: requests=42, p50=11ms, p95=22ms, p99=33ms, error_rate=0%"
    if got != want { t.Errorf(...) }
}

func TestSummaryLine_SmallSampleSuffix(t *testing.T) {
    m := Metrics{Requests: 10, SmallSampleWarning: true}
    got := SummaryLine(m)
    if !strings.Contains(got, "(p99 may be imprecise for small samples)") { t.Error(...) }
}

func TestSummaryLine_ErrorRateRounding(t *testing.T) {
    // 0.123 -> 12%, 0.129 -> 13%
}

func TestWriteJSON_SchemaAndFields(t *testing.T) {
    // Round-trip: encode, decode into map, assert schema="apitest.perf.v1",
    // metrics.requests, metrics.p95_ms, buckets[0].second, etc.
}

func TestWriteJSON_DeterministicBucketOrder(t *testing.T) {
    // Add samples into buckets 2, 0, 1 in that order; verify JSON
    // buckets field is ordered 0, 1, 2 by Second.
}

func TestWriteJSON_EmptyMetricsValidJSON(t *testing.T) {
    // Zero Metrics still produces valid JSON with requests=0.
}
```

#### Impact on Existing Tests

None.

---

### Step 4: `WriteHTML` + embedded Chart.js template

**Rationale:** biggest surface area; built once Metrics is stable so the
template can bind to real field names.

#### Files to Modify

| File                                      | Action | Description                                                          |
|-------------------------------------------|--------|----------------------------------------------------------------------|
| `internal/loadgen/report/html.go`         | create | `WriteHTML`, `htmlReport` data view, template                        |
| `internal/loadgen/report/html_test.go`    | create | Template renders; title, Chart.js tag, and summary fields present    |

Template uses `html/template` (not `text/template`) for auto-escaping of
title/run metadata. Chart.js loads via CDN `<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.1"></script>`. Series data marshaled to JSON inline
into a `<script>` block via the `printf "%s"` of `template.JS` from a
pre-encoded JSON blob.

#### Key assertions

- `strings.Contains(out, "<title>apitest perf report</title>")` — matches observable
- `strings.Contains(out, "cdn.jsdelivr.net/npm/chart.js")` — Chart.js wired
- `strings.Contains(out, "perf-latency-chart")` — canvas id present
- `strings.Contains(out, `"p50_ms"`)` — series data inlined (JSON literal)
- `strings.Contains(out, "<title>apitest perf report</title>")` — exact from observable
- Title defaults to `"apitest perf report"` when the caller passes an empty
  title string; behaviour is enforced via a short helper
  `func titleOrDefault(t string) string`.

#### Impact on Existing Tests

None.

---

### Step 5: Wire samples through `loadgen.Run`

**Rationale:** after the report package is complete we can rig the producer
without risk of landing a broken CLI; this is the only step that touches
`internal/loadgen`'s exported surface.

#### Files to Modify

| File                             | Action  | Description                                                                                       |
|----------------------------------|---------|---------------------------------------------------------------------------------------------------|
| `internal/loadgen/run.go`        | modify  | Add `OnSample func(Sample)` to `RunOptions`; emit samples from `runVU`                            |
| `internal/loadgen/run_test.go`   | modify  | Add one test: `TestRun_OnSampleReceivesOneCallPerRequest`                                         |

#### Current Code (`run.go:19-23`)

```go
// RunOptions injects test seams. Zero value uses production defaults.
type RunOptions struct {
    Execute ExecuteFunc // nil = httpexec.Execute
    Now     func() time.Time
}
```

#### New Code

```go
// RunOptions injects test seams. Zero value uses production defaults.
type RunOptions struct {
    Execute  ExecuteFunc // nil = httpexec.Execute
    Now      func() time.Time
    OnSample func(Sample) // nil = samples discarded
}
```

`runVU` gains a `start time.Time` captured from `Run` and the per-request
timestamps are computed from `now()`:

```go
func runVU(ctx context.Context, execute ExecuteFunc, req *httpexec.Request,
    tickC <-chan time.Time, start time.Time, now func() time.Time,
    onSample func(Sample),
    reqs, succ, fail *int64,
) {
    for {
        if ctx.Err() != nil { return }
        if tickC != nil {
            select { case <-ctx.Done(): return; case <-tickC: }
        }
        reqStart := now()
        atomic.AddInt64(reqs, 1)
        res, err := execute(ctx, req)
        reqEnd := now()
        ok := false
        switch {
        case err != nil:
            if ctx.Err() != nil {
                atomic.AddInt64(reqs, -1)
                return
            }
            atomic.AddInt64(fail, 1)
        case res.StatusCode >= 400:
            atomic.AddInt64(fail, 1)
        default:
            atomic.AddInt64(succ, 1)
            ok = true
        }
        if onSample != nil {
            onSample(Sample{
                StartOffsetNs: reqStart.Sub(start).Nanoseconds(),
                LatencyNs:     reqEnd.Sub(reqStart).Nanoseconds(),
                Success:       ok,
            })
        }
    }
}
```

`Run` passes `start` (already computed) and `opts.OnSample` through. Backward
compatibility: existing callers who pass `RunOptions{Execute: x}` still work —
`OnSample == nil` means samples are discarded, counters unchanged.

#### Tests to Write FIRST

```go
func TestRun_OnSampleReceivesOneCallPerRequest(t *testing.T) {
    var mu sync.Mutex
    var samples []Sample
    fake := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
        time.Sleep(time.Millisecond)
        return &httpexec.Result{StatusCode: 200}, nil
    }
    sum, _ := Run(ctx, Config{VUs: 2, Duration: 100*time.Millisecond},
        &httpexec.Request{URL: "x"},
        RunOptions{
            Execute: fake,
            OnSample: func(s Sample) {
                mu.Lock(); samples = append(samples, s); mu.Unlock()
            },
        })
    if len(samples) != sum.Requests {
        t.Errorf("samples=%d, requests=%d", len(samples), sum.Requests)
    }
    // Sanity: Success fields true, LatencyNs > 0, StartOffsetNs >= 0.
}
```

#### Impact on Existing Tests

- `TestRun_VUsExecuteUntilDeadline` and friends in `run_test.go` do **not**
  pass `OnSample`; they continue to pass unchanged because `OnSample == nil`
  is a fast-path no-op.
- `runVU` signature expands — it is unexported, so no out-of-package
  consumers. All callers are in `run.go`; the test file uses only `Run`, not
  `runVU`.

---

### Step 6: CLI wiring in `cmd/apitest/perf.go`

**Rationale:** final integration; uses every previously landed piece.

#### Files to Modify

| File                            | Action | Description                                                                                 |
|---------------------------------|--------|---------------------------------------------------------------------------------------------|
| `cmd/apitest/perf.go`           | modify | Swap stdout-only branch for format dispatch; aggregator plumbing; summary + file write      |
| `cmd/apitest/perf_test.go`      | modify | Add tests for `--output` json/html success paths, unsupported ext exit 2, overwrite behavior|
| `testdata/perf/sample-request.yaml` | keep | already created by M5-011                                                                  |

#### Current Code (`perf.go:67-74`)

```go
// Output destination validation (stdout only in this slice).
if flags.output != "" && flags.output != "stdout" {
    _, _ = fmt.Fprintf(os.Stderr,
        "error: --output %q not supported; only 'stdout' is supported (json/html land in M5-012)\n",
        flags.output)
    return 2
}
```

#### New Code

```go
// Replaces the stdout-only gate.
format, outPath, fmtErr := report.DetectFormat(flags.output)
if fmtErr != nil {
    _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", fmtErr)
    return 2
}

// ...later, after LoadRequestFile and before loadgen.Run...
agg := report.New()
runOpts := loadgen.RunOptions{
    OnSample: func(s loadgen.Sample) {
        agg.Observe(report.Sample{
            StartOffsetNs: s.StartOffsetNs,
            LatencyNs:     s.LatencyNs,
            Success:       s.Success,
        })
    },
}

sum, runErr := loadgen.Run(ctx, cfg, req, runOpts)
// ... existing error handling ...

metrics := agg.Metrics(sum.Elapsed)

// Always print the summary line.
fmt.Println(report.SummaryLine(metrics))

switch format {
case report.FormatJSON:
    if err := writeReportFile(outPath, func(w io.Writer) error {
        return report.WriteJSON(w, metrics, "apitest perf")
    }); err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
        return 1
    }
    fmt.Printf("Wrote %s\n", outPath)
case report.FormatHTML:
    if err := writeReportFile(outPath, func(w io.Writer) error {
        return report.WriteHTML(w, metrics, "apitest perf")
    }); err != nil {
        _, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
        return 1
    }
    fmt.Printf("Wrote %s\n", outPath)
}
```

`writeReportFile` helper (local to `perf.go`) wraps `os.OpenFile(path, O_WRONLY|O_CREATE|O_TRUNC, 0o644)` — this is how we guarantee behavior 8
(second run overwrites, no append).

Help text updated to:

```
  --output <dest>    Output destination — extension selects format:
                       .json    write JSON time-series + metrics
                       .html    write self-contained HTML (Chart.js CDN)
                       stdout   print summary only (default)
```

#### Tests to Write FIRST

```go
func TestPerfCmd_OutputJSON_WritesFile(t *testing.T) {
    // 1) start httptest.NewServer (200 OK)
    // 2) tmp dir, out := filepath.Join(dir, "perf.json")
    // 3) run perfCmd([]string{reqFile, "--vus","2","--duration","200ms","--output",out})
    // 4) assert code == 0; assert file exists; decode JSON; assert .metrics.requests > 0
}

func TestPerfCmd_OutputHTML_WritesFile(t *testing.T) {
    // assert output file contains "<title>apitest perf report</title>" and "chart.js"
}

func TestPerfCmd_UnsupportedExtensionExitCode2(t *testing.T) {
    // --output report.xyz -> exit 2 + stderr contains "unsupported report format .xyz"
    // This REPLACES the existing TestPerfCmd_UnsupportedOutputExitCode2 test which
    // currently asserts that --output html (without extension) exits 2.
}

func TestPerfCmd_OutputJSON_OverwritesExistingFile(t *testing.T) {
    // write dummy content to perf.json, run perfCmd twice, assert final
    // file is valid JSON and larger than 0 bytes (the dummy content is gone).
}

func TestPerfCmd_SummaryLinePrintedToStdout(t *testing.T) {
    // stdout contains "Results: requests="
}
```

#### Impact on Existing Tests

- `TestPerfCmd_UnsupportedOutputExitCode2` in `perf_test.go:193-200` currently
  passes `--output html` (literal string with no extension) and asserts exit
  2 because M5-011 rejected everything but "stdout". Under M5-012 the word
  `html` has no `.` so `DetectFormat` still returns `ErrUnsupportedFormat`
  (no extension → error). **The test passes unchanged** but the error message
  it observes changes. If the test matches on the message text, rewrite to
  match `unsupported report format`. On current inspection the test only
  asserts the exit code, so it remains green.
- `TestPrintPerfHelp_MentionsAllFlags` still passes — `--output` remains in
  help text.
- No other tests affected.

---

### Step 7: Smoke test + CHANGELOG

**Rationale:** final verification surface; runs the real binary end-to-end
against the inlined `python3 -m http.server` already used by M5-011.

#### Files to Modify

| File               | Action  | Description                                                     |
|--------------------|---------|-----------------------------------------------------------------|
| `smoke/run.sh`     | modify  | Extend the perf block to assert `--output json` and `--output html` behaviors |
| `CHANGELOG.md`     | modify  | `[Unreleased] › Added` entry for M5-012                         |

Smoke additions (placed right after the existing `Requests sent:` check):

```bash
PERF_JSON="/tmp/apitest_perf_report_$$.json"
APITEST_TIER=enterprise ./apitest perf "$PERF_COL" --vus 2 --duration 1s \
  --output "$PERF_JSON" > /tmp/apitest_perf_stdout_$$.log 2>&1
grep -q "Results: requests=" /tmp/apitest_perf_stdout_$$.log \
  && echo "PASS: perf --output json prints summary line" \
  || { echo "FAIL: missing Results line"; exit 1; }
[ -s "$PERF_JSON" ] && python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$PERF_JSON" \
  && echo "PASS: perf --output json produces valid JSON" \
  || { echo "FAIL: invalid or empty JSON"; cat "$PERF_JSON"; exit 1; }
rm -f "$PERF_JSON" /tmp/apitest_perf_stdout_$$.log

PERF_HTML="/tmp/apitest_perf_report_$$.html"
APITEST_TIER=enterprise ./apitest perf "$PERF_COL" --vus 2 --duration 1s \
  --output "$PERF_HTML" > /dev/null 2>&1
grep -q "<title>apitest perf report</title>" "$PERF_HTML" \
  && echo "PASS: perf --output html wrote expected title" \
  || { echo "FAIL: perf html missing title"; exit 1; }
rm -f "$PERF_HTML"

# Unsupported extension -> exit 2
SMOKE_RC=0
APITEST_TIER=enterprise ./apitest perf "$PERF_COL" --vus 1 --duration 200ms \
  --output "/tmp/x.xyz" > /dev/null 2>/tmp/apitest_perf_err_$$.txt || SMOKE_RC=$?
[ "$SMOKE_RC" -eq 2 ] && echo "PASS: unsupported extension exits 2" \
  || { echo "FAIL: unsupported extension exited $SMOKE_RC"; cat /tmp/apitest_perf_err_$$.txt; exit 1; }
grep -qi "unsupported report format" /tmp/apitest_perf_err_$$.txt \
  && echo "PASS: stderr mentions unsupported format" \
  || { echo "FAIL: stderr missing message"; exit 1; }
rm -f /tmp/apitest_perf_err_$$.txt
```

CHANGELOG:

```markdown
### Added
- `apitest perf --output <path>` now supports `.json` and `.html` report
  formats in addition to the default summary-only stdout output: per-request
  latency samples are aggregated by a new `internal/loadgen/report` subpackage
  into p50/p95/p99 percentiles, error rate, throughput, and 1-second-resolution
  time-series buckets; the JSON report emits a machine-readable document with
  schema `apitest.perf.v1`; the HTML report is self-contained with a CDN
  Chart.js latency-vs-time line chart and a metrics summary. Unsupported
  extensions (e.g. `.xyz`) fail fast with exit code 2 and the message
  `error: unsupported report format .xyz`. Small sample sets (<100 requests)
  fall back to max for p99 and append a warning note to the summary line
  (M5-012)
```

#### Impact on Existing Tests

- Existing smoke `PASS: perf prints header` / `PASS: perf prints summary`
  lines are untouched.

---

## Test Impact Summary

| Test File                                | Test Function                             | Impact  | Action Required                                      |
|------------------------------------------|-------------------------------------------|---------|------------------------------------------------------|
| `internal/loadgen/run_test.go`           | all existing                              | none    | Still pass — `OnSample` defaults to nil              |
| `internal/loadgen/run_test.go`           | new: `TestRun_OnSampleReceivesOneCallPerRequest` | create | Verifies sample emission                     |
| `cmd/apitest/perf_test.go`               | `TestPerfCmd_UnsupportedOutputExitCode2`  | keeps behavior | Exit code 2 still correct for `--output html` (no ext). Update asserted message if added. |
| `cmd/apitest/perf_test.go`               | `TestPrintPerfHelp_MentionsAllFlags`      | none    | Help still lists `--output`                           |
| `cmd/apitest/perf_test.go`               | new: `TestPerfCmd_OutputJSON_WritesFile`  | create  | End-to-end JSON output via httptest server            |
| `cmd/apitest/perf_test.go`               | new: `TestPerfCmd_OutputHTML_WritesFile`  | create  | End-to-end HTML output via httptest server            |
| `cmd/apitest/perf_test.go`               | new: `TestPerfCmd_UnsupportedExtensionExitCode2` | create  | `.xyz` rejected with exit 2                        |
| `cmd/apitest/perf_test.go`               | new: `TestPerfCmd_OutputJSON_Overwrites`  | create  | Second run truncates first                            |
| `cmd/apitest/perf_test.go`               | new: `TestPerfCmd_SummaryLinePrintedToStdout` | create | `Results: requests=` present on stdout         |
| `internal/loadgen/report/*_test.go`      | all new                                   | create  | >=6 tests total (meets observable)                    |

## Risks and Edge Cases

- **Risk: Large-run memory blow-up.** At 10k RPS for 60s a simple sort needs
  ~9.6 MB of latencies. Document the bound and leave a TODO comment pointing
  to `tdigest` as the future swap point. Hard limit beyond which we refuse to
  allocate is out of scope for M5.
  → **Mitigation:** doc comment on `Aggregator` + plan to re-visit in a later
  milestone if customers hit it.

- **Risk: `OnSample` serializes VU execution.** The callback holds the
  aggregator mutex per sample. At 50k RPS this is ~20 μs critical section per
  call — not a bottleneck for Enterprise test scales (task spec does not
  require >10k RPS).
  → **Mitigation:** accept the contention; if profiling shows a hot spot,
  switch to per-VU local buffers that flush periodically.

- **Risk: Clock skew between `Run` start timestamp and VU observation.** VUs
  capture `now()` at the moment just before/after `execute`; the Aggregator's
  bucket index is derived from `reqStart - runStart`. Under ramp-up, early
  VUs start at offset 0, later VUs at offsets up to `RampUp`. First bucket
  therefore has lower throughput — that's the correct and desired display.
  → **Mitigation:** N/A (correct behavior).

- **Edge case: Zero samples (ctx cancelled before first request completes).**
  `Metrics()` returns zero values; `SummaryLine` prints `requests=0, p50=0ms...`.
  JSON emits the schema doc with empty buckets. HTML renders a "No samples"
  banner via `{{if eq .Metrics.Requests 0}}`. Covered by
  `TestAggregator_EmptyReturnsZeroMetrics`.

- **Edge case: Path is a directory, not a file.** `os.OpenFile(dir, ...)`
  returns EISDIR on Unix. `writeReportFile` wraps the error with
  `fmt.Errorf("writing %s: %w", path, err)` and `perfCmd` returns exit 1
  (generic failure, not usage error — the format was valid, the path just
  wasn't writable). Not explicitly in the observable, so not a behavior test,
  but the error path is exercised indirectly by `TestPerfCmd_OutputJSON_WritesFile`
  variants if we need to add a negative case later.

- **Edge case: Zero-length output filename (`--output ""`).** Flag parser
  already requires a value after `--output`; the case is `--output ""` which
  `DetectFormat` treats as empty → stdout. We preserve this — empty strings
  are equivalent to the flag not being passed.

- **Edge case: Case-sensitivity on extensions.** `report.JSON` → treat as
  JSON. `filepath.Ext` returns `.JSON` and we `strings.ToLower` before
  comparison. Covered by the `DetectFormat` test matrix.

- **Risk: Chart.js CDN unreachable at view time.** The HTML report degrades
  gracefully — the `<canvas>` is empty but the metric table is still readable.
  Not a test case (requires network mocking); documented in CHANGELOG.

- **Risk: Template injection via request URL in HTML.** `html/template`
  auto-escapes the title and any user-supplied strings. The Chart.js series
  data is a JSON blob inserted as `template.JS` — values are integers/floats
  only, no strings from user input.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Coverage check (must be ≥ 80 % for the new package):

```bash
go test -coverprofile=coverage.out ./internal/loadgen/report/...
go tool cover -func=coverage.out | tail -1
```

Observable verification:

```bash
go build ./cmd/apitest
go test ./internal/loadgen/report/...
# Expected: ok  internal/loadgen/report  (>=6 tests passing)

./apitest perf testdata/perf/sample-request.yaml \
  --vus 5 --duration 3s --output perf-report.json
# Expected stdout (tail):
#   Results: requests=N, p50=Xms, p95=Yms, p99=Zms, error_rate=0%
#   Wrote perf-report.json

./apitest perf testdata/perf/sample-request.yaml \
  --vus 5 --duration 3s --output perf-report.html
# Expected: perf-report.html created; file contains <title>apitest perf report</title>
```

Note: the observable commands require a server at `http://127.0.0.1:8080/`.
The smoke test script handles this automatically via `python3 -m http.server`;
running the observable manually requires the operator to provide a target
server. Behavior 4 ("--output stdout (default)") is covered by the existing
M5-011 smoke block, which runs `apitest perf` without `--output`.
