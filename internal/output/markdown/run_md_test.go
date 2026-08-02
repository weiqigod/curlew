package markdown

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/assertion"
)

// TestMarkdown_RunMD_WaveGrouping verifies that when IsParallel is true,
// run.md groups entries under ## Wave N headers in ascending order.
func TestMarkdown_RunMD_WaveGrouping(t *testing.T) {
	report := &Report{
		CollectionName: "Parallel Run",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 4, Passed: 4},
		IsParallel:     true,
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "login", Name: "Login", StatusCode: 200, WaveIndex: 0, StartedAt: fixedTime},
			{RequestID: "req-2", Slug: "profile", Name: "Profile", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
			{RequestID: "req-3", Slug: "settings", Name: "Settings", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
			{RequestID: "req-4", Slug: "logout", Name: "Logout", StatusCode: 200, WaveIndex: 2, StartedAt: fixedTime},
		},
	}
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "run.md"))

	// Three wave headers in ascending order.
	waveCount := strings.Count(string(got), "\n## Wave ")
	if waveCount != 3 {
		t.Fatalf("expected 3 ## Wave headers, got %d:\n%s", waveCount, got)
	}
	// Verify ascending order: Wave 0 before Wave 1 before Wave 2.
	pos0 := strings.Index(string(got), "## Wave 0")
	pos1 := strings.Index(string(got), "## Wave 1")
	pos2 := strings.Index(string(got), "## Wave 2")
	if pos0 < 0 || pos1 <= pos0 || pos2 <= pos1 {
		t.Fatalf("waves not in ascending order: 0@%d 1@%d 2@%d\n%s", pos0, pos1, pos2, got)
	}
	// Wave 1 contains both profile and settings.
	wave1Block := string(got)[pos1:pos2]
	if !strings.Contains(wave1Block, "profile.md") || !strings.Contains(wave1Block, "settings.md") {
		t.Errorf("Wave 1 missing profile or settings:\n%s", wave1Block)
	}
}

// TestMarkdown_RunMD_WaveGrouping_MixedSequential verifies the ## Sequential
// fallback section inside renderRunMDByWave: when IsParallel is true but some
// entries have WaveIndex == -1 (e.g. data-driven aggregates interleaved in a
// parallel run), those entries appear under ## Sequential after the wave groups.
func TestMarkdown_RunMD_WaveGrouping_MixedSequential(t *testing.T) {
	report := &Report{
		CollectionName: "Mixed Parallel",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 3, Passed: 3},
		IsParallel:     true,
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "login", Name: "Login", StatusCode: 200, WaveIndex: 0, StartedAt: fixedTime},
			{RequestID: "req-2", Slug: "profile", Name: "Profile", StatusCode: 200, WaveIndex: 1, StartedAt: fixedTime},
			// This entry has WaveIndex == -1 (e.g. a data-driven aggregate collapsed entry).
			// Two passing iterations (no Err, no failed Assertions, not Skipped → "pass").
			{
				RequestID:  "req-3",
				Slug:       "seed-users",
				Name:       "Seed Users",
				StatusCode: 0,
				WaveIndex:  -1,
				StartedAt:  fixedTime,
				Iterations: []IterationEntry{
					{Index: 0},
					{Index: 1},
				},
			},
		},
	}
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "run.md"))
	content := string(got)

	// Wave headers for waved entries.
	if !strings.Contains(content, "## Wave 0") {
		t.Errorf("expected ## Wave 0 in run.md:\n%s", content)
	}
	if !strings.Contains(content, "## Wave 1") {
		t.Errorf("expected ## Wave 1 in run.md:\n%s", content)
	}

	// ## Sequential section for the WaveIndex==-1 entry.
	if !strings.Contains(content, "## Sequential") {
		t.Errorf("expected ## Sequential section for WaveIndex==-1 entry:\n%s", content)
	}

	// seed-users appears under ## Sequential (after all wave sections).
	posWave1 := strings.Index(content, "## Wave 1")
	posSeq := strings.Index(content, "## Sequential")
	posSeed := strings.Index(content, "seed-users")
	if posSeq <= posWave1 {
		t.Errorf("## Sequential must appear after ## Wave 1: Wave1@%d Seq@%d", posWave1, posSeq)
	}
	if posSeed <= posSeq {
		t.Errorf("seed-users link must appear after ## Sequential: Seq@%d seed@%d", posSeq, posSeed)
	}
}

// TestMarkdown_RunMD_Sequential verifies that when IsParallel == false,
// the existing flat layout with ## Requests is preserved.
func TestMarkdown_RunMD_Sequential(t *testing.T) {
	report := &Report{
		CollectionName: "Sequential",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 2, Passed: 2},
		IsParallel:     false,
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "login", Name: "Login", WaveIndex: -1, StartedAt: fixedTime},
			{RequestID: "req-2", Slug: "logout", Name: "Logout", WaveIndex: -1, StartedAt: fixedTime},
		},
	}
	dir := t.TempDir()
	_ = WriteReport(report, dir, WriteOptions{})
	got, _ := os.ReadFile(filepath.Join(dir, "run.md"))
	if strings.Contains(string(got), "## Wave ") {
		t.Errorf("sequential collection must not have ## Wave headers:\n%s", got)
	}
	if !strings.Contains(string(got), "## Requests") {
		t.Errorf("sequential collection must have ## Requests:\n%s", got)
	}
}

