# Implementation Plan: M6-007

## Overview
Build a new `cmd/apitest-agent-harness/` Go test binary that drives the real `apitest run --events` against a curated set of deliberately-broken fixture collections, asserts each failure event satisfies the agent-diagnosability contract (non-empty `category`, `code`, `file`, `line`, and a hint with a concrete-action verb), and once all scenarios pass, atomically promotes the event-stream schema from `0.1` to `1.0`. This is the gate task that converts the v0.x stream into a publicly-stable v1.0 contract — the only task that mutates `events.SchemaVersion`.

## Task Details
- **ID:** M6-007
- **Title:** Validation harness: gate for v0.1 → v1.0 schema promotion
- **Phase:** M6: AI Agent Integration
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-005 | Wire --events flag into run subcommand end-to-end | done |
| M6-006 | Event schema documentation and stability policy | done |

---

## Architectural decisions (resolved)

These decisions are made up-front so the harness implementation is unambiguous.

### D1: Harness lives in `cmd/apitest-agent-harness/` as a Go test package, not a runnable binary

The task wording says "Go test binary, excluded from release builds by a build tag." Two valid interpretations exist:
- **(a)** A package that compiles to an actual binary plus tests that exercise it.
- **(b)** A test-only package (no `main.go`) under `cmd/` whose `*_test.go` files build and exec the real `apitest` binary.

I pick **(b)** because:
1. The observable line is `go test ./cmd/apitest-agent-harness/...`, not a binary invocation.
2. We already have `buildBinary(t)` / `runBinary(t,…)` helpers in `cmd/apitest/main_test.go` that build the real binary on demand; we follow the same pattern.
3. There is nothing for the harness to expose other than the test scenarios — no daemon, no CLI of its own.
4. A "build tag to exclude from release" is unnecessary for test files; they are already excluded from `go build ./cmd/apitest`. We add a stub `harness.go` file with `//go:build never` so the directory is not entirely test-only (some IDEs and tools dislike that), and we leave the entire test surface in `harness_test.go`. The build tag also makes the no-release-binary intent explicit.

### D2: Each scenario is a YAML pair: `<scenario>.yaml` (fixture) and `<scenario>.expect.yaml` (contract)

The task scope says "Each fixture is paired with an expected-output YAML describing the required shape of the failure event." We co-locate the pair under `testdata/agent-harness/<scenario>/`:
- `collection.yaml` — the fixture collection (or another base name when the input is intentionally not a collection, e.g. `not-a-collection.yaml` for `bad-yaml`).
- `expect.yaml` — the contract: which event kind, which fields must be set, which must match patterns, which CLI args/env to set up.
- Optional `env.yaml` — environment file for the run.
- Optional `apitest-args.txt` — extra CLI args (one per line, `#` comments allowed).
- Optional `setup.yaml` — instructions to start a backing server (only `unreachable-host` needs none; `failing-assertion` needs an `httptest.Server`).

This keeps each scenario a self-contained directory readable from the tree without hopping into Go code.

### D3: Expectation contract format

```yaml
# testdata/agent-harness/missing-variable/expect.yaml
description: |
  Run with a {{NEEDED}} variable but no --var flag and no env file.
  Should emit run.error with category=input, code=VAR_UNDEFINED, with file+line
  pointing at the request that referenced the variable.
exit_code: 5
events:
  - kind: run.error
    must_have:
      error.category: input
      error.code: VAR_UNDEFINED
      error.file_nonempty: true
      error.line_nonzero: true
    hint_contains_any:
      - Set
      - Add
      - Pass
      - Define
extra_args: []
allow_network: false
```

The expectation is simpler than a JSON-Schema-style match because the harness tests *agent diagnosability*, not stream conformance — schema conformance is already covered by `internal/output/events.TestEmitter_GoldenSchemaValidates`.

Each expectation entry has:
- `kind` — required, the event kind to find in the stream.
- `must_have` — a flat map of dotted JSON paths to expected values. Special suffixes:
  - `_nonempty: true` — the field must be present and non-empty.
  - `_nonzero: true` — the field must be present and non-zero (integer).
  - `_pattern: <re>` — the field's value must match the regex.
  - bare value — exact match.
- `hint_contains_any` — the event's `error.hint` must contain at least one of the listed substrings (by default the canonical action verbs from the contract: `Set`, `Add`, `Pass`, `Verify`, `Remove`, `Run`, `Define`, `Refresh`, `Check`).

### D4: `network` category is allowed `line=0`

The task spec says "non-zero line (or explicit line=0 with category=network)". Network errors don't originate from a YAML line — they originate from a network event. The harness allows line=0 only when the failing event has `error.category=network`. Other categories must have a non-zero line.

### D5: `auth-missing` scenario semantics

The spec lists `auth-missing` as a scenario. The closest existing surface is an authn profile that fails to extract a token (`AUTH_PROFILE_NO_EXTRACT`) or one that is referenced but not declared (`RUNNER_AUTH_PROFILE_NOT_FOUND`). I use **`RUNNER_AUTH_PROFILE_NOT_FOUND`** because:
- It does not require an httptest.Server (no network).
- It is the most agent-actionable failure ("Define the auth profile named in the message").
- It exercises the pre-flight auth path the spec is gating on.

The fixture references an auth profile name that is not defined in the collection.

### D6: `feature-gate-denied` uses `--format junit` on free tier

