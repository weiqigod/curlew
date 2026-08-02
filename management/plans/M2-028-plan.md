# Implementation Plan: M2-028

## Overview
Extend the HTML report generator with data-driven visualizations (per-group iteration summary card, filterable iteration table with data values, status filters) and parallel execution visualizations (wave diagram, speedup factor, max parallelism) while keeping the report fully self-contained (no external CSS/JS). An inline SVG timeline chart renders iteration duration bars.

## Task Details
- **ID:** M2-028
- **Title:** HTML report data-driven and parallel visualization
- **Phase:** M2: Reporting
- **Priority:** 5
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-016 | Parallel execution | done |
| M2-019 | Data-driven testing | done |
| M2-027 | HTML report generation | done |

## Key Architectural Decisions

1. **No external Chart.js.** The scope suggests "Chart.js (embedded)" but the existing test `TestWriteHTML` asserts the report contains no `<link rel="stylesheet">` and no external `<script src=`, and Chart.js minified (~200 KB) would bloat every report. Spec's guiding principle (`docs/SPECIFICATION.md` line 4759) is that reports must be self-contained and openable without a web server. Decision: render iteration duration timelines as **inline SVG bars** generated server-side by the template. SVG is self-contained, requires no JS runtime, and keeps reports small. This satisfies the observable behavior "timeline chart showing iteration duration" without introducing an external dependency.

2. **Propagate iteration row data through the runner.** The current `runner.RequestResult` struct carries iteration metadata (`IsDataDriven`, `IterationIndex`, `IterationTotal`, `DataDrivenName`) but not the row data itself. Behavior "filterable table shows each iteration with status, duration, and data values" requires the row. Decision: add `IterationData map[string]string` to `RequestResult`, populated by both `executeDataDriven` (sequential) and `executeDataDrivenParallel` (parallel). The map is nil for non-data-driven results. This is a low-risk additive field change.

3. **Aggregate inside the output package, not the runner.** `buildHTMLReport` in `cmd/apitest/main.go` passes flat results to the output package. The output package is the right layer to **aggregate** iteration results into `HTMLDataDrivenGroup` (by `DataDrivenName`) and wave results into `HTMLWave` (by `WaveIndex`). This keeps `main.go` thin and the logic fully testable in `internal/output`.

4. **Client-side filtering via inline JS.** Filtering by status (All / Passed / Failed) is implemented with a small inline `<script>` that toggles `display: none` on rows by CSS class. No external JS, consistent with existing `toggle(id)` inline function.

5. **Speedup factor formula.** `speedup = (sum of successful request durations) / (sum of wave durations)`. If `sum(wave durations) == 0`, speedup is reported as `N/A` (avoids divide-by-zero for skipped/empty runs). Max parallelism is taken directly from `summary.MaxParallelism`.

6. **Backward compatibility.** All new fields on `HTMLReport` are additive and optional. Reports generated for non-data-driven, non-parallel runs render identically to the current M2-027 output — the new sections are conditionally rendered with `{{if .DataDrivenGroups}}` and `{{if .Waves}}` guards. Every existing `TestWriteHTML` case must continue to pass unchanged.

## Implementation Steps

### Step 1: Propagate iteration row data through the runner
**Rationale:** Smallest blast radius — an additive field on `RequestResult`. Nothing downstream reads it yet, so no existing callers break. Must land first so later output-layer tests have data to consume.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `IterationData map[string]string` field to `RequestResult`; populate it in `executeDataDriven` and `executeDataDrivenParallel` |
| `internal/runner/runner_test.go` | modify | Add test asserting `IterationData` is populated on data-driven iterations and nil on plain requests |

#### Current Code
```go
// internal/runner/runner.go:132-137
// Data-driven iteration metadata (populated only for data-driven results)
IsDataDriven   bool   // true when this result is from a data-driven iteration
DataDrivenName string // base request name (without [X/Y] suffix)
IterationIndex int    // 0-based iteration index
IterationTotal int    // total number of iterations in this data-driven group
}
```