// TestMarkdown_RunMD_WithEnvName verifies that when EnvName is set, the
// run.md sentinel block contains an "environment: <name>" line.
// This covers the gated branch at run_md.go:54 that was previously untested.
func TestMarkdown_RunMD_WithEnvName(t *testing.T) {
	report := &Report{
		CollectionName: "Sample API",
		EnvName:        "staging",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 1, Passed: 1},
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "get-user", Name: "Get user", StatusCode: 200, WaveIndex: -1, StartedAt: fixedTime},
		},
	}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "run.md"))
	if err != nil {
		t.Fatalf("ReadFile run.md: %v", err)
	}
	content := string(got)

	if !strings.Contains(content, "environment: staging") {
		t.Errorf("expected 'environment: staging' in run.md, got:\n%s", content)
	}
}

// TestMarkdown_RunMD_SkippedEntry verifies that renderRunMDEntry emits
// "- skip: [name](slug.md)" for a skipped non-data-driven request, and that
// the summary table shows Skipped == 1. This covers the Skipped branch at
// run_md.go lines 133–136 and satisfies DoD item 6 for the markdown formatter.
func TestMarkdown_RunMD_SkippedEntry(t *testing.T) {
	report := &Report{
		CollectionName: "Conditional API",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 3, Passed: 1, Failed: 0, Skipped: 2},
		IsParallel:     false,
		Requests: []RequestEntry{
			{
				RequestID:  "req-1",
				Slug:       "seed",
				Name:       "seed",
				StatusCode: 200,
				WaveIndex:  -1,
				StartedAt:  fixedTime,
			},
			{
				RequestID:  "req-2",
				Slug:       "confirm-pending-order",
				Name:       "confirm-pending-order",
				Skipped:    true,
				SkipReason: "if: false",
				WaveIndex:  -1,
				StartedAt:  fixedTime,
			},
			{
				RequestID:  "req-3",
				Slug:       "notify",
				Name:       "notify",
				Skipped:    true,
				SkipReason: "parent skipped: confirm-pending-order",
				WaveIndex:  -1,
				StartedAt:  fixedTime,
			},
		},
	}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "run.md"))
	if err != nil {
		t.Fatalf("ReadFile run.md: %v", err)
	}
	content := string(got)

	// Summary table must show 2 skipped.
	if !strings.Contains(content, "| Skipped | 2 |") {
		t.Errorf("expected '| Skipped | 2 |' in run.md:\n%s", content)
	}

	// Both skipped requests must appear as "- skip: [name](slug.md)".
	if !strings.Contains(content, "- skip: [confirm-pending-order](confirm-pending-order.md)") {
		t.Errorf("expected skip line for confirm-pending-order in run.md:\n%s", content)
	}
	if !strings.Contains(content, "- skip: [notify](notify.md)") {
		t.Errorf("expected skip line for notify in run.md:\n%s", content)
	}

	// The non-skipped seed request must appear as "- pass: [seed](seed.md)".
	if !strings.Contains(content, "- pass: [seed](seed.md)") {
		t.Errorf("expected pass line for seed in run.md:\n%s", content)
	}

	// No skip entry must be labelled pass: or fail:.
	if strings.Contains(content, "- pass: [confirm-pending-order]") {
		t.Errorf("confirm-pending-order must not appear as pass in run.md:\n%s", content)
	}
	if strings.Contains(content, "- fail: [confirm-pending-order]") {
		t.Errorf("confirm-pending-order must not appear as fail in run.md:\n%s", content)
	}
}

// TestMarkdown_RunMD verifies that run.md is created with links to every
// per-request file in execution order.
func TestMarkdown_RunMD(t *testing.T) {
	report := &Report{
		CollectionName: "Sample API",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 3, Passed: 2, Failed: 1},
		Requests: []RequestEntry{
			{RequestID: "req-1", Slug: "get-user", Name: "Get user", StatusCode: 200, WaveIndex: -1, StartedAt: fixedTime},
			{RequestID: "req-2", Slug: "list-posts", Name: "List posts", StatusCode: 200, WaveIndex: -1, StartedAt: fixedTime},
			{
				RequestID: "req-3", Slug: "delete-user", Name: "Delete user", StatusCode: 404,
				Assertions: &assertion.Results{Passed: false, Items: []assertion.Result{
					{Type: "status", Expected: "200", Actual: "404", Passed: false},
				}},
				WaveIndex: -1, StartedAt: fixedTime,
			},
		},
	}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "run.md"))
	if err != nil {
		t.Fatalf("ReadFile run.md: %v", err)
	}
	content := string(got)

	// run.md must have links to all 3 per-request files.
	linkRE := regexp.MustCompile(`\.md\)`)
	matches := linkRE.FindAllString(content, -1)
	if len(matches) != 3 {
		t.Errorf("expected 3 .md) links in run.md, got %d:\n%s", len(matches), content)
	}

	// Links appear in execution order.
	slugOrder := []string{"get-user", "list-posts", "delete-user"}
	lastIdx := -1
	for _, slug := range slugOrder {
		idx := strings.Index(content, slug+".md)")
		if idx < 0 {
			t.Errorf("link to %s.md not found in run.md:\n%s", slug, content)
			continue
		}
		if idx <= lastIdx {
			t.Errorf("link to %s.md appears out of order (at %d, previous was %d)", slug, idx, lastIdx)
		}
		lastIdx = idx
	}

	// run.md has sentinel pair.
	loc, err := parseSentinels(got)
	if err != nil {
		t.Fatalf("parseSentinels for run.md: %v", err)
	}
	if loc == nil {
		t.Error("run.md must have a sentinel pair")
	}

	compareOrUpdateGolden(t, "run_md.md", got)
}
