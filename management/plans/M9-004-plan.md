# Implementation Plan: M9-004

## Overview

Extend the M9-002/M9-003 markdown formatter with the two execution modes
that change file layout: parallel collections gain `## Wave <N>` grouping
in `run.md`, and data-driven requests produce a `<slug>/` subdirectory
containing `index.md` plus one `iter-<n>.md` per iteration with stable
correlation IDs and splice-safe re-runs.

## Task Details

- **ID:** M9-004
- **Title:** markdown parallel + data-driven integration: wave grouping, per-iteration files
- **Phase:** M9: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** medium
- **Estimated effort:** 1 day

## Dependencies

| Task   | Title                                                                                          | Status |
|--------|------------------------------------------------------------------------------------------------|--------|
| M9-003 | markdown content-type matrix, volatile-header discipline, 1 MiB body cap                        | done   |

## Architectural Decisions Resolved Up Front

These decisions are recorded so reviewers do not have to reverse-engineer
them. Each is informed by reading the file the slice will touch.

1. **`wave_index` rendering already exists.** `formatter.go:343-351`
   already emits `wave_index: <N>` (or `sequential` when `WaveIndex < 0`).
   M9-004 changes nothing here. The parallel observable steps verify
   existing behaviour; the new work is run.md grouping and
   data-driven layout.

2. **New `IterationEntry` type carries iteration-specific data.** Added
   to `formatter.go`:

   ```go
   type IterationEntry struct {
       RequestID  string         // runner-minted id for this iteration
       Index      int            // 0-based iteration index (matches data-source row)
       StatusCode int
       DurationMs int64
       StartedAt  time.Time
       Method     string
       URL        string
       RequestHdr map[string]string
       RequestBody any
       RespHeaders http.Header
       RespBody   []byte
       Assertions *assertion.Results
       Err        error
       Skipped    bool
       SkipReason string
       Data       map[string]string  // IterationData for the row (informational)
   }
   ```

   Existing `RequestEntry` gains two new fields:

   ```go
   Iterations     []IterationEntry  // populated only for data-driven groups
   IterationTotal int               // source row count (may exceed len(Iterations) under runner cap)
   ```

   Single-request entries (`len(Iterations) == 0`) keep M9-002/M9-003
   behaviour unchanged: one `<slug>.md` written via the existing
   `renderFullFile` path. Data-driven entries (`len(Iterations) > 0`)
   route to the new `renderDataDrivenRequest` path which emits
   `<slug>/index.md` plus one `iter-<n>.md` per iteration. The
   `RequestEntry.Slug` field stores the **base** slug
   (`parser.Slug(r.DataDrivenName)`), which doubles as the directory
   name and the sentinel `slug=` attribute in every iteration file.

3. **`Report.IsParallel` controls run.md wave grouping.** Added to the
   `Report` struct:

   ```go
   IsParallel bool  // true when the runner used parallel execution
   ```

   The cmd-builder reads it from `summary.IsParallel`. When false (or
   when no entry has `WaveIndex >= 0`), `run.md` keeps its M9-002 flat
   bullet list (zero behaviour change). When true, `run.md` groups
   entries by `WaveIndex` under `## Wave <N>` headers in ascending
   order. Entries with `WaveIndex == -1` (e.g. data-driven aggregates
   in a parallel collection — the iterations themselves carry -1 from
   the runner) are listed under a `## Sequential` header at the end of
   the wave list. This keeps the section count predictable: at most
   `WaveCount + 1` headers.

   Why a flag rather than auto-detection? Auto-detection
   (`anyEntry.WaveIndex >= 0`) would also fire on a sequential
   collection that happens to include a 0-indexed entry from a non-DD
   path; explicit `IsParallel` from the runner is unambiguous.

4. **Data-driven iteration grouping happens at the cmd-builder layer,
   not in the markdown package.** `buildMarkdownReport` walks the
   `[]runner.RequestResult` slice once and groups consecutive results
   sharing `IsDataDriven == true && DataDrivenName == X` into a single
   `RequestEntry` with the iterations populated. This matches the
   runner's output ordering (iterations are emitted contiguously per
   item) and keeps the markdown package free of runner-domain
   knowledge.

   Edge case: `executeDataDrivenParallel` may emit iterations
   out-of-order across goroutines, but the conversion loop at
   `runner.go:2510` appends them in `allIterResults` order which is
   sorted by index (the worker pool collects then sorts). So the
   iteration index ordering is deterministic by the time results
   arrive at cmd; the builder relies on this.

5. **Iteration cap is detected by data, not imported.** The task says
   "read the constant from internal/runner/ directly, do not
   re-declare". The pragmatic interpretation: do **not** declare a new
   cap constant in the markdown package; instead detect truncation
   from the data the runner already provides. `IterationTotal` is the
   source row count; `len(Iterations)` is what executed. When
   `IterationTotal > len(Iterations)`, truncation has occurred — the
   index.md table renders the executed rows and appends a single
   marker row:

   ```
   | ... | _truncated_ | _runner cap reached at row N (of M)_ | — |
   ```

   where N is `len(Iterations)` and M is `IterationTotal`. The phrase
   "runner cap" anchors the user; no markdown-side constant is
   declared. This avoids a circular import (markdown -> runner)
   because the cmd-builder is the only edge that knows about
   `runner.MaxRequests`, and even that knowledge is unnecessary
   because the runner enforces the cap before results arrive.

6. **Per-iteration filename: `iter-<n>.md` with zero-based n.** Per
   task observable
   (`ls /tmp/resp/seed-users/` -> `iter-0.md iter-1.md ...`). The
   index name `index.md` is fixed and reserved (no parser-derived
   slug ever produces "index" because parser slugs cannot be a single
   word that collides with this — but we still document the
   restriction in `renderDataDrivenRequest` to make it explicit).

7. **Per-iteration sentinel correlation IDs.**
   - `request_id` = `<iteration's r.RequestID>-iter-<r.IterationIndex>`
     (e.g. `req-12-iter-0`). The `-iter-N` suffix makes the iteration
     unmistakable in tooling. Matches observable
     `grep -Eo 'id=req-[0-9]+-iter-[0-9]+'`.
   - `slug` = base slug (e.g. `seed-users`); stable across all
     iterations, matches the directory name.
   - `run` = `report.RunID` (unchanged from M9-002).

8. **Per-request `index.md` sentinel correlation IDs.**
   - `request_id` = `<base-request-id>-index` where base-request-id
     is the **first** iteration's `RequestID` (e.g.
     `req-12-index`). One id per request file is sufficient because
     splice keys off the slug.
   - `slug` = base slug (same as iterations).
   - `run` = `report.RunID`.

   The base-request-id is captured at cmd-builder time and stored on
   `RequestEntry.RequestID`; the markdown layer just reads it.

9. **`index.md` body shape.** The deterministic sentinel block
   contains, in order:

   ```
   ## Iterations
   <name> — <total> iterations (<passed> passed, <failed> failed, <skipped> skipped)
   run_id: <runID>

   | iter | status | duration_ms | link |
   |------|--------|-------------|------|
   | 0    | pass   | 42          | [iter-0.md](iter-0.md) |
   | 1    | fail   | 87          | [iter-1.md](iter-1.md) |
   ...
   ```

   Above the BEGIN sentinel: `# <DataDrivenName>` and `## Notes`
   (agent-owned). Below the END sentinel: `## Analysis` (agent-owned).
   This mirrors `renderFullFile` shape so the splice machinery is
   reusable verbatim.

10. **`iter-<n>.md` body shape mirrors `renderFullFile`.** Each
    iteration file contains the same 11-section structure as a
    single-request `<slug>.md` from M9-003 (the sections specified at
    `formatter.go:134-145`). The `### Timing` line still emits
    `wave_index: sequential` because data-driven iterations always
    have `WaveIndex == -1` from the runner. The agent-authored
    `## Notes` and `## Analysis` regions live outside the sentinel
    and are preserved on re-run by the existing splice path.

