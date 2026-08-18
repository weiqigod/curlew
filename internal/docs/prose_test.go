package docs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

func TestExtractProse(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // claim texts, in order
	}{
		{
			name: "modal claim with flag referent",
			src:  "Colour is never emitted on stdout when `--format` is non-terminal.",
			want: []string{"Colour is never emitted on stdout when `--format` is non-terminal."},
		},
		{
			name: "same-as claim across formats",
			src:  "`run_id` is the same hex value across `--events` and `--log`.",
			want: []string{"`run_id` is the same hex value across `--events` and `--log`."},
		},
		{
			name: "hard-wrapped paragraph is reflowed",
			src:  "`request_id` and `request_slug` link\na specific event line to a specific `run.md`.",
			want: []string{"`request_id` and `request_slug` link a specific event line to a specific `run.md`."},
		},
		{
			name: "semicolon splits two claims",
			src:  "`run_id` is the same across formats; `request_id` and `request_slug` link a line to `run.md`.",
			want: []string{
				"`run_id` is the same across formats;",
				"`request_id` and `request_slug` link a line to `run.md`.",
			},
		},
		{
			name: "sentence boundary across bold closer",
			src:  "**Every `{{$uuid}}` resolves to the same UUID.** If you want distinct UUIDs, use extract.",
			want: []string{"**Every `{{$uuid}}` resolves to the same UUID.**"},
		},
		{
			name: "fenced code is not prose",
			src:  "```\nalways --format json\n```",
			want: nil,
		},
		{
			name: "table row is not prose",
			src:  "| Flag | Meaning |\n|---|---|\n| `--x` | always on |",
			want: nil,
		},
		{
			name: "heading is not a claim",
			src:  "## Every flag is documented",
			want: nil,
		},
		{
			name: "abbreviation does not split",
			src:  "It is redacted (e.g. `--token`) in every format.",
			want: []string{"It is redacted (e.g. `--token`) in every format."},
		},
		{
			name: "shape without referent declines",
			src:  "It always works well.",
			want: nil,
		},
		{
			name: "referent without shape declines",
			src:  "The `--format` flag takes a value.",
			want: nil,
		},
		{
			name: "imperative advice declines",
			src:  "Always source HMAC keys from a sensitive-named variable like `--secret`.",
			want: nil,
		},
		{
			name: "second-person advice declines",
			src:  "If you disable colour, `--no-color` always suppresses ANSI codes on stderr.",
			want: nil,
		},
		{
			name: "prohibition with 'you' is a claim",
			src:  "You cannot set both `path:` and `request:` on the same item.",
			want: []string{"You cannot set both `path:` and `request:` on the same item."},
		},
		{
			name: "cross-reference declines",
			src:  "For the full schema — every field, every enum, every ordering guarantee — see [`docs/EVENTS_SCHEMA_v1.6.md`](x.md).",
			want: nil,
		},
		{
			name: "ignore-names region is skipped",
			src:  "<!-- doc-check: ignore-names -->\nEvery `CURLEW_BACKEND_URL` variable.",
			want: nil,
		},
		{
			name: "marker exempts every claim in next block",
			src:  "<!-- doc-check: prose-not-executable why -->\nA is always `--x`. B is never `--y`.",
			want: []string{"A is always `--x`.", "B is never `--y`."},
		},
		{
			name: "marker is spent by the following block",
			src:  "<!-- doc-check: prose-not-executable why -->\nA is always `--x`.\n\nB is never `--y`.",
			want: []string{"A is always `--x`.", "B is never `--y`."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := docs.ExtractProse("TEST.md", tt.src)
			var got []string
			for _, r := range refs {
				got = append(got, r.Text)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractProse() texts = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestExtractProse_markerScope checks that only the first block after a
// prose-not-executable marker carries the Exempt reason, and that it is
// spent by the block that follows -- the property that lets a marker cover a
// whole paragraph without leaking onto the next one.
func TestExtractProse_markerScope(t *testing.T) {
	src := "<!-- doc-check: prose-not-executable narrative example -->\n" +
		"A is always `--x`.\n\n" +
		"B is never `--y`."
	refs := docs.ExtractProse("TEST.md", src)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2: %#v", len(refs), refs)
	}
	if refs[0].Exempt != "narrative example" {
		t.Errorf("first claim Exempt = %q, want %q", refs[0].Exempt, "narrative example")
	}
	if refs[1].Exempt != "" {
		t.Errorf("second claim Exempt = %q, want empty (marker spent by first block)", refs[1].Exempt)
	}
}

func TestExtractProse_lineAndHeading(t *testing.T) {
	src := "# Part 1\n\n## 1.1 Correlation\n\n" +
		"`run_id` is the same hex value across `--events` and `--log`.\n"
	refs := docs.ExtractProse("TEST.md", src)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %#v", len(refs), refs)
	}
	if refs[0].Heading != "1.1 Correlation" {
		t.Errorf("Heading = %q, want %q", refs[0].Heading, "1.1 Correlation")
	}
	if refs[0].Line != 5 {
		t.Errorf("Line = %d, want 5", refs[0].Line)
	}
	if refs[0].Shape != "same-as" {
		t.Errorf("Shape = %q, want %q", refs[0].Shape, "same-as")
	}
}

func TestExtractProse_listItemIsOwnBlock(t *testing.T) {
	src := "- `run_id` is always the same across `--events`.\n" +
		"- `request_id` is never reused across `--log` entries.\n"
	refs := docs.ExtractProse("TEST.md", src)
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2 (one per list item): %#v", len(refs), refs)
	}
}

func TestProseRef_Key(t *testing.T) {
	r := docs.ProseRef{Doc: "MANUAL.md", Heading: "1.1 Correlation", Text: "short claim"}
	want := "MANUAL.md\t1.1 Correlation\tshort claim"
	if got := r.Key(); got != want {
		t.Errorf("Key() = %q, want %q", got, want)
	}
}

func TestProseRef_KeyTruncatesLongText(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	r := docs.ProseRef{Doc: "MANUAL.md", Heading: "H", Text: long}
	key := r.Key()
	// doc + "\t" + heading + "\t" + text(120)
	wantLen := len("MANUAL.md") + 1 + len("H") + 1 + 120
	if len(key) != wantLen {
		t.Errorf("Key() length = %d, want %d (text must be truncated to 120)", len(key), wantLen)
	}
}

// TestProseClaims mirrors internal/docs/claims_test.go's shape for
// docs.Claims: a claim is derived from a call expression carrying a ".md"
// string literal and a substring, read out of test sources rather than a
// hand-kept list.
func TestProseClaims(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []docs.ProseClaim
	}{
		{
			name: "direct call",
			src: `package p
import "github.com/weiqigod/curlew/internal/docs"
func f() { docs.Prose("MANUAL.md", "link a specific event line") }`,
			want: []docs.ProseClaim{{Doc: "MANUAL.md", Substr: "link a specific event line"}},
		},
		{
			name: "unqualified call",
			src: `package p
func f() { Prose("MANUAL.md", "always") }`,
			want: []docs.ProseClaim{{Doc: "MANUAL.md", Substr: "always"}},
		},
		{
			name: "non-md first arg is not a claim",
			src: `package p
func f() { Prose("x.txt", "y") }`,
			want: nil,
		},
		{
			name: "no string args",
			src: `package p
func f(doc, substr string) { Prose(doc, substr) }`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "x_test.go")
			if err := os.WriteFile(path, []byte(tt.src), 0o644); err != nil {
				t.Fatal(err)
			}
			claims, err := docs.ProseClaims(dir)
			if err != nil {
				t.Fatalf("ProseClaims: %v", err)
			}
			var got []docs.ProseClaim
			for _, c := range claims {
				got = append(got, docs.ProseClaim{Doc: c.Doc, Substr: c.Substr})
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ProseClaims() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestProse_reader exercises docs.Prose against the real MANUAL.md, the same
// way docs.Table is exercised against real documents elsewhere in this
// package.
func TestProse_reader(t *testing.T) {
	got, err := docs.Prose("MANUAL.md", "link a specific event line")
	if err != nil {
		t.Fatalf("Prose: %v", err)
	}
	if !strings.Contains(got, "link a specific event line") {
		t.Errorf("Prose() = %q, want it to contain %q", got, "link a specific event line")
	}
}

func TestProse_readerErrors(t *testing.T) {
	t.Run("substring matches no claim", func(t *testing.T) {
		_, err := docs.Prose("MANUAL.md", "zzz_no_such_claim_in_the_manual_zzz")
		if err == nil {
			t.Fatal("want error: substring matches no claim")
		}
	})
	t.Run("substring matches two or more claims", func(t *testing.T) {
		_, err := docs.Prose("MANUAL.md", "the")
		if err == nil {
			t.Fatal("want error: substring is ambiguous across multiple claims")
		}
	})
}

// TestProse_register_cannot_grow proves the shrink-only guards by mutation
// against synthetic documents, in all three directions the task requires:
// new debt, stale debt, and (in TestProse_inventory_is_complete) zero
// claims. AuditProse takes slices and a map rather than reading the
// filesystem specifically so this proof does not depend on the real
// documents' current content.
func TestProse_register_cannot_grow(t *testing.T) {
	const claimA = "`run_id` is always the same across `--events` and `--log`."
	const claimB = "`request_id` is never reused across `--log` entries."

	tests := []struct {
		name      string
		src       string
		claims    []docs.ProseClaim
		baseline  func(refs []docs.ProseRef) map[string]bool
		wantNew   int
		wantStale int
	}{
		{
			name:     "clean: executed claim, empty register",
			src:      claimA,
			claims:   []docs.ProseClaim{{Doc: "SYN.md", Substr: "is always the same"}},
			baseline: func([]docs.ProseRef) map[string]bool { return map[string]bool{} },
		},
		{
			name:   "clean: unexecuted claim, registered",
			src:    claimA,
			claims: nil,
			baseline: func(refs []docs.ProseRef) map[string]bool {
				return map[string]bool{refs[0].Key(): true}
			},
		},
		{
			name:     "MUTATION new debt: unexecuted claim absent from register",
			src:      claimA,
			claims:   nil,
			baseline: func([]docs.ProseRef) map[string]bool { return map[string]bool{} },
			wantNew:  1,
		},
		{
			name:   "MUTATION stale debt: executed claim still registered",
			src:    claimA,
			claims: []docs.ProseClaim{{Doc: "SYN.md", Substr: "is always the same"}},
			baseline: func(refs []docs.ProseRef) map[string]bool {
				return map[string]bool{refs[0].Key(): true}
			},
			wantStale: 1,
		},
		{
			name:   "MUTATION both at once",
			src:    claimA + " " + claimB,
			claims: []docs.ProseClaim{{Doc: "SYN.md", Substr: "is always the same"}},
			baseline: func(refs []docs.ProseRef) map[string]bool {
				// refs[0] (claimA) is executed and still registered -> stale.
				// refs[1] (claimB) is unexecuted and unregistered -> new.
				return map[string]bool{refs[0].Key(): true}
			},
			wantNew:   1,
			wantStale: 1,
		},
		{
			name:     "exempt claim is neither new nor stale",
			src:      "<!-- doc-check: prose-not-executable why -->\n" + claimA,
			claims:   nil,
			baseline: func([]docs.ProseRef) map[string]bool { return map[string]bool{} },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := docs.ExtractProse("SYN.md", tt.src)
			if len(refs) == 0 {
				t.Fatal("fixture produced zero refs; the fixture itself is broken")
			}
			audit := docs.AuditProse(refs, tt.claims, tt.baseline(refs))
			if len(audit.NewDebt) != tt.wantNew {
				t.Errorf("NewDebt = %d, want %d: %v", len(audit.NewDebt), tt.wantNew, audit.NewDebt)
			}
			if len(audit.StaleDebt) != tt.wantStale {
				t.Errorf("StaleDebt = %d, want %d: %v", len(audit.StaleDebt), tt.wantStale, audit.StaleDebt)
			}
		})
	}
}

// maxProseMarkers caps how much of the documentation can declare a prose
// claim unexecutable, the same guard maxExemptTables is for tables: without
// a cap the check can be hollowed out one paragraph at a time. 105-ish
// claims at the table register's ~10% ratio is about eleven; twelve gives
// one marker of headroom and raising it is still a deliberate, diffable act.
const maxProseMarkers = 12

// proseBaselineFile holds the prose claims still owed an executor, beside
// the documents it is about -- same contract as
// docs/table-execution-baseline.txt: a debt register, not an exemption
// list, and it can only shrink.
var proseBaselineFile = filepath.Join(docs.Dir, "prose-claim-baseline.txt")

func readProseBaseline() (map[string]bool, error) {
	data, err := os.ReadFile(proseBaselineFile)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, nil
}

// countProseMarkers counts marker occurrences, not the claims each one
// exempts -- one marker can cover several claims in the same block, and the
// cap is on the deliberate act of writing a marker, not on its blast radius.
func countProseMarkers(t *testing.T) int {
	t.Helper()
	count := 0
	for _, doc := range docs.Docs {
		src, err := docs.ReadDoc(doc)
		if err != nil {
			t.Fatalf("reading %s: %v", doc, err)
		}
		for _, line := range strings.Split(src, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), docs.ProseMarker) {
				count++
			}
		}
	}
	return count
}

