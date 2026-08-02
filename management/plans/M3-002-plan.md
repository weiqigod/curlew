# Implementation Plan: M3-002

## Overview
Add glob pattern discovery to `curlew run` as a Professional-tier feature. A new
`internal/discovery` package expands patterns like `**/*_test.yaml` into a
deterministic, ignore-aware list of collection files, which `runCmd` executes
sequentially and aggregates into a single summary (terminal, JSON, TAP, JUnit, or
HTML).

## Task Details
- **ID:** M3-002
- **Title:** Glob pattern discovery for curlew run
- **Phase:** M3: Professional Tier
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| — | (none) | — |

## Architectural Decisions

Decisions made during planning (documented to resolve open questions without
user confirmation, per pipeline mode):

1. **No new third-party dependency.** The task YAML mentions `doublestar` as an
   option, but the project's `TECH_CHOICES.md` / standard-library-first rule
   (reiterated in `CLAUDE.md`) favors avoiding external glob libraries when a
   hand-rolled walker + `filepath.Match` can handle the needed surface.
   We implement a minimal `**`-aware matcher internally (roughly 80 LOC).
   The matcher supports `*`, `?`, `[...]`, and `**` (multi-segment wildcard)
   on the relative path of each walked file. This is the subset required by
   the behaviors; we document the supported syntax in a package doc comment.
2. **Glob detection rule.** A path is treated as a glob if it contains any of
   `*`, `?`, or `[`. A literal existing file path bypasses discovery
   completely — preserving today's single-file behavior (a behavior
   explicitly required by the task).
3. **Safety rule: reject `..` in the pattern.** Directory traversal outside
   the working directory is rejected with `ErrTraversalOutsideRoot`
   before any filesystem walk. Absolute patterns are also rejected.
4. **Walk root:** When the pattern is a glob, the discovery root is the
   current working directory (`.`). The pattern is always interpreted as
   relative to that root. We resolve CWD once up-front.
5. **`.curlewignore` location:** We look for `.curlewignore` at the CWD
   (the walk root). Ignore patterns use the same matcher as the primary
   glob. Behavior mirrors `.gitignore`: blank lines and `#` comments are
   skipped, patterns are matched against the relative path.
6. **Aggregated summary:** Rather than invent a new cross-collection schema,
   we call the existing `runner.Run` once per matched file, then fold each
   per-collection `runner.Summary` into an aggregate `runner.Summary`
   (sum of counts, max guard rail, OR of boolean flags) and also into a
   new `output.MultiJSONOutput` that wraps a slice of per-collection
   `output.JSONOutput` entries. Terminal output renders each collection
   with its existing header/summary, then prints a final combined summary
   line.
7. **Exit-code aggregation:** The worst (highest severity) per-collection
   exit code wins — but with severity ordering:
   `0 < 4 < 2 < 5 < 3 < 1 < 6`. Rationale: a single assertion failure (1)
   should dominate a lone network error (4) because the user cares most
   about contract breakage; but guard-rail (2) and config errors (5) are
   elevated above network errors, and feature-gate (6) / collection error
   (3) / assertion failure (1) are fatal classes. This matches behaviors
   "A and C still run and the final exit code reflects the failure" and
   "exit code reflects the worst collection result".
8. **Feature gate timing:** Gate check happens in `parseRunArgs`/`runCmdInner`
   **after** arg parsing but **before** any file I/O, so Free-tier users
   receive exit code 6 immediately (matches behavior
   "before any file I/O"). Gate message uses the existing
   `output.GateJSONOutput` / `errOut.StructuredError` path so JSON/TAP/etc.
   keep consistent feature-gate output.
9. **Per-collection context:** Each collection is parsed with its own
   `collectionDir` (the directory of the file), so relative paths inside
   each collection resolve as they do today. Environments / `curlew.yaml` /
   `.env` are re-discovered per collection (mirrors existing single-file
   semantics). This is expensive but correct — caching is out of scope
   for this slice.
