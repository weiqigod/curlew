# Implementation Plan: M1-016

## Overview
Add `setup:` and `teardown:` sections to collections, with teardown running unconditionally (even after main failures) and `required:` flag on setup requests to gate main execution.

## Task Details
- **ID:** M1-016
- **Title:** Setup and teardown sections
- **Phase:** M1: Core CLI
- **Priority:** 16
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-015 | External request file references | done |

---

## Implementation Steps

### Step 1: Parser — Add `Setup`, `Teardown`, and `Required` fields
**Rationale:** Pure data model expansion. No logic changes. Smallest blast radius — only struct definitions and YAML tags.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Setup`, `Teardown` to `Collection`; `Required` to `RequestItem`; `IsRequired()` helper |
| `internal/parser/collection_test.go` | create/modify | New tests for new fields (or add to parser_test.go) |
| `internal/parser/testdata/with_setup.yaml` | create | Collection with setup section |
| `internal/parser/testdata/with_teardown.yaml` | create | Collection with teardown section |
| `internal/parser/testdata/with_setup_teardown.yaml` | create | Both sections |
| `internal/parser/testdata/with_setup_required.yaml` | create | Setup with `required: true` |
| `internal/parser/testdata/with_setup_extract.yaml` | create | Setup with `extract:` block |

#### Current Code
```go
// internal/parser/collection.go
type Collection struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    Variables   map[string]string `yaml:"variables,omitempty"`
    Requests    []RequestItem     `yaml:"requests"`
    Options     Options           `yaml:"options,omitempty"`
}

type RequestItem struct {
    Name       string            `yaml:"name"`
    Path       string            `yaml:"path,omitempty"`
    Request    Request           `yaml:"request"`
    Variables  map[string]string `yaml:"variables,omitempty"`
    Assertions Assertions        `yaml:"assertions,omitempty"`
    Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
type Collection struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description,omitempty"`
    Variables   map[string]string `yaml:"variables,omitempty"`
    Setup       []RequestItem     `yaml:"setup,omitempty"`
    Requests    []RequestItem     `yaml:"requests"`
    Teardown    []RequestItem     `yaml:"teardown,omitempty"`
    Options     Options           `yaml:"options,omitempty"`
}

type RequestItem struct {
    Name       string            `yaml:"name"`
    Path       string            `yaml:"path,omitempty"`
    Required   *bool             `yaml:"required,omitempty"` // nil = false (default)
    Request    Request           `yaml:"request"`
    Variables  map[string]string `yaml:"variables,omitempty"`
    Assertions Assertions        `yaml:"assertions,omitempty"`
    Extract    map[string]string `yaml:"extract,omitempty"`
}

// IsRequired returns whether this item is marked required.
// Defaults to false when not explicitly set.
func (ri *RequestItem) IsRequired() bool {
    return ri.Required != nil && *ri.Required
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_setup_teardown(t *testing.T) {
    tests := []struct {
        name          string
        file          string
        wantSetupLen  int
        wantMainLen   int
        wantTdLen     int
    }{
        {"with setup only", "testdata/with_setup.yaml", 1, 2, 0},
        {"with teardown only", "testdata/with_teardown.yaml", 0, 2, 1},
        {"with both", "testdata/with_setup_teardown.yaml", 1, 2, 1},
        {"no setup or teardown", "testdata/minimal.yaml", 0, 1, 0},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            col, err := ParseFile(tt.file)
            require.NoError(t, err)
            assert.Len(t, col.Setup, tt.wantSetupLen)
            assert.Len(t, col.Requests, tt.wantMainLen)
            assert.Len(t, col.Teardown, tt.wantTdLen)
        })
    }
}

