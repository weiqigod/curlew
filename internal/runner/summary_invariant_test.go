package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/datadriven"
	"github.com/weiqigod/curlew/internal/parser"
)

// summaryTestItem builds a plain GET item. When wantAssertionFailure is true,
// the item asserts a status code the stub executor never returns, so it fails
// its assertion without aborting the run -- unlike a fatal error, an assertion
// failure lets later phases proceed exactly as §11D.1's "teardown runs even
// when main fails" case requires.
func summaryTestItem(name string, wantAssertionFailure bool) parser.RequestItem {
	item := parser.RequestItem{
		Name:    name,
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
	}
	if wantAssertionFailure {
		item.Assertions = parser.Assertions{Status: parser.StatusCodes{Codes: []int{999}}}
	}
	return item
}

// summaryTestDataDrivenItem writes a rows-row CSV under dir and returns a
// data_driven request item sourced from it. One declared item, rows results --
// the exact shape that broke Total (§11D.1).
func summaryTestDataDrivenItem(t *testing.T, dir, name string, rows int) parser.RequestItem {
	t.Helper()

	var buf strings.Builder
	buf.WriteString("n\n")
	for i := 0; i < rows; i++ {
		fmt.Fprintf(&buf, "%d\n", i)
	}
	csvName := name + ".csv"
	if err := os.WriteFile(filepath.Join(dir, csvName), []byte(buf.String()), 0o600); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	return parser.RequestItem{
		Name:       name,
		DataDriven: &datadriven.Config{Source: csvName},
		Request:    parser.Request{Method: "GET", URL: "https://example.com/{{n}}"},
	}
}

// The summary is the only part of a run most consumers read. Its parts must
// add up to its total, and its total must equal what was actually reported --
// otherwise a run can announce "4 passed" out of a total of 2 and exit 0.
//
// Total was computed from DECLARED items (runPhases: the sum of each phase's
// col.*.Items) while the parts were counted from EXECUTED results
// (computeSummary, ranging over the accumulated result slice). A data_driven
// request expands one declared item into one result per row, so the two
// disagreed the moment anything expanded. §11D.1
// (docs/TESTAPI_SPECIFICATION.md).
func TestSummary_TotalEqualsWhatWasReported(t *testing.T) {
	tests := []struct {
		name          string
		setupPlain    int
		mainPlain     int
		teardownPlain int
		ddRows        int // >0 adds one extra data_driven item to main, expanding into this many results
		mainFails     bool
	}{
		{"plain requests only", 0, 3, 0, 0, false},
		{"data-driven expands one item into three", 0, 0, 0, 3, false},
		{"data-driven beside a plain request", 0, 1, 0, 3, false},
		{"data-driven across all three phases", 1, 1, 1, 3, false},
		{"single-row data-driven still balances", 0, 1, 0, 1, false},
		{"edge case - empty collection", 0, 0, 0, 0, false},
		{"edge case - setup only", 2, 0, 0, 0, false},
		{"teardown runs even when main fails", 0, 1, 1, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			col := &parser.Collection{Name: "Ledger Invariant"}

			for i := 0; i < tt.setupPlain; i++ {
				col.Setup.Items = append(col.Setup.Items, summaryTestItem(fmt.Sprintf("setup-%d", i), false))
			}
			for i := 0; i < tt.mainPlain; i++ {
				col.Requests.Items = append(col.Requests.Items, summaryTestItem(fmt.Sprintf("main-%d", i), tt.mainFails && i == 0))
			}
			if tt.ddRows > 0 {
				col.Requests.Items = append(col.Requests.Items, summaryTestDataDrivenItem(t, dir, "expand", tt.ddRows))
			}
			for i := 0; i < tt.teardownPlain; i++ {
				col.Teardown.Items = append(col.Teardown.Items, summaryTestItem(fmt.Sprintf("teardown-%d", i), false))
			}

			results, summary, err := Run(context.Background(), col, successExecutor, VarSources{CollectionDir: dir})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			if sum := summary.Passed + summary.Failed + summary.Skipped; summary.Total != sum {
				t.Errorf("Total = %d, but Passed(%d)+Failed(%d)+Skipped(%d) = %d",
					summary.Total, summary.Passed, summary.Failed, summary.Skipped, sum)
			}
			if summary.Total != len(results) {
				t.Errorf("Total = %d, but len(results) = %d -- requests[] and summary.total would disagree", summary.Total, len(results))
			}
		})
	}
}
