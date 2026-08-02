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
	// runner.Summary.Total counts the data-driven group as one planned item
	// while Passed counts iterations — pre-existing runner semantics the spec
	// adopts (§4.8 "matching runner.Summary.Total semantics").
	if summary["total"] != float64(1) || summary["passed"] != float64(3) {
		t.Errorf("summary = %v, want total 1 / passed 3", summary)
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
