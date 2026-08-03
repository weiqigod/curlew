package prcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeResultsFile writes a results JSON to a temp file and returns the path.
func makeResultsFile(t *testing.T, passCount, failCount int) string {
	t.Helper()
	payload := ResultsPayload{
		CollectionName: "test-suite",
		RunAt:          "2026-04-16T10:00:00Z",
		DurationMs:     500,
		PassCount:      passCount,
		FailCount:      failCount,
		TriggeredBy:    "pr-check",
		Items:          []ResultItem{},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal results: %v", err)
	}
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write results file: %v", err)
	}
	return path
}

func TestRun_Success(t *testing.T) {
	results := makeResultsFile(t, 5, 0)
	var out bytes.Buffer

	res, err := Run(Config{ResultsFile: results}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.State != "success" {
		t.Errorf("State = %q, want success", res.State)
	}
	if res.Pass != 5 || res.Fail != 0 {
		t.Errorf("Pass/Fail = %d/%d, want 5/0", res.Pass, res.Fail)
	}
}

func TestRun_FailingTests_StateFailure(t *testing.T) {
	results := makeResultsFile(t, 3, 2)
	var out bytes.Buffer

	res, err := Run(Config{ResultsFile: results}, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.State != "failure" {
		t.Errorf("State = %q, want failure", res.State)
	}
	if res.Fail != 2 {
		t.Errorf("Fail = %d, want 2", res.Fail)
	}
}

func TestRun_WritesSummaryFile(t *testing.T) {
	results := makeResultsFile(t, 4, 1)
	summaryPath := filepath.Join(t.TempDir(), "summary.json")
	var out bytes.Buffer

	if _, err := Run(Config{ResultsFile: results, SummaryFile: summaryPath}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(summaryPath) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("read summary file: %v", err)
	}
	var summary CheckSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("summary is not valid JSON: %v", err)
	}
	if summary.State != "failure" {
		t.Errorf("summary state = %q, want failure", summary.State)
	}
	if summary.PassCount != 4 || summary.FailCount != 1 {
		t.Errorf("summary counts = %d/%d, want 4/1", summary.PassCount, summary.FailCount)
	}
	if summary.CollectionName != "test-suite" {
		t.Errorf("summary collection_name = %q, want test-suite", summary.CollectionName)
	}
}

func TestRun_NoSummaryFileWritesNothing(t *testing.T) {
	results := makeResultsFile(t, 1, 0)
	dir := t.TempDir()
	var out bytes.Buffer

	if _, err := Run(Config{ResultsFile: results}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no files written, got %v", entries)
	}
	if out.Len() != 0 {
		t.Errorf("expected no output without --dry-run, got %q", out.String())
	}
}

func TestRun_DryRun_PrintsSummaryInsteadOfWriting(t *testing.T) {
	results := makeResultsFile(t, 2, 0)
	summaryPath := filepath.Join(t.TempDir(), "summary.json")
	var out bytes.Buffer

	if _, err := Run(Config{ResultsFile: results, SummaryFile: summaryPath, DryRun: true}, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.Contains(out.String(), "\"state\"") {
		t.Errorf("dry-run output does not contain the summary: %q", out.String())
	}
	if _, err := os.Stat(summaryPath); !os.IsNotExist(err) {
		t.Error("dry-run must not write the summary file")
	}
}

func TestRun_MissingConfig_Error(t *testing.T) {
	var out bytes.Buffer

	_, err := Run(Config{}, &out)
	if err == nil {
		t.Fatal("Run with empty config = nil error; want ErrResultsFileMissing")
	}
	if !errors.Is(err, ErrResultsFileMissing) {
		t.Errorf("err = %v; want ErrResultsFileMissing", err)
	}
}

func TestRun_BadResultsFile_Error(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write bad file: %v", err)
	}
	var out bytes.Buffer

	if _, err := Run(Config{ResultsFile: path}, &out); err == nil {
		t.Fatal("Run with malformed results = nil error; want a parse error")
	}
}

func TestRun_UnwritableSummary_Error(t *testing.T) {
	results := makeResultsFile(t, 1, 0)
	// Parent is a regular file, so Create must fail.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	var out bytes.Buffer

	if _, err := Run(Config{ResultsFile: results, SummaryFile: filepath.Join(blocker, "s.json")}, &out); err == nil {
		t.Fatal("Run with an unwritable summary path = nil error; want a create error")
	}
}
