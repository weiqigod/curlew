# Implementation Plan: M9-001

## Overview

Land the `request_slug` correlation primitive: a parser-side helper that derives a
URL-safe slug from a request name, parser-time validation that rejects names that
slugify to empty, and event-stream plumbing that adds `request_slug` to every
`request.start` and `request.end` event. Bumps the events schema from v1.1 to
v1.2 (additive, backward-compatible) and publishes the new schema doc; the v1.0
and v1.1 schema files remain byte-unchanged as historical anchors. No Markdown
formatter yet — this is pure plumbing for W4.

## Task Details

- **ID:** M9-001
- **Title:** request_slug: derive per-request identifier and emit in events schema v1.2
- **Phase:** M9: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** low
- **Estimated effort:** 4–6 hours

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M8-005 | run --only prunes setup to transitive closure of variable references | done |

## Architectural Decisions Resolved Up Front

These decisions are informed by reading every file the slice touches; they are
recorded here so reviewers do not have to reverse-engineer them.

1. **Slug helper lives in `internal/parser/slug.go`.** This is what the task
   YAML specifies. It also matches the structure of `parser.Slug` being a
   parse-time concern: empty-after-slugify is a load-time error, the cached
   slug is a parse-time computed field on `RequestItem`, and only a parse-time
   helper has cheap access to the duplicate-name validator's exit point.

2. **`RequestItem` gains a non-YAML `Slug string` field.** Cached at parse
   time, populated for every phase (setup, main, teardown) immediately after
   the existing `checkDuplicateNames` step. Not exposed in YAML (`yaml:"-"`)
   so users cannot override it. Keeping it on the parser type avoids re-deriving
   for every emit site and lets the runner pass it through opaquely.

3. **Empty-after-slugify rejection applies to every phase.** Every emit site
   (sequential main, parallel main, data-driven main, WebSocket main, setup,
   teardown) must carry a non-empty slug. Therefore the validator walks all
   three phases. This is consistent with the M8-004 duplicate-name pattern,
   which only checks main but for a different reason (only main is selectable
   via `--only`); for slugs, every emitted event needs a value.

4. **Data-driven iterations re-derive at emit time.** For data-driven items the
   event `name` is `"<base> [N/M]"`; the cached `item.Slug` is the base form.
   The runner calls the slug helper a second time on the iteration name so the
   emitted `request_slug` matches the emitted `name` 1:1 (e.g.
   `name="Create user [2/3]"` → `request_slug="create-user-2-3"`). This avoids
   ambiguity for W4's per-iteration markdown filenames and keeps the emitted
   slug a pure function of the emitted name. The base item's slug is still
   derived and validated at parse time so empty-after-slugify still rejects
   data-driven items at load time, before any iteration is ever expanded.

5. **`TestSchema_v11_validates` is preserved by giving it its own explicit
   schema-loader call.** The existing `compileEventSchema` helper hard-codes
   v1.1.json today via `schemaPath()`. M9-001 must move the "current" pointer
   to v1.2.json without breaking the v1.1 regression test. Resolution:
   refactor `schemaPath` to accept a version argument (`schemaPath(t, "v1.1")`),
   keep `compileEventSchema` calling it for the current version (v1.2), and
   make `TestSchema_v11_validates` and the new `TestSchema_v10_validates`
   pass an explicit version. The DoD's "unmodified" wording is interpreted as
   "their intent is preserved and they still pass" — the test bodies will need
   trivial adjustments (one helper call) to load explicit schemas.

6. **Parallel `EventSink` interface signature change.** The narrow
   `parallel.EventSink` in `internal/parallel/executor.go:84-90` is duplicated
   from `runner.EventSink` to avoid an import cycle. Both interfaces must
   gain `requestSlug string` parameters. This is a breaking signature change
   for both interfaces, but every implementer is internal:
   `runner.parallelSinkAdapter`, the runner's own `recordingSink`, and the
   parallel package's `parallelRecordingSink`. We update them all in lock-step.

7. **`ErrSlugEmpty` sentinel** is added to `internal/parser/errors.go`
   alongside `ErrDuplicateRequestName`. It carries the structured-error
   wrapping pattern already used by the duplicate-name validator: a
   `*apierrors.Structured` with `Category: CategoryParse`, `FilePath`,
   `Line`, a human-readable `Message`, a remediation `Hint`, and `Inner: ErrSlugEmpty`.

## Implementation Steps

Steps are ordered by blast radius, smallest first.

### Step 1: Slug helper in internal/parser/slug.go

