# Implementation Plan: M9-002

## Overview

Ship the foundation slice of the markdown response format. `--format markdown
--report <dir>` writes one `<slug>.md` per main-phase request plus a `run.md`
index, each with a sentinel-delimited deterministic block carrying `run_id`,
`request_id`, and `request_slug`. JSON response bodies are pretty-printed via
`json.Indent`; non-JSON content-type rendering is deferred to M9-003. All file
writes are atomic (O_EXCL temp-file + rename); splice rules preserve
agent-authored content above and below the sentinels and never overwrite an
existing file that lacks the expected sentinel pair.

## Task Details

- **ID:** M9-002
- **Title:** markdown formatter: --format markdown with sentinel splice, JSON body, run.md index
- **Phase:** M9: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** medium
- **Estimated effort:** 1–2 days

## Dependencies

| Task   | Title                                                                           | Status |
|--------|---------------------------------------------------------------------------------|--------|
| M9-001 | request_slug: derive per-request identifier and emit in events schema v1.2      | done   |

## Architectural Decisions Resolved Up Front

These decisions are informed by reading every file the slice will touch; they
are recorded here so reviewers do not have to reverse-engineer them.

1. **Markdown formatter package: `internal/output/markdown/`.** New package
   matching the task scope. Mirrors the layout of `internal/output/events/`
   (one package per non-trivial output format). Exposes the minimum surface
   the cmd layer needs: a `Report` struct, a `WriteReport(report *Report, dir
   string) error` entry point, and the sentinel constants used by tests.

2. **Per-request and run-level correlation IDs are populated on every result.**
   The sentinel opening tag carries three IDs: `id=<request_id>`,
   `slug=<request_slug>`, `run=<run_id>`. Today `request_id` is minted only
   when `--events` is enabled (`runner.go:1710-1711`) and `run_id` lives only
   inside `events.Emitter`. To make markdown correlate with the event stream
   even when `--events` is not used, **the runner mints `request_id` and
   `run_id` unconditionally** in this slice. Concretely:

   - `runner.RequestResult` gains two additive fields: `RequestID string` and
     `RequestSlug string`. Existing literal sites in `internal/runner/runner.go`
     copy `reqID` and `item.Slug` (or `iterReqID` and `iterSlug`) into them.
   - `runner.Summary` gains an additive `RunID string` field, set once at the
     top of `Run`. The cmd layer reads it for sentinel rendering; events
     emitter reuses it via `events.Options{RunID: summary.RunID}` if
     `--events` is enabled (drop-in: `Options.RunID` already exists).
   - The runner drops the `if vars.OnEvent != nil` guard around
     `vars.nextRequestID()` so reqIDs are minted in execution order regardless
     of events. Cost: one atomic increment per request — negligible.
   - `vars.RunID` is added to `VarSources` for test injection
     (deterministic-mode tests pin `run_id` to make goldens stable). Empty
     string means "generate via crypto/rand", matching `events.newRunID`.

3. **Run-level `runID` is generated once in `cmd/apitest/runCmdInner` and
   threaded through both events and markdown.** A new helper
   `newRunID()` in `main.go` wraps the same crypto/rand-based pattern as
   `events.newRunID` (32-char lowercase hex). The helper is shared via
   `vars.RunID` so that **both** the events emitter and the markdown
   formatter see the same `run_id`. When `--events` and `--format markdown`
   are used together, the same hex string appears in event headers and the
   markdown sentinels — by construction.

4. **Sentinel format is exact and parser-stable.** Opening:
   `<!-- BEGIN apitest:response id=<request_id> slug=<request_slug> run=<run_id> -->`.
   Closing: `<!-- END apitest:response id=<request_id> slug=<request_slug> run=<run_id> -->`.
   Both lines are LF-terminated. The parser anchors on the literal prefix
   `<!-- BEGIN apitest:response ` / `<!-- END apitest:response ` — the
   whitespace and attribute order are part of the contract. Test
   `TestMarkdown_SentinelExactFormat` pins this with a byte-for-byte regex
   match against `^<!-- BEGIN apitest:response id=req-\d+ slug=[a-z0-9-]+ run=[0-9a-f]{32} -->$`.

5. **Splice decision matrix lives in `splice.go` and is exhaustively tested.**
   Six cases enumerated in the task behaviors:

   | Existing file state                                       | Action                                              |
   |-----------------------------------------------------------|-----------------------------------------------------|
   | File does not exist                                       | atomic write of full markdown (sentinel block only) |
   | BEGIN/END both present, slug matches current request      | rewrite bytes between sentinels; preserve outside   |
   | BEGIN/END both present, slug differs (request renamed)    | append new sentinel block below; orphan preserved + stderr warning |
   | BEGIN without END (or END without BEGIN, or nested)       | write `<slug>.md.new`; original untouched + stderr warning |
   | No sentinels at all                                       | write `<slug>.md.new`; original untouched + stderr warning |
   | BEGIN/END match slug+request_id, run= differs             | splice (run= is volatile, not part of identity)     |

6. **Atomic write via O_EXCL temp-file + rename.** Pattern matches
   `internal/auth/cache.go:80-109`. Temp file is created in the same
   directory (so rename is POSIX atomic on the same filesystem). Suffix is
   `.<slug>.<random>.tmp` (per-request scope so two concurrent writers to
   the same `--report` dir cannot collide on the temp filename). Rename is
   `os.Rename`.

7. **`run.md` is treated like any other markdown file.** Same sentinel
   discipline (BEGIN/END with `id=run` reserved sentinel slug, plus
   `run=<run_id>`), same atomic write, same splice rules. The body inside
   the sentinels is: H1 collection name, summary table (total/pass/fail/skip),
   environment name, ISO-8601 timestamp, and a bullet list linking each
   per-request `.md` file in execution order.

8. **`ensureReportDir(path string, subpath ...string) error`** is added in
   `writer.go` with the exact signature the task scope mandates. The
   variadic `subpath` is unused in M9-002 but reserved so M9-004 can call
   `ensureReportDir(report, slug, "iter-3.md")` without changing the
   signature.

9. **Schema enum bump applies to both schemas.** `schemas/collection-v1.json`
   and `schemas/project-v1.json` add `"markdown"` to the `output.format`
   enum. Two existing tests assume `"markdown"` is invalid and must be
   updated to use a different invalid value:

   - `TestConfig_Validate.invalid_format` (`internal/output/config_test.go:42`)
     swaps `"markdown"` → `"yaml"` (still invalid, still triggers
     `ErrUnknownFormat`).
   - `TestSchema_rejects_unknown_output_format`
     (`internal/schema/validate_coverage_test.go:168-181`) swaps
     `"format": "markdown"` → `"format": "yaml"`.

10. **CLI dispatch lives at the existing format switch ladder in `runCmdInner`.**
    Add `format == "markdown"` between the `html` branch (line 1546) and the
    terminal fallback (line 1588). Mirror the html block for: report-path
    requirement (markdown requires `--report` directory at load time), exit
    codes for guard-rail/varErr/assertion-failed/main-failed, and the
    feature-gate path is **omitted** (markdown is not gated; spec & task
    say nothing about gating).

