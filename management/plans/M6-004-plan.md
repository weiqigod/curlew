# Implementation Plan: M6-004

## Overview

Add a new `internal/output/events` package that defines the NDJSON event types for the M6 agent event stream (RunStart, RunError, RequestStart, RequestEnd, AssertionResult, RunEnd) and an `Emitter` that serializes them deterministically, safely under concurrent emit, with a monotonic atomic id counter and a checked-in JSON Schema (`docs/events-schema/v0.1.json`) validated by the package tests. No runner integration — that lands in M6-005.

## Task Details

- **ID:** M6-004
- **Title:** Events emitter package with NDJSON event types
- **Phase:** M6: AI Agent Integration
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-001 | Error taxonomy extension and sentinel hint registry | done |
| M6-002 | Source-location plumbing: file and line onto parsed items and results | done |

## Architectural Decisions

The task YAML leaves several implementation details open. The following decisions are made for this plan and documented here so downstream slices (M6-005/006/007) have a stable contract to build on.

1. **Package path:** `internal/output/events`, module-qualified
   `github.com/weiqigod/curlew/internal/output/events`. Consistent with the other formatters under `internal/output/`. The package imports `internal/errors` (alias `apierrors`) for `ClassifyError` and nothing else from the rest of the codebase — this preserves its isolation as required by the DoD ("new package is isolated").

2. **Schema version:** string constant `SchemaVersion = "0.1"`. Serialized as `schema_version`. M6-007 is the only slice authorised to bump this to `"1.0"`.

3. **Event header:** every event struct embeds a common `Header` (see signatures below). Header fields appear first in every emitted line in this order: `schema_version, run_id, id, at_ms, kind`. Kind-specific fields follow. Stable field order is achieved by defining fields in that order in Go structs — `encoding/json` preserves field declaration order.

4. **run_id generation:** a ULID-shaped random hex (16 bytes = 32 hex chars). Uses `crypto/rand` — deterministic seeding is NOT required for run_id (it identifies a single process run; only assertion randomness needs the `--deterministic` seed). Exposed as `Emitter.RunID()` for tests.

5. **at_ms:** `int64` milliseconds since `Emitter.startTime`. `RunStart` always reports `at_ms=0` (it IS the moment startTime is captured — Emitter captures startTime in its constructor, and RunStart uses the same constructor-captured time).

6. **id counter:** `sync/atomic.Int64` starting at 0, incremented with `Add(1)` so the first emitted event gets `id=1`. Monotonic across goroutines; the test `TestEmitter_ConcurrentEmitMonotonicIDs` launches N goroutines, collects ids from the emitted lines, asserts len==N and the sorted set == {1..N}.

7. **Write atomicity:** a `sync.Mutex` guards the `io.Writer`. Each event is marshaled into a buffer, appended with `\n`, then `Write` is called under the mutex. This guarantees no interleaved partial writes even for a non-atomic `io.Writer` (e.g. a plain `os.File`).

8. **Body truncation:** default threshold `DefaultBodyLimit = 2048`. The helper `truncateBody(raw []byte, limit int) (content string, size int, truncated bool)` returns the head of the bytes up to `limit`, the original byte length, and a `truncated=true` flag when `len(raw) > limit`. UTF-8 safety: we truncate at byte boundaries and use `strings.ToValidUTF8` so any partial multi-byte sequence at the tail is replaced with the Unicode replacement char (events consumers get valid UTF-8 JSON). Binary-looking bodies (bytes outside typical text range) are emitted as a base64-encoded string with a `body_encoding: "base64"` field; see signatures.

9. **Error object:** `EventError { Category, Code, Message, Hint, File string; Line int }`. Populated by calling `apierrors.ClassifyError(err)` inside `RequestEnd` and `RunError`. File/Line come from the `*Structured.FilePath` and `.Line` fields when present.