#### New Code
```go
// Data-driven iteration metadata (populated only for data-driven results)
IsDataDriven   bool              // true when this result is from a data-driven iteration
DataDrivenName string            // base request name (without [X/Y] suffix)
IterationIndex int               // 0-based iteration index
IterationTotal int               // total number of iterations in this data-driven group
IterationData  map[string]string // row data values for this iteration (nil when not data-driven)
}
```

In `executeDataDriven` (sequential path, ~line 1168 and ~line 1191) populate `IterationData: cloneRow(row)` on each constructed `RequestResult`. In `executeDataDrivenParallel` (result conversion, ~line 1447) populate `IterationData: cloneRow(ir.Row)`.

Add helper:
```go
func cloneRow(r datadriven.Row) map[string]string {
    if len(r) == 0 {
        return nil
    }
    out := make(map[string]string, len(r))
    for k, v := range r {
        out[k] = v
    }
    return out
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteDataDriven_PopulatesIterationData(t *testing.T) {
    // Arrange: minimal data-driven collection with 2 rows
    // Act: run via Run(...)
    // Assert: each data-driven result has IterationData matching the row,
    //         non-data-driven results have IterationData == nil.
}
```

#### Impact on Existing Tests
- No existing test reads `IterationData`; the field is additive. All current runner tests remain green.

---

### Step 2: Extend `HTMLReport` data model in `internal/output`
**Rationale:** Pure data-type additions with no rendering or logic. Lets later steps' tests be written against a stable struct contract. No existing callers break because all new fields are optional.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/html.go` | modify | Add `HTMLDataDrivenGroup`, `HTMLIteration`, `HTMLWave`, `HTMLWaveItem`, and parallel-summary fields to `HTMLReport` |

#### New Types

```go
// HTMLDataDrivenGroup represents one data-driven request and its iterations.
type HTMLDataDrivenGroup struct {
    Name             string          // base request name (e.g. "Create User")
    Total            int             // iterations executed
    Passed           int
    Failed           int
    Skipped          int
    Errored          int
    PassRatePercent  int             // 0..100 (integer for stable display)
    AvgDurationMs    int64
    TotalDurationMs  int64
    ThroughputPerSec float64         // iterations per second, 2 decimals
    DataColumns      []string        // sorted column names across iterations
    Iterations       []HTMLIteration // in iteration index order
    TimelineBars     []HTMLTimelineBar // precomputed SVG bar data (aligned to Iterations)
}

// HTMLIteration is one data-driven iteration row in the filterable table.
type HTMLIteration struct {
    Index      int               // 0-based
    Label      string            // "1/100"
    Status     string            // "passed", "failed", "skipped", "error"
    StatusCode int
    DurationMs int64
    Data       map[string]string // row data values
    Error      string
}

// HTMLTimelineBar is one bar in the inline SVG iteration timeline.
type HTMLTimelineBar struct {
    XPercent float64 // 0..100
    WidthPercent float64
    HeightPercent float64 // 0..100 relative to max duration
    Color    string // "#22863a" passed, "#cb2431" failed, "#959da5" skipped
    Title    string // SVG <title> tooltip
}

// HTMLWave represents one parallel execution wave.
type HTMLWave struct {
    Index      int           // 0-based wave number
    DurationMs int64
    Items      []HTMLWaveItem
}

// HTMLWaveItem is one request within a wave.
type HTMLWaveItem struct {
    Name       string
    Status     string
    DurationMs int64
    StatusCode int
}
```

Add to `HTMLReport`:

```go
type HTMLReport struct {
    // ... existing fields ...

    // Data-driven visualization (nil/empty when no data-driven requests ran).
    DataDrivenGroups []HTMLDataDrivenGroup

    // Parallel execution visualization (nil/empty when --parallel was not used).
    Waves            []HTMLWave
    IsParallel       bool
    WaveCount        int
    MaxParallelism   int
    SpeedupFactor    string // formatted "2.34x" or "N/A"
    TotalWaveDurationMs int64
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestHTMLReport_DataDrivenGroupFields(t *testing.T) {
    // Struct-level test: the new types compile and fields are addressable.
    // (Thin compilation test that exists mainly to document expected shape.)
}
```