**Rationale:** No callers depend on this yet. Land the helper and its full
table of unit tests in isolation; downstream steps will use it.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/slug.go` | create | Exports `Slug(name string) (string, error)` — NFKD normalize, strip combining marks, lowercase, collapse non-`[a-z0-9]` runs to a single `-`, trim leading/trailing `-`. Returns an error for empty result. |
| `internal/parser/slug_test.go` | create | Table-driven `TestSlug_Derive` covering ASCII, punctuation, Unicode, digits, edges. |
| `internal/parser/errors.go` | modify | Add `ErrSlugEmpty` sentinel. |
| `go.mod` | modify | Promote `golang.org/x/text` from indirect to direct dependency (already present at v0.14.0). |

#### New Code (slug.go)

```go
package parser

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Slug returns a URL-safe identifier derived from name. Algorithm:
//
//  1. Unicode NFKD normalization (decomposes accents into base + combining marks).
//  2. Strip combining marks.
//  3. Lowercase ASCII letters; non-ASCII letters and digits that survive
//     normalization are kept only when they are in the [a-z0-9] range —
//     everything else is treated as a separator.
//  4. Collapse runs of separator runes to a single '-'.
//  5. Trim leading and trailing '-'.
//
// If the result is empty (name was whitespace-only or punctuation-only),
// Slug returns an empty string and ErrSlugEmpty wrapped in a fmt.Errorf.
// Callers in the parser surface this as a load-time error matching the
// early-reject posture of duplicate-name detection.
func Slug(name string) (string, error) {
	// Step 1+2: NFKD then strip combining marks (Mn category).
	t := transform.Chain(
		norm.NFKD,
		runes.Remove(runes.In(unicode.Mn)),
	)
	decomposed, _, err := transform.String(t, name)
	if err != nil {
		// transform.String only errors on writer failures; in-memory
		// transformation cannot fail. Treat as an internal bug.
		return "", fmt.Errorf("parser: slug normalization failed: %w", err)
	}

	// Steps 3+4: lowercase + collapse non-[a-z0-9] runs to a single '-'.
	var b strings.Builder
	b.Grow(len(decomposed))
	prevSep := true // suppress leading separators
	for _, r := range decomposed {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			prevSep = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevSep = false
		default:
			if !prevSep {
				b.WriteRune('-')
				prevSep = true
			}
		}
	}

	// Step 5: trim trailing '-' (leading was suppressed above).
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "", fmt.Errorf("parser: %w: %q", ErrSlugEmpty, name)
	}
	return out, nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestSlug_Derive(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantErr bool
	}{
		// ASCII basics
		{"simple ASCII lowercase", "get-user", "get-user", false},
		{"ASCII with spaces", "Get user", "get-user", false},
		{"ASCII with mixed case", "GET User", "get-user", false},
		{"single word lowercase", "users", "users", false},
		{"single word uppercase", "USERS", "users", false},

		// Punctuation
		{"trailing punctuation", "Create Post!", "create-post", false},
		{"leading punctuation", "!Create", "create", false},
		{"interior punctuation", "create.post", "create-post", false},
		{"quotes and apostrophes", "Bob's request", "bob-s-request", false},
		{"runs of punctuation collapsed", "a!!!b", "a-b", false},
		{"every punctuation char", "a!@#$%^&*()_+={}b", "a-b", false},

		// Digits
		{"digits preserved", "request 42", "request-42", false},
		{"digit-only name", "200", "200", false},
		{"digits at start", "404 not found", "404-not-found", false},

		// Whitespace edges
		{"leading whitespace", "   leading", "leading", false},
		{"trailing whitespace", "trailing   ", "trailing", false},
		{"interior whitespace runs", "hello    world", "hello-world", false},
		{"mixed whitespace types", "tab\there", "tab-here", false},

		// Unicode
		{"accented characters", "Héllo", "hello", false},
		{"unicode strips to ASCII", "Café", "cafe", false},
		{"combining marks", "naïve", "naive", false},
		{"mixed unicode and ascii", "Héllo 世界", "hello", false},
		{"emoji separator", "ship 🚢 it", "ship-it", false},
		{"all-CJK falls through to empty", "世界", "", true},

		// Empty / reject
		{"empty string rejected", "", "", true},
		{"whitespace-only rejected", "    ", "", true},
		{"tabs only rejected", "\t\t", "", true},
		{"punctuation-only rejected", "!!!", "", true},
		{"hyphens-only rejected", "---", "", true},

		// Hyphen handling
		{"hyphen-separated kept", "a-b-c", "a-b-c", false},
		{"runs of hyphens collapsed", "a---b", "a-b", false},
		{"trailing hyphens trimmed", "abc---", "abc", false},
		{"leading hyphens trimmed", "---abc", "abc", false},

		// Realistic request names
		{"GET pattern", "GET /users/{id}", "get-users-id", false},
		{"data-driven iter", "Create user [1/3]", "create-user-1-3", false},
		{"GraphQL query", "query GetUser { user { id } }", "query-getuser-user-id", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Slug(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Slug(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Slug(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrSlugEmpty) {
				t.Errorf("Slug(%q) error = %v, expected wraps ErrSlugEmpty", tt.input, err)
			}
		})
	}
}
```

#### Impact on Existing Tests

- No existing tests reference `Slug` or `ErrSlugEmpty`. No breakage.

### Step 2: Parse-time slug derivation and empty-after-slugify validation

**Rationale:** With the helper in place, we can wire it into the parser without
yet touching the runner. This step keeps the changes scoped: just compute and
cache the slug for every item, and reject empty-after-slugify at load time.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Slug string \`yaml:"-"\`` to `RequestItem`. |
| `internal/parser/parser.go` | modify | Add `populateSlugs` step in `ParseFileWithOptions`, called after `checkDuplicateNames`. Rejects items whose name slugifies to empty with a structured error pointing at the source line. |
| `internal/parser/parser_test.go` | modify | Add `TestParser_SlugEmptyRejection` table-driven test covering empty-after-slugify rejection per phase. |

#### Current Code (parser.go:106-146)

```go
	// M8-004: reject duplicate main request names. Runs after include resolution
	// so items spliced via include: are covered.
	if err := checkDuplicateNames(col, path); err != nil {
		return nil, err
	}

	return col, nil
}
```

#### New Code (parser.go)