11. **Markdown requires `--report` at load time, exit code 3.** Mirror of
    html's behaviour, but with exit code **3** (config error) per the task
    YAML observable rather than html's exit code 1. Two enforcement points,
    matching the html pattern: an early CLI-set check after gate validation
    (~line 663), and a YAML-resolved check after `resolveOutputPrecedence`
    (~line 1009-1042). Both emit a structured error
    `format: markdown requires --report <dir>` to stderr.

12. **Atomic-write concurrency test uses goroutines, not sub-processes.**
    `TestMarkdown_Concurrent` spins two goroutines that each call
    `WriteReport` on the same `<report>` directory with the same slug.
    Assertion: after both return, the file contains a complete sentinel
    pair (one BEGIN, one END) and no half-written bytes.

13. **Determinism regression test scope.** `TestMarkdown_DeterminismRegression`
    runs `WriteReport` twice against an identical `Report` value and
    compares the resulting bytes. Volatile lines (`duration_ms:`,
    `wave_index:`, `started_at:`) are masked via line-prefix substitution
    before the comparison. The second-run bytes minus the volatile lines
    must equal the first-run bytes minus the volatile lines.

14. **Newline normalization scope.** Inside the sentinel block: LF line
    endings only (the formatter writes `\n`, never `\r\n`). Outside the
    sentinel block (preserved on splice): the original byte sequence is
    preserved verbatim, including any CRLF endings. Tests use a CRLF
    fixture above the BEGIN line and assert the CRLF survives.

15. **Existing event-driven tests are not affected.** The runner change
    (always mint reqID) does not alter the events stream byte-for-byte
    because the event emission gate (`if vars.OnEvent != nil`) only wraps
    the `vars.OnEvent.RequestStart(...)` call; the `reqID = vars.nextRequestID()`
    line moves out of that gate. The events emitter still receives the
    same `req-1, req-2, ...` sequence in the same order. Schema v1.2
    goldens remain byte-identical.

16. **Watch-mode and discovery flows are unaffected.** The markdown
    formatter is invoked via the same `runCmdInner` switch ladder that
    `runDiscoveredCollections` (glob mode) and watch mode call into;
    no separate dispatch is needed.

## Implementation Steps

Steps are ordered by blast radius, smallest first.

### Step 1: Plumb RequestID, RequestSlug, RunID into runner result types

**Rationale:** Smallest blast radius — additive struct fields with no
behavioural change. Failing tests in this step would block every later
step, so land it first and let `go test ./...` confirm runner tests still
pass before touching the formatter.

#### Files to Modify

| File                                  | Action  | Description                                                                          |
|---------------------------------------|---------|--------------------------------------------------------------------------------------|
| `internal/runner/runner.go`           | modify  | Add `RequestID`, `RequestSlug` to `RequestResult`. Add `RunID` to `Summary`. Add `RunID` to `VarSources`. Drop `if vars.OnEvent != nil` guard around `vars.nextRequestID()`. Mint runID in `Run()`. Populate `RequestID`/`RequestSlug` on every `RequestResult{...}` literal. |
| `internal/runner/runner_test.go`      | modify  | Add `TestRunner_AlwaysMintsRequestIDs` and `TestRunner_GeneratesRunID`. Existing tests pass unmodified (additive fields default to "" but are now populated). |

#### Current Code (runner.go:140-219)

```go
// RequestEvent describes a request lifecycle start for event emission.
type RequestEvent struct {
    RequestID   string
    RequestSlug string // M9-001: derived from Name at parse time; pairs with RequestEndEvent
    Name        string
    // ...
}

// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
    Name             string
    Phase            Phase
    Method           string
    URL              string
    RequestHeaders   map[string]string
    RequestBody      any
    Result           *httpexec.Result
    Err              error
    Skipped          bool
    SkipReason       string
    AssertionResults *assertion.Results
    RetryCount       int
    RetryWarnings    []string
    AttemptDetails   []retry.AttemptDetail
    WaveIndex        int
    Warnings         []string
    IsDataDriven   bool
    DataDrivenName string
    IterationIndex int
    IterationTotal int
    IterationData  map[string]string
    SourceFile string
    SourceLine int
}

// Summary holds aggregate execution results.
type Summary struct {
    Total                   int
    Passed                  int
    Failed                  int
    // ...
}
```

#### New Code (runner.go)

```go
// RequestResult holds the outcome of a single request execution.
type RequestResult struct {
    // ... existing fields unchanged ...

    // RequestID is the per-run identifier minted by Run; matches the
    // request_id field in --events NDJSON when events are enabled.
    // Always populated for non-skipped requests; empty for context-cancelled
    // skips. M9-002.
    RequestID string

    // RequestSlug is the URL-safe slug (parser.Slug). For top-level items
    // this is item.Slug; for data-driven iterations this is the slug of
    // the iteration name (e.g. "create-user-2-3"). M9-002.
    RequestSlug string
}

// Summary holds aggregate execution results.
type Summary struct {
    // ... existing fields unchanged ...

    // RunID is the per-run identifier minted by Run. 32-char lowercase
    // hex. Stable across all phases. M9-002.
    RunID string
}

// VarSources gains:
type VarSources struct {
    // ... existing fields unchanged ...

    // RunID overrides the auto-generated run identifier. When empty,
    // Run mints one via crypto/rand. Used by deterministic tests to pin
    // run_id for byte-stable goldens. M9-002.
    RunID string
}
```

Inside `Run()`:

```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, vars VarSources) ([]RequestResult, *Summary, error) {
    // ... existing prologue (idc, scope, gate checks) ...

    // M9-002: mint run_id once. Empty vars.RunID generates a fresh value;
    // a non-empty value is honoured (deterministic tests).
    runID := vars.RunID
    if runID == "" {
        runID = newRunID()
    }
    // emptySummary already exists; populate RunID before any early-return.
    emptySummary.RunID = runID
    // ... and on the success-path Summary too (search for Summary literal) ...
}

// newRunID produces a 32-char lowercase hex identifier using crypto/rand.
// Mirrors events.newRunID so events and markdown sentinels carry the same
// identifier when both are active.
func newRunID() string {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil {
        return fmt.Sprintf("fallback-%016x", time.Now().UnixNano())
    }
    return hex.EncodeToString(b[:])
}
```

Inside the per-request loop (sequential main path, ~line 1709):

```go
// Always mint the request ID so it is available even without --events.
reqID := vars.nextRequestID()
if vars.OnEvent != nil {
    vars.OnEvent.RequestStart(RequestEvent{
        RequestID:   reqID,
        RequestSlug: item.Slug,
        // ...
    })
}
// Every RequestResult{...} literal in this scope gains:
rr := RequestResult{
    Name: item.Name, Phase: phase,
    RequestID:   reqID,        // M9-002
    RequestSlug: item.Slug,    // M9-002
    // ...
}
```

