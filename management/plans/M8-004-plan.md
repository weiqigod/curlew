# Implementation Plan: M8-004

## Overview

Add a repeatable `--only "<name>"` flag to `curlew run` (and transitively `curlew watch`) that filters the collection's main requests to the selected set while leaving setup/teardown intact; reject duplicate main request names at parse time so every subcommand (`run`, `watch`, `validate`) inherits the invariant; enrich variable-cliff errors when a missing producer is a filtered-out main request; bump the events schema from v1.0 to v1.1 with an additive optional `selection` field on `run.start`.

## Task Details

- **ID:** M8-004
- **Title:** `run --only "<name>"` for single-request execution with duplicate-name rejection
- **Phase:** M8: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** medium
- **Estimated effort:** 2-3 days
- **Branch:** `feature/M8-004-only-flag-and-duplicate-rejection`

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M8-003 | `output:` block in project + collection YAML with CLI>collection>project precedence | done |

## Design decisions (locked)

| Decision | Choice | Rationale |
|---|---|---|
| Dup-check placement | In `ParseFileWithOptions` **after** `resolveIncludes` completes — not in `parseCollectionBytes` | `include:` can splice main items from child files; the duplicate invariant must cover the fully-resolved collection, not just the parent file's own items. Includes merge into `col.Requests.Items` at the parent level, so a post-include check catches both same-file and cross-file duplicates with no extra walk state. |
| Dup-check scope | Only `col.Requests.Items` (main phase) | Per spec: "duplicate main request names". Setup and teardown can legally share names with main (e.g. a setup "Create user" and a main "Create user" exist as distinct test stages). Extending to setup/teardown is out of scope. |
| Dup-check error format | `duplicate request name "<name>" at <file1>:<line1> and <file2>:<line2>` with the first two occurrences named when there are more | Matches the task's observable example verbatim. Uses existing M6-002 `SourceFile` / `SourceLine` plumbing. |
| New sentinel | `ErrDuplicateRequestName` in `internal/parser/errors.go` | Mirrors the existing sentinel-per-failure-mode pattern (`ErrEmptyCollection`, `ErrInvalidFieldValue`, etc.). Enables `errors.Is` in tests. |
| Hint registration | New `RegisteredError{Name: "ErrDuplicateRequestName", …, Code: "PARSE_DUPLICATE_REQUEST_NAME"}` in `hints_init.go` | Consistent with every other parser sentinel; gives events schema a stable `code`. |
| Selection field home | `runner.VarSources.Selection []string` | Same package that owns `CLI` / `EnvVar` / `EnvFile` / `AuthProfiles`; no new import; naturally threaded to `Run` → `runPhases`. |
| Filter application point | Shallow-copy the `*parser.Collection` inside `runner.Run` and replace `Requests.Items` with a filtered slice **before** calling `runPhases` | `runPhases`, `executePhase`, and `executeParallelMain` all read `col.Requests.Items`; filtering once at the top of `Run` is the minimal change. `col.Setup` and `col.Teardown` are shared by the shallow copy, so setup/teardown still run in full. |
| No-match behaviour | Fail before any HTTP: `runner.Run` returns a structured error (`ErrNoMatchingRequests`) naming available main request names; `cmd/curlew` maps this to exit 3 with a stderr message | Observable requires exit 3 with "available:" list. Handling in `runner.Run` keeps the check central for all callers (watch, future programmatic consumers). |
| Variable-cliff diagnostic | Wrap the existing `enrichInterpErr` output with a selection-aware enricher at the main-phase interpolation call sites only (sequential + parallel + websocket branches) | Setup/teardown items cannot "be filtered out by --only", so the diagnostic is main-phase-specific. Uses `parallel.ExtractProducedVars` over the **full** main items list (not the filtered one) to locate the producer. |
| Undefined-variable name extraction | Regex-match `^undefined variable "([^"]+)"$` on the structured error's `Message`, guarded by a unit test that asserts the format stays stable | The format is owned by `internal/variable/variable.go` at a single emission site; adding a typed field on `*apierrors.Structured` would touch a shared cross-package type for one caller. Regex is the least-invasive option; a format-guard test prevents silent drift. |
| Selection flag parsing | Repeatable `--only <name>` in `parseRunArgs` (hand-rolled, slice-accumulated, whitespace-trimmed) | Mirrors `--var` / `--env-var` exactly; no new flag framework. |
| Whitespace handling | `strings.TrimSpace` applied after accumulation; case-sensitive exact match | Task: "surrounding whitespace is trimmed; values are exact, case-sensitive matches". |
| Setup/teardown names via --only | Match against main-phase names only; a setup/teardown name will produce zero matches and trigger the exit-3 no-match error | Task: "--only does not target setup or teardown items". |
| Events schema bump | `SchemaVersion` const `"1.0"` → `"1.1"`; new file `docs/events-schema/v1.1.json`; retain `v1.0.json` unchanged | Matches v0.1 → v1.0 retention precedent documented in `internal/output/events/events.go:3`. |
| Emitter API extension | Add `EmitRunStartWithInput(RunStartInput)` method + `RunStartInput` struct; keep existing `EmitRunStart(cliArgs, collectionFile, envName)` as a thin wrapper for backward compatibility | Mirrors the existing `RequestEndInput` + `EmitRequestEnd(RequestEndInput)` pattern. New field (`Selection`) is added to `RunStartInput`; existing tests and callers keep working. |
| Schema test migration | `schemaPath` in `internal/output/events/schema_test.go` switches to `v1.1.json`; add a new `TestSchema_v10ArtifactsRetained` guard alongside existing `TestSchema_v01ArtifactsRetained` | The authoritative schema for compile-time validation must match the emitted `schema_version`. v1.0.json stays as a historical anchor, guarded by a retention test. |
| Golden file updates | Regenerate `run_happy.ndjson`, `run_error.ndjson`, `run_failed_assertion.ndjson` with `"schema_version":"1.1"` | Goldens must match emitter output. |
| New doc | `docs/EVENTS_SCHEMA_v1.1.md` (human-facing reference) alongside existing `EVENTS_SCHEMA_v1.0.md` and `EVENTS_SCHEMA_v0.1.md` | Matches retention precedent. |
| Feature gate | None | `--only` is free-tier; matches existing `--var` / `--env-var` gating policy. |
| Data-driven iterations | When a data-driven main request is selected, all its iterations run | Task: "Data-driven requests selected by --only run all of their declared iterations; per-iteration filtering is V2 scope." Requires no extra code — the filter runs at the main-items level, `executeDataDriven` handles all iterations of any item it receives. |
| Parallel analyzer | `parallel.Analyze` is re-run on the filtered subset (no code change required) | Task: "the analyzer already operates on any []RequestItem and requires no changes." Confirmed by reading `internal/parallel/analyze.go`: `Analyze(items []parser.RequestItem, preExecVars map[string]bool, …)` is slice-scoped and re-computes waves from scratch on every call. |
| Watch plumbing | None — `watchCmdOut` passes filtered CLI args verbatim through `watch.Config.RunFunc` on every rebuild (M7-005 pattern) | Task: "Watch mode requires no plumbing changes." Verified by reading `cmd/curlew/main.go:1748-1758`. |
| Validate behaviour | `validator.Validate` calls `parser.ParseFile`, which now carries the duplicate-name error; `validateCmdOut` already surfaces parse errors at exit 3 | Task: "curlew validate surfaces duplicate-name rejection as a static error (exit 3)." Requires no change to `validator.go` or `validateCmdOut`. |