```go
	// M8-004: reject duplicate main request names. Runs after include resolution
	// so items spliced via include: are covered.
	if err := checkDuplicateNames(col, path); err != nil {
		return nil, err
	}

	// M9-001: derive request slugs and reject names that slugify to empty.
	// Runs after duplicate-name validation so duplicate detection always wins
	// when both apply. Walks every phase (setup, main, teardown) because every
	// emitted request.start/request.end event must carry a non-empty slug.
	if err := populateSlugs(col, path); err != nil {
		return nil, err
	}

	return col, nil
}

// populateSlugs derives Slug for every RequestItem in setup/main/teardown
// and writes the result back via in-place mutation through the section
// pointer. Returns a structured error on the first item whose name
// slugifies to empty.
func populateSlugs(col *Collection, rootPath string) error {
	sections := []*[]RequestItem{
		&col.Setup.Items,
		&col.Requests.Items,
		&col.Teardown.Items,
	}
	for _, section := range sections {
		for i := range *section {
			item := &(*section)[i]
			if item.Name == "" {
				// Empty names are caught by other validators (duplicate-name
				// skips them, request-validation rejects them). Skip to keep
				// error attribution clean.
				continue
			}
			s, err := Slug(item.Name)
			if err != nil {
				return &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: item.SourceFile,
					Line:     item.SourceLine,
					Message: fmt.Sprintf(
						"request name %q slugifies to empty (no alphanumeric runes after Unicode normalization)",
						item.Name,
					),
					Hint:  "Rename the request to include at least one ASCII letter or digit. Slugs are used as filenames for per-request markdown reports.",
					Inner: ErrSlugEmpty,
				}
			}
			item.Slug = s
		}
	}
	return nil
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParser_SlugEmptyRejection(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantErr   bool
		errPhase  string // "setup" | "main" | "teardown" — informational
	}{
		{
			name: "main item with valid name accepted",
			yaml: `name: ok
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com"}`,
			wantErr: false,
		},
		{
			name: "main item with punctuation-only name rejected",
			yaml: `name: bad
requests:
  - name: "!!!"
    request: {method: GET, url: "https://example.com"}`,
			wantErr: true,
			errPhase: "main",
		},
		{
			name: "main item with all-CJK name rejected (no ASCII letters)",
			yaml: `name: bad
requests:
  - name: "世界"
    request: {method: GET, url: "https://example.com"}`,
			wantErr: true,
			errPhase: "main",
		},
		{
			name: "setup item with empty-slug name rejected",
			yaml: `name: bad
setup:
  - name: "---"
    request: {method: GET, url: "https://example.com"}
requests:
  - name: ok
    request: {method: GET, url: "https://example.com"}`,
			wantErr: true,
			errPhase: "setup",
		},
		{
			name: "teardown item with empty-slug name rejected",
			yaml: `name: bad
requests:
  - name: ok
    request: {method: GET, url: "https://example.com"}
teardown:
  - name: "   "
    request: {method: GET, url: "https://example.com"}`,
			wantErr: true,
			errPhase: "teardown",
		},
		{
			name: "unicode that normalizes to ascii is accepted",
			yaml: `name: ok
requests:
  - name: "Héllo"
    request: {method: GET, url: "https://example.com"}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "c.yaml")
			if err := os.WriteFile(p, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ParseFile(p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseFile error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrSlugEmpty) {
				t.Errorf("ParseFile error = %v, expected wraps ErrSlugEmpty", err)
			}
		})
	}
}

func TestParser_SlugPopulated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	body := `name: ok
setup:
  - name: Login
    request: {method: POST, url: "https://example.com/login"}
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/u"}
  - name: Create Post!
    request: {method: POST, url: "https://example.com/p"}
teardown:
  - name: Logout
    request: {method: POST, url: "https://example.com/logout"}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	col, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	want := map[string]string{
		"Login":        "login",
		"Get user":     "get-user",
		"Create Post!": "create-post",
		"Logout":       "logout",
	}
	for _, items := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, it := range items {
			if w, ok := want[it.Name]; ok {
				if it.Slug != w {
					t.Errorf("item %q: Slug = %q, want %q", it.Name, it.Slug, w)
				}
			}
		}
	}
}
```

#### Impact on Existing Tests

- `TestParser_DuplicateNameRejection` — runs before `populateSlugs`, so its
  test fixtures with valid-but-duplicate names continue to fail at the
  duplicate check, not the slug check. No fix needed.
- All other parser tests use names that slugify cleanly (`"Get user"`,
  `"Create user"`, `"login"`, etc.). No breakage.
- `TestParseFile_StructuredErrors` and similar — no overlap.

### Step 3: Bump SchemaVersion to 1.2 and add RequestSlug to event structs

**Rationale:** Pure data-shape change. Once this lands, every emitted event
will report `schema_version="1.2"` and the new `RequestStart`/`RequestEnd`
structs will accept a `RequestSlug` value. Schema docs update next; runner
plumbing comes after that. The order matters because the schema doc-sync test
will fail until the schema JSON also has `request_slug` listed.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/events.go` | modify | Bump `SchemaVersion` from `"1.1"` to `"1.2"`; add `RequestSlug string \`json:"request_slug,omitempty"\`` to `RequestStart` and `RequestEnd`; update package doc-comment. |
| `internal/output/events/emitter.go` | modify | Add `requestSlug string` parameter to `EmitRequestStart`; add `RequestSlug string` field to `RequestEndInput`. Wire both into the emitted event. |

#### Current Code (events.go:13)

```go
const SchemaVersion = "1.1"
```

#### New Code (events.go)

```go
// SchemaVersion is the current event-stream schema version. Promoted from
// "1.1" to "1.2" by M9-001 with an additive optional "request_slug" field
// on request.start and request.end. Removals and renames now require a v2.0
// bump.
const SchemaVersion = "1.2"
```

#### Current Code (events.go:80-109)

```go
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
	RequestID             string      `json:"request_id"`
	Outcome               Outcome     `json:"outcome"`
	// ...existing fields...
}
```

#### New Code (events.go)

```go
// RequestStart marks the beginning of a single request execution.
type RequestStart struct {
	Header
	RequestID    string `json:"request_id"`
	RequestSlug  string `json:"request_slug,omitempty"` // M9-001 v1.2 additive
	Name         string `json:"name,omitempty"`
	Method       string `json:"method"`
	URL          string `json:"url"`
	Phase        string `json:"phase,omitempty"`
	SourceFile   string `json:"source_file,omitempty"`
	SourceLine   int    `json:"source_line,omitempty"`
}

// RequestEnd marks the end of a single request execution.
type RequestEnd struct {
	Header
	RequestID             string      `json:"request_id"`
	RequestSlug           string      `json:"request_slug,omitempty"` // M9-001 v1.2 additive
	Outcome               Outcome     `json:"outcome"`
	// ...existing fields unchanged...
}
```

#### Current Code (emitter.go:174-194)

