package main

import (
	"encoding/json"
	"errors"
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

// §10.3's three retention policies, and the premise the whole table rests on.
//
// The column is headed "Retained". A policy decides what a finished run keeps,
// which means it must not decide what the run *was*: the same three iterations
// pass or fail identically whether their details are stored or thrown away.
// That premise had never been checked, and `summary` broke it — a run with a
// failing iteration exited 0 and reported "3 passed, 0 failed", because the
// summary counter reads AssertionResults and the filter had removed it. The
// CLI's own out-of-memory hint recommends the flag ("set store_results:
// summary|failed_only to bound what the run retains"), so the advice for a
// large suite was also the way to stop noticing it was broken.

// retention is what one row of the table says survives the run.
type retention int

const (
	retainEvery retention = iota
	retainNone
	retainFailingOnly
)

// classifyRetention reads a "Retained" cell. Wording that matches no rule is an
// error, so a reworded row cannot quietly stop being executed.
func classifyRetention(cell string) (retention, error) {
	c := strings.ToLower(cell)
	switch {
	case strings.Contains(c, "counts for the rest"):
		return retainFailingOnly, nil
	case strings.Contains(c, "counts only"):
		return retainNone, nil
	case strings.Contains(c, "every") && strings.Contains(c, "full"):
		return retainEvery, nil
	}
	return 0, fmt.Errorf("no retention rule recognised in %q", cell)
}

// storeResultsRow is one row of §10.3.
type storeResultsRow struct {
	policy    string // "all"
	isDefault bool   // the row marked "(default)"
	keeps     retention
}

func readStoreResultsTable(t *testing.T) []storeResultsRow {
	t.Helper()

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "store_results", "Retained")
	if err != nil {
		t.Fatalf("store_results table: %v", err)
	}
	policyCol, keepCol := docs.Column(hdr, "store_results"), docs.Column(hdr, "Retained")
	if policyCol < 0 || keepCol < 0 {
		t.Fatalf("store_results table lost a column: %v", hdr)
	}

	var out []storeResultsRow
	for _, row := range rows {
		if len(row) <= keepCol {
			continue
		}
		policy := docs.FirstName(row[policyCol])
		if policy == "" {
			continue
		}
		keeps, classifyErr := classifyRetention(row[keepCol])
		if classifyErr != nil {
			t.Errorf("row %q: %v", policy, classifyErr)
			continue
		}
		out = append(out, storeResultsRow{
			policy:    policy,
			isDefault: strings.Contains(row[policyCol], "(default)"),
			keeps:     keeps,
		})
	}
	if len(out) == 0 {
		t.Fatal("no policies read from the store_results table")
	}
	return out
}

// storeReport is what one run under one policy reported.
type storeReport struct {
	exitCode int
	passed   int
	failed   int
	// detail records, per iteration, whether its full result survived: an
	// assertion list and a status code are what "full result" means in the
	// report a user reads.
	detail []bool
}