10. **`--env` / `--report` with multi-collection:** For `--report <file>`
    with a glob, a single combined report is written (JUnit merges all
    testsuites, HTML produces one merged report). JSON produces a single
    document containing a `collections: []` array. This keeps CI
    integrations simple.
11. **Zero-match behavior:** Exit code 2 (guard-rail/"unusable" class) with
    a `no collections matched pattern X` error. The task states "exit code
    2 and a 'no collections matched' error" explicitly.

## Implementation Steps

### Step 1: Create `internal/discovery` package (core matcher + walker)
**Rationale:** Pure-Go, no dependencies on other `internal/` packages,
fully unit-testable in isolation. Smallest blast radius — building it first
unblocks every subsequent integration step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/discovery/discovery.go` | create | Public `Expand`, `ErrNoMatches`, `ErrTraversalOutsideRoot`, `ErrAbsolutePattern`, `IsGlob`, `LoadIgnore`. |
| `internal/discovery/match.go` | create | Internal `matchPattern(pattern, path string) bool` supporting `*`, `?`, `[...]`, and `**`. |
| `internal/discovery/discovery_test.go` | create | Table-driven tests for `Expand`, glob detection, ignore semantics, traversal rejection, sort order. |
| `internal/discovery/match_test.go` | create | Table-driven tests for the matcher covering `**`, `*`, `?`, `[...]`, and edge cases. |
| `internal/discovery/testdata/` | create | Fixture tree used by discovery_test (a.yaml, b.yaml, sub/c.yaml, drafts/d.yaml, ignored.txt, .curlewignore). |

#### New Code (sketch)

```go
// Package discovery expands glob patterns into collection file paths,
// honoring .curlewignore and rejecting unsafe patterns.
//
// Supported glob syntax (relative to the walk root):
//   *        matches any sequence of non-separator characters
//   ?        matches any single non-separator character
//   [abc]    character class (as in filepath.Match)
//   **       matches zero or more path segments (including separators)
package discovery

import "errors"

var (
    // ErrNoMatches is returned when a glob pattern matches zero files.
    ErrNoMatches = errors.New("no collections matched pattern")
    // ErrTraversalOutsideRoot is returned when a pattern contains "..".
    ErrTraversalOutsideRoot = errors.New("pattern escapes working directory")
    // ErrAbsolutePattern is returned when an absolute path is used as a glob.
    ErrAbsolutePattern = errors.New("absolute glob patterns are not supported")
)

// IsGlob reports whether pattern contains glob metacharacters.
func IsGlob(pattern string) bool

// Expand walks root and returns all files matching pattern, sorted
// deterministically, with .curlewignore rules applied. Returns
// ErrNoMatches when nothing matches.
func Expand(root, pattern string) ([]string, error)

// LoadIgnore reads .curlewignore at root (if present) and returns
// its non-comment, non-blank patterns.
func LoadIgnore(root string) ([]string, error)
```

```go
// internal/discovery/match.go
package discovery

// matchPattern reports whether path matches pattern. Pattern supports
// *, ?, [...], and **. path uses forward slashes regardless of OS.
func matchPattern(pattern, path string) bool
```

#### Tests to Write FIRST (RED phase)

`discovery_test.go`:

```go
func TestIsGlob(t *testing.T) {
    tests := []struct {
        name    string
        pattern string
        want    bool
    }{
        {"literal path", "testdata/a.yaml", false},
        {"star", "*.yaml", true},
        {"double star", "**/*.yaml", true},
        {"question mark", "a?.yaml", true},
        {"char class", "a[12].yaml", true},
        {"empty", "", false},
    }
    // ...
}

