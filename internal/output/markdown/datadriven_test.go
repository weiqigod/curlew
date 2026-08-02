package markdown

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/assertion"
)

// makeDataDrivenReport constructs a Report with a single data-driven entry
// having iterCount iterations and total source rows = total.
func makeDataDrivenReport(iterCount, total int) *Report {
	iters := make([]IterationEntry, iterCount)
	for i := range iters {
		iters[i] = IterationEntry{
			RequestID:  "req-12",
			Index:      i,
			Method:     "POST",
			URL:        fmt.Sprintf("https://api.example.com/users/%d", i+1),
			StatusCode: 201,
			DurationMs: int64(40 + i),
			StartedAt:  fixedTime,
			RespHeaders: http.Header{
				"Content-Type": {"application/json"},
			},
			RespBody: []byte(fmt.Sprintf(`{"id":%d}`, i+1)),
			Assertions: &assertion.Results{
				Passed: true,
				Items: []assertion.Result{
					{Type: "status", Expected: "201", Actual: "201", Passed: true},
				},
			},
		}
	}
	return &Report{
		CollectionName: "Sample API",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: iterCount, Passed: iterCount},
		Requests: []RequestEntry{{
			RequestID:      "req-12",
			Slug:           "seed-users",
			Name:           "Seed users",
			DataDrivenName: "Seed users",
			IterationTotal: total,
			Iterations:     iters,
		}},
	}
}

// TestMarkdown_IterationOutcomeCounts verifies the pass/fail/skip counter helper.
func TestMarkdown_IterationOutcomeCounts(t *testing.T) {
	iters := []IterationEntry{
		{Index: 0},                          // pass
		{Index: 1, Err: fmt.Errorf("boom")}, // fail
		{Index: 2, Assertions: &assertion.Results{Passed: false}}, // fail
		{Index: 3, Skipped: true},                                 // skip
		{Index: 4},                                                // pass
	}
	p, f, s := iterationOutcomeCounts(iters)
	if p != 2 || f != 2 || s != 1 {
		t.Errorf("counts = (p=%d, f=%d, s=%d), want (2,2,1)", p, f, s)
	}
}

// TestMarkdown_RenderIteration verifies the per-iteration file structure.
func TestMarkdown_RenderIteration(t *testing.T) {
	parent := &RequestEntry{
		RequestID:      "req-12",
		Slug:           "seed-users",
		DataDrivenName: "Seed users",
		IterationTotal: 5,
	}
	it := &IterationEntry{
		RequestID:  "req-12",
		Index:      0,
		Method:     "POST",
		URL:        "https://api.example.com/users",
		StatusCode: 201,
		DurationMs: 42,
		StartedAt:  fixedTime,
		RespHeaders: http.Header{
			"Content-Type": {"application/json"},
		},
		RespBody: []byte(`{"id":1}`),
		Assertions: &assertion.Results{
			Passed: true,
			Items: []assertion.Result{
				{Type: "status", Expected: "201", Actual: "201", Passed: true},
			},
		},
	}
	got := renderIterationFile(parent, it, fixedRunID)
	s := string(got)

	// Sentinel format: id=req-12-iter-0 slug=seed-users.
	if !strings.Contains(s, "id=req-12-iter-0 slug=seed-users") {
		t.Errorf("expected sentinel id=req-12-iter-0 slug=seed-users:\n%s", s)
	}
	// 11-section structure (same as M9-003 single-request).
	for _, marker := range []string{
		"# Seed users [1/5]",
		"## Notes",
		"<!-- BEGIN curlew:response",
		"## Response (deterministic)",
		"### Request",
		"### Response 201",
		"### Response metadata",
		"### Timing",
		"### Assertions",
		"<!-- END curlew:response",
		"## Analysis",
	} {
		if !strings.Contains(s, marker) {
			t.Errorf("missing marker %q in iter file:\n%s", marker, s)
		}
	}
}

// TestMarkdown_RenderIndex verifies the index.md structure and iteration table.
func TestMarkdown_RenderIndex(t *testing.T) {
	req := &RequestEntry{
		RequestID:      "req-12",
		Slug:           "seed-users",
		Name:           "Seed users",
		DataDrivenName: "Seed users",
		StartedAt:      fixedTime,
		IterationTotal: 5,
		Iterations: []IterationEntry{
			{Index: 0, DurationMs: 42},
			{Index: 1, DurationMs: 50, Err: fmt.Errorf("boom")},
			{Index: 2, DurationMs: 38},
			{Index: 3, DurationMs: 41},
			{Index: 4, DurationMs: 39},
		},
	}
	got := renderIndex(req, fixedRunID)
	s := string(got)

	if !strings.Contains(s, "id=req-12-index slug=seed-users") {
		t.Errorf("expected sentinel id=req-12-index:\n%s", s)
	}
	if !strings.Contains(s, "5 iterations (4 passed, 1 failed)") {
		t.Errorf("expected aggregate count line:\n%s", s)
	}
	if !strings.Contains(s, "[iter-0.md](iter-0.md)") {
		t.Errorf("expected iter-0 link in table:\n%s", s)
	}
	if !strings.Contains(s, "[iter-4.md](iter-4.md)") {
		t.Errorf("expected iter-4 link in table:\n%s", s)
	}
}