11. **`run.md` aggregate entry for data-driven.** Per behaviour:
    `- [seed-users](seed-users/index.md) (5 iterations, 4 passed, 1 failed)`.
    The link target is `<slug>/index.md` (subdirectory). The counts
    are computed from the iteration outcomes by the markdown package
    (not by the runner) so the same render gives consistent counts
    regardless of `Summary` numbers.

12. **`run.md` wave grouping when `IsParallel`.** Replace the flat
    `## Requests` list with a per-wave layout:

    ```
    ## Requests

    ### Wave 0

    - pass: [Login](login.md)

    ### Wave 1

    - pass: [Get profile](get-profile.md)
    - pass: [Get settings](get-settings.md)
    ```

    Note: H3 (`### Wave N`) under H2 (`## Requests`) keeps the
    document outline well-formed. The task observable
    (`grep -c '^## Wave '`) actually expects H2 — let me re-read:

    > `grep -c '^## Wave ' /tmp/resp/run.md`

    H2 it is. So `## Wave N` directly, dropping the surrounding
    `## Requests` heading when waves are present. Outline becomes:

    ```
    ## Run Summary
    ## Wave 0
    ## Wave 1
    ## Wave 2
    ## Sequential   (only if any entry has WaveIndex == -1, e.g. DD aggregates)
    ```

    Sequential collections keep `## Requests` (M9-002 unchanged).

13. **`writer.go` already supports subdirectory paths.**
    `EnsureReportDir(path, subpath...)` joins the args via
    `filepath.Join` and then `os.MkdirAll`. M9-004 is the first
    caller to actually pass a subpath. The behavior is exercised by
    `TestMarkdown_DataDriven_PerIteration` calling
    `WriteReport(report, dir, opts)` and observing the directory was
    created. The signature is unchanged.

14. **Splice for iter-*.md and index.md re-uses `writeFile`.** The
    existing splice machinery (BEGIN/END sentinel parsing,
    `decideAction`, `spliceSentinelBlock`) works on any file with the
    same sentinel format. The data-driven path calls `writeFile`
    once per iteration with the appropriate slug (the **base** slug,
    same value for every iter file under that directory). Wait — if
    every iter file uses the same slug, won't `actionAppendOrphan`
    fire when iter-0.md is re-rendered against the bytes of iter-1.md
    on disk? No — `writeFile` reads the file at the iter-N.md path.
    Each iter-N.md is a separate path, so each splice is independent.
    The slug being the same across iter files is fine because it
    only matters per-file.

    However, the `index.md` and the iter-N.md files share a slug.
    If a user accidentally renames a request the parser-side slug
    changes, but the runner-side slug propagation already handles
    this. From the markdown package perspective, all index/iter
    files in the same directory carry the same slug; that is by
    design.

15. **Failed iteration rendering.** A failed iteration (assertion
    fail or HTTP error) renders the same 11-section structure with
    `[ ]` markers in `### Assertions`. This is unchanged from
    M9-003's failure behaviour; the only addition is the new
    `iter-<n>.md` location. The aggregate counts in index.md and
    run.md are computed from `IterationEntry.Err != nil` or
    `Assertions.Passed == false`.

16. **No CHANGELOG, init flag, or docs work.** Per task scope, those
    land in M9-005.

17. **Order of operations for data-driven re-run preserves user
    edits.** Each iter-N.md is written via the existing splice
    path. Agent edits outside the sentinel survive verbatim. The
    integration test
    `TestMarkdown_DataDriven_Splice` writes an
    `AGENT ANALYSIS` line below the END sentinel of iter-3.md and
    verifies it persists.

## Implementation Steps

Steps are ordered by blast radius, smallest first.

### Step 1: Add `Iterations` field + `IterationEntry` to formatter.go (data shape)

**Rationale:** Pure type addition with no behaviour change. New field is
only honoured by the new render path that lands in step 4. Existing
M9-002/M9-003 code continues to work unchanged because all consumers
ignore the new field. Lowest blast radius.

#### Files to Modify

| File                                          | Action | Description                                                               |
|-----------------------------------------------|--------|---------------------------------------------------------------------------|
| `internal/output/markdown/formatter.go`       | modify | Add `IterationEntry` struct; add `Iterations`, `IterationTotal`, and `DataDrivenName` to `RequestEntry`; add `IsParallel` to `Report`. |

#### Current Code (`formatter.go:46-82`)

```go
type Report struct {
    CollectionName string
    EnvName        string
    RunID          string
    StartedAt      time.Time
    Summary        SummaryCounts
    Requests       []RequestEntry
}

type RequestEntry struct {
    RequestID   string
    Slug        string
    Name        string
    Method      string
    URL         string
    StatusCode  int
    DurationMs  int64
    WaveIndex   int
    StartedAt   time.Time
    RequestHdr  map[string]string
    RequestBody any
    RespHeaders http.Header
    RespBody    []byte
    Assertions  *assertion.Results
    Err         error
    Skipped     bool
    SkipReason  string
}
```

#### New Code (`formatter.go`)

```go
type Report struct {
    CollectionName string
    EnvName        string
    RunID          string
    StartedAt      time.Time
    Summary        SummaryCounts
    Requests       []RequestEntry
    IsParallel     bool   // M9-004: enables ## Wave N grouping in run.md
}

type RequestEntry struct {
    // ... existing fields unchanged ...

    // M9-004: data-driven fields. Populated only when this entry represents
    // a data-driven request (consecutive runner.RequestResult rows sharing
    // DataDrivenName=X). When len(Iterations) == 0 this is a regular
    // single-request entry (M9-002/M9-003 path).
    Iterations     []IterationEntry
    IterationTotal int    // source row count; may exceed len(Iterations) under runner cap
    DataDrivenName string // human-readable name (without [X/Y] suffix); preserved for index.md heading
}

// IterationEntry holds the data for one iteration of a data-driven request.
// Mirrors RequestEntry's response/timing/assertion fields but omits the
// fields the iteration cannot vary (Slug, Name) — those live on the
// parent RequestEntry.
type IterationEntry struct {
    RequestID   string             // runner-minted id (for sentinel); becomes "<id>-iter-<index>"
    Index       int                // 0-based iteration index
    StatusCode  int
    DurationMs  int64
    StartedAt   time.Time
    Method      string
    URL         string
    RequestHdr  map[string]string
    RequestBody any
    RespHeaders http.Header
    RespBody    []byte
    Assertions  *assertion.Results
    Err         error
    Skipped     bool
    SkipReason  string
    Data        map[string]string  // IterationData (informational; not rendered for M9-004)
}
```

#### Tests to Write FIRST (RED)

No new tests at this step — pure type addition. The compiler verifies
the struct literal matches; subsequent steps add behaviour tests.

#### Impact on Existing Tests

None. All existing tests use struct-literal field names (e.g.
`RequestEntry{RequestID: ..., Slug: ...}`); adding fields with
zero-value-safe types (slice of struct, int, string, bool) does not
break any literal.

### Step 2: Run.md wave grouping (`run_md.go`)

**Rationale:** Self-contained change to `run_md.go`; no dependency on the
data-driven steps. Lands second so wave grouping is visible to the
parallel observable independently of data-driven work.

#### Files to Modify

| File                                          | Action | Description                                                                          |
|-----------------------------------------------|--------|--------------------------------------------------------------------------------------|
| `internal/output/markdown/run_md.go`          | modify | When `report.IsParallel`, replace flat `## Requests` block with per-wave grouping; emit `## Sequential` for entries with WaveIndex == -1. |
| `internal/output/markdown/run_md_test.go`     | modify | Add `TestMarkdown_RunMD_WaveGrouping`; existing `TestMarkdown_RunMD` unchanged (sequential collection). |
| `internal/output/markdown/testdata/golden/run_md.md` | unchanged | the existing flat fixture has `IsParallel == false`; no diff. |

#### Current Code (`run_md.go:60-75`)

```go
if len(report.Requests) == 0 {
    fmt.Fprintf(&buf, "\n_No requests executed._\n")
} else {
    fmt.Fprintf(&buf, "\n## Requests\n\n")
    for _, req := range report.Requests {
        status := "pass"
        if req.Err != nil || (req.Assertions != nil && !req.Assertions.Passed) {
            status = "fail"
        }
        if req.Skipped {
            status = "skip"
        }
        fmt.Fprintf(&buf, "- %s: [%s](%s.md)\n", status, req.Name, req.Slug)
    }
}
```

