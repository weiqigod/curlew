package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/appdir"
	"github.com/peterlindqvist/apitest/internal/backend"
	"github.com/peterlindqvist/apitest/internal/backend/device"
)

func TestParseWorkerArgs_SchedulePull(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantSched workerSchedulePullCfg
		wantErr   string
	}{
		{
			name: "--schedule-pull defaults",
			args: []string{"--schedule-pull"},
			wantSched: workerSchedulePullCfg{
				enabled:      true,
				pollInterval: 30 * time.Second,
			},
		},
		{
			name: "backend URL override",
			args: []string{"--schedule-pull", "--backend", "http://localhost:5000"},
			wantSched: workerSchedulePullCfg{
				enabled:      true,
				backendURL:   "http://localhost:5000",
				pollInterval: 30 * time.Second,
			},
		},
		{
			name: "poll-interval override",
			args: []string{"--schedule-pull", "--poll-interval", "5s"},
			wantSched: workerSchedulePullCfg{
				enabled:      true,
				pollInterval: 5 * time.Second,
			},
		},
		{
			name: "--once flag",
			args: []string{"--schedule-pull", "--once"},
			wantSched: workerSchedulePullCfg{
				enabled:      true,
				once:         true,
				pollInterval: 30 * time.Second,
			},
		},
		{
			name: "shared vault environment",
			args: []string{"--schedule-pull", "--env", "production"},
			wantSched: workerSchedulePullCfg{
				enabled:      true,
				teamEnv:      "production",
				pollInterval: 30 * time.Second,
			},
		},
		{
			name:    "--schedule-pull with --job rejects",
			args:    []string{"--schedule-pull", "--job", "job_x"},
			wantErr: "--schedule-pull cannot be combined with --job",
		},
		{
			name:    "--poll-interval requires value",
			args:    []string{"--schedule-pull", "--poll-interval"},
			wantErr: "requires a value",
		},
		{
			name:    "--backend requires value",
			args:    []string{"--schedule-pull", "--backend"},
			wantErr: "requires a value",
		},
		{
			name:    "--poll-interval invalid duration",
			args:    []string{"--schedule-pull", "--poll-interval", "bogus"},
			wantErr: "invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, sched, _, err := parseWorkerArgs(tc.args)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseWorkerArgs() error = nil; want error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q; want containing %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseWorkerArgs() error = %v; want nil", err)
			}
			if sched.enabled != tc.wantSched.enabled {
				t.Errorf("enabled = %v; want %v", sched.enabled, tc.wantSched.enabled)
			}
			if sched.backendURL != tc.wantSched.backendURL {
				t.Errorf("backendURL = %q; want %q", sched.backendURL, tc.wantSched.backendURL)
			}
			if sched.pollInterval != tc.wantSched.pollInterval {
				t.Errorf("pollInterval = %v; want %v", sched.pollInterval, tc.wantSched.pollInterval)
			}
			if sched.once != tc.wantSched.once {
				t.Errorf("once = %v; want %v", sched.once, tc.wantSched.once)
			}
			if sched.teamEnv != tc.wantSched.teamEnv {
				t.Errorf("teamEnv = %q; want %q", sched.teamEnv, tc.wantSched.teamEnv)
			}
		})
	}
}

