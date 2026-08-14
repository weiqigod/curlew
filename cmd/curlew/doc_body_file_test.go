package main

import (
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The two body-file tables: what Content-Type an extension produces, and what
// each variant does when it recognises none.
//
// Both are claims about a header that reaches a server, and both are stated
// beside prose that hedges — "typical", "OS-dependent MIME database". The hedge
// is honest and it is also what let the rows go unchecked: nobody can tell by
// reading whether a row is stale or merely host-specific. So the rows are run
// twice over. Curlew's chosen header must equal what the host's MIME database
// says, which is the mechanism the document names and holds on any machine; and
// the host's answer must be one the row lists, which is what makes the row
// worth printing.

// Extensions and media types are pulled out of a cell by their shape rather
// than by their backticks. A cell whose whole contents are one code span
// arrives here already unwrapped, and one that holds several does not, so
// counting backticks tells you about the markdown rather than about the row.
// A reader recognises `.json` and `text/csv` by what they look like; so does
// this.
var (
	extensionShape = regexp.MustCompile(`\.[a-z0-9]+`)
	mediaTypeShape = regexp.MustCompile(`[a-z]+/[a-z0-9.+-]+(?:; ?charset=[a-z0-9-]+)?`)
)

func extensionsIn(cell string) []string { return extensionShape.FindAllString(cell, -1) }
func mediaTypesIn(cell string) []string { return mediaTypeShape.FindAllString(cell, -1) }

// extensionRow is one row of the manual's extension table.
type extensionRow struct {
	ext string // ".json"
	// want are the media types the row allows for this extension. A cell
	// joined by "or" offers alternatives for every extension; a cell joined by
	// commas pairs with the extensions positionally.
	want []string
}

// unknownRow is a row about an extension the database does not know. Both
// tables have these two, spelled differently.
type unknownRow struct {
	binary bool
	// wantType is the media type the variant falls back to, or "" for the
	// variant that sets no header at all.
	wantType string
}

func readExtensionTable(t *testing.T) ([]extensionRow, []unknownRow) {
	t.Helper()

	hdr, rows, err := docs.TableUnder("MANUAL.md", "Loading the body from a file",
		"Extension", "Detected Content-Type (typical)")
	if err != nil {
		t.Fatalf("extension table: %v", err)
	}
	extCol := docs.Column(hdr, "Extension")
	typeCol := docs.Column(hdr, "Detected Content-Type (typical)")
	if extCol < 0 || typeCol < 0 {
		t.Fatalf("extension table lost a column: %v", hdr)
	}

	var (
		exts     []extensionRow
		unknowns []unknownRow
	)
	for _, row := range rows {
		if len(row) <= typeCol {
			continue
		}
		left, right := row[extCol], row[typeCol]

		if strings.Contains(strings.ToLower(left), "unknown") {
			u := unknownRow{binary: strings.Contains(strings.ToLower(left), "binary")}
			if types := mediaTypesIn(right); len(types) == 1 {
				u.wantType = types[0]
			}
			unknowns = append(unknowns, u)
			continue
		}

		names, types := extensionsIn(left), mediaTypesIn(right)
		if len(names) == 0 || len(types) == 0 {
			t.Errorf("extension row %q / %q names no extension or no media type", left, right)
			continue
		}
		switch {
		case strings.Contains(right, " or "), len(types) == 1:
			// One type, or a list of alternatives joined by "or": every
			// extension in the row may produce any of them.
			for _, n := range names {
				exts = append(exts, extensionRow{ext: n, want: types})
			}
		case len(names) == len(types):
			for i, n := range names {
				exts = append(exts, extensionRow{ext: n, want: []string{types[i]}})
			}
		default:
			t.Errorf("extension row %q lists %d extensions and %d media types, and does not join them "+
				"with \"or\"; the row cannot be read as either alternatives or pairs", left, len(names), len(types))
		}
	}
	if len(exts) == 0 {
		t.Fatal("no extensions read from the manual's body-file table")
	}
	if len(unknowns) != 2 {
		t.Fatalf("the manual's body-file table has %d unknown-extension rows; both variants need one", len(unknowns))
	}
	return exts, unknowns
}

// sendBodyFile writes a body file with the given extension, posts it with the
// named field, and returns the Content-Type the server saw ("" when none).
func sendBodyFile(t *testing.T, bin, field, ext string) string {
	t.Helper()

	var seen string
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Content-Type")
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	body := filepath.Join(dir, "payload"+ext)
	if err := os.WriteFile(body, []byte("payload"), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	collection := fmt.Sprintf(
		"name: body-file\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
			"      url: %q\n      %s: %q\n", srv.URL+"/b", field, body)
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(collection), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	cmd := exec.Command(bin, "run", path)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("run with %s %q: %v\n%s", field, ext, runErr, out)
	}
	if !called {
		t.Fatalf("the server was never called for %s %q:\n%s", field, ext, out)
	}
	return seen
}

// mediaType drops any parameters, so "text/csv; charset=utf-8" compares as
// "text/csv" when a row lists it without the charset.
func mediaType(v string) string {
	mt, _, err := mime.ParseMediaType(v)
	if err != nil {
		return v
	}
	return mt
}

func allows(want []string, got string) bool {
	for _, w := range want {
		if w == got || mediaType(w) == mediaType(got) {
			return true
		}
	}
	return false
}

func TestDocTables_bodyFileExtensionsDetectWhatIsDocumented(t *testing.T) {
	bin := buildBinary(t)
	exts, _ := readExtensionTable(t)

	for _, row := range exts {
		t.Run(row.ext, func(t *testing.T) {
			// What the mechanism the document names says on this host.
			host := mime.TypeByExtension(row.ext)
			if host == "" {
				t.Fatalf("this host's MIME database knows nothing about %q, though the manual "+
					"lists it; the row is describing a mapping that does not exist here", row.ext)
			}
			if !allows(row.want, host) {
				t.Errorf("the manual says %s detects %v; this host's MIME database says %q",
					row.ext, row.want, host)
			}

			for _, field := range []string{"body_file", "body_binary_file"} {
				got := sendBodyFile(t, bin, field, row.ext)
				if got != host {
					t.Errorf("%s with %s sent Content-Type %q; mime.TypeByExtension, which the "+
						"specification names as the mechanism, says %q", field, row.ext, got, host)
				}
			}
		})
	}
}

// The §5.1.2 table: the two variants differ only in what they do when the
// extension means nothing.
func TestDocTables_unknownExtensionFallbackMatchesBothTables(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Variant", "Unknown extension")
	if err != nil {
		t.Fatalf("variant table: %v", err)
	}
	variantCol, behaviourCol := docs.Column(hdr, "Variant"), docs.Column(hdr, "Unknown extension")
	if variantCol < 0 || behaviourCol < 0 {
		t.Fatalf("variant table lost a column: %v", hdr)
	}

	// An extension no MIME database has an entry for. The premise of the whole
	// table is that this is possible.
	const unknownExt = ".curlewnotatype"
	if host := mime.TypeByExtension(unknownExt); host != "" {
		t.Fatalf("this host maps %q to %q; the fixture needs an extension nothing knows", unknownExt, host)
	}

	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) <= behaviourCol {
			continue
		}
		variant := docs.FirstName(row[variantCol])
		if variant != "body_file" && variant != "body_binary_file" {
			t.Errorf("the variant table names %q, which is not a body field", variant)
			continue
		}
		seen[variant] = true

		got := sendBodyFile(t, bin, variant, unknownExt)
		cell := row[behaviourCol]

		if types := mediaTypesIn(cell); len(types) == 1 {
			if got != types[0] {
				t.Errorf("%s with an unknown extension sent Content-Type %q; the specification says %q",
					variant, got, types[0])
			}
			continue
		}
		// No media type named: the row says the header is left unset.
		if !strings.Contains(strings.ToLower(cell), "unset") {
			t.Errorf("%s's row names no media type and does not say the header is left unset: %q", variant, cell)
			continue
		}
		if got != "" {
			t.Errorf("the specification says %s leaves Content-Type unset for an unknown extension; "+
				"the server saw %q", variant, got)
		}
	}

	for _, want := range []string{"body_file", "body_binary_file"} {
		if !seen[want] {
			t.Errorf("the specification's variant table has no row for %s", want)
		}
	}

	// The manual's two unknown-extension rows say the same thing, and must.
	_, unknowns := readExtensionTable(t)
	for _, u := range unknowns {
		variant := "body_file"
		if u.binary {
			variant = "body_binary_file"
		}
		got := sendBodyFile(t, bin, variant, unknownExt)
		if got != u.wantType {
			t.Errorf("the manual says %s falls back to %q for an unknown extension; the server saw %q",
				variant, u.wantType, got)
		}
	}
}