```go
func (e *Emitter) EmitRequestStart(requestID, name, method, url, phase, sourceFile string, sourceLine int) error {
	id := e.nextID()
	ev := RequestStart{
		Header: Header{ /* ... */ },
		RequestID:  requestID,
		Name:       name,
		// ...
	}
	return e.writeEvent(ev)
}
```

#### New Code (emitter.go)

```go
// EmitRequestStart emits a request.start event. requestSlug is the v1.2
// additive field, derived from name; pass "" to omit it (omitempty).
func (e *Emitter) EmitRequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine int) error {
	id := e.nextID()
	ev := RequestStart{
		Header: Header{
			SchemaVersion: SchemaVersion,
			RunID:         e.runID,
			ID:            id,
			AtMs:          e.atMs(),
			Kind:          KindRequestStart,
		},
		RequestID:   requestID,
		RequestSlug: requestSlug,
		Name:        name,
		Method:      method,
		URL:         url,
		Phase:       phase,
		SourceFile:  sourceFile,
		SourceLine:  sourceLine,
	}
	return e.writeEvent(ev)
}
```

```go
// RequestEndInput packages the inputs to EmitRequestEnd.
type RequestEndInput struct {
	RequestID    string
	RequestSlug  string // M9-001: v1.2 additive; empty → omitted
	Outcome      Outcome
	StatusCode   int
	Duration     time.Duration
	WaveIndex    int
	RequestBody  []byte
	ResponseBody []byte
	Err          error
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestEmitter_RequestStartCarriesSlug(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:          fixedClock(t, "2026-04-25T10:00:00Z"),
		RunID:          "rs-slug-001",
		ApitestVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user",
		"GET", "https://example.com", "main", "test.yaml", 5); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["schema_version"] != "1.2" {
		t.Errorf("schema_version = %v, want 1.2", got["schema_version"])
	}
	if got["request_slug"] != "get-user" {
		t.Errorf("request_slug = %v, want get-user", got["request_slug"])
	}
}

func TestEmitter_RequestStartOmitsEmptySlug(t *testing.T) {
	// requestSlug="" must NOT appear in the output (omitempty).
	var buf bytes.Buffer
	em, _ := events.NewEmitter(&buf, events.Options{ApitestVersion: "0.1.0-test"})
	_ = em.EmitRequestStart("req-1", "", "n", "GET", "u", "", "", 0)
	if strings.Contains(buf.String(), "request_slug") {
		t.Error("expected request_slug omitted when empty, got:", buf.String())
	}
}

func TestEmitter_RequestEndCarriesSlug(t *testing.T) {
	var buf bytes.Buffer
	em, _ := events.NewEmitter(&buf, events.Options{
		ApitestVersion: "0.1.0-test",
	})
	_ = em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "create-post",
		Outcome:     events.OutcomePassed,
		StatusCode:  201,
		Duration:    5 * time.Millisecond,
	})
	if !strings.Contains(buf.String(), `"request_slug":"create-post"`) {
		t.Error("expected request_slug field in output:", buf.String())
	}
}
```

#### Impact on Existing Tests

- **Every existing emitter test** that calls `EmitRequestStart(...)` now has
  the wrong arity. Fix every call by inserting `""` (or a slug) as the second
  argument. Files to edit:
  - `internal/output/events/emitter_test.go` — ~6 call sites at lines
    114, 559, 913.
  - `internal/output/events/schema_test.go` — 4 call sites at lines 104, 222,
    290, 697.
- **Every test that compares against existing golden files** will need the
  goldens regenerated (`UPDATE_GOLDEN=1 go test ...`):
  - `testdata/golden/run_happy.ndjson`
  - `testdata/golden/run_error.ndjson`
  - `testdata/golden/run_failed_assertion.ndjson`
  These golden files will now report `schema_version="1.2"` (v1.2 is
  additive — no other change).
- **`TestSchema_DocInSyncWithCode`** will fail until the v1.2 schema JSON has
  `request_slug` in `RequestStart` / `RequestEnd` properties. Step 4 fixes
  this.
- **`TestSchema_v11_validates`** currently exercises `EmitRequestStart` with
  the old arity and expects `schema_version="1.1"`. We refactor it (Step 5)
  to load v1.1.json explicitly and emit a v1.1-shaped object via a new
  `EmitRequestStartV11` test helper, OR simpler: rebuild the line by hand
  (since the test only needs to validate one v1.1-shaped record per kind).
  See Step 5 for details.

### Step 4: Publish docs/events-schema/v1.2.json and EVENTS_SCHEMA_v1.2.md

**Rationale:** The schema files are pure additions. Once committed, the
test harness can validate v1.2 emission against them. v1.0.json and v1.1.json
must be byte-unchanged.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `docs/events-schema/v1.2.json` | create | Copy of v1.1.json with: `$id` updated to v1.2; title/description bumped to v1.2; every `"const": "1.1"` replaced with `"const": "1.2"`; new optional `request_slug` property on `RequestStart` and `RequestEnd` with description noting "Added in v1.2." |
| `docs/EVENTS_SCHEMA_v1.2.md` | create | Cloned from v1.1 doc, replaces the v1.0→v1.1 changelog with a v1.1→v1.2 changelog, marks every example with `schema_version="1.2"`, updates the `request.start` / `request.end` field tables to include `request_slug` with the derivation rule, adds a stability-policy note that slug derivation is the canonical algorithm and changing it would be a v2.0 bump. |

#### v1.2.json delta (vs v1.1.json)

```diff
   "$id": "https://apitest.dev/events-schema/v1.2.json",
   "title": "ApiTool Agent Event Stream v1.2",
-  "description": "...v1.1 adds an optional 'selection' field to run.start carrying --only values.",
+  "description": "...v1.2 adds an optional 'request_slug' field to request.start and request.end derived from the request name.",
@@ definitions/Header
-          "const": "1.1"
+          "const": "1.2"
@@ every other "const": "1.1" -> "1.2"
@@ definitions/RequestStart/properties
+        "request_slug": {
+          "type": "string",
+          "minLength": 1,
+          "description": "URL-safe identifier derived from name. NFKD-normalized, lowercased, non-alphanumeric runs collapsed to '-'. Added in v1.2."
+        }
@@ definitions/RequestEnd/properties
+        "request_slug": {
+          "type": "string",
+          "minLength": 1,
+          "description": "URL-safe identifier derived from name; matches the paired request.start. Added in v1.2."
+        }
```