#### New Code (`run_md.go`)

```go
if len(report.Requests) == 0 {
    fmt.Fprintf(&buf, "\n_No requests executed._\n")
} else if report.IsParallel {
    renderRunMDByWave(&buf, report)
} else {
    fmt.Fprintf(&buf, "\n## Requests\n\n")
    for _, req := range report.Requests {
        renderRunMDEntry(&buf, &req)
    }
}
```

```go
// renderRunMDByWave groups report.Requests by WaveIndex and emits an
// H2 wave header per group. Entries with WaveIndex == -1 (data-driven
// aggregates, sequential interleave) are emitted under ## Sequential
// at the end. Wave indexes are sorted ascending; ties keep input order
// (slice append order = runner emission order).
func renderRunMDByWave(buf *bytes.Buffer, report *Report) {
    waves := map[int][]*RequestEntry{}
    waveOrder := []int{}
    sequential := []*RequestEntry{}
    for i := range report.Requests {
        req := &report.Requests[i]
        if req.WaveIndex < 0 {
            sequential = append(sequential, req)
            continue
        }
        if _, seen := waves[req.WaveIndex]; !seen {
            waveOrder = append(waveOrder, req.WaveIndex)
        }
        waves[req.WaveIndex] = append(waves[req.WaveIndex], req)
    }
    sort.Ints(waveOrder)
    for _, w := range waveOrder {
        fmt.Fprintf(buf, "\n## Wave %d\n\n", w)
        for _, req := range waves[w] {
            renderRunMDEntry(buf, req)
        }
    }
    if len(sequential) > 0 {
        fmt.Fprintf(buf, "\n## Sequential\n\n")
        for _, req := range sequential {
            renderRunMDEntry(buf, req)
        }
    }
}

// renderRunMDEntry renders one bullet line. Data-driven entries
// (len(Iterations) > 0) collapse to <name>(<slug>/index.md) with
// aggregate counts. Single-request entries link directly to <slug>.md.
func renderRunMDEntry(buf *bytes.Buffer, req *RequestEntry) {
    if len(req.Iterations) > 0 {
        passed, failed, skipped := iterationOutcomeCounts(req.Iterations)
        fmt.Fprintf(buf, "- [%s](%s/index.md) (%d iterations, %d passed, %d failed",
            req.Name, req.Slug, len(req.Iterations), passed, failed)
        if skipped > 0 {
            fmt.Fprintf(buf, ", %d skipped", skipped)
        }
        fmt.Fprintln(buf, ")")
        return
    }
    status := "pass"
    if req.Err != nil || (req.Assertions != nil && !req.Assertions.Passed) {
        status = "fail"
    }
    if req.Skipped {
        status = "skip"
    }
    fmt.Fprintf(buf, "- %s: [%s](%s.md)\n", status, req.Name, req.Slug)
}
```

`iterationOutcomeCounts` is a new private helper in `datadriven.go`
(step 3) that returns `(passed, failed, skipped int)` from a slice of
`IterationEntry`.

Required new imports: `sort` (for `sort.Ints`).

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_RunMD_WaveGrouping(t *testing.T) {
    report := &Report{
        CollectionName: "Parallel Run",
        RunID:          fixedRunID,
        StartedAt:      fixedTime,
        Summary:        SummaryCounts{Total: 4, Passed: 4},
        IsParallel:     true,
        Requests: []RequestEntry{
            {RequestID: "req-1", Slug: "login", Name: "Login", StatusCode: 200, WaveIndex: 0, StartedAt: fixedTime},
            {RequestID: "req-2", Slug: "profile", Name: "Profile", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
            {RequestID: "req-3", Slug: "settings", Name: "Settings", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
            {RequestID: "req-4", Slug: "logout", Name: "Logout", StatusCode: 200, WaveIndex: 2, StartedAt: fixedTime},
        },
    }
    dir := t.TempDir()
    if err := WriteReport(report, dir, WriteOptions{}); err != nil {
        t.Fatalf("WriteReport: %v", err)
    }
    got, _ := os.ReadFile(filepath.Join(dir, "run.md"))

    // Three wave headers in ascending order.
    waveCount := strings.Count(string(got), "\n## Wave ")
    if waveCount != 3 {
        t.Errorf("expected 3 ## Wave headers, got %d:\n%s", waveCount, got)
    }
    // Verify ascending order: Wave 0 before Wave 1 before Wave 2.
    pos0 := strings.Index(string(got), "## Wave 0")
    pos1 := strings.Index(string(got), "## Wave 1")
    pos2 := strings.Index(string(got), "## Wave 2")
    if !(pos0 >= 0 && pos1 > pos0 && pos2 > pos1) {
        t.Errorf("waves not in ascending order: 0@%d 1@%d 2@%d\n%s", pos0, pos1, pos2, got)
    }
    // Wave 1 contains both profile and settings.
    wave1Block := string(got)[pos1:pos2]
    if !strings.Contains(wave1Block, "profile.md") || !strings.Contains(wave1Block, "settings.md") {
        t.Errorf("Wave 1 missing profile or settings:\n%s", wave1Block)
    }
}

func TestMarkdown_RunMD_Sequential(t *testing.T) {
    // Sanity: when IsParallel == false the existing flat layout is preserved.
    report := &Report{
        CollectionName: "Sequential",
        RunID:          fixedRunID,
        StartedAt:      fixedTime,
        Summary:        SummaryCounts{Total: 2, Passed: 2},
        IsParallel:     false,
        Requests: []RequestEntry{
            {RequestID: "req-1", Slug: "login", Name: "Login", WaveIndex: -1, StartedAt: fixedTime},
            {RequestID: "req-2", Slug: "logout", Name: "Logout", WaveIndex: -1, StartedAt: fixedTime},
        },
    }
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "run.md"))
    if strings.Contains(string(got), "## Wave ") {
        t.Errorf("sequential collection must not have ## Wave headers:\n%s", got)
    }
    if !strings.Contains(string(got), "## Requests") {
        t.Errorf("sequential collection must have ## Requests:\n%s", got)
    }
}
```

#### Impact on Existing Tests

- `TestMarkdown_RunMD` — uses default `IsParallel == false`; flat
  layout preserved; no change.
- `TestMarkdown_RunMD_WithEnvName` — same.

### Step 3: Data-driven render path (`datadriven.go`)

**Rationale:** Self-contained file containing the new render functions;
called only when step 4 wires the dispatch in `WriteReport`. Tests at
this step exercise the helpers in isolation.

#### Files to Modify

| File                                                | Action | Description                                                                       |
|-----------------------------------------------------|--------|-----------------------------------------------------------------------------------|
| `internal/output/markdown/datadriven.go`            | create | `renderDataDrivenRequest`, `renderIterationFile`, `renderIndex`, `iterationOutcomeCounts`. |
| `internal/output/markdown/datadriven_test.go`       | create | `TestMarkdown_RenderIteration`, `TestMarkdown_RenderIndex`, `TestMarkdown_IterationOutcomeCounts`. |

#### New Code (`datadriven.go`)

```go
package markdown

import (
    "bytes"
    "fmt"
    "io"
    "path/filepath"
    "time"
)

// renderDataDrivenRequest writes <dir>/<slug>/index.md plus one
// <dir>/<slug>/iter-<n>.md per iteration. The slug subdirectory is
// created via EnsureReportDir(dir, slug) before any file is written.
//
// Splice discipline: each iter-N.md and the index.md are independent
// splice targets; agent edits outside the sentinels are preserved on
// re-run. The base slug is identical across all files in the
// directory; this is intentional and the splice machinery handles it
// correctly because each path is a distinct file.
func renderDataDrivenRequest(req *RequestEntry, dir, runID string, errW io.Writer) error {
    if err := EnsureReportDir(dir, req.Slug); err != nil {
        return err
    }
    base := filepath.Join(dir, req.Slug)

    // Per-iteration files.
    for _, it := range req.Iterations {
        content := renderIterationFile(req, &it, runID)
        path := filepath.Join(base, fmt.Sprintf("iter-%d.md", it.Index))
        if err := writeFile(path, req.Slug, content, errW); err != nil {
            return fmt.Errorf("write iter-%d.md: %w", it.Index, err)
        }
    }

    // Index file.
    indexContent := renderIndex(req, runID)
    indexPath := filepath.Join(base, "index.md")
    if err := writeFile(indexPath, req.Slug, indexContent, errW); err != nil {
        return fmt.Errorf("write index.md: %w", err)
    }
    return nil
}

