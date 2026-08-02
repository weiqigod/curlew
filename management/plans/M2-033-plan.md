# Implementation Plan: M2-033

## Overview

Extend the WebSocket executor with an explicit per-connection message buffer,
multi-pattern `any_of:` expect, multi-message `count:` expect with array
variable extraction, external-file `message_template:` loading, and an explicit
buffer-size warning. The work is confined to `internal/websocket/` and
`internal/parser/` with a thin hook through `parser.ParseFile` (mirroring the
existing GraphQL external-file pattern).

## Task Details

- **ID:** M2-033
- **Title:** WebSocket message buffering, expect patterns, and variable extraction
- **Phase:** M2: WebSocket
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task   | Title                              | Status |
| ------ | ---------------------------------- | ------ |
| M2-032 | WebSocket protocol adapter         | done   |

## Architectural Decisions

Resolved in this plan without user input:

1. **Buffer ownership.** A new `messageBuffer` type lives inside
   `internal/websocket/` and is created per `Execute` call, owned by the
   step loop. Each `runExpect` call receives it (instead of `conn`) so the
   expect logic can drain the buffer before blocking on the network. This
   isolates the buffer from `Conn`, which stays a pure I/O abstraction and
   keeps `fakeConn` unchanged.

2. **Buffer warning surface.** Warnings are carried on the existing
   `websocket.Result` via a new `Warnings []string` field. The runner
   already copies `Warnings` onto `RequestResult` for GraphQL
   (`runner.go:977`), so plumbing the WS warning through that same field
   needs only a one-line addition in `runner.go`. A single warning is
   emitted the first time the buffer crosses the 100-message threshold; it
   does not repeat every append.

3. **`any_of:` semantics.** `any_of:` is a list of alternative
   `BodyAssertions`. A message passes if **any** alternative matches. A
   step can specify **either** `message:` (the existing
   `ExpectAssertions`) **or** `any_of:`, not both. Parser rejects the
   combined form with a structured error. If both are absent on an
   `expect` step we preserve existing behaviour (no assertions = any
   message matches) - no regression.

4. **`count:` semantics.** `count: N` means "collect exactly N matching
   messages within `timeout_ms`". Extraction runs on **each** matching
   message; extracted values are accumulated into a JSON-array string and
   stored in the scope under the variable name (so
   `{{notifications}}` becomes `["a","b","c"]`). Partial collection on
   timeout is a failure (`ErrExpectTimeout`), with the number collected
   surfaced in the error message. `count: 1` is equivalent to the existing
   single-message behaviour.

5. **`count:` + `any_of:`.** Allowed. Each collected message must match
   *some* `any_of` alternative. Buffer consumption still happens in FIFO
   order.

6. **Variable extraction for `count:`.** The extracted value is the JSON
   serialization of a `[]any` of per-message values. When a single
   JSONPath is used (e.g. `notifications: "$.data"`), each extraction
   yields one element; the final variable is a JSON array literal. This
   is consistent with `variable.Extract`'s existing stringify behaviour
   for composite values.

7. **`message_template:` loader.** Lives in a new
   `internal/websocket/templates/` package (mirrors
   `internal/graphql/files/`). The file is read **at parse time** in
   `parser.ParseFile` (alongside the GraphQL loader block) so that
   templates participate in `col.ExternalFiles` and can be watched. The
   raw file contents are stored on the step as `MessageRawTemplate`; the
   existing interpolation path handles `{{variable}}` substitution at
   send time, so no new interpolation code is needed. A step-level
   `variables:` map merges into the scope **just for that send** (push a
   child scope, interpolate, pop) - keeps template variables from
   leaking into subsequent steps.

8. **Parser validation.** `message`, `message_raw`, and `message_template`
   on a `send` step are mutually exclusive. A structured parser error
   rejects any two-of-three combinations. `variables:` is only meaningful
   when `message_template:` is set; using it otherwise is a parse error
   with a hint.

9. **`message_raw:` already exists.** The behaviour listed in the task
   ("plain text string sent as-is") is already implemented and covered by
   `TestExecute_SendRawMessage`. I keep that test, add a more explicit
   regression test that asserts the payload is **not** JSON-marshalled,
   and move on. No code change required for this behaviour.

10. **Step ordering for TDD.** Parser schema extensions first
    (lowest blast radius - no runtime impact), then the message buffer
    (refactor of existing behaviour, covered by existing tests), then
    `any_of:`/`count:`, then `message_template:`. Template loading is last
    because it crosses package boundaries (new sub-package) and is easier
    to integration-test once the runtime pieces are stable.

## Implementation Steps

### Step 1: Extend parser schema for new WebSocket step fields

