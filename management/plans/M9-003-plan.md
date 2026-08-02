# Implementation Plan: M9-003

## Overview

Extend the M9-002 markdown formatter with full content-type coverage (JSON,
YAML, XML, HTML, Text, Empty, HEAD, Binary), a 1 MiB body cap applied
post-redaction, a `### Response metadata` subsection that quarantines
volatile headers (Date, X-Request-ID, Set-Cookie, ETag, Server, Age) below
the response signal, and a redaction invariant test proving
`--allow-sensitive` cannot leak secrets into markdown.

## Task Details

- **ID:** M9-003
- **Title:** markdown content-type matrix, volatile-header discipline, 1 MiB body cap
- **Phase:** M9: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** medium
- **Estimated effort:** 1-2 days

## Dependencies

| Task   | Title                                                                                   | Status |
|--------|-----------------------------------------------------------------------------------------|--------|
| M9-002 | markdown formatter: --format markdown with sentinel splice, JSON body, run.md index     | done   |

## Architectural Decisions Resolved Up Front

These decisions are recorded so reviewers do not have to reverse-engineer
them. Each is informed by reading the file the slice will touch.

1. **Three new files in `internal/output/markdown/`.**
   - `contenttype.go` exposes the `Kind` enum and `Classify(body []byte,
     method string, contentType string) Kind`. Method is part of the
     signature so HEAD wins regardless of `Content-Length` or body bytes.
   - `metadata.go` owns the volatile-header set as a package-level
     `volatileHeaders map[string]struct{}` and the `renderMetadata`
     helper. This is the single source of truth referenced by behavior 12.
   - `bodycap.go` exposes `truncateForMarkdown(body []byte, cap int)
     (out []byte, truncated bool, original int)`. Default cap is exposed
     as `BodyCapBytes = 1 << 20` (1048576).

2. **Renderer dispatch is a switch on `Kind`, not a map of funcs.** A
   switch is the most compact form for eight cases, allows per-kind
   helpers to keep their own signatures, and avoids the heap allocation
   of a closure table. The new `renderBody(w *bytes.Buffer, kind Kind,
   body []byte, original int, truncated bool)` function lives in
   `formatter.go` and replaces the inline non-JSON fallback at
   `formatter.go:252-260`.

3. **JSON pretty-printer is reused, not duplicated.** The existing JSON
   path at `formatter.go:235-249` is extracted into a private
   `renderJSONBody(w *bytes.Buffer, body []byte)` so both the request
   body path (`renderRequest`) and the response body path
   (`renderBody(KindJSON, ...)`) call the same function. M9-002
   pretty-print semantics (2-space indent, fall-through to raw fence on
   parse error) are preserved byte-for-byte.

4. **Classifier signal hierarchy.** Per the task behavior:
   1. Method == HEAD -> `KindHEAD`. Wins unconditionally.
   2. `len(body) == 0` -> `KindEmpty`.
   3. `mime.ParseMediaType(contentType)` succeeds -> map media type to
      `Kind`. Subtype suffixes `+json` and `+xml` are honoured.
   4. Header missing or unparseable -> heuristic fallback:
      - `isBinary(body)` -> `KindBinary`. Reuses the same NUL-byte +
        non-UTF-8 algorithm at `events/emitter.go:342-349` so the events
        and markdown layers classify identically.
      - First non-whitespace byte is `{` or `[` -> `KindJSON` (sniff).
      - Otherwise -> `KindText`.

   Step 4's `isBinary` is duplicated, not extracted to a shared package,
   because (a) it's three lines, (b) extracting it requires a new
   exported package or moving a private function out of `events`, and
   (c) the events package owns its truncation rules and we should not
   couple them. The duplication is documented in `contenttype.go` with
   a comment pointing back to `events/emitter.go:342`.

5. **Binary rendering uses `encoding/hex.Dump` of the first 512 bytes.**
   `hex.Dump` is the canonical Go textual hex dump (offset, hex bytes,
   ASCII gutter). 512 bytes is enough to identify magic numbers (PNG,
   PDF, JPEG, ZIP, etc.) without flooding the markdown. The fence is
   ` ```hexdump ` (custom info-string, not standard but markdown
   renderers ignore unknown info-strings). Footer line:
   `_<N> bytes total_` where N is the post-redaction body size.

6. **YAML canonicalisation uses `gopkg.in/yaml.v3` default Encoder.**
   `gopkg.in/yaml.v3` is already a direct dep (`go.mod:11`). Default
   options: 4-space indent (yaml.v3's built-in), LF line endings, no
   document marker. The encoder accepts `any` so we
   `yaml.Unmarshal(body, &v)` first, then `yaml.NewEncoder(&buf).Encode(v)`.
   Round-trip failures (malformed YAML on the wire) fall through to
   verbatim rendering inside the same `yaml` fence — we never silently
   drop bytes.

7. **XML and HTML are verbatim, not prettified.** The server output is
   authoritative; reformatting risks breaking diffability against
   golden snapshots. Both render inside their respective fences with
   exactly one trailing newline before the closing fence (matching the
   text path).

8. **Body cap order is redact -> classify -> truncate -> format.** The
   redaction at `main.go:1442-1450` already overwrites
   `results[i].Result.Body` with redacted bytes; by the time the
   formatter sees `entry.RespBody`, all secrets are `[REDACTED]`. The
   cap is applied to those redacted bytes, so no truncation can split
   a redaction marker (the smallest redacted run is 10 bytes
   `[REDACTED]`; the cap is 1048576 bytes). The classifier runs against
   the truncated bytes for non-binary types, but for binary types the
   classifier runs on the full body (to detect binary signals) and
   truncation happens at hex-dump level (first 512 bytes are the
   preview; we still report `original` bytes total).

   Concretely, the renderBody flow is:

   ```
   redacted    = entry.RespBody                   // already redacted by main.go
   capped, tr  = truncateForMarkdown(redacted, BodyCapBytes)
   kind        = Classify(capped, entry.Method, ct)
   renderBody(w, kind, capped, len(redacted), tr) // original = pre-cap size
   ```

9. **Truncation marker.** When `truncated == true`, after the body code
   block close fence, emit a single line:

   ```
   _... truncated (body was <original> bytes, showing first 1048576)_
   ```

   For the binary path the `_<N> bytes total_` footer already conveys
   the original size; the truncation marker is appended *after* it on
   its own line to make the truncation explicit.

10. **`### Response metadata` is a third-level heading rendered between
    the response body block and the `### Timing` block.** Section order
    becomes:

    1. # name
    2. ## Notes
    3. BEGIN sentinel
    4. ## Response (deterministic)
    5. ### Request
    6. ### Response <status>     (signal: body fence, no volatile headers)
    7. ### Response metadata     (NEW: Content-Type, Content-Length, all other headers post-filter, alphabetical)
    8. ### Timing
    9. ### Assertions
    10. END sentinel
    11. ## Analysis

    Inserting metadata between the body and timing keeps the body next
    to its status line, and keeps timing/assertion as the last
    deterministic lines (where mask-friendliness matters most for
    diffs).

