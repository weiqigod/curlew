package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// "Where CEL appears" — two sites, and the sentence under the table saying
// where it deliberately does not.
//
// A table of sites is the kind of claim that goes stale in the safe direction:
// a site that quietly stopped accepting CEL still parses everything else, and
// a site that quietly started accepting it is a second expression language
// nobody decided to add. So both rows are compiled and evaluated, the count is
// held to the prose, and `extract:` is asked for a CEL expression to confirm it
// still refuses.

// celSite is how one row of the table is exercised.
type celSite struct {
	// match is the distinguishing text of the Site cell.
	match string
	// collection embeds a CEL expression at that site. The expression decides
	// whether the request runs (if:) or whether the run passes (cel:).
	collection func(base, expr string) string
	// observed reports what the site did with a true expression and a false
	// one: the request runs or not, the run passes or not.
	observedFrom func(ranRequest bool, exitCode int) bool
}

var celSites = []celSite{
	{
		match: "if:",
		collection: func(base, expr string) string {
			return fmt.Sprintf("name: cel\nrequests:\n  - name: One\n    if: %q\n"+
				"    request:\n      method: GET\n      url: \"%s/one\"\n", expr, base)
		},
		// An `if:` decides whether the request is sent at all.
		observedFrom: func(ranRequest bool, _ int) bool { return ranRequest },
	},
	{
		match: "cel:",
		collection: func(base, expr string) string {
			return fmt.Sprintf("name: cel\nrequests:\n  - name: One\n"+
				"    request:\n      method: GET\n      url: \"%s/one\"\n"+
				"    assertions:\n      cel:\n        - %q\n", base, expr)
		},
		// A `cel:` assertion decides whether the run passes.
		observedFrom: func(_ bool, exitCode int) bool { return exitCode == 0 },
	},
}

// runCel runs a collection carrying expr at one site and reports whether the
// request was sent and what the run exited with.
func runCel(t *testing.T, bin string, site celSite, expr string) (bool, int, string) {
	t.Helper()

	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[1,2],"total":2}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(site.collection(srv.URL, expr)), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	cmd := exec.Command(bin, "run", path)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return called, code, string(out)
}

func siteFor(t *testing.T, cell string) (celSite, bool) {
	t.Helper()
	for _, s := range celSites {
		if strings.Contains(cell, s.match) {
			return s, true
		}
	}
	return celSite{}, false
}

func TestDocTables_celAppearsAtTheSitesListed(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.TableUnder("MANUAL.md", "Where CEL appears", "Site", "Type", "Example")
	if err != nil {
		t.Fatalf("CEL site table: %v", err)
	}
	siteCol, typeCol := docs.Column(hdr, "Site"), docs.Column(hdr, "Type")
	if siteCol < 0 || typeCol < 0 {
		t.Fatalf("CEL site table lost a column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= typeCol {
			continue
		}
		cell := row[siteCol]
		site, ok := siteFor(t, cell)
		if !ok {
			t.Errorf("the CEL site table lists %q, which no case exercises; write one rather than "+
				"leaving the site unproven", cell)
			continue
		}
		if got := docs.FirstName(row[typeCol]); got != "bool" {
			t.Errorf("the CEL site table gives %q the type %q; this test only knows how to run bool sites",
				cell, got)
			continue
		}
		seen++

		t.Run(site.match, func(t *testing.T) {
			// A true expression and a false one must lead to different
			// outcomes, or the site is accepting CEL and ignoring it.
			whenTrue, trueCode, trueOut := runCel(t, bin, site, "response.status == 200 || true")
			whenFalse, falseCode, falseOut := runCel(t, bin, site, "1 == 2")

			if site.observedFrom(whenTrue, trueCode) == site.observedFrom(whenFalse, falseCode) {
				t.Errorf("the manual says CEL appears at %q; a true expression and a false one led to "+
					"the same outcome, so the expression is not being evaluated there.\n"+
					"--- true ---\n%s\n--- false ---\n%s", cell, trueOut, falseOut)
			}
			if !site.observedFrom(whenTrue, trueCode) {
				t.Errorf("at %q, a true CEL expression did not produce the passing outcome:\n%s", cell, trueOut)
			}
		})
	}
	if seen == 0 {
		t.Fatal("no sites read from the CEL site table")
	}
}

// "CEL appears at two sites" — a count stated in prose beside the list it
// counts, which is the cheapest kind of drift.
func TestDocTables_theCELSiteCountMatchesTheTable(t *testing.T) {
	_, rows, err := docs.TableUnder("MANUAL.md", "Where CEL appears", "Site", "Type", "Example")
	if err != nil {
		t.Fatalf("CEL site table: %v", err)
	}
	data, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("the CEL site table has %d rows", len(rows))
	}
	if !strings.Contains(data, "CEL appears at two sites") {
		t.Error("the manual no longer says how many sites CEL appears at")
	}
}

// "`extract:` intentionally remains JSONPath-only — there is exactly one
// extraction language." The sentence under the table is the reason the table
// has two rows and not three, so it is run as well: a CEL expression in an
// extract must not quietly become a third site.
func TestDocTables_extractRemainsJSONPathOnly(t *testing.T) {
	bin := buildBinary(t)

	data, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}
	if !strings.Contains(data, "intentionally remains JSONPath-only") {
		t.Fatal("the manual no longer says extract: is JSONPath-only; this test checks a claim it has dropped")
	}

	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[1,2],"total":2}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	body := fmt.Sprintf("name: cel\nrequests:\n  - name: One\n"+
		"    request:\n      method: GET\n      url: \"%s/one\"\n"+
		"    extract:\n      n: \"response.body.total\"\n"+
		"  - name: Two\n    request:\n      method: GET\n      url: \"%s/two/{{n}}\"\n", srv.URL, srv.URL)
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	cmd := exec.Command(bin, "run", path)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		t.Errorf("a CEL expression in extract: ran to completion; the manual says extraction is "+
			"JSONPath-only:\n%s", out)
	}
	if !called {
		t.Fatalf("the run never reached the server, so it failed for some other reason:\n%s", out)
	}
	// And it must fail *as* an extraction problem. A run that died of something
	// incidental would satisfy the check above while proving nothing.
	lower := strings.ToLower(string(out))
	if !strings.Contains(lower, "extract") && !strings.Contains(lower, "jsonpath") {
		t.Errorf("the run failed, but not with anything about extraction; this test cannot tell "+
			"a JSONPath-only extractor from an unrelated failure:\n%s", out)
	}
}