#### v1.2.md key sections

```markdown
# ApiTool Agent Event Stream — v1.2

## v1.1 → v1.2 changelog

v1.2 is a backward-compatible, additive release. No existing fields are renamed
or removed. A v1.1 consumer will continue to work against a v1.2 stream by
ignoring the new optional field.

**New optional field in v1.2:**

- `request.start.request_slug` (string, omitempty) — URL-safe identifier
  derived from the request `name`. Pairs with `request.end.request_slug`. Used
  by the W4 markdown formatter for filenames and sentinel tags.
- `request.end.request_slug` (string, omitempty) — same value as the paired
  `request.start.request_slug`.

**Slug derivation rule:**

1. Apply Unicode NFKD normalization to `name`.
2. Strip combining marks (Unicode category Mn).
3. Lowercase ASCII letters; preserve `[a-z0-9]`.
4. Collapse runs of non-alphanumeric runes to a single `-`.
5. Trim leading and trailing `-`.

If the result is empty (the name had no alphanumeric runes after
normalization), `apitest run` rejects the collection at load time with
`PARSE_SLUG_EMPTY`. Names that produce non-empty slugs are otherwise unrestricted.

`schema_version` changes from `"1.1"` to `"1.2"` in every emitted event.
```

#### Tests to Write FIRST (RED phase)

The schema-doc-sync test (`TestSchema_DocInSyncWithCode`) and the
markdown-fence validator (`TestSchema_MarkdownExamplesValidate`) will
automatically exercise these files once Step 5's helper changes route them to
v1.2. No new tests are required *for the files themselves*; they are exercised
by the broader suite below.

#### Impact on Existing Tests

- None directly until Step 5 wires the helpers to v1.2.

### Step 5: Wire schema_test.go to v1.2 + add v1.0/v1.1/v1.2 regression validators

**Rationale:** This is the test-harness refactor. Keep the helper layer narrow
so each version's regression test owns its own schema-loader call.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/events/schema_test.go` | modify | Generalize `schemaPath(t, version)` and `eventSchemaDocPath(t, version)`. Default `compileEventSchema(t)` loads v1.2.json. Add `compileEventSchemaVersion(t, version)`. Add `TestSchema_v10_validates` and refactor `TestSchema_v11_validates` to load explicit schemas and emit hand-built JSON lines that match those versions (so we never depend on the current emitter for historical validation). Add new `TestSchema_v12_validates` that exercises the current emitter with `request_slug` set. |
| `internal/output/events/testdata/golden/*.ndjson` | regenerate | Re-run with `UPDATE_GOLDEN=1`. New goldens carry `schema_version="1.2"` and `request_slug` for the request lines. |

#### Current Code (schema_test.go:22-34)

```go
func schemaPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(root, "docs", "events-schema", "v1.1.json")
}
```

#### New Code (schema_test.go)

```go
// schemaPath returns the absolute path to docs/events-schema/<version>.json
// by walking up from the test file location. version is e.g. "v1.2".
func schemaPath(t *testing.T, version string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(root, "docs", "events-schema", version+".json")
}

// compileEventSchema compiles the current (v1.2) schema. Backwards-compatible
// helper: most tests exercising the live emitter call this.
func compileEventSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileEventSchemaVersion(t, "v1.2")
}

func compileEventSchemaVersion(t *testing.T, version string) *jsonschema.Schema {
	t.Helper()
	path := schemaPath(t, version)
	// ...same body as today, calling AddResource/Compile on path...
}
```

#### TestSchema_v12_validates (new)

```go
// TestSchema_v12_validates compiles docs/events-schema/v1.2.json and
// validates one event of each kind, including request.start and request.end
// with request_slug set, against it.
func TestSchema_v12_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.2")
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:          fixedClock(t, "2026-04-25T10:00:00Z"),
		RunID:          "v12-validate-001",
		ApitestVersion: "0.1.0-test",
	})
	// emit one of each kind, with request_slug set on request.* events
	// validate every line against sch
}
```

#### TestSchema_v11_validates (refactored)

The current body emits live events via the current emitter. Since the emitter
now produces v1.2 records, we instead build v1.1-shaped JSON lines by hand
(or via a small inline helper that strips the v1.2-only fields and rewrites
schema_version to "1.1"). Either approach keeps the test asserting that
v1.1.json continues to validate v1.1-shaped events.

Simpler approach: hand-craft the JSON lines for each kind.

```go
func TestSchema_v11_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.1")
	lines := []string{
		`{"schema_version":"1.1","run_id":"r","id":1,"at_ms":0,"kind":"run.start","started_at":"2026-04-25T10:00:00Z","apitest_version":"0.1.0-test","cli_args":["run","t.yaml","--only","Get user"],"selection":["Get user"]}`,
		`{"schema_version":"1.1","run_id":"r","id":2,"at_ms":1,"kind":"request.start","request_id":"req-1","name":"Get user","method":"GET","url":"https://example.com"}`,
		`{"schema_version":"1.1","run_id":"r","id":3,"at_ms":2,"kind":"assertion.result","request_id":"req-1","type":"status","passed":true,"expected":"200","actual":"200"}`,
		`{"schema_version":"1.1","run_id":"r","id":4,"at_ms":3,"kind":"request.end","request_id":"req-1","outcome":"passed","duration_ms":42}`,
		`{"schema_version":"1.1","run_id":"r","id":5,"at_ms":4,"kind":"run.error","error":{"category":"input","code":"ONLY_NO_MATCH","message":"no match","hint":"Pass --only with a name matching a main request."}}`,
		`{"schema_version":"1.1","run_id":"r","id":6,"at_ms":5,"kind":"run.end","duration_ms":5,"total":1,"passed":1,"failed":0,"skipped":0,"exit_code":0,"event_count":6}`,
	}
	for _, line := range lines {
		validateLine(t, sch, line)
	}
}
```