func TestWorkerHelp_DocumentsSchedulePull(t *testing.T) {
	var buf bytes.Buffer
	printWorkerHelpTo(&buf)
	if !strings.Contains(buf.String(), "--schedule-pull") {
		t.Errorf("help missing --schedule-pull; got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "--env <name>") {
		t.Errorf("help missing schedule vault environment selector; got:\n%s", buf.String())
	}
}

// TestWorkerCmd_SchedulePull_OnceMode runs an integration test of the --once
// mode against a scripted httptest.Server.
func TestWorkerCmd_SchedulePull_OnceMode(t *testing.T) {
	// Write a minimal collection YAML.
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "api.yaml")
	yamlContent := `name: test-collection
requests:
  - name: GET health
    request:
      method: GET
      url: "{{SRV_URL}}/health"
    assertions:
      status: 200
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	// Track which endpoints were called.
	var calledNextRun, calledResult bool

	// Spin up a fake backend.
	mux := http.NewServeMux()
	var srvURL string

	// GET /api/v1/schedules/next-run — return a claim the first time, then 204.
	nextRunCalls := 0
	mux.HandleFunc("/api/v1/schedules/next-run", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-access-token" {
			t.Errorf("Authorization = %q; want stored backend access token", got)
		}
		nextRunCalls++
		if nextRunCalls == 1 {
			calledNextRun = true
			w.Header().Set("Content-Type", "application/json")
			// Return a claim that points to our temp YAML. We include SRV_URL as an env var.
			w.WriteHeader(200)
			body := `{"run_id":"run_test_001","schedule_id":"sched_1","collection_ref":"file:` + yamlPath + `","env_vars":{"SRV_URL":"` + srvURL + `"},"claim_token":"claim-tok-1","deadline":"2026-05-11T10:00:00Z"}`
			_, _ = io.WriteString(w, body)
		} else {
			w.WriteHeader(204)
		}
	})

	// POST /api/v1/schedules/runs/{id}/heartbeat — accept.
	mux.HandleFunc("/api/v1/schedules/runs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/heartbeat") {
			w.WriteHeader(200)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/result") {
			calledResult = true
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	})
	mux.HandleFunc("/api/v1/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"license_jwt":"`+testLoginJWT+`","access_token":"test-access-token","refresh_token":"refresh-new"}`)
	})

	// The health endpoint that the collection hits.
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	backendSrv := httptest.NewServer(mux)
	defer backendSrv.Close()
	srvURL = backendSrv.URL

	// Store the backend access token in the config dir.
	configDir := t.TempDir()
	if err := device.Write(configDir, device.Record{DeviceID: "dev-test", IssuedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("write device: %v", err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: configDir, DeviceID: "dev-test"})
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}
	if err := storage.SetRefreshToken("refresh-old"); err != nil {
		t.Fatalf("save refresh token: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.DeleteAccessToken()
		_ = storage.DeleteRefreshToken()
	})
	t.Setenv(appdir.ConfigEnv, configDir)

	// Run the worker in --once mode.
	var stdout, stderr bytes.Buffer
	exitCode := workerCmdOut([]string{
		"--schedule-pull",
		"--backend", backendSrv.URL,
		"--once",
	}, &stdout, &stderr)

	t.Logf("stdout:\n%s", stdout.String())
	t.Logf("stderr:\n%s", stderr.String())

	if exitCode != 0 {
		t.Errorf("exit code = %d; want 0", exitCode)
	}
	if !calledNextRun {
		t.Error("GET /api/v1/schedules/next-run was not called")
	}
	if !calledResult {
		t.Error("POST /api/v1/schedules/runs/{id}/result was not called")
	}
	out := stdout.String()
	if !strings.Contains(out, "claimed run run_test_001") {
		t.Errorf("stdout missing 'claimed run run_test_001'; got:\n%s", out)
	}
	if !strings.Contains(out, "posted result for run_test_001") {
		t.Errorf("stdout missing 'posted result for run_test_001'; got:\n%s", out)
	}
}

// TestWorkerCmd_SchedulePull_NoToken verifies that missing access token
// produces exit code 10 with a helpful error message.
func TestWorkerCmd_SchedulePull_NoToken(t *testing.T) {
	// Point config dir to an empty temp dir (no stored tokens).
	t.Setenv(appdir.ConfigEnv, t.TempDir())

	var stdout, stderr bytes.Buffer
	exitCode := workerCmdOut([]string{
		"--schedule-pull",
		"--backend", "http://localhost:5000",
		"--once",
	}, &stdout, &stderr)

	if exitCode != 10 {
		t.Errorf("exit code = %d; want 10", exitCode)
	}
	if !strings.Contains(stderr.String(), "not logged in") {
		t.Errorf("stderr missing 'not logged in'; got: %s", stderr.String())
	}
}