The same pattern applies to:
- WebSocket main path (~line 1597, `wsReqID`)
- Sequential main path (~line 1711, `reqID`)
- Data-driven sequential path (~line 2073, `iterReqID`)
- Data-driven parallel path (~line 2279, `iterReqID`)
- Setup and teardown loops (these reuse the same `reqID` mint pattern)
- Skipped-request literals (~lines 1466, 1476, 1486, 1119, 1582): RequestID is left empty (skip-before-execute has no event), RequestSlug = item.Slug.

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go

// TestRunner_AlwaysMintsRequestIDs verifies that every non-skipped
// RequestResult carries a non-empty RequestID, even when no EventSink is
// attached. Regression guard for the M9-002 splice contract: the markdown
// formatter relies on RequestID being populated regardless of --events.
func TestRunner_AlwaysMintsRequestIDs(t *testing.T) {
    // Two-request collection, no OnEvent.
    col := makeTestCollection(t, []requestSpec{
        {name: "First", method: "GET"},
        {name: "Second", method: "GET"},
    })
    results, summary, err := runner.Run(ctx, col, fakeExec, runner.VarSources{})
    // assert err == nil
    // assert len(results) == 2
    // assert results[0].RequestID == "req-1"
    // assert results[1].RequestID == "req-2"
    // assert summary.RunID is non-empty 32-char hex
}

// TestRunner_RunIDInjectable verifies that vars.RunID overrides the
// auto-generated run identifier. Deterministic tests rely on this.
func TestRunner_RunIDInjectable(t *testing.T) {
    col := makeTestCollection(t, []requestSpec{{name: "Only", method: "GET"}})
    _, summary, err := runner.Run(ctx, col, fakeExec, runner.VarSources{
        RunID: "deadbeef00000000deadbeef00000000",
    })
    // assert summary.RunID == "deadbeef00000000deadbeef00000000"
}

// TestRunner_RequestSlugCarriedToResult verifies that the slug derived at
// parse time on item.Slug appears on the RequestResult. Pairs with the M9-001
// guarantee that slugs are non-empty on parse.
func TestRunner_RequestSlugCarriedToResult(t *testing.T) {
    col := makeTestCollection(t, []requestSpec{
        {name: "Get user", method: "GET"},
    })
    results, _, _ := runner.Run(ctx, col, fakeExec, runner.VarSources{})
    // assert results[0].RequestSlug == "get-user"
}
```

#### Impact on Existing Tests

- `TestRunner_*` (existing): unaffected — additive struct fields default to
  zero value when not asserted on.
- `TestEvents_*` golden tests in `internal/output/events/schema_test.go`:
  unaffected — the `reqID` sequence (`req-1, req-2, ...`) is unchanged
  because the gate moved from "around mint" to "around emit"; mint order
  matches event order.
- `TestEvents_M9001_v12_validates`: unaffected — same as above.
- `internal/parallel/executor_test.go` tests: unaffected — parallel sink
  also receives the same reqID sequence, just always (not gated).

### Step 2: New package internal/output/markdown — formatter, splice, writer

**Rationale:** All new code; no caller depends on it yet. Land the package
with full unit + golden tests in isolation. Step 3 (CLI dispatch) is the
only step that imports it.

#### Files to Create

| File                                                    | Action | Description                                                                            |
|---------------------------------------------------------|--------|----------------------------------------------------------------------------------------|
| `internal/output/markdown/formatter.go`                 | create | `Report`, `RequestEntry`, `WriteReport(*Report, string) error`, sentinel constants, section renderers (`renderRequest`, `renderResponse`, `renderTiming`, `renderAssertions`). |
| `internal/output/markdown/splice.go`                    | create | `parseSentinels([]byte) (sentinelMatch, error)`, `decideAction(existing []byte, slug, runID, reqID string) action`, the action enum (`actionFreshWrite`, `actionSplice`, `actionAppendOrphan`, `actionDotNew`). |
| `internal/output/markdown/writer.go`                    | create | `ensureReportDir(path string, subpath ...string) error`, `writeAtomic(path string, data []byte) error`. |
| `internal/output/markdown/run_md.go`                    | create | `renderRunMD(report *Report) []byte` — the index file body with summary + bullet list of links. |
| `internal/output/markdown/formatter_test.go`            | create | `TestMarkdown_Render_*`, `TestMarkdown_EmptyAssertions`, `TestMarkdown_Newlines`. |
| `internal/output/markdown/splice_test.go`               | create | `TestMarkdown_Splice_*` (six cases from decision matrix), `TestMarkdown_SentinelExactFormat`. |
| `internal/output/markdown/writer_test.go`               | create | `TestMarkdown_EnsureReportDir`, `TestMarkdown_Concurrent`. |
| `internal/output/markdown/run_md_test.go`               | create | `TestMarkdown_RunMD`. |
| `internal/output/markdown/golden_test.go`               | create | `compareOrUpdateGolden` helper copied verbatim from `events/schema_test.go:323-358`. |
| `internal/output/markdown/regression_test.go`           | create | `TestMarkdown_DeterminismRegression`. |
| `internal/output/markdown/testdata/golden/*.md`         | create | Hand-authored goldens: `pass_json.md`, `fail_json.md`, `empty_assertions.md`, `splice_match_rewrite.md`, `splice_append_orphan.md`, `splice_dot_new.md`, `crlf_outside_sentinels.md`, `run_md.md`. |

#### Sentinel Constants

```go
// internal/output/markdown/formatter.go
package markdown

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "regexp"
    "strings"
    "time"

    "github.com/peterlindqvist/apitest/internal/assertion"
    "github.com/peterlindqvist/apitest/internal/httpexec"
)

// SentinelBeginPrefix is the literal prefix of the BEGIN sentinel line.
// The full line carries id, slug, run attributes after this prefix.
const SentinelBeginPrefix = "<!-- BEGIN apitest:response "

// SentinelEndPrefix is the literal prefix of the END sentinel line.
const SentinelEndPrefix = "<!-- END apitest:response "

// SentinelSuffix terminates both BEGIN and END sentinel lines.
const SentinelSuffix = " -->"

// RunMDSentinelSlug is the reserved slug for the run.md index file.
const RunMDSentinelSlug = "run"

// sentinelLineRE matches a complete sentinel line (BEGIN or END).
// Captures: 1=marker (BEGIN|END), 2=request_id, 3=slug, 4=run_id.
var sentinelLineRE = regexp.MustCompile(
    `^<!-- (BEGIN|END) apitest:response id=([^ ]+) slug=([a-z0-9-]+) run=([0-9a-f]{32}|run|[a-z0-9-]+) -->$`,
)
```

#### Report Type

```go
// Report is the complete dataset for one markdown render. The cmd/apitest
// builder converts []runner.RequestResult into a *Report; the markdown
// package owns no runner types directly so the dependency graph stays
// internal/output/* -> internal/runner (one-way only at the cmd layer).
type Report struct {
    CollectionName string
    EnvName        string         // optional; empty when --env not passed
    RunID          string         // 32-char hex
    StartedAt      time.Time      // RFC3339Nano in output
    Summary        SummaryCounts
    Requests       []RequestEntry // main-phase only; setup/teardown excluded for M9-002
}

type SummaryCounts struct {
    Total   int
    Passed  int
    Failed  int
    Skipped int
}

type RequestEntry struct {
    RequestID   string // matches events stream
    Slug        string // matches filename: <slug>.md
    Name        string // human-readable name
    Method      string
    URL         string
    StatusCode  int    // 0 if no response
    DurationMs  int64
    WaveIndex   int    // -1 when sequential
    StartedAt   time.Time
    RequestHdr  map[string]string
    RequestBody any
    RespHeaders map[string][]string  // http.Header form
    RespBody    []byte
    Assertions  *assertion.Results // nil when not evaluated
    Err         error
    Skipped     bool
    SkipReason  string
}
```

#### WriteReport Entry Point

```go
// WriteReport renders a Report into <dir>/run.md and one <dir>/<slug>.md per
// main-phase request. Splices into existing files when the BEGIN/END
// sentinels match the current slug. Writes <slug>.md.new (and emits a
// stderr warning via the io.Writer in opts) when the existing file lacks
// the expected sentinel pair.
//
// The dir must exist; callers use ensureReportDir to create it.
func WriteReport(report *Report, dir string, opts WriteOptions) error {
    // 1. Render run.md
    // 2. Render each per-request .md
    // 3. For each, decide splice action against any existing file
    // 4. Write atomically via writeAtomic
}

type WriteOptions struct {
    // Stderr receives orphan-slug warnings and dot-new warnings. Defaults
    // to io.Discard when nil.
    Stderr io.Writer
}
```

#### Section Renderers (formatter.go)

```go
// renderSentinelBlock emits the bytes between (and including) the BEGIN/END
// sentinel lines for one request entry. This is the only block the splice
// path overwrites; surrounding content is preserved.
//
// Section order (10 markers per task observable):
//   1. # <name>
//   2. ## Notes
//   3. (empty notes paragraph - placeholder for agent text)
//   4. <!-- BEGIN apitest:response id=... slug=... run=... -->
//   5. ## Response (deterministic)
//   6. ### Request
//   7. ### Response <status>
//   8. ### Timing
//   9. ### Assertions
//  10. <!-- END apitest:response id=... slug=... run=... -->
//  + ## Analysis (below the END sentinel; agent-owned)
func renderSentinelBlock(entry *RequestEntry, runID string) []byte { ... }

func renderRequest(w *bytes.Buffer, entry *RequestEntry) { ... }
func renderResponse(w *bytes.Buffer, entry *RequestEntry) { ... }
func renderTiming(w *bytes.Buffer, entry *RequestEntry) {
    // Three lines, fixed prefixes:
    fmt.Fprintf(w, "duration_ms: %d\n", entry.DurationMs)
    if entry.WaveIndex < 0 {
        fmt.Fprintln(w, "wave_index: sequential")
    } else {
        fmt.Fprintf(w, "wave_index: %d\n", entry.WaveIndex)
    }
    fmt.Fprintf(w, "started_at: %s\n", entry.StartedAt.UTC().Format(time.RFC3339Nano))
}
func renderAssertions(w *bytes.Buffer, entry *RequestEntry) {
    if entry.Assertions == nil || len(entry.Assertions.Items) == 0 {
        fmt.Fprintln(w, "_No assertions declared._")
        return
    }
    for _, it := range entry.Assertions.Items {
        marker := "[x]"
        if !it.Passed { marker = "[ ]" }
        fmt.Fprintf(w, "- %s %s expected=%q actual=%q\n", marker, it.Type, it.Expected, it.Actual)
    }
}
```

#### renderResponse: JSON Body Path (M9-002 only)

```go
func renderResponse(w *bytes.Buffer, entry *RequestEntry) {
    // status line
    fmt.Fprintf(w, "Status: %d\n\n", entry.StatusCode)

    // body
    ct := lookupHeader(entry.RespHeaders, "Content-Type")
    if isJSONContentType(ct) {
        var pretty bytes.Buffer
        if err := json.Indent(&pretty, entry.RespBody, "", "  "); err == nil {
            fmt.Fprintln(w, "```json")
            w.Write(pretty.Bytes())
            if pretty.Len() > 0 && pretty.Bytes()[pretty.Len()-1] != '\n' {
                fmt.Fprintln(w)
            }
            fmt.Fprintln(w, "```")
            return
        }
        // Fall through on JSON parse failure: render as raw text fence.
    }
    // Non-JSON path is deferred to M9-003. For M9-002 we render as a raw
    // text fence so observable output still has *something* in the slot.
    fmt.Fprintln(w, "```")
    w.Write(entry.RespBody)
    if len(entry.RespBody) > 0 && entry.RespBody[len(entry.RespBody)-1] != '\n' {
        fmt.Fprintln(w)
    }
    fmt.Fprintln(w, "```")
}

func isJSONContentType(ct string) bool {
    // Lowercased, parameter-stripped match for application/json or */*+json.
    // Reuses pattern from internal/output/events when M9-003 lands; here we
    // implement the minimum the foundation slice needs.
    if ct == "" { return false }
    media := strings.ToLower(strings.SplitN(ct, ";", 2)[0])
    media = strings.TrimSpace(media)
    return media == "application/json" || strings.HasSuffix(media, "+json")
}
```

#### Splice Logic (splice.go)

```go
package markdown

