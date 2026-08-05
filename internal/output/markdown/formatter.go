// Package markdown renders API test results as markdown files under a
// --report directory. Each main-phase request produces a <slug>.md file;
// a run.md index links all per-request files.
//
// File writes are atomic (O_EXCL temp-file + rename). Existing files are
// spliced when the BEGIN/END sentinels match the current request slug;
// agent-authored content above and below the sentinels is preserved verbatim.
//
// M9-002 ships JSON body rendering only; the full content-type matrix lands
// in M9-003.
package markdown

import (
	"bytes"
	"cmp"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"slices"
	"time"

	"github.com/weiqigod/curlew/internal/assertion"
	"gopkg.in/yaml.v3"
)

// SentinelBeginPrefix is the literal prefix of the BEGIN sentinel line.
// The full line carries id, slug, run attributes after this prefix.
const SentinelBeginPrefix = "<!-- BEGIN curlew:response "

// SentinelEndPrefix is the literal prefix of the END sentinel line.
const SentinelEndPrefix = "<!-- END curlew:response "

// SentinelSuffix terminates both BEGIN and END sentinel lines.
const SentinelSuffix = " -->"

// RunMDSentinelSlug is the reserved slug for the run.md index file.
const RunMDSentinelSlug = "run"

// Report is the complete dataset for one markdown render. The cmd/curlew
// builder converts []runner.RequestResult into a *Report; the markdown
// package owns no runner types directly so the dependency graph stays
// internal/output/* -> internal/runner (one-way only at the cmd layer).
type Report struct {
	CollectionName string
	EnvName        string    // optional; empty when --env not passed
	RunID          string    // 32-char hex
	StartedAt      time.Time // RFC3339Nano in output
	Summary        SummaryCounts
	Requests       []RequestEntry // main-phase only; setup/teardown excluded for M9-002
	IsParallel     bool           // M9-004: enables ## Wave N grouping in run.md
}

// SummaryCounts holds aggregate pass/fail/skip counts for the run summary.
type SummaryCounts struct {
	Total   int
	Passed  int
	Failed  int
	Skipped int
}

// RequestEntry holds the data for one per-request markdown file.
type RequestEntry struct {
	RequestID   string // matches events stream
	Slug        string // matches filename: <slug>.md
	Name        string // human-readable name
	Method      string
	URL         string
	StatusCode  int // 0 if no response
	DurationMs  int64
	WaveIndex   int // -1 when sequential
	StartedAt   time.Time
	RequestHdr  map[string]string
	RequestBody any
	RespHeaders http.Header // http.Header form
	RespBody    []byte
	Assertions  *assertion.Results // nil when not evaluated
	Err         error
	Skipped     bool
	SkipReason  string

	// M9-004: data-driven fields. Populated only when this entry represents a
	// data-driven request (consecutive runner.RequestResult rows sharing
	// DataDrivenName=X). When len(Iterations) == 0 this is a regular
	// single-request entry (M9-002/M9-003 path).
	Iterations     []IterationEntry // per-iteration data; non-nil only for data-driven entries
	IterationTotal int              // source row count; may exceed len(Iterations) under runner cap
	DataDrivenName string           // human-readable name (without [X/Y] suffix); used for index.md heading
}

// IterationEntry holds the data for one iteration of a data-driven request.
// Mirrors RequestEntry's response/timing/assertion fields but omits the
// fields the iteration cannot vary (Slug, Name) — those live on the
// parent RequestEntry.
type IterationEntry struct {
	RequestID   string // runner-minted id (for sentinel); becomes "<id>-iter-<index>"
	Index       int    // 0-based iteration index
	StatusCode  int
	DurationMs  int64
	StartedAt   time.Time
	Method      string
	URL         string
	RequestHdr  map[string]string
	RequestBody any
	RespHeaders http.Header
	RespBody    []byte
	Assertions  *assertion.Results
	Err         error
	Skipped     bool
	SkipReason  string
	Data        map[string]string // IterationData (informational; not rendered for M9-004)
}