11. **Volatile-header set is closed and lives in `metadata.go`.** Per
    the behavior YAML it is exactly `{Date, X-Request-ID, Set-Cookie,
    ETag, Server, Age}`. Header names are normalised via
    `http.CanonicalHeaderKey` before comparison so `set-cookie` and
    `Set-Cookie` both filter. The set is exposed as
    `IsVolatileHeader(name string) bool` so future code can ask without
    re-implementing the membership check.

12. **Request-side volatile filter applies symmetrically.** The
    request-headers block (`renderRequest`) currently emits every
    header in alphabetical order. M9-003 filters out the same volatile
    set so request blocks are also diff-friendly. (The events stream is
    untouched — emitter has its own redaction & is not in scope.)

13. **Redaction invariant test does not exercise main.go.** It exercises
    the markdown formatter directly with a `RequestEntry` whose body
    bytes already contain `[REDACTED]` (mimicking what main.go produces
    after `variable.RedactBody`). Asserting that the formatter never
    re-emits the raw secret given redacted input proves the only
    promise the formatter can keep. The full pipeline test
    (collection + `--allow-sensitive` + `--format markdown`) lives in
    `cmd/apitest/run_test.go` so it covers the dispatcher contract.

14. **Body-cap test fixtures use generated bytes, not on-disk files.** A
    1.1 MiB or 2 MiB byte slice is built in-test with `bytes.Repeat`.
    No fixture file is created on disk (saves repo size, keeps the test
    self-describing).

15. **Existing tests do not break.** The only runtime change to
    M9-002-stable behaviour is the inserted `### Response metadata`
    section. The five M9-002 regression tests (`TestMarkdown_Render_*`,
    `TestMarkdown_DeterminismRegression`, `TestMarkdown_Newlines`,
    `TestMarkdown_RunMD*`, `TestRun_MarkdownFormat_*`) all use
    `application/json` bodies; their goldens need a refresh to add the
    new metadata section but no semantic changes. We commit the
    regenerated goldens as part of step 6.

16. **No CHANGELOG, init flag, or docs work.** Per task scope, those
    land in M9-005. M9-003 ships only the formatter changes plus
    integration tests.

## Implementation Steps

Steps are ordered by blast radius, smallest first.

### Step 1: Content-type classifier (zero call sites yet)

**Rationale:** Pure addition with no call sites — the classifier is
unreferenced by formatter.go until step 4. Failing tests here block
nothing else, so land it first and let the unit tests freeze its
contract before integrating.

#### Files to Modify

| File                                                     | Action  | Description                                                                          |
|----------------------------------------------------------|---------|--------------------------------------------------------------------------------------|
| `internal/output/markdown/contenttype.go`                | create  | `Kind` enum + `Classify` function + private `isBinary` (mirrors events).             |
| `internal/output/markdown/contenttype_test.go`           | create  | `TestMarkdown_ContentType` (table) + `TestMarkdown_ContentType_FallbackSniff`.       |

#### New Code (`contenttype.go`)

```go
// Package markdown ... (existing doc).

package markdown

import (
    "mime"
    "strings"
    "unicode/utf8"
)

// Kind classifies a response body for markdown rendering.
type Kind int

const (
    KindEmpty  Kind = iota // len(body) == 0 (and method != HEAD)
    KindHEAD               // request method == HEAD; body suppressed
    KindJSON               // application/json or *+json (or sniffed)
    KindYAML               // application/yaml or text/yaml (or *+yaml)
    KindXML                // application/xml, text/xml, or *+xml
    KindHTML               // text/html
    KindText               // any other text/* (after html, xml, yaml carved out)
    KindBinary             // isBinary heuristic + UTF-8 fallback
)

// Classify returns the Kind to use when rendering body in markdown.
//
// Method is checked first: HEAD always returns KindHEAD regardless of
// body or Content-Type, matching HTTP semantics where a HEAD response's
// body is by definition absent.
//
// Empty bodies (len 0) return KindEmpty before any header parsing.
//
// Otherwise the Content-Type header is the primary signal; mime.ParseMediaType
// extracts the media type and the subtype suffix (+json, +xml, +yaml).
// Missing or unparseable Content-Type falls back to: binary heuristic,
// then JSON-sniff (first non-whitespace byte is { or [), then text.
func Classify(body []byte, method, contentType string) Kind {
    if strings.EqualFold(method, "HEAD") {
        return KindHEAD
    }
    if len(body) == 0 {
        return KindEmpty
    }
    if mt, _, err := mime.ParseMediaType(contentType); err == nil {
        switch {
        case mt == "application/json" || strings.HasSuffix(mt, "+json"):
            return KindJSON
        case mt == "application/yaml" || mt == "text/yaml" || strings.HasSuffix(mt, "+yaml"):
            return KindYAML
        case mt == "application/xml" || mt == "text/xml" || strings.HasSuffix(mt, "+xml"):
            return KindXML
        case mt == "text/html":
            return KindHTML
        case strings.HasPrefix(mt, "text/"):
            return KindText
        }
        // Other media types (application/octet-stream, image/*, multipart/*)
        // fall through to the binary heuristic.
    }
    // Header missing or unparseable; or media type is non-text/non-structured.
    if isBinary(body) {
        return KindBinary
    }
    if jsonSniff(body) {
        return KindJSON
    }
    return KindText
}

// isBinary mirrors the heuristic at internal/output/events/emitter.go:342;
// duplicated rather than extracted because the events package owns its own
// classification rules and we should not couple the two layers.
func isBinary(raw []byte) bool {
    for _, b := range raw {
        if b == 0x00 {
            return true
        }
    }
    return !utf8.Valid(raw)
}

// jsonSniff returns true when the first non-whitespace byte is { or [.
// Used only when Content-Type is missing/unparseable.
func jsonSniff(body []byte) bool {
    for _, b := range body {
        switch b {
        case ' ', '\t', '\r', '\n':
            continue
        case '{', '[':
            return true
        default:
            return false
        }
    }
    return false
}
```

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_ContentType(t *testing.T) {
    tests := []struct {
        name        string
        body        []byte
        method      string
        contentType string
        want        Kind
    }{
        {"head wins over body", []byte("ignored"), "HEAD", "application/json", KindHEAD},
        {"head case-insensitive", []byte("ignored"), "head", "", KindHEAD},
        {"empty body", []byte{}, "GET", "application/json", KindEmpty},
        {"application/json", []byte(`{"a":1}`), "GET", "application/json", KindJSON},
        {"vendor +json", []byte(`{}`), "GET", "application/vnd.github.v3+json", KindJSON},
        {"json with charset", []byte(`{}`), "GET", "application/json; charset=utf-8", KindJSON},
        {"application/yaml", []byte("a: 1\n"), "GET", "application/yaml", KindYAML},
        {"text/yaml", []byte("a: 1\n"), "GET", "text/yaml", KindYAML},
        {"application/xml", []byte("<root/>"), "GET", "application/xml", KindXML},
        {"text/xml", []byte("<root/>"), "GET", "text/xml", KindXML},
        {"vendor +xml", []byte("<x/>"), "GET", "application/atom+xml", KindXML},
        {"text/html", []byte("<html></html>"), "GET", "text/html", KindHTML},
        {"text/plain", []byte("hello"), "GET", "text/plain", KindText},
        {"text/markdown", []byte("# hi"), "GET", "text/markdown", KindText},
        {"image/png", pngHeader, "GET", "image/png", KindBinary},
        {"octet-stream nul", []byte{0x00, 0x01}, "GET", "application/octet-stream", KindBinary},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Classify(tt.body, tt.method, tt.contentType)
            if got != tt.want {
                t.Errorf("Classify() = %v, want %v", got, tt.want)
            }
        })
    }
}