10. **Assertion classification for request.end:** when `AssertionResults` contains any `Passed=false` entries, `RequestEnd` with an assertion-failure outcome carries `Error.Category = "assertion"` and the first failing item's Type/Expected/Actual encoded in the message. The explicit `AssertionResult` events (one per assertion) are still emitted separately. This is consistent with M6-005's contract ("request.end with outcome=failed carrying an error object with category=assertion").

11. **RunEnd.event_count:** counted as total events emitted including RunStart, RunError (if any), RequestStart, RequestEnd, AssertionResult, and RunEnd itself. Implementation: capture the atomic counter's value just after incrementing for RunEnd — that's the total. Satisfies behaviour "event_count equal to the total number of events emitted (including RunStart and itself)".

12. **Outcome vocabulary for RequestEnd:** `passed | failed | skipped | error`. `passed` = HTTP round-trip succeeded AND all assertions passed. `failed` = round-trip succeeded but at least one assertion failed. `skipped` = runner marked the request Skipped. `error` = network error, pre-interpolation error, or any non-assertion error from the request's chain.

13. **JSON Schema authoring:** the schema is hand-written for v0.1 (not auto-generated). M6-006 will introduce the generator + drift test. For v0.1, correctness is enforced by the `TestEmitter_GoldenSchemaValidates` test which validates every emitted event kind against the schema using the existing `santhosh-tekuri/jsonschema/v6` library (already in go.mod — used by `internal/assertion/schema.go`).

14. **Golden NDJSON files:** stored under `internal/output/events/testdata/golden/` (not top-level `testdata/events/` — the task YAML scope says "testdata/events/" but the Go convention puts testdata under the package that owns it; this is the only deviation from the YAML wording and is stylistic, not functional). Each golden covers one scenario and is produced by the test via `Emitter` with frozen inputs (startTime fixed, run_id injected). Line-by-line comparison; update via `UPDATE_GOLDEN=1 go test`.

15. **Time injection:** the Emitter takes an optional `Clock func() time.Time` via `Options`; tests inject a fake clock. Defaults to `time.Now`.

16. **run_id injection:** similarly, `Options.RunID string` overrides the randomly-generated id for deterministic tests.

17. **CLI args hashing:** `RunStart.CLIArgs` is a `[]string`. Sensitive flag values (e.g. `--var TOKEN=secret`) are NOT redacted at this layer — M6-005 decides what to pass in and is responsible for redaction. This slice accepts whatever slice the caller provides. Documented in the godoc.

18. **No global state:** the package has no package-level mutable state. Every test constructs a fresh emitter with its own writer.

## Implementation Steps

### Step 1: Define event types and header

**Rationale:** Types first — everything else (Emitter, tests, schema) references them. Smallest blast radius: pure declarations, no behaviour.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/events.go` | create | Header, event struct per kind, SchemaVersion constant, outcome enum |

#### New Code

```go
// Package events defines the NDJSON agent event stream emitted by `curlew run`
// when the --events flag is provided. The schema is documented in
// docs/events-schema/v0.1.json.
package events

// SchemaVersion is the current event-stream schema version. Bumped only by
// M6-007 upon v1.0 promotion.
const SchemaVersion = "0.1"

// DefaultBodyLimit is the byte threshold above which request and response
// bodies are truncated in event payloads.
const DefaultBodyLimit = 2048

// Kind identifies an event's type. Appears in every event's "kind" field.
type Kind string

const (
    KindRunStart        Kind = "run.start"
    KindRunError        Kind = "run.error"
    KindRequestStart    Kind = "request.start"
    KindRequestEnd      Kind = "request.end"
    KindAssertionResult Kind = "assertion.result"
    KindRunEnd          Kind = "run.end"
)

// Outcome is the terminal state of a request.end event.
type Outcome string

const (
    OutcomePassed  Outcome = "passed"
    OutcomeFailed  Outcome = "failed"
    OutcomeSkipped Outcome = "skipped"
    OutcomeError   Outcome = "error"
)