#### Impact on Existing Tests
- None. Purely additive type surface.

---

### Step 3: Aggregate iteration and wave data at report build time
**Rationale:** Introduces new functions in `internal/output` that transform flat iteration/wave results into the structured `HTMLDataDrivenGroup`/`HTMLWave` slices. Pure functions, fully unit-testable without any template rendering. Done before templating so the template can be written against known inputs.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/html.go` | modify | Add `BuildDataDrivenGroups`, `BuildWaves`, `computeSpeedup` helpers |
| `internal/output/html_test.go` | modify | Add table-driven tests for the aggregation helpers |
| `cmd/apitest/main.go` | modify | Call the new helpers in `buildHTMLReport` to populate the new fields |

#### Proposed Signatures

```go
// IterationInput is the minimal shape the output package needs from the runner
// to build data-driven groups, decoupling it from runner.RequestResult.
type IterationInput struct {
    GroupName   string
    Index       int
    Total       int
    Status      string // "passed"|"failed"|"skipped"|"error"
    StatusCode  int
    DurationMs  int64
    Data        map[string]string
    Error       string
}

// BuildDataDrivenGroups aggregates flat iteration inputs into groups, one per
// data-driven request name, preserving input order.
func BuildDataDrivenGroups(iters []IterationInput) []HTMLDataDrivenGroup

// WaveInput is one request-within-a-wave input for BuildWaves.
type WaveInput struct {
    WaveIndex  int
    Name       string
    Status     string
    DurationMs int64
    StatusCode int
}

// BuildWaves groups wave inputs by WaveIndex, with durations from waveDurations
// (indexed by wave). Returns nil when len(waveInputs) == 0.
func BuildWaves(waveInputs []WaveInput, waveDurations []int64) []HTMLWave

// computeSpeedup returns "2.34x" or "N/A" when wave durations sum to zero.
func computeSpeedup(totalRequestDurationMs, totalWaveDurationMs int64) string
```

`buildHTMLReport` (in `cmd/apitest/main.go`) collects iteration and wave inputs in a single pass over `results` and calls these helpers. It also sets `IsParallel`, `WaveCount`, `MaxParallelism`, and `SpeedupFactor` from `summary`.

#### Tests to Write FIRST (RED phase)

```go
func TestBuildDataDrivenGroups(t *testing.T) {
    tests := []struct {
        name   string
        input  []IterationInput
        want   []HTMLDataDrivenGroup
    }{
        {"empty input returns nil", nil, nil},
        {"single group with mixed statuses", /* ... */},
        {"two groups preserve input order", /* ... */},
        {"pass rate rounds to nearest integer", /* ... */},
        {"throughput handles zero total duration", /* ... */},
        {"data columns sorted alphabetically", /* ... */},
        {"timeline bars scaled to max duration", /* ... */},
    }
    // ...
}

func TestBuildWaves(t *testing.T) {
    tests := []struct {
        name          string
        inputs        []WaveInput
        waveDurations []int64
        want          []HTMLWave
    }{
        {"empty returns nil", nil, nil, nil},
        {"single wave with one request", /* ... */},
        {"three waves grouped by index", /* ... */},
        {"missing wave duration defaults to zero", /* ... */},
    }
    // ...
}