func TestMarkdown_ContentType_FallbackSniff(t *testing.T) {
    tests := []struct {
        name string
        body []byte
        want Kind
    }{
        {"no header json object", []byte(`{"a":1}`), KindJSON},
        {"no header json array", []byte(`[1,2,3]`), KindJSON},
        {"no header json with leading whitespace", []byte("  {\"a\":1}"), KindJSON},
        {"no header binary nul", []byte{0x00, 0xff}, KindBinary},
        {"no header invalid utf8", []byte{0xff, 0xfe}, KindBinary},
        {"no header plain text", []byte("hello world"), KindText},
        {"unparseable header text", []byte("hi"), KindText},     // contentType="???"
        {"unparseable header json", []byte("{}"), KindJSON},     // contentType="!!!"
    }
    // ... loop, calling Classify with empty contentType for the no-header cases
    //     and "???"/"!!!" for the unparseable ones.
}
```

#### Impact on Existing Tests

None — pure addition.

### Step 2: Body-cap helper (zero call sites yet)

**Rationale:** Independent helper; can be unit-tested in isolation. No
dependency on classifier or renderer. Lands second so step 4 has both
ingredients ready.

#### Files to Modify

| File                                       | Action | Description                                        |
|--------------------------------------------|--------|----------------------------------------------------|
| `internal/output/markdown/bodycap.go`      | create | `BodyCapBytes` constant + `truncateForMarkdown`.   |
| `internal/output/markdown/bodycap_test.go` | create | `TestMarkdown_BodyCap`, `TestMarkdown_BodyCap_PostRedaction`. |

#### New Code (`bodycap.go`)

```go
package markdown

// BodyCapBytes is the maximum number of body bytes rendered in markdown.
// Bodies larger than this are truncated and a footer marker is emitted.
// The cap is applied AFTER redaction so secret rewrites can never be
// split across the truncation boundary.
const BodyCapBytes = 1 << 20 // 1 MiB

// truncateForMarkdown returns body unchanged if its length is at or below
// cap. Otherwise the first cap bytes are returned along with truncated=true
// and the original (pre-cap) length. Callers use original to render the
// truncation marker.
func truncateForMarkdown(body []byte, cap int) (out []byte, truncated bool, original int) {
    if len(body) <= cap {
        return body, false, len(body)
    }
    return body[:cap], true, len(body)
}
```

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_BodyCap(t *testing.T) {
    tests := []struct {
        name           string
        in             []byte
        cap            int
        wantOut        []byte
        wantTruncated  bool
        wantOriginal   int
    }{
        {"empty", []byte{}, 1024, []byte{}, false, 0},
        {"under cap", []byte("hello"), 1024, []byte("hello"), false, 5},
        {"at cap", bytes.Repeat([]byte("x"), 1024), 1024, bytes.Repeat([]byte("x"), 1024), false, 1024},
        {"over cap", bytes.Repeat([]byte("x"), 1025), 1024, bytes.Repeat([]byte("x"), 1024), true, 1025},
        {"1 MiB cap with 2 MiB body", bytes.Repeat([]byte("y"), 2<<20), 1 << 20, bytes.Repeat([]byte("y"), 1<<20), true, 2 << 20},
    }
    // ... loop with truncateForMarkdown.
}

// Proves that redaction shrinking a body below the cap does NOT trigger
// truncation: only post-redaction size matters.
func TestMarkdown_BodyCap_PostRedaction(t *testing.T) {
    // 2 MiB raw input, redacted to 100 bytes by the time it reaches the cap helper.
    redactedSmall := []byte("[REDACTED] secret content here, originally 2 MiB on the wire")
    out, trunc, _ := truncateForMarkdown(redactedSmall, BodyCapBytes)
    if trunc {
        t.Error("redaction-shrunk body should not be truncated")
    }
    if !bytes.Equal(out, redactedSmall) {
        t.Error("expected unchanged bytes for sub-cap input")
    }

    // 2 MiB redacted body still over cap: truncate.
    big := bytes.Repeat([]byte("x"), 2<<20)
    out2, trunc2, orig := truncateForMarkdown(big, BodyCapBytes)
    if !trunc2 {
        t.Error("over-cap body should be truncated")
    }
    if len(out2) != 1<<20 || orig != 2<<20 {
        t.Errorf("unexpected sizes: out=%d original=%d", len(out2), orig)
    }
}
```

#### Impact on Existing Tests

None.

### Step 3: Volatile-header set + metadata renderer

**Rationale:** Standalone helper that takes `http.Header` and writes
metadata. No call site yet. Smaller than step 4 because it owns one
concern only — the closed set + sorted output. Step 4 calls it.

#### Files to Modify