type spliceAction int

const (
    actionFreshWrite   spliceAction = iota // file does not exist
    actionSplice                            // BEGIN/END match slug; rewrite between
    actionAppendOrphan                      // BEGIN/END match but slug differs; append below
    actionDotNew                            // malformed or no sentinels; write .md.new
)

type sentinelLocation struct {
    BeginLine int    // 0-based
    EndLine   int
    Begin     sentinelAttrs
    End       sentinelAttrs
}

type sentinelAttrs struct {
    RequestID string
    Slug      string
    RunID     string
}

// parseSentinels scans the existing file bytes and returns the locations of
// the first BEGIN/END pair. Returns:
//   - (loc, nil) when both sentinels are present and well-formed.
//   - (nil, errMalformed) when BEGIN exists without END, or END without
//     BEGIN, or BEGIN/END are nested.
//   - (nil, nil) when no sentinels are present.
func parseSentinels(existing []byte) (*sentinelLocation, error) { ... }

// decideAction inspects the existing file and the current write to choose
// one of the four actions.
func decideAction(existing []byte, currentSlug string) (spliceAction, *sentinelLocation, error) { ... }

var errMalformedSentinel = errors.New("markdown: malformed sentinel pair")
```

#### Writer (writer.go)

```go
package markdown

import (
    "fmt"
    "os"
    "path/filepath"
)

// ensureReportDir creates the target directory via os.MkdirAll(0755).
// The variadic subpath argument is reserved for M9-004 (data-driven
// per-iteration files) and currently unused; passing it has no effect.
func ensureReportDir(path string, subpath ...string) error {
    full := path
    if len(subpath) > 0 {
        full = filepath.Join(append([]string{path}, subpath...)...)
    }
    if err := os.MkdirAll(full, 0o755); err != nil {
        return fmt.Errorf("create report directory %q: %w", full, err)
    }
    return nil
}