**Rationale:** New fields with zero runtime effect - adding them first lets
every later step write assertions against `parser.WebSocketStep` without a
second schema churn. Parser unit tests will catch YAML-decoding regressions
immediately.

#### Files to Modify

| File                                                 | Action   | Description                                                             |
| ---------------------------------------------------- | -------- | ----------------------------------------------------------------------- |
| `internal/parser/collection.go`                      | modify   | Add `AnyOf`, `Count`, `MessageTemplate`, `MessageRawTemplate`, `StepVariables` fields and YAML decoding |
| `internal/parser/parser.go`                          | modify   | Validate exclusivity + load message templates at parse time             |
| `internal/parser/parser_test.go`                     | modify   | Cover new schema and validation paths                                   |
| `internal/parser/testdata/websocket_any_of.yaml`     | create   | Fixture for `any_of:` expect                                            |
| `internal/parser/testdata/websocket_count.yaml`      | create   | Fixture for `count:` expect                                             |
| `internal/parser/testdata/websocket_template.yaml`   | create   | Fixture for `message_template:` send                                    |
| `internal/parser/testdata/websocket_template.json`   | create   | Template payload referenced by the fixture                              |
| `internal/parser/testdata/websocket_conflict_msg.yaml` | create | Fixture for `message` + `message_template` conflict                     |

#### Current Code

```go
// internal/parser/collection.go
type WebSocketStep struct {
    Action           string            // "send" | "expect" | "wait" | "close"
    Message          map[string]any    // send: literal JSON object
    MessageRaw       string            // send: literal string payload
    TimeoutMs        int               // expect: per-step wait for a matching message
    ExpectAssertions BodyAssertions    // expect: JSONPath assertions on the received frame
    Extract          map[string]string // expect: variable name -> JSONPath
    DurationMs       int               // wait: pause duration
    Code             int               // close: WebSocket close code (default 1000)
    Reason           string            // close: optional reason string
}
```

#### New Code

```go
// internal/parser/collection.go
type WebSocketStep struct {
    Action             string            // "send" | "expect" | "wait" | "close"
    Message            map[string]any    // send: literal JSON object
    MessageRaw         string            // send: literal string payload
    MessageTemplate    string            // send: path to external template file (parser-resolved)
    MessageRawTemplate string            // send: resolved template file contents (populated at parse time)
    StepVariables      map[string]string // send: scoped variables merged in for message_template interpolation
    TimeoutMs          int               // expect: per-step wait for a matching message
    ExpectAssertions   BodyAssertions    // expect: single-pattern JSONPath assertions ("message:")
    AnyOf              []BodyAssertions  // expect: alternative JSONPath assertion sets ("any_of:")
    Count              int               // expect: collect N matching messages (default 1)
    Extract            map[string]string // expect: variable name -> JSONPath
    DurationMs         int               // wait: pause duration
    Code               int               // close: WebSocket close code (default 1000)
    Reason             string            // close: optional reason string
}
```

Key additions to `UnmarshalYAML`:

```go
case "any_of":
    if v.Kind != yaml.SequenceNode {
        return fmt.Errorf("any_of: expected sequence, got %v", v.Kind)
    }
    for _, item := range v.Content {
        if item.Kind != yaml.MappingNode {
            return fmt.Errorf("any_of: each entry must be a mapping")
        }
        var inner BodyAssertions
        for j := 0; j+1 < len(item.Content); j += 2 {
            if item.Content[j].Value == "message" {
                if err := item.Content[j+1].Decode(&inner); err != nil {
                    return fmt.Errorf("any_of.message: %w", err)
                }
            }
        }
        s.AnyOf = append(s.AnyOf, inner)
    }
case "count":
    if err := v.Decode(&s.Count); err != nil {
        return fmt.Errorf("count: %w", err)
    }
case "message_template":
    s.MessageTemplate = v.Value
case "variables":
    if err := v.Decode(&s.StepVariables); err != nil {
        return fmt.Errorf("variables: %w", err)
    }
```

And in `parser.ParseFile`, a new block (after the GraphQL loader) that
walks every WebSocket step, loads `MessageTemplate`, and populates
`MessageRawTemplate` + appends the resolved path to `col.ExternalFiles`.
Validation additions:

```go
// Send mutual exclusion
hasMsg := step.Message != nil
hasRaw := step.MessageRaw != ""
hasTpl := step.MessageTemplate != ""
if step.Action == "send" {
    switch {
    case !hasMsg && !hasRaw && !hasTpl:
        return structuredErr("send requires message, message_raw, or message_template")
    case moreThanOne(hasMsg, hasRaw, hasTpl):
        return structuredErr("send: message, message_raw, and message_template are mutually exclusive")
    }
    if len(step.StepVariables) > 0 && !hasTpl {
        return structuredErr("variables only allowed with message_template")
    }
}
// Expect mutual exclusion
if step.Action == "expect" && len(step.AnyOf) > 0 && len(step.ExpectAssertions.Items) > 0 {
    return structuredErr("expect: message and any_of are mutually exclusive")
}
// Count must be >= 0
if step.Action == "expect" && step.Count < 0 {
    return structuredErr("expect.count must be >= 0")
}
```