| File                                          | Action | Description                                                        |
|-----------------------------------------------|--------|--------------------------------------------------------------------|
| `internal/output/markdown/metadata.go`        | create | `volatileHeaders` set, `IsVolatileHeader`, `renderMetadata`.       |
| `internal/output/markdown/metadata_test.go`   | create | `TestMarkdown_VolatileHeaderSet`, `TestMarkdown_RenderMetadata`.   |

#### New Code (`metadata.go`)

```go
package markdown

import (
    "bytes"
    "cmp"
    "fmt"
    "net/http"
    "slices"
)

// volatileHeaders is the closed set of response headers that are filtered
// out of the request and response signal blocks and rendered exclusively
// inside the ### Response metadata subsection. Keys are stored in their
// http.CanonicalHeaderKey form.
//
// Adding to this set is a behaviour change; do not extend without a
// corresponding task and golden update.
var volatileHeaders = map[string]struct{}{
    "Date":         {},
    "X-Request-Id": {}, // CanonicalHeaderKey lowercases everything after the first hyphen segment
    "Set-Cookie":   {},
    "Etag":         {},
    "Server":       {},
    "Age":          {},
}

// IsVolatileHeader reports whether name (case-insensitive) is in the
// volatile-header set.
func IsVolatileHeader(name string) bool {
    _, ok := volatileHeaders[http.CanonicalHeaderKey(name)]
    return ok
}

// renderMetadata writes the body of the ### Response metadata subsection.
// Headers are emitted in alphabetical order. Content-Length is synthesised
// from the original (post-redaction, pre-truncation) body size when the
// response did not include it on the wire.
//
// Empty headers map renders nothing (caller decides whether to emit the
// heading).
func renderMetadata(w *bytes.Buffer, h http.Header, originalBodyLen int) {
    if h == nil && originalBodyLen == 0 {
        return
    }
    keys := make([]string, 0, len(h))
    for k := range h {
        keys = append(keys, k)
    }
    slices.SortFunc(keys, cmp.Compare)
    for _, k := range keys {
        for _, v := range h[k] {
            fmt.Fprintf(w, "%s: %s\n", k, v)
        }
    }
}
```

(Note: `Etag` is the canonical form of `ETag` after
`http.CanonicalHeaderKey`. Test `TestMarkdown_VolatileHeaderSet` proves
the canonical normalisation.)

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_VolatileHeaderSet(t *testing.T) {
    cases := []struct {
        name string
        in   string
        want bool
    }{
        {"Date", "Date", true},
        {"date lowercase", "date", true},
        {"DATE upper", "DATE", true},
        {"X-Request-ID", "X-Request-ID", true},
        {"x-request-id lowercase", "x-request-id", true},
        {"Set-Cookie", "Set-Cookie", true},
        {"set-cookie lowercase", "set-cookie", true},
        {"ETag canonical", "ETag", true},
        {"etag lowercase", "etag", true},
        {"Server", "Server", true},
        {"Age", "Age", true},
        {"Content-Type not volatile", "Content-Type", false},
        {"Content-Length not volatile", "Content-Length", false},
        {"Authorization not volatile", "Authorization", false},
        {"X-Custom not volatile", "X-Custom-Header", false},
    }
    for _, tt := range cases {
        t.Run(tt.name, func(t *testing.T) {
            if got := IsVolatileHeader(tt.in); got != tt.want {
                t.Errorf("IsVolatileHeader(%q) = %v, want %v", tt.in, got, tt.want)
            }
        })
    }
}

func TestMarkdown_RenderMetadata(t *testing.T) {
    h := http.Header{
        "Content-Type":   {"application/json"},
        "Content-Length": {"42"},
        "Cache-Control":  {"no-cache"},
        "X-Trace-Id":     {"abc"},
    }
    var buf bytes.Buffer
    renderMetadata(&buf, h, 42)
    got := buf.String()
    // Sorted: Cache-Control, Content-Length, Content-Type, X-Trace-Id
    want := "Cache-Control: no-cache\nContent-Length: 42\nContent-Type: application/json\nX-Trace-Id: abc\n"
    if got != want {
        t.Errorf("renderMetadata mismatch:\n got: %q\nwant: %q", got, want)
    }
}
```

#### Impact on Existing Tests

None — new file, new helpers.

### Step 4: Wire renderers + metadata + cap into formatter

**Rationale:** Aggregates steps 1-3 into the public `WriteReport` path.
This is the largest blast-radius change because it adds a new section
to every per-request markdown file (volatile filter on request and
response signal, plus the new metadata block). Existing M9-002 goldens
update in step 6.

#### Files to Modify

| File                                          | Action | Description                                                                          |
|-----------------------------------------------|--------|--------------------------------------------------------------------------------------|
| `internal/output/markdown/formatter.go`       | modify | Add `Method` to be passed down to `renderResponse`. Replace inline non-JSON fallback with `renderBody(kind, ...)` dispatch. Add `renderBody`. Insert `### Response metadata` block. Filter volatile headers in `renderRequest` and the response signal block. |
| `internal/output/markdown/formatter_test.go`  | modify | Add `TestMarkdown_Render_Text`, `TestMarkdown_Render_Empty`, `TestMarkdown_Render_HEAD`, `TestMarkdown_Render_Binary`, `TestMarkdown_Render_YAML`, `TestMarkdown_Render_XML`, `TestMarkdown_Render_HTML`, `TestMarkdown_VolatileHeaders`. |

#### Current Code (`formatter.go:200-260`)

```go
func renderRequest(w *bytes.Buffer, entry *RequestEntry) {
    fmt.Fprintf(w, "%s %s\n", entry.Method, entry.URL)
    if len(entry.RequestHdr) > 0 {
        keys := slices.SortedFunc(func(yield func(string) bool) {
            for k := range entry.RequestHdr {
                if !yield(k) {
                    return
                }
            }
        }, cmp.Compare)
        for _, k := range keys {
            fmt.Fprintf(w, "%s: %s\n", k, entry.RequestHdr[k])
        }
    }
    if entry.RequestBody != nil {
        body, _ := json.Marshal(entry.RequestBody)
        if len(body) > 0 && string(body) != "null" {
            fmt.Fprintln(w, "```json")
            // ... pretty-print ...
        }
    }
}