#### TestSchema_v10_validates (new)

```go
func TestSchema_v10_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.0")
	// hand-built v1.0 lines (no selection on run.start, no request_slug)
	// ...validate every kind...
}
```

#### TestSchema_DocInSyncWithCode and TestSchema_MarkdownExamplesValidate

Both helpers currently call `eventSchemaDocPath` for v1.1.md and `schemaPath`
for v1.1.json. After this refactor they call `eventSchemaDocPath(t, "v1.2")`
and `compileEventSchemaVersion(t, "v1.2")` (or use the default). Their
test logic is otherwise unchanged.

#### Impact on Existing Tests

- `TestEmitter_AllKindsValidateAgainstSchema` and `TestEmitter_GoldenSchemaValidates`
  call `compileEventSchema(t)` — that helper now points at v1.2.json. The live
  emitter now produces v1.2 records, so validation still passes.
- `TestEmitter_GoldenRunHappy` / `GoldenRunError` / `GoldenRunFailedAssertion`
  compare to fixture files. After regenerating with `UPDATE_GOLDEN=1`, the
  fixtures shift to `schema_version="1.2"` and the request lines gain
  `request_slug` (test passes a slug to `EmitRequestStart` / sets
  `RequestEndInput.RequestSlug`).
- `TestSchema_v01ArtifactsRetained` and `TestSchema_v10ArtifactsRetained`
  (file-existence checks) are unaffected.

### Step 6: Runner wiring — thread RequestSlug through every emit site

**Rationale:** Once the emitter API and schema are in place, this is the
final integration: every place that builds a `runner.RequestEvent` /
`runner.RequestEndEvent` must populate `RequestSlug`. We also widen
`runner.RequestEvent`/`RequestEndEvent` and the parallel `EventSink` interface
in lock-step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `RequestSlug string` to `RequestEvent` and `RequestEndEvent`. Update `parallelSinkAdapter` methods to forward the new parameters. Update every internal emit site to pass the slug: sequential main loop (line ~1709), data-driven sequential (~2069), data-driven parallel (~2273), WebSocket (~1596), and the helper `emitRequestEnd` (~2684). Sequential/parallel/websocket use `item.Slug`; data-driven iterations call `Slug(iterName)` and bubble the error as a parser-style structured error. |
| `internal/parallel/executor.go` | modify | Widen `EventSink.RequestStart`/`RequestEnd` signatures with `requestSlug string`. Widen `Config.NextRequestID` semantics OR add a parallel `NextRequestSlug func() string`. Simpler: pass `slug` into `cfg.EventSink.RequestStart` from the runner-side `executePhase` adapter call site, by including `Slug` in the `parser.RequestItem` passed in `Items`. Since RequestItem already carries Slug after Step 2, the parallel executor can read `item.Slug` directly at line 365. |
| `internal/parallel/executor_test.go` | modify | `parallelRecordingSink.RequestStart`/`RequestEnd` gain the slug parameter; existing tests pass `""` or assert against the cached `item.Slug`. |
| `internal/runner/runner_test.go` | modify | `recordingSink` already captures `RequestEvent` / `RequestEndEvent` by value — once those structs gain `RequestSlug` the captures pick it up automatically. Add `TestRunner_RequestSlugAllEmitSites` covering all four paths. Existing tests that build `RequestEvent` for non-event reasons may need updates (none expected). |
| `cmd/apitest/main.go` | modify | `eventsAdapter.RequestStart`/`RequestEnd` forward `e.RequestSlug` into the emitter. |

#### Current Code (runner.go:140-161)

```go
type RequestEvent struct {
	RequestID  string
	Name       string
	// ...
}
type RequestEndEvent struct {
	RequestID    string
	Outcome      string
	// ...
}
```

#### New Code (runner.go)

```go
type RequestEvent struct {
	RequestID   string
	RequestSlug string // M9-001: derived from Name; pairs with RequestEndEvent
	Name        string
	Method      string
	URL         string
	Phase       string
	SourceFile  string
	SourceLine  int
}
type RequestEndEvent struct {
	RequestID    string
	RequestSlug  string // M9-001: same value as the paired RequestEvent
	Outcome      string
	StatusCode   int
	Duration     time.Duration
	WaveIndex    int
	RequestBody  []byte
	ResponseBody []byte
	Err          error
}
```

#### Current Code (runner.go:1705-1718, sequential main loop)

```go
		var reqID string
		if vars.OnEvent != nil {
			reqID = vars.nextRequestID()
			vars.OnEvent.RequestStart(RequestEvent{
				RequestID:  reqID,
				Name:       item.Name,
				// ...
			})
		}
```

#### New Code (runner.go)

```go
		var reqID string
		if vars.OnEvent != nil {
			reqID = vars.nextRequestID()
			vars.OnEvent.RequestStart(RequestEvent{
				RequestID:   reqID,
				RequestSlug: item.Slug,
				Name:        item.Name,
				// ...other fields unchanged...
			})
		}
```

The same edit applies to:
- WebSocket emit at line ~1596 (`Slug: item.Slug`)
- Data-driven sequential at line ~2069: needs `iterSlug, _ := parser.Slug(iterName)`; on error fall back to `item.Slug` (we already validated the base name; iter suffixes always slugify cleanly)
- Data-driven parallel at line ~2273: same pattern as sequential
- `emitRequestEnd` helper at line ~2684: takes a `slug string` parameter (or reads from a passed-in struct)

#### Current Code (runner.go:2655-2693, emitRequestEnd helper)