// Header is embedded in every event type. Field order is deliberate: it
// appears first in the serialized JSON in this order.
type Header struct {
    SchemaVersion string `json:"schema_version"`
    RunID         string `json:"run_id"`
    ID            int64  `json:"id"`
    AtMs          int64  `json:"at_ms"`
    Kind          Kind   `json:"kind"`
}

// EventError is the error payload shared by RunError and RequestEnd.
type EventError struct {
    Category string `json:"category"`
    Code     string `json:"code,omitempty"`
    Message  string `json:"message"`
    Hint     string `json:"hint,omitempty"`
    File     string `json:"file,omitempty"`
    Line     int    `json:"line,omitempty"`
}

// RunStart is the first event emitted for every run.
type RunStart struct {
    Header
    StartedAt      string   `json:"started_at"`       // RFC3339Nano UTC
    CurlewVersion string   `json:"curlew_version"`
    CLIArgs        []string `json:"cli_args"`
    CollectionFile string   `json:"collection_file,omitempty"`
    EnvName        string   `json:"env_name,omitempty"`
}

// RunError is emitted when the run fails before normal completion.
type RunError struct {
    Header
    Error EventError `json:"error"`
}

// RequestStart marks the beginning of a single request execution.
type RequestStart struct {
    Header
    RequestID  string `json:"request_id"`
    Name       string `json:"name,omitempty"`
    Method     string `json:"method"`
    URL        string `json:"url"`
    Phase      string `json:"phase,omitempty"`
    SourceFile string `json:"source_file,omitempty"`
    SourceLine int    `json:"source_line,omitempty"`
}

// RequestEnd marks the end of a single request execution.
type RequestEnd struct {
    Header
    RequestID      string      `json:"request_id"`
    Outcome        Outcome     `json:"outcome"`
    StatusCode     int         `json:"status_code,omitempty"`
    DurationMs     int64       `json:"duration_ms"`
    WaveIndex      int         `json:"wave_index,omitempty"`
    RequestBody    string      `json:"request_body,omitempty"`
    RequestBodySize int        `json:"request_body_size,omitempty"`
    RequestBodyTruncated bool  `json:"request_body_truncated,omitempty"`
    ResponseBody   string      `json:"response_body,omitempty"`
    ResponseBodySize int       `json:"response_body_size,omitempty"`
    ResponseBodyTruncated bool `json:"response_body_truncated,omitempty"`
    BodyEncoding   string      `json:"body_encoding,omitempty"` // "base64" when set
    Error          *EventError `json:"error,omitempty"`
}

// AssertionResult records one assertion outcome.
type AssertionResult struct {
    Header
    RequestID string `json:"request_id"`
    Type      string `json:"type"`     // "status" | "body" | "header" | "schema"
    Passed    bool   `json:"passed"`
    Expected  string `json:"expected,omitempty"`
    Actual    string `json:"actual,omitempty"`
}

// RunEnd is the terminal event for every run.
type RunEnd struct {
    Header
    DurationMs int64 `json:"duration_ms"`
    Total      int   `json:"total"`
    Passed     int   `json:"passed"`
    Failed     int   `json:"failed"`
    Skipped    int   `json:"skipped"`
    ExitCode   int   `json:"exit_code"`
    EventCount int64 `json:"event_count"`
}
```

#### Tests to Write FIRST (RED phase)

No standalone tests for this step — the types are consumed by Step 2's Emitter tests. Step 2's table-driven tests exercise every field.

#### Impact on Existing Tests

- None. New package, no existing consumers.

---

### Step 2: Emitter with atomic id counter and mutex-guarded writer

**Rationale:** Second smallest blast radius — still a single new file, only imports `encoding/json`, `crypto/rand`, `sync`, `sync/atomic`, `time`, and the types from Step 1.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/emitter.go` | create | Emitter, Options, NewEmitter, per-kind emit methods, body truncation helper |
| `internal/output/events/emitter_test.go` | create | Full behaviour coverage including concurrent-emit test |

