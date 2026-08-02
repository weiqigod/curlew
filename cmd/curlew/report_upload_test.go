package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// minimalCollection returns a YAML collection with a single passing request
// that hits a given URL.
func minimalCollection(t *testing.T, serverURL string) string {
	t.Helper()
	content := "name: e2e-test\nrequests:\n  - name: health-check\n    request:\n      method: GET\n      url: \"" + serverURL + "\"\n    assertions:\n      status: 200\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

// failingCollection returns a YAML collection with a failing assertion.
func failingCollection(t *testing.T, serverURL string) string {
	t.Helper()
	content := "name: e2e-failing\nrequests:\n  - name: expect-fail\n    request:\n      method: GET\n      url: \"" + serverURL + "\"\n    assertions:\n      status: 999\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

func TestRunWithReportUpload(t *testing.T) {
	tests := []struct {
		name                     string
		setup                    func(t *testing.T) (collectionPath, backendURL string, cleanup func())
		extraFlags               []string
		wantExit                 int
		wantStdoutContains       []string
		wantStdoutMustNotContain []string
		wantStderrContains       []string
	}{
		{
			name: "upload_then_pr_check_on_pass",
			setup: func(t *testing.T) (string, string, func()) {
				var resultsPosts, prCheckPosts int32
				// apiTS serves the collection's HTTP requests (the API under test).
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				// backendTS serves the upload endpoints.
				backendTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "/pr-checks") {
						atomic.AddInt32(&prCheckPosts, 1)
						w.WriteHeader(200)
						_, _ = w.Write([]byte(`{"status":"ok"}`))
						return
					}
					if strings.Contains(r.URL.Path, "/results") {
						atomic.AddInt32(&resultsPosts, 1)
						w.WriteHeader(202)
						_, _ = w.Write([]byte(`{"result_id":"res_e2e001","status":"accepted"}`))
						return
					}
					w.WriteHeader(404)
				}))
				col := minimalCollection(t, apiTS.URL)
				return col, backendTS.URL, func() {
					apiTS.Close()
					backendTS.Close()
					if atomic.LoadInt32(&resultsPosts) != 1 {
						t.Errorf("expected 1 results POST, got %d", resultsPosts)
					}
					if atomic.LoadInt32(&prCheckPosts) != 1 {
						t.Errorf("expected 1 pr-checks POST, got %d", prCheckPosts)
					}
				}
			},
			extraFlags:         []string{"--report-upload", "--org", "acme", "--pr", "7", "--repo", "acme/api"},
			wantExit:           0,
			wantStdoutContains: []string{"Uploaded result res_e2e001; check-run posted; status=success"},
		},
		{
			name: "upload_only_no_pr",
			setup: func(t *testing.T) (string, string, func()) {
				var resultsPosts, prCheckPosts int32
				// apiTS serves the collection's HTTP requests (the API under test).
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				// backendTS serves the upload endpoints.
				backendTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "/pr-checks") {
						atomic.AddInt32(&prCheckPosts, 1)
						w.WriteHeader(200)
						_, _ = w.Write([]byte(`{"status":"ok"}`))
						return
					}
					if strings.Contains(r.URL.Path, "/results") {
						atomic.AddInt32(&resultsPosts, 1)
						w.WriteHeader(202)
						_, _ = w.Write([]byte(`{"result_id":"res_e2e002","status":"accepted"}`))
						return
					}
					w.WriteHeader(404)
				}))
				col := minimalCollection(t, apiTS.URL)
				return col, backendTS.URL, func() {
					apiTS.Close()
					backendTS.Close()
					if atomic.LoadInt32(&resultsPosts) != 1 {
						t.Errorf("expected 1 results POST, got %d", resultsPosts)
					}
					if atomic.LoadInt32(&prCheckPosts) != 0 {
						t.Errorf("expected 0 pr-checks POST (no --pr), got %d", prCheckPosts)
					}
				}
			},
			extraFlags:         []string{"--report-upload", "--org", "acme"},
			wantExit:           0,
			wantStdoutContains: []string{"Uploaded result res_e2e002"},
		},
		{
			name: "upload_without_pr_repo_omits_check_run_phrase",
			setup: func(t *testing.T) (string, string, func()) {
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				backendTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "/results") {
						w.WriteHeader(202)
						_, _ = w.Write([]byte(`{"result_id":"res_noPR001","status":"accepted"}`))
						return
					}
					w.WriteHeader(404)
				}))
				col := minimalCollection(t, apiTS.URL)
				return col, backendTS.URL, func() {
					apiTS.Close()
					backendTS.Close()
				}
			},
			extraFlags:               []string{"--report-upload", "--org", "acme"},
			wantExit:                 0,
			wantStdoutContains:       []string{"Uploaded result res_"},
			wantStdoutMustNotContain: []string{"check-run posted"},
		},
		{
			name: "failing_run_still_uploads_state_failure",
			setup: func(t *testing.T) (string, string, func()) {
				var prCheckBody []byte
				// apiTS serves the collection's HTTP requests (returns 200, but
				// the collection asserts status: 999, so the run fails).
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				// backendTS serves the upload endpoints.
				backendTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, "/results") {
						w.WriteHeader(202)
						_, _ = w.Write([]byte(`{"result_id":"res_fail001","status":"accepted"}`))
						return
					}
					if strings.Contains(r.URL.Path, "/pr-checks") {
						var buf [4096]byte
						n, _ := r.Body.Read(buf[:])
						prCheckBody = make([]byte, n)
						copy(prCheckBody, buf[:n])
						w.WriteHeader(200)
						_, _ = w.Write([]byte(`{"status":"ok"}`))
						return
					}
					w.WriteHeader(404)
				}))
				col := failingCollection(t, apiTS.URL)
				return col, backendTS.URL, func() {
					apiTS.Close()
					backendTS.Close()
					var body map[string]interface{}
					if len(prCheckBody) > 0 {
						if err := json.Unmarshal(prCheckBody, &body); err == nil {
							if state, ok := body["state"].(string); !ok || state != "failure" {
								t.Errorf("pr-check state = %v; want failure", body["state"])
							}
						}
					}
				}
			},
			extraFlags:         []string{"--report-upload", "--org", "acme", "--pr", "8", "--repo", "acme/api"},
			wantExit:           1, // failing assertions = exit 1
			wantStdoutContains: []string{"check-run posted; status=failure"},
		},
		{
			name: "backend_unreachable_on_passing_run_returns_exit_2",
			setup: func(t *testing.T) (string, string, func()) {
				// Start a backend server and immediately close it to simulate unreachable.
				backendTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				// apiTS serves the collection's HTTP requests (returns 200).
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				col := minimalCollection(t, apiTS.URL)
				addr := backendTS.URL
				backendTS.Close() // backend not reachable
				return col, addr, func() {
					apiTS.Close()
				}
			},
			extraFlags:         []string{"--report-upload", "--org", "acme"},
			wantExit:           2,
			wantStderrContains: []string{"backend unreachable"},
		},
		{
			name: "missing_CURLEW_BACKEND_URL_returns_exit_2",
			setup: func(t *testing.T) (string, string, func()) {
				apiTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(200)
				}))
				col := minimalCollection(t, apiTS.URL)
				return col, "", func() { apiTS.Close() } // empty URL = not set
			},
			extraFlags:         []string{"--report-upload", "--org", "acme"},
			wantExit:           2,
			wantStderrContains: []string{"backend URL not configured"},
		},
		{
			name: "missing_org_returns_exit_1_at_parse_time",
			setup: func(t *testing.T) (string, string, func()) {
				col := minimalCollection(t, "http://localhost/health")
				return col, "http://localhost", func() {}
			},
			extraFlags:         []string{"--report-upload"},
			wantExit:           1,
			wantStderrContains: []string{"--org is required"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			colPath, backendURL, cleanup := tc.setup(t)
			defer cleanup()

			if backendURL != "" {
				t.Setenv("CURLEW_BACKEND_URL", backendURL)
				t.Setenv("CURLEW_BACKEND_TOKEN", "test-token")
			} else {
				t.Setenv("CURLEW_BACKEND_URL", "")
				t.Setenv("CURLEW_BACKEND_TOKEN", "")
			}

			args := append([]string{colPath}, tc.extraFlags...)
			stdout, stderr, code := captureRunCmd(t, args...)

			if code != tc.wantExit {
				t.Errorf("exit code = %d; want %d\nstdout: %s\nstderr: %s",
					code, tc.wantExit, stdout, stderr)
			}
			for _, want := range tc.wantStdoutContains {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout missing %q\nstdout: %s\nstderr: %s", want, stdout, stderr)
				}
			}
			for _, want := range tc.wantStdoutMustNotContain {
				if strings.Contains(stdout, want) {
					t.Errorf("stdout must NOT contain %q\nstdout: %s\nstderr: %s", want, stdout, stderr)
				}
			}
			for _, want := range tc.wantStderrContains {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr missing %q\nstdout: %s\nstderr: %s", want, stdout, stderr)
				}
			}
		})
	}
}

