package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/weiqigod/curlew/internal/datadriven"
	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/variable"
)

// The three built-in iteration variables, in both documents.
//
// Every row is arithmetic — `_index` zero-based, `_count` one-based, `_total`
// the number of rows that will run — and arithmetic stated in prose beside a
// loop is the cheapest thing in either document to get wrong by one.
//
// `_total` is the row worth running rather than reading. Both tables say the
// rows that *will execute*, not the rows in the file, so a `limit:` that
// truncates the data must move it. Nothing had ever asked.

// iterationTables are the two tables. They share a header, so each is found by
// its section.
var iterationTables = []struct {
	doc    string
	where  string
	header []string
}{
	{"CLI_SPECIFICATION.md", "10.2 Iteration Variables", []string{"Variable", "Meaning"}},
	{"MANUAL.md", "Built-in iteration variables", []string{"Variable", "Meaning"}},
}

// expectation is what one row of the table says a variable holds, as a function
// of the row's position and how many rows will run.
type expectation func(pos, total int) string

// classify turns a Meaning cell into the arithmetic it describes. A row whose
// wording matches nothing is an error rather than a skip: silently ignoring a
// reworded row is how a table stops being executed without anyone deleting a
// test.
func classify(meaning string) (expectation, error) {
	m := strings.ToLower(meaning)
	switch {
	case strings.Contains(m, "zero-based") && strings.Contains(m, "index"):
		return func(pos, _ int) string { return fmt.Sprint(pos) }, nil
	case strings.Contains(m, "one-based"):
		return func(pos, _ int) string { return fmt.Sprint(pos + 1) }, nil
	case strings.Contains(m, "total"):
		return func(_, total int) string { return fmt.Sprint(total) }, nil
	}
	return nil, fmt.Errorf("no arithmetic recognised in %q", meaning)
}

// iterationRow is one row of the table, ready to run.
type iterationRow struct {
	variable string // "_index"
	want     expectation
}

func readIterationTable(t *testing.T, doc, where string, header []string) []iterationRow {
	t.Helper()

	hdr, rows, err := docs.TableUnder(doc, where, header...)
	if err != nil {
		t.Fatalf("%s under %q: %v", doc, where, err)
	}
	varCol, meaningCol := docs.Column(hdr, "Variable"), docs.Column(hdr, "Meaning")
	if varCol < 0 || meaningCol < 0 {
		t.Fatalf("%s under %q lost a column: %v", doc, where, hdr)
	}

	var out []iterationRow
	for _, row := range rows {
		if len(row) <= meaningCol {
			continue
		}
		name := strings.Trim(docs.FirstName(row[varCol]), "{}")
		if name == "" {
			continue
		}
		want, classifyErr := classify(row[meaningCol])
		if classifyErr != nil {
			t.Errorf("%s under %q, row %q: %v", doc, where, name, classifyErr)
			continue
		}
		out = append(out, iterationRow{variable: name, want: want})
	}
	if len(out) == 0 {
		t.Fatalf("%s under %q: no variables read from the table", doc, where)
	}
	return out
}

// recorder is a server that remembers every path it was asked for.
type recorder struct {
	mu    sync.Mutex
	paths []string
}

func (rec *recorder) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.paths = append(rec.paths, r.URL.Path)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
}

func (rec *recorder) sorted() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	out := append([]string{}, rec.paths...)
	sort.Strings(out)
	return out
}

// runIterations executes a data-driven collection over rowCount rows, with the
// three variables encoded into the request path, and returns the paths the
// server saw.
func runIterations(t *testing.T, bin string, vars []string, rowCount int, extra string) []string {
	t.Helper()

	rec := &recorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	dir := t.TempDir()

	csv := "name\n"
	for i := 0; i < rowCount; i++ {
		csv += fmt.Sprintf("row%d\n", i)
	}
	dataPath := filepath.Join(dir, "rows.csv")
	if err := os.WriteFile(dataPath, []byte(csv), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	// One path segment per documented variable, in table order.
	segments := make([]string, len(vars))
	for i, v := range vars {
		segments[i] = "{{" + v + "}}"
	}

	collection := fmt.Sprintf(
		"name: iteration-vars\nrequests:\n"+
			"  - name: Each row\n"+
			"    data_driven:\n      source: \"./rows.csv\"\n%s"+
			"    request:\n      method: GET\n      url: \"%s/r/%s\"\n",
		extra, srv.URL, strings.Join(segments, "/"))

	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(collection), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	cmd := exec.Command(bin, "run", path)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, out)
	}
	return rec.sorted()
}