## Implementation Steps

Ordered for smallest blast radius. Parser invariant first (all subcommands inherit free), then runner, then CLI wiring, then schema/docs.

### Step 1: Parser duplicate-name rejection

**Rationale:** Zero new dependencies from the rest of the changes. Once this step lands, every path that loads a collection (`run`, `watch`, `validate`) already fails correctly on duplicates. This is the smallest self-contained unit of work and de-risks the rest of the task.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/errors.go` | modify | Add `ErrDuplicateRequestName` sentinel. |
| `internal/parser/hints_init.go` | modify | Register `ErrDuplicateRequestName` with code `PARSE_DUPLICATE_REQUEST_NAME` and actionable hint. |
| `internal/parser/parser.go` | modify | Add `checkDuplicateNames(col *Collection, rootPath string) error` and invoke it at the end of `ParseFileWithOptions` (after `resolveIncludes`). |
| `internal/parser/parser_test.go` | modify | Add `TestParser_DuplicateNameRejection` table-driven test covering same-file, cross-include, and no-duplicate cases. |

#### New sentinel (errors.go)

```go
// ErrDuplicateRequestName is returned when two or more main requests share the
// same name within a collection (including items spliced in via include:).
ErrDuplicateRequestName = errors.New("duplicate request name")
```

#### New hint (hints_init.go) — appended to the `RegisterPackage("parser", …)` call

```go
apierrors.RegisteredError{
    Name: "ErrDuplicateRequestName",
    Err:  ErrDuplicateRequestName,
    Hint: apierrors.ClassifiedHint{
        Category: apierrors.CategoryParse,
        Code:     "PARSE_DUPLICATE_REQUEST_NAME",
        Hint:     "Rename one of the duplicated requests. Main request names must be unique because --only, per-request markdown files, and CI report rows all address requests by name.",
    },
},
```

#### New check (parser.go, appended after `return col, nil` block in `ParseFileWithOptions` — before the function returns)

```go
func checkDuplicateNames(col *Collection, rootPath string) error {
    type loc struct{ file string; line int }
    first := make(map[string]loc, len(col.Requests.Items))
    for _, item := range col.Requests.Items {
        if item.Name == "" {
            continue // empty names are caught by other validators
        }
        l := loc{file: item.SourceFile, line: item.SourceLine}
        if prev, dup := first[item.Name]; dup {
            return &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: rootPath,
                Line:     prev.line,
                Message: fmt.Sprintf(
                    "duplicate request name %q at %s:%d and %s:%d",
                    item.Name, prev.file, prev.line, l.file, l.line,
                ),
                Hint:  "Rename one of the duplicated requests. Main request names must be unique because --only, per-request markdown files, and CI report rows all address requests by name.",
                Inner: ErrDuplicateRequestName,
            }
        }
        first[item.Name] = l
    }
    return nil
}
```

Called at the end of `ParseFileWithOptions`:

```go
// M8-004: reject duplicate main request names. Runs after include resolution
// so items spliced via include: are covered.
if err := checkDuplicateNames(col, path); err != nil {
    return nil, err
}
return col, nil
```

#### Tests to Write FIRST (RED phase)

```go
func TestParser_DuplicateNameRejection(t *testing.T) {
    tests := []struct {
        name         string
        makeFiles    func(t *testing.T, dir string) string // returns path to root collection
        wantErr      bool
        wantErrIs    error
        wantMsgParts []string // substrings that must appear in err.Error()
    }{
        {
            name: "same-file duplicate rejected",
            makeFiles: func(t *testing.T, dir string) string {
                p := filepath.Join(dir, "dup.yaml")
                body := `name: Dup
requests:
  - name: Get user
    request: {method: GET, url: https://example.com/a}
  - name: Get user
    request: {method: GET, url: https://example.com/b}
`
                if err := os.WriteFile(p, []byte(body), 0o600); err != nil { t.Fatal(err) }
                return p
            },
            wantErr:      true,
            wantErrIs:    ErrDuplicateRequestName,
            wantMsgParts: []string{`duplicate request name "Get user"`, `dup.yaml:3`, `dup.yaml:5`},
        },
        {
            name: "cross-include duplicate rejected",
            makeFiles: func(t *testing.T, dir string) string { /* parent + child, both with "Get user" in main */ return p },
            wantErr:      true,
            wantErrIs:    ErrDuplicateRequestName,
            wantMsgParts: []string{`duplicate request name "Get user"`, "parent.yaml", "child.yaml"},
        },
        {
            name: "setup and main with same name is allowed",
            makeFiles: func(t *testing.T, dir string) string { /* setup: Get token; requests: Get token */ return p },
            wantErr:   false,
        },
        {
            name: "no duplicates is accepted",
            makeFiles: func(t *testing.T, dir string) string { /* two main, distinct names */ return p },
            wantErr:   false,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            dir := t.TempDir()
            root := tt.makeFiles(t, dir)
            _, err := ParseFile(root)
            if tt.wantErr {
                if err == nil { t.Fatal("want error, got nil") }
                if !errors.Is(err, tt.wantErrIs) { t.Fatalf("got %v, want errors.Is %v", err, tt.wantErrIs) }
                for _, part := range tt.wantMsgParts {
                    if !strings.Contains(err.Error(), part) {
                        t.Errorf("err missing substring %q:\n  err: %s", part, err)
                    }
                }
                return
            }
            if err != nil { t.Fatalf("unexpected error: %v", err) }
        })
    }
}
```

#### Impact on Existing Tests

- `internal/parser/parser_test.go::TestParseFile` — no impact (no fixture has duplicate names).
- `internal/parser/include_*_test.go` — audit fixtures; if any intentionally share a main-request name across included files, rename one. Search plan: `grep -rn "name:" internal/parser/testdata/include_*.yaml` at execute time; unlikely given the focus of those tests on variable propagation.
- `internal/runner/runner_test.go` — programmatically-constructed `parser.Collection` values that share a main name across items? No: the check lives in `ParseFile`, which those tests do not call. No impact.
- `cmd/curlew/*_test.go` — any test fixture YAML with duplicate names? Search: `grep -l "requests:" cmd/curlew/testdata/**/*.yaml | xargs grep -A 1 "name:"`. Audit at execute time.

---

### Step 2: Runner selection filter + no-match error

**Rationale:** Now that the parser guarantees unique main names, the runner filter can trust name equality as a primary key. This step is also free-standing — no CLI code touches it yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `Selection []string` field to `VarSources`. Add `ErrNoMatchingRequests` sentinel. In `Run`, apply the filter to a shallow-copied `*parser.Collection` before calling `runPhases`. |
| `internal/runner/runner_test.go` | modify | Add `TestRunner_OnlyFilter` covering single match, union, no-match, case-sensitive, whitespace-trimmed, setup/teardown still run in full. |

#### New field on VarSources (runner.go)

```go
// Selection is the list of main request names passed via --only. When non-empty,
// the runner filters col.Requests.Items to items whose Name matches exactly
// (case-sensitive). Setup and teardown are never filtered. An empty or nil
// Selection means "run all main requests" (the default). Whitespace trimming
// is the caller's responsibility.
Selection []string
```

#### New sentinel

```go
// ErrNoMatchingRequests is returned when --only names at least one request but
// none of the provided names match any main-phase item. The error message
// lists the available main request names.
var ErrNoMatchingRequests = errors.New("no main requests matched --only")
```

#### Filter in Run (runner.go)

Insert after `buildScope` succeeds and BEFORE `runPhases`:

```go
// M8-004: apply --only selection to main-phase items. Setup and teardown
// are unaffected. When Selection is set but yields zero matches, fail with
// a structured error before any HTTP runs.
if len(vars.Selection) > 0 {
    filtered, err := filterMainItemsBySelection(col.Requests.Items, vars.Selection)
    if err != nil {
        return nil, emptySummary, err
    }
    // Shallow-copy col so we replace Requests.Items without mutating the
    // caller's parsed collection. Setup and Teardown remain shared pointers
    // (no copy needed — we only read them).
    shallow := *col
    shallow.Requests = parser.Section{Retry: col.Requests.Retry, Items: filtered}
    col = &shallow
}
```

```go
func filterMainItemsBySelection(items []parser.RequestItem, selection []string) ([]parser.RequestItem, error) {
    selected := make(map[string]struct{}, len(selection))
    for _, n := range selection {
        selected[n] = struct{}{}
    }
    out := make([]parser.RequestItem, 0, len(selection))
    for _, it := range items {
        if _, ok := selected[it.Name]; ok {
            out = append(out, it)
        }
    }
    if len(out) == 0 {
        available := make([]string, 0, len(items))
        for _, it := range items {
            available = append(available, it.Name)
        }
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryInput,
            Code:     "ONLY_NO_MATCH",
            Message:  fmt.Sprintf("no request named %q; available: %s", selection[0], quotedJoin(available)),
            Hint:     "Pass --only <name> with a name that matches one of the available main requests. --only does not target setup or teardown items.",
            Inner:    ErrNoMatchingRequests,
        }
    }
    return out, nil
}
```

Note: `col.Requests.Retry` is preserved on the shallow copy so section-level retry configuration continues to apply to the filtered items.

#### Tests to Write FIRST (RED phase)

```go
func TestRunner_OnlyFilter(t *testing.T) {
    tests := []struct {
        name             string
        mainNames        []string   // items in col.Requests
        selection        []string   // vars.Selection
        wantExecuted     []string   // names of requests actually executed (in order)
        wantSetupRun     bool
        wantTeardownRun  bool
        wantErrIs        error      // non-nil when the filter itself returns an error
    }{
        {"single match",               []string{"A","B","C"}, []string{"B"},        []string{"B"},     true, true, nil},
        {"union of two",               []string{"A","B","C"}, []string{"A","C"},    []string{"A","C"}, true, true, nil},
        {"case-sensitive mismatch",    []string{"Get user"},  []string{"get user"}, nil,                true, true, ErrNoMatchingRequests},
        {"no match lists available",   []string{"A","B"},     []string{"Z"},        nil,                true, true, ErrNoMatchingRequests},
        {"setup name does not match",  []string{"Main"},      []string{"Setup"},    nil,                true, true, ErrNoMatchingRequests},
        {"nil selection = all",        []string{"A","B"},     nil,                   []string{"A","B"},  true, true, nil},
    }
    // construct *parser.Collection with a setup item named "Setup", main items per mainNames, and a teardown item named "Teardown"; use a recording executor to assert execution order.
}
```

#### Impact on Existing Tests

- All existing tests leave `VarSources.Selection` as nil → zero behavioural change.
- `enrichInterpErr`-using tests are unchanged at this step.
- Shallow-copy safety: `runPhases` reads `col.Setup`, `col.Requests`, `col.Teardown`, `col.Retry`, `col.Options`, `col.RateLimitRPS`, and `col.Output`. The shallow copy preserves all non-main fields by value (`col.Retry` is a pointer shared with the caller — safe; `col.Output` same). No existing tests assert pointer identity.

---

### Step 3: Variable-cliff diagnostic

**Rationale:** Depends on Step 2 (needs `vars.Selection` to know when to enrich) and the `SourceFile`/`SourceLine` already provided by M6-002. Independent of CLI wiring. Only fires on the main-phase interpolation error paths.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `enrichSelectionCliff(err, item, fullMainItems, selection)` helper. Wrap `enrichInterpErr` output at the three main-phase interpolation error return sites: sequential (`executePhase`), parallel-HTTP (`executeParallelMain` via `parallel.Config.ExecFunc`), and WebSocket (`buildWebSocketFunc`). |
| `internal/runner/runner_test.go` | modify | Add `TestRun_OnlyVariableCliff`: diagnostic fires when producer is filtered out; does NOT fire when producer is in the selection; does NOT fire when the variable has no producer (falls back to existing error). |

#### New helper (runner.go)

```go
// undefinedVarRe captures the variable name from a structured error produced by
// variable.Scope.Interpolate. The producing format is owned by
// internal/variable/variable.go: fmt.Sprintf("undefined variable %q", name).
// A unit test in this package guards that contract.
var undefinedVarRe = regexp.MustCompile(`^undefined variable "([^"]+)"$`)