#### New Code

```go
package events

import (
    "crypto/rand"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "sync"
    "sync/atomic"
    "time"

    apierrors "github.com/weiqigod/curlew/internal/errors"
)

// ErrEmitterClosed is returned when Emit* is called after Close.
var ErrEmitterClosed = errors.New("events: emitter closed")

// Options configures an Emitter. Zero values are valid.
type Options struct {
    // Clock overrides time.Now. Used for deterministic tests.
    Clock func() time.Time
    // RunID overrides the randomly-generated run id. Used for deterministic tests.
    RunID string
    // BodyLimit overrides DefaultBodyLimit. Zero means use DefaultBodyLimit.
    BodyLimit int
    // CurlewVersion is recorded in RunStart. Required.
    CurlewVersion string
}

// Emitter serializes events to an io.Writer as NDJSON. Safe for concurrent use.
type Emitter struct {
    w         io.Writer
    mu        sync.Mutex // guards w and closed
    idCounter atomic.Int64
    startTime time.Time
    runID     string
    opts      Options
    closed    bool
    bodyLimit int
}

// NewEmitter constructs an Emitter writing to w. w must be non-nil.
func NewEmitter(w io.Writer, opts Options) (*Emitter, error) {
    if w == nil {
        return nil, errors.New("events: nil writer")
    }
    clock := opts.Clock
    if clock == nil {
        clock = time.Now
    }
    runID := opts.RunID
    if runID == "" {
        runID = newRunID()
    }
    bodyLimit := opts.BodyLimit
    if bodyLimit <= 0 {
        bodyLimit = DefaultBodyLimit
    }
    return &Emitter{
        w:         w,
        startTime: clock(),
        runID:     runID,
        opts:      opts,
        bodyLimit: bodyLimit,
    }, nil
}

// RunID returns the run identifier.
func (e *Emitter) RunID() string { return e.runID }

// BodyLimit returns the effective body truncation threshold.
func (e *Emitter) BodyLimit() int { return e.bodyLimit }

// Close marks the Emitter as closed. Subsequent Emit* calls return ErrEmitterClosed.
func (e *Emitter) Close() error {
    e.mu.Lock()
    defer e.mu.Unlock()
    e.closed = true
    return nil
}

// EmitRunStart emits a run.start event.
func (e *Emitter) EmitRunStart(cliArgs []string, collectionFile, envName string) error { /* … */ }

// EmitRunError emits a run.error event. err is classified via apierrors.ClassifyError.
func (e *Emitter) EmitRunError(err error) error { /* … */ }

// EmitRequestStart emits a request.start event. requestID is the caller's opaque id.
func (e *Emitter) EmitRequestStart(requestID, name, method, url, phase, sourceFile string, sourceLine int) error { /* … */ }

// EmitRequestEnd emits a request.end event.
func (e *Emitter) EmitRequestEnd(in RequestEndInput) error { /* … */ }

// EmitAssertionResult emits an assertion.result event.
func (e *Emitter) EmitAssertionResult(requestID, aType, expected, actual string, passed bool) error { /* … */ }

// EmitRunEnd emits a run.end event. The EventCount on the emitted event is the
// total number of events (including RunStart and RunEnd itself).
func (e *Emitter) EmitRunEnd(total, passed, failed, skipped, exitCode int) error { /* … */ }

// RequestEndInput packages the many inputs to EmitRequestEnd.
type RequestEndInput struct {
    RequestID    string
    Outcome      Outcome
    StatusCode   int
    Duration     time.Duration
    WaveIndex    int
    RequestBody  []byte
    ResponseBody []byte
    Err          error // non-nil implies Outcome == OutcomeError or OutcomeFailed
}

func (e *Emitter) nextID() int64 { return e.idCounter.Add(1) }
func (e *Emitter) atMs() int64   { return (e.opts.Clock()).Sub(e.startTime).Milliseconds() }
// clock() safely returns opts.Clock or time.Now
func (e *Emitter) clock() time.Time {
    if e.opts.Clock != nil {
        return e.opts.Clock()
    }
    return time.Now()
}

// writeEvent marshals v and writes the NDJSON line under the mutex.
func (e *Emitter) writeEvent(v any) error {
    data, err := json.Marshal(v)
    if err != nil {
        return fmt.Errorf("events: marshal: %w", err)
    }
    data = append(data, '\n')
    e.mu.Lock()
    defer e.mu.Unlock()
    if e.closed {
        return ErrEmitterClosed
    }
    if _, err := e.w.Write(data); err != nil {
        return fmt.Errorf("events: write: %w", err)
    }
    return nil
}

// newRunID produces a 32-char lowercase hex identifier.
func newRunID() string {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil {
        // crypto/rand should never fail; fall back to a time-based id.
        return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
    }
    return hex.EncodeToString(b[:])
}

// truncateBody returns the content for emission, the original size in bytes,
// whether truncation occurred, and the body_encoding ("base64" for binary,
// empty for UTF-8 text).
func truncateBody(raw []byte, limit int) (content string, size int, truncated bool, encoding string) { /* … */ }

// classifyForEvent converts err into an EventError using apierrors.ClassifyError.
func classifyForEvent(err error) EventError { /* … */ }
```