#### Tests to Write FIRST (RED phase)

`internal/parser/parser_test.go`:

```go
func TestParseFile_WebSocketAnyOf(t *testing.T) {
    col, err := ParseFile("testdata/websocket_any_of.yaml")
    if err != nil { t.Fatal(err) }
    step := col.Requests.Items[0].Request.WebSocket.Steps[0]
    if len(step.AnyOf) != 2 { t.Fatalf("want 2 alternatives, got %d", len(step.AnyOf)) }
    if step.AnyOf[0].Items[0].Value != "success" { t.Errorf(...) }
}

func TestParseFile_WebSocketCount(t *testing.T) {
    col, err := ParseFile("testdata/websocket_count.yaml")
    if err != nil { t.Fatal(err) }
    step := col.Requests.Items[0].Request.WebSocket.Steps[0]
    if step.Count != 5 { t.Errorf("count = %d, want 5", step.Count) }
    if step.Extract["notifications"] != "$.data" { t.Errorf(...) }
}

func TestParseFile_WebSocketMessageTemplate(t *testing.T) {
    col, err := ParseFile("testdata/websocket_template.yaml")
    if err != nil { t.Fatal(err) }
    step := col.Requests.Items[0].Request.WebSocket.Steps[0]
    if step.MessageRawTemplate == "" { t.Fatal("template body not loaded") }
    if !strings.Contains(step.MessageRawTemplate, "{{channel}}") {
        t.Error("template should preserve placeholders")
    }
    if step.StepVariables["channel"] != "news" { t.Errorf(...) }
    found := false
    for _, p := range col.ExternalFiles {
        if strings.HasSuffix(p, "websocket_template.json") { found = true }
    }
    if !found { t.Error("template path missing from ExternalFiles") }
}

func TestParseFile_WebSocketSendMutualExclusion(t *testing.T) {
    tests := []struct {
        name    string
        file    string
        wantErr string
    }{
        {"message_and_template", "testdata/websocket_conflict_msg.yaml", "mutually exclusive"},
    }
    // ... table-driven
}

func TestParseFile_WebSocketExpectMutualExclusion(t *testing.T) { /* any_of + message */ }

func TestParseFile_WebSocketVariablesWithoutTemplate(t *testing.T) { /* error */ }
```

#### Impact on Existing Tests

- No existing parser tests reference the new fields; they should all
  pass unchanged. The mutual-exclusion validation only triggers on the
  new fixtures above.

---

### Step 2: Add the in-memory message buffer

**Rationale:** Refactor existing behaviour before adding new features.
The current executor discards stray frames; after this step it buffers
them, which is the foundation for `count:` and out-of-order matches.
Covered by existing tests, which continue to pass.

#### Files to Modify

| File                                    | Action | Description                                                      |
| ---------------------------------------- | ------ | ---------------------------------------------------------------- |
| `internal/websocket/buffer.go`          | create | `messageBuffer` type with bounded warn-at-100 behaviour          |
| `internal/websocket/buffer_test.go`     | create | Unit tests for buffer FIFO, warning at 100, drain, match         |
| `internal/websocket/executor.go`        | modify | Use buffer in `runExpect`; emit warnings via new `Result.Warnings` field |

#### Current Code

```go
// executor.go - Result has no Warnings slice
type Result struct {
    Steps    []StepResult
    Duration time.Duration
    Passed   bool
    Err      error
}

// runExpect drains stray messages by continuing the read loop (see executor.go:194-232)
```

#### New Code

```go
// internal/websocket/buffer.go
package websocket

const bufferWarnThreshold = 100

// messageBuffer is an unbounded FIFO of frames received on a WebSocket
// connection, awaiting consumption by an expect step. It is not safe for
// concurrent use; the executor owns it on the step loop goroutine.
type messageBuffer struct {
    frames  [][]byte
    warned  bool // true once the warn threshold has been crossed
    warning string
}

// push appends a frame. If this push crosses the warn threshold for the
// first time, it records a one-shot warning string the caller can surface.
func (b *messageBuffer) push(data []byte) {
    cpy := make([]byte, len(data))
    copy(cpy, data)
    b.frames = append(b.frames, cpy)
    if !b.warned && len(b.frames) > bufferWarnThreshold {
        b.warned = true
        b.warning = fmt.Sprintf(
            "websocket message buffer exceeded %d frames (current %d); " +
                "unmatched server messages are accumulating",
            bufferWarnThreshold, len(b.frames))
    }
}

// takeMatch removes and returns the first buffered frame that satisfies match.
// Returns (-1, nil) if no buffered frame matches.
func (b *messageBuffer) takeMatch(match func([]byte) bool) (int, []byte) {
    for i, f := range b.frames {
        if match(f) {
            b.frames = append(b.frames[:i], b.frames[i+1:]...)
            return i, f
        }
    }
    return -1, nil
}

func (b *messageBuffer) len() int       { return len(b.frames) }
func (b *messageBuffer) drainWarning() string {
    w := b.warning
    b.warning = ""
    return w
}
```

