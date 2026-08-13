package variable

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The manual lists every dynamic function across twelve tables, split by
// category. Nothing read any of them, in either direction: a function could be
// renamed and keep its row, or added and never get one.
//
// Both directions matter and they fail differently. A documented function that
// does not exist is a user following the manual into an error. An existing
// function with no row is a feature nobody can find.

// docFuncName matches a function as the tables spell it: `{{$uuid}}`,
// `{{$faker.firstName}}`, `{{$dateAdd(...)}}`.
var docFuncName = regexp.MustCompile(`\{\{\$([A-Za-z][A-Za-z0-9.]*)`)

// undocumentedByDesign lists functions with no row, and why. An entry is a
// statement that users are not meant to find the function in the manual.
var undocumentedByDesign = map[string]string{}

// documentedFunctions reads every function name out of the manual's function
// tables, with the table each came from.
func documentedFunctions(t *testing.T) map[string]docs.TableRef {
	t.Helper()

	tables, err := docs.AllTables("MANUAL.md", "Function")
	if err != nil {
		t.Fatalf("reading the manual's function tables: %v", err)
	}
	if len(tables) < 2 {
		t.Fatalf("found %d function tables in the manual; the reader is broken", len(tables))
	}

	found := map[string]docs.TableRef{}
	for _, tbl := range tables {
		col := docs.Column(tbl.Header, "Function")
		if col < 0 {
			t.Errorf("%s: no Function column in %v", tbl.Ref, tbl.Header)
			continue
		}
		for _, row := range tbl.Rows {
			if len(row) <= col {
				continue
			}
			for _, m := range docFuncName.FindAllStringSubmatch(row[col], -1) {
				found[m[1]] = tbl.Ref
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no function names extracted from the manual; the column lookup is broken")
	}
	return found
}

func TestDocTables_everyDocumentedFunctionExists(t *testing.T) {
	seed := int64(1)
	available := map[string]bool{}
	for _, name := range NewRegistry(&seed).Available() {
		available[name] = true
	}

	for name, ref := range documentedFunctions(t) {
		if !available[name] {
			t.Errorf("%s documents {{$%s}}, which the registry does not provide", ref, name)
		}
	}
}

func TestDocTables_everyFunctionIsDocumented(t *testing.T) {
	documented := documentedFunctions(t)

	seed := int64(1)
	var missing []string
	for _, name := range NewRegistry(&seed).Available() {
		if _, ok := documented[name]; ok {
			continue
		}
		if _, exempt := undocumentedByDesign[name]; exempt {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d function(s) exist with no row in the manual's function tables:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// The one table that names its seed is the one that can be run rather than
// read. "Example (seed 42)" is a reproducibility promise: a reader who sets
// --seed 42 must see the value in the row.
//
// Each row is evaluated on its own freshly seeded registry, which is what a
// reader gets — the first draw under that seed — and is the only reading
// independent of what order the rows happen to be in.
func TestDocTables_seededExamplesReproduce(t *testing.T) {
	tables, err := docs.AllTables("MANUAL.md", "Function", "Example (seed 42)")
	if err != nil {
		t.Fatalf("reading the seeded example table: %v", err)
	}

	checked := 0
	for _, tbl := range tables {
		fnCol := docs.Column(tbl.Header, "Function")
		exCol := docs.Column(tbl.Header, "Example (seed 42)")
		if fnCol < 0 || exCol < 0 {
			t.Fatalf("%s: header lost a column: %v", tbl.Ref, tbl.Header)
		}
		for _, row := range tbl.Rows {
			if len(row) <= fnCol || len(row) <= exCol {
				continue
			}
			m := docFuncName.FindStringSubmatch(row[fnCol])
			if m == nil {
				continue
			}
			name, want := m[1], row[exCol]

			seed := int64(42)
			got, evalErr := NewRegistry(&seed).Evaluate(name, nil, map[string]string{})
			if evalErr != nil {
				t.Errorf("%s: {{$%s}}: %v", tbl.Ref, name, evalErr)
				continue
			}
			checked++
			if got != want {
				t.Errorf("%s: {{$%s}} under seed 42 documents %q, produces %q",
					tbl.Ref, name, want, got)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no seeded examples were evaluated; the table moved or its header changed")
	}
	t.Logf("%d seeded examples reproduced", checked)
}