func TestExpand(t *testing.T) {
    tests := []struct {
        name    string
        pattern string
        want    []string
        wantErr error
    }{
        {"matches all yaml recursively", "**/*.yaml", []string{
            "a.yaml", "b.yaml", "sub/c.yaml",
        }, nil},
        {"matches top level only", "*.yaml", []string{
            "a.yaml", "b.yaml",
        }, nil},
        {"excludes ignored drafts directory", "**/*.yaml", []string{
            "a.yaml", "b.yaml", "sub/c.yaml",
        }, nil}, // drafts/d.yaml excluded via .curlewignore
        {"zero matches returns ErrNoMatches", "**/*.nomatch", nil, ErrNoMatches},
        {"traversal rejected", "../etc/passwd", nil, ErrTraversalOutsideRoot},
        {"absolute rejected", "/etc/*.yaml", nil, ErrAbsolutePattern},
        {"deterministic sort order", "**/*.yaml", []string{
            "a.yaml", "b.yaml", "sub/c.yaml",
        }, nil}, // run twice, verify identical order
    }
    // Each subtest changes into testdata/ and calls Expand(".", tt.pattern)
}

func TestLoadIgnore(t *testing.T) {
    tests := []struct {
        name     string
        contents string
        want     []string
    }{
        {"skips blank and comment lines",
            "# comment\n\n**/drafts/*.yaml\n\n",
            []string{"**/drafts/*.yaml"}},
        {"missing file returns empty",
            "", nil},
    }
    // ...
}
```

`match_test.go`:

```go
func TestMatchPattern(t *testing.T) {
    tests := []struct {
        name    string
        pattern string
        path    string
        want    bool
    }{
        {"star matches single segment", "*.yaml", "a.yaml", true},
        {"star does not cross separator", "*.yaml", "sub/a.yaml", false},
        {"double star matches zero segments", "**/*.yaml", "a.yaml", true},
        {"double star matches multiple segments", "**/*.yaml", "x/y/z.yaml", true},
        {"double star standalone", "**", "a/b/c", true},
        {"question mark matches one char", "a?.yaml", "ab.yaml", true},
        {"question mark does not match empty", "a?.yaml", "a.yaml", false},
        {"char class", "a[12].yaml", "a1.yaml", true},
        {"char class miss", "a[12].yaml", "a3.yaml", false},
        {"prefix match", "sub/**", "sub/a/b.yaml", true},
        {"suffix match", "**/c.yaml", "sub/c.yaml", true},
    }
    // ...
}
```

#### Impact on Existing Tests
- None. New package.

---

### Step 2: Register `test_discovery` in the auth registry
**Rationale:** Trivial, isolated change that unblocks the gate check in Step 4.
Doing it before CLI wiring keeps each commit green.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Register `test_discovery` at `TierProfessional`. |
| `internal/auth/registry_test.go` | modify | Add a case verifying `test_discovery` presence and tier. |

#### Current Code
```go
r.Register(FeatureDefinition{
    Name:         "rate_limit_global",
    RequiredTier: TierProfessional,
    Description:  "Global rate_limit_rps requires Professional tier ($19/month)",
    Workaround:   "Use per-request pacing in your own scripts, or the data-driven rate_limit_rps for single requests",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "rate_limit_global",
    RequiredTier: TierProfessional,
    Description:  "Global rate_limit_rps requires Professional tier ($19/month)",
    Workaround:   "Use per-request pacing in your own scripts, or the data-driven rate_limit_rps for single requests",
})
r.Register(FeatureDefinition{
    Name:         "test_discovery",
    RequiredTier: TierProfessional,
    Description:  "Glob pattern test discovery requires Professional tier ($19/month)",
    Workaround:   "Pass each collection file explicitly, or script the expansion with your shell",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
func TestDefaultRegistry_TestDiscovery(t *testing.T) {
    r := DefaultRegistry()
    def, ok := r.Lookup("test_discovery")
    if !ok {
        t.Fatal("test_discovery not registered")
    }
    if def.RequiredTier != TierProfessional {
        t.Errorf("test_discovery RequiredTier = %s, want %s",
            def.RequiredTier, TierProfessional)
    }
}
```

Also add `{"contains test_discovery", "test_discovery", true}` to the
table in `TestDefaultRegistry`.

#### Impact on Existing Tests
- `TestDefaultRegistry` table gains one row; existing rows unaffected.

---

### Step 3: Aggregation helpers in `cmd/curlew`
**Rationale:** Pure functions with no global state — can be unit tested
against in-memory `runner.Summary`/`output.JSONOutput` without touching
I/O. Establishing them before the CLI wiring keeps Step 4 focused on
orchestration.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/discovery_run.go` | create | Aggregation helpers: `aggregateSummaries`, `aggregateExitCodes`, `buildMultiJSONOutput`, etc. |
| `cmd/curlew/discovery_run_test.go` | create | Unit tests for each helper. |
| `internal/output/json.go` | modify | Add `MultiJSONOutput` and `WriteMultiJSON`. |
| `internal/output/json_test.go` | modify | Add a round-trip test for `MultiJSONOutput`. |

#### New Code (sketch)

```go
// cmd/curlew/discovery_run.go
package main

import (
    "github.com/weiqigod/curlew/internal/runner"
)

// collectionOutcome holds the fully-rendered result of running one
// collection within a glob batch.
type collectionOutcome struct {
    Path     string
    Name     string
    ExitCode int
    Summary  *runner.Summary
    // jsonDoc is non-nil when --format json is in effect.
    jsonDoc  *output.JSONOutput
}

// aggregateSummaries folds per-collection summaries into one.
func aggregateSummaries(items []*runner.Summary) *runner.Summary

// worseExitCode returns the more-severe of two exit codes using the
// severity ordering 0 < 4 < 2 < 5 < 3 < 1 < 6.
func worseExitCode(a, b int) int

// aggregateExitCodes reduces per-collection codes to a single code.
func aggregateExitCodes(codes []int) int
```

```go
// internal/output/json.go (addition)

// MultiJSONOutput wraps multiple per-collection JSON outputs into a
// single document produced by `curlew run <glob>`.
type MultiJSONOutput struct {
    Status      string       `json:"status"`
    DurationMs  int64        `json:"duration_ms"`
    Collections []JSONOutput `json:"collections"`
    Total       int          `json:"total_collections"`
    Passed      int          `json:"passed_collections"`
    Failed      int          `json:"failed_collections"`
}

// WriteMultiJSON serializes a MultiJSONOutput.
func WriteMultiJSON(w io.Writer, out *MultiJSONOutput) error
```

#### Tests to Write FIRST (RED phase)

```go
func TestWorseExitCode(t *testing.T) {
    tests := []struct {
        name string
        a, b int
        want int
    }{
        {"both zero", 0, 0, 0},
        {"assertion beats network", 1, 4, 1},
        {"feature gate beats all", 6, 1, 6},
        {"collection error beats guard rail", 3, 2, 3},
        {"guard rail beats network", 2, 4, 2},
        {"config error beats guard rail", 5, 2, 5},
    }
    // ...
}

func TestAggregateSummaries(t *testing.T) {
    tests := []struct {
        name string
        in   []*runner.Summary
        want runner.Summary
    }{
        {"single passing", /* ... */},
        {"two summaries sum counts", /* ... */},
        {"any limit_exceeded wins", /* ... */},
        {"empty input returns zero", []*runner.Summary{}, runner.Summary{}},
    }
    // ...
}

func TestAggregateExitCodes_WorstWins(t *testing.T) {
    if got := aggregateExitCodes([]int{0, 1, 0}); got != 1 {
        t.Errorf("got %d, want 1", got)
    }
}
```

```go
func TestWriteMultiJSON(t *testing.T) {
    // encode, decode, assert.
}
```

#### Impact on Existing Tests
- None. Strict additions.

---

### Step 4: Wire glob detection into `runCmdInner`
**Rationale:** Largest blast radius — touches the single command entry
point and must preserve every existing flag/behavior. Guarded by the
discovery path being entered only when `discovery.IsGlob(file)` is true,
so literal-path callers are unaffected.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | In `runCmdInner`, if positional arg is a glob, gate-check `test_discovery`, call `discovery.Expand`, loop over matches, aggregate results and exit codes. Help text updated. |
| `cmd/curlew/main_test.go` | modify | Integration-level tests: glob matches 3 files, zero-match error, traversal rejected, literal path unchanged, Free-tier gate. |
| `cmd/curlew/testdata/discovery/` | create | Fixture tree: `a_test.yaml`, `b_test.yaml`, `sub/c_test.yaml`, `ignore.yaml`, `.curlewignore`. |

#### Current Code (runCmdInner, around line 234)
```go
col, err := parser.ParseFile(file)
if err != nil {
    if format == "json" {
        jsonOut := buildJSONOutput("", nil, &runner.Summary{}, err, output.VerbosityDefault)
        _ = output.WriteJSON(os.Stdout, jsonOut)
        return 3, nil
    }
    // ...
}
```

#### New Code (inserted immediately after `parseRunArgs` and early format/gate checks, around line 230)
```go
// Glob-pattern discovery: if the positional argument contains glob
// metachars, run test_discovery (Professional tier) instead of the
// single-file path.
if discovery.IsGlob(file) {
    reg := auth.DefaultRegistry()
    if gateErr := auth.CheckFeature(reg, "test_discovery", currentTier()); gateErr != nil {
        writeGateForFormat(os.Stdout, os.Stderr, format, report, noColor, gateErr)
        return 6, nil
    }
    cwd, cwdErr := os.Getwd()
    if cwdErr != nil {
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        errOut.StructuredError(fmt.Errorf("cannot resolve working directory: %w", cwdErr))
        return 3, nil
    }
    matches, expandErr := discovery.Expand(cwd, file)
    if expandErr != nil {
        errOut := output.NewPrinter(os.Stderr, shouldUseColor(os.Stderr, noColor))
        errOut.StructuredError(expandErr)
        switch {
        case errors.Is(expandErr, discovery.ErrNoMatches):
            return 2, nil
        case errors.Is(expandErr, discovery.ErrTraversalOutsideRoot),
             errors.Is(expandErr, discovery.ErrAbsolutePattern):
            return 1, nil
        default:
            return 3, nil
        }
    }
    return runDiscoveredCollections(matches, args, envName, format, report,
        cliVars, envVarVars, seed, noColor, verbosity, allowSensitive,
        showDeps, dryRun, runParallel, confirmLargeDS)
}

col, err := parser.ParseFile(file)
// ... (unchanged)
```

A new helper `runDiscoveredCollections` factors the per-collection
execution loop. For each match:
1. Call `runCmdInner` via a refactored `runSingleCollection(path, ...)`
   that takes already-parsed args and returns `(exitCode int, summary
   *runner.Summary, jsonDoc *output.JSONOutput)`.
2. Accumulate outcomes.
3. After the loop, emit aggregated output per format and return the
   aggregated exit code via `aggregateExitCodes`.

This means we refactor the bulk of `runCmdInner` body (from `parser.ParseFile`
onward) into `runSingleCollection(path string, parsed runArgs) (...)`. The
public surface (`runCmd`, `runCmdInner`) remains unchanged.

`writeGateForFormat` is a small helper that consolidates the
format-dispatching already duplicated in `runCmdInner` for
`parallel_execution`.

#### Help Text Update
Add a new line to `printHelp`:
```
fmt.Println("  curlew run <pattern>   Run all collections matching a glob (Professional tier)")
```
and in Run Options:
```
fmt.Println("                          Glob patterns support *, ?, [abc], and ** (Professional tier)")
fmt.Println("                          Honors .curlewignore in the working directory")
```

#### Tests to Write FIRST (RED phase)

```go
func TestRunCmd_GlobDiscovery_ThreeCollections(t *testing.T) {
    // Build 3 collection files; override httpexec; assert exit 0 and
    // that each collection's "header" line appears in stdout.
}

func TestRunCmd_GlobDiscovery_ZeroMatches(t *testing.T) {
    // Assert exit code 2 and "no collections matched" on stderr.
}

func TestRunCmd_GlobDiscovery_LiteralPathUnchanged(t *testing.T) {
    // "testdata/single.yaml" (no metachars) should bypass discovery
    // entirely — assert by verifying a stub discovery.Expand is NOT called.
    // (Done by making discovery.Expand package-level var and overriding
    // to panic in this test.)
}

func TestRunCmd_GlobDiscovery_TraversalRejected(t *testing.T) {
    // "../**/*.yaml" → exit 1 and "escapes working directory" error.
}

func TestRunCmd_GlobDiscovery_FreeTierGate(t *testing.T) {
    // Override currentTier to TierFree; assert exit 6 and no file I/O
    // (achieved by using a glob pattern pointing at a nonexistent root
    // — if discovery ran, we'd see ErrNoMatches not the gate error).
}

func TestRunCmd_GlobDiscovery_MiddleFailureDoesNotAbort(t *testing.T) {
    // Three collections, middle one has failing assertion; assert all
    // three executed (httpexec stub counts calls) and exit code is 1.
}

func TestRunCmd_GlobDiscovery_JSONFormat(t *testing.T) {
    // --format json → single document with collections: [] having 3 entries.
}

func TestRunCmd_GlobDiscovery_Ignored(t *testing.T) {
    // .curlewignore with **/drafts/*.yaml; assert drafts file excluded.
}
```

#### Impact on Existing Tests
- `TestRunCmd_*` literal-file tests in `cmd/curlew/main_test.go`:
  no change expected because `IsGlob("foo.yaml") == false`.
- The refactor of `runCmdInner` body into `runSingleCollection` must
  preserve every existing exit-code branch. Risk: any code path that
  previously returned from `runCmdInner` must return from the helper
  identically. Mitigation: the refactor is a rename-and-wrap, not a
  rewrite — we move the body into a new function and call it once,
  verifying all existing `cmd/curlew` tests still pass before adding
  glob tests.
- `run_test.go` may need adjustment only if its signatures reference the
  (private) helper — we keep them green as a pre-condition of Step 4.

---

### Step 5: Smoke test + observable verification fixture
**Rationale:** Comes last — requires both code paths to exist. This step
fulfills the `observable` section of the task YAML.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/discovery/a_test.yaml` | create | Minimal httpbin GET collection. |
| `testdata/discovery/b_test.yaml` | create | Minimal httpbin GET collection. |
| `testdata/discovery/sub/c_test.yaml` | create | Minimal httpbin GET collection. |
| `testdata/discovery/ignore.yaml` | create | Non-matching fixture. |
| `smoke/run.sh` | modify | Append discovery smoke tests (Professional tier override + Free tier gate). |
| `CHANGELOG.md` | modify | Add M3-002 entry under Unreleased. |

#### Smoke Additions
```bash
echo "--- Discovery: glob matches 3 collections (Professional tier) ---"
CURLEW_TIER=professional ./curlew run "testdata/discovery/**/*_test.yaml" \
  && echo "Pass: exit 0" || echo "Exit: $?"

echo "--- Discovery: free-tier gate (expect exit 6) ---"
./curlew run "testdata/discovery/**/*_test.yaml" \
  && echo "ERROR: expected gate" || echo "Exit: $?"
```

The Professional tier override is achieved via an existing `CURLEW_TIER`
env var hook — if one does not yet exist, we add a one-liner in
`currentTier()` that reads it (documented as test-only). This is the
same mechanism used by existing tier-gated smoke tests.

#### Tests to Write FIRST (RED phase)
- None (smoke is verification, not unit test).

#### Impact on Existing Tests
- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | extends table | add row for `test_discovery` |
| `internal/auth/registry_test.go` | new | adds case | `TestDefaultRegistry_TestDiscovery` |
| `internal/discovery/*_test.go` | new | adds package | implement per Step 1 |
| `cmd/curlew/discovery_run_test.go` | new | adds aggregation helpers | implement per Step 3 |
| `cmd/curlew/main_test.go` | new tests | adds integration | implement per Step 4 |
| `cmd/curlew/main_test.go` | existing `TestRunCmd_*` | unchanged | verify still green after Step 4 refactor |
| `internal/output/json_test.go` | new | adds MultiJSONOutput round-trip | implement per Step 3 |

## Risks and Edge Cases

- **Risk — matcher bugs with `**` semantics.**
  **Mitigation:** Extensive table-driven tests covering zero-segment,
  single-segment, multi-segment, leading, trailing, and middle positions.
  Cross-check against `doublestar` semantics in comments.

- **Risk — refactoring `runCmdInner` silently breaks an existing exit
  code branch.**
  **Mitigation:** Move the body wholesale into `runSingleCollection`
  without editing the logic; verify all existing `cmd/curlew` tests pass
  as a pre-commit gate before adding glob tests.

- **Risk — Windows path separators.**
  **Mitigation:** Normalize all walked paths to forward slashes before
  matching. Document that patterns are forward-slash-only in the package
  doc comment. Tests use forward slashes throughout.

- **Edge case — symlink loops.**
  **Handling:** Use `filepath.WalkDir`, which does not follow symlinks
  into directories. Document this as a constraint.

- **Edge case — `**` matching hidden directories.**
  **Handling:** Skip directory entries whose name starts with `.`
  (matches `.git`, `.curlew/`, `node_modules`-style conventions only
  if user opts in via pattern). Document this and provide an ignore-
  list escape hatch via `.curlewignore`.

- **Edge case — `.curlewignore` missing.**
  **Handling:** `LoadIgnore` returns `nil, nil`; `Expand` proceeds.

- **Edge case — pattern matches directories, not files.**
  **Handling:** Discovery only returns files, not directories.

- **Edge case — very large match sets.**
  **Mitigation:** Out of scope. Document that discovery is intended for
  reasonable project sizes (hundreds, not millions, of files). No
  streaming.

- **Edge case — one collection's guard-rail limit bleeds into the next.**
  **Handling:** Guard-rail counter is per-`runner.Run` invocation, so each
  collection has its own budget. Aggregated `Summary.LimitExceeded` is
  `true` if any single collection tripped it; aggregated
  `RequestsExecuted` is the sum. Documented in aggregator unit test.

- **Edge case — Free-tier user runs with `--format json`.**
  **Handling:** Gate error flows through `writeGateForFormat` so the JSON
  response is the structured `GateJSONOutput`, matching existing
  `parallel_execution` behavior.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
mkdir -p testdata/discovery/sub
cat > testdata/discovery/a_test.yaml <<'YAML'
name: A
requests:
  - name: Get
    request: { method: GET, url: "https://httpbin.org/get" }
    assertions: { status: 200 }
YAML
cp testdata/discovery/a_test.yaml testdata/discovery/b_test.yaml
cp testdata/discovery/a_test.yaml testdata/discovery/sub/c_test.yaml
cat > testdata/discovery/ignore.yaml <<'YAML'
name: ignored
requests: []
YAML

go build ./cmd/curlew

# Professional tier: three collections executed, exit code reflects worst
CURLEW_TIER=professional ./curlew run "testdata/discovery/**/*_test.yaml"
echo "exit: $?"

# Free tier: exit code 6 + test_discovery feature gate message
./curlew run "testdata/discovery/**/*_test.yaml"
echo "exit: $?"

# Unit tests
go test ./internal/discovery/...
```