func TestComputeSpeedup(t *testing.T) {
    tests := []struct {
        name                       string
        totalRequestMs, totalWaveMs int64
        want                       string
    }{
        {"2x speedup", 1000, 500, "2.00x"},
        {"fractional speedup", 1234, 500, "2.47x"},
        {"wave time zero returns N/A", 1000, 0, "N/A"},
        {"no requests returns N/A", 0, 0, "N/A"},
    }
    // ...
}
```

#### Impact on Existing Tests
- `TestWriteHTML` cases: unchanged inputs produce reports where new sections are absent (guarded by `{{if}}`), so expected substrings still match and no new absent-asserted strings accidentally appear. Verify by rerunning the full test suite after Step 4.
- No existing test in `cmd/apitest/main_test.go` asserts on `HTMLReport` internals.

---

### Step 4: Template rendering — data-driven section
**Rationale:** Adds UI. Runs after aggregation is covered by unit tests so template logic can focus on markup. No runtime logic beyond HTML/SVG output, still fully self-contained.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/html.go` | modify | Extend `htmlTemplate` with a `{{range .DataDrivenGroups}}` block rendering: summary card (total iterations, pass rate, avg duration, throughput), inline SVG timeline, filter buttons, filterable iteration table with data columns |
| `internal/output/html_test.go` | modify | Add test cases asserting data-driven sections appear and behaviors |

#### Template Additions (outline)

```html
{{range $gi, $g := .DataDrivenGroups}}
<div class="dd-group" data-group="{{$gi}}">
  <h2 class="dd-name">{{$g.Name}}</h2>
  <div class="dd-summary">
    <div class="summary-card"><div class="label">Iterations</div><div class="value">{{$g.Total}}</div></div>
    <div class="summary-card"><div class="label">Pass rate</div><div class="value">{{$g.PassRatePercent}}%</div></div>
    <div class="summary-card"><div class="label">Avg duration</div><div class="value">{{$g.AvgDurationMs}}ms</div></div>
    <div class="summary-card"><div class="label">Throughput</div><div class="value">{{printf "%.2f" $g.ThroughputPerSec}}/s</div></div>
  </div>

  {{if $g.TimelineBars}}
  <svg class="dd-timeline" viewBox="0 0 100 40" preserveAspectRatio="none" aria-label="Iteration timeline">
    {{range $b := $g.TimelineBars}}
    <rect x="{{printf "%.3f" $b.XPercent}}" y="{{printf "%.3f" (sub 40.0 (mulf $b.HeightPercent 0.4))}}"
          width="{{printf "%.3f" $b.WidthPercent}}" height="{{printf "%.3f" (mulf $b.HeightPercent 0.4)}}"
          fill="{{$b.Color}}"><title>{{$b.Title}}</title></rect>
    {{end}}
  </svg>
  {{end}}

  <div class="dd-filters" data-group="{{$gi}}">
    <button class="dd-filter active" data-filter="all" onclick="ddFilter({{$gi}},'all',this)">All</button>
    <button class="dd-filter" data-filter="passed" onclick="ddFilter({{$gi}},'passed',this)">Passed</button>
    <button class="dd-filter" data-filter="failed" onclick="ddFilter({{$gi}},'failed',this)">Failed</button>
  </div>

  <table class="dd-table">
    <thead>
      <tr>
        <th>#</th><th>Status</th><th>Duration</th>
        {{range $col := $g.DataColumns}}<th>{{$col}}</th>{{end}}
      </tr>
    </thead>
    <tbody>
      {{range $it := $g.Iterations}}
      <tr class="dd-row dd-status-{{$it.Status}}" data-status="{{$it.Status}}">
        <td>{{$it.Label}}</td>
        <td><span class="badge {{$it.Status}}">{{$it.Status}}</span></td>
        <td>{{$it.DurationMs}}ms</td>
        {{range $col := $g.DataColumns}}<td>{{index $it.Data $col}}</td>{{end}}
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}
```

Inline filter JS appended to the existing `<script>`:

