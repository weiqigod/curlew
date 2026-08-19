package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The documented contract (docs/MANUAL.md's worked JSON example) is that
// summary.total equals the number of entries in requests[], and that total ==
// passed+failed+skipped. A consumer computing a pass rate must not be able to
// get a number above 100%.
//
// §11D.1 (docs/TESTAPI_SPECIFICATION.md) found this contract violated by a
// data_driven request: it expands one declared item into one result per row,
// and summary.total was derived from the declared count rather than the
// reported one. This test pins the actual --format json surface, one level
// above internal/runner's TestSummary_TotalEqualsWhatWasReported, because
// that is what an external consumer parses.
func TestJSONOutput_SummaryTotalMatchesRequestCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rows.csv"), []byte("n\n1\n2\n3"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: Ledger JSON
requests:
  - name: Plain
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
  - name: Expand
    data_driven:
      source: rows.csv
    request:
      method: GET
      url: "%s/{{n}}"
    assertions:
      status: 200
`, srv.URL, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	var result struct {
		Summary struct {
			Total   int `json:"total"`
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Skipped int `json:"skipped"`
		} `json:"summary"`
		Requests []json.RawMessage `json:"requests"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}

	if result.Summary.Total != len(result.Requests) {
		t.Errorf("summary.total = %d, but requests[] holds %d entries", result.Summary.Total, len(result.Requests))
	}
	if sum := result.Summary.Passed + result.Summary.Failed + result.Summary.Skipped; result.Summary.Total != sum {
		t.Errorf("summary.total = %d, but passed(%d)+failed(%d)+skipped(%d) = %d",
			result.Summary.Total, result.Summary.Passed, result.Summary.Failed, result.Summary.Skipped, sum)
	}
}