```go
func emitRequestEnd(sink EventSink, reqID string, rr RequestResult, waveIndex int) {
	// ...build RequestEndEvent without slug...
	sink.RequestEnd(RequestEndEvent{
		RequestID: reqID,
		Outcome:   outcomeString(rr),
		// ...
	})
}
```

#### New Code (runner.go)

```go
// emitRequestEnd takes the slug as an explicit parameter because RequestResult
// does not carry the per-emit slug (data-driven iterations have a different
// slug than the base item).
func emitRequestEnd(sink EventSink, reqID, reqSlug string, rr RequestResult, waveIndex int) {
	// ...
	sink.RequestEnd(RequestEndEvent{
		RequestID:   reqID,
		RequestSlug: reqSlug,
		Outcome:     outcomeString(rr),
		// ...
	})
}
```

Every call site of `emitRequestEnd` updates with the appropriate slug.

#### Current Code (parallel/executor.go:84-90)

```go
type EventSink interface {
	RequestStart(requestID, name, method, url, phase, sourceFile string, sourceLine, waveIndex int)
	RequestEnd(requestID, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error)
	AssertionResult(requestID, aType, expected, actual string, passed bool)
}
```

#### New Code (parallel/executor.go)

```go
type EventSink interface {
	// requestSlug is the v1.2 additive correlation field; pass "" to omit.
	RequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine, waveIndex int)
	RequestEnd(requestID, requestSlug, outcome string, statusCode int, duration time.Duration, waveIndex int, reqBody, respBody []byte, err error)
	AssertionResult(requestID, aType, expected, actual string, passed bool)
}
```

The single call site at executor.go:365 reads the slug straight from `item`:

```go
// Before
cfg.EventSink.RequestStart(reqID, item.Name, req.Method, req.URL, "main",
    item.SourceFile, item.SourceLine, waveIdx)
// After
cfg.EventSink.RequestStart(reqID, item.Slug, item.Name, req.Method, req.URL, "main",
    item.SourceFile, item.SourceLine, waveIdx)
```

`runner.parallelSinkAdapter.RequestStart`/`RequestEnd` widen accordingly:

```go
func (a *parallelSinkAdapter) RequestStart(requestID, requestSlug, name, method, url, phase, sourceFile string, sourceLine, waveIndex int) {
	a.inner.RequestStart(RequestEvent{
		RequestID:   requestID,
		RequestSlug: requestSlug,
		Name:        name,
		// ...
	})
}
```

#### Current Code (cmd/apitest/main.go:417-454)

```go
func (a *eventsAdapter) RequestStart(e runner.RequestEvent) {
	if err := a.em.EmitRequestStart(e.RequestID, e.Name, e.Method, e.URL, e.Phase, e.SourceFile, e.SourceLine); err != nil {
		// ...
	}
}
// RequestEnd similar
```

#### New Code (cmd/apitest/main.go)

```go
func (a *eventsAdapter) RequestStart(e runner.RequestEvent) {
	if err := a.em.EmitRequestStart(e.RequestID, e.RequestSlug, e.Name, e.Method, e.URL, e.Phase, e.SourceFile, e.SourceLine); err != nil {
		_, _ = fmt.Fprintf(a.errOut, "events: emit request.start: %v\n", err)
	}
}

func (a *eventsAdapter) RequestEnd(e runner.RequestEndEvent) {
	// ...existing redaction code unchanged...
	in := events.RequestEndInput{
		RequestID:    e.RequestID,
		RequestSlug:  e.RequestSlug,
		Outcome:      events.Outcome(e.Outcome),
		// ...rest unchanged...
	}
	// ...
}
```

#### Tests to Write FIRST (RED phase)

```go
// In runner_test.go.
//
// TestRunner_RequestSlugAllEmitSites covers every code path that emits
// request.start / request.end events: sequential main, parallel waves,
// data-driven sequential, data-driven parallel, and websocket. Each path
// must populate RequestSlug from the parser-cached item.Slug (or from the
// per-iteration name for data-driven).
func TestRunner_RequestSlugAllEmitSites(t *testing.T) {
	t.Run("sequential main", func(t *testing.T) {
		// 2 main items with distinct names → 2 distinct slugs on starts and ends
	})
	t.Run("parallel main waves", func(t *testing.T) {
		// 3 items in one wave executing in parallel; check slug populated
	})
	t.Run("data-driven sequential", func(t *testing.T) {
		// 3-row CSV; verify each iteration's slug includes the iteration suffix
	})
	t.Run("data-driven parallel", func(t *testing.T) {
		// 3-row CSV with parallel: true; same expectation
	})
	t.Run("websocket main", func(t *testing.T) {
		// one WebSocket request; slug derived from item.Name
	})
	t.Run("setup and teardown phases", func(t *testing.T) {
		// setup item + teardown item; slug populated on both
	})
}
```

The integration "concrete consumer" test reads NDJSON output for a real
fixture collection and asserts `request_slug` matches the expected derivation
for each request name. This satisfies behavior #8.

#### Impact on Existing Tests

- Every `RequestEvent` / `RequestEndEvent` literal in `runner_test.go` needs
  the new field zero-valued or set; Go zero-fills missing fields, so most
  existing tests need no changes. Tests that *assert* on the literal struct
  (e.g. `if rr != expectedRR` patterns) will need the expected struct to
  include `RequestSlug: ""`.
- `parallel/executor_test.go` test sinks need their method signatures widened.
  Test bodies that don't compare slugs add `_ = requestSlug` (or just omit
  the parameter name).