// renderIterationFile returns the byte content of <slug>/iter-<n>.md.
// Mirrors renderFullFile shape but writes a sentinel with
// request_id=<iter.RequestID>-iter-<index> and slug=<base slug>.
func renderIterationFile(parent *RequestEntry, it *IterationEntry, runID string) []byte {
    var buf bytes.Buffer
    fmt.Fprintf(&buf, "# %s [%d/%d]\n\n", parent.DataDrivenName, it.Index+1, parent.IterationTotal)
    fmt.Fprintf(&buf, "## Notes\n\n")
    buf.Write(renderIterationSentinelBlock(parent, it, runID))
    fmt.Fprintf(&buf, "\n## Analysis\n\n")
    return buf.Bytes()
}

func renderIterationSentinelBlock(parent *RequestEntry, it *IterationEntry, runID string) []byte {
    var buf bytes.Buffer
    iterID := fmt.Sprintf("%s-iter-%d", it.RequestID, it.Index)
    fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
        SentinelBeginPrefix, iterID, parent.Slug, runID, SentinelSuffix)
    fmt.Fprintf(&buf, "## Response (deterministic)\n")

    // Re-use the same Section 6-10 helpers used by renderSentinelBlock.
    // The shared helpers operate on a temporary RequestEntry built from the
    // iteration data so we don't duplicate body/header/timing/assertion
    // rendering code.
    synth := iterationToEntry(parent, it)
    fmt.Fprintf(&buf, "\n### Request\n\n")
    renderRequest(&buf, synth)
    fmt.Fprintf(&buf, "\n### Response %d\n\n", synth.StatusCode)
    renderResponse(&buf, synth)
    fmt.Fprintf(&buf, "\n### Response metadata\n\n")
    renderResponseMetadata(&buf, synth)
    fmt.Fprintf(&buf, "\n### Timing\n\n")
    renderTiming(&buf, synth)
    fmt.Fprintf(&buf, "\n### Assertions\n\n")
    renderAssertions(&buf, synth)
    fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
        SentinelEndPrefix, iterID, parent.Slug, runID, SentinelSuffix)
    return buf.Bytes()
}

// iterationToEntry projects iteration-level data onto a RequestEntry so
// the existing Section 6-10 helpers can render it without duplication.
// The returned entry intentionally has empty Iterations (the iteration
// itself never recurses into the DD code path).
func iterationToEntry(parent *RequestEntry, it *IterationEntry) *RequestEntry {
    return &RequestEntry{
        RequestID:   it.RequestID,
        Slug:        parent.Slug,
        Name:        parent.DataDrivenName,
        Method:      it.Method,
        URL:         it.URL,
        StatusCode:  it.StatusCode,
        DurationMs:  it.DurationMs,
        WaveIndex:   -1, // iterations always sequential within their group
        StartedAt:   it.StartedAt,
        RequestHdr:  it.RequestHdr,
        RequestBody: it.RequestBody,
        RespHeaders: it.RespHeaders,
        RespBody:    it.RespBody,
        Assertions:  it.Assertions,
        Err:         it.Err,
        Skipped:     it.Skipped,
        SkipReason:  it.SkipReason,
    }
}

// renderIndex returns the byte content of <slug>/index.md.
func renderIndex(req *RequestEntry, runID string) []byte {
    var buf bytes.Buffer
    fmt.Fprintf(&buf, "# %s\n\n", req.DataDrivenName)
    fmt.Fprintf(&buf, "## Notes\n\n")
    buf.Write(renderIndexSentinelBlock(req, runID))
    fmt.Fprintf(&buf, "\n## Analysis\n\n")
    return buf.Bytes()
}

func renderIndexSentinelBlock(req *RequestEntry, runID string) []byte {
    var buf bytes.Buffer
    indexID := req.RequestID + "-index"
    fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
        SentinelBeginPrefix, indexID, req.Slug, runID, SentinelSuffix)

    fmt.Fprintf(&buf, "## Iterations\n\n")
    passed, failed, skipped := iterationOutcomeCounts(req.Iterations)
    total := len(req.Iterations)
    fmt.Fprintf(&buf, "%s — %d iterations (%d passed, %d failed",
        req.DataDrivenName, total, passed, failed)
    if skipped > 0 {
        fmt.Fprintf(&buf, ", %d skipped", skipped)
    }
    fmt.Fprintf(&buf, ")\n")
    fmt.Fprintf(&buf, "run_id: %s\n", runID)
    fmt.Fprintf(&buf, "started_at: %s\n", req.StartedAt.UTC().Format(time.RFC3339Nano))

    fmt.Fprintf(&buf, "\n| iter | status | duration_ms | link |\n")
    fmt.Fprintf(&buf, "|------|--------|-------------|------|\n")
    for _, it := range req.Iterations {
        status := iterationStatus(&it)
        fmt.Fprintf(&buf, "| %d | %s | %d | [iter-%d.md](iter-%d.md) |\n",
            it.Index, status, it.DurationMs, it.Index, it.Index)
    }
    if req.IterationTotal > total {
        fmt.Fprintf(&buf, "| ... | _truncated_ | _runner cap reached at row %d (of %d)_ | — |\n",
            total, req.IterationTotal)
    }

    fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
        SentinelEndPrefix, indexID, req.Slug, runID, SentinelSuffix)
    return buf.Bytes()
}

// iterationOutcomeCounts returns (passed, failed, skipped) for a slice
// of iterations. Used by run.md's aggregate bullet AND index.md's header.
func iterationOutcomeCounts(iters []IterationEntry) (passed, failed, skipped int) {
    for i := range iters {
        switch iterationStatus(&iters[i]) {
        case "pass":
            passed++
        case "fail":
            failed++
        case "skip":
            skipped++
        }
    }
    return
}