`executor.go`:

```go
type Result struct {
    Steps    []StepResult
    Duration time.Duration
    Passed   bool
    Err      error
    Warnings []string // buffer-size warnings, etc.
}

// Execute owns a single buffer for the connection lifetime.
func Execute(...) *Result {
    ...
    buf := &messageBuffer{}
    for i, step := range req.WebSocket.Steps {
        ...
        sr := runStep(ctx, conn, buf, step, scope)
        ...
        if w := buf.drainWarning(); w != "" {
            result.Warnings = append(result.Warnings, w)
        }
    }
    ...
}
```

`runExpect` changes (high-level): check buffer first with the new
`takeMatch` helper, then fall through to `conn.ReadMessage` in a loop.
Each unmatched frame from the wire goes into `buf.push(...)` rather than
being discarded. This preserves the "stray messages don't satisfy"
invariant (see `TestExecute_ExpectTimesOutAfterOnlyStrayMessages`) while
retaining frames for later steps.

#### Tests to Write FIRST (RED phase)

`internal/websocket/buffer_test.go`:

```go
func TestMessageBuffer_PushAndMatchFIFO(t *testing.T) {
    b := &messageBuffer{}
    b.push([]byte("a"))
    b.push([]byte("b"))
    b.push([]byte("c"))
    idx, frame := b.takeMatch(func(f []byte) bool { return string(f) == "b" })
    if idx != 1 || string(frame) != "b" { t.Errorf(...) }
    if b.len() != 2 { t.Errorf("len = %d, want 2", b.len()) }
    // FIFO order preserved for the remainder
    _, frame = b.takeMatch(func([]byte) bool { return true })
    if string(frame) != "a" { t.Errorf("want a first, got %s", frame) }
}

func TestMessageBuffer_WarningAt101(t *testing.T) {
    b := &messageBuffer{}
    for i := 0; i < 100; i++ { b.push([]byte("x")) }
    if b.drainWarning() != "" { t.Error("no warning at 100") }
    b.push([]byte("x")) // 101
    w := b.drainWarning()
    if w == "" { t.Error("expected warning after 101") }
    if b.drainWarning() != "" { t.Error("warning is one-shot") }
}

func TestMessageBuffer_WarningNotRepeated(t *testing.T) {
    b := &messageBuffer{}
    for i := 0; i < 250; i++ { b.push([]byte("x")) }
    // still exactly one warning
    if b.drainWarning() == "" { t.Error("want warning") }
    if b.drainWarning() != "" { t.Error("want one-shot") }
}
```

`internal/websocket/executor_test.go`:

```go
func TestExecute_BufferCarriesFramesAcrossSteps(t *testing.T) {
    // Three frames arrive up-front; the first expect consumes 'a',
    // the second expect instantly consumes 'b' from the buffer (no new read).
    fc := &fakeConn{incoming: [][]byte{
        []byte(`{"type":"a"}`),
        []byte(`{"type":"b"}`),
        []byte(`{"type":"c"}`),
    }}
    // expect a, expect c, expect b (out of order, buffer must handle it)
    ...
}

func TestExecute_BufferWarningSurfaced(t *testing.T) {
    // Inject 101 stray frames before the matching one, assert Result.Warnings
    // contains the buffer warning.
}
```

#### Impact on Existing Tests

- `TestExecute_ExpectSkipsStrayMessagesBeforeMatch` - still passes
  (buffer drains in FIFO order before the match).
- `TestExecute_ExpectTimesOutAfterOnlyStrayMessages` - still passes
  (buffer never yields a match -> timeout).
- `TestExecute_BufferFIFO` (already present in suite) - still passes;
  becomes a buffered-path test.
- `TestExecute_ExpectMatchesFromBuffer` - still passes.

---

### Step 3: Implement `any_of:` expect

**Rationale:** With the buffer in place, `any_of:` is a thin change to
the "does this frame match?" predicate - we wrap multiple assertion
sets in an OR. No network I/O changes.

#### Files to Modify