```javascript
function ddFilter(gi, status, btn) {
  var group = document.querySelector('.dd-group[data-group="' + gi + '"]');
  if (!group) return;
  var rows = group.querySelectorAll('.dd-row');
  for (var i = 0; i < rows.length; i++) {
    if (status === 'all' || rows[i].getAttribute('data-status') === status) {
      rows[i].style.display = '';
    } else {
      rows[i].style.display = 'none';
    }
  }
  var btns = group.querySelectorAll('.dd-filter');
  for (var j = 0; j < btns.length; j++) btns[j].classList.remove('active');
  btn.classList.add('active');
}
```

Note on `mulf`/`sub`: these are not in `text/template` by default. Decision: precompute `YPercent` and `BarHeightPercent` inside `BuildDataDrivenGroups` so the template only emits the values — avoids registering a custom FuncMap.

Refined `HTMLTimelineBar`:
```go
type HTMLTimelineBar struct {
    XPercent     float64
    YPercent     float64
    WidthPercent float64
    HeightPercent float64
    Color        string
    Title        string
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteHTML_DataDrivenSection(t *testing.T) {
    tests := []struct {
        name       string
        input      *HTMLReport
        wantSubstr []string
        wantAbsent []string
    }{
        {
            "data-driven summary card shows iterations, pass rate, avg, throughput",
            &HTMLReport{/* with DataDrivenGroups having 3 iters, 2 pass 1 fail */},
            []string{"Iterations", "3", "Pass rate", "67%", "Avg duration", "ms", "Throughput", "/s"},
            nil,
        },
        {
            "data-driven table shows iteration rows with data columns",
            &HTMLReport{/* group with columns ["user","role"] */},
            []string{"<th>user</th>", "<th>role</th>", "alice", "admin"},
            nil,
        },
        {
            "filter buttons rendered with onclick handler",
            &HTMLReport{/* any group */},
            []string{`data-filter="all"`, `data-filter="passed"`, `data-filter="failed"`, "ddFilter"},
            nil,
        },
        {
            "failed iteration row carries data-status=failed",
            &HTMLReport{/* one passed, one failed */},
            []string{`data-status="passed"`, `data-status="failed"`},
            nil,
        },
        {
            "inline SVG timeline rendered when bars present",
            &HTMLReport{/* group with 2 bars */},
            []string{"<svg", "<rect", "</svg>"},
            nil,
        },
        {
            "no data-driven section when groups empty",
            &HTMLReport{/* no DataDrivenGroups */},
            nil,
            []string{"dd-group", "dd-filter", "dd-timeline"},
        },
        {
            "report remains self-contained with data-driven section",
            &HTMLReport{/* with groups */},
            nil,
            []string{`<link rel="stylesheet"`, `<script src="http`, `<script src="/`},
        },
    }
    // ... run & assert
}
```

#### Impact on Existing Tests
- `TestWriteHTML` "no external CSS or JS dependencies": still passes because we only add inline `<style>` and inline `<script>` content.
- `TestWriteHTML` "special HTML characters escaped": template still uses `html/template` which auto-escapes data values including the new data columns.
- All other cases: unaffected because they do not populate `DataDrivenGroups`.

---

### Step 5: Template rendering — parallel wave section
**Rationale:** Independent of Step 4, but done after so the developer rhythm is one feature per step. Still no runtime logic beyond markup.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/html.go` | modify | Extend `htmlTemplate` with a `{{if .IsParallel}}` block rendering: parallelism summary card (waves, max parallelism, speedup), wave-by-wave visualization |
| `internal/output/html_test.go` | modify | Add tests asserting wave section behaviors |

#### Template Additions (outline)

```html
{{if .IsParallel}}
<div class="parallel-summary">
  <h2>Parallel execution</h2>
  <div class="summary">
    <div class="summary-card"><div class="label">Waves</div><div class="value">{{.WaveCount}}</div></div>
    <div class="summary-card"><div class="label">Max parallelism</div><div class="value">{{.MaxParallelism}}</div></div>
    <div class="summary-card"><div class="label">Speedup</div><div class="value">{{.SpeedupFactor}}</div></div>
  </div>

  <div class="waves">
    {{range $wi, $w := .Waves}}
    <div class="wave">
      <div class="wave-header">Wave {{$w.Index}} <span class="wave-duration">{{$w.DurationMs}}ms</span></div>
      <div class="wave-items">
        {{range $item := $w.Items}}
        <div class="wave-item {{$item.Status}}" title="{{$item.Name}} ({{$item.DurationMs}}ms)">
          <span class="wave-item-name">{{$item.Name}}</span>
          <span class="wave-item-duration">{{$item.DurationMs}}ms</span>
        </div>
        {{end}}
      </div>
    </div>
    {{end}}
  </div>