#### Tests to Write FIRST (RED phase)

```go
func TestEmitter_RunStart_MinimalFields(t *testing.T) {
    var buf bytes.Buffer
    em, err := NewEmitter(&buf, Options{
        Clock:          fixedClock(t, "2026-04-21T10:00:00Z"),
        RunID:          "test-run-001",
        CurlewVersion: "0.1.0-dev",
    })
    // assertions: buf has exactly one line; parsed JSON has kind=run.start,
    // schema_version=0.1, id=1, at_ms=0, run_id=test-run-001, started_at RFC3339Nano,
    // curlew_version=0.1.0-dev, cli_args=["run","x.yaml"].
}

func TestEmitter_RequestStartEnd_PairedIDs(t *testing.T) {
    // Call RequestStart then RequestEnd; both have same request_id, ids 1+2,
    // at_ms non-decreasing.
}

func TestEmitter_RequestEnd_RegisteredSentinelHint(t *testing.T) {
    // Fabricate an error wrapping parser.ErrInvalidYAML, pass into RequestEndInput.Err,
    // verify Error.Category=parse, Code=PARSE_INVALID_YAML, Hint non-empty.
    // (Uses blank import of parser to register hints.)
}

func TestEmitter_RequestEnd_NetworkErrorKinds(t *testing.T) {
    tests := []struct {
        name     string
        kind     apierrors.NetworkErrorKind
        wantCode string
    }{
        {"dns", apierrors.NetworkDNS, "NETWORK_DNS"},
        {"timeout", apierrors.NetworkTimeout, "NETWORK_TIMEOUT"},
        {"tls", apierrors.NetworkTLS, "NETWORK_TLS"},
        {"refused", apierrors.NetworkConnectionRefused, "NETWORK_CONNECTION_REFUSED"},
        {"other", apierrors.NetworkOther, "NETWORK_OTHER"},
    }
}

func TestEmitter_ConcurrentEmitMonotonicIDs(t *testing.T) {
    // Launch N=200 goroutines; each calls EmitRequestEnd once.
    // After Wait, split buffer on \n, parse each line as JSON, extract "id".
    // Assert: line count == N+1 (RunStart + N ends); ids form {1..N+1} with no duplicates;
    // NDJSON is line-oriented (each line is valid JSON).
}

func TestEmitter_BodyTruncation_OverLimit(t *testing.T) {
    // body of length 4096 with limit 2048 ⇒ body_truncated=true, body_size=4096,
    // request_body is the first 2048 bytes (UTF-8 safe).
}

func TestEmitter_BodyTruncation_UnderLimit(t *testing.T) {
    // body of length 100 ⇒ body_truncated omitted, body_size omitted when small,
    // request_body is the full body.
}

func TestEmitter_BodyTruncation_BinaryBase64(t *testing.T) {
    // body containing NUL bytes ⇒ body_encoding=base64, content is base64-encoded.
}

func TestEmitter_RunEnd_EventCount(t *testing.T) {
    // Emit RunStart, 3× RequestStart/End pairs, then RunEnd.
    // Parse RunEnd; expect event_count=8 (1 + 3 + 3 + 1).
}

func TestEmitter_EmitAfterClose(t *testing.T) {
    // Close, then EmitRunEnd; expect ErrEmitterClosed.
}

func TestEmitter_NilWriter(t *testing.T) {
    _, err := NewEmitter(nil, Options{})
    if err == nil { t.Fatal("expected error") }
}

func TestEmitter_RunID_Default(t *testing.T) {
    // With Options.RunID empty, RunID() returns a 32-char lowercase hex string.
}

func TestEmitter_AllKindsValidateAgainstSchema(t *testing.T) {
    // Emit one of each kind into a buffer, compile the checked-in schema,
    // split lines, validate each line against the oneOf in the schema.
}
```