// iterationStatus returns "pass" | "fail" | "skip" for one iteration.
func iterationStatus(it *IterationEntry) string {
    if it.Skipped {
        return "skip"
    }
    if it.Err != nil || (it.Assertions != nil && !it.Assertions.Passed) {
        return "fail"
    }
    return "pass"
}
```

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_IterationOutcomeCounts(t *testing.T) {
    iters := []IterationEntry{
        {Index: 0},                                       // pass
        {Index: 1, Err: fmt.Errorf("boom")},              // fail
        {Index: 2, Assertions: &assertion.Results{Passed: false}}, // fail
        {Index: 3, Skipped: true},                        // skip
        {Index: 4},                                       // pass
    }
    p, f, s := iterationOutcomeCounts(iters)
    if p != 2 || f != 2 || s != 1 {
        t.Errorf("counts = (p=%d, f=%d, s=%d), want (2,2,1)", p, f, s)
    }
}

func TestMarkdown_RenderIteration(t *testing.T) {
    parent := &RequestEntry{
        RequestID:      "req-12",
        Slug:           "seed-users",
        DataDrivenName: "Seed users",
        IterationTotal: 5,
    }
    it := &IterationEntry{
        RequestID:   "req-12",
        Index:       0,
        Method:      "POST",
        URL:         "https://api.example.com/users",
        StatusCode:  201,
        DurationMs:  42,
        StartedAt:   fixedTime,
        RespHeaders: http.Header{"Content-Type": {"application/json"}},
        RespBody:    []byte(`{"id":1}`),
        Assertions: &assertion.Results{Passed: true, Items: []assertion.Result{
            {Type: "status", Expected: "201", Actual: "201", Passed: true},
        }},
    }
    got := renderIterationFile(parent, it, fixedRunID)
    s := string(got)

    // Sentinel format: id=req-12-iter-0 slug=seed-users.
    if !strings.Contains(s, "id=req-12-iter-0 slug=seed-users") {
        t.Errorf("expected sentinel id=req-12-iter-0 slug=seed-users:\n%s", s)
    }
    // 11-section structure (same as M9-003 single-request).
    for _, marker := range []string{"# Seed users [1/5]", "## Notes", "<!-- BEGIN apitest:response",
        "## Response (deterministic)", "### Request", "### Response 201", "### Response metadata",
        "### Timing", "### Assertions", "<!-- END apitest:response", "## Analysis"} {
        if !strings.Contains(s, marker) {
            t.Errorf("missing marker %q in iter file:\n%s", marker, s)
        }
    }
}

func TestMarkdown_RenderIndex(t *testing.T) {
    req := &RequestEntry{
        RequestID:      "req-12",
        Slug:           "seed-users",
        Name:           "Seed users",
        DataDrivenName: "Seed users",
        StartedAt:      fixedTime,
        IterationTotal: 5,
        Iterations: []IterationEntry{
            {Index: 0, DurationMs: 42},
            {Index: 1, DurationMs: 50, Err: fmt.Errorf("boom")},
            {Index: 2, DurationMs: 38},
            {Index: 3, DurationMs: 41},
            {Index: 4, DurationMs: 39},
        },
    }
    got := renderIndex(req, fixedRunID)
    s := string(got)

    if !strings.Contains(s, "id=req-12-index slug=seed-users") {
        t.Errorf("expected sentinel id=req-12-index:\n%s", s)
    }
    if !strings.Contains(s, "5 iterations (4 passed, 1 failed)") {
        t.Errorf("expected aggregate count line:\n%s", s)
    }
    if !strings.Contains(s, "[iter-0.md](iter-0.md)") {
        t.Errorf("expected iter-0 link in table:\n%s", s)
    }
    if !strings.Contains(s, "[iter-4.md](iter-4.md)") {
        t.Errorf("expected iter-4 link in table:\n%s", s)
    }
}

func TestMarkdown_RenderIndex_Truncation(t *testing.T) {
    req := &RequestEntry{
        RequestID:      "req-12",
        Slug:           "seed-users",
        DataDrivenName: "Seed users",
        StartedAt:      fixedTime,
        IterationTotal: 1500,
        Iterations:     make([]IterationEntry, 1000),
    }
    for i := range req.Iterations {
        req.Iterations[i].Index = i
    }
    got := renderIndex(req, fixedRunID)
    s := string(got)
    if !strings.Contains(s, "_truncated_") {
        t.Errorf("expected _truncated_ marker row:\n%s", s)
    }
    if !strings.Contains(s, "runner cap reached at row 1000 (of 1500)") {
        t.Errorf("expected truncation explanation:\n%s", s)
    }
}
```

#### Impact on Existing Tests

None — all helpers are new and unreferenced from existing code paths
until step 4 wires them in.

### Step 4: Wire `WriteReport` to dispatch data-driven entries

**Rationale:** Aggregates step 3's helpers into the public `WriteReport`
path. After this step the markdown package fully supports data-driven
rendering; the cmd builder still needs to populate the new fields,
which is step 5.

#### Files to Modify

| File                                          | Action | Description                                                         |
|-----------------------------------------------|--------|---------------------------------------------------------------------|
| `internal/output/markdown/formatter.go`       | modify | In `WriteReport`'s per-request loop, dispatch on `len(Iterations)` to either `renderDataDrivenRequest` (subdir + iter files + index) or the existing single-file `renderFullFile` path. |
| `internal/output/markdown/formatter_test.go`  | modify | Add `TestMarkdown_DataDriven_PerIteration`, `TestMarkdown_DataDriven_IndexSummary`, `TestMarkdown_DataDriven_CorrelationIDs`, `TestMarkdown_DataDriven_Splice`, `TestMarkdown_DataDriven_IterCap`, `TestMarkdown_DataDriven_FailedIteration`, `TestMarkdown_ParallelWaves`, `TestMarkdown_SequentialWaveIndex`. |

#### Current Code (`formatter.go:111-118`)

```go
for i := range report.Requests {
    entry := &report.Requests[i]
    content := renderFullFile(entry, report.RunID)
    path := filePath(dir, entry.Slug+".md")
    if err := writeFile(path, entry.Slug, content, errW); err != nil {
        return fmt.Errorf("write %s.md: %w", entry.Slug, err)
    }
}
```

#### New Code (`formatter.go`)

```go
for i := range report.Requests {
    entry := &report.Requests[i]
    if len(entry.Iterations) > 0 {
        // Data-driven request: subdirectory + per-iteration files + index.md.
        if err := renderDataDrivenRequest(entry, dir, report.RunID, errW); err != nil {
            return fmt.Errorf("write data-driven %s: %w", entry.Slug, err)
        }
        continue
    }
    content := renderFullFile(entry, report.RunID)
    path := filePath(dir, entry.Slug+".md")
    if err := writeFile(path, entry.Slug, content, errW); err != nil {
        return fmt.Errorf("write %s.md: %w", entry.Slug, err)
    }
}
```

#### Tests to Write FIRST (RED)