// WriteOptions configures the behaviour of WriteReport.
type WriteOptions struct {
	// Stderr receives orphan-slug warnings and dot-new warnings. Defaults
	// to io.Discard when nil.
	Stderr io.Writer
}

// WriteReport renders a Report into <dir>/run.md and one <dir>/<slug>.md per
// main-phase request. Splices into existing files when the BEGIN/END
// sentinels match the current slug. Writes <slug>.md.new (and emits a
// stderr warning via opts.Stderr) when the existing file lacks the expected
// sentinel pair.
//
// The dir must exist; callers use EnsureReportDir to create it.
//
// Reserved slug: RunMDSentinelSlug ("run") is used for the run.md index file.
// A request whose parser-derived slug is "run" (e.g. a request named "Run")
// will produce a filename collision with run.md. The cmd-layer builder
// (buildMarkdownReport) is responsible for skipping or warning on such
// requests. This constraint is documented here so future authors are aware.
func WriteReport(report *Report, dir string, opts WriteOptions) error {
	errW := opts.Stderr
	if errW == nil {
		errW = io.Discard
	}

	// Render and write each per-request file.
	for i := range report.Requests {
		entry := &report.Requests[i]
		if len(entry.Iterations) > 0 {
			// Data-driven request: subdirectory + per-iteration files + index.md.
			if err := renderDataDrivenRequest(entry, dir, report.RunID, errW); err != nil {
				return fmt.Errorf("write data-driven %s: %w", entry.Slug, err)
			}
			continue
		}
		content := renderFullFile(entry, report.RunID)
		path := filePath(dir, entry.Slug+".md")
		if err := writeFile(path, entry.Slug, content, errW); err != nil {
			return fmt.Errorf("write %s.md: %w", entry.Slug, err)
		}
	}

	// Render and write run.md.
	runContent := renderRunMD(report)
	runPath := filePath(dir, "run.md")
	if err := writeFile(runPath, RunMDSentinelSlug, runContent, errW); err != nil {
		return fmt.Errorf("write run.md: %w", err)
	}

	return nil
}

// renderFullFile produces the complete byte content for a per-request .md file
// when writing fresh (no existing file). When splicing, only the sentinel
// block (between and including the BEGIN/END lines) is replaced.
//
// Section order (11 markers per task observable, M9-003):
//  1. # <name>
//  2. ## Notes
//  3. (empty notes paragraph — placeholder for agent text)
//  4. <!-- BEGIN curlew:response id=... slug=... run=... -->
//  5. ## Response (deterministic)
//  6. ### Request
//  7. ### Response <status>
//  8. ### Response metadata    (NEW: M9-003)
//  9. ### Timing
//  10. ### Assertions
//  11. <!-- END curlew:response id=... slug=... run=... -->
//
// Plus ## Analysis below the END sentinel (agent-owned).
func renderFullFile(entry *RequestEntry, runID string) []byte {
	var buf bytes.Buffer

	// Section 1: H1 name
	fmt.Fprintf(&buf, "# %s\n\n", entry.Name)

	// Section 2+3: ## Notes placeholder
	fmt.Fprintf(&buf, "## Notes\n\n")

	// Sections 4–11: sentinel block
	buf.Write(renderSentinelBlock(entry, runID))

	// ## Analysis (below END sentinel; agent-owned space)
	fmt.Fprintf(&buf, "\n## Analysis\n\n")

	return buf.Bytes()
}