#### Impact on Existing Tests

- None. New package.

---

### Step 3: Checked-in JSON Schema and golden NDJSON fixtures

**Rationale:** Schema is the public contract. Written after types are stable (so schema mirrors code) and before the schema-validation test (which depends on the file existing).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/events-schema/v0.1.json` | create | JSON Schema draft-07 covering all six event kinds |
| `internal/output/events/testdata/golden/run_happy.ndjson` | create | Golden for a minimal 1-request happy run |
| `internal/output/events/testdata/golden/run_error.ndjson` | create | Golden for a pre-execution failure |
| `internal/output/events/testdata/golden/run_failed_assertion.ndjson` | create | Golden for a failing-assertion run |

#### Schema Shape (sketch)

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://curlew.dev/events-schema/v0.1.json",
  "title": "Curlew Agent Event Stream v0.1",
  "oneOf": [
    { "$ref": "#/definitions/RunStart" },
    { "$ref": "#/definitions/RunError" },
    { "$ref": "#/definitions/RequestStart" },
    { "$ref": "#/definitions/RequestEnd" },
    { "$ref": "#/definitions/AssertionResult" },
    { "$ref": "#/definitions/RunEnd" }
  ],
  "definitions": {
    "Header": {
      "type": "object",
      "required": ["schema_version", "run_id", "id", "at_ms", "kind"],
      "properties": {
        "schema_version": {"const": "0.1"},
        "run_id": {"type": "string", "minLength": 1},
        "id": {"type": "integer", "minimum": 1},
        "at_ms": {"type": "integer", "minimum": 0},
        "kind": {"enum": ["run.start","run.error","request.start","request.end","assertion.result","run.end"]}
      }
    },
    "EventError": { /* category,code,message,hint,file,line */ },
    "RunStart": { /* allOf Header + specific props, required cli_args curlew_version started_at */ },
    "RunError": { /* allOf Header + error object */ },
    "RequestStart": { /* request_id method url, optional name/phase/source_file/source_line */ },
    "RequestEnd": { /* request_id outcome duration_ms, optional status_code wave_index body fields */ },
    "AssertionResult": { /* request_id type passed, optional expected actual */ },
    "RunEnd": { /* duration_ms total passed failed skipped exit_code event_count */ }
  }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestEmitter_GoldenSchemaValidates(t *testing.T) {
    // For each golden file in testdata/golden/*.ndjson:
    //   - read line by line
    //   - for each line: unmarshal as any, validate against compiled schema
    //   - expect zero validation errors
    // Uses santhosh-tekuri/jsonschema/v6 and testdata/../../../docs/events-schema/v0.1.json
    // (relative path resolved via runtime.Caller + filepath.Join).
}

func TestEmitter_GoldenRunHappy(t *testing.T) {
    // Replay a deterministic scenario; compare output bytes to the golden file.
    // UPDATE_GOLDEN=1 rewrites goldens.
}

func TestEmitter_GoldenRunError(t *testing.T) { /* … */ }
func TestEmitter_GoldenRunFailedAssertion(t *testing.T) { /* … */ }
```