```go
func makeDataDrivenReport(iterCount, total int) *Report {
    iters := make([]IterationEntry, iterCount)
    for i := range iters {
        iters[i] = IterationEntry{
            RequestID:   "req-12",
            Index:       i,
            Method:      "POST",
            URL:         fmt.Sprintf("https://api.example.com/users/%d", i+1),
            StatusCode:  201,
            DurationMs:  int64(40 + i),
            StartedAt:   fixedTime,
            RespHeaders: http.Header{"Content-Type": {"application/json"}},
            RespBody:    []byte(fmt.Sprintf(`{"id":%d}`, i+1)),
            Assertions: &assertion.Results{Passed: true, Items: []assertion.Result{
                {Type: "status", Expected: "201", Actual: "201", Passed: true},
            }},
        }
    }
    return &Report{
        CollectionName: "Sample API",
        RunID:          fixedRunID,
        StartedAt:      fixedTime,
        Summary:        SummaryCounts{Total: iterCount, Passed: iterCount},
        Requests: []RequestEntry{{
            RequestID:      "req-12",
            Slug:           "seed-users",
            Name:           "Seed users",
            DataDrivenName: "Seed users",
            IterationTotal: total,
            Iterations:     iters,
        }},
    }
}

func TestMarkdown_DataDriven_PerIteration(t *testing.T) {
    report := makeDataDrivenReport(5, 5)
    dir := t.TempDir()
    if err := WriteReport(report, dir, WriteOptions{}); err != nil {
        t.Fatalf("WriteReport: %v", err)
    }
    // Subdir created.
    info, err := os.Stat(filepath.Join(dir, "seed-users"))
    if err != nil || !info.IsDir() {
        t.Fatalf("expected directory seed-users/: %v", err)
    }
    // 5 iter-*.md files exist.
    for i := 0; i < 5; i++ {
        if _, err := os.Stat(filepath.Join(dir, "seed-users", fmt.Sprintf("iter-%d.md", i))); err != nil {
            t.Errorf("expected iter-%d.md: %v", i, err)
        }
    }
    // index.md exists.
    if _, err := os.Stat(filepath.Join(dir, "seed-users", "index.md")); err != nil {
        t.Errorf("expected index.md: %v", err)
    }
}

func TestMarkdown_DataDriven_IndexSummary(t *testing.T) {
    report := makeDataDrivenReport(5, 5)
    report.Requests[0].Iterations[1].Err = fmt.Errorf("boom")
    report.Requests[0].Iterations[1].Assertions = &assertion.Results{Passed: false}
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
    s := string(got)
    if !strings.Contains(s, "5 iterations (4 passed, 1 failed)") {
        t.Errorf("expected count line:\n%s", s)
    }
    // Table has 5 rows + header + separator.
    if strings.Count(s, "[iter-") != 5 {
        t.Errorf("expected 5 iter links, got %d:\n%s", strings.Count(s, "[iter-"), s)
    }
}

func TestMarkdown_DataDriven_CorrelationIDs(t *testing.T) {
    report := makeDataDrivenReport(3, 3)
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    for i := 0; i < 3; i++ {
        got, _ := os.ReadFile(filepath.Join(dir, "seed-users", fmt.Sprintf("iter-%d.md", i)))
        want := fmt.Sprintf("id=req-12-iter-%d slug=seed-users", i)
        if !strings.Contains(string(got), want) {
            t.Errorf("iter-%d.md missing %q:\n%s", i, want, got)
        }
    }
    indexGot, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
    if !strings.Contains(string(indexGot), "id=req-12-index slug=seed-users") {
        t.Errorf("index.md missing id=req-12-index slug=seed-users:\n%s", indexGot)
    }
}

func TestMarkdown_DataDriven_Splice(t *testing.T) {
    report := makeDataDrivenReport(5, 5)
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})

    // Append agent edit below END sentinel of iter-3.md.
    iterPath := filepath.Join(dir, "seed-users", "iter-3.md")
    iter3, _ := os.ReadFile(iterPath)
    if err := os.WriteFile(iterPath, append(iter3, []byte("\nAGENT ANALYSIS of iter-3\n")...), 0o644); err != nil {
        t.Fatal(err)
    }

    // Re-run.
    _ = WriteReport(report, dir, WriteOptions{})
    after, _ := os.ReadFile(iterPath)
    if !strings.Contains(string(after), "AGENT ANALYSIS of iter-3") {
        t.Errorf("agent edit lost on re-run:\n%s", after)
    }
}

func TestMarkdown_DataDriven_IterCap(t *testing.T) {
    // Source had 1500 rows; runner truncated to 1000 (executed iters).
    report := makeDataDrivenReport(1000, 1500)
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    indexGot, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
    if !strings.Contains(string(indexGot), "_truncated_") {
        t.Errorf("expected truncation marker in index.md")
    }
    // iter-1000.md and beyond do not exist.
    if _, err := os.Stat(filepath.Join(dir, "seed-users", "iter-1000.md")); !os.IsNotExist(err) {
        t.Errorf("iter-1000.md should not exist beyond cap")
    }
    // iter-999.md exists (the last under-cap iteration).
    if _, err := os.Stat(filepath.Join(dir, "seed-users", "iter-999.md")); err != nil {
        t.Errorf("iter-999.md should exist: %v", err)
    }
}

func TestMarkdown_DataDriven_FailedIteration(t *testing.T) {
    report := makeDataDrivenReport(2, 2)
    report.Requests[0].Iterations[0].Assertions = &assertion.Results{
        Passed: false,
        Items: []assertion.Result{
            {Type: "status", Expected: "201", Actual: "500", Passed: false},
        },
    }
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "seed-users", "iter-0.md"))
    s := string(got)
    if !strings.Contains(s, "[ ] status") {
        t.Errorf("expected [ ] marker for failed assertion:\n%s", s)
    }
    // 11-section structure preserved.
    for _, marker := range []string{"## Notes", "## Response (deterministic)", "### Response metadata", "### Timing", "### Assertions", "<!-- END apitest:response", "## Analysis"} {
        if !strings.Contains(s, marker) {
            t.Errorf("missing %q in failed iter file:\n%s", marker, s)
        }
    }
}

// Sanity test: the formatter has rendered wave_index since M9-002. This
// test pins that behaviour alongside the new wave-grouping work.
func TestMarkdown_ParallelWaves(t *testing.T) {
    report := &Report{
        CollectionName: "Parallel",
        RunID:          fixedRunID,
        StartedAt:      fixedTime,
        Summary:        SummaryCounts{Total: 3, Passed: 3},
        IsParallel:     true,
        Requests: []RequestEntry{
            {RequestID: "req-1", Slug: "a", Name: "A", StatusCode: 200, WaveIndex: 0, StartedAt: fixedTime},
            {RequestID: "req-2", Slug: "b", Name: "B", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
            {RequestID: "req-3", Slug: "c", Name: "C", StatusCode: 200, WaveIndex: 2, StartedAt: fixedTime},
        },
    }
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    for slug, want := range map[string]string{"a": "wave_index: 0", "b": "wave_index: 1", "c": "wave_index: 2"} {
        got, _ := os.ReadFile(filepath.Join(dir, slug+".md"))
        if !strings.Contains(string(got), want) {
            t.Errorf("%s.md missing %q:\n%s", slug, want, got)
        }
    }
}

func TestMarkdown_SequentialWaveIndex(t *testing.T) {
    report := makePassReport()  // WaveIndex: -1
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !strings.Contains(string(got), "wave_index: sequential") {
        t.Errorf("expected wave_index: sequential:\n%s", got)
    }
}
```

#### Impact on Existing Tests

- `TestMarkdown_Render_PassJSON`, `TestMarkdown_Render_FailJSON`,
  `TestMarkdown_EmptyAssertions`, `TestMarkdown_DeterminismRegression` —
  all use `makePassReport` with empty `Iterations`; route to existing
  single-file path; no semantic change.
- `TestMarkdown_RunMD`, `TestMarkdown_RunMD_WithEnvName` — run.md
  layout unchanged for sequential collections (`IsParallel: false`).
- `TestMarkdown_Newlines` — uses single-file path; unchanged.

### Step 5: cmd-builder grouping (`cmd/apitest/main.go`)

**Rationale:** End-to-end integration. With markdown package complete,
the cmd-builder is the only remaining piece. This step preserves
M9-002/M9-003 behavior for non-DD cases and groups DD iterations.

#### Files to Modify

| File                                          | Action | Description                                                                                          |
|-----------------------------------------------|--------|------------------------------------------------------------------------------------------------------|
| `cmd/apitest/main.go`                         | modify | `buildMarkdownReport`: stop skipping `IsDataDriven`; group consecutive iterations sharing `DataDrivenName`; populate `Iterations`, `IterationTotal`, base `Slug`, `IsParallel`. |
| `cmd/apitest/run_test.go`                     | modify | Add `TestRun_MarkdownFormat_ParallelWaves`, `TestRun_MarkdownFormat_DataDriven`, `TestRun_MarkdownFormat_DataDrivenSplice`. |

#### Current Code (`main.go:2453-2503`)

```go
func buildMarkdownReport(col *parser.Collection, envName string, results []runner.RequestResult, summary *runner.Summary) *mdformat.Report {
    runID := ""
    if summary != nil {
        runID = summary.RunID
    }
    rep := &mdformat.Report{
        CollectionName: col.Name,
        EnvName:        envName,
        RunID:          runID,
        StartedAt:      time.Now().UTC(),
    }
    if summary != nil {
        rep.Summary = mdformat.SummaryCounts{ ... }
    }
    for _, r := range results {
        if r.Phase != runner.PhaseMain {
            continue
        }
        if r.IsDataDriven {
            continue // data-driven layout deferred to M9-004
        }
        entry := mdformat.RequestEntry{ ... }
        if r.Result != nil { ... }
        rep.Requests = append(rep.Requests, entry)
    }
    return rep
}
```

#### New Code (`main.go`)