`junit_xml` is the smallest gated feature in `auth.DefaultRegistry()`. The fixture sets `APITEST_TIER=free` (env in `apitest-args.txt`) and runs with `--format junit`. This emits a `run.error` with `*GateError` — currently classified as `internal` (no sentinel match). **This is an issue we must fix as part of this task** — see D8.

### D7: `circular-include` uses two YAML files: `a.yaml` includes `b.yaml`, `b.yaml` includes `a.yaml`

Already supported by `parser.ErrCircularInclude` (`PARSE_CIRCULAR_INCLUDE`). The fixture is two files in the scenario directory.

### D8: Pre-promotion code fixes (this is part of M6-007 scope)

The task says definition_of_done #2 is "harness fails loudly when a fixture's expected-output contract is unmet (negative test included)" and the harness must pass for every scenario. Some fixtures will *initially fail* against the contract because the production code does not yet attach `category`/`code`/`file`/`line`/`hint` cleanly. Per TDD: write the harness first (RED), then fix production code until the harness goes GREEN.

Concrete pre-promotion fixes already required:

1. **`variable.ErrUndefinedVariable` → category mismatch.** `internal/variable/variable.go:245` builds `&Structured{Category: CategoryConfig, …, Inner: ErrUndefinedVariable}`. The registered hint says `CategoryInput`. Per `classify.go:138-152`, the existing Category is used as-is. Fix: change the produced category to `CategoryInput` to match the hint, or strip Category (let the registered hint fill it). I pick the first because the explicit struct read site is unambiguous.

2. **`variable.ErrUndefinedVariable` → no `FilePath`/`Line`.** The interpolation site in `variable.go` doesn't know which YAML file/line it's interpolating for. Fix: thread the request's `SourceFile`/`SourceLine` into the runner's wrap. The runner already holds `item.SourceFile`/`item.SourceLine` (see `runner.go:1066-1090`). Wrap the interpolation error in a Structured with FilePath/Line set from `item.SourceFile`/`item.SourceLine` before returning to the caller. This must happen for *all four* call sites: `runner.go:1065`, `1236`, `1731`, `1932`.

