package docs_test

import (
	"reflect"
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