```go
func buildMarkdownReport(col *parser.Collection, envName string, results []runner.RequestResult, summary *runner.Summary) *mdformat.Report {
    runID := ""
    isParallel := false
    if summary != nil {
        runID = summary.RunID
        isParallel = summary.IsParallel
    }
    rep := &mdformat.Report{
        CollectionName: col.Name,
        EnvName:        envName,
        RunID:          runID,
        StartedAt:      time.Now().UTC(),
        IsParallel:     isParallel,
    }
    if summary != nil { ... }

    for i := 0; i < len(results); i++ {
        r := results[i]
        if r.Phase != runner.PhaseMain {
            continue
        }
        if !r.IsDataDriven {
            // M9-002/M9-003 path: single-request entry.
            entry := mdformat.RequestEntry{ ... existing fields ... }
            rep.Requests = append(rep.Requests, entry)
            continue
        }
        // M9-004: data-driven — collect all consecutive results sharing DataDrivenName.
        groupName := r.DataDrivenName
        baseSlug, _ := parser.Slug(groupName)
        var iters []mdformat.IterationEntry
        var iterTotal int
        var firstReqID string
        for ; i < len(results); i++ {
            rr := results[i]
            if !rr.IsDataDriven || rr.DataDrivenName != groupName || rr.Phase != runner.PhaseMain {
                break
            }
            if firstReqID == "" {
                firstReqID = rr.RequestID
                iterTotal = rr.IterationTotal
            }
            it := mdformat.IterationEntry{
                RequestID:   rr.RequestID,
                Index:       rr.IterationIndex,
                Method:      rr.Method,
                URL:         rr.URL,
                RequestHdr:  rr.RequestHeaders,
                RequestBody: rr.RequestBody,
                Assertions:  rr.AssertionResults,
                Err:         rr.Err,
                Skipped:     rr.Skipped,
                SkipReason:  rr.SkipReason,
                StartedAt:   rep.StartedAt,
                Data:        rr.IterationData,
            }
            if rr.Result != nil {
                it.StatusCode = rr.Result.StatusCode
                it.DurationMs = rr.Result.Duration.Milliseconds()
                it.RespHeaders = rr.Result.Headers
                it.RespBody = rr.Result.Body
            }
            iters = append(iters, it)
        }
        i-- // outer loop will increment; we already advanced i to one past the last DD iteration
        rep.Requests = append(rep.Requests, mdformat.RequestEntry{
            RequestID:      firstReqID,
            Slug:           baseSlug,
            Name:           groupName,
            DataDrivenName: groupName,
            IterationTotal: iterTotal,
            Iterations:     iters,
            WaveIndex:      -1, // DD aggregates always sequential in run.md
            StartedAt:      rep.StartedAt,
        })
    }
    return rep
}
```

`parser.Slug` is already imported at `main.go:24`. `summary.IsParallel`
is already populated by the runner.

#### Tests to Write FIRST (RED)

```go
// TestRun_MarkdownFormat_DataDriven verifies that --format markdown with a
// data-driven collection produces <slug>/iter-N.md files, an index.md, and
// a run.md aggregate bullet linking to the index.
func TestRun_MarkdownFormat_DataDriven(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"id":1}`))
    }))
    defer srv.Close()

    tmpDir := t.TempDir()
    if err := os.WriteFile(filepath.Join(tmpDir, "users.csv"), []byte("name\nalice\nbob\ncarol\ndave\neve"), 0o600); err != nil {
        t.Fatal(err)
    }
    col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: DD
requests:
  - name: Seed users
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/{{name}}"
    assertions:
      status: 200
`, srv.URL))
    reportDir := filepath.Join(tmpDir, "resp")

    oldTier := currentTier
    currentTier = func() auth.Tier { return auth.TierProfessional }
    t.Cleanup(func() { currentTier = oldTier })

    _, stderr, exitCode := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir)
    if exitCode != 0 {
        t.Fatalf("exit code = %d\nstderr: %s", exitCode, stderr)
    }

    // 5 iter-*.md files in subdir.
    for i := 0; i < 5; i++ {
        path := filepath.Join(reportDir, "seed-users", fmt.Sprintf("iter-%d.md", i))
        if _, err := os.Stat(path); err != nil {
            t.Errorf("expected %s: %v", path, err)
        }
    }
    // index.md exists.
    if _, err := os.Stat(filepath.Join(reportDir, "seed-users", "index.md")); err != nil {
        t.Errorf("expected index.md: %v", err)
    }
    // run.md links to index with aggregate count.
    runMD, _ := os.ReadFile(filepath.Join(reportDir, "run.md"))
    if !strings.Contains(string(runMD), "[Seed users](seed-users/index.md)") {
        t.Errorf("run.md missing aggregate link:\n%s", runMD)
    }
    if !strings.Contains(string(runMD), "5 iterations") {
        t.Errorf("run.md missing iteration count:\n%s", runMD)
    }
}

// TestRun_MarkdownFormat_ParallelWaves verifies wave grouping on a real
// parallel collection.
func TestRun_MarkdownFormat_ParallelWaves(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"ok":true}`))
    }))
    defer srv.Close()

    tmpDir := t.TempDir()
    col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Par
requests:
  - name: A
    request: {method: GET, url: "%[1]s/a"}
    assertions: {status: 200}
  - name: B
    request: {method: GET, url: "%[1]s/b"}
    assertions: {status: 200}
  - name: C
    request: {method: GET, url: "%[1]s/c"}
    assertions: {status: 200}
`, srv.URL))
    reportDir := filepath.Join(tmpDir, "resp")

    oldTier := currentTier
    currentTier = func() auth.Tier { return auth.TierProfessional }
    t.Cleanup(func() { currentTier = oldTier })

    _, _, exitCode := captureRunCmd(t, col, "--parallel", "--format", "markdown", "--report", reportDir)
    if exitCode != 0 {
        t.Fatalf("exit code = %d", exitCode)
    }
    runMD, _ := os.ReadFile(filepath.Join(reportDir, "run.md"))
    if !strings.Contains(string(runMD), "## Wave 0") {
        t.Errorf("expected ## Wave 0 in run.md:\n%s", runMD)
    }
    // wave_index in per-request files.
    aMD, _ := os.ReadFile(filepath.Join(reportDir, "a.md"))
    if !regexp.MustCompile(`wave_index: \d+`).Match(aMD) {
        t.Errorf("expected wave_index: <N> in a.md:\n%s", aMD)
    }
}
```

#### Impact on Existing Tests

- `TestRun_MarkdownFormat_HappyPath` — sequential collection; unchanged.
- `TestRun_MarkdownFormat_SpliceOnRerun` — sequential; unchanged.
- `TestRun_MarkdownFormat_NoSentinelWritesDotNew` — sequential;
  unchanged.
- `TestRun_MarkdownFormat_MalformedSentinelWritesDotNew` — sequential;
  unchanged.
- `TestRun_MarkdownFormat_RedactionInvariant` — sequential, single
  request; unchanged.
- All other `TestRunCmd_*` tests do not touch `--format markdown`;
  unchanged.

## Test Impact Summary

| Test File                                     | Test Function                                           | Impact   | Action Required                                            |
|-----------------------------------------------|----------------------------------------------------------|----------|------------------------------------------------------------|
| `internal/output/markdown/run_md_test.go`     | `TestMarkdown_RunMD_WaveGrouping`                       | NEW      | step 2                                                     |
| `internal/output/markdown/run_md_test.go`     | `TestMarkdown_RunMD_Sequential`                         | NEW      | step 2 (regression guard)                                  |
| `internal/output/markdown/run_md_test.go`     | `TestMarkdown_RunMD`, `TestMarkdown_RunMD_WithEnvName`  | none     | unchanged (IsParallel=false)                               |
| `internal/output/markdown/datadriven_test.go` | `TestMarkdown_IterationOutcomeCounts`                   | NEW      | step 3                                                     |
| `internal/output/markdown/datadriven_test.go` | `TestMarkdown_RenderIteration`                          | NEW      | step 3                                                     |
| `internal/output/markdown/datadriven_test.go` | `TestMarkdown_RenderIndex`                              | NEW      | step 3                                                     |
| `internal/output/markdown/datadriven_test.go` | `TestMarkdown_RenderIndex_Truncation`                   | NEW      | step 3                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_PerIteration`                  | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_IndexSummary`                  | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_CorrelationIDs`                | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_Splice`                        | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_IterCap`                       | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_DataDriven_FailedIteration`               | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_ParallelWaves`                            | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_SequentialWaveIndex`                      | NEW      | step 4                                                     |
| `internal/output/markdown/formatter_test.go`  | `TestMarkdown_Render_PassJSON` and other M9-003 tests   | none     | use empty `Iterations`; single-file path unchanged         |
| `cmd/apitest/run_test.go`                     | `TestRun_MarkdownFormat_DataDriven`                     | NEW      | step 5                                                     |
| `cmd/apitest/run_test.go`                     | `TestRun_MarkdownFormat_ParallelWaves`                  | NEW      | step 5                                                     |
| `cmd/apitest/run_test.go`                     | `TestRun_MarkdownFormat_HappyPath` (and other existing) | none     | sequential single-request; unchanged                       |

