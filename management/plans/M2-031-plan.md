# Implementation Plan: M2-031

## Overview
Extend the GraphQL protocol support with comprehensive error handling per specification: three modes (`fail`, `warn`, `ignore`), a global `defaults.graphql.error_handling.partial_success` setting in `curlew.yaml`, per-request override semantics, correct partial-success vs full-failure distinction, and surfacing GraphQL warnings to terminal and JSON output.

## Task Details
- **ID:** M2-031
- **Title:** GraphQL error handling and partial success configuration
- **Phase:** M2: GraphQL
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-029 | GraphQL protocol adapter | done |

## Key Design Decisions

1. **Three modes, not two.** `fail` (default), `warn`, `ignore`. `ignore` silently drops GraphQL errors; `warn` emits a warning but keeps exit code 0; `fail` fails the request.
2. **Full failure always fails.** Per spec scenario 2, responses with `errors` populated and `data == null` always fail, regardless of mode. The mode only affects *partial success* (both `data` and `errors` populated). Scenario 1 (no errors) is always a success.
3. **Per-request override wins.** The effective mode resolution is: `per-request graphql.error_handling` > `global defaults.graphql.error_handling.partial_success` > built-in default (`fail`). This mirrors the precedence pattern already used for retry defaults.
4. **Global config lives in `defaults.graphql`.** Keep the shape aligned with spec: `defaults.graphql.error_handling.partial_success: warn`. The nested `error_handling.partial_success` mirrors future extensibility (e.g., `null_propagation`).
5. **Warnings surface via a new generic `Warnings []string` on `RequestResult`.** Terminal output renders them yellow after assertion details; JSON output emits them as an optional `warnings` array field on `JSONRequest`. This is a narrow, additive field that avoids tangling with `RetryWarnings` (which has a distinct semantic for retry-specific conditions).
6. **Assertions on `$.errors[0].extensions.code` and similar already work.** The existing `assertion.Evaluate` runs JSONPath over the raw response body, and GraphQL responses ship as JSON. No assertion-layer changes required. We add a regression test in the GraphQL package to lock this in.
7. **Don't pollute `graphql.CheckResponse`.** The existing `CheckResponse` returns `HasErrors` / `HasData` / `Errors`. We add a small helper `ClassifyOutcome(check *ResponseCheck) Outcome` that returns one of `OutcomeSuccess`, `OutcomePartialSuccess`, `OutcomeFullFailure`, `OutcomeNoBody` so the runner doesn't need to re-derive the classification.
8. **Parser validation accepts `ignore`.** Extend the existing allow-list in `internal/parser/parser.go` and its test fixtures.
9. **Config module parses `defaults.graphql`.** Add a `GraphQLDefaults` type in `internal/config/project.go` wired into `DefaultsConfig.GraphQL`. Move runtime interpretation (mode enum) to the graphql package via `ParseErrorHandling` (already exists, extended for `ignore`).

## Implementation Steps

### Step 1: Extend `graphql.ErrorHandlingMode` with `ignore` and add outcome classifier
**Rationale:** Smallest blast radius. Self-contained additions inside `internal/graphql/` with no consumers forced to change yet. Once the primitive is in place, the runner and parser can use it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/graphql/graphql.go` | modify | Add `ErrorHandlingIgnore` constant; extend `ParseErrorHandling` to accept `"ignore"`; add `Outcome` type, `OutcomeSuccess`/`OutcomePartialSuccess`/`OutcomeFullFailure`/`OutcomeEmpty` constants; add `ClassifyOutcome(*ResponseCheck) Outcome` helper |
| `internal/graphql/graphql_test.go` | modify | Add test cases to `TestParseErrorHandling` for `ignore`; add new `TestClassifyOutcome` table-driven test |

#### Current Code (relevant excerpts)
```go
// internal/graphql/graphql.go
type ErrorHandlingMode string

const (
    ErrorHandlingFail ErrorHandlingMode = "fail"
    ErrorHandlingWarn ErrorHandlingMode = "warn"
)