</div>
{{end}}
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteHTML_ParallelSection(t *testing.T) {
    tests := []struct {
        name       string
        input      *HTMLReport
        wantSubstr []string
        wantAbsent []string
    }{
        {
            "parallel summary shows waves, max parallelism, speedup",
            &HTMLReport{IsParallel: true, WaveCount: 3, MaxParallelism: 4, SpeedupFactor: "2.50x",
                Waves: []HTMLWave{/* ... */}},
            []string{"Waves", "3", "Max parallelism", "4", "Speedup", "2.50x"},
            nil,
        },
        {
            "wave diagram shows each wave with its items",
            &HTMLReport{IsParallel: true, Waves: []HTMLWave{
                {Index: 0, DurationMs: 100, Items: []HTMLWaveItem{{Name: "A", Status: "passed", DurationMs: 100}}},
                {Index: 1, DurationMs: 50, Items: []HTMLWaveItem{{Name: "B", Status: "passed", DurationMs: 50}}},
            }},
            []string{"Wave 0", "Wave 1", "A", "B", "100ms", "50ms"},
            nil,
        },
        {
            "no parallel section when IsParallel false",
            &HTMLReport{IsParallel: false},
            nil,
            []string{"parallel-summary", "Max parallelism", "wave-item"},
        },
        {
            "speedup N/A rendered when formatted as N/A",
            &HTMLReport{IsParallel: true, SpeedupFactor: "N/A"},
            []string{"N/A"},
            nil,
        },
    }
    // ...
}
```

#### Impact on Existing Tests
- None. All existing test inputs have `IsParallel == false` (zero value) so the new section is guarded off.

---

### Step 6: Wire runner results into new output helpers inside `main.go`
**Rationale:** End-to-end wiring. Touches `cmd/apitest/main.go` only; the helpers and template are already covered by unit tests. Done last so any regression surfaces against a stable output layer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/main.go` | modify | In `buildHTMLReport`: map data-driven results into `IterationInput`s, map parallel results into `WaveInput`s, call the new helpers, set `IsParallel`/`WaveCount`/`MaxParallelism`/`SpeedupFactor` |
| `cmd/apitest/main_test.go` | modify (if it tests `buildHTMLReport`) | Add assertions on populated groups/waves; otherwise rely on smoke test |

#### New Logic (pseudo)

```go
func buildHTMLReport(name string, results []runner.RequestResult, summary *runner.Summary) *output.HTMLReport {
    // ... existing code ...

    var iterInputs []output.IterationInput
    var waveInputs []output.WaveInput
    var totalRequestMs int64

    for _, r := range results {
        // existing per-request HTMLRequest building ...

        if r.IsDataDriven {
            iterInputs = append(iterInputs, output.IterationInput{
                GroupName:  r.DataDrivenName,
                Index:      r.IterationIndex,
                Total:      r.IterationTotal,
                Status:     htmlStatusFor(r),
                StatusCode: safeStatusCode(r),
                DurationMs: safeDurationMs(r),
                Data:       r.IterationData,
                Error:      safeErr(r),
            })
        }
        if summary != nil && summary.IsParallel && r.WaveIndex >= 0 {
            waveInputs = append(waveInputs, output.WaveInput{
                WaveIndex:  r.WaveIndex,
                Name:       r.Name,
                Status:     htmlStatusFor(r),
                DurationMs: safeDurationMs(r),
                StatusCode: safeStatusCode(r),
            })
        }
        if r.Result != nil {
            totalRequestMs += r.Result.Duration.Milliseconds()
        }
    }

    htmlReport.DataDrivenGroups = output.BuildDataDrivenGroups(iterInputs)

    if summary != nil && summary.IsParallel {
        htmlReport.IsParallel = true
        htmlReport.WaveCount = summary.WaveCount
        htmlReport.MaxParallelism = summary.MaxParallelism
        waveDurs := make([]int64, len(summary.WaveDurations))
        var totalWaveMs int64
        for i, d := range summary.WaveDurations {
            waveDurs[i] = d.Milliseconds()
            totalWaveMs += waveDurs[i]
        }
        htmlReport.Waves = output.BuildWaves(waveInputs, waveDurs)
        htmlReport.TotalWaveDurationMs = totalWaveMs
        htmlReport.SpeedupFactor = output.ComputeSpeedup(totalRequestMs, totalWaveMs)
    }
    return htmlReport
}
```