Total: **13 new test functions** across 3 test files.

## Risks and Edge Cases

- **Risk:** `executeDataDrivenParallel` may emit iterations to `results`
  in non-monotonic index order (worker pool concurrency).
  **Mitigation:** Read `runner.go:2462` (`allIterResults`) — the worker
  pool sorts by index before appending to results. Verified by reading
  the post-loop sort step. The cmd-builder's `if rr.IterationIndex !=
  expectedNext { ... }` is unnecessary; we trust the runner's
  ordering. A defensive `sort.SliceStable(iters, byIndex)` after the
  group is collected protects against future regression at zero
  performance cost (5–1000 elements typical).

- **Risk:** Re-running a collection after a request is renamed
  (`Seed users` → `Generate users`) creates an orphan
  `seed-users/` directory alongside a fresh `generate-users/`. The
  splice machinery only handles single-file orphans.
  **Mitigation:** Out of scope. The existing M9-002 single-request
  orphan path emits a stderr warning and a `.md.new` file; the
  data-driven case can leave an orphan directory which is
  user-recoverable. Document in `iter` rendering doc-comment.

- **Risk:** A user-authored `iter-3.md` outside the sentinel region
  (e.g. embedded fenced block containing `<!-- BEGIN apitest:response`)
  could confuse the sentinel parser.
  **Mitigation:** Same as M9-002. The existing `parseSentinels` regex
  requires the line *start* to match (anchored `^`), so fenced
  content does not collide.

- **Risk:** Iteration count = 0 (data-driven file empty after header).
  **Mitigation:** `makeDataDrivenReport(0, 0)` produces an entry with
  empty `Iterations` slice. The cmd-builder treats this as "not data
  driven" because the loop never collects anything; the result is
  emitted as a single-file entry. We do not need to handle this in
  the markdown package because the runner emits zero results for an
  empty source — the loop simply doesn't see them. (If for some
  reason `IterationTotal > 0 && len(Iterations) == 0` flowed in, the
  index.md would render an empty table, which is acceptable.) Tested
  by `TestMarkdown_DataDriven_IterCap` indirectly (the truncation
  path).

- **Risk:** `parser.Slug(DataDrivenName)` returns an empty string for
  a name that contains only punctuation.
  **Mitigation:** The runner's parser already rejects empty-slug
  names at load time (`parser.Slug` returns `ErrSlugEmpty`); the
  markdown layer never sees a DD result with an empty
  `DataDrivenName` because the collection wouldn't have loaded.
  Defensive check in cmd-builder: if `parser.Slug(groupName)` errs,
  fall back to `slug-empty-<index>` and emit a stderr warning. (Test
  not required because parser guards this; the fallback is
  belt-and-suspenders.)

- **Risk:** `summary.IsParallel == true` but no entry has
  `WaveIndex >= 0` (e.g. parallel mode with all data-driven items).
  **Mitigation:** `renderRunMDByWave` falls through to the sequential
  group: `## Sequential` is emitted with the DD aggregates. No
  `## Wave N` headers. Tested by a parallel-DD-only scenario in
  `TestMarkdown_RunMD_ParallelDataDriven` (added under step 2's
  expansion if time permits; otherwise covered by the integration
  test at step 5).

- **Risk:** `iter-N.md` filename collides with a hand-written file
  named `iter-3.md` that happens to live in the report dir.
  **Mitigation:** The slug subdir scoping (`<slug>/iter-N.md`) makes
  collision practically impossible — a user would have to manually
  create the same path. The splice path (existing) handles this
  gracefully: existing file with no sentinel → `.md.new`; existing
  file with sentinel → splice (likely a re-run, which is the desired
  behaviour).

- **Edge case:** `IterationTotal == len(Iterations)` (no truncation).
  **Handling:** `renderIndex` skips the `_truncated_` row. Tested
  implicitly by `TestMarkdown_DataDriven_IndexSummary` (5 of 5).

- **Edge case:** Single iteration (`len(Iterations) == 1`).
  **Handling:** Index.md renders one row in the table; iter-0.md
  renders normally. Run.md aggregate bullet says "1 iterations"
  (grammar deliberately not pluralised — keeping the format
  uniform).

- **Edge case:** Iteration with `r.Result == nil` (network error
  before HTTP).
  **Handling:** `IterationEntry` fields default to zero values
  (StatusCode 0, empty body); `iterationStatus` returns "fail"
  because `Err != nil`. `renderResponse` with empty body renders
  `_(empty body)_` which is acceptable for an error case.

- **Edge case:** A skipped iteration (`r.Skipped == true`,
  e.g. context cancelled mid-run).
  **Handling:** `iterationStatus` returns "skip"; renders the
  iteration's section structure with "_(empty body)_" because no
  response. Counted in `skipped` aggregate.

## Verification

```bash
go build ./cmd/apitest
go test ./internal/output/markdown/... ./cmd/apitest/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# Parallel: Timing section has wave_index for every parallel request.
./apitest run collections/parallel.yaml --format markdown --report /tmp/resp
grep '^wave_index: ' /tmp/resp/*.md | awk -F': ' '{print $NF}' | sort -u
# Expected: multiple distinct wave values (e.g., 0, 1, 2).

# Sequential requests still render `wave_index: sequential`.
./apitest run collections/sequential.yaml --format markdown --report /tmp/resp
grep '^wave_index: sequential' /tmp/resp/*.md
# Expected: at least one match.

# run.md groups entries by wave header.
grep -c '^## Wave ' /tmp/resp/run.md
# Expected: number equal to distinct wave count for the collection.

# Data-driven: one .md per iteration in a slug subdirectory + index.md.
./apitest run collections/datadriven.yaml --format markdown --report /tmp/resp
ls /tmp/resp/seed-users/
# Expected: index.md iter-0.md iter-1.md iter-2.md iter-3.md iter-4.md

# Iteration index summarizes counts and links to iter files.
grep -c '\(iter-[0-9]*\.md\)' /tmp/resp/seed-users/index.md
# Expected: 5 links (one per iteration).

# Iteration correlation: request_id req-N-iter-M in sentinel.
grep -Eo 'id=req-[0-9]+-iter-[0-9]+ slug=seed-users' /tmp/resp/seed-users/iter-0.md
# Expected: one match with iter-0.

# run.md entry for data-driven request shows aggregate counts.
grep 'seed-users' /tmp/resp/run.md
# Expected: bullet link like "- [seed-users](seed-users/index.md) (5 iterations, 4 passed, 1 failed)"

# Splice preserves agent notes in iter files across re-runs.
printf '\nAGENT ANALYSIS of iter-3\n' >> /tmp/resp/seed-users/iter-3.md
./apitest run collections/datadriven.yaml --format markdown --report /tmp/resp
grep 'AGENT ANALYSIS' /tmp/resp/seed-users/iter-3.md
# Expected: present.

# Iteration cap: source exceeding runner row cap truncates index table.
./apitest run collections/datadriven-huge.yaml --format markdown --report /tmp/resp
grep 'truncated' /tmp/resp/seed-users-huge/index.md
# Expected: truncation marker present; iter files beyond cap not written.

# Full unit + integration suite.
go test -run 'TestMarkdown_ParallelWaves|TestMarkdown_RunMD_WaveGrouping|TestMarkdown_DataDriven_PerIteration|TestMarkdown_DataDriven_IndexSummary|TestMarkdown_DataDriven_CorrelationIDs|TestMarkdown_DataDriven_Splice|TestMarkdown_DataDriven_IterCap|TestMarkdown_DataDriven_FailedIteration' ./...
# Expected: PASS.
```

The fixture collections referenced in the observable
(`collections/parallel.yaml`, `collections/sequential.yaml`,
`collections/datadriven.yaml`, `collections/datadriven-huge.yaml`) do
not need to be checked into the repo — the integration tests at step 5
construct equivalent collections inline via `httptest.NewServer` and
ad-hoc YAML, mirroring the M9-002 test pattern. The observable
verification block in this plan is provided for manual smoke
verification by the developer once the implementation lands.