- `cmd/apitest/main.go` end-to-end smoke pathways: the smoke-test fixture
  collection's request names slugify cleanly (`run.ndjson` → request_slug
  appears). `./smoke/run.sh` exercises this implicitly.

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|---------------|--------|----------------|
| `internal/parser/slug_test.go` | `TestSlug_Derive` | new | write before helper |
| `internal/parser/parser_test.go` | `TestParser_SlugEmptyRejection` | new | write before populateSlugs |
| `internal/parser/parser_test.go` | `TestParser_SlugPopulated` | new | new |
| `internal/parser/parser_test.go` | `TestParser_DuplicateNameRejection` | unchanged | none (passes through) |
| `internal/output/events/emitter_test.go` | every `EmitRequestStart` caller | breaks | add `""` slug arg |
| `internal/output/events/emitter_test.go` | golden tests | breaks | regenerate goldens |
| `internal/output/events/emitter_test.go` | `TestEmitter_RequestStartCarriesSlug` | new | new |
| `internal/output/events/emitter_test.go` | `TestEmitter_RequestStartOmitsEmptySlug` | new | new |
| `internal/output/events/emitter_test.go` | `TestEmitter_RequestEndCarriesSlug` | new | new |
| `internal/output/events/schema_test.go` | `compileEventSchema` callers | breaks | helper signature change |
| `internal/output/events/schema_test.go` | `TestSchema_v10_validates` | new | new (hand-built v1.0 lines) |
| `internal/output/events/schema_test.go` | `TestSchema_v11_validates` | refactor | hand-built v1.1 lines |
| `internal/output/events/schema_test.go` | `TestSchema_v12_validates` | new | new |
| `internal/output/events/schema_test.go` | `TestSchema_v01ArtifactsRetained` | unchanged | none |
| `internal/output/events/schema_test.go` | `TestSchema_v10ArtifactsRetained` | unchanged | none |
| `internal/output/events/schema_test.go` | `TestSchema_DocInSyncWithCode` | passes | new fields added to schema |
| `internal/output/events/schema_test.go` | `TestSchema_MarkdownExamplesValidate` | passes | new doc validates |
| `internal/output/events/testdata/golden/*.ndjson` | n/a | regenerate | `UPDATE_GOLDEN=1 go test ./internal/output/events/` |
| `internal/runner/runner_test.go` | `TestRunner_RequestSlugAllEmitSites` | new | new |
| `internal/runner/runner_test.go` | every `RequestEvent` struct | additive field | zero-fills automatically |
| `internal/parallel/executor_test.go` | `parallelRecordingSink` | breaks | widen method signatures |
| `cmd/apitest/main.go` | smoke flow | additive | no test code change |

## Risks and Edge Cases

- **Risk: data-driven iteration slug uniqueness.** Two iterations with names
  `"X [1/3]"` and `"X [2/3]"` produce slugs `x-1-3` and `x-2-3` — distinct.
  But `"X [11/12]"` slugifies to `x-11-12` and `"X [1/2]"` slugifies to
  `x-1-2`; both unique. **Mitigation:** the slug helper's table-driven test
  includes `"Create user [1/3]"` → `"create-user-1-3"` to lock the format.

- **Risk: data-driven base item with valid slug but iteration name that
  slugifies to empty.** Impossible in practice — iteration names always
  contain `[N/M]` suffix with digits, which guarantees a non-empty slug.
  **Mitigation:** the runner falls back to `item.Slug` if `Slug(iterName)`
  somehow returns an error (defensive code; should never happen).

- **Risk: include: directive splices items with names that slugify to empty.**
  `populateSlugs` runs after `resolveIncludes` and after `checkDuplicateNames`,
  so all spliced items are validated. **Mitigation:** test case in
  `TestParser_SlugEmptyRejection` covers an include-spliced empty-slug name.

- **Risk: golden NDJSON re-generation introduces unintended diffs.** v1.2 is
  intentionally additive — only `schema_version` changes from `1.1` to `1.2`
  and `request_slug` is added on request lines. **Mitigation:** review the
  diff manually; assert the diff is exactly those two changes.

- **Risk: backwards compatibility of EmitRequestStart signature change.**
  External callers don't exist (it's `internal/...`), but the cmd layer in
  this repo is one caller. **Mitigation:** the signature change is intentional
  and updated in lock-step in cmd/apitest/main.go.

- **Edge case: `omitempty` on `RequestSlug` field.** When a caller passes "" 
  the field is omitted from emitted JSON, which is what we want for forward-
  compat with consumers that don't expect the field. **Handling:** `,omitempty`
  on the JSON tag.

- **Edge case: `additionalProperties: false` on schema definitions.** The v1.1
  schema sets `additionalProperties: false` on every request kind. Adding
  `request_slug` requires listing it in `properties`. **Handling:** v1.2.json
  adds the property to both `RequestStart` and `RequestEnd`.

- **Edge case: schema-doc-sync test (`TestSchema_DocInSyncWithCode`).** This
  test reflects on Go struct fields and compares to schema `properties`. It
  will fail until v1.2.json lists `request_slug`. **Handling:** Step 4
  publishes v1.2.json with the field listed.

- **Edge case: `IMPROVEMENT.md` mentions v1.1 → v1.2 mismatch.** The
  IMPROVEMENT.md document section §6 says "Added to the events schema as an
  additive field in v1.1." That is now factually incorrect (v1.2 is the
  shipped version). **Handling:** scope explicitly defers IMPROVEMENT.md
  edits to M9-005, so we do not touch them here. Acceptable as a known
  doc-stale state for the duration of W4.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):

```bash
# Slug derivation
go test -run 'TestSlug_Derive' ./internal/parser/...

# Schema regression (v1.0, v1.1, v1.2)
go test -run 'TestSchema_v10|TestSchema_v11|TestSchema_v12_validates' ./internal/output/events/...

# Runner all-emit-sites integration
go test -run 'TestRunner_RequestSlugAllEmitSites' ./internal/runner/...

# Files exist / unchanged
ls docs/events-schema/v1.2.json docs/events-schema/v1.1.json docs/events-schema/v1.0.json

# End-to-end NDJSON inspection (smoke)
./apitest run smoke/collections/sample.yaml --events run.ndjson
jq -c 'select(.kind=="request.start") | {name, request_id, request_slug}' run.ndjson

# Aggregate
go test -run 'TestSlug_Derive|TestEvents_v12_RequestSlug|TestSchema_v12_validates|TestRunner_RequestSlugAllEmitSites' ./...
```