// writeAtomic writes data to path using O_EXCL temp-file + rename.
// Two concurrent invocations on the same path produce one valid file
// (the loser's bytes are discarded by os.Rename's atomic replace).
func writeAtomic(path string, data []byte) error {
    dir := filepath.Dir(path)
    f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
    if err != nil {
        return fmt.Errorf("create temp file in %q: %w", dir, err)
    }
    tmpPath := f.Name()
    if _, err := f.Write(data); err != nil {
        _ = f.Close()
        _ = os.Remove(tmpPath)
        return fmt.Errorf("write temp file %q: %w", tmpPath, err)
    }
    if err := f.Close(); err != nil {
        _ = os.Remove(tmpPath)
        return fmt.Errorf("close temp file %q: %w", tmpPath, err)
    }
    if err := os.Rename(tmpPath, path); err != nil {
        _ = os.Remove(tmpPath)
        return fmt.Errorf("rename temp file %q -> %q: %w", tmpPath, path, err)
    }
    return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
// formatter_test.go

func TestMarkdown_Render_PassJSON(t *testing.T) {
    report := &Report{
        CollectionName: "Sample API",
        RunID:          "0123456789abcdef0123456789abcdef",
        StartedAt:      time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
        Summary:        SummaryCounts{Total: 1, Passed: 1},
        Requests: []RequestEntry{{
            RequestID:   "req-1",
            Slug:        "get-user",
            Name:        "Get user",
            Method:      "GET",
            URL:         "https://api.example.com/users/1",
            StatusCode:  200,
            DurationMs:  42,
            WaveIndex:   -1,
            StartedAt:   time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
            RespHeaders: http.Header{"Content-Type": {"application/json"}},
            RespBody:    []byte(`{"id":1,"name":"Alice"}`),
            Assertions:  &assertion.Results{Passed: true, Items: []assertion.Result{
                {Type: "status", Expected: "200", Actual: "200", Passed: true},
            }},
        }},
    }
    dir := t.TempDir()
    if err := WriteReport(report, dir, WriteOptions{}); err != nil { t.Fatal(err) }

    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    compareOrUpdateGolden(t, "pass_json.md", got)
}

func TestMarkdown_Render_FailJSON(t *testing.T) { /* assertion fails; checklist shows [ ] */ }

func TestMarkdown_EmptyAssertions(t *testing.T) {
    // Assertions = &Results{Items: nil}  →  body is _No assertions declared._
}

func TestMarkdown_Newlines(t *testing.T) {
    // Existing file with CRLF above BEGIN; CRLF must survive.
}

func TestMarkdown_SentinelExactFormat(t *testing.T) {
    // Regex-match the BEGIN line bytes against:
    // ^<!-- BEGIN apitest:response id=req-1 slug=get-user run=[0-9a-f]{32} -->$
}

// splice_test.go

func TestMarkdown_Splice_MatchRewritesRegion(t *testing.T) {
    // Pre-create get-user.md with:
    //   # Get user\n## Notes\nAGENT_ABOVE\n<!-- BEGIN ... -->\nOLD_BLOCK\n<!-- END ... -->\nAGENT_BELOW\n
    // After WriteReport, AGENT_ABOVE and AGENT_BELOW preserved verbatim;
    // OLD_BLOCK replaced with new sentinel content.
}

func TestMarkdown_Splice_RenameAppends(t *testing.T) {
    // Existing file has BEGIN/END with slug=old-slug; current slug=new-slug.
    // Action: append new sentinel block after old; orphan preserved.
    // Stderr captured contains warning message naming "old-slug".
}

func TestMarkdown_Splice_MalformedWritesDotNew(t *testing.T) {
    // Existing file has BEGIN without END.
    // Action: write get-user.md.new; original untouched.
    // Stderr contains "malformed sentinel".
}

func TestMarkdown_Splice_NoSentinelWritesDotNew(t *testing.T) {
    // Existing file has no sentinels.
    // Action: write get-user.md.new; original untouched.
    // Stderr contains "no sentinel pair".
}

func TestMarkdown_Splice_MismatchedRunStillSplices(t *testing.T) {
    // Existing BEGIN/END have slug=get-user run=AAAAAAAA...; current run=BBBBBBBB...
    // Action: splice (run= is volatile, slug+id is identity).
}

func TestMarkdown_Splice_FreshWrite(t *testing.T) {
    // No existing file. Action: full atomic write.
}

// writer_test.go

func TestMarkdown_EnsureReportDir(t *testing.T) {
    // Absolute and relative paths; subpath variadic; MkdirAll mode 0755.
}

func TestMarkdown_Concurrent(t *testing.T) {
    // Two goroutines each call WriteReport with same dir+slug.
    // Both return nil; resulting file has exactly one BEGIN and one END;
    // contents are byte-identical to a single-write baseline.
}

// run_md_test.go

func TestMarkdown_RunMD(t *testing.T) {
    // Three main requests → run.md has three bullet links, each pointing
    // to <slug>.md, in execution order.
}

// regression_test.go

func TestMarkdown_DeterminismRegression(t *testing.T) {
    // Build identical Report value; render twice; mask volatile lines;
    // assert byte-equal after masking.
}
```

#### Impact on Existing Tests

- None — this is a brand-new package.

### Step 3: Schema and config — add markdown to enums

**Rationale:** Pure data file changes. Independent of the formatter and the
runner; lands cleanly between them. Two existing tests assume markdown is
invalid and need to swap to a different invalid value.

#### Files to Modify

| File                                                | Action | Description                                                                            |
|-----------------------------------------------------|--------|----------------------------------------------------------------------------------------|
| `internal/output/config.go`                         | modify | Line 15: add `"markdown"` to `SupportedFormats`. Drop the `; excludes markdown per scope` clause from the comment. |
| `schemas/collection-v1.json`                        | modify | Line 70: add `"markdown"` to the format enum. Trailing comma + element. |
| `schemas/project-v1.json`                           | modify | Line 22: add `"markdown"` to the format enum. |
| `internal/output/config_test.go`                    | modify | Line 42: change `{"invalid_format", &Config{Format: "markdown"}, ErrUnknownFormat}` to use `"yaml"` (still invalid). Add `{"valid_markdown", &Config{Format: "markdown"}, nil}`. |
| `internal/schema/validate_coverage_test.go`         | modify | Lines 167-181: rename `TestSchema_rejects_unknown_output_format` body to use `"yaml"` instead of `"markdown"`. Add `TestSchema_AcceptsMarkdownFormat` covering both schemas. |

#### Current Code

```go
// internal/output/config.go:13-15
// SupportedFormats lists the valid output.format values (matches the CLI
// --format enum as of M8-003; excludes markdown per scope).
var SupportedFormats = []string{"terminal", "json", "tap", "junit", "html"}
```

```json
// schemas/collection-v1.json:70
"format":    { "type": "string", "enum": ["terminal", "json", "tap", "junit", "html"] },

// schemas/project-v1.json:22
"format":    { "type": "string", "enum": ["terminal", "json", "tap", "junit", "html"] },
```

#### New Code

```go
// internal/output/config.go:13-15
// SupportedFormats lists the valid output.format values (matches the CLI
// --format enum).
var SupportedFormats = []string{"terminal", "json", "tap", "junit", "html", "markdown"}
```

```json
"format":    { "type": "string", "enum": ["terminal", "json", "tap", "junit", "html", "markdown"] },
```

#### Tests to Write FIRST (RED phase)

```go
// internal/output/config_test.go (replace existing case)
{"invalid_format", &Config{Format: "yaml"}, ErrUnknownFormat},
{"valid_markdown", &Config{Format: "markdown"}, nil},

// internal/schema/validate_coverage_test.go (new test)
func TestSchema_AcceptsMarkdownFormat(t *testing.T) {
    cases := []struct{ name, target string }{
        {"collection", "collection"},
        {"project", "project"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            doc := buildDocWithFormat(tc.target, "markdown")
            sch := compileCollectionSchema(t)
            if tc.target == "project" { sch = compileProjectSchema(t) }
            if err := sch.Validate(doc); err != nil {
                t.Fatalf("expected markdown to validate, got %v", err)
            }
        })
    }
}
```

#### Impact on Existing Tests

- `TestConfig_Validate.invalid_format`: was asserting `Format: "markdown"`
  fails — now markdown is valid, so the case is updated to `"yaml"`.
- `TestSchema_rejects_unknown_output_format`: was using `"markdown"` —
  now uses `"yaml"` for the same purpose.
- All other schema tests pass unchanged.

### Step 4: CLI dispatch — wire --format markdown into runCmdInner

**Rationale:** Final step; pulls every prior step together. Imports the
new `internal/output/markdown` package. Adds the `format == "markdown"`
case at the existing format switch ladder.

#### Files to Modify

| File                                          | Action | Description                                                                            |
|-----------------------------------------------|--------|----------------------------------------------------------------------------------------|
| `cmd/apitest/main.go`                         | modify | (a) Line 567 early format check: add `format != "markdown"` to the allowed list. (b) Generate `runID` once after parseRunArgs (always), store in `runID` local; pass via `events.Options{RunID: runID}` and `vars.RunID`. (c) After CLI-set gate (~line 663) and after YAML-resolved (~line 1042): add markdown-requires-report check returning exit 3. (d) Insert `format == "markdown"` switch case between html (line 1546) and terminal default (line 1588). The case calls `buildMarkdownReport(...)` and `markdown.WriteReport(...)`. |
| `cmd/apitest/main.go`                         | modify | Add `buildMarkdownReport` helper near `buildHTMLReport` (line 2250). Shape mirrors that function. |
| `cmd/apitest/run_test.go`                     | modify | Add `TestRun_MarkdownFormat_HappyPath`, `TestRun_MarkdownFormat_RequiresReport`, `TestRun_MarkdownFormat_SpliceOnRerun`. |
| `cmd/apitest/output_precedence_test.go`       | modify | Extend `TestOutputPrecedence` table with a row for `markdown` (CLI > collection > project). |
| `internal/output/markdown/integration_test.go`| create | Optional cross-package integration test exercising `runCmdInner` end-to-end; observable assertions match the task YAML's grep commands. (Could also live in `cmd/apitest/run_test.go` — final placement decided during execute.) |

#### New Code (cmd/apitest/main.go)

```go
// imports
import "github.com/peterlindqvist/apitest/internal/output/markdown"
```

Inside `runCmdInner`, after `parseRunArgs` succeeds:

```go
// M9-002: mint run_id once. Used by both events emitter (when enabled)
// and markdown formatter (when format=markdown).
runID := newRunID()
```

(Helper `newRunID()` is added in this file or imported from runner; the
canonical version lives in `internal/runner/runner.go` per Step 1.)

Replace the `events.NewEmitter(... Options{ApitestVersion: version})` call
to also pass `RunID: runID`:

```go
em, emErr := events.NewEmitter(evF, events.Options{
    ApitestVersion: version,
    RunID:          runID,
})
```

Update the early format check:

```go
if format != "" && format != "json" && format != "terminal" && format != "tap" &&
   format != "junit" && format != "html" && format != "markdown" {
    errOut := newStderrPrinterTo(stderr, noColor)
    errOut.StructuredError(fmt.Errorf("unknown output format %q (supported: terminal, json, tap, junit, html, markdown)", format))
    return 1, nil
}
```

Add markdown-requires-report check (CLI-set side, ~line 663, after html
gate check):

```go
if flags.formatSet && format == "markdown" {
    if report == "" {
        mdReportErr := fmt.Errorf("format: markdown requires --report <dir>")
        errOut := newStderrPrinterTo(stderr, noColor)
        errOut.StructuredError(mdReportErr)
        if eventsEmitter != nil {
            _ = eventsEmitter.EmitRunError(mdReportErr)
            evExitCode = 3
        }
        return 3, nil
    }
}
```

Add markdown-requires-report check (YAML-resolved side, after
`resolveOutputPrecedence` ~line 1042):

```go
if !flags.formatSet && format == "markdown" {
    if report == "" {
        mdReportErr := fmt.Errorf("format: markdown requires --report <dir>")
        errOut.StructuredError(mdReportErr)
        if eventsEmitter != nil {
            _ = eventsEmitter.EmitRunError(mdReportErr)
            evExitCode = 3
        }
        return 3, nil
    }
}
```

Pass `runID` through to runner via `vars.RunID`:

```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    // ... existing fields ...
    RunID: runID,
})
```

Add the dispatch case (between line 1545 html and line 1588 terminal default):

```go
if format == "markdown" {
    mdReport := buildMarkdownReport(col, envName, runID, results, summary)
    if err := markdown.EnsureReportDir(report); err != nil {
        errOut.StructuredError(fmt.Errorf("cannot create report directory: %w", err))
        evExitCode = 1
        return 1, summary
    }
    if err := markdown.WriteReport(mdReport, report, markdown.WriteOptions{Stderr: stderr}); err != nil {
        errOut.StructuredError(fmt.Errorf("cannot write markdown report: %w", err))
        evExitCode = 1
        return 1, summary
    }
    if summary != nil && summary.LimitExceeded {
        evExitCode = 2
        return 2, summary
    }
    if varErr != nil {
        if eventsEmitter != nil { _ = eventsEmitter.EmitRunError(varErr) }
        evExitCode = 5
        return 5, summary
    }
    if summary != nil {
        mainAssertionFailed := summary.AssertionFailures - summary.TeardownAssertionErrors
        mainFailed := summary.Failed - summary.TeardownErrors
        if mainAssertionFailed > 0 { evExitCode = 1; return 1, summary }
        if mainFailed > 0 { evExitCode = 4; return 4, summary }
    }
    return 0, summary
}
```

Add the builder helper:

```go
// buildMarkdownReport converts runner results into a markdown.Report.
// Mirrors buildHTMLReport in shape and call sites. Filters to main-phase
// requests only; setup and teardown are excluded from per-request markdown
// in M9-002 (see SPECIFICATION.md W4 — markdown is response-focused).
func buildMarkdownReport(col *parser.Collection, envName, runID string,
    results []runner.RequestResult, summary *runner.Summary) *markdown.Report {
    rep := &markdown.Report{
        CollectionName: col.Name,
        EnvName:        envName,
        RunID:          runID,
        StartedAt:      time.Now().UTC(),
    }
    if summary != nil {
        rep.Summary = markdown.SummaryCounts{
            Total: summary.Total, Passed: summary.Passed,
            Failed: summary.Failed, Skipped: summary.Skipped,
        }
    }
    for _, r := range results {
        if r.Phase != runner.PhaseMain {
            continue // setup/teardown not rendered as per-request markdown
        }
        entry := markdown.RequestEntry{
            RequestID:  r.RequestID,
            Slug:       r.RequestSlug,
            Name:       r.Name,
            Method:     r.Method,
            URL:        r.URL,
            WaveIndex:  r.WaveIndex,
            StartedAt:  rep.StartedAt, // per-request startedAt threading deferred to M9-004
            RequestHdr: r.RequestHeaders,
            RequestBody: r.RequestBody,
            Assertions: r.AssertionResults,
            Err:        r.Err,
            Skipped:    r.Skipped,
            SkipReason: r.SkipReason,
        }
        if r.Result != nil {
            entry.StatusCode = r.Result.StatusCode
            entry.DurationMs = r.Result.Duration.Milliseconds()
            entry.RespHeaders = r.Result.Headers
            entry.RespBody = r.Result.Body
        }
        rep.Requests = append(rep.Requests, entry)
    }
    return rep
}
```

Note: `EnsureReportDir` is exported as `markdown.EnsureReportDir` (capitalised
public form of the internal `ensureReportDir`); the cmd layer needs it.

#### Tests to Write FIRST (RED phase)

```go
// cmd/apitest/run_test.go