| File                                | Action | Description                                                |
| ---------------------------------- | ------ | ---------------------------------------------------------- |
| `internal/websocket/executor.go`   | modify | Teach `runExpect` about `step.AnyOf`                       |
| `internal/websocket/executor_test.go` | modify | Tests for `any_of:` match / mismatch / timeout              |

#### Current Code

```go
// executor.go:178
inputs := requtil.ToBodyInputs(step.ExpectAssertions.Items)
...
results := assertion.CheckBody(inputs, data)
ar := &assertion.Results{Items: results, Passed: allPassed(results)}
```

#### New Code

```go
// runExpect builds a list of candidate assertion sets. For a classic
// "message:" expect this list has length 1; for "any_of:" it has one
// entry per alternative. A frame matches if any candidate set passes.
candidates := buildExpectCandidates(step)

matches := func(data []byte) (*assertion.Results, bool) {
    var lastFailed *assertion.Results
    for _, inputs := range candidates {
        results := assertion.CheckBody(inputs, data)
        ar := &assertion.Results{Items: results, Passed: allPassed(results)}
        if ar.Passed {
            return ar, true
        }
        lastFailed = ar
    }
    return lastFailed, false
}

func buildExpectCandidates(step parser.WebSocketStep) [][]assertion.BodyInput {
    if len(step.AnyOf) > 0 {
        out := make([][]assertion.BodyInput, 0, len(step.AnyOf))
        for _, alt := range step.AnyOf {
            out = append(out, requtil.ToBodyInputs(alt.Items))
        }
        return out
    }
    return [][]assertion.BodyInput{requtil.ToBodyInputs(step.ExpectAssertions.Items)}
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_ExpectAnyOfMatchesFirst(t *testing.T) {
    fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"success"}`)}}
    step := parser.WebSocketStep{
        Action: "expect", TimeoutMs: 100,
        AnyOf: []parser.BodyAssertions{
            {Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "success"}}},
            {Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "error"}}},
        },
    }
    // ... assert Passed
}

func TestExecute_ExpectAnyOfMatchesSecond(t *testing.T) { /* symmetric */ }

func TestExecute_ExpectAnyOfNoneMatchTimesOut(t *testing.T) { /* stray frames only */ }
```

#### Impact on Existing Tests

- None. `runExpect` for a plain `message:` still takes the single-candidate
  path.

---

### Step 4: Implement `count:` expect with array extraction

**Rationale:** Builds on both the buffer (for pre-arrived frames) and
`any_of:` (candidates abstraction). Touches only `runExpect` and
`applyExtract`.

#### Files to Modify

| File                                  | Action | Description                                                       |
| ------------------------------------ | ------ | ----------------------------------------------------------------- |
| `internal/websocket/executor.go`     | modify | Loop until `count` matches collected or deadline hits             |
| `internal/websocket/executor_test.go` | modify | Tests for count=N, partial-on-timeout, array extraction           |

#### Current Code

```go
// applyExtract (executor.go:235) returns a flat map[string]string for a single match
func applyExtract(extract map[string]string, body []byte, scope *variable.Scope) (map[string]string, error)
```

#### New Code

```go
// runExpect collects N matching messages (N=1 by default).
count := step.Count
if count <= 0 { count = 1 }

collected := 0
var collectedVars []map[string]string // one map per matched message
var lastAr *assertion.Results

for collected < count {
    data, ar, ok, timedOut := nextMatchingFrame(ctx, conn, buf, candidates, deadline)
    if timedOut {
        return StepResult{
            Err: fmt.Errorf("%w after %dms: collected %d/%d", ErrExpectTimeout, timeoutMs, collected, count),
            Assertions: lastAr,
        }
    }
    if !ok {
        lastAr = ar
        continue
    }
    lastAr = ar
    extracted, err := extractForMatch(step.Extract, data)
    if err != nil {
        return StepResult{Err: fmt.Errorf("%w: %w", ErrExtractFailed, err), Assertions: ar}
    }
    collectedVars = append(collectedVars, extracted)
    collected++
}

// Apply aggregated extraction to the scope.
aggregated := aggregateExtraction(step.Extract, collectedVars, count)
for k, v := range aggregated {
    scope.Set(k, v)
}
return StepResult{Assertions: lastAr, Extracted: aggregated, Passed: true}
```

```go
// aggregateExtraction folds per-message extracted values into the final
// per-variable string. For count==1 the behaviour is identical to the old
// single-message extract. For count>1 each variable becomes a JSON array
// encoded as a string.
func aggregateExtraction(extract map[string]string, perMsg []map[string]string, count int) map[string]string {
    if len(extract) == 0 {
        return nil
    }
    if count == 1 {
        if len(perMsg) == 0 {
            return map[string]string{}
        }
        return perMsg[0]
    }
    out := make(map[string]string, len(extract))
    for name := range extract {
        arr := make([]string, 0, len(perMsg))
        for _, m := range perMsg { arr = append(arr, m[name]) }
        data, _ := json.Marshal(arr)
        out[name] = string(data)
    }
    return out
}