func TestParseRunArgs_ReportUploadFlags(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		wantReportUpload bool
		wantOrg          string
		wantPR           int
		wantRepo         string
		wantTriggeredBy  string
		wantGitSha       string
		wantErr          bool
		wantErrContains  string
	}{
		{
			name:             "report_upload_all_flags",
			args:             []string{"col.yaml", "--report-upload", "--org", "acme", "--pr", "7", "--repo", "acme/api"},
			wantReportUpload: true,
			wantOrg:          "acme",
			wantPR:           7,
			wantRepo:         "acme/api",
		},
		{
			name:             "report_upload_with_triggered_by",
			args:             []string{"col.yaml", "--report-upload", "--org", "acme", "--triggered-by", "ci"},
			wantReportUpload: true,
			wantOrg:          "acme",
			wantTriggeredBy:  "ci",
		},
		{
			name:             "report_upload_with_git_sha",
			args:             []string{"col.yaml", "--report-upload", "--org", "acme", "--git-sha", "abc1234"},
			wantReportUpload: true,
			wantOrg:          "acme",
			wantGitSha:       "abc1234",
		},
		{
			name:            "report_upload_missing_org",
			args:            []string{"col.yaml", "--report-upload"},
			wantErr:         true,
			wantErrContains: "--org is required",
		},
		{
			name:    "no_report_upload_flag",
			args:    []string{"col.yaml"},
			wantErr: false,
		},
		{
			name:            "pr_without_repo",
			args:            []string{"col.yaml", "--report-upload", "--org", "acme", "--pr", "7"},
			wantErr:         true,
			wantErrContains: "--pr and --repo must be set together",
		},
		{
			name:            "repo_without_pr",
			args:            []string{"col.yaml", "--report-upload", "--org", "acme", "--repo", "acme/api"},
			wantErr:         true,
			wantErrContains: "--pr and --repo must be set together",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flags, err := parseRunArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseRunArgs() = nil error; want error containing %q", tc.wantErrContains)
				}
				if tc.wantErrContains != "" && !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Errorf("error = %q; want containing %q", err.Error(), tc.wantErrContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRunArgs() = %v; want nil", err)
			}
			if flags.reportUpload != tc.wantReportUpload {
				t.Errorf("reportUpload = %v; want %v", flags.reportUpload, tc.wantReportUpload)
			}
			if tc.wantOrg != "" && flags.org != tc.wantOrg {
				t.Errorf("org = %q; want %q", flags.org, tc.wantOrg)
			}
			if tc.wantPR != 0 && flags.pr != tc.wantPR {
				t.Errorf("pr = %d; want %d", flags.pr, tc.wantPR)
			}
			if tc.wantRepo != "" && flags.repo != tc.wantRepo {
				t.Errorf("repo = %q; want %q", flags.repo, tc.wantRepo)
			}
			if tc.wantTriggeredBy != "" && flags.triggeredBy != tc.wantTriggeredBy {
				t.Errorf("triggeredBy = %q; want %q", flags.triggeredBy, tc.wantTriggeredBy)
			}
			if tc.wantGitSha != "" && flags.gitSha != tc.wantGitSha {
				t.Errorf("gitSha = %q; want %q", flags.gitSha, tc.wantGitSha)
			}
		})
	}
}
