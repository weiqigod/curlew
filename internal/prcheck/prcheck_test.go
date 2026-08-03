package prcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"valid_config", Config{ResultsFile: "f.json"}, ""},
		{"valid_with_summary", Config{ResultsFile: "f.json", SummaryFile: "s.json"}, ""},
		{"missing_results", Config{}, "--results"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v; want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil; want error containing %q", tc.wantErr)
			}
			if !contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() = %q; want error containing %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadResultsFile(t *testing.T) {
	validJSON := `{
		"collection_name": "smoke-tests",
		"run_at": "2026-04-16T10:00:00Z",
		"duration_ms": 1234,
		"pass_count": 3,
		"fail_count": 0,
		"skipped_count": 0,
		"triggered_by": "pr-check",
		"items": []
	}`
	failingJSON := `{
		"collection_name": "smoke-tests",
		"run_at": "2026-04-16T10:00:00Z",
		"duration_ms": 500,
		"pass_count": 2,
		"fail_count": 1,
		"skipped_count": 0,
		"triggered_by": "pr-check",
		"items": []
	}`
	noNameJSON := `{
		"run_at": "2026-04-16T10:00:00Z",
		"pass_count": 1,
		"fail_count": 0,
		"items": []
	}`

	tests := []struct {
		name     string
		content  string
		filePath string // empty means use a temp file with content
		wantErr  string
		wantPass int
		wantFail int
	}{
		{"valid_all_pass", validJSON, "", "", 3, 0},
		{"valid_with_failures", failingJSON, "", "", 2, 1},
		{"invalid_json", "{bad", "", "parsing results JSON", 0, 0},
		{"neither_known_shape", noNameJSON, "", "neither a results payload", 0, 0},
		{"file_not_found", "", "/nonexistent/path/file.json", "reading results file", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.filePath
			if path == "" && tc.wantErr != "reading results file" {
				f, err := os.CreateTemp(t.TempDir(), "results*.json")
				if err != nil {
					t.Fatalf("create temp file: %v", err)
				}
				if _, err := f.WriteString(tc.content); err != nil {
					t.Fatalf("write temp file: %v", err)
				}
				if err := f.Close(); err != nil {
					t.Fatalf("close temp file: %v", err)
				}
				path = f.Name()
			}

			payload, err := LoadResultsFile(path)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("LoadResultsFile() = nil error; want error containing %q", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Errorf("LoadResultsFile() error = %q; want containing %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadResultsFile() = %v; want nil", err)
			}
			if payload.PassCount != tc.wantPass {
				t.Errorf("PassCount = %d; want %d", payload.PassCount, tc.wantPass)
			}
			if payload.FailCount != tc.wantFail {
				t.Errorf("FailCount = %d; want %d", payload.FailCount, tc.wantFail)
			}
		})
	}
}

func TestResultsPayload_HasFailures(t *testing.T) {
	tests := []struct {
		name      string
		failCount int
		want      bool
	}{
		{"no_failures", 0, false},
		{"has_failures", 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &ResultsPayload{FailCount: tc.failCount}
			if got := p.HasFailures(); got != tc.want {
				t.Errorf("HasFailures() = %v; want %v", got, tc.want)
			}
		})
	}
}

// contains is a helper for substring matching in error messages.
func contains(s, sub string) bool {
	return sub == "" || strings.Contains(s, sub)
}

// A results file written by `curlew run --format json` must be readable by
// pr-check: it is the only producer of results in a backend-free CLI.
func TestLoadResultsFile_AcceptsRunJSONOutput(t *testing.T) {
	runJSON := `{
  "name": "Sample Collection",
  "status": "failed",
  "duration_ms": 450,
  "summary": {"total": 3, "passed": 1, "failed": 1, "skipped": 1},
  "requests": [
    {"name": "ok", "status": "passed", "duration_ms": 12, "assertions": []},
    {"name": "bad", "status": "failed", "duration_ms": 30, "assertions": []},
    {"name": "gone", "status": "skipped", "duration_ms": 0, "assertions": []}
  ]
}`
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte(runJSON), 0o600); err != nil {
		t.Fatalf("write results: %v", err)
	}

	payload, err := LoadResultsFile(path)
	if err != nil {
		t.Fatalf("LoadResultsFile on run --format json output: %v", err)
	}
	if payload.CollectionName != "Sample Collection" {
		t.Errorf("CollectionName = %q, want %q", payload.CollectionName, "Sample Collection")
	}
	if payload.PassCount != 1 || payload.FailCount != 1 || payload.SkippedCount != 1 {
		t.Errorf("counts = %d/%d/%d, want 1/1/1", payload.PassCount, payload.FailCount, payload.SkippedCount)
	}
	if payload.DurationMs != 450 {
		t.Errorf("DurationMs = %d, want 450", payload.DurationMs)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("Items = %d, want 3", len(payload.Items))
	}
	if !payload.HasFailures() {
		t.Error("HasFailures() = false, want true")
	}
}

// The pre-existing payload shape must keep working.
func TestLoadResultsFile_StillAcceptsPayloadShape(t *testing.T) {
	payloadJSON := `{"collection_name":"suite","pass_count":2,"fail_count":0,"items":[]}`
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte(payloadJSON), 0o600); err != nil {
		t.Fatalf("write results: %v", err)
	}

	payload, err := LoadResultsFile(path)
	if err != nil {
		t.Fatalf("LoadResultsFile on payload shape: %v", err)
	}
	if payload.CollectionName != "suite" || payload.PassCount != 2 {
		t.Errorf("got %+v, want collection_name=suite pass_count=2", payload)
	}
}

// A JSON document that is neither shape must still be rejected.
func TestLoadResultsFile_RejectsUnrecognisedShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.json")
	if err := os.WriteFile(path, []byte(`{"unrelated":true}`), 0o600); err != nil {
		t.Fatalf("write results: %v", err)
	}

	if _, err := LoadResultsFile(path); err == nil {
		t.Fatal("LoadResultsFile on an unrecognised document = nil error; want a shape error")
	}
}
