package markdown

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §4.1a's content-type matrix.
//
// The markdown report is described in the manual as "the canonical record you
// check into git or share with a teammate", so this table is a promise about
// what a reader will find in a file they may never regenerate. Eight rows, each
// naming a fence language or a marker, and none had been run against the
// formatter — the fences were covered one at a time by hand-written tests that
// did not know the table existed.

// renderedBody is what the formatter writes for one body, without the
// surrounding report.
func renderedBody(body []byte, method, contentType string) string {
	var w bytes.Buffer
	renderBody(&w, Classify(body, method, contentType), body, len(body), false)
	return w.String()
}

// bodyTypeCase is one row of the matrix: a body that belongs to it, and the
// content type that says so.
type bodyTypeCase struct {
	// match is the distinguishing text of the Body type cell.
	match string
	// method and contentType are what the formatter is told about the body.
	method      string
	contentType string
	body        []byte
	// verbatim, when set, must survive into the rendered block unchanged.
	verbatim string
}

var bodyTypeCases = []bodyTypeCase{
	{match: "json", contentType: "application/json", body: []byte(`{"a":{"b":1}}`)},
	{match: "yaml", contentType: "application/yaml", body: []byte("b: 2\na: 1\n")},
	{match: "xml", contentType: "application/xml", body: []byte("<r><a>1</a></r>"), verbatim: "<a>1</a>"},
	{match: "text/html", contentType: "text/html", body: []byte("<p onclick=\"x()\">hi</p>"), verbatim: "<p onclick=\"x()\">hi</p>"},
	{match: "text/*", contentType: "text/plain", body: []byte("just words"), verbatim: "just words"},
	{match: "octet-stream", contentType: "application/octet-stream", body: bytes.Repeat([]byte{0x00, 0xff}, 700)},
	{match: "HEAD", method: "HEAD", contentType: "application/json", body: []byte(`{"a":1}`)},
	{match: "Empty", contentType: "application/json", body: []byte{}},
}

// fenceLanguages are the ```lang fences opened in the rendered block.
var fenceLanguages = regexp.MustCompile("(?m)^```([a-z]+)$")

func TestDocTables_markdownRenderingMatrixIsWhatIsWritten(t *testing.T) {
	hdr, rows, err := docs.TableUnder("MANUAL.md", "Markdown response files", "Body type", "Rendering")
	if err != nil {
		t.Fatalf("content-type matrix: %v", err)
	}
	typeCol, renderCol := docs.Column(hdr, "Body type"), docs.Column(hdr, "Rendering")
	if typeCol < 0 || renderCol < 0 {
		t.Fatalf("content-type matrix lost a column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= renderCol {
			continue
		}
		bodyType, rendering := row[typeCol], row[renderCol]

		var tc *bodyTypeCase
		for i := range bodyTypeCases {
			if strings.Contains(bodyType, bodyTypeCases[i].match) {
				tc = &bodyTypeCases[i]
				break
			}
		}
		if tc == nil {
			t.Errorf("§4.1a lists body type %q, which no case renders; write one rather than leaving "+
				"the row unproven", bodyType)
			continue
		}
		seen++

		t.Run(tc.match, func(t *testing.T) {
			got := renderedBody(tc.body, tc.method, tc.contentType)

			// The fence language, when the row names one.
			if lang := fenceLanguageIn(rendering); lang != "" {
				found := fenceLanguages.FindStringSubmatch(got)
				if found == nil {
					t.Fatalf("§4.1a says %s renders %q; nothing was fenced:\n%s", bodyType, rendering, got)
				}
				if found[1] != lang {
					t.Errorf("§4.1a says %s renders in a %q block; the fence is %q:\n%s",
						bodyType, lang, found[1], got)
				}
			}

			// A marker the row states literally, like `(no body)`.
			for _, marker := range markersIn(rendering) {
				if !strings.Contains(got, marker) {
					t.Errorf("§4.1a says %s renders %q; the block is:\n%s", bodyType, marker, got)
				}
			}

			if tc.verbatim != "" && !strings.Contains(got, tc.verbatim) {
				t.Errorf("§4.1a says %s renders %q; %q did not survive:\n%s",
					bodyType, rendering, tc.verbatim, got)
			}

			checkRenderingDetail(t, bodyType, rendering, tc, got)
		})
	}
	if seen == 0 {
		t.Fatal("no body types read from §4.1a")
	}
}