#### Impact on Existing Tests

- None. Schema file is new and read only by new tests.

---

### Step 4: Documentation and CHANGELOG

**Rationale:** Last — picks up whatever the code ended up doing. Small, non-risky.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add entry under Unreleased: "events emitter package with NDJSON event types (M6-004)" |

#### Impact on Existing Tests

- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|---------------|--------|-----------------|
| — | — | none | No existing tests are touched. The new package is isolated. |

## Risks and Edge Cases

- **Risk:** Using `sync/atomic.Int64` + `sync.Mutex` could confuse readers into thinking the mutex protects the counter.
  → **Mitigation:** Godoc on `Emitter` explains the split: atomic guards id assignment (cheap under contention), mutex guards the io.Writer (required for non-atomic writes).

- **Risk:** `time.Time` granularity on Windows can cause `at_ms` to go backwards if the monotonic clock is not used.
  → **Mitigation:** `time.Now()` returns a monotonic value in Go 1.9+ and `Sub` uses it. Tests explicitly inject a deterministic clock so this isn't flaky. `at_ms` is documented as "milliseconds since run start (monotonic; may skew under parallel)".

- **Risk:** JSON schema validation is slow if we recompile per event.
  → **Mitigation:** tests compile once at test package level (TestMain or sync.Once in a helper); production code does not validate at runtime.

- **Risk:** Hand-written schema drifts from Go structs; downstream M6-006 introduces the generator-diff test.
  → **Mitigation:** v0.1 schema is validated against emitted output of every kind in `TestEmitter_AllKindsValidateAgainstSchema` and `TestEmitter_GoldenSchemaValidates`. Any schema-code drift fails these tests immediately.

- **Risk:** `RunStart.CLIArgs` could leak secrets when M6-005 passes `os.Args` directly.
  → **Mitigation:** out of scope for this slice. Documented in the godoc for `EmitRunStart` that the caller is responsible for redaction. M6-005 plan will address.

- **Edge case:** Empty body passed to the truncation helper.
  → **Handling:** returns `content=""`, `size=0`, `truncated=false`, `encoding=""`. All body fields are omitted via `omitempty`.

- **Edge case:** err is nil on `EmitRunError`.
  → **Handling:** returns a validation error (`fmt.Errorf("events: nil error for run.error")`). `EmitRequestEnd` with a nil `Err` and `OutcomeError` similarly validates.

- **Edge case:** Body containing mid-truncation multi-byte UTF-8 sequence.
  → **Handling:** `strings.ToValidUTF8(raw[:limit], "�")` so the emitted content is valid UTF-8 and JSON-encodable without escape surprises.

- **Edge case:** Concurrent Close + Emit.
  → **Handling:** both paths acquire `e.mu`. Close flips `closed=true` under the lock; later Emit observes it and returns `ErrEmitterClosed`.

- **Edge case:** Extremely large body.
  → **Handling:** truncation runs first so only the first `BodyLimit` bytes are ever marshaled. No unbounded allocation.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/output/events/...
go test -cover ./internal/output/events/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
go test ./internal/output/events/...
# Expected: PASS, including TestEmitter_ConcurrentEmitMonotonicIDs
# and TestEmitter_GoldenSchemaValidates.
go test -cover ./internal/output/events/...
# Expected coverage >= 80%.
```