`htmlStatusFor`, `safeStatusCode`, `safeDurationMs`, `safeErr` are small helpers extracted from the existing per-request switch so the same status logic is reused for both `HTMLRequest` and `IterationInput`/`WaveInput`.

#### Tests to Write FIRST (RED phase)

If `cmd/apitest/main_test.go` already has a test for `buildHTMLReport`, add cases:
- Data-driven result produces `DataDrivenGroups` populated with row data.
- Parallel result sets `IsParallel`, `WaveCount`, `MaxParallelism`, `SpeedupFactor`, `Waves`.
- Non-data-driven non-parallel result produces empty new sections.

If no test for `buildHTMLReport` exists, add one in this step.

#### Impact on Existing Tests
- Any existing smoke or integration test that runs a data-driven collection with `--report-html` will produce a richer report; only substring-based assertions (if any) could fail. Check `smoke/run.sh` for `--report-html` usages and adjust only if the smoke test scrapes for known-absent markup.

---

### Step 7: CHANGELOG and smoke-test update
**Rationale:** Completeness contract — document the change and add a smoke test step that exercises the new visualizations end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add entry under Unreleased: data-driven and parallel HTML visualization |
| `smoke/run.sh` | modify (if a data-driven or parallel HTML smoke case exists) | Add grep assertions for new markers (`dd-group`, `parallel-summary`) when a data-driven report is generated |

#### Impact on Existing Tests
- None directly. The smoke test additions are additive grep assertions.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| `internal/output/html_test.go` | `TestWriteHTML` | none | New test inputs do not set new fields; existing assertions continue to hold |
| `internal/output/html_test.go` | `TestWriteHTML_validHTML` | none | Still valid HTML with new sections |
| `internal/output/html_test.go` | `TestWriteHTML_nilReport` | none | Nil check unchanged |
| `internal/output/html_test.go` | `TestWriteHTML_writeError` | none | Write error path unchanged |
| `internal/output/html_test.go` | `TestWriteHTML_generatedAt` | none | Unchanged |
| `internal/output/html_test.go` | `TestBuildDataDrivenGroups` | new | Aggregation logic |
| `internal/output/html_test.go` | `TestBuildWaves` | new | Aggregation logic |
| `internal/output/html_test.go` | `TestComputeSpeedup` | new | Formula |
| `internal/output/html_test.go` | `TestWriteHTML_DataDrivenSection` | new | Template rendering for data-driven |
| `internal/output/html_test.go` | `TestWriteHTML_ParallelSection` | new | Template rendering for parallel |
| `internal/runner/runner_test.go` | data-driven tests | none | Additive `IterationData` field |
| `internal/runner/runner_test.go` | `TestExecuteDataDriven_PopulatesIterationData` | new | Verifies row propagation |
| `cmd/apitest/main_test.go` | `buildHTMLReport` tests (if present) | extended | New assertions on aggregated fields |

## Risks and Edge Cases