// An explicit header wins over detection — the sentence both tables sit under.
func TestDocTables_explicitContentTypeBeatsDetection(t *testing.T) {
	bin := buildBinary(t)

	for _, field := range []string{"body_file", "body_binary_file"} {
		var seen string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = r.Header.Get("Content-Type")
			_, _ = w.Write([]byte(`{}`))
		}))

		dir := t.TempDir()
		body := filepath.Join(dir, "payload.json")
		if err := os.WriteFile(body, []byte(`{"a":1}`), 0o600); err != nil {
			t.Fatalf("write body: %v", err)
		}
		collection := fmt.Sprintf(
			"name: explicit\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
				"      url: %q\n      headers:\n        Content-Type: \"application/vnd.curlew+test\"\n"+
				"      %s: %q\n", srv.URL+"/b", field, body)
		path := filepath.Join(dir, "c.yaml")
		if err := os.WriteFile(path, []byte(collection), 0o600); err != nil {
			t.Fatalf("write collection: %v", err)
		}

		cmd := exec.Command(bin, "run", path)
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			t.Fatalf("run: %v\n%s", runErr, out)
		}
		srv.Close()

		if seen != "application/vnd.curlew+test" {
			t.Errorf("%s with an explicit Content-Type sent %q; detection overrode the header", field, seen)
		}
	}
}