// TestProse_inventory_is_complete is about extraction: claims are found,
// non-zero overall and per document, every shape still matches something (a
// rotted predicate is a build failure, not a silent narrowing), the marker
// cap holds, and every claim is accounted for as executed, exempt, or
// registered.
func TestProse_inventory_is_complete(t *testing.T) {
	claims, err := docs.ProseClaims(repoRoot)
	if err != nil {
		t.Fatalf("reading prose claims from test sources: %v", err)
	}
	if len(claims) == 0 {
		t.Fatal("no test anywhere reads a documented prose claim via docs.Prose; the extractor is broken")
	}

	var all []docs.ProseRef
	shapesSeen := map[string]bool{}
	for _, doc := range docs.Docs {
		refs, invErr := docs.ProseInventory(doc)
		if invErr != nil {
			t.Fatalf("prose inventory of %s: %v", doc, invErr)
		}
		if len(refs) == 0 {
			t.Fatalf("no checkable prose claims found in %s; the extractor is broken, not the document", doc)
		}
		for _, r := range refs {
			shapesSeen[r.Shape] = true
		}
		all = append(all, refs...)
	}
	if len(all) == 0 {
		t.Fatal("no checkable prose claims found in either document; this must never read as a clean register")
	}

	for _, shape := range []string{"modal", "same-as", "written-appears", "given-curlew"} {
		if !shapesSeen[shape] {
			t.Errorf("no claim anywhere matches shape %q; the predicate has rotted", shape)
		}
	}

	if markerCount := countProseMarkers(t); markerCount > maxProseMarkers {
		t.Fatalf("%d prose-not-executable markers, cap is %d; raise the cap deliberately or execute a claim instead",
			markerCount, maxProseMarkers)
	}

	baseline, err := readProseBaseline()
	if err != nil {
		t.Fatalf("reading %s: %v", proseBaselineFile, err)
	}
	audit := docs.AuditProse(all, claims, baseline)

	if len(audit.NewDebt) > 0 || len(audit.Unresolved) > 0 {
		var b strings.Builder
		if len(audit.NewDebt) > 0 {
			fmt.Fprintf(&b, "%d prose claim(s) state something about the binary that no test runs.\n", len(audit.NewDebt))
			b.WriteString("Execute each one with docs.Prose(doc, substring), or mark it with\n    ")
			b.WriteString(docs.ProseMarker)
			b.WriteString(" <why> -->\non the line before its paragraph.\n\n")
			for _, r := range audit.NewDebt {
				fmt.Fprintf(&b, "  %s\n    baseline id: %s\n", r, r.Key())
			}
		}
		if len(audit.Unresolved) > 0 {
			b.WriteString("\ndocs.Prose(...) call(s) that matched no claim, or an ambiguous one:\n")
			for _, c := range audit.Unresolved {
				fmt.Fprintf(&b, "  %s:%d docs.Prose(%q, %q)\n", c.File, c.Line, c.Doc, c.Substr)
			}
		}
		t.Fatal(b.String())
	}

	if len(audit.StaleDebt) > 0 {
		sort.Strings(audit.StaleDebt)
		t.Errorf("%d baseline entr(ies) are now executed or exempt — delete them from %s so the debt cannot grow back:\n  %s",
			len(audit.StaleDebt), proseBaselineFile, strings.Join(audit.StaleDebt, "\n  "))
	}

	t.Logf("%d prose claims: %d executed, %d exempt (cap %d), %d owed an executor (%s)",
		len(all), len(audit.Executed), len(audit.Exempt), maxProseMarkers, len(audit.StillOwed), proseBaselineFile)
}