3. **`*auth.GateError` → not classified.** When run.error fires for a gated feature, the EventError carries Category=internal, no Code, no Hint. Fix: add a sentinel + a `Format`-aware classification path in `internal/errors/classify.go` so `*auth.GateError` is recognized. Since `GateError` is not a comparable sentinel (it's a typed error with a struct payload), use `errors.As` rather than `errors.Is`. We add a small extension point: `RegisterTypeClassifier(matcher func(error) bool, hint ClassifiedHint)`. Used once from `internal/auth/hints_init.go` to register the gate-error classifier.

4. **`assertion` failures emit `request.end` with `outcome=failed` but currently no `Error` field.** The task requires "an error object with category=assertion and a concrete Hint". Fix: when `Outcome=OutcomeFailed` because of an assertion (not a network error), the runner must surface an `EventError{Category: assertion, Code: ASSERTION_FAILED, Hint: "Inspect the assertion.result events for the failing request and adjust either the assertion or the request to make them agree."}`. We do this in the `eventsAdapter.RequestEnd` method when `e.Err == nil && e.Outcome == OutcomeFailed`. We also add `ErrAssertionFailed` as a sentinel in `internal/assertion` (or `internal/runner`) and register it.

5. **Network errors already have category/code/hint via `ClassifyNetworkError`.** Verified — `unreachable-host` should pass without code changes.

6. **Bad YAML / circular include — already populate FilePath/Line via `parser.Structured`.** Verified — should pass without code changes.

7. **Hint contains a concrete action verb.** Audit the seven canonical scenario hints and adjust any that don't start with one of the concrete-action markers (`Set`, `Add`, `Pass`, `Verify`, `Remove`, `Run`, `Define`, `Refresh`, `Check`). Existing hints in `internal/parser/hints_init.go`, `internal/variable/hints_init.go`, etc. already use these verbs; spot-fixes only.

### D9: Schema promotion is a single atomic commit at the end

The promotion step is only done after every fixture passes (i.e. the test would already be failing in CI if it was wrong). The promotion changes:
- `internal/output/events/events.go` — `SchemaVersion = "1.0"`.
- `docs/events-schema/v0.1.json` → `docs/events-schema/v1.0.json` (`git mv` to preserve history) and update the `$id`, `title`, and every `"const": "0.1"` to `"1.0"`. **Keep `v0.1.json` as well** (per task DoD: "v0.1 artefacts retained for history") — copy `v0.1.json` to a frozen copy before mutating. So end state: both `v0.1.json` and `v1.0.json` exist, and `v0.1.json` has a top-level `"deprecated": true`.
- `docs/EVENTS_SCHEMA_v0.1.md` → `docs/EVENTS_SCHEMA_v1.0.md` (similar pattern: keep v0.1 as a frozen historical anchor with a deprecation note at the top).
- New `docs/EVENTS_SCHEMA_v1.0.md` body documents the v0.1 → v1.0 diff explicitly (per DoD #6) — even when the diff is "no breaking changes", that needs to be stated.
- `internal/output/events/schema_test.go` — point `schemaPath` and `eventSchemaDocPath` at v1.0.
- `internal/output/events/testdata/golden/*.ndjson` — regenerate via `UPDATE_GOLDEN=1 go test ./internal/output/events/...` because `schema_version` strings change.
- The harness contracts in `testdata/agent-harness/*/expect.yaml` — update if any reference `"0.1"` literal (none should, but verify).
- Stability policy section in the new doc gets updated wording: "removals/renames now require a v2.0 bump" (per task scope line 48).

---

## Implementation Steps

Steps are ordered by blast radius (smallest first). The harness is built and tightened on the still-v0.1 schema; code fixes are made as the harness exposes gaps; promotion is the final commit.

### Step 1: Harness scaffolding and fixture loader (no production code changes)

**Rationale:** Lowest blast radius — pure new files under `cmd/apitest-agent-harness/` and `testdata/agent-harness/`. No existing code or test affected. Establishes the testbed before exercising it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest-agent-harness/harness.go` | create | `//go:build never` stub package declaration so `go build ./...` ignores the dir but `go test` finds it. |
| `cmd/apitest-agent-harness/harness_test.go` | create | Main test driver: discovers scenarios, builds binary, executes each, asserts contract. |
| `cmd/apitest-agent-harness/contract.go` | create | YAML contract loader and matcher (NOT under build tag — it is exercised by the tests). Plain Go code that parses `expect.yaml`. |
| `cmd/apitest-agent-harness/contract_test.go` | create | Unit tests for the contract loader and matcher (pure Go, no binary). |
| `testdata/agent-harness/README.md` | create | One-paragraph overview pointing at this plan and the contract format. |

#### Tests to Write FIRST (RED phase)

```go
// contract_test.go — table-driven coverage of the contract matcher.
func TestContract_Match(t *testing.T) {
    tests := []struct {
        name      string
        expect    Expectation         // parsed from YAML
        events    []map[string]any    // synthesized event stream
        wantErr   string              // empty = no mismatch; otherwise diff substring
    }{
        {"exact match", /*...*/, /*...*/, ""},
        {"missing kind", /*...*/, /*...*/, "no event with kind=run.error"},
        {"empty file when nonempty required", /*...*/, /*...*/, "error.file is empty, want non-empty"},
        {"line=0 when nonzero required", /*...*/, /*...*/, "error.line is 0, want non-zero"},
        {"hint contains none of allowed verbs", /*...*/, /*...*/, "error.hint contains none of [Set Add Pass …]"},
        {"line=0 allowed for category=network", /*...*/, /*...*/, ""},
        {"pattern mismatch", /*...*/, /*...*/, "does not match pattern"},
        {"exit code mismatch", /*...*/, /*...*/, "exit_code = 1, want 5"},
    }
    // ...
}

func TestContract_Load(t *testing.T) {
    // Loads testdata fixtures and verifies parsing.
}
```

```go
// harness_test.go — the orchestrating driver. Negative test included.
func TestHarness_AllScenarios(t *testing.T) {
    if testing.Short() { t.Skip("harness builds the apitest binary; skipping in -short mode") }
    binary := buildApitestBinary(t)              // similar to cmd/apitest/main_test.go:buildBinary
    scenarios := discoverScenarios(t, "testdata/agent-harness")
    if len(scenarios) < 7 {
        t.Fatalf("want at least 7 scenarios, got %d", len(scenarios))
    }
    for _, s := range scenarios {
        t.Run(s.Name, func(t *testing.T) {
            evPath, exitCode := runApitestForScenario(t, binary, s)
            stream := parseNDJSON(t, evPath)
            if err := s.Expect.Verify(stream, exitCode); err != nil {
                t.Fatalf("scenario %q failed contract:\n%s", s.Name, err)
            }
        })
    }
}

// Negative test: the harness must complain when an expectation is unmet.
func TestHarness_FailsLoudlyOnContractBreach(t *testing.T) {
    syntheticStream := []map[string]any{
        {"kind": "run.start", "schema_version": "0.1"},
        {"kind": "run.end",   "exit_code": 0.0},
    }
    expect := Expectation{
        ExitCode: 5,
        Events: []ExpectedEvent{{
            Kind: "run.error",
            MustHave: map[string]any{"error.category": "input"},
        }},
    }
    if err := expect.Verify(syntheticStream, 0); err == nil {
        t.Fatal("expected contract verification to fail; got nil")
    } else if !strings.Contains(err.Error(), "no event with kind=run.error") {
        t.Errorf("error = %q, want substring 'no event with kind=run.error'", err.Error())
    }
}
```

#### Impact on Existing Tests
- None. New directory, new files.

---

### Step 2: Author the seven canonical fixtures

**Rationale:** Tests in Step 1 reference `testdata/agent-harness/*/`. Authoring fixtures next means Step 1's tests can be run end-to-end (and will surface production-code gaps).

#### Files to Create

For each scenario in {`missing-variable`, `bad-yaml`, `failing-assertion`, `unreachable-host`, `auth-missing`, `feature-gate-denied`, `circular-include`}, under `testdata/agent-harness/<scenario>/`:

| File | Description |
|------|-------------|
| `collection.yaml` (or `*.yaml`) | The intentionally-broken collection. |
| `expect.yaml` | The contract per D3. |
| `args.txt` (optional) | Extra CLI args, one per line. |
| `env.txt` (optional) | Env vars to set, `KEY=VALUE` per line. |

Specific contracts per scenario:

| Scenario | kind | required category | required code | line=0 ok? |
|----------|------|-------------------|---------------|------------|
| missing-variable | run.error | input | VAR_UNDEFINED | no |
| bad-yaml | run.error | parse | PARSE_INVALID_YAML | no |
| failing-assertion | request.end (outcome=failed) + assertion.result (passed=false) | assertion | ASSERTION_FAILED | no |
| unreachable-host | request.end (outcome=error) | network | NETWORK_CONNECTION_REFUSED or NETWORK_DNS | yes |
| auth-missing | run.error | auth | RUNNER_AUTH_PROFILE_NOT_FOUND | yes (no specific source line for missing profile reference; see D5) |
| feature-gate-denied | run.error | (any non-empty, will be `auth` after D8.3 fix) | (any non-empty) | yes |
| circular-include | run.error | parse | PARSE_CIRCULAR_INCLUDE | no |

For `failing-assertion` and `unreachable-host` the harness must spawn an httptest.Server (or use a known-unreachable port like `127.0.0.1:1`) — wired in `runApitestForScenario` based on a `setup` field in `expect.yaml`. We keep this minimal:
- `failing-assertion` uses an httptest.Server returning 200; the collection expects status 201.
- `unreachable-host` uses `http://127.0.0.1:1` (always-refused on macOS/Linux test runners). No setup needed.

#### Tests to Write FIRST (RED phase)
- `TestContract_Load` (Step 1) parameterized per scenario already exercises that each `expect.yaml` is parseable.
- `TestHarness_AllScenarios` (Step 1) is now runnable end-to-end. Initially fails for at least: `missing-variable` (D8.1, D8.2), `failing-assertion` (D8.4), `feature-gate-denied` (D8.3). Fixing those is Steps 3–5.

#### Impact on Existing Tests
- None.

---

### Step 3: Fix `variable.ErrUndefinedVariable` classification (D8.1, D8.2)

**Rationale:** Smallest production-code change. Changes one constant, threads two existing fields through four call sites. Locally scoped to `internal/variable` and `internal/runner`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/variable.go` | modify | Change `Category: apierrors.CategoryConfig` to `CategoryInput` on the `ErrUndefinedVariable` Structured (line 246). |
| `internal/runner/runner.go` | modify | At each `interpErr` site (lines 1066, 1236, 1731, 1932), if `errors.As(interpErr, &structured)` and structured.FilePath is empty, set `structured.FilePath = item.SourceFile; structured.Line = item.SourceLine` before propagating. Helper `enrichInterpErr(err, item)`. |
| `internal/runner/runner.go` | add | `enrichInterpErr(err error, item parser.RequestItem) error` helper near other small helpers. |
| `internal/variable/variable_test.go` | modify | Update assertions if they hard-code `CategoryConfig` for ErrUndefinedVariable. |
| `internal/runner/runner_test.go` | add | New table-driven test `TestRun_VarUndefined_CarriesSourceLocation` verifying that a request with `{{MISSING}}` produces an error chain whose Structured carries the request's SourceFile and SourceLine. |
| `internal/output/events/testdata/golden/run_error.ndjson` | unchanged | Still uses parse error — no regen needed. |
| `cmd/apitest/run_test.go` | modify | `TestRunCmd_Events_UndefinedVariable_EmitsRunError` (lines 1208-1256) assertions extended to verify `error.file` non-empty and `error.line > 0` and `error.category == "input"`. |

#### Current Code

```go
// internal/variable/variable.go:243-252
val, ok := s.resolved[name]
if !ok {
    retErr = &apierrors.Structured{
        Category: apierrors.CategoryConfig,
        Message:  fmt.Sprintf("undefined variable %q", name),
        Hint:     fmt.Sprintf("Available variables: %s", strings.Join(s.AvailableVars(), ", ")),
        Inner:    ErrUndefinedVariable,
    }
    return match
}
```

#### New Code

```go
val, ok := s.resolved[name]
if !ok {
    retErr = &apierrors.Structured{
        Category: apierrors.CategoryInput,
        Code:     "VAR_UNDEFINED",
        Message:  fmt.Sprintf("undefined variable %q", name),
        Hint:     "Define the variable in the environment file, pass it via --var NAME=VALUE, or add a default in the collection.",
        Inner:    ErrUndefinedVariable,
    }
    return match
}
```

(Hint mirrors the registered hint from `hints_init.go` so the in-Structured value matches what the registry would supply via enrichment. Also sets `Code` so `EventError.Code` is non-empty without relying on enrichment.)

```go
// internal/runner/runner.go — new helper near other helpers
func enrichInterpErr(err error, item parser.RequestItem) error {
    var s *apierrors.Structured
    if !errors.As(err, &s) {
        return err
    }
    if s.FilePath == "" {
        s.FilePath = item.SourceFile
    }
    if s.Line == 0 {
        s.Line = item.SourceLine
    }
    return err
}
```

Each `interpErr := …` site becomes:
```go
interpolated, interpErr := requtil.InterpolateRequest(scope, &req)
if interpErr != nil {
    interpErr = enrichInterpErr(interpErr, item)
    // …existing wrap…
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go
func TestRun_VarUndefined_CarriesSourceLocation(t *testing.T) {
    tests := []struct {
        name        string
        sourceLine  int
        sourceFile  string
    }{
        {"line 7 in tests.yaml", 7, "tests.yaml"},
        {"line 12 in setup.yaml", 12, "setup.yaml"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            col := parser.Collection{
                Name: "X",
                Requests: parser.Requests{
                    Items: []parser.RequestItem{{
                        Name: "needs-var",
                        Request: parser.Request{Method: "GET", URL: "{{MISSING}}"},
                        SourceFile: tt.sourceFile,
                        SourceLine: tt.sourceLine,
                    }},
                },
            }
            _, _, err := runner.Run(context.Background(), &col, fakeExec, runner.VarSources{})
            if err == nil { t.Fatal("want err") }
            var s *apierrors.Structured
            if !errors.As(err, &s) { t.Fatalf("err is not Structured: %T %v", err, err) }
            if s.FilePath != tt.sourceFile { t.Errorf("FilePath = %q, want %q", s.FilePath, tt.sourceFile) }
            if s.Line != tt.sourceLine { t.Errorf("Line = %d, want %d", s.Line, tt.sourceLine) }
            if s.Category != apierrors.CategoryInput { t.Errorf("Category = %q, want input", s.Category) }
            if s.Code != "VAR_UNDEFINED" { t.Errorf("Code = %q, want VAR_UNDEFINED", s.Code) }
        })
    }
}
```

#### Impact on Existing Tests
- `internal/variable/variable_test.go::TestInterpolate*` — any assertion that the Structured Category is `config` for undefined variables will break. Update to expect `input` and verify Code=VAR_UNDEFINED.
- `cmd/apitest/run_test.go::TestRunCmd_Events_UndefinedVariable_EmitsRunError` — currently only asserts non-empty category and code; tighten to expect `category=="input"`, `code=="VAR_UNDEFINED"`, `error.file` non-empty, `error.line > 0`. This is a strengthening, not a breakage.

---

### Step 4: Add `*auth.GateError` classification (D8.3)

**Rationale:** Localized change to `internal/errors/classify.go` and `internal/auth/hints_init.go`. Required so the `feature-gate-denied` scenario passes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/errors/classify.go` | modify | Add `RegisterTypeClassifier(matcher func(error) bool, hint ClassifiedHint)` and consult it from `LookupHint` (and `ClassifyError`) when the simple sentinel registry misses. |
| `internal/errors/classify_test.go` | modify | Cover the new `RegisterTypeClassifier` path. |
| `internal/auth/hints_init.go` | modify | Register a type-classifier for `*GateError` with `Category: CategoryAuth`, `Code: FEATURE_GATE_DENIED`, `Hint: "Upgrade to the required tier or run with a tier that includes this feature. See {URL} or use --no-color to print the upgrade URL."` |
| `internal/auth/gate_test.go` | add | `TestGateError_Classified` asserts `apierrors.ClassifyError(&GateError{…}).Code == "FEATURE_GATE_DENIED"` and category=auth. |
| `internal/output/events/emitter_test.go` | unchanged | The schema test compiles `EventError` and rejects empty `code`; with the new classifier, gate errors now carry a code, so anything previously asserting `Code == ""` for gate errors must move to `== "FEATURE_GATE_DENIED"`. Audit and update. |

#### Current Code

```go
// internal/errors/classify.go
var (
    registryMu     sync.RWMutex
    registry       = map[error]ClassifiedHint{}
    packageEntries = map[string][]RegisteredError{}
)
```

#### New Code

```go
// New in classify.go
type typeClassifier struct {
    Match func(error) bool
    Hint  ClassifiedHint
}

var typeClassifiers []typeClassifier

// RegisterTypeClassifier adds a classifier that matches errors by Go type
// (via the supplied matcher, typically using errors.As). Used for typed errors
// that carry struct payloads (e.g. *auth.GateError) and therefore cannot be
// matched by errors.Is in the sentinel registry.
func RegisterTypeClassifier(match func(error) bool, hint ClassifiedHint) {
    registryMu.Lock()
    defer registryMu.Unlock()
    typeClassifiers = append(typeClassifiers, typeClassifier{Match: match, Hint: hint})
}

// LookupHint walks the chain via errors.Is for sentinels, then via the
// registered type classifiers. First match wins; sentinel matches take
// priority because they are more specific.
func LookupHint(err error) (ClassifiedHint, bool) {
    if err == nil { return ClassifiedHint{}, false }
    registryMu.RLock()
    defer registryMu.RUnlock()
    for sentinel, hint := range registry {
        if errors.Is(err, sentinel) { return hint, true }
    }
    for _, tc := range typeClassifiers {
        if tc.Match(err) { return tc.Hint, true }
    }
    return ClassifiedHint{}, false
}
```

```go
// internal/auth/hints_init.go
import (
    "errors"
    apierrors "github.com/peterlindqvist/apitest/internal/errors"
)

func init() {
    apierrors.RegisterPackage("auth", /* existing entries */ )
    apierrors.RegisterTypeClassifier(
        func(err error) bool {
            var ge *GateError
            return errors.As(err, &ge)
        },
        apierrors.ClassifiedHint{
            Category: apierrors.CategoryAuth,
            Code:     "FEATURE_GATE_DENIED",
            Hint:     "Upgrade to a tier that includes this feature, or run on a different tier. See the upgrade_url in the message.",
        },
    )
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/errors/classify_test.go
func TestRegisterTypeClassifier(t *testing.T) {
    type myErr struct{ msg string }
    e := &myErr{msg: "boom"}
    // …implement Error(), register classifier, assert ClassifyError result…
}

// internal/auth/gate_test.go
func TestGateError_ClassifiedAsAuth(t *testing.T) {
    ge := &GateError{Result: GateResult{Feature: "junit_xml", Message: "junit gated"}}
    s := apierrors.ClassifyError(ge)
    if s.Category != apierrors.CategoryAuth { t.Errorf("Category = %q, want auth", s.Category) }
    if s.Code != "FEATURE_GATE_DENIED" { t.Errorf("Code = %q, want FEATURE_GATE_DENIED", s.Code) }
    if !strings.Contains(strings.ToLower(s.Hint), "upgrade") { t.Errorf("Hint missing actionable verb: %q", s.Hint) }
}
```

#### Impact on Existing Tests
- `cmd/apitest/run_test.go::TestRunCmd_Events_FormatGateError_EmitsRunError` — loosely asserts a run.error appears; remains green and is strengthened in Step 6 to also check `error.code == "FEATURE_GATE_DENIED"` and `error.category == "auth"`.
- Coverage tests in `internal/errors/coverage_test.go` — review for whether the new classifier path needs to be enumerated; likely safe (it's a different mechanism).

---

### Step 5: Add `assertion` error to `request.end` for assertion-failed outcomes (D8.4)

**Rationale:** The `failing-assertion` scenario requires a `request.end` event with `outcome=failed` AND an `error` object with `category=assertion` and a concrete Hint. Today, `RequestEnd` carries no error when the failure is purely an assertion mismatch.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/errors.go` (or add to existing file) | modify/create | Add `ErrAssertionFailed = errors.New("assertion failed")` sentinel. |
| `internal/assertion/hints_init.go` | create or modify | Register `ErrAssertionFailed` with `Category: CategoryAssertion`, `Code: "ASSERTION_FAILED"`, `Hint: "Inspect the assertion.result events for this request and adjust either the assertion or the request to make them agree."` |
| `cmd/apitest/main.go` | modify | In `eventsAdapter.RequestEnd` (around line 373), when `e.Err == nil && e.Outcome == events.OutcomeFailed`, set `in.Err = assertion.ErrAssertionFailed`. |
| `cmd/apitest/main.go` | modify | Add `"github.com/peterlindqvist/apitest/internal/assertion"` to imports if not already present. |
| `cmd/apitest/run_test.go` | modify | Existing `TestRunCmd_Events_HappyPath` uses status=200; passing assertion stays. Add `TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd` covering the new behavior. |
| `internal/output/events/testdata/golden/run_failed_assertion.ndjson` | regenerate | Now carries an `error` field on the request.end line. Regen with `UPDATE_GOLDEN=1`. |

#### Current Code

```go
// cmd/apitest/main.go:373-394
func (a *eventsAdapter) RequestEnd(e runner.RequestEndEvent) {
    var reqBody, respBody []byte
    /* …redaction… */
    in := events.RequestEndInput{
        RequestID:    e.RequestID,
        Outcome:      events.Outcome(e.Outcome),
        StatusCode:   e.StatusCode,
        Duration:     e.Duration,
        WaveIndex:    e.WaveIndex,
        RequestBody:  reqBody,
        ResponseBody: respBody,
        Err:          e.Err,
    }
    // …
}
```

#### New Code

```go
in := events.RequestEndInput{ /* …unchanged fields… */ Err: e.Err }
// When the request failed purely because of assertion mismatches, surface a
// sentinel error so the request.end event carries a structured error block.
// Pre-existing e.Err (e.g. network/plugin error) takes precedence.
if in.Err == nil && in.Outcome == events.OutcomeFailed {
    in.Err = assertion.ErrAssertionFailed
}
```

#### Tests to Write FIRST (RED phase)

```go
// cmd/apitest/run_test.go
func TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd(t *testing.T) {
    srv := httptest.NewServer(/* always 200 */)
    defer srv.Close()
    /* write collection asserting status 201 */
    _, _, code := captureRunCmd(t, col, "--events", evPath)
    if code != 1 { t.Fatalf("want exit 1, got %d", code) }
    lines := parseJSONL(t, evPath)
    var endLine map[string]any
    for _, l := range lines {
        if l["kind"] == "request.end" { endLine = l; break }
    }
    if endLine == nil { t.Fatal("no request.end event") }
    if endLine["outcome"] != "failed" { t.Errorf("outcome = %v, want failed", endLine["outcome"]) }
    errObj, ok := endLine["error"].(map[string]any)
    if !ok { t.Fatal("request.end missing error block") }
    if errObj["category"] != "assertion" { t.Errorf("category = %v, want assertion", errObj["category"]) }
    if errObj["code"] != "ASSERTION_FAILED" { t.Errorf("code = %v, want ASSERTION_FAILED", errObj["code"]) }
    if h, _ := errObj["hint"].(string); !containsAnyVerb(h) { t.Errorf("hint lacks action verb: %q", h) }
}
```

#### Impact on Existing Tests
- `internal/output/events/schema_test.go::TestEmitter_GoldenRunFailedAssertion` — golden file changes (now carries error). Regenerate via `UPDATE_GOLDEN=1`.
- Any consumer assuming `request.end.error` is absent on `outcome=failed` — none currently in the tree.

---

### Step 6: Run the harness end-to-end and tighten failing-assertion / feature-gate scenario assertions

**Rationale:** With Steps 3–5 landed, the harness should now go GREEN for all seven scenarios. Tighten the existing `--events` integration tests that overlap with harness scenarios so we don't have two sources of truth.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/apitest/run_test.go` | modify | Strengthen `TestRunCmd_Events_FormatGateError_EmitsRunError` to check the new code/hint. Strengthen `TestRunCmd_Events_UndefinedVariable_EmitsRunError` to check file/line/code/category. |
| `cmd/apitest-agent-harness/harness_test.go` | run | Confirm `go test ./cmd/apitest-agent-harness/...` is GREEN for all seven scenarios. |
| `scripts/ci-local.sh` (if it filters paths) | verify | Confirm `cmd/apitest-agent-harness/` is part of `./...`. |

#### Tests to Write FIRST (RED phase)
- None new beyond Step 5. This step is verification.

#### Impact on Existing Tests
- The two strengthened tests in `cmd/apitest/run_test.go` may or may not pass depending on whether D8.1/D8.3 already landed; they should after Steps 3–5.

---

### Step 7: Schema promotion (the gate)

**Rationale:** This is the irreversible step. Done last, in a single commit, only after Step 6 confirms all scenarios are GREEN.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/events.go` | modify | `const SchemaVersion = "1.0"`. Update doc comment to point at `docs/events-schema/v1.0.json`. |
| `docs/events-schema/v1.0.json` | create | Copy of v0.1.json with every `"const": "0.1"` replaced by `"1.0"`, `$id` and `title` updated, `"version_status": "stable"` added at the top. |
| `docs/events-schema/v0.1.json` | modify | Add top-level `"deprecated": true` and a `"deprecation_message"` pointing to v1.0. Otherwise unchanged. |
| `docs/EVENTS_SCHEMA_v1.0.md` | create | Body of the v1.0 doc. Header includes "Promoted: <date>" and a "What changed since v0.1" section explicitly listing: (1) `EventError` always carries `code` and `hint` for production-emitted events (additive contract tightening); (2) `request.end` events with `outcome=failed` now carry `error.category=assertion`; (3) Stability policy update — removals/renames now require **v2.0**. |
| `docs/EVENTS_SCHEMA_v0.1.md` | modify | Insert at top: `> **DEPRECATED.** The current stable schema is [v1.0](EVENTS_SCHEMA_v1.0.md). v0.1 is retained for historical reference only.` Otherwise unchanged. |
| `internal/output/events/schema_test.go` | modify | `schemaPath` returns `v1.0.json`; `eventSchemaDocPath` returns `v1.0.md`. Add `TestSchema_v01ArtifactsRetained` asserting both v0.1.json and v0.1.md still exist (the gate's history requirement). |
| `internal/output/events/testdata/golden/*.ndjson` | regenerate | Run `UPDATE_GOLDEN=1 go test ./internal/output/events/...` so `schema_version` strings are `"1.0"`. |
| `cmd/apitest-agent-harness/contract.go` | review | If any expectation was hard-coded to `"0.1"`, update. (Should be none — contracts don't reference schema_version.) |
| `CHANGELOG.md` | modify | Add an `### Added` entry under `[Unreleased]` documenting M6-007: harness, schema promotion, and the production-code fixes (variable category, gate classification, assertion error). |

#### Tests to Write FIRST
- Tests for the promotion are already in place via the regenerated goldens and `TestSchema_DocInSyncWithCode`. Add the new `TestSchema_v01ArtifactsRetained` *before* the rename to enforce the retention rule.

#### Impact on Existing Tests
- `internal/output/events/schema_test.go::TestEmitter_AllKindsValidateAgainstSchema` — passes because the schema is now v1.0 and the emitter writes `"1.0"`.
- `internal/output/events/schema_test.go::TestEmitter_GoldenSchemaValidates` — passes after golden regen.
- `internal/output/events/schema_test.go::TestEmitter_GoldenRunHappy/RunError/RunFailedAssertion` — pass after golden regen.
- `internal/output/events/schema_test.go::TestSchema_MarkdownExamplesValidate` — passes if the doc points at v1.0 and examples use `"1.0"`.
- `cmd/apitest/run_test.go` — any test asserting `schema_version == "0.1"` must be updated to `"1.0"`. Search: only `TestRunCmd_Events_*` tests check via `kind` field and don't pin schema_version, so likely no breakage. Verify with `grep`.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/variable/variable_test.go` | tests asserting `CategoryConfig` for undefined variables | breaks | Update to `CategoryInput` and verify Code=VAR_UNDEFINED |
| `internal/runner/runner_test.go` | (new) `TestRun_VarUndefined_CarriesSourceLocation` | new | Write per Step 3 |
| `internal/errors/classify_test.go` | (new) `TestRegisterTypeClassifier` | new | Write per Step 4 |
| `internal/auth/gate_test.go` | (new) `TestGateError_ClassifiedAsAuth` | new | Write per Step 4 |
| `cmd/apitest/run_test.go` | `TestRunCmd_Events_UndefinedVariable_EmitsRunError` | strengthens | Add file/line/code/category assertions |
| `cmd/apitest/run_test.go` | `TestRunCmd_Events_FormatGateError_EmitsRunError` | strengthens | Add code=FEATURE_GATE_DENIED, category=auth assertions |
| `cmd/apitest/run_test.go` | (new) `TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd` | new | Write per Step 5 |
| `cmd/apitest-agent-harness/contract_test.go` | full new | new | Write per Step 1 |
| `cmd/apitest-agent-harness/harness_test.go` | `TestHarness_AllScenarios`, `TestHarness_FailsLoudlyOnContractBreach` | new | Write per Step 1 |
| `internal/output/events/schema_test.go` | `TestEmitter_GoldenRun*` | breaks at Step 7 | Regen goldens with UPDATE_GOLDEN=1 |
| `internal/output/events/schema_test.go` | (new) `TestSchema_v01ArtifactsRetained` | new | Write per Step 7 |
| `internal/output/events/schema_test.go` | `TestSchema_MarkdownExamplesValidate` | breaks at Step 7 (path change) | Update path resolver to v1.0.md |
| `internal/output/events/schema_test.go` | `TestSchema_DocInSyncWithCode` | breaks at Step 7 (path change) | Update path resolver to v1.0.json |
| `internal/output/events/coverage_test.go` (if any) | — | none | — |

---

## Risks and Edge Cases

- **Risk:** Schema promotion lands but a downstream test pins `schema_version=="0.1"` literal. → **Mitigation:** `grep -r '"0.1"' --include="*.go"` before Step 7 and update each hit.
- **Risk:** Goldens regenerated from a non-deterministic state (e.g. timestamp drift). → **Mitigation:** All golden tests use `Clock: fixedClock(t, "2026-04-21T10:00:00Z")` and a fixed `RunID` — verified in `schema_test.go`. Regen is reproducible.
- **Risk:** `unreachable-host` test flaky on hosts where `127.0.0.1:1` is NOT refused (rare but possible inside containers with port-1 bound). → **Mitigation:** Use the existing pattern from `TestRunCmd_network_error` (line 263) which uses `http://127.0.0.1:1/fail` — already proven across CI. Allow either `NETWORK_CONNECTION_REFUSED` or `NETWORK_DNS` in the contract.
- **Risk:** `failing-assertion` httptest.Server URL is dynamic; the harness must spawn the server and template the URL into the fixture. → **Mitigation:** Rather than templating, generate the collection on-the-fly inside `runApitestForScenario` using a `t.TempDir()` scratch file derived from the checked-in `collection.template.yaml` with `{{SERVER_URL}}` substituted at run time. Document this in `testdata/agent-harness/README.md`.
- **Risk:** The new `request.end.error` block on assertion failure surprises existing consumers that only check `outcome`. → **Mitigation:** This is a documented v0.1 → v1.0 diff; documented in `docs/EVENTS_SCHEMA_v1.0.md` per Step 7. Per stability policy (additive optional fields are allowed in v0.x without bump), this is also a backward-compatible change.
- **Risk:** Coverage drop because of new helper functions in `cmd/apitest-agent-harness/`. → **Mitigation:** All new code paths in the harness package are exercised by the harness tests themselves. Run `go test -coverprofile=coverage.out ./cmd/apitest-agent-harness/...` to verify >= 80% per `definition_of_done`.
- **Risk:** `golangci-lint run` complains about exported types in `cmd/apitest-agent-harness/contract.go` lacking doc comments. → **Mitigation:** Add doc comments on all exported types and functions during Step 1.
- **Risk:** Step 4 (RegisterTypeClassifier) introduces an ordering bug — sentinel matches and type classifiers race. → **Mitigation:** `LookupHint` checks sentinels first (`errors.Is`), then type classifiers (`errors.As`). Order is deterministic (sentinels are a map, but the chain via errors.Is is determined by the chain itself; type classifiers iterate a slice in registration order). Test `TestRegisterTypeClassifier_DoesNotShadowSentinels` covers this.
- **Edge case:** `auth-missing` fixture might initially produce category=`internal` because `RUNNER_AUTH_PROFILE_NOT_FOUND` is registered in `runner/hints_init.go` with `Category: CategoryAuth` — verified, no fix needed.
- **Edge case:** `feature-gate-denied` interaction with Step 4: `currentTier()` reads `APITEST_TIER` from env. The harness must set `APITEST_TIER=free` only for that scenario via the per-scenario `env.txt` — done via `cmd.Env` on `exec.Command`, not via `t.Setenv`, so other scenarios are unaffected.
- **Edge case:** The harness can be flaky if the test binary was built with stale code. → **Mitigation:** `buildApitestBinary(t)` always rebuilds into `t.TempDir()` — fresh per test run.

---

## Verification

```bash
# Step-by-step gate
go test ./internal/variable/...
go test ./internal/runner/...
go test ./internal/errors/...
go test ./internal/auth/...
go test ./internal/output/events/...
go test ./cmd/apitest/...
go test ./cmd/apitest-agent-harness/...
~/go/bin/golangci-lint run
./smoke/run.sh

# After schema promotion
UPDATE_GOLDEN=1 go test ./internal/output/events/...   # one-shot regen
go test ./...                                          # full tree

# Final
./scripts/ci-local.sh
```

Observable verification (matches the task YAML observable field):

```bash
go test ./cmd/apitest-agent-harness/...
# Expected: PASS for every scenario:
#   missing-variable, bad-yaml, failing-assertion, unreachable-host,
#   auth-missing, feature-gate-denied, circular-include.
```

Each scenario's pass condition:
- Stream contains the expected event kind(s).
- Every failure-carrying event has non-empty `category`, `code`, and `hint`.
- File and line are present (line may be 0 only when category=network).
- The hint contains at least one of: `Set`, `Add`, `Pass`, `Verify`, `Remove`, `Run`, `Define`, `Refresh`, `Check`.
- Process exit code matches the scenario's contract.