func TestRun_MarkdownFormat_HappyPath(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"id":1}`))
    }))
    defer srv.Close()

    dir := t.TempDir()
    col := writeTempCollection(t, dir, fmt.Sprintf(
        "name: Test\nrequests:\n  - name: Get user\n    request:\n      method: GET\n      url: %q\n",
        srv.URL))
    reportDir := filepath.Join(dir, "resp")

    var stdout, stderr bytes.Buffer
    code, _ := runCmdInner([]string{col, "--format", "markdown", "--report", reportDir}, &stdout, &stderr)
    if code != 0 {
        t.Fatalf("exit=%d stderr=%s", code, stderr.String())
    }

    // Files exist.
    requireFile(t, filepath.Join(reportDir, "run.md"))
    body := readFile(t, filepath.Join(reportDir, "get-user.md"))

    // 10-marker count from task observable.
    countMarkers := func(b []byte) int { /* grep -c the 10 markers */ }
    if got := countMarkers(body); got != 10 {
        t.Errorf("section markers = %d, want 10", got)
    }

    // Sentinel format
    re := regexp.MustCompile(`<!-- BEGIN apitest:response id=req-\d+ slug=get-user run=[0-9a-f]{32} -->`)
    if !re.Match(body) {
        t.Errorf("BEGIN sentinel not found or malformed: %s", body)
    }
}

func TestRun_MarkdownFormat_RequiresReport(t *testing.T) {
    // Run with --format markdown but no --report → exit 3 with stderr message.
}

func TestRun_MarkdownFormat_SpliceOnRerun(t *testing.T) {
    // Run once, append AGENT NOTE below the END sentinel, run again,
    // assert the AGENT NOTE survives byte-for-byte.
}

func TestRun_MarkdownFormat_NoSentinelWritesDotNew(t *testing.T) {
    // Pre-create reportDir/get-user.md without sentinels.
    // Run; assert original untouched, .md.new created, stderr warning.
}

func TestRun_MarkdownFormat_MalformedSentinelWritesDotNew(t *testing.T) {
    // Pre-create with BEGIN but no END; run; assert .md.new + warning.
}
```