func TestDocTables_iterationVariablesAreWhatTheyClaim(t *testing.T) {
	bin := buildBinary(t)

	for _, it := range iterationTables {
		t.Run(it.doc, func(t *testing.T) {
			table := readIterationTable(t, it.doc, it.where, it.header)

			names := make([]string, len(table))
			for i, r := range table {
				names[i] = r.variable
			}

			const rowCount = 3
			got := runIterations(t, bin, names, rowCount, "")

			want := make([]string, rowCount)
			for pos := 0; pos < rowCount; pos++ {
				segs := make([]string, len(table))
				for i, r := range table {
					segs[i] = r.want(pos, rowCount)
				}
				want[pos] = "/r/" + strings.Join(segs, "/")
			}
			sort.Strings(want)

			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("%s under %q documents %v.\n  server saw: %v\n  table says: %v",
					it.doc, it.where, names, got, want)
			}
		})
	}
}

// The other direction: an iteration variable the binary injects and no table
// names is a variable nobody knows they can ask for.
//
// The set is read out of the injector rather than restated here — calling it
// and looking at what appeared is the only list that cannot drift from it.
func TestDocTables_everyInjectedIterationVariableIsDocumented(t *testing.T) {
	base := variable.NewScope(map[string]string{})
	if err := base.Resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	before := base.Resolved()

	injected := map[string]bool{}
	for name := range datadriven.InjectIterationVars(base, datadriven.Row{}, 0, 1).Resolved() {
		if _, existed := before[name]; !existed && strings.HasPrefix(name, "_") {
			injected[name] = true
		}
	}
	if len(injected) == 0 {
		t.Fatal("InjectIterationVars injected nothing; this test is reading the wrong scope, not finding a gap")
	}

	for _, it := range iterationTables {
		documented := map[string]bool{}
		for _, row := range readIterationTable(t, it.doc, it.where, it.header) {
			documented[row.variable] = true
		}
		for name := range injected {
			if !documented[name] {
				t.Errorf("%s under %q omits %q, which every data-driven iteration defines",
					it.doc, it.where, name)
			}
		}
	}
}

// "Total rows that will execute" — the manual and the specification both say
// *will execute*, which is not the same as the rows in the file. A `limit:`
// truncating five rows to two has to move `_total` to 2.
func TestDocTables_totalCountsRowsThatWillRunNotRowsInTheFile(t *testing.T) {
	bin := buildBinary(t)

	// The claim is only worth testing if both documents actually make it.
	for _, it := range iterationTables {
		_, rows, err := docs.TableUnder(it.doc, it.where, it.header...)
		if err != nil {
			t.Fatalf("%s under %q: %v", it.doc, it.where, err)
		}
		said := false
		for _, row := range rows {
			joined := strings.ToLower(strings.Join(row, " "))
			if strings.Contains(joined, "total") &&
				(strings.Contains(joined, "will execute") || strings.Contains(joined, "will run")) {
				said = true
			}
		}
		if !said {
			t.Fatalf("%s under %q no longer says _total counts the rows that will run; "+
				"this test is checking a claim the document has dropped", it.doc, it.where)
		}
	}

	const (
		rowCount = 5
		limit    = 2
	)
	got := runIterations(t, bin, []string{"_count", "_total"}, rowCount,
		fmt.Sprintf("      limit: %d\n", limit))

	want := []string{"/r/1/2", "/r/2/2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("with limit: %d over %d rows, the server saw %v; the tables say %v",
			limit, rowCount, got, want)
	}
}