// fenceLanguageIn returns the fence language a Rendering cell names — the word
// in backticks immediately before "block" — or "" when it names none.
var fenceNaming = regexp.MustCompile("`?([a-z]+)`? fenced? block")

func fenceLanguageIn(rendering string) string {
	if m := fenceNaming.FindStringSubmatch(rendering); m != nil {
		return m[1]
	}
	return ""
}

// markersIn returns the literal markers a Rendering cell states — a code span
// that is itself the whole rendering, like `_(empty body)_`, rather than a
// parenthetical describing one, like "(2-space indent)".
var markerNaming = regexp.MustCompile("`(_?\\([^`]+\\)_?)`")

func markersIn(rendering string) []string {
	var out []string
	for _, m := range markerNaming.FindAllStringSubmatch(rendering, -1) {
		if strings.HasPrefix(strings.TrimSpace(rendering), "`"+m[1]+"`") {
			out = append(out, m[1])
		}
	}
	return out
}

// checkRenderingDetail runs the parts of a Rendering cell that are more than a
// fence: the indent width, the byte count, the ordering promise.
func checkRenderingDetail(t *testing.T, bodyType, rendering string, tc *bodyTypeCase, got string) {
	t.Helper()

	if n := indentWidthIn(rendering); n > 0 {
		want := "\n" + strings.Repeat(" ", n) + `"a"`
		if !strings.Contains(got, want) {
			t.Errorf("§4.1a says %s is pretty-printed with a %d-space indent; the block is:\n%s",
				bodyType, n, got)
		}
	}

	if n := byteCountIn(rendering); n > 0 {
		dumped := hexDumpedBytes(got)
		if dumped != n {
			t.Errorf("§4.1a says %s dumps the first %d bytes; %d were dumped", bodyType, n, dumped)
		}
		if !strings.Contains(got, fmt.Sprintf("%d bytes total", len(tc.body))) {
			t.Errorf("§4.1a says %s is followed by the total byte count; the block is:\n%s", bodyType, got)
		}
	}

	if strings.Contains(strings.ToLower(rendering), "stable key order") {
		twice := renderedBody(tc.body, tc.method, tc.contentType)
		if twice != got {
			t.Errorf("§4.1a says %s is re-serialised with stable key order; two renderings differ", bodyType)
		}
		// "b" was written first and must stay first: stable means the source's
		// order, not the encoder's.
		if bi, ai := strings.Index(got, "b:"), strings.Index(got, "a:"); bi < 0 || ai < 0 || bi > ai {
			t.Errorf("§4.1a says %s keeps a stable key order; the source order was not preserved:\n%s",
				bodyType, got)
		}
	}

	if strings.Contains(strings.ToLower(rendering), "never executed") {
		// The promise is that the HTML sits inside a fence rather than in the
		// document's own voice, which is what stops a viewer running it.
		if !strings.Contains(got, "```html\n") {
			t.Errorf("§4.1a says %s is preserved verbatim and never executed; it is not fenced:\n%s",
				bodyType, got)
		}
	}
}

var (
	indentNaming = regexp.MustCompile(`(\d+)-space indent`)
	bytesNaming  = regexp.MustCompile(`[Ff]irst (\d+) bytes`)
)

func indentWidthIn(rendering string) int {
	if m := indentNaming.FindStringSubmatch(rendering); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func byteCountIn(rendering string) int {
	if m := bytesNaming.FindStringSubmatch(rendering); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// hexDumpedBytes counts the bytes a hex.Dump block covers, read off its last
// offset line plus that line's byte count.
func hexDumpedBytes(rendered string) int {
	total := 0
	for _, line := range strings.Split(rendered, "\n") {
		if len(line) < 10 || !strings.HasPrefix(line, "0") {
			continue
		}
		offset, err := strconv.ParseInt(strings.TrimSpace(line[:8]), 16, 64)
		if err != nil {
			continue
		}
		// hex.Dump lines carry up to 16 bytes; count them from the ASCII pane.
		open := strings.LastIndex(line, "|")
		pre := strings.Index(line, "|")
		if open <= pre || pre < 0 {
			continue
		}
		n := open - pre - 1
		if int(offset)+n > total {
			total = int(offset) + n
		}
	}
	return total
}