#### Impact on Existing Tests

- `TestRun_*Format_*` (existing): unaffected — they test other formats.
- `TestRun_BadFormat`: regression — must verify the error message lists
  `markdown` in the supported set. Update the wantStderr string.
- `TestOutputPrecedence`: extend with one row for markdown; unchanged
  cases pass.
- `TestSchema_*` (existing): see Step 3.
- `TestConfig_*` (existing): see Step 3.

## Test Impact Summary

| Test File                                                     | Test Function                                  | Impact            | Action Required                                                    |
|---------------------------------------------------------------|------------------------------------------------|-------------------|--------------------------------------------------------------------|
| `internal/output/config_test.go`                              | `TestConfig_Validate.invalid_format`           | breaks            | Swap input from `"markdown"` to `"yaml"`. Add `"valid_markdown"` case. |
| `internal/schema/validate_coverage_test.go`                   | `TestSchema_rejects_unknown_output_format`     | breaks            | Swap input from `"markdown"` to `"yaml"`.                          |
| `internal/schema/validate_coverage_test.go`                   | `TestSchema_AcceptsMarkdownFormat` (new)       | new               | Cover both schemas accept `"markdown"`.                            |
| `internal/runner/runner_test.go`                              | `TestRunner_AlwaysMintsRequestIDs` (new)       | new               | Verify reqIDs minted without OnEvent.                              |
| `internal/runner/runner_test.go`                              | `TestRunner_RunIDInjectable` (new)             | new               | Verify vars.RunID override.                                        |
| `internal/runner/runner_test.go`                              | `TestRunner_RequestSlugCarriedToResult` (new)  | new               | Verify item.Slug → result.RequestSlug.                             |
| `internal/output/events/schema_test.go`                       | `TestEvents_*` golden tests                    | none              | Mint moves up; emit gate unchanged; sequence preserved.            |
| `internal/output/markdown/formatter_test.go`                  | `TestMarkdown_Render_PassJSON` etc.            | new               | New package suite.                                                 |
| `internal/output/markdown/splice_test.go`                     | `TestMarkdown_Splice_*` (six cases)            | new               | New package suite.                                                 |
| `internal/output/markdown/writer_test.go`                     | `TestMarkdown_Concurrent`                      | new               | New package suite.                                                 |
| `internal/output/markdown/run_md_test.go`                     | `TestMarkdown_RunMD`                           | new               | New package suite.                                                 |
| `internal/output/markdown/regression_test.go`                 | `TestMarkdown_DeterminismRegression`           | new               | New package suite.                                                 |
| `cmd/apitest/run_test.go`                                     | `TestRun_MarkdownFormat_*` (5 new)             | new               | End-to-end via runCmdInner.                                        |
| `cmd/apitest/run_test.go`                                     | `TestRun_BadFormat` (existing)                 | breaks            | Update wantStderr to include `markdown` in the supported list.     |
| `cmd/apitest/output_precedence_test.go`                       | `TestOutputPrecedence` (existing)              | extends           | Add markdown row.                                                  |
| `internal/parallel/executor_test.go`                          | parallel sink tests                            | none              | Sequence preserved.                                                |