func renderResponse(w *bytes.Buffer, entry *RequestEntry) {
    ct := lookupHeader(entry.RespHeaders, "Content-Type")
    if isJSONContentType(ct) {
        // ... json.Indent ...
    }
    if len(entry.RespBody) > 0 {
        // ... raw fence fallback ...
    }
}
```

#### New Code (`formatter.go`)

```go
func renderRequest(w *bytes.Buffer, entry *RequestEntry) {
    fmt.Fprintf(w, "%s %s\n", entry.Method, entry.URL)
    if len(entry.RequestHdr) > 0 {
        keys := slices.SortedFunc(func(yield func(string) bool) {
            for k := range entry.RequestHdr {
                if !yield(k) {
                    return
                }
            }
        }, cmp.Compare)
        for _, k := range keys {
            if IsVolatileHeader(k) {
                continue
            }
            fmt.Fprintf(w, "%s: %s\n", k, entry.RequestHdr[k])
        }
    }
    if entry.RequestBody != nil {
        body, _ := json.Marshal(entry.RequestBody)
        if len(body) > 0 && string(body) != "null" {
            renderJSONBody(w, body)
        }
    }
}

// renderResponse writes the response signal: code fence body block then a
// blank line. Volatile headers are NOT shown here; the request- and response-
// signal-block contracts forbid them. They appear only in renderResponseMetadata.
func renderResponse(w *bytes.Buffer, entry *RequestEntry) {
    ct := lookupHeader(entry.RespHeaders, "Content-Type")
    capped, truncated, original := truncateForMarkdown(entry.RespBody, BodyCapBytes)
    kind := Classify(capped, entry.Method, ct)
    renderBody(w, kind, capped, original, truncated)
}

// renderResponseMetadata emits the `### Response metadata` section: the body-
// signal-free home for Content-Type, Content-Length, and all other headers
// (post-redaction, alphabetical), including the volatile-header set.
func renderResponseMetadata(w *bytes.Buffer, entry *RequestEntry) {
    if len(entry.RespHeaders) == 0 {
        return
    }
    renderMetadata(w, entry.RespHeaders, len(entry.RespBody))
}

// renderBody dispatches per-Kind rendering. truncated/original carry the
// post-redaction-pre-truncation size used by KindBinary's footer line and
// by every kind's truncation marker.
func renderBody(w *bytes.Buffer, kind Kind, body []byte, original int, truncated bool) {
    switch kind {
    case KindHEAD:
        fmt.Fprintln(w, "_(HEAD — no body)_")
    case KindEmpty:
        fmt.Fprintln(w, "_(empty body)_")
    case KindJSON:
        renderJSONBody(w, body)
    case KindYAML:
        renderYAMLBody(w, body)
    case KindXML:
        renderFenced(w, "xml", body)
    case KindHTML:
        renderFenced(w, "html", body)
    case KindText:
        renderFenced(w, "text", body)
    case KindBinary:
        renderBinaryBody(w, body, original)
    }
    if truncated {
        fmt.Fprintf(w, "_... truncated (body was %d bytes, showing first %d)_\n", original, BodyCapBytes)
    }
}

// renderJSONBody is the M9-002 JSON pretty-printer extracted so request and
// response bodies share one path. Falls through to a raw text fence on
// json.Indent failure.
func renderJSONBody(w *bytes.Buffer, body []byte) {
    var pretty bytes.Buffer
    if err := json.Indent(&pretty, body, "", "  "); err == nil {
        fmt.Fprintln(w, "```json")
        w.Write(pretty.Bytes())
        if pretty.Len() == 0 || pretty.Bytes()[pretty.Len()-1] != '\n' {
            fmt.Fprintln(w)
        }
        fmt.Fprintln(w, "```")
        return
    }
    // Parse failure: emit raw text fence so the slot is never empty.
    renderFenced(w, "text", body)
}

// renderYAMLBody canonicalises body via gopkg.in/yaml.v3. On round-trip
// failure (malformed YAML on the wire) falls back to verbatim ```yaml.
func renderYAMLBody(w *bytes.Buffer, body []byte) {
    var v any
    if err := yaml.Unmarshal(body, &v); err == nil {
        var buf bytes.Buffer
        enc := yaml.NewEncoder(&buf)
        if err := enc.Encode(v); err == nil {
            _ = enc.Close()
            fmt.Fprintln(w, "```yaml")
            w.Write(buf.Bytes())
            if buf.Len() == 0 || buf.Bytes()[buf.Len()-1] != '\n' {
                fmt.Fprintln(w)
            }
            fmt.Fprintln(w, "```")
            return
        }
        _ = enc.Close()
    }
    renderFenced(w, "yaml", body)
}

// renderFenced emits body inside a ```<lang> fence with exactly one trailing
// newline before the closing fence.
func renderFenced(w *bytes.Buffer, lang string, body []byte) {
    fmt.Fprintf(w, "```%s\n", lang)
    w.Write(body)
    if len(body) == 0 || body[len(body)-1] != '\n' {
        fmt.Fprintln(w)
    }
    fmt.Fprintln(w, "```")
}

// renderBinaryBody writes the first 512 bytes via hex.Dump in a ```hexdump
// fence followed by the total-bytes footer.
func renderBinaryBody(w *bytes.Buffer, body []byte, original int) {
    const previewLimit = 512
    preview := body
    if len(preview) > previewLimit {
        preview = preview[:previewLimit]
    }
    fmt.Fprintln(w, "```hexdump")
    w.WriteString(hex.Dump(preview))
    fmt.Fprintln(w, "```")
    fmt.Fprintf(w, "_%d bytes total_\n", original)
}
```

Imports added to `formatter.go`: `encoding/hex`, `gopkg.in/yaml.v3`.
The unused `isJSONContentType` and the inline non-JSON fallback are
removed; the M9-002 export contract is unchanged.

In `renderSentinelBlock`, after the `### Response <status>` body and
before the `### Timing` heading, insert:

```go
// Section 7: ### Response metadata (volatile headers + Content-Type/Length, alphabetical)
fmt.Fprintf(&buf, "\n### Response metadata\n\n")
renderResponseMetadata(&buf, entry)
```

Section numbering doc-comment in `renderFullFile` is updated:

```
// Section order (11 markers):
//  1. # <name>
//  2. ## Notes
//  3. (empty notes paragraph — placeholder for agent text)
//  4. <!-- BEGIN apitest:response id=... slug=... run=... -->
//  5. ## Response (deterministic)
//  6. ### Request
//  7. ### Response <status>
//  8. ### Response metadata    (NEW: M9-003)
//  9. ### Timing
// 10. ### Assertions
// 11. <!-- END apitest:response id=... slug=... run=... -->
```

#### Tests to Write FIRST (RED)

```go
func TestMarkdown_Render_Text(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{"Content-Type": {"text/plain"}}
    report.Requests[0].RespBody = []byte("hello world")

    dir := t.TempDir()
    if err := WriteReport(report, dir, WriteOptions{}); err != nil {
        t.Fatalf("WriteReport: %v", err)
    }
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !regexp.MustCompile("(?m)^```text$").Match(got) {
        t.Errorf("expected ```text fence in:\n%s", got)
    }
    if !strings.Contains(string(got), "hello world") {
        t.Errorf("expected verbatim text in:\n%s", got)
    }
}