// TestMarkdown_RenderIndex_Truncation verifies that an index.md with more source
// rows than executed iterations renders a truncation marker row.
func TestMarkdown_RenderIndex_Truncation(t *testing.T) {
	req := &RequestEntry{
		RequestID:      "req-12",
		Slug:           "seed-users",
		DataDrivenName: "Seed users",
		StartedAt:      fixedTime,
		IterationTotal: 1500,
		Iterations:     make([]IterationEntry, 1000),
	}
	for i := range req.Iterations {
		req.Iterations[i].Index = i
	}
	got := renderIndex(req, fixedRunID)
	s := string(got)
	if !strings.Contains(s, "_truncated_") {
		t.Errorf("expected _truncated_ marker row:\n%s", s)
	}
	if !strings.Contains(s, "runner cap reached at row 1000 (of 1500)") {
		t.Errorf("expected truncation explanation:\n%s", s)
	}
}

// TestMarkdown_DataDriven_PerIteration verifies that WriteReport creates the
// subdirectory and all per-iteration files plus index.md.
func TestMarkdown_DataDriven_PerIteration(t *testing.T) {
	report := makeDataDrivenReport(5, 5)
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	// Subdir created.
	info, err := os.Stat(filepath.Join(dir, "seed-users"))
	if err != nil || !info.IsDir() {
		t.Fatalf("expected directory seed-users/: %v", err)
	}
	// 5 iter-*.md files exist.
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(dir, "seed-users", fmt.Sprintf("iter-%d.md", i))); err != nil {
			t.Errorf("expected iter-%d.md: %v", i, err)
		}
	}
	// index.md exists.
	if _, err := os.Stat(filepath.Join(dir, "seed-users", "index.md")); err != nil {
		t.Errorf("expected index.md: %v", err)
	}
}

// TestMarkdown_DataDriven_IndexSummary verifies that index.md shows correct
// aggregate counts matching the iteration outcomes.
func TestMarkdown_DataDriven_IndexSummary(t *testing.T) {
	report := makeDataDrivenReport(5, 5)
	report.Requests[0].Iterations[1].Err = fmt.Errorf("boom")
	report.Requests[0].Iterations[1].Assertions = &assertion.Results{Passed: false}
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	got, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
	s := string(got)
	if !strings.Contains(s, "5 iterations (4 passed, 1 failed)") {
		t.Errorf("expected count line:\n%s", s)
	}
	// Table has 5 rows + header + separator.
	if strings.Count(s, "[iter-") != 5 {
		t.Errorf("expected 5 iter links, got %d:\n%s", strings.Count(s, "[iter-"), s)
	}
}

// TestMarkdown_DataDriven_CorrelationIDs verifies that each iter file has
// request_id=req-N-iter-M and a stable slug; index.md has request_id=req-N-index.
func TestMarkdown_DataDriven_CorrelationIDs(t *testing.T) {
	report := makeDataDrivenReport(3, 3)
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	for i := 0; i < 3; i++ {
		got, _ := os.ReadFile(filepath.Join(dir, "seed-users", fmt.Sprintf("iter-%d.md", i)))
		want := fmt.Sprintf("id=req-12-iter-%d slug=seed-users", i)
		if !strings.Contains(string(got), want) {
			t.Errorf("iter-%d.md missing %q:\n%s", i, want, got)
		}
	}
	indexGot, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
	if !strings.Contains(string(indexGot), "id=req-12-index slug=seed-users") {
		t.Errorf("index.md missing id=req-12-index slug=seed-users:\n%s", indexGot)
	}
}