func ParseErrorHandling(mode string) (ErrorHandlingMode, error) {
    switch mode {
    case "", "fail":
        return ErrorHandlingFail, nil
    case "warn":
        return ErrorHandlingWarn, nil
    default:
        return "", fmt.Errorf("%w: %q (allowed: fail, warn)", ErrInvalidErrorHandling, mode)
    }
}
```

#### New Code
```go
// internal/graphql/graphql.go
type ErrorHandlingMode string

const (
    ErrorHandlingFail   ErrorHandlingMode = "fail"
    ErrorHandlingWarn   ErrorHandlingMode = "warn"
    ErrorHandlingIgnore ErrorHandlingMode = "ignore"
)

// ParseErrorHandling returns the error handling mode from the config string.
// Empty defaults to fail. Accepts "fail", "warn", "ignore".
func ParseErrorHandling(mode string) (ErrorHandlingMode, error) {
    switch mode {
    case "", "fail":
        return ErrorHandlingFail, nil
    case "warn":
        return ErrorHandlingWarn, nil
    case "ignore":
        return ErrorHandlingIgnore, nil
    default:
        return "", fmt.Errorf("%w: %q (allowed: fail, warn, ignore)", ErrInvalidErrorHandling, mode)
    }
}

// Outcome classifies a GraphQL response against the three spec scenarios.
type Outcome int

const (
    OutcomeEmpty          Outcome = iota // no body or no recognisable fields
    OutcomeSuccess                       // data present, errors absent/empty
    OutcomePartialSuccess                // data present AND errors present
    OutcomeFullFailure                   // errors present, data absent or null
)

// ClassifyOutcome maps a parsed ResponseCheck into a spec scenario.
// ClassifyOutcome(nil) returns OutcomeEmpty.
func ClassifyOutcome(c *ResponseCheck) Outcome {
    if c == nil || (!c.HasData && !c.HasErrors) {
        return OutcomeEmpty
    }
    switch {
    case c.HasErrors && c.HasData:
        return OutcomePartialSuccess
    case c.HasErrors && !c.HasData:
        return OutcomeFullFailure
    default:
        return OutcomeSuccess
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// Additions to TestParseErrorHandling
{"ignore mode", "ignore", ErrorHandlingIgnore, false},
{"error message mentions ignore", "bogus", "", true}, // assert message contains "ignore"

func TestClassifyOutcome(t *testing.T) {
    tests := []struct {
        name  string
        check *ResponseCheck
        want  Outcome
    }{
        {"nil check is empty", nil, OutcomeEmpty},
        {"both absent is empty", &ResponseCheck{}, OutcomeEmpty},
        {"data only is success", &ResponseCheck{HasData: true}, OutcomeSuccess},
        {"errors only (data null) is full failure", &ResponseCheck{HasErrors: true}, OutcomeFullFailure},
        {"data + errors is partial success", &ResponseCheck{HasData: true, HasErrors: true}, OutcomePartialSuccess},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := ClassifyOutcome(tt.check); got != tt.want {
                t.Errorf("ClassifyOutcome(%+v) = %v, want %v", tt.check, got, tt.want)
            }
        })
    }
}
```

#### Impact on Existing Tests
- `TestParseErrorHandling` "invalid mode returns error" — unchanged; the new error message still contains "invalid graphql error_handling mode" via sentinel.
- No other existing test affected.

---

### Step 2: Extend parser validation for `ignore`
**Rationale:** Parser validation must accept `ignore` before any integration test that uses `ignore` at the per-request level can pass. Keeps validation fix isolated from wiring concerns.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | Extend `error_handling` allow-list from `{fail, warn}` to `{fail, warn, ignore}` and update hint text |
| `internal/parser/testdata/graphql_error_handling.yaml` | modify or add fixture | Add an `ignore` request (or add new fixture) |
| `internal/parser/testdata/graphql_invalid_error_handling.yaml` | verify | Ensure the invalid value used is still invalid (not "ignore") |
| `internal/parser/parser_test.go` | modify | Extend existing graphql error_handling validation test to cover `ignore` as valid |

#### Current Code
```go
// internal/parser/parser.go:188-199
if protocol == "graphql" && (*section)[i].Request.GraphQL != nil {
    eh := (*section)[i].Request.GraphQL.ErrorHandling
    if eh != "" && eh != "fail" && eh != "warn" {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryParse,
            FilePath: path,
            Message:  fmt.Sprintf("invalid error_handling value %q in request %q", eh, (*section)[i].Name),
            Hint:     "Allowed values: fail, warn",
            Inner:    ErrInvalidFieldValue,
        }
    }
}
```

#### New Code
```go
if protocol == "graphql" && (*section)[i].Request.GraphQL != nil {
    eh := (*section)[i].Request.GraphQL.ErrorHandling
    if eh != "" && eh != "fail" && eh != "warn" && eh != "ignore" {
        return nil, &apierrors.Structured{
            Category: apierrors.CategoryParse,
            FilePath: path,
            Message:  fmt.Sprintf("invalid error_handling value %q in request %q", eh, (*section)[i].Name),
            Hint:     "Allowed values: fail, warn, ignore",
            Inner:    ErrInvalidFieldValue,
        }
    }
}
```

#### Tests to Write FIRST (RED phase)
Extend the existing parser test covering `graphql_error_handling.yaml` (or add a new sub-case):
```go
// In parser_test.go — add a case under the existing graphql validation test
{
    name: "graphql error_handling ignore is valid",
    file: "testdata/graphql_error_handling_ignore.yaml",
    wantErr: false,
},
```
Add fixture `testdata/graphql_error_handling_ignore.yaml`:
```yaml
name: GQL Ignore
requests:
  - name: Ignore Errors
    request:
      protocol: graphql
      url: https://example.com/graphql
      graphql:
        query: "{ me { id } }"
        error_handling: ignore