func TestMarkdown_Render_Empty(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{}
    report.Requests[0].RespBody = nil
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !strings.Contains(string(got), "_(empty body)_") {
        t.Errorf("expected _(empty body)_ marker:\n%s", got)
    }
}

func TestMarkdown_Render_HEAD(t *testing.T) {
    report := makePassReport()
    report.Requests[0].Method = "HEAD"
    report.Requests[0].RespBody = nil
    report.Requests[0].RespHeaders = http.Header{"Content-Length": {"42"}} // header lies; HEAD wins
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !strings.Contains(string(got), "_(HEAD") {
        t.Errorf("expected HEAD marker:\n%s", got)
    }
}

func TestMarkdown_Render_Binary(t *testing.T) {
    pngHeader := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
    body := append(pngHeader, bytes.Repeat([]byte{0xff}, 600)...)
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{"Content-Type": {"image/png"}}
    report.Requests[0].RespBody = body
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !regexp.MustCompile("(?m)^```hexdump$").Match(got) {
        t.Errorf("expected ```hexdump fence:\n%s", got)
    }
    if !regexp.MustCompile(`_\d+ bytes total_`).Match(got) {
        t.Errorf("expected total-bytes footer:\n%s", got)
    }
}

func TestMarkdown_Render_YAML(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{"Content-Type": {"application/yaml"}}
    report.Requests[0].RespBody = []byte("name: Alice\nage: 30\n")
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !regexp.MustCompile("(?m)^```yaml$").Match(got) {
        t.Errorf("expected ```yaml fence:\n%s", got)
    }
}

func TestMarkdown_Render_XML(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{"Content-Type": {"application/xml"}}
    report.Requests[0].RespBody = []byte("<root><a/></root>")
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !regexp.MustCompile("(?m)^```xml$").Match(got) {
        t.Errorf("expected ```xml fence:\n%s", got)
    }
}

func TestMarkdown_Render_HTML(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{"Content-Type": {"text/html"}}
    report.Requests[0].RespBody = []byte("<html><body>hi</body></html>")
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    if !regexp.MustCompile("(?m)^```html$").Match(got) {
        t.Errorf("expected ```html fence:\n%s", got)
    }
}

func TestMarkdown_VolatileHeaders(t *testing.T) {
    report := makePassReport()
    report.Requests[0].RespHeaders = http.Header{
        "Content-Type":  {"application/json"},
        "Date":          {"Thu, 25 Apr 2026 12:00:00 GMT"},
        "X-Request-Id":  {"abc-123"},
        "Set-Cookie":    {"session=xyz"},
        "Etag":          {`"v1"`},
        "Server":        {"nginx"},
        "Age":           {"60"},
        "Cache-Control": {"no-cache"},
    }
    dir := t.TempDir()
    _ = WriteReport(report, dir, WriteOptions{})
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))

    // Volatile headers must NOT appear in the response signal block (between
    // `### Response 200` and `### Response metadata`).
    signalRE := regexp.MustCompile(`### Response 200\n([\s\S]*?)### Response metadata`)
    sig := signalRE.FindSubmatch(got)
    if sig == nil {
        t.Fatalf("could not find response signal block:\n%s", got)
    }
    for _, name := range []string{"Date:", "X-Request-Id:", "Set-Cookie:", "Etag:", "Server:", "Age:"} {
        if bytes.Contains(sig[1], []byte(name)) {
            t.Errorf("volatile header %q leaked into signal block:\n%s", name, sig[1])
        }
    }

    // Volatile headers MUST appear in the metadata block.
    metaRE := regexp.MustCompile(`### Response metadata\n\n([\s\S]*?)### Timing`)
    meta := metaRE.FindSubmatch(got)
    if meta == nil {
        t.Fatalf("could not find metadata block:\n%s", got)
    }
    for _, name := range []string{"Date:", "X-Request-Id:", "Set-Cookie:", "Etag:", "Server:", "Age:"} {
        if !bytes.Contains(meta[1], []byte(name)) {
            t.Errorf("volatile header %q missing from metadata:\n%s", name, meta[1])
        }
    }
}
```

#### Impact on Existing Tests

- `TestMarkdown_Render_PassJSON` — markdown structure gains the
  `### Response metadata` heading. Test asserts presence of 10 section
  markers; we add one more to the regex list, regenerate the
  `pass_json.md` golden in step 6, and the test passes.
- `TestMarkdown_Render_FailJSON` — same story; goldens regenerated.
- `TestMarkdown_EmptyAssertions` — same story.
- `TestMarkdown_RenderRequest_Headers` — passes unchanged (test headers
  are Authorization/Accept/X-Request-ID; X-Request-ID is now filtered out
  of the request signal block, so we update the assertion to look for it
  in the metadata block instead). Or, change the test fixture headers to
  Accept/Authorization/X-Custom-Header so the rendering logic stays
  unchanged.

Decision: **change the fixture** to avoid the volatile X-Request-ID
making the test about two concerns. Replace `X-Request-ID` with
`X-Custom-Header: my-value` in the fixture so the test continues to
verify deterministic alphabetical ordering of non-volatile headers.

- `TestMarkdown_RenderRequest_HeadersDeterminism` — uses Z-Last/A-First/
  M-Mid which are all non-volatile; unchanged.
- `TestMarkdown_DeterminismRegression` — `maskVolatileLines` already
  masks duration/wave/started; the new metadata block contains
  Content-Type which is stable; passes unchanged. Goldens regenerated.
- `TestMarkdown_RunMD*` — run.md does not render bodies; unchanged.
- `TestRun_MarkdownFormat_HappyPath` (cmd/apitest/run_test.go) — the
  10-marker list expands by one; we add `"### Response metadata"` to
  the markers slice. No semantic change.
- `TestRun_MarkdownFormat_SpliceOnRerun` — verifies `AGENT NOTE`
  preservation outside sentinels; passes unchanged.

### Step 5: Redaction-invariant test (formatter + cmd integration)

**Rationale:** Pure assertion harness. Once the formatter is wired we
prove that bytes labelled `[REDACTED]` arriving at the formatter never
get re-de-redacted. This is the single most important regression guard
in the slice. Splits into a unit test (formatter level) and an
integration test (cmd level via `--allow-sensitive`).

#### Files to Modify

