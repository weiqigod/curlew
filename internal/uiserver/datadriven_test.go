package uiserver_test

import (
	"testing"

	"github.com/weiqigod/curlew/internal/uiserver"
)

// TestRun_DataDrivenIterations covers data-driven expansion through the UI
// server: the planned seed row is replaced by per-iteration rows, each
// carrying iteration metadata with the base slug (spec §4.8); shutdown paths
// are exercised via the run completing.
func TestRun_DataDrivenIterations(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	// Data sources resolve relative to the collection file's directory.
	writeFile(t, root, "collections/data/users.csv", "username,role\nalice,admin\nbob,viewer\ncarol,editor\n")
	writeFile(t, root, "collections/dd.yaml", `name: DD
requests:
  - name: Create user
    data_driven:
      source: data/users.csv
    request:
      method: POST
      url: "http://t.test/ok"
      body: {"username": "{{username}}", "role": "{{role}}"}
`)
	runID := startRun(t, ts, map[string]any{"collection": "collections/dd.yaml"})
	final := waitTerminal(t, ts, runID)
	if final["exit_status"] != "passed" {
		t.Fatalf("exit = %v (%v)", final["exit_status"], final["state"])
	}
	summary, _ := final["summary"].(map[string]any)
	// runner.Summary.Total is derived from the results actually reported
	// (computeSummary), not from the count of declared items, so a
	// data-driven group of 3 iterations contributes 3 to Total, matching
	// Passed. Before the fix in docs/TESTAPI_SPECIFICATION.md §11D.1, Total
	// was derived from declared items instead and read 1 here while Passed
	// read 3 — the exact "more passes than requests" defect that section
	// documents.
	if summary["total"] != float64(3) || summary["passed"] != float64(3) {
		t.Errorf("summary = %v, want total 3 / passed 3", summary)
	}

	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			Slug      string  `json:"slug"`
			Outcome   *string `json:"outcome"`
			Iteration *struct {
				Index    int    `json:"index"`
				Total    int    `json:"total"`
				BaseName string `json:"base_name"`
				BaseSlug string `json:"base_slug"`
			} `json:"iteration"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()

	if len(list.Requests) != 3 {
		t.Fatalf("rows = %d, want 3 (no leftover planned seed row)", len(list.Requests))
	}
	for i, e := range list.Requests {
		if e.Outcome == nil || *e.Outcome != "passed" {
			t.Errorf("row %d outcome = %v", i, e.Outcome)
		}
		if e.Iteration == nil {
			t.Fatalf("row %d missing iteration metadata", i)
		}
		if e.Iteration.Total != 3 || e.Iteration.BaseSlug != "create-user" || e.Iteration.BaseName != "Create user" {
			t.Errorf("row %d iteration = %+v", i, e.Iteration)
		}
	}
}