```

#### Impact on Existing Tests
- `TestParseCollection` (or whichever test loads `graphql_invalid_error_handling.yaml`) — verify the invalid value inside that fixture is not `ignore`. Read fixture; if it contains `ignore`, change to `"bogus"`.

---

### Step 3: Add `GraphQLDefaults` to project config
**Rationale:** Add the global config type before wiring it through. Config parsing is a leaf dependency — runner will consume it next.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/config/project.go` | modify | Add `GraphQLDefaults` / `GraphQLErrorHandling` types; add `GraphQL *GraphQLDefaults` field to `DefaultsConfig`; validate on parse |
| `internal/config/project_test.go` | modify | Add test cases for parsing valid and invalid `defaults.graphql.error_handling.partial_success` |

#### New Code
```go
// internal/config/project.go
// GraphQLDefaults holds project-wide GraphQL defaults.
type GraphQLDefaults struct {
    ErrorHandling GraphQLErrorHandling `yaml:"error_handling,omitempty"`
}

// GraphQLErrorHandling holds configurable GraphQL error handling.
// Mirrors spec: defaults.graphql.error_handling.partial_success: fail|warn|ignore
type GraphQLErrorHandling struct {
    PartialSuccess string `yaml:"partial_success,omitempty"`
}

type DefaultsConfig struct {
    Retry   *retry.FullConfig `yaml:"retry,omitempty"`
    GraphQL *GraphQLDefaults  `yaml:"graphql,omitempty"`
}
```

Validation (after `pf.Defaults.Decode(&defaults)`):
```go
if defaults.GraphQL != nil {
    ps := defaults.GraphQL.ErrorHandling.PartialSuccess
    if ps != "" && ps != "fail" && ps != "warn" && ps != "ignore" {
        return nil, fmt.Errorf("%w: defaults.graphql.error_handling.partial_success: invalid value %q (allowed: fail, warn, ignore)",
            ErrInvalidProjectConfig, ps)
    }
}
```