// enrichSelectionCliff looks for an undefined-variable error in err's chain
// and, when --only is active, checks whether the missing variable's producer is
// a filtered-out main item. Returns err unchanged when Selection is empty, when
// the error is not an undefined-variable error, or when the producer is not in
// fullMainItems (fallback to the existing error).
func enrichSelectionCliff(err error, fullMainItems []parser.RequestItem, selection []string) error {
    if err == nil || len(selection) == 0 {
        return err
    }
    var s *apierrors.Structured
    if !errors.As(err, &s) { return err }
    if !errors.Is(err, variable.ErrUndefinedVariable) { return err }
    m := undefinedVarRe.FindStringSubmatch(s.Message)
    if len(m) != 2 { return err }
    varName := m[1]
    // Build the set of selected names.
    sel := make(map[string]struct{}, len(selection))
    for _, n := range selection { sel[n] = struct{}{} }
    // Scan full main items for a producer of varName.
    for _, it := range fullMainItems {
        produced, _ := parallel.ExtractProducedVars(it.Extract)
        if !produced[varName] { continue }
        if _, inSelection := sel[it.Name]; inSelection {
            // Producer IS selected — different failure mode, leave err alone.
            return err
        }
        // Producer is filtered out by --only. Enrich.
        s.Message = fmt.Sprintf(
            "variable {{%s}} is not defined; normally extracted from %q which was not included by --only",
            varName, it.Name,
        )
        s.Hint = "Add the producer to --only (e.g. --only %q --only %q) or pass the variable explicitly via --var %s=<value>."
        s.Hint = fmt.Sprintf(s.Hint, it.Name, selection[0], varName)
        return err
    }
    return err
}
```

Thread `fullMainItems` through the error path. Since the filter in Step 2 replaces `col.Requests.Items` on the shallow copy, we need to pass the ORIGINAL items list to the executor sites. Simplest approach: capture the pre-filter items in a local `fullMain := col.Requests.Items` at the top of `Run`, and thread it via `VarSources` (unexported field `fullMainForCliff []parser.RequestItem`, set by `Run` itself — matches the existing unexported `globalLimiter` / `reqIDCounter` pattern on `VarSources`).

Alternative: add an explicit parameter to `executePhase` and `executeParallelMain`. The unexported-VarSources approach is preferred — it extends a documented internal-state pattern and avoids adding a parameter to two call sites (plus all their tests).

```go
// In runner.VarSources (existing struct):
// fullMainForCliff is internal state populated by Run so the --only
// variable-cliff diagnostic can look up producers over the un-filtered main
// items list. Callers should leave this nil.
fullMainForCliff []parser.RequestItem
```

At each existing `interpErr = enrichInterpErr(interpErr, item)` line in main-phase branches only, add:

```go
interpErr = enrichSelectionCliff(interpErr, vars.fullMainForCliff, vars.Selection)
```

Setup/teardown interpolation sites do NOT get this wrapper (not main-phase; the "filtered-out producer" concept does not apply).

#### Format-guard test (prevents regex drift)

```go
// TestRunner_UndefinedVarMessageFormatStable locks the structured error message
// format that enrichSelectionCliff parses. If internal/variable changes the
// emission, this test fails so the regex is updated deliberately.
func TestRunner_UndefinedVarMessageFormatStable(t *testing.T) {
    scope := variable.NewScope(nil)
    _, err := scope.Interpolate("{{foo_bar}}")
    var s *apierrors.Structured
    if !errors.As(err, &s) { t.Fatalf("want *apierrors.Structured, got %T", err) }
    want := `undefined variable "foo_bar"`
    if s.Message != want {
        t.Errorf("undefined-variable message drifted:\n  got:  %q\n  want: %q\n  (update undefinedVarRe in runner.go if the format changed intentionally)", s.Message, want)
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_OnlyVariableCliff(t *testing.T) {
    // main items: "Create user" extracts user_id; "Update user" references {{user_id}}.
    // --only "Update user": interpolation fails; error must name "Create user".
    // --only "Create user" and "Update user": error does NOT fire (producer included) — passes.
    // No --only: existing error path.
    // Producer not found: fall back to the plain undefined-variable message.
    tests := []struct {
        name        string
        selection   []string
        wantMsgPart string
    }{
        {"producer filtered out", []string{"Update user"}, `variable {{user_id}} is not defined; normally extracted from "Create user" which was not included by --only`},
        {"producer selected",     []string{"Create user","Update user"}, `undefined variable "user_id"`}, // shouldn't fail at all; but if we construct a scenario where it does (e.g. extract fails), fall back.
        {"no producer exists",    []string{"Update user"}, `undefined variable "user_id"`}, // extract map empty on all items
    }
    // ...
}
```

#### Impact on Existing Tests

- Zero: `enrichSelectionCliff` short-circuits when `Selection` is empty. All existing tests pass without modification.

---

### Step 4: Events schema v1.1 — struct field + schema file + golden updates

**Rationale:** Additive schema change; depends on Step 2 only to know what to emit (the Selection slice is already available via `vars.Selection`). Independent of Step 3 (variable-cliff messages go through `run.error` / `request.end`, not `run.start`). Sequenced early because it locks the observable contract for Step 5's CLI wiring.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/events.go` | modify | Bump `SchemaVersion` from `"1.0"` to `"1.1"`. Add `Selection []string "json:\"selection,omitempty\""` field to `RunStart`. Update package doc comment to describe v1.1 contract. |
| `internal/output/events/emitter.go` | modify | Add `RunStartInput` struct + `EmitRunStartWithInput(RunStartInput)` method. Refactor existing `EmitRunStart(cliArgs, collectionFile, envName)` to delegate. |
| `docs/events-schema/v1.1.json` | create | Full copy of v1.0.json; update `$id`, `title`, every `const: "1.0"` → `"1.1"`; add optional `selection: {type: array, items: {type: string, minLength: 1}, description: …}` under `RunStart.properties`; keep everything else byte-identical. |
| `docs/events-schema/v1.0.json` | — | **Unchanged** (historical anchor). |
| `docs/EVENTS_SCHEMA_v1.1.md` | create | Human-facing reference. v1.0 → v1.1 changelog (one additive field). Reuses the v1.0 body verbatim otherwise. |
| `docs/EVENTS_SCHEMA_v1.0.md` | — | **Unchanged** (historical anchor). |
| `internal/output/events/emitter_test.go` | modify | Add `TestEvents_v11_Selection`. Update existing `schema_version` string assertions from `"1.0"` to `"1.1"` (see Test Impact Summary). |
| `internal/output/events/schema_test.go` | modify | `schemaPath` returns `v1.1.json`. `eventSchemaDocPath` returns `EVENTS_SCHEMA_v1.1.md`. Add `TestSchema_v10ArtifactsRetained`. Add `TestSchema_v11_validates` new test. |
| `internal/output/events/testdata/golden/*.ndjson` | regenerate | Run with `UPDATE_GOLDEN=1` after schema bump; every line now has `"schema_version":"1.1"`. |

#### New RunStart field (events.go)

```go
// RunStart is the first event emitted for every run.
type RunStart struct {
    Header
    StartedAt      string   `json:"started_at"` // RFC3339Nano UTC
    CurlewVersion string   `json:"curlew_version"`
    CLIArgs        []string `json:"cli_args"`
    CollectionFile string   `json:"collection_file,omitempty"`
    EnvName        string   `json:"env_name,omitempty"`
    // Selection carries the --only values for this run (M8-004). Omitted when
    // --only was not supplied. Each entry is a main request name (case-sensitive).
    Selection      []string `json:"selection,omitempty"`
}
```

#### RunStartInput + new method (emitter.go)

```go
// RunStartInput packages the inputs to EmitRunStartWithInput.
type RunStartInput struct {
    CLIArgs        []string
    CollectionFile string
    EnvName        string
    // Selection carries --only values. Nil or empty => field omitted in the
    // emitted event per v1.1 schema.
    Selection      []string
}

// EmitRunStartWithInput emits a run.start event with optional selection. Added
// in schema v1.1. Callers that do not need --only can continue to use the
// backwards-compatible EmitRunStart(cliArgs, collectionFile, envName).
func (e *Emitter) EmitRunStartWithInput(in RunStartInput) error {
    cliArgs := in.CLIArgs
    if cliArgs == nil { cliArgs = []string{} }
    id := e.nextID()
    ev := RunStart{
        Header: Header{
            SchemaVersion: SchemaVersion,
            RunID:         e.runID,
            ID:            id,
            AtMs:          0,
            Kind:          KindRunStart,
        },
        StartedAt:      e.startTime.UTC().Format(time.RFC3339Nano),
        CurlewVersion: e.opts.CurlewVersion,
        CLIArgs:        cliArgs,
        CollectionFile: in.CollectionFile,
        EnvName:        in.EnvName,
        Selection:      in.Selection,
    }
    return e.writeEvent(ev)
}

// EmitRunStart is retained for backward compatibility. Equivalent to calling
// EmitRunStartWithInput with Selection unset.
func (e *Emitter) EmitRunStart(cliArgs []string, collectionFile, envName string) error {
    return e.EmitRunStartWithInput(RunStartInput{
        CLIArgs:        cliArgs,
        CollectionFile: collectionFile,
        EnvName:        envName,
    })
}
```

#### v1.1 schema delta (docs/events-schema/v1.1.json)

In the RunStart definition, add to `properties`:

```json
"selection": {
  "type": "array",
  "items": { "type": "string", "minLength": 1 },
  "description": "Names passed via --only for this run. Omitted when --only was not supplied. Added in v1.1."
}
```

All existing `const: "1.0"` become `const: "1.1"`. `$id` becomes `https://curlew.dev/events-schema/v1.1.json`. Title becomes `Curlew Agent Event Stream v1.1`.

#### Tests to Write FIRST (RED phase)

```go
func TestEvents_v11_Selection(t *testing.T) {
    // Case 1: selection set → event carries "selection":["Get user","Update user"]
    // Case 2: selection empty/nil → event omits "selection" field entirely
    // Case 3: schema_version on every emitted event is "1.1"
    tests := []struct {
        name       string
        selection  []string
        wantField  bool   // true if "selection" key must appear
        wantValues []string
    }{
        {"selection present",  []string{"Get user"},                  true,  []string{"Get user"}},
        {"selection with two", []string{"Get user","Update user"},    true,  []string{"Get user","Update user"}},
        {"selection nil",      nil,                                    false, nil},
        {"selection empty",    []string{},                             false, nil}, // omitempty on []string with len==0 drops the field
    }
    // Build an emitter; call EmitRunStartWithInput; decode the first line;
    // assert schema_version == "1.1" and the presence/absence of "selection".
}

func TestSchema_v11_validates(t *testing.T) {
    // Compile v1.1.json; emit one of each event kind with Selection set on RunStart; validate every line.
}
```

#### Impact on Existing Tests

| Test file | Test function | Impact | Action Required |
|-----------|---------------|--------|-----------------|
| `internal/output/events/emitter_test.go` | Every test that asserts `"schema_version":"1.0"` in the serialized line | breaks | Update literal `"1.0"` → `"1.1"`. |
| `internal/output/events/schema_test.go` | `TestEmitter_AllKindsValidateAgainstSchema`, `TestEmitter_GoldenSchemaValidates`, `TestEmitter_GoldenRunHappy`, `TestEmitter_GoldenRunError`, `TestEmitter_GoldenRunFailedAssertion`, `TestSchema_DocInSyncWithCode`, `TestSchema_MarkdownExamplesValidate` | breaks when schemaPath/docPath switch | Update paths to `v1.1.json` / `EVENTS_SCHEMA_v1.1.md`. |
| `internal/output/events/schema_test.go::TestSchema_v01ArtifactsRetained` | — | no impact | Retained (still guards v0.1 presence). |
| `internal/output/events/testdata/golden/*.ndjson` | `TestEmitter_GoldenRun*` | golden mismatch | Regenerate with `UPDATE_GOLDEN=1` after the SchemaVersion bump. |
| `cmd/curlew/testdata/**/*.golden` / related | events snapshots? | search at execute time | Run `grep -rn "schema_version\":\"1.0\"" cmd/ docs/` and update. |
| `cmd/curlew/run_test.go::TestRunCmd_events_*` (if any) | events content-matching | possibly breaks | Update literal version strings. |

Add new test:
```go
func TestSchema_v10ArtifactsRetained(t *testing.T) {
    // Assert docs/events-schema/v1.0.json and docs/EVENTS_SCHEMA_v1.0.md still exist after the v1.1 promotion (retention contract, mirrors TestSchema_v01ArtifactsRetained).
}
```

---

### Step 5: CLI wiring — parse `--only`, propagate, early no-match diagnostic, help text

**Rationale:** Depends on all previous steps. Smallest surface change at this point: add the flag, wire it into `VarSources.Selection` and `RunStartInput.Selection`, update help/usage strings.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add `onlyNames []string` to `runFlags`. Parse `--only <name>` in `parseRunArgs` (repeatable, whitespace-trimmed). Thread `flags.onlyNames` into `runner.VarSources.Selection` and `events.RunStartInput.Selection`. Update the run Usage string and `printRunHelpTo` Run Options section. |
| `cmd/curlew/run_test.go` | modify | Add `TestRun_OnlyNoMatch` covering exit 3 + stderr content. Add `TestRunCmd_only_filter_integration` covering single match and union end-to-end. |
| `cmd/curlew/main_test.go` | modify | Add parse-level tests: `TestParseRunArgs_Only` (single, repeated, missing value, whitespace-trim). |
| `CHANGELOG.md` | modify | Add Added and Changed bullets under `## [Unreleased]`. |
| `docs/SPECIFICATION.md` | modify | Document `--only` in the run command section (flags reference). |
| `docs/MANUAL.md` | modify | Add an `--only` example to Part 4 (Running Tests in CI) or a dedicated "Running a single request" subsection. |

#### New runFlags field

```go
// M8-004: --only flag accumulating main request names to run exclusively.
onlyNames []string
```

#### New parseRunArgs case (mirrors --var)

```go
case "--only":
    i++
    if i >= len(args) {
        return errorf("--only requires a name (e.g. --only \"Get user\")")
    }
    f.onlyNames = append(f.onlyNames, strings.TrimSpace(args[i]))
```

After accumulation, in `runCmdInner` just before the `runner.Run(…)` call:

```go
// M8-004: selection passed down into runner.VarSources.
vars.Selection = flags.onlyNames
```

`events.RunStartInput.Selection = flags.onlyNames` in the new `EmitRunStartWithInput` call sites. Both `EmitRunStart` call sites (early and late-open) must switch to `EmitRunStartWithInput`.

#### Exit mapping for no-match

`runner.Run` returns the structured error wrapping `ErrNoMatchingRequests`. In `runCmdInner`, after `runner.Run` returns `varErr`, add:

```go
if varErr != nil && errors.Is(varErr, runner.ErrNoMatchingRequests) {
    if eventsEmitter != nil {
        _ = eventsEmitter.EmitRunError(varErr)
        evExitCode = 3
    }
    errOut.StructuredError(varErr)
    return 3, summary
}
```

Place this BEFORE the existing gate-error / team-template-error handling so no-match never gets misclassified.

#### Help text additions (printRunHelpTo / Run Options section)

```go
_, _ = fmt.Fprintln(w, "  --only \"<name>\"      Run only the named main request; repeatable for a union; setup/teardown still run")
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseRunArgs_Only(t *testing.T) {
    tests := []struct {
        name      string
        args      []string
        want      []string
        wantErr   bool
    }{
        {"single", []string{"--only","Get user","file.yaml"},                        []string{"Get user"}, false},
        {"repeated union", []string{"--only","A","--only","B","file.yaml"},          []string{"A","B"},     false},
        {"whitespace trimmed", []string{"--only","  Get user  ","file.yaml"},        []string{"Get user"}, false},
        {"missing value", []string{"--only"},                                         nil,                   true},
    }
    // ...
}

func TestRun_OnlyNoMatch(t *testing.T) {
    // Fixture: collection with two main requests, neither matching --only "Nope".
    // Expect exit code 3. Expect stderr to contain `no request named "Nope"` and `available: "A", "B"`.
    // Expect zero HTTP requests sent (spy executor count == 0).
}

func TestRunCmd_only_filter_integration(t *testing.T) {
    // httptest.Server recording hits. Collection with setup + 3 main + teardown.
    // --only "B": observe setup hit + B hit + teardown hit, in that order.
}

func TestEvents_v11_RunStart_carriesSelection(t *testing.T) {
    // End-to-end via runCmdInner: --only X --only Y --events ev.ndjson → first line's selection is ["X","Y"].
}

func TestWatch_OnlyPropagates(t *testing.T) {
    // Minimal: watchCmdOut called with --only; inspect the Args field on a fake watch.Config RunFunc; assert "--only" and the value both appear in the passthrough. (Verifies no plumbing change is needed.)
}
```

#### Impact on Existing Tests

- `parseRunArgs` tests: zero impact; new flag branches fall through default path on existing fixtures.
- Help output snapshot tests (`TestStreamHelp`, `printHelp` golden): update golden to include the new `--only` line.
- Usage-string tests (if any asserting the exact Usage: synopsis): `grep -rn "curlew run" cmd/curlew/*_test.go` to audit. The Usage line is long; safer to assert the substring `[--only "<name>"]` rather than re-spell the whole line.

---

### Step 6: Docs

**Rationale:** Trails code — documenting actual landed behaviour.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/SPECIFICATION.md` | modify | Extend the run command section (near the other flags listed around line 2863 / 4729 / 5102) with a `--only "<name>"` entry: name, semantics, repeatable, setup/teardown still run, no-match exit 3, interaction with --parallel. |
| `docs/MANUAL.md` | modify | In Part 4 (Running Tests in CI) or new subsection, add an "--only" example. Mention the variable-cliff hint. |
| `CHANGELOG.md` | modify | Under `## [Unreleased]`, `### Added`: "CLI: `--only "<name>"` flag on `curlew run` … (M8-004)". Under `### Changed`: "Events schema promoted from v1.0 to v1.1 (additive): `run.start` gains an optional `selection` field carrying --only values … (M8-004)". |

No code impact.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| `internal/parser/parser_test.go` | (new) `TestParser_DuplicateNameRejection` | new | Create in Step 1. |
| `internal/parser/parser_test.go` | `TestParseFile` | none expected | Audit at execute time; fixtures do not currently share main names. |
| `internal/parser/include_*_test.go` | various | audit | Rename any duplicate main-request fixtures introduced for other purposes. |
| `internal/runner/runner_test.go` | (new) `TestRunner_OnlyFilter` | new | Create in Step 2. |
| `internal/runner/runner_test.go` | (new) `TestRun_OnlyVariableCliff` | new | Create in Step 3. |
| `internal/runner/runner_test.go` | (new) `TestRunner_UndefinedVarMessageFormatStable` | new | Create in Step 3 (regex format guard). |
| `internal/runner/runner_test.go` | existing tests using `VarSources` | none | `Selection` defaults to nil; all pass unchanged. |
| `internal/output/events/emitter_test.go` | any test literal-matching `"schema_version":"1.0"` | breaks | Update string literals to `"1.1"`. |
| `internal/output/events/emitter_test.go` | (new) `TestEvents_v11_Selection` | new | Create in Step 4. |
| `internal/output/events/schema_test.go` | `schemaPath`, `eventSchemaDocPath` helpers | breaks | Switch to `v1.1.json` / `EVENTS_SCHEMA_v1.1.md`. |
| `internal/output/events/schema_test.go` | `TestSchema_DocInSyncWithCode` | breaks | After adding `Selection []string` to `RunStart` and adding the schema property, this asserts they stay aligned. |
| `internal/output/events/schema_test.go` | `TestSchema_MarkdownExamplesValidate` | breaks | Update test to read `EVENTS_SCHEMA_v1.1.md` instead; examples therein must match v1.1 schema. |
| `internal/output/events/schema_test.go` | `TestSchema_v01ArtifactsRetained` | none | Keep. |
| `internal/output/events/schema_test.go` | (new) `TestSchema_v10ArtifactsRetained` | new | Mirror of v0.1 retention guard. |
| `internal/output/events/schema_test.go` | (new) `TestSchema_v11_validates` | new | Validates all emitted kinds against the v1.1 schema. |
| `internal/output/events/testdata/golden/*.ndjson` | `TestEmitter_GoldenRun*` | golden updates | Regenerate with `UPDATE_GOLDEN=1` after SchemaVersion bump. |
| `cmd/curlew/main_test.go` | (new) `TestParseRunArgs_Only` | new | Flag parsing. |
| `cmd/curlew/run_test.go` | (new) `TestRun_OnlyNoMatch` | new | Exit 3, stderr content. |
| `cmd/curlew/run_test.go` | (new) `TestRunCmd_only_filter_integration` | new | End-to-end with httptest.Server. |
| `cmd/curlew/run_test.go` | (new) `TestEvents_v11_RunStart_carriesSelection` | new | End-to-end event emission. |
| `cmd/curlew/stream_help_test.go` | `TestStreamHelp` | possibly breaks | Audit help-text golden; extend with new `--only` line if necessary. |
| `cmd/curlew/validate_team_test.go` / validator tests | existing | no direct impact | Duplicate rejection inherited via `parser.ParseFile`; new fixture added in Step 1 exercises it. Optionally add `TestValidate_DuplicateRejected` (exit 3, line-numbered error). |
| `cmd/curlew/testdata/**` fixtures | various | audit | Search for any fixture with duplicate main-request names; rename or delete. |

---

## Risks and Edge Cases

- **Risk: Include-based fixtures with intentional duplicate names** → Mitigation: audit `internal/parser/testdata/include_*.yaml`, `cmd/curlew/testdata/**`, and `smoke/**` for fixtures that intentionally share main-request names. Estimated low probability (all existing include tests focus on variable propagation, not name collision). If found, rename one occurrence and add a comment.
- **Risk: Help-text snapshot tests breaking on the new `--only` line** → Mitigation: audit `TestStreamHelp` and any other golden-help assertions at execute time. Prefer substring assertions over equality.
- **Risk: Format drift in the undefined-variable error message breaks the variable-cliff enricher silently** → Mitigation: `TestRunner_UndefinedVarMessageFormatStable` locks the contract with a clear failure message pointing to the regex.
- **Risk: Events schema goldens get stale when fields are added in unrelated future work** → Mitigation: unchanged from current practice (UPDATE_GOLDEN=1 dance). Documented in `compareOrUpdateGolden`.
- **Risk: Parallel analyzer behaves differently on a filtered subset than on the full graph** → Mitigation: `parallel.Analyze` is stateless over its items slice (confirmed by reading `internal/parallel/analyze.go` and `scan.go`). No change to the analyzer is required; the behaviour change is intentional and matches the spec's "parallel execution on a filtered subset re-runs parallel.Analyze". Add a parallel-mode slice of `TestRunner_OnlyFilter` to exercise this explicitly.
- **Risk: Shallow-copying the collection inside Run could surprise a caller that expects the original pointer to be mutated** → Mitigation: no existing caller mutates the collection post-`Run`. Comment the shallow copy explicitly. Setup/teardown remain shared references (we only read them).
- **Risk: Reordering the filter before `buildScope` vs after** → Decision: filter runs AFTER `buildScope` so that auth profiles still execute (their semantics are independent of main-phase filtering). Documented in the inline comment.
- **Risk: Events schema test `TestSchema_MarkdownExamplesValidate` reads the v1.0 doc currently** → Mitigation: explicitly switch to v1.1 doc in Step 4; v1.0 doc's examples continue to use `"1.0"` schema_version and are not re-validated (they are a historical artefact).
- **Edge case: empty `--only ""`** → After `TrimSpace`, the name is `""`. The empty string will never match any item with `Name != ""`, triggering the no-match error. Acceptable. Add a test case.
- **Edge case: `--only` names a data-driven request** → All iterations run (per spec). No special code; `executeDataDriven` inherits the filtered item.
- **Edge case: `--only` names a WebSocket request** → Handled via the existing WebSocket branch in `executePhase` / `buildWebSocketFunc`; no filter-aware code needed beyond Step 2.
- **Edge case: Repeated `--only A --only A`** → Deduplicated implicitly (set semantics in `filterMainItemsBySelection`). Recorded in `events.run.start.selection` as-provided (no dedup in the event — matches Zinsser-style "record what the user typed" philosophy).
- **Edge case: `--only` with `--dry-run --show-dependencies`** → `showDeps` path runs `parallel.Analyze` over `col.Requests.Items`. Decision: **filter before `parallel.Analyze` in the `--show-dependencies` path too**, so the graph visualisation matches the run-time semantics. Add the filter in `runCmdInner` at the show-deps branch with a one-line helper call. Test: verify waves-render exercises the filter.
- **Edge case: `--only` with `--parallel`** → Already covered by Step 2's shallow-copy; parallel path uses the same filtered `col.Requests.Items`.

---

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh --go
```

Observable verification (matches the task YAML `observable` field verbatim):

```bash
# Build
go build -o curlew ./cmd/curlew

# 1. --only runs exactly one named main request; setup + teardown still run.
./curlew run collections/multi.yaml --only "Get user"

# 2. Repeatable --only unions the selection.
./curlew run collections/multi.yaml --only "Get user" --only "Update user"

# 3. Unknown name: exit 3 before any HTTP runs.
./curlew run collections/multi.yaml --only "Nope"

# 4. Variable-cliff diagnostic.
./curlew run collections/multi.yaml --only "Update user"

# 5. Duplicate request names rejected.
./curlew run collections/dupes.yaml
./curlew validate collections/dupes.yaml

# 6. Events selection field.
./curlew run collections/multi.yaml --only "Get user" --events run.ndjson
jq -c 'select(.kind == "run.start") | {schema_version, selection}' run.ndjson

# 7. Watch mode passthrough.
./curlew watch collections/multi.yaml --only "Get user"

# 8. Full unit + integration suite.
go test -run 'TestParser_DuplicateNameRejection|TestRunner_OnlyFilter|TestRun_OnlyNoMatch|TestRun_OnlyVariableCliff|TestEvents_v11_Selection|TestSchema_v11_validates' ./...
```