- **Risk:** Vendoring Chart.js would break the `no external CSS or JS dependencies` existing test and bloat reports by 200 KB+. **Mitigation:** Use inline SVG bars; document the deviation from scope wording with reference to the self-contained requirement in `docs/SPECIFICATION.md` line 4759.
- **Risk:** `html/template` auto-escapes data, which could break SVG path data if values were interpolated into attributes. **Mitigation:** Pre-format all SVG numeric attributes in Go (`printf "%.3f"`) and use numeric fields only; never inject user-controlled strings into path/attribute values.
- **Edge case:** Zero-duration iterations (`DurationMs == 0`). **Handling:** Timeline bar renders with a minimum of `0.5%` height so the bar remains visible; `computeSpeedup` returns `N/A` when total wave duration is zero.
- **Edge case:** A single iteration per group (one bar). **Handling:** Bar spans full width (`WidthPercent = 100.0 / n` where `n = 1`, so `100.0`).
- **Edge case:** Data-driven iteration inside a parallel wave (already supported by runner). **Handling:** Result appears in both `DataDrivenGroups` (aggregated by name) and `Waves` (grouped by wave index). Both views reference the same underlying iteration; no double counting because each view counts its own axis.
- **Edge case:** Data columns differ between iterations (sparse data). **Handling:** `BuildDataDrivenGroups` computes the union of keys across iterations and sorts alphabetically; missing values render as empty cells.
- **Edge case:** `store_results: failed` policy (M2-022) filters out successful data-driven results before they reach `buildHTMLReport`. **Handling:** Report accurately reflects what the runner returns; totals come from `summary`, iteration rows reflect only stored results. A comment in the template (`{{/* */}}`) notes this behavior.
- **Edge case:** Skipped parallel requests (cancelled due to upstream failure) have `DurationMs == 0`. **Handling:** Wave items with `Status == "skipped"` render with a grey bar and 0ms duration; status badge clearly indicates skipped.
- **Edge case:** `html/template` escapes the embedded JS `ddFilter` function. **Handling:** The function uses only safe constructs (DOM API, no template interpolation) and lives in a `<script>` block which `html/template` treats as a JS context; numeric indices `{{$gi}}` are interpolated via `data-group` attributes, not inside the JS string.
- **Risk:** `IterationData` map iteration order is non-deterministic in Go. **Mitigation:** Always iterate via the sorted `DataColumns` slice in the template; never range the map directly.
- **Risk:** Large data-driven runs (10,000 rows) produce huge HTML. **Mitigation:** Out of scope for this task; file-size guard is M2-040-ish future work. Document the limit in the CHANGELOG.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/output/...
go test ./internal/runner/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# Data-driven report
./apitest run examples/data-driven-collection.yaml --report-html /tmp/dd.html
open /tmp/dd.html   # confirm iteration summary card and filterable table

# Parallel report
./apitest run examples/parallel-collection.yaml --parallel --report-html /tmp/par.html
open /tmp/par.html  # confirm wave diagram and speedup factor

# Unit tests
go test ./internal/output/...
```

## Quality Checklist

- [x] Every file to be modified has been fully read
- [x] Every affected `_test.go` file has been read
- [x] All call sites of changed interfaces identified (`buildHTMLReport` in `cmd/apitest/main.go`, runner data-driven paths)
- [x] Before/after code snippets for non-trivial changes
- [x] Impact on existing tests explicitly listed (none break)
- [x] Edge cases and risks identified with mitigations
- [x] Tests specified BEFORE implementation (TDD)
- [x] Error wrapping pattern preserved (`fmt.Errorf("context: %w", err)`)
- [x] No stuttering in names (`output.HTMLDataDrivenGroup`, not `output.HTMLHTMLDataDrivenGroup`)
- [x] Table-driven test cases named upfront
- [x] Steps ordered by blast radius (runner additive field → output types → output logic → output template → output template → wiring → docs)
- [x] Observable verification command is concrete and runnable