#### Tests to Write FIRST (RED phase)
Add to `project_test.go`:
```go
func TestParseProjectConfig_graphqlDefaults(t *testing.T) {
    tests := []struct {
        name    string
        yaml    string
        wantPS  string   // empty = expect nil GraphQL or empty partial_success
        wantErr bool
    }{
        {
            name: "defaults.graphql.error_handling.partial_success warn",
            yaml: "defaults:\n  graphql:\n    error_handling:\n      partial_success: warn\n",
            wantPS: "warn",
        },
        {
            name: "fail value",
            yaml: "defaults:\n  graphql:\n    error_handling:\n      partial_success: fail\n",
            wantPS: "fail",
        },
        {
            name: "ignore value",
            yaml: "defaults:\n  graphql:\n    error_handling:\n      partial_success: ignore\n",
            wantPS: "ignore",
        },
        {
            name: "invalid value returns ErrInvalidProjectConfig",
            yaml: "defaults:\n  graphql:\n    error_handling:\n      partial_success: maybe\n",
            wantErr: true,
        },
        {
            name: "defaults without graphql block",
            yaml: "defaults:\n  retry:\n    max_attempts: 3\n",
            wantPS: "",
        },
    }
    // ... standard file-write + ParseProjectConfig invocation
}
```

#### Impact on Existing Tests
- `TestParseProjectConfig` — unchanged; new field is `omitempty` and defaults to nil. No existing fixture sets it.

---

### Step 4: Thread `GlobalGraphQL` through `VarSources` and resolve effective mode in runner
**Rationale:** Now all primitives exist. This step wires them through the runner, implements the full `fail`/`warn`/`ignore` behaviour, distinguishes full failure from partial success, and surfaces warnings via a new `RequestResult.Warnings` field. This is the step with the biggest blast radius, so it comes after the primitives.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `GlobalGraphQL *config.GraphQLDefaults` to `VarSources`; add `Warnings []string` field to `RequestResult`; replace inline GraphQL error block with call to helper that classifies outcome and resolves effective mode |
| `internal/runner/runner_test.go` | modify | Add tests for partial vs full failure, per-request override over global, `ignore` mode, and warning surfacing |

#### Key implementation block in runner (replacement)
```go
// internal/runner/runner.go — replace the existing GraphQL error checking block
if item.Request.Protocol == "graphql" && result != nil {
    // Resolve effective mode: per-request > global > built-in default (fail)
    effectiveMode := graphql.ErrorHandlingFail
    if vars.GlobalGraphQL != nil && vars.GlobalGraphQL.ErrorHandling.PartialSuccess != "" {
        m, mErr := graphql.ParseErrorHandling(vars.GlobalGraphQL.ErrorHandling.PartialSuccess)
        if mErr != nil {
            return results, requiredFailed, fmt.Errorf("global graphql defaults: %w", mErr)
        }
        effectiveMode = m
    }
    if item.Request.GraphQL != nil && item.Request.GraphQL.ErrorHandling != "" {
        m, mErr := graphql.ParseErrorHandling(item.Request.GraphQL.ErrorHandling)
        if mErr != nil {
            return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, mErr)
        }
        effectiveMode = m
    }

    check, checkErr := graphql.CheckResponse(result.Body)
    if checkErr == nil {
        outcome := graphql.ClassifyOutcome(check)
        // Full failure: always fails regardless of mode (per spec).
        // Partial success: mode decides.
        shouldFail := false
        shouldWarn := false
        switch outcome {
        case graphql.OutcomeFullFailure:
            shouldFail = true
        case graphql.OutcomePartialSuccess:
            switch effectiveMode {
            case graphql.ErrorHandlingFail:
                shouldFail = true
            case graphql.ErrorHandlingWarn:
                shouldWarn = true
            case graphql.ErrorHandlingIgnore:
                // no-op
            }
        }
        if shouldFail {
            if ar == nil {
                ar = &assertion.Results{Passed: false}
            }
            ar.Passed = false
            errMsg := "GraphQL response contains errors"
            if len(check.Errors) > 0 {
                errMsg = fmt.Sprintf("GraphQL error: %s", check.Errors[0].Message)
            }
            ar.Items = append(ar.Items, assertion.Result{
                Type:     "graphql_error",
                Expected: "no errors",
                Actual:   errMsg,
                Passed:   false,
            })
        }
        if shouldWarn && len(check.Errors) > 0 {
            for _, e := range check.Errors {
                warningsForReq = append(warningsForReq, fmt.Sprintf("GraphQL partial success: %s", e.Message))
            }
        }
    }
}
// ... later where rr is constructed:
rr.Warnings = warningsForReq
```