// extractForMatch evaluates Extract against a single frame, returning the
// per-variable map WITHOUT writing to the scope.
func extractForMatch(extract map[string]string, body []byte) (map[string]string, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_ExpectCountCollectsArray(t *testing.T) {
    fc := &fakeConn{incoming: [][]byte{
        []byte(`{"type":"update","data":"x"}`),
        []byte(`{"type":"update","data":"y"}`),
        []byte(`{"type":"update","data":"z"}`),
    }}
    scope := variable.NewScope(nil)
    step := parser.WebSocketStep{
        Action: "expect", TimeoutMs: 200, Count: 3,
        ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{
            {Path: "$.type", Operator: "equals", Value: "update"},
        }},
        Extract: map[string]string{"updates": "$.data"},
    }
    // Expect scope["updates"] == `["x","y","z"]`
}

func TestExecute_ExpectCountPartialTimesOut(t *testing.T) {
    fc := &fakeConn{incoming: [][]byte{[]byte(`{"type":"update","data":"x"}`)}}
    // count: 3, timeout_ms: 60 - should fail with "collected 1/3"
}

func TestExecute_ExpectCountOne_BackwardCompat(t *testing.T) {
    // Count: 1 with extract yields a flat (not array) variable
}

func TestExecute_ExpectCountWithAnyOf(t *testing.T) {
    // 2x success + 1x error message in buffer, count: 3, any_of matches
}

func TestExecute_ExpectCountSkipsNonMatchingMessages(t *testing.T) {
    // Interleaved noise + matches - only matches count towards N
}
```

#### Impact on Existing Tests

- `TestExecute_ExpectExtractsVariable` - still passes (Count defaults to
  1, aggregated extraction mirrors the old single-message map).
- `TestExecute_ExtractedVariableAvailableInLaterStep` - still passes.

---

### Step 5: Implement `message_template:` send

**Rationale:** Template loading is isolated to the parser; the runtime
only sees an already-resolved string on `MessageRawTemplate`. This lets
`runSend` stay simple: one new branch that interpolates the template with
an optional child scope carrying `StepVariables`.

#### Files to Modify

| File                                           | Action | Description                                                      |
| --------------------------------------------- | ------ | ---------------------------------------------------------------- |
| `internal/websocket/templates/templates.go`   | create | `LoadTemplate(baseDir, relPath) (body string, abs string, err)` |
| `internal/websocket/templates/templates_test.go` | create | Unit tests for load, relative/absolute, missing file, size limit |
| `internal/websocket/templates/errors.go`      | create | Sentinel errors (`ErrTemplateNotFound`)                          |
| `internal/parser/parser.go`                   | modify | Call loader in a post-parse pass, just like GraphQL files        |
| `internal/websocket/executor.go`              | modify | `runSend` branch for `MessageRawTemplate` with child scope       |
| `internal/websocket/executor_test.go`         | modify | Tests for template send with + without step variables            |

#### New Code

```go
// internal/websocket/templates/templates.go
package templates

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
)

var ErrTemplateNotFound = errors.New("websocket message template not found")

// LoadTemplate reads a message template file and returns its contents plus
// the absolute path. Relative paths are resolved against baseDir.
// Variable placeholders ({{name}}) are preserved for later interpolation.
func LoadTemplate(baseDir, relPath string) (body, abs string, err error) {
    abs = relPath
    if !filepath.IsAbs(abs) {
        abs = filepath.Join(baseDir, relPath)
    }
    data, readErr := os.ReadFile(abs)
    if readErr != nil {
        if errors.Is(readErr, os.ErrNotExist) {
            return "", "", fmt.Errorf("%w: %s", ErrTemplateNotFound, relPath)
        }
        return "", "", fmt.Errorf("reading message_template %q: %w", relPath, readErr)
    }
    return string(data), abs, nil
}
```

`parser.ParseFile` (new block):

```go
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        ws := (*section)[i].Request.WebSocket
        if ws == nil { continue }
        for j := range ws.Steps {
            if ws.Steps[j].MessageTemplate == "" { continue }
            body, abs, loadErr := wstemplates.LoadTemplate(collectionDir, ws.Steps[j].MessageTemplate)
            if loadErr != nil {
                return nil, &apierrors.Structured{
                    Category: apierrors.CategoryParse,
                    FilePath: path,
                    Message:  fmt.Sprintf("websocket request %q step %d: %s", (*section)[i].Name, j+1, loadErr),
                    Hint:     "Check the message_template path",
                    Inner:    loadErr,
                }
            }
            ws.Steps[j].MessageRawTemplate = body
            col.ExternalFiles = append(col.ExternalFiles, abs)
        }
    }
}
```

`executor.go`:

```go
// runSend - new branch (placed before the existing MessageRaw case)
case step.MessageRawTemplate != "":
    payload, err := renderTemplate(step, scope)
    if err != nil {
        return StepResult{Err: fmt.Errorf("%w: %w", ErrSendFailed, err)}
    }
    if writeErr := conn.WriteMessage(gws.TextMessage, payload); writeErr != nil {
        return StepResult{Err: fmt.Errorf("%w: %w", ErrSendFailed, writeErr)}
    }
    return StepResult{Passed: true}