func TestParseFile_required_field(t *testing.T) {
    tests := []struct {
        name         string
        file         string
        wantRequired bool
    }{
        {"required:true", "testdata/with_setup_required.yaml", true},
        {"required not set defaults false", "testdata/with_setup.yaml", false},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing tests affected. `Setup` and `Teardown` are `omitempty` — existing YAML files without these keys unmarshal to nil slices, which match expected structs (nil == nil in `reflect.DeepEqual`).

---

### Step 2: Parser — Resolve and validate Setup/Teardown items
**Rationale:** Extends existing `ParseFile` to apply the same external-reference resolution and method validation to setup/teardown items as already applied to main requests.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Apply `resolveExternalReferences` and method normalization to `col.Setup` and `col.Teardown` |
| `internal/parser/parser_test.go` | modify | Add tests for setup/teardown resolution/validation |
| `internal/parser/testdata/with_setup_external_ref.yaml` | create | Setup item using external file ref |

#### Current Code (parser.go, the resolution loop)
```go
// Currently only processes col.Requests
for i := range col.Requests {
    if err := resolveExternalReferences(..., &col.Requests[i]); err != nil {
        return nil, err
    }
}
```

#### New Code
```go
// Process setup, main, and teardown sections
for _, items := range [][]RequestItem{col.Setup, col.Requests, col.Teardown} {
    for i := range items {
        if err := resolveExternalReferences(..., &items[i]); err != nil {
            return nil, err
        }
    }
}
```

*(Same pattern for method normalization and validation loops.)*

#### Tests to Write FIRST (RED phase)

```go
{"setup external ref resolves", "testdata/with_setup_external_ref.yaml", /* assert resolved */},
{"teardown external ref resolves", "testdata/with_teardown_external_ref.yaml", /* assert resolved */},
{"setup invalid method errors", "testdata/with_setup_bad_method.yaml", /* assert error */},
```

#### Impact on Existing Tests
- None. Existing test YAML files have no setup/teardown sections.

---

### Step 3: Runner — Phase type and struct additions
**Rationale:** Additive-only changes to runner data types. No logic changes. Existing tests are unaffected.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `Phase` type/constants, `Phase` field to `RequestResult`, `TeardownErrors` to `Summary` |

#### Current Code
```go
type RequestResult struct {
    Name             string
    Result           *httpexec.Result
    Err              error
    Skipped          bool
    AssertionResults *assertion.Results
}

type Summary struct {
    Total             int
    Passed            int
    Failed            int
    Skipped           int
    AssertionFailures int
    Duration          time.Duration
}
```

#### New Code
```go
// Phase identifies the execution phase of a request.
type Phase string

const (
    PhaseSetup    Phase = "setup"
    PhaseMain     Phase = "main"
    PhaseTeardown Phase = "teardown"
)

type RequestResult struct {
    Name             string
    Phase            Phase           // empty string treated as PhaseMain for backward compat
    Result           *httpexec.Result
    Err              error
    Skipped          bool
    AssertionResults *assertion.Results
}

type Summary struct {
    Total             int
    Passed            int
    Failed            int
    Skipped           int
    AssertionFailures int
    Duration          time.Duration
    TeardownErrors    int  // failures in teardown phase (do not affect exit code)
}
```

#### Tests to Write FIRST (RED phase)
```go
// Compile-time check that Phase constants exist — covered by using them in next step's tests.
```

#### Impact on Existing Tests
- None. New fields default to zero values. Existing tests access fields by name, not struct literals.

---

### Step 4: Runner — Implement three-phase execution
**Rationale:** Core behavioral change. Depends on Step 3 types. Implements all seven task behaviors.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Extract `executePhase` helper, restructure `Run` for setup→main→teardown |
| `internal/runner/runner_test.go` | modify | Add seven behavior tests (table-driven) |

#### New Code (Run function structure)
```go
func Run(ctx context.Context, col *parser.Collection, exec ExecuteFunc, vars VarSources) ([]RequestResult, *Summary, error) {
    // ... build scope (unchanged) ...

    var all []RequestResult
    start := time.Now()

    // Phase 1: Setup
    setupFailed := false
    if len(col.Setup) > 0 {
        setupResults, reqFailed := executePhase(ctx, col.Setup, scope, exec, vars, PhaseSetup,
            true /* checkRequired */, false /* stopOnFailure */)
        all = append(all, setupResults...)
        setupFailed = reqFailed
    }

    // Phase 2: Main (skipped if required setup failed)
    if setupFailed {
        for _, item := range col.Requests {
            all = append(all, RequestResult{Name: item.Name, Phase: PhaseMain, Skipped: true})
        }
    } else {
        mainResults, _ := executePhase(ctx, col.Requests, scope, exec, vars, PhaseMain,
            false /* checkRequired */, col.Options.StopOnFailure)
        all = append(all, mainResults...)
    }

    // Phase 3: Teardown — ALWAYS runs
    if len(col.Teardown) > 0 {
        tdResults, _ := executePhase(ctx, col.Teardown, scope, exec, vars, PhaseTeardown,
            false /* checkRequired */, false /* stopOnFailure */)
        all = append(all, tdResults...)
    }

    summary := computeSummary(all, time.Since(start))
    return all, summary, nil
}

// executePhase runs items sequentially, tagging each result with the given phase.
// Returns results and whether a required item failed (only meaningful for setup phase).
func executePhase(ctx context.Context, items []parser.RequestItem, scope *variable.Scope,
    exec ExecuteFunc, vars VarSources, phase Phase,
    checkRequired bool, stopOnFailure bool) ([]RequestResult, bool) {
    // ... per-item: interpolate → execute → assert → extract → tag phase ...
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_phases(t *testing.T) {
    tests := []struct {
        name           string
        col            *parser.Collection
        // mock exec responses
        wantPhaseOrder []Phase
        wantSkipped    []bool
    }{
        {
            name: "setup runs before main",
            // setup:1 item, main:2 items
            // wantPhaseOrder: [setup, main, main]
        },
        {
            name: "teardown runs after main",
            // main:2 items, teardown:1 item
            // wantPhaseOrder: [main, main, teardown]
        },
        {
            name: "teardown runs when main fails",
            // main:1 failing item, teardown:1 item
            // wantPhaseOrder: [main, teardown], teardown executed
        },
        {
            name: "required setup failure skips main runs teardown",
            // setup:1 required failing, main:2 items, teardown:1 item
            // wantSkipped: [false, true, true, false] (setup, main, main, teardown)
        },
        {
            name: "non-required setup failure continues main",
            // setup:1 non-required failing, main:2 items
            // wantSkipped: [false, false, false]
        },
        {
            name: "setup extract available in main and teardown",
            // setup extracts "id"="123", main and teardown use {{id}}
            // assert interpolated URLs contain "123"
        },
        {
            name: "teardown failure does not change main pass/fail",
            // main passes, teardown fails
            // summary.TeardownErrors==1, summary.Failed==1, but exit code logic ignores it
        },
        {
            name: "no setup no teardown behaves as before",
            // only Requests, no Setup/Teardown
            // wantPhaseOrder: [main, main]
        },
    }
}
```

#### Impact on Existing Tests
- Existing runner tests construct collections with only `Requests`. They will hit the `executePhase` path for main. Extracted phase tag is `PhaseMain`. Existing assertions on `Name`, `Err`, `Skipped`, `AssertionResults` are unaffected.

---

### Step 5: Output — Phase section headers
**Rationale:** Visual distinction in terminal output. Small, self-contained change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `PrintSectionHeader` function |
| `internal/output/terminal_test.go` | modify | Add test for `PrintSectionHeader` |

#### New Code
```go
// PrintSectionHeader writes a phase section header (e.g., "Setup:", "Teardown:").
func PrintSectionHeader(w io.Writer, section string) {
    _, _ = fmt.Fprintf(w, "\n%s:\n", section)
}
```

#### Tests to Write FIRST (RED phase)
```go
func TestPrintSectionHeader(t *testing.T) {
    tests := []struct {
        name    string
        section string
        want    string
    }{
        {"setup header", "Setup", "\nSetup:\n"},
        {"teardown header", "Teardown", "\nTeardown:\n"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var buf bytes.Buffer
            PrintSectionHeader(&buf, tt.section)
            assert.Equal(t, tt.want, buf.String())
        })
    }
}
```

#### Impact on Existing Tests
- None. New function only.

---

### Step 6: Main — Phase-aware rendering and exit code
**Rationale:** Wire everything together in the CLI. Depends on Steps 3–5.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Render section headers on phase transitions; exclude teardown from exit code determination |

#### Current Code (result rendering loop)
```go
for _, result := range results {
    // flat rendering, no phase awareness
    output.PrintResult(...)
}
// exit code based on summary.Failed + summary.AssertionFailures
```

#### New Code
```go
var currentPhase runner.Phase
for _, result := range results {
    if result.Phase != currentPhase {
        currentPhase = result.Phase
        switch currentPhase {
        case runner.PhaseSetup:
            output.PrintSectionHeader(os.Stdout, "Setup")
        case runner.PhaseTeardown:
            output.PrintSectionHeader(os.Stdout, "Teardown")
        }
    }
    // ... existing render logic ...
}

// Exit code excludes teardown failures
mainFailed := summary.Failed - summary.TeardownErrors
if mainFailed > 0 || summary.AssertionFailures > 0 {
    os.Exit(1)  // or appropriate exit code per spec
}
```

#### Impact on Existing Tests
- Collections without setup/teardown produce results with `Phase == PhaseMain`. No section header is printed for `PhaseMain` (it is the implicit default). Output identical to current behavior.

---

### Step 7: Smoke test
**Rationale:** End-to-end validation per Definition of Done.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add test case for setup→main→teardown execution |
| `smoke/collections/setup_teardown.yaml` | create | Smoke collection with all three sections |

---

### Step 8: CHANGELOG.md
Add entry for M1-016 setup/teardown feature.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` (existing) | none | — |
| `internal/runner/runner_test.go` | All existing tests | none | — |
| `internal/output/terminal_test.go` | All existing tests | none | — |
| `internal/runner/runner_test.go` | New phase behavior tests | new | Write first (RED) |
| `internal/parser/parser_test.go` | New setup/teardown parsing tests | new | Write first (RED) |
| `internal/output/terminal_test.go` | `TestPrintSectionHeader` | new | Write first (RED) |

---

## Variable Propagation Design

All three phases share a single `*variable.Scope`:

```
collection vars → scope
  → setup items: interpolate → execute → extract → scope.Set()
  → main items:  interpolate → execute → extract → scope.Set()  (sees setup extractions)
  → teardown:    interpolate → execute → extract → scope.Set()  (sees setup + main extractions)
```

Per-item `variables:` blocks create child scopes (precedence 8) within each item execution. This is unchanged from current behavior.

---

## Risks and Edge Cases

- **Required on teardown items** → Spec is silent. Runner ignores `required` in teardown phase; parser accepts it without error.
- **Context cancellation during teardown** → Teardown requests may time out. Acceptable for M1-016. Future: separate teardown context.
- **Multiple required setup failures** → Stop on first required failure; remaining setup items are also skipped.
- **`stop_on_failure` interaction** → `stop_on_failure` applies only to main phase. Setup uses `required`, teardown is always exhaustive.
- **Empty `setup: []` in YAML** → `len(col.Setup) == 0`, treated as absent (no-op).
- **External references in setup/teardown** → Must apply same `resolveExternalReferences` call as main requests (Step 2).
- **Summary counts** → `Total`/`Passed`/`Failed`/`Skipped` include all phases. `TeardownErrors` separates teardown failures so exit code logic can ignore them.

---

## Verification

```bash
go build ./cmd/curlew
go test ./...
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create smoke collection, run it, verify setup/main/teardown order in output
./curlew run smoke/collections/setup_teardown.yaml
# Expected: "Setup:" header → setup results → main results → "Teardown:" header → teardown results
```