Introduce a local slice `var warningsForReq []string` above the GraphQL block so the warnings survive to the `rr` construction site.

#### Struct additions
```go
// VarSources (add field)
GlobalGraphQL *config.GraphQLDefaults // from curlew.yaml defaults.graphql

// RequestResult (add field)
Warnings []string // non-fatal warnings surfaced to output (e.g. graphql partial success)
```

Note: The runner already imports `internal/config`? Let me check — if not, we must avoid an import cycle. `internal/runner` imports `internal/auth`, `internal/retry`, `internal/parser`, etc. `internal/config` imports `internal/retry`, `internal/auth`, `internal/vault`. So runner importing config is fine (no cycle: config does not import runner).

#### Tests to Write FIRST (RED phase)

```go
// internal/runner/runner_test.go

func TestRun_graphql_partial_success_fail_by_default(t *testing.T) {
    // body: data + errors → default mode = fail → assertion fail
}

func TestRun_graphql_partial_success_warn_mode_global(t *testing.T) {
    // global defaults.graphql.error_handling.partial_success = warn
    // body: data + errors → passes, summary.Failed == 0, rr.Warnings non-empty
}

func TestRun_graphql_per_request_override_fail_beats_global_warn(t *testing.T) {
    // global warn, per-request fail → fails
}

func TestRun_graphql_per_request_override_warn_beats_global_fail(t *testing.T) {
    // global fail (implicit default), per-request warn → passes with warning
}

func TestRun_graphql_full_failure_always_fails_even_warn(t *testing.T) {
    // body: {"errors":[...],"data":null}, mode = warn → still fails per spec
}

func TestRun_graphql_full_failure_always_fails_even_ignore(t *testing.T) {
    // body: {"errors":[...],"data":null}, mode = ignore → still fails per spec
}

func TestRun_graphql_ignore_partial_success(t *testing.T) {
    // body: data + errors, mode = ignore → passes, no warnings, no failures
}

func TestRun_graphql_assertion_on_errors_extensions_code(t *testing.T) {
    // body has errors[0].extensions.code = "NOT_FOUND"
    // Request asserts $.errors[0].extensions.code equals NOT_FOUND
    // Mode: warn (so outer check won't fail the request)
    // Expect assertion passes and no GraphQL-error-level failure
}

func TestRun_graphql_assertion_on_errors_message(t *testing.T) {
    // Similar, asserting $.errors[0].message contains "deprecated"
}
```

Table-driven variant for mode matrix:
```go
func TestRun_graphql_mode_matrix(t *testing.T) {
    tests := []struct{
        name         string
        body         string
        globalMode   string // empty = unset
        perReqMode   string // empty = unset
        wantFailed   int
        wantWarnings bool
    }{
        {"success no errors", `{"data":{"ok":true}}`, "", "", 0, false},
        {"partial default fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "", "", 1, false},
        {"partial global warn", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "warn", "", 0, true},
        {"partial global ignore", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "ignore", "", 0, false},
        {"per-request fail beats global warn", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "warn", "fail", 1, false},
        {"per-request warn beats global fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "fail", "warn", 0, true},
        {"per-request ignore beats global fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "fail", "ignore", 0, false},
        {"full failure fail default", `{"data":null,"errors":[{"message":"boom"}]}`, "", "", 1, false},
        {"full failure warn still fails", `{"data":null,"errors":[{"message":"boom"}]}`, "warn", "", 1, false},
        {"full failure ignore still fails", `{"data":null,"errors":[{"message":"boom"}]}`, "ignore", "", 1, false},
    }
    // ...
}
```

#### Impact on Existing Tests
- `TestRun_graphql_errors_fail_default` — still valid. Body is full failure (`data:null`), mode default fail → still fails. No change.
- `TestRun_graphql_errors_warn` — **behaviour change**. Current test body is `{"errors":[{"message":"deprecated field"}],"data":{"user":{"name":"Alice"}}}` (partial success) with `ErrorHandling: "warn"`. Before: passes (assertion `Passed: true`). After: passes AND now carries a warning on `rr.Warnings`. Update the assertion to also verify `results[0].Warnings` is non-empty — this locks in the new warning-surfacing behaviour.
- No other runner tests use GraphQL error_handling.