| File                                              | Action | Description                                                                  |
|---------------------------------------------------|--------|------------------------------------------------------------------------------|
| `internal/output/markdown/formatter_test.go`      | modify | Add `TestMarkdown_RedactionInvariant` (formatter-level: pre-redacted body).  |
| `cmd/apitest/run_test.go`                         | modify | Add `TestRun_MarkdownFormat_RedactionInvariant` with `!sensitive` collection. |

#### New Code (formatter_test.go)

```go
func TestMarkdown_RedactionInvariant(t *testing.T) {
    report := makePassReport()
    // Body and headers arrive at the formatter already redacted (mimicking
    // what main.go does at line 1442-1450 before calling buildMarkdownReport).
    report.Requests[0].RespHeaders = http.Header{
        "Content-Type":  {"application/json"},
        "Authorization": {"Bearer [REDACTED]"},
    }
    report.Requests[0].RespBody = []byte(`{"token":"[REDACTED]"}`)
    report.Requests[0].RequestHdr = map[string]string{
        "Authorization": "Bearer [REDACTED]",
    }
    report.Requests[0].RequestBody = map[string]any{"secret": "[REDACTED]"}

    dir := t.TempDir()
    if err := WriteReport(report, dir, WriteOptions{}); err != nil {
        t.Fatalf("WriteReport: %v", err)
    }
    got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
    // Must contain [REDACTED] tokens (proving headers/body flowed through).
    if bytes.Count(got, []byte("[REDACTED]")) < 4 {
        t.Errorf("expected at least 4 [REDACTED] tokens, got %d:\n%s", bytes.Count(got, []byte("[REDACTED]")), got)
    }
    // Must NOT contain any non-redacted secret-looking values. The fixture has
    // none; this is a defence against future regressions.
    for _, secret := range []string{"sk_live_", "ghp_", "Bearer eyJ"} {
        if bytes.Contains(got, []byte(secret)) {
            t.Errorf("unexpected raw secret %q leaked into markdown:\n%s", secret, got)
        }
    }
}
```

#### New Code (run_test.go)

```go
// TestRun_MarkdownFormat_RedactionInvariant proves that --allow-sensitive
// does NOT flow through to the markdown formatter: even when the flag is
// set, body and headers in the .md output remain redacted because main.go
// rewrites results[i] before dispatch (see main.go:1442-1450).
func TestRun_MarkdownFormat_RedactionInvariant(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("Authorization", "Bearer sk_live_secret123")
        _, _ = w.Write([]byte(`{"token":"sk_live_secret123"}`))
    }))
    defer srv.Close()

    tmpDir := t.TempDir()
    col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
variables:
  - name: token
    value: sk_live_secret123
    !sensitive
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
      headers:
        Authorization: "Bearer {{token}}"
`, srv.URL))
    reportDir := filepath.Join(tmpDir, "resp")

    _, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir, "--allow-sensitive")
    if code != 0 {
        t.Fatalf("exit code = %d", code)
    }
    got, _ := os.ReadFile(filepath.Join(reportDir, "get-user.md"))
    if bytes.Contains(got, []byte("sk_live_secret123")) {
        t.Errorf("raw secret leaked into markdown despite redaction:\n%s", got)
    }
    if !bytes.Contains(got, []byte("[REDACTED]")) {
        t.Errorf("expected [REDACTED] in markdown output:\n%s", got)
    }
}
```

#### Impact on Existing Tests

None.

### Step 6: Regenerate goldens + smoke

**Rationale:** All M9-002 goldens contain the now-changed section
order (added `### Response metadata`). Running with `UPDATE_GOLDEN=1`
regenerates them; we hand-inspect the diff and commit.

#### Files to Modify

| File                                                           | Action | Description                                  |
|----------------------------------------------------------------|--------|----------------------------------------------|
| `internal/output/markdown/testdata/golden/pass_json.md`        | modify | regen via UPDATE_GOLDEN=1                    |
| `internal/output/markdown/testdata/golden/fail_json.md`        | modify | regen via UPDATE_GOLDEN=1                    |
| `internal/output/markdown/testdata/golden/empty_assertions.md` | modify | regen via UPDATE_GOLDEN=1                    |
| `internal/output/markdown/testdata/golden/run_md.md`           | modify | only changes if section numbering changes; otherwise unchanged |

Verification: `go test ./internal/output/markdown/...` passes after
regen; visual diff must show only the inserted `### Response metadata`
section.

#### Impact on Existing Tests

All M9-002 markdown tests now read the new goldens; no test code
changes (the `compareOrUpdateGolden` helper is the same).

## Test Impact Summary

| Test File                                          | Test Function                                      | Impact   | Action Required                                              |
|----------------------------------------------------|----------------------------------------------------|----------|--------------------------------------------------------------|
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_Render_PassJSON`                     | breaks   | append `### Response metadata` marker to expected list; regen golden |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_Render_FailJSON`                     | breaks   | regen golden only                                            |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_EmptyAssertions`                     | breaks   | regen golden only                                            |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_Newlines`                            | none     | section count changes but the test asserts CRLF, not section list |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_RenderRequest_Headers`               | breaks   | swap `X-Request-ID` -> `X-Custom-Header` in fixture (it would now be filtered)         |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_RenderRequest_JSONBody`              | none     | unchanged                                                    |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_RenderRequest_HeadersDeterminism`    | none     | unchanged                                                    |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_SentinelExactFormat`                 | none     | unchanged                                                    |
| `internal/output/markdown/regression_test.go`      | `TestMarkdown_DeterminismRegression`               | none     | mask still applies; regen golden                             |
| `internal/output/markdown/run_md_test.go`          | `TestMarkdown_RunMD`, `TestMarkdown_RunMD_WithEnvName` | none | run.md does not embed bodies                                 |
| `internal/output/markdown/contenttype_test.go`     | NEW                                                | adds     | step 1                                                       |
| `internal/output/markdown/bodycap_test.go`         | NEW                                                | adds     | step 2                                                       |
| `internal/output/markdown/metadata_test.go`        | NEW                                                | adds     | step 3                                                       |
| `internal/output/markdown/formatter_test.go`       | `TestMarkdown_Render_{Text,Empty,HEAD,Binary,YAML,XML,HTML}`, `TestMarkdown_VolatileHeaders`, `TestMarkdown_RedactionInvariant` | adds | step 4-5 |
| `cmd/apitest/run_test.go`                          | `TestRun_MarkdownFormat_HappyPath`                 | breaks   | append `### Response metadata` to markers slice              |
| `cmd/apitest/run_test.go`                          | `TestRun_MarkdownFormat_RedactionInvariant`        | NEW      | step 5                                                       |

