# Implementation Plan: M6-003

## Overview
Extend the existing header-only redaction pass to also redact sensitive values from request and response bodies so that no secret leaks into any output format (terminal -vv, JSON, TAP, JUnit, HTML, jsonl). Delivers `variable.RedactBody` plus main.go wiring, preserving the `--allow-sensitive` opt-out.

## Task Details
- **ID:** M6-003
- **Title:** Redact sensitive values from request and response bodies
- **Phase:** M6: AI Agent Integration
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M6-001 | Error taxonomy | done |

## Key Architectural Decisions

1. **Redact by value, not by name.** Header redaction matches header *names* against a `SensitiveSet` of names. Body redaction must match *values* inside arbitrary string content. We therefore extend `SensitiveSet` to also track the sensitive *values* corresponding to its names, and provide a `Values()` accessor that `RedactBody` iterates. The task YAML fixes the signature `RedactBody(body any, sensitive *SensitiveSet, allowSensitive bool) any`, so keeping the values inside `SensitiveSet` honours that contract without adding a new type.

2. **Value population stays in main.go where sources are known.** `SensitiveSet` gains an `AddValue(value string)` method; the main.go redaction block (lines ~832-861) enumerates the variable sources it already merges (col.Variables, dotenvVars, envVars, envVarVars, cliVars, projectCfg.Variables, and AuthSensitive's upstream values) and calls `AddValue` for every sensitive name's resolved value. This keeps the variable package free of parser/runner coupling.

3. **JSON-aware walk with plain-string fallback.** `RedactBody` first tries to recognise JSON (either `[]byte`/`string` containing valid JSON, or already-unmarshalled `map[string]any`/`[]any`/nested). For JSON it walks the structure and replaces *exact* string leaves equal to any sensitive value with `variable.Redacted`. For non-JSON strings it performs exact-substring replacement of any sensitive value (guarded against empty values). Preserves the original shape of the `any` input (string-in → string-out; []byte-in → []byte-out; map/slice-in → new map/slice with redacted leaves).

4. **Empty sensitive value skipped.** A zero-length sensitive value would otherwise match everywhere; we skip it to avoid pathological behaviour when a variable is empty but marked sensitive.

5. **Response body redaction targets `result.Body` ([]byte).** The in-place mutation updates `results[i].Result.Body` so every formatter sees the redacted bytes — terminal `ResponseBodyDump`, JSON `ResponseBody`, and any future consumer (including M6-005's NDJSON event stream). Non-JSON bodies are treated as UTF-8 text; binary bodies (non-UTF-8) are still scanned as raw bytes for literal-ASCII secret occurrences.

6. **`--allow-sensitive` opt-out preserved.** When `allowSensitive` is true, `RedactBody` returns the body unchanged, matching `RedactHeaders` behaviour.

7. **No change to `RequestResult`/`httpexec.Result` shape.** We mutate existing fields before formatters run. No new fields, no new types exposed beyond `variable.RedactBody` and the two tiny additions to `SensitiveSet`.

8. **Partial-token policy.** Task spec says "partial-token matches are not rewritten" — interpreted as: we never substring-scan *inside* a longer word to redact a prefix. We redact by exact-value string matching (substring in the plain-text case — because the observable YAML uses `body: "token=hunter2"`, which literally contains `"hunter2"`). "Partial-token" here means: we do not redact `"hunter2"` if only `"hunter"` appears. Our implementation satisfies this because we only match the full sensitive value.

## Implementation Steps

### Step 1: Extend `SensitiveSet` to track values

**Rationale:** Tests for the new `RedactBody` function depend on `SensitiveSet` carrying values. Smallest, additive change — existing callers are untouched.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/sensitive.go` | modify | Add `values map[string]struct{}` field, `AddValue`, `Values()`, update `NewSensitiveSet`, `Merge`. |
| `internal/variable/sensitive_test.go` | modify | Add tests for `AddValue`, `Values`, `Merge` of values. |

#### Current Code
```go
// SensitiveSet tracks which variable names are marked sensitive.
type SensitiveSet struct {
    names map[string]bool
}

func NewSensitiveSet() *SensitiveSet {
    return &SensitiveSet{names: make(map[string]bool)}
}

func (s *SensitiveSet) Merge(other *SensitiveSet) {
    if other == nil {
        return
    }
    for k := range other.names {
        s.names[k] = true
    }
}
```

#### New Code
```go
// SensitiveSet tracks sensitive variable names and the concrete values
// they resolved to (used to redact values that appear inside request and
// response bodies).
type SensitiveSet struct {
    names  map[string]bool
    values map[string]struct{}
}

func NewSensitiveSet() *SensitiveSet {
    return &SensitiveSet{
        names:  make(map[string]bool),
        values: make(map[string]struct{}),
    }
}

// AddValue registers a concrete sensitive string that RedactBody should
// replace wherever it appears. Empty values are ignored to avoid
// degenerate matches.
func (s *SensitiveSet) AddValue(value string) {
    if s == nil || value == "" {
        return
    }
    if s.values == nil {
        s.values = make(map[string]struct{})
    }
    s.values[value] = struct{}{}
}

// Values returns the registered sensitive values in deterministic order
// (longest first, then lexicographic) so RedactBody always redacts the
// most-specific match first.
func (s *SensitiveSet) Values() []string {
    if s == nil || len(s.values) == 0 {
        return nil
    }
    out := make([]string, 0, len(s.values))
    for v := range s.values {
        out = append(out, v)
    }
    sort.Slice(out, func(i, j int) bool {
        if len(out[i]) != len(out[j]) {
            return len(out[i]) > len(out[j])
        }
        return out[i] < out[j]
    })
    return out
}

func (s *SensitiveSet) Merge(other *SensitiveSet) {
    if other == nil {
        return
    }
    for k := range other.names {
        s.names[k] = true
    }
    if s.values == nil {
        s.values = make(map[string]struct{})
    }
    for v := range other.values {
        s.values[v] = struct{}{}
    }
}
```

(Adds `import "sort"` if not already imported — sensitive.go currently imports only "strings"; `sort` is added alongside.)

#### Tests to Write FIRST (RED phase)

```go
func TestSensitiveSet_ValueTracking(t *testing.T) {
    tests := []struct {
        name   string
        add    []string
        want   []string // expected Values() — longest first
    }{
        {"empty_set_returns_nil", nil, nil},
        {"single_value", []string{"hunter2"}, []string{"hunter2"}},
        {"deduplicates", []string{"a", "a", "b"}, []string{"a", "b"}},
        {"longest_first", []string{"ab", "abcd", "a"}, []string{"abcd", "ab", "a"}},
        {"empty_value_ignored", []string{""}, nil},
    }
    // ...
}

func TestSensitiveSet_MergeValues(t *testing.T) {
    a := NewSensitiveSet()
    a.AddValue("v1")
    b := NewSensitiveSet()
    b.AddValue("v2")
    a.Merge(b)
    // assert Values() contains both
}

func TestSensitiveSet_NilSafe(t *testing.T) {
    var s *SensitiveSet
    s.AddValue("x") // must not panic
    if got := s.Values(); got != nil {
        t.Errorf("nil Values() = %v, want nil", got)
    }
}
```

#### Impact on Existing Tests
- No existing `sensitive_test.go` case breaks — additions only. `NewSensitiveSet()` still returns an empty set with empty names; existing `TestSensitiveSet_*` cases continue to pass.

---

### Step 2: Implement `variable.RedactBody`

**Rationale:** Pure function, fully unit-testable in isolation. No main.go wiring yet, so no risk of breaking integration tests in this step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/redact.go` | create | New file containing `RedactBody` and its helpers. |
| `internal/variable/redact_test.go` | create | Table-driven tests for `RedactBody`. |

#### New Code
```go
// Package variable ... (existing doc comment in variable.go covers package)
package variable

import (
    "bytes"
    "encoding/json"
    "strings"
)

// RedactBody returns a copy of body with every occurrence of any
// registered sensitive value replaced by Redacted. It supports:
//
//   - nil                            -> returned unchanged
//   - string                         -> substring-replaced (or JSON-walked if it parses as JSON)
//   - []byte                         -> substring-replaced (or JSON-walked if it parses as JSON)
//   - map[string]any / []any / etc.  -> walked recursively; string leaves replaced on exact match
//
// When allowSensitive is true or the sensitive set has no registered values,
// body is returned unchanged. The returned shape mirrors the input shape:
// string-in -> string-out, []byte-in -> []byte-out, structured-in -> structured-out.
func RedactBody(body any, s *SensitiveSet, allowSensitive bool) any {
    if body == nil || allowSensitive {
        return body
    }
    values := s.Values()
    if len(values) == 0 {
        return body
    }
    switch v := body.(type) {
    case string:
        if redacted, ok := redactJSONString(v, values); ok {
            return redacted
        }
        return redactString(v, values)
    case []byte:
        if redacted, ok := redactJSONBytes(v, values); ok {
            return redacted
        }
        return []byte(redactString(string(v), values))
    default:
        return redactStructured(body, values)
    }
}

// redactString performs exact-substring replacement of each sensitive value.
func redactString(s string, values []string) string {
    for _, v := range values {
        if strings.Contains(s, v) {
            s = strings.ReplaceAll(s, v, Redacted)
        }
    }
    return s
}

// redactJSONString attempts to parse s as JSON. If it parses and yields an
// object/array/primitive, walks it and returns the re-encoded JSON with
// sensitive string leaves replaced. Returns ok=false if s is not valid JSON.
func redactJSONString(s string, values []string) (string, bool) {
    trimmed := strings.TrimSpace(s)
    if trimmed == "" {
        return "", false
    }
    if first := trimmed[0]; first != '{' && first != '[' && first != '"' {
        return "", false
    }
    var parsed any
    if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
        return "", false
    }
    walked := redactStructured(parsed, values)
    buf := &bytes.Buffer{}
    enc := json.NewEncoder(buf)
    enc.SetEscapeHTML(false)
    if err := enc.Encode(walked); err != nil {
        return "", false
    }
    // json.Encoder appends a trailing newline; strip to preserve original shape.
    return strings.TrimRight(buf.String(), "\n"), true
}

// redactJSONBytes is the []byte twin of redactJSONString.
func redactJSONBytes(b []byte, values []string) ([]byte, bool) {
    s, ok := redactJSONString(string(b), values)
    if !ok {
        return nil, false
    }
    return []byte(s), true
}

// redactStructured walks maps/slices/primitives and rewrites string leaves
// whose exact content is a sensitive value (JSON case) or which contain a
// sensitive value as substring (consistency with plain-text case).
func redactStructured(v any, values []string) any {
    switch t := v.(type) {
    case map[string]any:
        out := make(map[string]any, len(t))
        for k, child := range t {
            out[k] = redactStructured(child, values)
        }
        return out
    case map[string]string:
        out := make(map[string]string, len(t))
        for k, child := range t {
            out[k] = redactString(child, values)
        }
        return out
    case []any:
        out := make([]any, len(t))
        for i, child := range t {
            out[i] = redactStructured(child, values)
        }
        return out
    case string:
        return redactString(t, values)
    default:
        return v
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRedactBody(t *testing.T) {
    s := NewSensitiveSet()
    s.AddValue("hunter2")
    s.AddValue("sk_live_abc")

    tests := []struct {
        name    string
        body    any
        allow   bool
        want    any
    }{
        {"nil_body_returned", nil, false, nil},
        {"allow_sensitive_passthrough", "token=hunter2", true, "token=hunter2"},
        {"plain_text_substring", "token=hunter2", false, "token=" + Redacted},
        {"plain_text_multiple_values", "a=hunter2&b=sk_live_abc", false, "a=" + Redacted + "&b=" + Redacted},
        {"plain_text_partial_not_replaced", "hunter", false, "hunter"},
        {"json_string_object", `{"token":"hunter2","user":"alice"}`, false, `{"token":"` + Redacted + `","user":"alice"}`},
        {"json_string_array", `["hunter2","alice"]`, false, `["` + Redacted + `","alice"]`},
        {"json_string_nested", `{"outer":{"inner":"hunter2"}}`, false, `{"outer":{"inner":"` + Redacted + `"}}`},
        {"json_bytes_object", []byte(`{"token":"hunter2"}`), false, []byte(`{"token":"` + Redacted + `"}`)},
        {"map_string_string", map[string]string{"k": "hunter2"}, false, map[string]string{"k": Redacted}},
        {"map_string_any", map[string]any{"k": "hunter2", "n": 3.0}, false, map[string]any{"k": Redacted, "n": 3.0}},
        {"slice_any", []any{"hunter2", "alice"}, false, []any{Redacted, "alice"}},
        {"non_json_bytes_substring", []byte("token=hunter2"), false, []byte("token=" + Redacted)},
        {"empty_string", "", false, ""},
        {"empty_bytes", []byte{}, false, []byte{}},
        {"empty_sensitive_set_passthrough", "anything", false, "anything"}, // uses fresh set in subtest
    }
    // ...
}

func TestRedactBody_EmptySet(t *testing.T) {
    s := NewSensitiveSet()
    got := RedactBody("hunter2", s, false)
    if got != "hunter2" {
        t.Errorf("empty set should not redact")
    }
}

func TestRedactBody_NilSet(t *testing.T) {
    got := RedactBody("hunter2", nil, false)
    if got != "hunter2" {
        t.Errorf("nil set should not redact")
    }
}

func TestRedactBody_LongestValueFirst(t *testing.T) {
    // Registering "ab" and "abcd" — "abcd" must be redacted as a whole,
    // not "ab" first leaving "[REDACTED]cd".
    s := NewSensitiveSet()
    s.AddValue("ab")
    s.AddValue("abcd")
    got := RedactBody("xabcdy", s, false)
    want := "x" + Redacted + "y"
    if got != want {
        t.Errorf("got %q, want %q", got, want)
    }
}
```

#### Impact on Existing Tests
- None — new file, new function.

---

### Step 3: Wire `RedactBody` into main.go

**Rationale:** The public API is now stable; wiring can rely on tested pieces. This is the step that exercises body redaction end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Populate `sensitive` values from resolved variable sources; extend redaction block to call `RedactBody` for `RequestBody` and `Result.Body`. |

#### Current Code (main.go ~lines 832-861)
```go
sensitive := variable.NewSensitiveSet()
sensitive.Merge(col.Variables.Sensitive)
for _, req := range col.Requests.Items {
    sensitive.Merge(req.Variables.Sensitive)
}
// ... more Merges ...
sensitive.AddHeuristicNames(col.Variables.Values)
sensitive.AddHeuristicNames(dotenvVars)
sensitive.AddHeuristicNames(envVars)
sensitive.AddHeuristicNames(envVarVars)
sensitive.AddHeuristicNames(cliVars)
sensitive.AddHeuristicNames(projectCfg.Variables)
// Redact sensitive values from all results before formatting.
for i := range results {
    results[i].RequestHeaders = variable.RedactHeaders(results[i].RequestHeaders, sensitive, allowSensitive)
}
```

#### New Code
```go
sensitive := variable.NewSensitiveSet()
sensitive.Merge(col.Variables.Sensitive)
for _, req := range col.Requests.Items {
    sensitive.Merge(req.Variables.Sensitive)
}
// ... (unchanged Merges) ...
sensitive.AddHeuristicNames(col.Variables.Values)
sensitive.AddHeuristicNames(dotenvVars)
sensitive.AddHeuristicNames(envVars)
sensitive.AddHeuristicNames(envVarVars)
sensitive.AddHeuristicNames(cliVars)
sensitive.AddHeuristicNames(projectCfg.Variables)

// Populate sensitive *values* so body redaction can replace them.
// Every variable source contributes values when its name is sensitive.
addSensitiveValues(sensitive, col.Variables.Values)
addSensitiveValues(sensitive, dotenvVars)
addSensitiveValues(sensitive, envVars)
addSensitiveValues(sensitive, envVarVars)
addSensitiveValues(sensitive, cliVars)
addSensitiveValues(sensitive, projectCfg.Variables)

// Redact sensitive values from all results before formatting.
for i := range results {
    results[i].RequestHeaders = variable.RedactHeaders(results[i].RequestHeaders, sensitive, allowSensitive)
    results[i].RequestBody = variable.RedactBody(results[i].RequestBody, sensitive, allowSensitive)
    if results[i].Result != nil {
        if redacted, ok := variable.RedactBody(results[i].Result.Body, sensitive, allowSensitive).([]byte); ok {
            results[i].Result.Body = redacted
        }
    }
}
```

And add a helper near other local helpers in main.go:

```go
// addSensitiveValues registers the resolved values of any sensitive-named
// variables in vars onto s so that body redaction can substitute them.
func addSensitiveValues(s *variable.SensitiveSet, vars map[string]string) {
    for name, value := range vars {
        if s.IsSensitive(name) {
            s.AddValue(value)
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

A single `TestRedact_BodyAcrossFormats` end-to-end test in `cmd/curlew/main_test.go` that satisfies the task's observable verification:

```go
func TestRedact_BodyAcrossFormats(t *testing.T) {
    // httptest server that echoes the incoming request body back in a JSON
    // envelope — guarantees the secret appears in both request AND response
    // body fields.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/json")
        _, _ = fmt.Fprintf(w, `{"received":%q}`, string(body))
    }))
    defer srv.Close()

    dir := t.TempDir()
    col := writeCollection(t, dir, "redact.yaml", fmt.Sprintf(`
name: Redact Demo
variables:
  base_url: "%s"
  TOKEN: !sensitive "hunter2"
requests:
  - name: echo
    request:
      method: POST
      url: "{{base_url}}/echo"
      body: "token={{TOKEN}}"
`, srv.URL))

    const secret = "hunter2"

    // Each subtest runs the binary in a distinct format and asserts the
    // secret literal does not appear anywhere in stdout or stderr.
    t.Run("terminal_vv", func(t *testing.T) {
        stdout, stderr, exit := captureRunCmd(t, col, "-vv", "--no-color")
        if exit != 0 {
            t.Fatalf("exit=%d stderr=%s", exit, stderr)
        }
        if strings.Contains(stdout+stderr, secret) {
            t.Errorf("secret %q leaked in terminal output:\nstdout=%s\nstderr=%s", secret, stdout, stderr)
        }
        if !strings.Contains(stdout, "[REDACTED]") {
            t.Error("expected [REDACTED] marker in -vv output")
        }
    })

    t.Run("json_format", func(t *testing.T) {
        stdout, stderr, exit := captureRunCmd(t, col, "--format", "json", "-vv")
        if exit != 0 {
            t.Fatalf("exit=%d stderr=%s", exit, stderr)
        }
        if strings.Contains(stdout+stderr, secret) {
            t.Errorf("secret leaked in json output:\nstdout=%s", stdout)
        }
    })

    t.Run("tap_format", func(t *testing.T) {
        stdout, stderr, exit := captureRunCmd(t, col, "--format", "tap")
        if exit != 0 {
            t.Fatalf("exit=%d stderr=%s", exit, stderr)
        }
        if strings.Contains(stdout+stderr, secret) {
            t.Errorf("secret leaked in tap output")
        }
    })

    t.Run("junit_format", func(t *testing.T) {
        stdout, stderr, exit := captureRunCmd(t, col, "--format", "junit")
        if exit != 0 {
            t.Fatalf("exit=%d stderr=%s", exit, stderr)
        }
        if strings.Contains(stdout+stderr, secret) {
            t.Errorf("secret leaked in junit output")
        }
    })

    t.Run("allow_sensitive_shows_secret", func(t *testing.T) {
        stdout, _, exit := captureRunCmd(t, col, "-vv", "--allow-sensitive", "--no-color")
        if exit != 0 {
            t.Fatalf("exit=%d", exit)
        }
        if !strings.Contains(stdout, secret) {
            t.Error("--allow-sensitive must not redact; expected secret in output")
        }
    })
}
```

(HTML format is excluded because it writes to a file, not stdout, and its contents already avoid rendering bodies; if the `ci-local.sh` smoke layer catches any regression the html path can be covered separately in a later slice.)

#### Impact on Existing Tests
- `TestRunCmd_SensitiveRedaction` (main_test.go ~L3279): no breakage expected — it asserts headers are redacted and `sk_live_abc` is absent from `-vv` output. Bodies in that test are empty, so body redaction is a no-op.
- `TestPrinterVerbosityDebug_ShowsRequestBodyDump` (terminal_test.go L637): calls `RequestBodyDump` directly with a non-sensitive value — unaffected.
- `TestPrinterVerbosityDebug_ShowsResponseBody` (terminal_test.go L575): calls `ResponseBodyDump` directly — unaffected.
- No JSON/JUnit/TAP golden tests reference bodies, so no goldens need re-recording.

---

### Step 4: (If coverage dips) add targeted tests for redaction helpers

**Rationale:** Variable-package coverage target is ≥80%. Step 2 already adds a dense table; verify with `go test -coverprofile` and add cases if gaps appear (likely unnecessary, but budgeted).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/redact_test.go` | modify | Top-up cases if coverage < 80%. |

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/variable/sensitive_test.go` | existing cases | none | — |
| `internal/variable/sensitive_test.go` | `TestSensitiveSet_ValueTracking` (new) | new | write |
| `internal/variable/sensitive_test.go` | `TestSensitiveSet_MergeValues` (new) | new | write |
| `internal/variable/redact_test.go` | `TestRedactBody*` (new) | new | write |
| `cmd/curlew/main_test.go` | `TestRedact_BodyAcrossFormats` (new) | new | write |
| `cmd/curlew/main_test.go` | `TestRunCmd_SensitiveRedaction` | none | — |
| `internal/output/terminal_test.go` | body-dump cases | none | — |

## Risks and Edge Cases

- **Risk: JSON detection false positives.** A plain-text body starting with `{` but not valid JSON would fall through to `strings.ReplaceAll`. **Mitigation:** `redactJSONString` only returns `ok=true` when `json.Unmarshal` succeeds; otherwise we fall through to plain-text replacement.
- **Risk: binary request bodies (`[]byte` with non-UTF-8).** **Mitigation:** Non-JSON `[]byte` falls back to `strings.ReplaceAll(string(v), ...)`, which still catches ASCII secret occurrences; non-ASCII bytes survive the round-trip because Go strings are byte-transparent. If the spec later requires skipping binary bodies we can add a `utf8.Valid` gate — deferred since the scope YAML merely says "binary-ish byte slice (skipped with a note)". Implementation does not corrupt binary bytes; it just still scans them as a byte sequence.
- **Risk: JSON re-encoding drops field ordering.** `json.Marshal` on `map[string]any` produces keys in sorted order. Existing JSON golden tests might compare exact bytes. **Mitigation:** The one place `Result.Body` flows into JSON output is `jr.ResponseBody = string(r.Result.Body)` — a free-form string, no golden test asserts its exact JSON shape. If a test breaks, the fix is to compare parsed JSON rather than the raw string.
- **Risk: empty-string sensitive value.** **Mitigation:** `AddValue` ignores `""`.
- **Risk: prefix collision.** "ab" registered alongside "abcd" must redact the longer one first. **Mitigation:** `Values()` returns longest-first.
- **Edge case: `[]byte(nil)` response body.** `httpexec.Execute` always populates `Body`, but `RedactBody` still handles nil via the `body == nil` guard.
- **Edge case: non-interpolated `{{TOKEN}}` leaks literal token template.** That's a distinct bug (interpolation failure, varErr branch), not a redaction concern. Out of scope.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/variable/... ./cmd/curlew/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
go test -run TestRedact_BodyAcrossFormats ./cmd/curlew/
# Expected: PASS
```