## Risks and Edge Cases

- **Risk: runner-wide change to drop the OnEvent guard touches every
  RequestResult literal site.** The runner has 16+ literal sites for
  `RequestResult{...}`. Each must populate `RequestID` (from `reqID`) and
  `RequestSlug` (from `item.Slug` or `iterSlug`). **Mitigation:** lock the
  pattern with `TestRunner_AlwaysMintsRequestIDs` (asserts every non-skip
  result has a non-empty RequestID); fail loudly if any path is missed.

- **Risk: events stream byte-stability under the runner change.** Moving
  `reqID = vars.nextRequestID()` out of the `if vars.OnEvent != nil` gate
  changes WHEN reqID is generated, not WHAT sequence it produces.
  **Mitigation:** existing event golden tests in
  `internal/output/events/schema_test.go` will catch any sequence drift.
  CI already includes them.

- **Risk: schema enum changes break editor integrations.** Files at
  `schemas/*.json` are consumed by `.vscode/settings.json` (yaml.schemas).
  Adding to an enum is additive; no existing valid YAML breaks.
  **Mitigation:** Step 3 schema tests explicitly cover both accept-markdown
  and reject-yaml-now-not-markdown cases.

- **Edge case: extremely long URL or request body in the `### Request`
  block.** M9-002 does not cap body size (the 1 MiB cap lands in M9-003).
  **Handling:** dump verbatim for now; document this in the package
  comment so reviewers don't flag it. M9-003 enforces the cap.

- **Edge case: data-driven iteration markdown filenames.** The task scope
  defers data-driven layout to M9-004. The markdown formatter for M9-002
  filters non-data-driven main results only; data-driven iterations
  produce no markdown file in this slice. **Handling:** `buildMarkdownReport`
  skips entries with `r.IsDataDriven == true`; an integration test
  `TestRun_MarkdownFormat_SkipsDataDriven` verifies this contract and
  documents it as M9-004's responsibility.

- **Edge case: parallel main with wave grouping in run.md.** The task
  scope defers wave grouping in run.md to M9-004. For M9-002, run.md
  lists requests in execution-completion order. **Handling:**
  `renderRunMD` iterates `report.Requests` in slice order; the cmd-layer
  builder feeds results in completion order (the order runner.Run returns
  them).

- **Edge case: empty collection (no main requests).** `--format markdown
  --report dir` against a collection with only setup/teardown produces a
  `run.md` with zero per-request links and zero `<slug>.md` files.
  **Handling:** `WriteReport` handles `len(report.Requests) == 0`
  cleanly; `renderRunMD` emits a "no requests executed" note inside the
  sentinel block.

- **Edge case: race between two `apitest run` invocations writing to the
  same `--report dir`.** Atomic writes (O_EXCL temp + rename) prevent
  byte-level corruption. The "last writer wins" semantics for the file
  contents is acceptable per M9 design (different runs may legitimately
  produce different outputs). **Handling:** `TestMarkdown_Concurrent`
  pins the no-corruption guarantee; the user-visible "last writer wins"
  behaviour is documented in the package doc.

- **Risk: mismatch between markdown sentinel slug and event request_slug
  for data-driven iterations.** Not in scope (data-driven deferred to
  M9-004); markdown skips data-driven entries entirely in M9-002. **Mitigation:**
  the `buildMarkdownReport` filter is the contract surface and is tested.

- **Risk: introducing the `RunID` field to `Summary` ripples through the
  HTML/JUnit/JSON output builders.** All three builders ignore unknown
  fields on `Summary` (they read specific fields explicitly). Additive
  changes are safe. **Mitigation:** confirmed by reading all three
  builders during exploration; no code reads `Summary` reflectively.

- **Risk: `reportDir` is a file (not a directory).** If the user passes
  `--report ./run.md` while `--format markdown`, `MkdirAll` succeeds for
  empty paths but `WriteReport` will fail trying to write inside a file.
  **Handling:** `EnsureReportDir` calls `MkdirAll`; if the path exists
  and is a file, `MkdirAll` returns `ErrNotDirectory`; the cmd layer
  surfaces this as a structured error. Test
  `TestRun_MarkdownFormat_ReportPointsToFile` covers it.

- **Future-work note: `--markdown-prune` flag for orphan cleanup.** When
  a request is renamed, the orphan sentinel block lives forever (per
  the splice rules, orphans are never deleted). A future flag could let
  the user opt into cleanup. Out of scope for M9-002 by explicit task
  decision; documented here so the design choice is traceable.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh --go
```

Observable verification (matches task YAML):

```bash
./apitest run collections/sample.yaml --format markdown --report /tmp/resp
ls /tmp/resp/
# Expected: run.md get-user.md list-posts.md

# Section count
grep -c -E '^(# |## Notes$|<!-- BEGIN apitest:response|## Response \(deterministic\)$|### Request$|### Response |### Timing$|### Assertions$|<!-- END apitest:response|## Analysis$)' /tmp/resp/get-user.md
# Expected: 10

# Sentinel format
grep -Eo 'BEGIN apitest:response id=req-[0-9]+ slug=[a-z0-9-]+ run=[0-9a-f]{32}' /tmp/resp/get-user.md
# Expected: one match per file

# Splice preserves outside content
printf '\nAGENT NOTE: hypothesis X\n' >> /tmp/resp/get-user.md
./apitest run collections/sample.yaml --format markdown --report /tmp/resp
grep 'AGENT NOTE' /tmp/resp/get-user.md
# Expected: AGENT NOTE preserved

# Malformed sentinel writes .new
printf '# Existing\n<!-- BEGIN apitest:response id=req-1 slug=ghost run=0 -->\n' > /tmp/resp/ghost.md
./apitest run collections/with-ghost-slug.yaml --format markdown --report /tmp/resp 2>err.log
ls /tmp/resp/ghost.md /tmp/resp/ghost.md.new
grep 'malformed sentinel' err.log

# No-sentinel writes .new
printf '# User handwrote this\n' > /tmp/resp/handwritten.md
./apitest run collections/that-matches-handwritten-slug.yaml --format markdown --report /tmp/resp 2>err.log
ls /tmp/resp/handwritten.md /tmp/resp/handwritten.md.new
grep 'no sentinel pair' err.log

# run.md links
grep -cE '\.md\)$' /tmp/resp/run.md

# Schema accepts markdown
./apitest schema | jq -r '.properties.output.properties.format.enum' | grep -o markdown
./apitest schema --project | jq -r '.properties.output.properties.format.enum' | grep -o markdown

# Missing --report exits 3
./apitest run collections/sample.yaml --format markdown 2>err.log; echo $?
# Expected: 3

# Full unit + integration suite
go test -run 'TestMarkdown_Render|TestMarkdown_Splice|TestMarkdown_RunMD|TestMarkdown_Concurrent|TestMarkdown_Newlines|TestMarkdown_EmptyAssertions|TestMarkdown_DeterminismRegression|TestConfig_RejectsMarkdownWithoutReport|TestSchema_AcceptsMarkdownFormat' ./...
# Expected: PASS
```
