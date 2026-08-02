package schedule_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/vault"
	teamtemplate "github.com/weiqigod/curlew/internal/vault/teamtemplate"
	"github.com/weiqigod/curlew/internal/worker/schedule"
)

func TestRunnerExecutor_Execute(t *testing.T) {
	tests := []struct {
		name          string
		yaml          string
		srvHandler    http.HandlerFunc
		envVars       map[string]string
		wantPass      int
		wantFail      int
		wantItemCount int
	}{
		{
			name: "single passing request",
			yaml: `
name: test-collection
requests:
  - name: GET health
    request:
      method: GET
      url: "{{BASE_URL}}/health"
    assertions:
      status: 200
`,
			srvHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			wantPass:      1,
			wantFail:      0,
			wantItemCount: 1,
		},
		{
			name: "single failing assertion",
			yaml: `
name: test-collection
requests:
  - name: POST users
    request:
      method: POST
      url: "{{BASE_URL}}/users"
    assertions:
      status: 201
`,
			srvHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
			},
			wantPass:      0,
			wantFail:      1,
			wantItemCount: 1,
		},
		{
			name: "envVars are exposed as variables",
			yaml: `
name: test-collection
requests:
  - name: GET ping
    request:
      method: GET
      url: "{{BASE_URL}}/ping"
    assertions:
      status: 200
`,
			srvHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(200)
			},
			wantPass:      1,
			wantFail:      0,
			wantItemCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Spin up a test HTTP server.
			srv := httptest.NewServer(tc.srvHandler)
			defer srv.Close()

			// Write YAML to a temp file, substituting the server URL.
			dir := t.TempDir()
			yamlPath := filepath.Join(dir, "api.yaml")
			content := tc.yaml
			if err := os.WriteFile(yamlPath, []byte(content), 0o600); err != nil {
				t.Fatalf("write yaml: %v", err)
			}

			// Build env vars.
			envVars := map[string]string{"BASE_URL": srv.URL}
			for k, v := range tc.envVars {
				envVars[k] = v
			}

			exec := schedule.NewRunnerExecutor()
			outcome, err := exec.Execute(t.Context(), yamlPath, envVars)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}

			if outcome.PassCount != tc.wantPass {
				t.Errorf("PassCount = %d; want %d", outcome.PassCount, tc.wantPass)
			}
			if outcome.FailCount != tc.wantFail {
				t.Errorf("FailCount = %d; want %d", outcome.FailCount, tc.wantFail)
			}
			if len(outcome.Items) != tc.wantItemCount {
				t.Errorf("Items len = %d; want %d", len(outcome.Items), tc.wantItemCount)
			}
			if outcome.DurationMs < 0 {
				t.Errorf("DurationMs = %d; want >= 0", outcome.DurationMs)
			}
		})
	}
}

func TestRunnerExecutor_ResolvesSharedVaultTemplatePerExecution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "api.yaml")
	collection := `name: scheduled-vault
requests:
  - name: secret-backed request
    request:
      method: GET
      url: "{{secrets.base_url}}/health"
    assertions:
      status: 200
`
	if err := os.WriteFile(yamlPath, []byte(collection), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	tpl, err := teamtemplate.Parse([]byte(`team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-north-1
      keys:
        base_url: scheduled/base-url
`))
	if err != nil {
		t.Fatalf("parse team template: %v", err)
	}

	loadCalls := 0
	var commands []string
	exec := schedule.NewRunnerExecutorWithOptions(schedule.RunnerExecutorOptions{
		TeamEnv: "production",
		LoadTeamTemplate: func(context.Context) (*teamtemplate.TeamTemplate, error) {
			loadCalls++
			return tpl, nil
		},
		VaultExecutor: func(_ context.Context, command string) (string, error) {
			commands = append(commands, command)
			return srv.URL, nil
		},
	})

	for i := 0; i < 2; i++ {
		outcome, runErr := exec.Execute(t.Context(), yamlPath, nil)
		if runErr != nil {
			t.Fatalf("Execute #%d: %v", i+1, runErr)
		}
		if outcome.PassCount != 1 || outcome.FailCount != 0 {
			t.Fatalf("Execute #%d outcome = pass %d fail %d; want pass 1 fail 0",
				i+1, outcome.PassCount, outcome.FailCount)
		}
	}

	if loadCalls != 2 {
		t.Errorf("template load calls = %d; want 2 (one per schedule claim)", loadCalls)
	}
	if len(commands) != 2 {
		t.Fatalf("vault command calls = %d; want 2", len(commands))
	}
	if !strings.Contains(commands[0], "--region 'eu-north-1'") {
		t.Errorf("vault command = %q; want configured AWS region", commands[0])
	}
}

func TestRunnerExecutor_AutoSelectsOnlyVaultEnvironment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "api.yaml")
	if err := os.WriteFile(yamlPath, []byte(`name: scheduled-vault
requests:
  - name: secret-backed request
    request:
      method: GET
      url: "{{secrets.base_url}}"
    assertions:
      status: 200
`), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	tpl, err := teamtemplate.Parse([]byte(`team_secrets:
  vault_configs:
    only:
      provider: azure-key-vault
      vault_name: scheduled-vault
      keys:
        base_url: base-url
`))
	if err != nil {
		t.Fatalf("parse team template: %v", err)
	}

	exec := schedule.NewRunnerExecutorWithOptions(schedule.RunnerExecutorOptions{
		LoadTeamTemplate: func(context.Context) (*teamtemplate.TeamTemplate, error) {
			return tpl, nil
		},
		VaultExecutor: func(_ context.Context, _ string) (string, error) {
			return srv.URL, nil
		},
	})
	outcome, runErr := exec.Execute(t.Context(), yamlPath, nil)
	if runErr != nil {
		t.Fatalf("Execute: %v", runErr)
	}
	if outcome.PassCount != 1 {
		t.Errorf("PassCount = %d; want 1", outcome.PassCount)
	}
}

var _ vault.CommandExecutor = func(context.Context, string) (string, error) { return "", nil }
