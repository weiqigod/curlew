package prcheck

import (
	"os"
	"strings"
	"testing"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			"valid_config",
			Config{BackendURL: "http://localhost", BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"},
			"",
		},
		{
			"missing_backend_url",
			Config{BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"},
			"backend URL not configured",
		},
		{
			"missing_backend_token",
			Config{BackendURL: "http://x", Org: "acme", PR: 42, Repo: "acme/api", ResultsFile: "f.json"},
			"CURLEW_BACKEND_TOKEN",
		},
		{
			"missing_org",
			Config{BackendURL: "http://x", BackendToken: "tok", PR: 42, Repo: "acme/api", ResultsFile: "f.json"},
			"--org",
		},
		{
			"zero_pr",
			Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 0, Repo: "acme/api", ResultsFile: "f.json"},
			"--pr",
		},
		{
			"missing_repo",
			Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 42, ResultsFile: "f.json"},
			"--repo",
		},
		{
			"missing_results",
			Config{BackendURL: "http://x", BackendToken: "tok", Org: "acme", PR: 42, Repo: "acme/api"},
			"--results",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v; want nil", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Validate() = nil; want error containing %q", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() = %q; want error containing %q", err.Error(), tc.wantErr)
				}
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envURL    string
		envToken  string
		wantURL   string
		wantToken string
	}{
		{"both_set", "http://back", "tok", "http://back", "tok"},
		{"empty_env", "", "", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CURLEW_BACKEND_URL", tc.envURL)
			t.Setenv("CURLEW_BACKEND_TOKEN", tc.envToken)
			cfg := ConfigFromEnv()
			if cfg.BackendURL != tc.wantURL {
				t.Errorf("BackendURL = %q; want %q", cfg.BackendURL, tc.wantURL)
			}
			if cfg.BackendToken != tc.wantToken {
				t.Errorf("BackendToken = %q; want %q", cfg.BackendToken, tc.wantToken)
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
		{"missing_collection_name", noNameJSON, "", "missing collection_name", 0, 0},
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