## Risks and Edge Cases

- **Risk:** YAML round-trip via `yaml.v3` may reorder map keys, causing
  byte-instability across runs.
  **Mitigation:** `yaml.v3` Encoder iterates Go map keys via Go's
  randomised map iteration ordering. To guarantee determinism we
  unmarshal into `yaml.Node` and re-encode the node verbatim — yaml.v3
  preserves source order in `yaml.Node`. The renderer changes from
  `yaml.Unmarshal(body, &v); enc.Encode(v)` to
  `var n yaml.Node; yaml.Unmarshal(body, &n); enc.Encode(&n)`. Test
  `TestMarkdown_Render_YAML_Determinism` runs encode twice and asserts
  byte equality.

- **Risk:** Binary classifier mis-classifies UTF-8 with embedded NULs
  (rare but possible in protobuf-encoded text).
  **Mitigation:** Documented limitation matching events emitter
  behavior. Adding a third heuristic is out of scope; if real-world
  demand surfaces it can be revisited.

- **Risk:** `mime.ParseMediaType` is strict and rejects whitespace
  variations like `application/json ; charset=utf-8` (note the space
  before `;`).
  **Mitigation:** `mime.ParseMediaType` actually tolerates this per
  RFC 2046; documented in stdlib. Test
  `TestMarkdown_ContentType` includes the `application/json;
  charset=utf-8` case to pin behaviour.

- **Edge case:** Body exactly 1048576 bytes — boundary case for the
  cap.
  **Handling:** `truncateForMarkdown` uses `len(body) <= cap` so 1 MiB
  exactly is not truncated. Tested by the "at cap" case in
  `TestMarkdown_BodyCap`.

- **Edge case:** Empty body with `Content-Type: application/json`.
  **Handling:** Classifier returns `KindEmpty` because the
  `len(body) == 0` check runs before mime parsing. Renders as
  `_(empty body)_`. Tested.

- **Edge case:** HEAD request with non-empty body bytes (servers
  sometimes do this incorrectly).
  **Handling:** `KindHEAD` wins over body presence; renders as
  `_(HEAD — no body)_`. Body bytes are discarded from rendering but
  remain accessible via the events stream / runner.

- **Edge case:** Binary body smaller than 512 bytes (the preview
  limit).
  **Handling:** `hex.Dump(body[:min(len(body), 512)])` shows the full
  body; `_<N> bytes total_` matches `len(body)`. No truncation. Tested
  via the PNG-header fixture (which is 8 + 600 = 608 bytes, exercising
  truncation).

- **Edge case:** Non-volatile headers with multiple values
  (`Set-Cookie` is the typical case).
  **Handling:** `renderMetadata` iterates `h[k]` so all values are
  emitted on separate lines with the same name. Test
  `TestMarkdown_RenderMetadata` includes a multi-value Set-Cookie.

- **Edge case:** `entry.Method == ""` (not yet populated for
  context-cancelled skips).
  **Handling:** `Classify("", body, ct)` falls through past the HEAD
  check (case-insensitive equality), so empty method is treated as
  non-HEAD. Tested explicitly.

- **Risk:** Removing `isJSONContentType` from formatter.go (was used by
  the old non-JSON path) breaks any downstream import.
  **Mitigation:** Function is unexported (lowercase first letter); no
  importers possible. Removing it is safe.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML; run after the implementation):

```bash
# Generated fixture collections under cmd/apitest/testdata/markdown/ exercise
# every Kind. The full spread:
./apitest run cmd/apitest/testdata/markdown/text.yaml --format markdown --report /tmp/resp
grep -A 1 '^```text$' /tmp/resp/echo-text.md | head -2

./apitest run cmd/apitest/testdata/markdown/empty.yaml --format markdown --report /tmp/resp
grep '_(empty body)_' /tmp/resp/health.md

./apitest run cmd/apitest/testdata/markdown/head.yaml --format markdown --report /tmp/resp
grep '_(HEAD' /tmp/resp/ping-head.md

./apitest run cmd/apitest/testdata/markdown/binary.yaml --format markdown --report /tmp/resp
grep '^```hexdump$' /tmp/resp/fetch-image.md
grep -E 'bytes total' /tmp/resp/fetch-image.md

./apitest run cmd/apitest/testdata/markdown/yaml-response.yaml --format markdown --report /tmp/resp
grep '^```yaml$' /tmp/resp/config-yaml.md

./apitest run cmd/apitest/testdata/markdown/xml-html.yaml --format markdown --report /tmp/resp
grep '^```xml$' /tmp/resp/fetch-xml.md
grep '^```html$' /tmp/resp/fetch-html.md

./apitest run cmd/apitest/testdata/markdown/large.yaml --format markdown --report /tmp/resp
grep 'truncated' /tmp/resp/huge.md
wc -c /tmp/resp/huge.md  # well under 2 MiB

./apitest run cmd/apitest/testdata/markdown/volatile.yaml --format markdown --report /tmp/resp
awk '/BEGIN apitest:response/,/### Response metadata/' /tmp/resp/volatile-demo.md | grep -Ec '^(Date|X-Request-ID|Set-Cookie|ETag|Server|Age):'  # 0
grep -A 20 '^### Response metadata' /tmp/resp/volatile-demo.md | grep -E '^(Date|X-Request-ID|Set-Cookie|ETag|Server|Age):'

./apitest run cmd/apitest/testdata/markdown/secret.yaml --format markdown --report /tmp/resp --allow-sensitive
grep 'sk_live' /tmp/resp/*.md  # no matches

# Full unit + integration suite
go test -run 'TestMarkdown_ContentType|TestMarkdown_Render_(Text|Empty|HEAD|Binary|YAML|XML|HTML)|TestMarkdown_BodyCap|TestMarkdown_VolatileHeaders|TestMarkdown_RedactionInvariant' ./...
```

The integration `cmd/apitest/run_test.go` test names will mirror these
collection names: `TestRun_MarkdownFormat_TextResponse`,
`...EmptyResponse`, `...HEADResponse`, `...BinaryResponse`,
`...YAMLResponse`, `...XMLHTMLResponses`, `...BodyCap`,
`...VolatileHeaders`, `...RedactionInvariant` (already named).

For the in-repo smoke path (no live HTTP), the integration tests
spin up `httptest.NewServer` instances that respond with the
appropriate Content-Type and body bytes, mirroring the `srv :=
httptest.NewServer(...)` pattern at the top of every existing
`TestRun_MarkdownFormat_*` test.