---

### Step 5: Wire `GlobalGraphQL` from `cmd/curlew/main.go` and render warnings in output
**Rationale:** Final step: expose the global config to users running the binary, render warnings in terminal output, and add warnings to JSON output. Keeps user-facing changes (output shape, YAML accepted) in a single commit-worthy unit at the top of the stack.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Pass `projectCfg.Defaults.GraphQL` into `VarSources.GlobalGraphQL`; render `r.Warnings` via `out.Warning` after each request in terminal flow; populate `jr.Warnings` in `buildJSONOutput` |
| `internal/output/json.go` | modify | Add `Warnings []string \`json:"warnings,omitempty"\`` to `JSONRequest` |
| `internal/output/json_test.go` | modify | Verify warnings appear in JSON output |
| `internal/output/terminal_test.go` | no change | (existing `Warning()` tests cover the printer primitive) |
| `smoke/run.sh` | modify | Add one smoke assertion: a GraphQL partial-success collection with `error_handling: warn` at Professional tier... cannot hit a real server from smoke. Instead, assert parser accepts `error_handling: ignore` (doesn't feature-gate through to an HTTP call) |

#### `main.go` diff (Run call-site)
```go
results, summary, varErr := runner.Run(ctx, col, httpexec.Execute, runner.VarSources{
    ...
    GlobalRetry:         projectCfg.Defaults.Retry,
    GlobalGraphQL:       projectCfg.Defaults.GraphQL, // NEW
    ...
})
```

#### `main.go` terminal rendering (inside `runCmdInner`, default branch after `AssertionResults` loop)
```go
if r.AssertionResults != nil {
    for _, ar := range r.AssertionResults.Items {
        if !ar.Passed {
            out.AssertionDetail(ar.Type, ar.Expected, ar.Actual)
        }
    }
}
for _, w := range r.Warnings { // NEW
    out.Warning(w)
}
```

#### `main.go` JSON output (inside `buildJSONOutput`)
```go
jr := output.JSONRequest{
    ...
    Warnings: r.Warnings, // NEW (omitempty via tag)
}
```

#### `output/json.go` struct change
```go
type JSONRequest struct {
    ...
    RetryCount     int                 `json:"retry_count,omitempty"`
    Warnings       []string            `json:"warnings,omitempty"` // NEW
    AttemptDetails []JSONAttemptDetail `json:"attempt_details,omitempty"`
    ...
}
```

#### Tests to Write FIRST (RED phase)

```go
// internal/output/json_test.go — add to existing JSON tests
func TestJSONOutput_includes_warnings(t *testing.T) {
    out := &JSONOutput{
        Name: "t", Status: "passed", Requests: []JSONRequest{
            {Name: "a", Warnings: []string{"GraphQL partial success: deprecated field"}},
        },
    }
    // marshal, verify "warnings" array present with expected content
}

func TestJSONOutput_omits_warnings_when_empty(t *testing.T) {
    // marshal a JSONRequest with nil Warnings → "warnings" key absent
}
```

For the terminal path, an integration-style test via the existing `runCmdInner` test harness (if any) is not in scope; the unit test on `Printer.Warning` already covers the primitive. If there is no existing `runCmdInner` test, skip; this is covered end-to-end by the smoke test.

#### Impact on Existing Tests
- `internal/output/json_test.go` — existing JSON marshalling tests should still pass because `Warnings` is `omitempty`. Check any golden-file comparisons.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/graphql/graphql_test.go` | `TestParseErrorHandling` | extends | add `ignore` case |
| `internal/graphql/graphql_test.go` | (new) `TestClassifyOutcome` | new | add table test |
| `internal/parser/parser_test.go` | graphql error_handling validation | extends | add `ignore` positive case; verify invalid fixture is not `ignore` |
| `internal/parser/testdata/graphql_error_handling_ignore.yaml` | — | new fixture | create |
| `internal/config/project_test.go` | (new) `TestParseProjectConfig_graphqlDefaults` | new | add table test |
| `internal/runner/runner_test.go` | `TestRun_graphql_errors_warn` | behaviour update | assert `Warnings` non-empty |
| `internal/runner/runner_test.go` | (new) `TestRun_graphql_mode_matrix` | new | table test covering all modes × scenarios |
| `internal/runner/runner_test.go` | (new) full-failure-mode tests | new | verify full failure ignores mode |
| `internal/runner/runner_test.go` | (new) assertion-on-errors tests | new | regression lock for `$.errors[0].*` |
| `internal/output/json_test.go` | (new) warnings marshalling tests | new | verify field surfaces |
| `smoke/run.sh` | GraphQL block | extends | add `ignore` parser check |

## Risks and Edge Cases

- **Risk:** Adding `GlobalGraphQL` to `VarSources` may ripple into many test call sites because `VarSources` is constructed by many runner tests.
  **Mitigation:** Leave the field as a pointer with nil zero value. Existing tests don't set it, and nil means "no global" → existing behaviour is preserved byte-for-byte.

- **Risk:** Import cycle if `internal/runner` imports `internal/config`.
  **Mitigation:** Verified direction of imports in existing code — `config` does not import `runner`. If a cycle *does* surface unexpectedly, fall back to defining a runner-local interface `type GlobalGraphQLConfig struct { PartialSuccess string }` and have `main.go` construct it.

- **Risk:** Spec says `ignore` should "completely ignore GraphQL errors", which could be read as ignoring full failures too. We are implementing the stricter interpretation: full failures always fail (per Scenario 2 in the spec, which has no configurability hedge). This matches the more conservative reading and matches the task behaviour list: *"Given full failure ... when any error_handling mode, then the test always fails"*.
  **Mitigation:** Document this decision in this plan (done). Tests lock it in.

- **Edge case:** Empty body or 200 with no `data`/`errors` fields → `OutcomeEmpty`, no action taken, existing success path applies. Covered by `ClassifyOutcome` test and existing GraphQL tests.

- **Edge case:** `errors: null` or `errors: []` → not `HasErrors`, not a failure. Already handled by `CheckResponse`.

- **Edge case:** Multiple errors in partial success under `warn` → we emit one warning line per error message. Keeps signal-to-noise reasonable; first error alone would be easy to miss.

- **Edge case:** Per-request `error_handling` set to invalid value — caught by parser validation at load time (Step 2), never reaches the runner.

- **Edge case:** Global `defaults.graphql.error_handling.partial_success` set to invalid value — caught at config parse time (Step 3), never reaches the runner.

- **Risk:** Existing `internal/runner/runner_test.go` expectation for `TestRun_graphql_errors_warn` uses a partial-success body. With new warning-surfacing, that test now gets a non-empty `Warnings` slice. The behaviour contract is strictly additive (still passes, exit code unchanged), but the test assertion should be extended to lock the new behaviour. Marked in Step 4.

- **Risk:** JSON golden-file tests elsewhere might regress.
  **Mitigation:** `Warnings` uses `omitempty`, so empty slices serialize to nothing. Grep for any golden JSON that includes a graphql request to double-check during execute phase.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):
```bash
# 1. Partial success, global warn, exit 0
cat > /tmp/curlew-m2031/curlew.yaml <<'YAML'
project_name: M2031
defaults:
  graphql:
    error_handling:
      partial_success: warn
YAML
cat > /tmp/curlew-m2031/tests.yaml <<'YAML'
name: GraphQL Partial Warn
requests:
  - name: Partial
    request:
      protocol: graphql
      url: http://localhost:8080/graphql
      graphql:
        query: "{ user { name } }"
YAML
# Against a stub server returning {"data":{"user":{"name":"Alice"}},"errors":[{"message":"deprecated"}]}
# Expect exit code 0 and "Warning: GraphQL partial success: deprecated" in output

# 2. Per-request override fail beats global warn
# Add error_handling: fail to the request → exit code 1

# 3. Targeted test run
go test ./internal/graphql/...
```
