package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendJSONL(t *testing.T) {
	tests := []struct {
		name       string
		entries    []*JSONLEntry
		wantLines  int
		checkEntry func(t *testing.T, decoded JSONLEntry)
	}{
		{
			name: "creates file if not exists",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 42},
			},
			wantLines: 1,
		},
		{
			name: "appends to existing file",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://a.com", StatusCode: 200, DurationMs: 10},
				{Timestamp: "2026-01-01T00:00:01Z", Method: "POST", URL: "http://b.com", StatusCode: 201, DurationMs: 20},
			},
			wantLines: 2,
		},
		{
			name: "entry is valid JSON",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 50},
			},
			wantLines: 1,
		},
		{
			name: "entry has newline terminator",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 50},
			},
			wantLines: 1,
		},
		{
			name: "multiple entries are separate lines",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://a.com", StatusCode: 200, DurationMs: 10},
				{Timestamp: "2026-01-01T00:00:01Z", Method: "POST", URL: "http://b.com", StatusCode: 201, DurationMs: 20},
				{Timestamp: "2026-01-01T00:00:02Z", Method: "PUT", URL: "http://c.com", StatusCode: 204, DurationMs: 30},
			},
			wantLines: 3,
		},
		{
			name: "error fields omitted when empty",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 50},
			},
			wantLines: 1,
			checkEntry: func(t *testing.T, decoded JSONLEntry) {
				t.Helper()
				if decoded.Error != "" {
					t.Errorf("error should be empty, got %q", decoded.Error)
				}
			},
		},
		{
			name: "dry run omits status_code and duration_ms",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", DryRun: true},
			},
			wantLines: 1,
			checkEntry: func(t *testing.T, decoded JSONLEntry) {
				t.Helper()
				if decoded.StatusCode != 0 {
					t.Errorf("status_code should be 0, got %d", decoded.StatusCode)
				}
				if decoded.DurationMs != 0 {
					t.Errorf("duration_ms should be 0, got %d", decoded.DurationMs)
				}
			},
		},
		{
			name: "dry_run field omitted when false",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 50, DryRun: false},
			},
			wantLines: 1,
			checkEntry: func(t *testing.T, decoded JSONLEntry) {
				t.Helper()
				if decoded.DryRun {
					t.Error("dry_run should be false")
				}
			},
		},
		{
			name: "run_id and request_id are written when populated",
			entries: []*JSONLEntry{
				{
					Timestamp:  "2026-01-01T00:00:00Z",
					Method:     "GET",
					URL:        "http://example.com",
					StatusCode: 200,
					DurationMs: 50,
					RunID:      "0123456789abcdef0123456789abcdef",
					RequestID:  "req-1",
				},
			},
			wantLines: 1,
			checkEntry: func(t *testing.T, decoded JSONLEntry) {
				t.Helper()
				if decoded.RunID != "0123456789abcdef0123456789abcdef" {
					t.Errorf("run_id = %q, want %q", decoded.RunID, "0123456789abcdef0123456789abcdef")
				}
				if decoded.RequestID != "req-1" {
					t.Errorf("request_id = %q, want %q", decoded.RequestID, "req-1")
				}
			},
		},
		{
			name: "run_id and request_id omitted when empty",
			entries: []*JSONLEntry{
				{Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com", StatusCode: 200, DurationMs: 50},
			},
			wantLines: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "test.jsonl")

			for _, entry := range tt.entries {
				if err := AppendJSONL(path, entry); err != nil {
					t.Fatalf("AppendJSONL() error = %v", err)
				}
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			content := string(data)
			// Verify newline terminator
			if !strings.HasSuffix(content, "\n") {
				t.Error("file should end with newline")
			}

			lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
			if len(lines) != tt.wantLines {
				t.Errorf("got %d lines, want %d", len(lines), tt.wantLines)
			}

			// Every line must be valid JSON
			for i, line := range lines {
				var decoded JSONLEntry
				if err := json.Unmarshal([]byte(line), &decoded); err != nil {
					t.Errorf("line %d is not valid JSON: %v\nline: %s", i, err, line)
				}
			}

			// Check omitempty on JSON raw bytes for first entry
			if tt.name == "error fields omitted when empty" {
				if strings.Contains(lines[0], `"error"`) {
					t.Error("JSON should not contain 'error' key when empty")
				}
			}
			if tt.name == "dry run omits status_code and duration_ms" {
				if strings.Contains(lines[0], `"status_code"`) {
					t.Error("JSON should not contain 'status_code' key for dry-run entry")
				}
				if strings.Contains(lines[0], `"duration_ms"`) {
					t.Error("JSON should not contain 'duration_ms' key for dry-run entry")
				}
			}
			if tt.name == "dry_run field omitted when false" {
				if strings.Contains(lines[0], `"dry_run"`) {
					t.Error("JSON should not contain 'dry_run' key when false")
				}
			}
			if tt.name == "run_id and request_id are written when populated" {
				if !strings.Contains(lines[0], `"run_id":"0123456789abcdef0123456789abcdef"`) {
					t.Errorf("JSON should contain run_id key/value, got: %s", lines[0])
				}
				if !strings.Contains(lines[0], `"request_id":"req-1"`) {
					t.Errorf("JSON should contain request_id key/value, got: %s", lines[0])
				}
			}
			if tt.name == "run_id and request_id omitted when empty" {
				if strings.Contains(lines[0], `"run_id"`) {
					t.Error("JSON should not contain 'run_id' key when empty")
				}
				if strings.Contains(lines[0], `"request_id"`) {
					t.Error("JSON should not contain 'request_id' key when empty")
				}
			}

			// Run custom check if provided
			if tt.checkEntry != nil && len(lines) > 0 {
				var decoded JSONLEntry
				if err := json.Unmarshal([]byte(lines[0]), &decoded); err == nil {
					tt.checkEntry(t, decoded)
				}
			}
		})
	}
}

func TestAppendJSONL_errorOnInvalidPath(t *testing.T) {
	err := AppendJSONL("/nonexistent/dir/file.jsonl", &JSONLEntry{
		Timestamp: "2026-01-01T00:00:00Z", Method: "GET", URL: "http://example.com",
	})
	if err == nil {
		t.Fatal("expected error for invalid path, got nil")
	}
	if !strings.Contains(err.Error(), "opening log file") {
		t.Errorf("error should mention 'opening log file', got: %v", err)
	}
}