// runStorePolicy runs three iterations, the second of which fails its status
// assertion, under the given policy. An empty policy omits the key entirely,
// which is what the "(default)" row is about.
func runStorePolicy(t *testing.T, bin, policy string) storeReport {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/2") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rows.csv"), []byte("n\n1\n2\n3\n"), 0o600); err != nil {
		t.Fatalf("write data: %v", err)
	}

	store := ""
	if policy != "" {
		store = fmt.Sprintf("      store_results: %s\n", policy)
	}
	collection := fmt.Sprintf(
		"name: store-results\nrequests:\n"+
			"  - name: Each\n"+
			"    data_driven:\n      source: \"./rows.csv\"\n%s"+
			"    request:\n      method: GET\n      url: \"%s/r/{{n}}\"\n"+
			"    assertions:\n      status: 200\n",
		store, srv.URL)
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(collection), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	cmd := exec.Command(bin, "run", path, "--format", "json")
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, runErr := cmd.Output()

	rep := storeReport{}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
		rep.exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run under policy %q: %v", policy, runErr)
	}

	var doc struct {
		Summary struct {
			Passed int `json:"passed"`
			Failed int `json:"failed"`
		} `json:"summary"`
		Requests []struct {
			Name       string `json:"name"`
			StatusCode int    `json:"status_code"`
			Assertions []struct {
				Label string `json:"label"`
			} `json:"assertions"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("parsing the report under policy %q: %v\n%s", policy, err, out)
	}

	rep.passed, rep.failed = doc.Summary.Passed, doc.Summary.Failed
	for _, r := range doc.Requests {
		rep.detail = append(rep.detail, len(r.Assertions) > 0 && r.StatusCode != 0)
	}
	if len(rep.detail) != 3 {
		t.Fatalf("policy %q produced %d iteration entries, want 3:\n%s", policy, len(rep.detail), out)
	}
	return rep
}

// The premise: retention decides what is kept, never what happened.
func TestDocTables_storeResultsDoesNotChangeTheVerdict(t *testing.T) {
	bin := buildBinary(t)
	table := readStoreResultsTable(t)

	// Whatever the table's own default row says, the honest baseline is the
	// policy that keeps everything: it cannot be hiding anything.
	var baseline storeReport
	for _, row := range table {
		if row.keeps == retainEvery {
			baseline = runStorePolicy(t, bin, row.policy)
		}
	}
	if baseline.failed == 0 {
		t.Fatal("the retain-everything policy reported no failures; the fixture no longer fails, " +
			"so a policy that hides failures would pass this test silently")
	}

	for _, row := range table {
		got := runStorePolicy(t, bin, row.policy)
		if got.failed != baseline.failed || got.passed != baseline.passed {
			t.Errorf("store_results: %s reports %d passed / %d failed; the same run under a policy "+
				"that retains everything is %d passed / %d failed. Retention changed the verdict.",
				row.policy, got.passed, got.failed, baseline.passed, baseline.failed)
		}
		if got.exitCode != baseline.exitCode {
			t.Errorf("store_results: %s exits %d; the same run retaining everything exits %d",
				row.policy, got.exitCode, baseline.exitCode)
		}
	}
}

// Each row's "Retained" cell, run.
func TestDocTables_storeResultsRetainsWhatTheTableSays(t *testing.T) {
	bin := buildBinary(t)

	// Iteration 2 is the failing one; the fixture is built that way.
	const failing = 1

	for _, row := range readStoreResultsTable(t) {
		t.Run(row.policy, func(t *testing.T) {
			got := runStorePolicy(t, bin, row.policy)

			for i, kept := range got.detail {
				want := false
				switch row.keeps {
				case retainEvery:
					want = true
				case retainNone:
					want = false
				case retainFailingOnly:
					want = i == failing
				}
				if kept != want {
					verb := map[bool]string{true: "kept", false: "dropped"}
					t.Errorf("store_results: %s %s iteration %d's full result; the table says it should be %s",
						row.policy, verb[kept], i+1, verb[want])
				}
			}
		})
	}
}

// The row marked "(default)" is what a collection with no store_results: key
// gets. Exactly one row may claim it.
func TestDocTables_storeResultsDefaultIsWhatOmittingTheKeyGives(t *testing.T) {
	bin := buildBinary(t)

	var defaults []storeResultsRow
	for _, row := range readStoreResultsTable(t) {
		if row.isDefault {
			defaults = append(defaults, row)
		}
	}
	if len(defaults) != 1 {
		t.Fatalf("%d rows of the store_results table are marked (default); exactly one must be", len(defaults))
	}

	omitted := runStorePolicy(t, bin, "")
	named := runStorePolicy(t, bin, defaults[0].policy)

	if fmt.Sprint(omitted.detail) != fmt.Sprint(named.detail) ||
		omitted.passed != named.passed || omitted.failed != named.failed {
		t.Errorf("omitting store_results retains %v (%d passed, %d failed); the table says the default "+
			"is %q, which retains %v (%d passed, %d failed)",
			omitted.detail, omitted.passed, omitted.failed,
			defaults[0].policy, named.detail, named.passed, named.failed)
	}
}