// TestMarkdown_DataDriven_Splice verifies that agent edits outside sentinels in
// iter files survive across re-runs.
func TestMarkdown_DataDriven_Splice(t *testing.T) {
	report := makeDataDrivenReport(5, 5)
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})

	// Append agent edit below END sentinel of iter-3.md.
	iterPath := filepath.Join(dir, "seed-users", "iter-3.md")
	iter3, _ := os.ReadFile(iterPath)
	if err := os.WriteFile(iterPath, append(iter3, []byte("\nAGENT ANALYSIS of iter-3\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-run.
	_ = WriteReport(report, dir, WriteOptions{})
	after, _ := os.ReadFile(iterPath)
	if !strings.Contains(string(after), "AGENT ANALYSIS of iter-3") {
		t.Errorf("agent edit lost on re-run:\n%s", after)
	}
}

// TestMarkdown_DataDriven_IterCap verifies that when IterationTotal > len(Iterations),
// index.md renders a truncation marker; iter files beyond the cap are not written.
func TestMarkdown_DataDriven_IterCap(t *testing.T) {
	// Source had 1500 rows; runner truncated to 1000 (executed iters).
	report := makeDataDrivenReport(1000, 1500)
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	indexGot, _ := os.ReadFile(filepath.Join(dir, "seed-users", "index.md"))
	if !strings.Contains(string(indexGot), "_truncated_") {
		t.Errorf("expected truncation marker in index.md")
	}
	// iter-1000.md and beyond do not exist.
	if _, err := os.Stat(filepath.Join(dir, "seed-users", "iter-1000.md")); !os.IsNotExist(err) {
		t.Errorf("iter-1000.md should not exist beyond cap")
	}
	// iter-999.md exists (the last under-cap iteration).
	if _, err := os.Stat(filepath.Join(dir, "seed-users", "iter-999.md")); err != nil {
		t.Errorf("iter-999.md should exist: %v", err)
	}
}

// TestMarkdown_DataDriven_FailedIteration verifies that a failed iteration renders
// the full 11-section structure with [ ] markers and remains splice-safe.
func TestMarkdown_DataDriven_FailedIteration(t *testing.T) {
	report := makeDataDrivenReport(2, 2)
	report.Requests[0].Iterations[0].Assertions = &assertion.Results{
		Passed: false,
		Items: []assertion.Result{
			{Type: "status", Expected: "201", Actual: "500", Passed: false},
		},
	}
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	got, _ := os.ReadFile(filepath.Join(dir, "seed-users", "iter-0.md"))
	s := string(got)
	if !strings.Contains(s, "[ ] status") {
		t.Errorf("expected [ ] marker for failed assertion:\n%s", s)
	}
	// 11-section structure preserved.
	for _, marker := range []string{
		"## Notes",
		"## Response (deterministic)",
		"### Response metadata",
		"### Timing",
		"### Assertions",
		"<!-- END curlew:response",
		"## Analysis",
	} {
		if !strings.Contains(s, marker) {
			t.Errorf("missing %q in failed iter file:\n%s", marker, s)
		}
	}
}

// TestMarkdown_RunMD_DataDrivenEntry verifies that run.md entry for a data-driven
// request shows a single aggregate bullet linking to <slug>/index.md.
func TestMarkdown_RunMD_DataDrivenEntry(t *testing.T) {
	report := makeDataDrivenReport(5, 5)
	// Make one failure so the counts are interesting.
	report.Requests[0].Iterations[2].Err = fmt.Errorf("timeout")
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "run.md"))
	s := string(got)

	if !strings.Contains(s, "[Seed users](seed-users/index.md)") {
		t.Errorf("run.md missing aggregate link to index.md:\n%s", s)
	}
	if !strings.Contains(s, "5 iterations") {
		t.Errorf("run.md missing iteration count:\n%s", s)
	}
	if !strings.Contains(s, "4 passed") {
		t.Errorf("run.md missing passed count:\n%s", s)
	}
	if !strings.Contains(s, "1 failed") {
		t.Errorf("run.md missing failed count:\n%s", s)
	}
}

// TestMarkdown_ParallelWaves verifies that wave_index values appear correctly
// in per-request .md files (pinning existing behaviour alongside new wave-grouping).
func TestMarkdown_ParallelWaves(t *testing.T) {
	report := &Report{
		CollectionName: "Parallel",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 3, Passed: 3},
		IsParallel:     true,
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "a", Name: "A", StatusCode: 200, WaveIndex: 0, StartedAt: fixedTime},
			{RequestID: "req-2", Slug: "b", Name: "B", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
			{RequestID: "req-3", Slug: "c", Name: "C", StatusCode: 200, WaveIndex: 2, StartedAt: fixedTime},
		},
	}
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	for slug, want := range map[string]string{
		"a": "wave_index: 0",
		"b": "wave_index: 1",
		"c": "wave_index: 2",
	} {
		got, _ := os.ReadFile(filepath.Join(dir, slug+".md"))
		if !strings.Contains(string(got), want) {
			t.Errorf("%s.md missing %q:\n%s", slug, want, got)
		}
	}
}

// TestMarkdown_SequentialWaveIndex verifies that a sequential request renders
// `wave_index: sequential` in its Timing section.
func TestMarkdown_SequentialWaveIndex(t *testing.T) {
	report := makePassReport() // WaveIndex: -1
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !strings.Contains(string(got), "wave_index: sequential") {
		t.Errorf("expected wave_index: sequential:\n%s", got)
	}
}