// renderTemplate interpolates the template with step-local variables
// layered on top of scope. Child scope keeps step variables from leaking.
func renderTemplate(step parser.WebSocketStep, scope *variable.Scope) ([]byte, error) {
    child := scope.Child()
    for k, v := range step.StepVariables {
        interp, err := scope.Interpolate(v) // values themselves may use {{vars}}
        if err != nil {
            return nil, fmt.Errorf("interpolate variables[%q]: %w", k, err)
        }
        child.Set(k, interp)
    }
    out, err := child.Interpolate(step.MessageRawTemplate)
    if err != nil {
        return nil, fmt.Errorf("interpolate message_template: %w", err)
    }
    return []byte(out), nil
}
```

I will verify `variable.Scope.Child()` exists in the `variable` package;
if not, I will add a minimal child-scope helper that overlays a new
`map[string]string` without mutating the parent (mirrors existing env
chaining patterns in `internal/variable/variable.go`).

#### Tests to Write FIRST (RED phase)

`internal/websocket/templates/templates_test.go`:

```go
func TestLoadTemplate_Relative(t *testing.T) { /* tmp dir + read */ }
func TestLoadTemplate_Absolute(t *testing.T) { /* absolute path */ }
func TestLoadTemplate_Missing(t *testing.T) { /* wraps ErrTemplateNotFound */ }
func TestLoadTemplate_PreservesPlaceholders(t *testing.T) { /* {{x}} kept verbatim */ }
```

`internal/websocket/executor_test.go`:

```go
func TestExecute_SendMessageTemplate(t *testing.T) {
    fc := &fakeConn{}
    step := parser.WebSocketStep{
        Action: "send",
        MessageRawTemplate: `{"channel":"{{channel}}"}`,
        StepVariables: map[string]string{"channel": "news"},
    }
    // assert fc.writes[0] == `{"channel":"news"}`
}

func TestExecute_SendMessageTemplate_UsesScopeVars(t *testing.T) {
    // Variables defined at scope level are visible to the template too.
}