// renderSentinelBlock emits the bytes between (and including) the BEGIN/END
// sentinel lines. This is the only region the splice path overwrites.
func renderSentinelBlock(entry *RequestEntry, runID string) []byte {
	var buf bytes.Buffer

	// Section 4: BEGIN sentinel
	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelBeginPrefix, entry.RequestID, entry.Slug, runID, SentinelSuffix)

	// Section 5: ## Response (deterministic)
	fmt.Fprintf(&buf, "## Response (deterministic)\n")

	// Section 6: ### Request
	fmt.Fprintf(&buf, "\n### Request\n\n")
	renderRequest(&buf, entry)

	// Section 7: ### Response <status>
	fmt.Fprintf(&buf, "\n### Response %d\n\n", entry.StatusCode)
	renderResponse(&buf, entry)

	// Section 8: ### Response metadata (NEW: M9-003)
	fmt.Fprintf(&buf, "\n### Response metadata\n\n")
	renderResponseMetadata(&buf, entry)

	// Section 9: ### Timing
	fmt.Fprintf(&buf, "\n### Timing\n\n")
	renderTiming(&buf, entry)

	// Section 10: ### Assertions
	fmt.Fprintf(&buf, "\n### Assertions\n\n")
	renderAssertions(&buf, entry)

	// Section 11: END sentinel
	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelEndPrefix, entry.RequestID, entry.Slug, runID, SentinelSuffix)

	return buf.Bytes()
}

// renderRequest writes the ### Request section body. Volatile headers are
// filtered out so request blocks remain diff-friendly.
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

// renderResponse writes the ### Response section body. Volatile headers are
// NOT included here; they appear only in renderResponseMetadata.
// Order: cap -> classify -> render.
func renderResponse(w *bytes.Buffer, entry *RequestEntry) {
	ct := lookupHeader(entry.RespHeaders, "Content-Type")
	capped, truncated, original := truncateForMarkdown(entry.RespBody, BodyCapBytes)
	kind := Classify(capped, entry.Method, ct)
	renderBody(w, kind, capped, original, truncated)
}

// renderResponseMetadata emits the ### Response metadata section body.
// All headers (including volatile) are rendered here in alphabetical order.
func renderResponseMetadata(w *bytes.Buffer, entry *RequestEntry) {
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

// renderYAMLBody canonicalises body via gopkg.in/yaml.v3 using yaml.Node
// to preserve source ordering. On round-trip failure falls back to verbatim
// ```yaml rendering.
func renderYAMLBody(w *bytes.Buffer, body []byte) {
	var n yaml.Node
	if err := yaml.Unmarshal(body, &n); err == nil {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(&n); err == nil {
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

// renderTiming writes the ### Timing section body with three fixed-prefix lines
// suitable for diff masking.
func renderTiming(w *bytes.Buffer, entry *RequestEntry) {
	fmt.Fprintf(w, "duration_ms: %d\n", entry.DurationMs)
	if entry.WaveIndex < 0 {
		fmt.Fprintln(w, "wave_index: sequential")
	} else {
		fmt.Fprintf(w, "wave_index: %d\n", entry.WaveIndex)
	}
	fmt.Fprintf(w, "started_at: %s\n", entry.StartedAt.UTC().Format(time.RFC3339Nano))
}

// renderAssertions writes the ### Assertions section body.
// When no assertions are declared, writes `_No assertions declared._`.
func renderAssertions(w *bytes.Buffer, entry *RequestEntry) {
	if entry.Assertions == nil || len(entry.Assertions.Items) == 0 {
		fmt.Fprintln(w, "_No assertions declared._")
		return
	}
	for _, it := range entry.Assertions.Items {
		marker := "[x]"
		if !it.Passed {
			marker = "[ ]"
		}
		fmt.Fprintf(w, "- %s %s expected=%q actual=%q\n", marker, it.Label(), it.Expected, it.Actual)
	}
}

// lookupHeader returns the first value for the named header (case-insensitive).
func lookupHeader(h http.Header, name string) string {
	if h == nil {
		return ""
	}
	vals := h[http.CanonicalHeaderKey(name)]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// filePath joins dir and name into a path string using the OS path separator.
func filePath(dir, name string) string {
	if dir == "" {
		return name
	}
	return filepath.Join(dir, name)
}
