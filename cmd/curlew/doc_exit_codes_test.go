package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The exit-code contract is stated three times — the manual's master table, the
// manual's appendix, and the specification — and was checked nowhere. Three
// copies of a contract drift pairwise: any two can agree while the third goes
// its own way, and the binary is free to disagree with all of them.
//
// So: the three tables must list the same codes, and each code must be produced
// by an invocation rather than merely described.

// exitCodeTables are the three statements of the contract.
var exitCodeTables = []struct {
	doc    string
	where  string
	header []string
	column string
}{
	{"MANUAL.md", "4.3 Exit codes", []string{"Code", "Meaning"}, "Code"},
	{"MANUAL.md", "D. Exit codes", []string{"Code", "Meaning"}, "Code"},
	{"CLI_SPECIFICATION.md", "17. Exit Codes", []string{"Code", "Meaning", "CI treatment"}, "Code"},
}

// codeCell reads a code out of a first column written as `0`, `130`.
var codeCell = regexp.MustCompile(`^\d+$`)

// documentedExitCodes reads the integer exit codes out of column in the table
// found under where in doc. column is a parameter rather than a hardcoded
// "Code" because docs/UI_SPECIFICATION.md's per-command table (§2.4, read by
// TestExitCodes_all_surfaces_agree, M26-002) heads its exit-code column
// "Exit", not "Code".
func documentedExitCodes(t *testing.T, doc, where, column string, header []string) []int {
	t.Helper()

	hdr, rows, err := docs.TableUnder(doc, where, header...)
	if err != nil {
		t.Fatalf("%s under %q: %v", doc, where, err)
	}
	col := docs.Column(hdr, column)
	if col < 0 {
		t.Fatalf("%s under %q: no %s column in %v", doc, where, column, hdr)
	}

	var codes []int
	for _, row := range rows {
		if len(row) <= col || !codeCell.MatchString(row[col]) {
			continue
		}
		n, convErr := strconv.Atoi(row[col])
		if convErr != nil {
			continue
		}
		codes = append(codes, n)
	}
	if len(codes) == 0 {
		t.Fatalf("%s under %q: no exit codes read from the table", doc, where)
	}
	sort.Ints(codes)
	return codes
}

func TestDocTables_theThreeExitCodeTablesAgree(t *testing.T) {
	var (
		first  []int
		firstS string
	)
	for i, tbl := range exitCodeTables {
		codes := documentedExitCodes(t, tbl.doc, tbl.where, tbl.column, tbl.header)
		got := fmt.Sprint(codes)
		if i == 0 {
			first, firstS = codes, got
			continue
		}
		if got != firstS {
			t.Errorf("%s under %q lists %v; %s under %q lists %v",
				tbl.doc, tbl.where, codes,
				exitCodeTables[0].doc, exitCodeTables[0].where, first)
		}
	}
}

// Every documented code must be reachable. A code nothing can produce is a
// promise to a CI pipeline that will never come true; a code the binary
// produces with no row is a pipeline treating an unknown number as a crash.
func TestDocTables_everyDocumentedExitCodeIsProduced(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	// A local server, so the passing and failing-assertion cases are real runs
	// rather than special-cased ones. Nothing leaves this machine.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}

	passing := write("pass.yaml", fmt.Sprintf(
		"name: pass\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: %q\n"+
			"    assertions:\n      status: 200\n", srv.URL,
	))
	failing := write("fail.yaml", fmt.Sprintf(
		"name: fail\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: %q\n"+
			"    assertions:\n      status: 418\n", srv.URL,
	))
	refused := write("refused.yaml",
		"name: refused\nrequests:\n  - name: r\n    request:\n      method: GET\n"+
			"      url: \"http://127.0.0.1:1/\"\n")
	broken := write("broken.yaml", "name: broken\nrequests:\n  - name: r\n   bad indent:\n")
	undefined := write("undefined.yaml",
		"name: undefined\nrequests:\n  - name: r\n    request:\n      method: GET\n"+
			"      url: \"{{NOT_DEFINED_ANYWHERE}}\"\n")

	// A data set past the guard rail, which must refuse without the opt-in.
	var rows strings.Builder
	rows.WriteString("id\n")
	for i := 0; i < 10_001; i++ {
		fmt.Fprintf(&rows, "%d\n", i)
	}
	dataFile := write("big.csv", rows.String())
	guarded := write("guarded.yaml", fmt.Sprintf(
		"name: guarded\nrequests:\n  - name: r\n    data_driven:\n      source: %q\n"+
			"    request:\n      method: GET\n      url: %q\n",
		dataFile, srv.URL,
	))

	produced := map[int]string{}
	for _, c := range []struct {
		what string
		args []string
	}{
		{"a collection whose assertions all pass", []string{"run", passing}},
		{"an assertion that fails", []string{"run", failing}},
		{"a data set over the guard rail", []string{"run", guarded}},
		{"invalid YAML", []string{"run", broken}},
		{"a refused connection", []string{"run", refused}},
		{"an undefined variable", []string{"run", undefined}},
	} {
		cmd := exec.Command(bin, c.args...)
		cmd.Env = append(os.Environ(), "NO_COLOR=1")
		out, _ := cmd.CombinedOutput()
		code := cmd.ProcessState.ExitCode()
		if _, seen := produced[code]; !seen {
			produced[code] = c.what
		}
		t.Logf("exit %d — %s", code, c.what)
		_ = out
	}

	// 130 is 128+SIGINT: it is produced by a signal, not by an invocation, and
	// TestPerfCmd_ContextCancelExitCode130 is what covers it. Requiring it here
	// would mean racing a signal against process startup for no added coverage.
	const signalCode = 130

	for _, tbl := range exitCodeTables {
		for _, code := range documentedExitCodes(t, tbl.doc, tbl.where, tbl.column, tbl.header) {
			if code == signalCode {
				continue
			}
			if _, ok := produced[code]; !ok {
				t.Errorf("%s under %q documents exit %d, which no scenario here produced (produced: %v)",
					tbl.doc, tbl.where, code, sortedCodes(produced))
			}
		}
	}

	// And the reverse: a code the binary produced with no row anywhere.
	documented := map[int]bool{}
	for _, tbl := range exitCodeTables {
		for _, code := range documentedExitCodes(t, tbl.doc, tbl.where, tbl.column, tbl.header) {
			documented[code] = true
		}
	}
	for code, what := range produced {
		if !documented[code] {
			t.Errorf("%s exits %d, which no exit-code table documents", what, code)
		}
	}
}

func sortedCodes(m map[int]string) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