func TestExecute_SendMessageTemplate_StepVarsDoNotLeak(t *testing.T) {
    // After the template step, scope.Resolved() does NOT contain StepVariables.
}
```

#### Impact on Existing Tests

- `TestExecute_SendJSONMessage`, `TestExecute_SendRawMessage` - unchanged
  paths, still pass.
- No existing parser tests touch `message_template:`.

---

### Step 6: Surface `Result.Warnings` through the runner

**Rationale:** Last - it's a single-line wiring change that depends on
Step 2 having added the field.

#### Files to Modify

| File                             | Action | Description                                                    |
| -------------------------------- | ------ | -------------------------------------------------------------- |
| `internal/runner/runner.go`      | modify | Copy `wsRes.Warnings` onto the synthesised `RequestResult`     |
| `internal/runner/runner_test.go` | modify | Regression test: buffer warning propagates to `RequestResult.Warnings` |

#### Current Code

```go
// runner.go:760
rr := RequestResult{
    Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
    RequestHeaders: req.Headers, RequestBody: req.Body,
    Result:    synth,
    WaveIndex: -1,
}
```

#### New Code

```go
rr := RequestResult{
    Name: item.Name, Phase: phase, Method: req.Method, URL: req.URL,
    RequestHeaders: req.Headers, RequestBody: req.Body,
    Result:    synth,
    WaveIndex: -1,
    Warnings:  wsRes.Warnings,
}
```

#### Tests to Write FIRST (RED phase)

`internal/runner/runner_test.go`:

```go
func TestRun_WebSocketBufferWarningPropagates(t *testing.T) {
    // Fake dialer + fake conn that preloads 101 stray frames then one match.
    // Execute a one-request collection; assert results[0].Warnings contains
    // a substring like "buffer exceeded".
}
```

#### Impact on Existing Tests

- None. `Warnings` defaults to nil, so existing WS runner tests are
  unaffected.

---

## Test Impact Summary

| Test File                                        | Test Function                                    | Impact | Action Required                       |
| ------------------------------------------------ | ----------------------------------------------- | ------ | -------------------------------------- |
| `internal/websocket/executor_test.go`           | `TestExecute_ExpectMatchesFromBuffer`            | none   | still passes with new buffer internal |
| `internal/websocket/executor_test.go`           | `TestExecute_ExpectSkipsStrayMessagesBeforeMatch` | none  | buffer drain path                      |
| `internal/websocket/executor_test.go`           | `TestExecute_ExpectTimesOutAfterOnlyStrayMessages` | none | stray frames in buffer still don't match |
| `internal/websocket/executor_test.go`           | `TestExecute_BufferFIFO`                         | none   | becomes a pure buffer-path test        |
| `internal/websocket/executor_test.go`           | `TestExecute_ExpectExtractsVariable`             | none   | count defaults to 1                    |
| `internal/websocket/executor_test.go`           | `TestExecute_ExtractedVariableAvailableInLaterStep` | none | count defaults to 1                   |
| `internal/websocket/executor_test.go`           | `TestExecute_SendRawMessage`                     | none   | untouched send branch                  |
| `internal/parser/parser_test.go`                | existing websocket parse tests                   | none   | new fields are optional                |
| `internal/runner/runner_test.go`                | existing websocket runner tests                  | none   | `Warnings` defaults to nil             |

All new tests listed in Steps 1-6.

## Risks and Edge Cases

- **Risk:** Slice-based buffer is O(n) per `takeMatch` and O(n) per
  deletion. At the 100-message warning threshold the constants are
  still tiny (<=16kB memory, sub-microsecond scans), so we accept the
  simplicity.
  **Mitigation:** Keep the buffer internal; if profiling later shows
  it's hot we can switch to a linked list without touching callers.

- **Risk:** `json.Marshal` of extracted strings for `count:` produces a
  string-array even if the JSONPath value was numeric (`stringify`
  already coerces to string in the existing single-message path).
  **Mitigation:** This preserves backward compatibility - a single-
  message extract already stores `"42"` as a string today. Array
  extraction follows the same convention. Document in the variable
  comment.

- **Risk:** `count:` with `count: 0` is nonsensical - parser validation
  rejects negative values, but treating 0 as "default to 1" would hide
  user errors.
  **Mitigation:** `count <= 0` in `runExpect` defaults to 1 (backward
  compatible for steps that omit `count:`). The parser explicitly
  rejects negative counts.

- **Risk:** Timeout-before-first-frame semantics. When `count: 5` but
  only 2 frames arrive before the deadline, the existing behaviour
  (return `ErrExpectTimeout`) must not swallow the partial data.
  **Mitigation:** Error message includes `collected N/M`; per-match
  extracted variables are NOT written to the scope on failure (only
  complete collections mutate state), matching the existing "all or
  nothing" semantics.

- **Risk:** `any_of:` YAML decoding - the spec shows alternatives as
  list entries each with a `message:` key, not a naked assertion map.
  **Mitigation:** The custom `UnmarshalYAML` for `WebSocketStep` walks
  sequence entries and extracts the `message:` sub-node before decoding
  into `BodyAssertions`. Fixture and test pin the exact YAML shape
  shown in the spec (`message: { $.type: { equals: "success" } }`).

- **Edge case:** `message_template:` file contains invalid JSON. We do
  not validate - raw bytes are sent as-is (templates may be any text
  format). This matches `message_raw:`.

- **Edge case:** `message_template:` pointing outside the collection
  directory (e.g. `../../etc/passwd`). The existing GraphQL loader does
  not sandbox paths; we match that behaviour here for consistency. A
  separate security hardening task can address both loaders together.

- **Edge case:** `Scope.Child()` may not exist on the existing
  `variable.Scope` type. If the inspection confirms it is absent I will
  implement a minimal override pattern: pass a copy of resolved vars to
  a shadow scope that supports `Set`/`Interpolate` without mutating the
  parent, and defer promoting it to a first-class API.

- **Edge case:** A send step with `message_template:` but empty
  `StepVariables` must still render correctly using only parent-scope
  variables.

- **Edge case:** The buffer warning is emitted after the step that
  crossed the threshold completes. A single step that consumes >100
  frames in one go (rare) still surfaces the warning when
  `drainWarning` is called post-step.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
go test ./internal/websocket/...
```

Additional observable checks:

```bash
# Parser accepts the new fields
go test ./internal/parser/... -run WebSocket

# Runner plumbs the buffer warning
go test ./internal/runner/... -run WebSocketBufferWarning

# Full suite
go test ./... -run WebSocket
```

Coverage verification:

```bash
go test -coverprofile=coverage.out ./internal/websocket/... ./internal/parser/... ./internal/runner/...
go tool cover -func=coverage.out | grep -E "websocket|parser|runner"
# Expect >= 80% for internal/websocket/ and internal/websocket/templates/
```
