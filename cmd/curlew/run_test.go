package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
)

func TestRun_version(t *testing.T) {
	code := run([]string{"--version"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRun_help(t *testing.T) {
	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			code := run([]string{flag})
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
		})
	}
}

func TestRun_no_args(t *testing.T) {
	code := run([]string{})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRun_unknown_command(t *testing.T) {
	code := run([]string{"bogus"})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunCmd_no_args(t *testing.T) {
	code := runCmd([]string{})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunCmd_missing_file(t *testing.T) {
	code := runCmd([]string{"nonexistent.yaml"})
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRunCmd_invalid_yaml(t *testing.T) {
	code := runCmd([]string{"testdata/invalid.yaml"})
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRunCmd_no_collection_name(t *testing.T) {
	code := runCmd([]string{"testdata/no_name.yaml"})
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRunCmd_successful_request(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_assertion_pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: Check\n    request:\n      method: GET\n      url: \"%s\"\n    assertions:\n      status: 200\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_assertion_fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: Check\n    request:\n      method: GET\n      url: \"%s\"\n    assertions:\n      status: 201\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunCmd_body_assertion_pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
      body:
        $.data.id:
          equals: 1
`, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_body_assertion_fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"id":2}}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      body:
        $.data.id:
          equals: 1
`, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunCmd_body_assertion_exists(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"token":"abc123"}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      body:
        $.token:
          exists: true
`, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_body_assertion_non_json(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      body:
        $.id:
          equals: 1
`, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestRunCmd_network_error(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := "name: Fail\nrequests:\n  - name: Bad\n    request:\n      method: GET\n      url: \"http://127.0.0.1:1/fail\"\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f})
	if code != 4 {
		t.Errorf("exit code = %d, want 4", code)
	}
}

func TestRunCmd_parallel_independent_requests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Parallel Test
requests:
  - name: A
    request:
      method: GET
      url: "%s/a"
  - name: B
    request:
      method: GET
      url: "%s/b"
  - name: C
    request:
      method: GET
      url: "%s/c"
`, srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_parallel_dependent_requests(t *testing.T) {
	// Server returns a JSON body with token on the first endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/login" {
			_, _ = w.Write([]byte(`{"token":"abc123"}`))
		} else {
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Parallel Dep Test
requests:
  - name: Login
    request:
      method: POST
      url: "%s/login"
    extract:
      token: "$.token"
  - name: Use Token
    request:
      method: GET
      url: "%s/api?token={{token}}"
    assertions:
      status: 200
`, srv.URL, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// overlapProbe proves concurrent dispatch without wall-clock assumptions
// (a duration bound flakes under CI load). Each enter() call blocks until at
// least `want` calls are in flight simultaneously — or a generous timeout
// expires — then returns. Sequential execution can never overlap, so its
// peak stays at 1 no matter how fast or slow the machine is.
type overlapProbe struct {
	mu       sync.Mutex
	want     int
	inFlight int
	peak     int
	release  chan struct{}
	once     sync.Once
}

func newOverlapProbe(want int) *overlapProbe {
	return &overlapProbe{want: want, release: make(chan struct{})}
}

func (p *overlapProbe) enter() {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.peak {
		p.peak = p.inFlight
	}
	if p.inFlight >= p.want {
		p.once.Do(func() { close(p.release) })
	}
	p.mu.Unlock()
	select {
	case <-p.release:
	case <-time.After(5 * time.Second):
	}
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()
}

func (p *overlapProbe) max() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

func TestRunCmd_parallel_speedup(t *testing.T) {
	probe := newOverlapProbe(2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		probe.enter()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Parallel Speedup
requests:
  - name: Slow A
    request:
      method: GET
      url: "%s/a"
  - name: Slow B
    request:
      method: GET
      url: "%s/b"
  - name: Slow C
    request:
      method: GET
      url: "%s/c"
`, srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel"})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if got := probe.max(); got < 2 {
		t.Errorf("max concurrent in-flight requests = %d, want >= 2 (--parallel must overlap independent requests)", got)
	}
}

func TestRunCmd_parallel_setup_sequential_main_parallel(t *testing.T) {
	var callOrder []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callOrder = append(callOrder, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"test"}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Parallel Phases
setup:
  - name: Setup
    request:
      method: GET
      url: "%s/setup"
    extract:
      token: "$.token"
requests:
  - name: Main A
    request:
      method: GET
      url: "%s/a?token={{token}}"
  - name: Main B
    request:
      method: GET
      url: "%s/b?token={{token}}"
teardown:
  - name: Teardown
    request:
      method: GET
      url: "%s/teardown"
`, srv.URL, srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	// Verify setup ran (should be first)
	mu.Lock()
	order := make([]string, len(callOrder))
	copy(order, callOrder)
	mu.Unlock()
	if len(order) < 1 || order[0] != "/setup" {
		t.Errorf("expected first call to be /setup, got %v", order)
	}
	// Verify teardown ran (should be last)
	if len(order) < 4 || order[len(order)-1] != "/teardown" {
		t.Errorf("expected last call to be /teardown, got %v", order)
	}
}

func TestRunCmd_parallel_failed_dep_skips(t *testing.T) {
	// First request will fail (bad port), second depends on it via variable
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := `name: Parallel Fail Test
requests:
  - name: Bad Request
    request:
      method: GET
      url: "http://127.0.0.1:1/fail"
    extract:
      token: "$.token"
  - name: Dependent
    request:
      method: GET
      url: "http://example.com/api?token={{token}}"
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel"})
	// Should not be 0 (the first request fails)
	if code == 0 {
		t.Error("expected non-zero exit code for failed request")
	}
}

func TestRunCmd_parallel_json_output(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Parallel JSON
requests:
  - name: A
    request:
      method: GET
      url: "%s/a"
  - name: B
    request:
      method: GET
      url: "%s/b"
`, srv.URL, srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{f, "--parallel", "--format", "json"})
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

// sharedVaultTemplateYAML is the two-environment fixture used by team template CLI tests.
const sharedVaultTemplateYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
`

func TestRunCmd_TeamSecrets_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo the Authorization header for assertion
		w.Header().Set("X-Auth", r.Header.Get("Authorization"))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()

	tpl := filepath.Join(tmp, "team.yaml")
	if err := os.WriteFile(tpl, []byte(sharedVaultTemplateYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	col := filepath.Join(tmp, "col.yaml")
	colContent := fmt.Sprintf(`name: T
requests:
  - name: A
    request:
      method: GET
      url: %s
      headers:
        Authorization: "Bearer {{secrets.api_key}}"
`, srv.URL)
	if err := os.WriteFile(col, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CURLEW_TEAM_CONFIG", tpl)
	t.Setenv("CURLEW_VAULT_STUB", "1")

	code := runCmd([]string{col, "--env", "production"})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestRunCmd_TeamSecrets_MissingFile(t *testing.T) {
	t.Setenv("CURLEW_TEAM_CONFIG", "/does/not/exist/team.yaml")

	// We need a valid collection file for the command to get past argument parsing.
	tmp := t.TempDir()
	col := filepath.Join(tmp, "col.yaml")
	if err := os.WriteFile(col, []byte("name: T\nrequests:\n  - name: A\n    request:\n      method: GET\n      url: http://example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runCmd([]string{col})
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

// A leftover team_vault.json from a pre-backend-removal install is inert: the CLI
// no longer reads or writes that cache, so it must not make a missing env file
// tolerable the way CURLEW_TEAM_CONFIG does.
func TestRunCmd_StaleVaultCache_DoesNotTolerateMissingEnvFile(t *testing.T) {
	// Local server so a regression fails on the exit code rather than reaching out.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()

	cfgDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "team_vault.json"), []byte(`{"template":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	col := filepath.Join(tmp, "col.yaml")
	colContent := fmt.Sprintf("name: T\nrequests:\n  - name: A\n    request:\n      method: GET\n      url: %s\n", srv.URL)
	if err := os.WriteFile(col, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CURLEW_CONFIG_DIR", cfgDir)
	t.Setenv("CURLEW_TEAM_CONFIG", "")

	code := runCmd([]string{col, "--env", "staging"})
	if code != 3 {
		t.Fatalf("exit code = %d, want 3 (environment not found)", code)
	}
}

func TestRunCmd_TeamSecrets_MissingEnvFlag(t *testing.T) {
	tmp := t.TempDir()

	tpl := filepath.Join(tmp, "team.yaml")
	if err := os.WriteFile(tpl, []byte(sharedVaultTemplateYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	col := filepath.Join(tmp, "col.yaml")
	colContent := "name: T\nrequests:\n  - name: A\n    request:\n      method: GET\n      url: http://example.com/{{secrets.api_key}}\n"
	if err := os.WriteFile(col, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CURLEW_TEAM_CONFIG", tpl)
	t.Setenv("CURLEW_VAULT_STUB", "1")

	// No --env flag
	code := runCmd([]string{col})
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}
}

func TestRunCmd_TeamSecrets_LogsResolvedCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()

	tpl := filepath.Join(tmp, "team.yaml")
	if err := os.WriteFile(tpl, []byte(sharedVaultTemplateYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	col := filepath.Join(tmp, "col.yaml")
	// Reference both api_key and db_password so SharedSecretsResolved == 2.
	colContent := fmt.Sprintf(`name: T
requests:
  - name: A
    request:
      method: GET
      url: "%s/{{secrets.api_key}}/{{secrets.db_password}}"
`, srv.URL)
	if err := os.WriteFile(col, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CURLEW_TEAM_CONFIG", tpl)
	t.Setenv("CURLEW_VAULT_STUB", "1")

	var outBuf, errBuf bytes.Buffer
	code := runCmdWithWriters([]string{col, "--env", "production"}, &outBuf, &errBuf)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}

	wantMsg := "Resolved 2 secrets from shared template (production)"
	stderrText := errBuf.String()
	if !strings.Contains(stderrText, wantMsg) {
		t.Errorf("stderr does not contain %q; got: %q", wantMsg, stderrText)
	}
	// Verify the message appears exactly once.
	count := strings.Count(stderrText, wantMsg)
	if count != 1 {
		t.Errorf("log message appears %d times, want exactly 1", count)
	}
}

func TestRunCmd_Help_MentionsTeamTemplate(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	helpText := buf.String()

	for _, want := range []string{
		"CURLEW_TEAM_CONFIG",
		"--env",
		"shared vault",
	} {
		if !strings.Contains(helpText, want) {
			t.Errorf("help text does not contain %q", want)
		}
	}
}

// --- M6-005: --events stream integration tests ---

// parseJSONL reads a file and returns its lines as a slice of decoded JSON objects.
func parseJSONL(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	var out []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			t.Fatalf("unmarshal NDJSON line %q: %v", line, err)
		}
		out = append(out, obj)
	}
	return out
}

// TestRunCmd_Events_HappyPath verifies that --events produces a well-formed NDJSON
// stream starting with run.start and ending with run.end with matching event_count.
func TestRunCmd_Events_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "happy.yaml", fmt.Sprintf(`
name: happy
requests:
  - name: ping
    request:
      method: GET
      url: %q
    assertions:
      status: 200
`, srv.URL))
	eventsFile := filepath.Join(tmp, "events.jsonl")

	_, _, code := captureRunCmd(t, col, "--events", eventsFile)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}

	lines := parseJSONL(t, eventsFile)
	if len(lines) == 0 {
		t.Fatal("events file is empty")
	}

	// First line must be run.start, last must be run.end.
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("first line kind = %q, want run.start", kind)
	}
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last line kind = %q, want run.end", kind)
	}

	// event_count in run.end must match total line count.
	if ec, _ := last["event_count"].(float64); int(ec) != len(lines) {
		t.Errorf("event_count = %v, want %d (line count)", ec, len(lines))
	}

	// Must include at least: run.start, request.start, assertion.result, request.end, run.end.
	wantKinds := []string{"run.start", "request.start", "assertion.result", "request.end", "run.end"}
	for i, want := range wantKinds {
		if i >= len(lines) {
			t.Errorf("line[%d] missing; want kind %q", i, want)
			break
		}
		got, _ := lines[i]["kind"].(string)
		if got != want {
			t.Errorf("line[%d] kind = %q, want %q", i, got, want)
		}
	}
}

// TestRunCmd_Events_UndefinedVariable_EmitsRunError verifies that a pre-run
// variable interpolation failure emits run.error before run.end.
func TestRunCmd_Events_UndefinedVariable_EmitsRunError(t *testing.T) {
	tmp := t.TempDir()
	col := writeCollection(t, tmp, "fail.yaml", `
name: unresolved
requests:
  - name: needs-var
    request:
      method: GET
      url: "{{MISSING}}"
`)
	eventsFile := filepath.Join(tmp, "events-fail.jsonl")

	_, _, code := captureRunCmd(t, col, "--events", eventsFile)
	if code == 0 {
		t.Fatal("want non-zero exit for undefined variable")
	}

	lines := parseJSONL(t, eventsFile)
	if len(lines) < 3 {
		t.Fatalf("want at least 3 events (run.start, run.error, run.end), got %d: %v", len(lines), lines)
	}

	// Second line must be run.error.
	if kind, _ := lines[1]["kind"].(string); kind != "run.error" {
		t.Errorf("line[1] kind = %q, want run.error", kind)
	}

	// run.error must have all agent-diagnosability fields (M6-007 contract).
	errObj, _ := lines[1]["error"].(map[string]any)
	if errObj == nil {
		t.Errorf("run.error missing 'error' field")
	} else {
		if cat, _ := errObj["category"].(string); cat != "input" {
			t.Errorf("run.error.error.category = %q, want input", cat)
		}
		if code2, _ := errObj["code"].(string); code2 != "VAR_UNDEFINED" {
			t.Errorf("run.error.error.code = %q, want VAR_UNDEFINED", code2)
		}
		if file, _ := errObj["file"].(string); file == "" {
			t.Errorf("run.error.error.file is empty, want non-empty (source location)")
		}
		if line, _ := errObj["line"].(float64); int(line) == 0 {
			t.Errorf("run.error.error.line = 0, want non-zero (source line number)")
		}
	}

	// Last line must be run.end with non-zero exit_code.
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last line kind = %q, want run.end", kind)
	}
	if ec, _ := last["exit_code"].(float64); int(ec) == 0 {
		t.Errorf("run.end.exit_code = 0, want non-zero")
	}
}

// TestBinary_Run_EventsFlag is an end-to-end test that builds the real curlew
// binary and asserts the NDJSON events stream shape for the --events happy path.
func TestBinary_Run_EventsFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping binary build in short mode")
	}
	binary := buildBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	col := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(col, []byte(fmt.Sprintf(`
name: binary-events-test
requests:
  - name: ping
    request:
      method: GET
      url: %q
    assertions:
      status: 200
`, srv.URL)), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	eventsPath := filepath.Join(dir, "events.jsonl")

	_, _, code := runBinary(t, binary, "run", col, "--events", eventsPath)
	if code != 0 {
		t.Fatalf("binary exit code = %d, want 0", code)
	}

	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	lines := parseJSONL(t, eventsPath)
	if len(lines) == 0 {
		t.Fatalf("events file has no lines; content: %q", string(data))
	}
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("first event kind = %q, want run.start", kind)
	}
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last event kind = %q, want run.end", kind)
	}
	if ec, _ := last["event_count"].(float64); int(ec) != len(lines) {
		t.Errorf("event_count = %v, want %d", ec, len(lines))
	}
}

// TestRunCmd_Events_ParseError_EmitsRunError verifies that a collection parse
// error emits run.error before run.end with non-zero exit_code.
func TestRunCmd_Events_ParseError_EmitsRunError(t *testing.T) {
	tmp := t.TempDir()
	col := writeCollection(t, tmp, "invalid.yaml", `this: is: not: valid: yaml: collection`)
	eventsFile := filepath.Join(tmp, "events-parse-err.jsonl")

	_, _, code := captureRunCmd(t, col, "--events", eventsFile)
	if code == 0 {
		t.Fatal("want non-zero exit for parse error")
	}

	lines := parseJSONL(t, eventsFile)
	if len(lines) < 3 {
		t.Fatalf("want at least 3 events (run.start, run.error, run.end), got %d: %v", len(lines), lines)
	}

	// First line must be run.start.
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("line[0] kind = %q, want run.start", kind)
	}

	// Second line must be run.error.
	if kind, _ := lines[1]["kind"].(string); kind != "run.error" {
		t.Errorf("line[1] kind = %q, want run.error", kind)
	}

	// Last line must be run.end with non-zero exit_code.
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last line kind = %q, want run.end", kind)
	}
	if ec, _ := last["exit_code"].(float64); int(ec) == 0 {
		t.Errorf("run.end.exit_code = 0, want non-zero for parse error")
	}
}

// TestRunCmd_Events_RedactsSensitiveBodyValues verifies that request and
// response bodies containing sensitive variable values are redacted in the
// emitted NDJSON events file (finding #1 from review iteration 5).
//
// A variable named API_KEY is heuristically detected as sensitive; its value
// must not appear in any event line of the events file.
func TestRunCmd_Events_RedactsSensitiveBodyValues(t *testing.T) {
	const sensitiveValue = "supersecret-api-key-12345"

	// Server echoes the request body (which will contain the sensitive value via
	// URL path). Use JSON response so the response body is a JSON object containing
	// the secret.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// Return a JSON body containing the sensitive value to test response body redaction.
		_, _ = fmt.Fprintf(w, `{"token":%q}`, sensitiveValue)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	// The collection uses the {{API_KEY}} variable in the request body so that
	// both the request body (via template interpolation) and response body (echoed
	// by the server) contain the sensitive value.
	col := writeCollection(t, tmp, "sensitive.yaml", fmt.Sprintf(`
name: sensitive-test
requests:
  - name: send-key
    request:
      method: POST
      url: %q
      body: '{"api_key":"{{API_KEY}}"}'
    assertions:
      status: 200
`, srv.URL))
	eventsFile := filepath.Join(tmp, "events-sensitive.jsonl")

	_, _, code := captureRunCmd(t, col, "--var", "API_KEY="+sensitiveValue, "--events", eventsFile)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}

	data, err := os.ReadFile(eventsFile)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}

	// The raw sensitive value must not appear anywhere in the NDJSON file.
	if strings.Contains(string(data), sensitiveValue) {
		t.Errorf("sensitive value %q found in events file; want [REDACTED]:\n%s", sensitiveValue, string(data))
	}

	// Verify the events file contains the redaction marker in the request.end events.
	lines := parseJSONL(t, eventsFile)
	var foundRequestEnd bool
	for _, line := range lines {
		if kind, _ := line["kind"].(string); kind == "request.end" {
			foundRequestEnd = true
			// The request.end line must contain [REDACTED] when the body has been redacted.
			lineJSON, _ := json.Marshal(line)
			if !strings.Contains(string(lineJSON), "[REDACTED]") {
				t.Errorf("request.end event does not contain [REDACTED]: %s", string(lineJSON))
			}
		}
	}
	if !foundRequestEnd {
		t.Errorf("no request.end event found in events file")
	}
}

// TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd verifies that when
// an assertion fails, request.end carries an error block with category=assertion
// and code=ASSERTION_FAILED and a hint with a concrete action verb. This
// satisfies M6-007 D8.4 and the failing-assertion harness scenario contract.
func TestRunCmd_Events_FailingAssertion_EmitsErrorOnRequestEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // always returns 200
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "failing-assert.yaml", fmt.Sprintf(`
name: failing-assertion-test
requests:
  - name: expect-201-got-200
    request:
      method: GET
      url: %q
    assertions:
      status: 201
`, srv.URL))
	eventsFile := filepath.Join(tmp, "events-assert-fail.jsonl")

	_, _, code := captureRunCmd(t, col, "--events", eventsFile)
	if code != 1 {
		t.Fatalf("want exit 1 (assertion failure), got %d", code)
	}

	lines := parseJSONL(t, eventsFile)

	// Find request.end event.
	var endLine map[string]any
	for _, l := range lines {
		if l["kind"] == "request.end" {
			endLine = l
			break
		}
	}
	if endLine == nil {
		t.Fatal("no request.end event found in events file")
	}

	outcome, _ := endLine["outcome"].(string)
	if outcome != "failed" {
		t.Errorf("request.end outcome = %q, want failed", outcome)
	}

	errObj, ok := endLine["error"].(map[string]any)
	if !ok {
		t.Fatal("request.end missing 'error' block (want error on assertion failure)")
	}
	if cat, _ := errObj["category"].(string); cat != "assertion" {
		t.Errorf("error.category = %q, want assertion", cat)
	}
	if code2, _ := errObj["code"].(string); code2 != "ASSERTION_FAILED" {
		t.Errorf("error.code = %q, want ASSERTION_FAILED", code2)
	}
	hint, _ := errObj["hint"].(string)
	verbFound := false
	for _, verb := range []string{"Inspect", "Check", "Verify", "Adjust", "Set", "Add"} {
		if strings.Contains(hint, verb) {
			verbFound = true
			break
		}
	}
	if !verbFound {
		t.Errorf("error.hint %q lacks a concrete action verb (Inspect/Check/Verify/...)", hint)
	}
}

// TestRun_MarkdownFormat_RequiresReport verifies that --format markdown without
// --report exits with code 3 and a meaningful error message.
func TestRun_MarkdownFormat_RequiresReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
`, srv.URL))

	_, stderr, exitCode := captureRunCmd(t, col, "--format", "markdown")
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "format: markdown requires --report") {
		t.Errorf("expected stderr to mention 'format: markdown requires --report', got: %q", stderr)
	}
}

// TestRun_MarkdownFormat_HappyPath verifies that --format markdown --report <dir>
// creates run.md and per-request .md files with the 10-section structure.
func TestRun_MarkdownFormat_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	_, stderr, exitCode := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
	}

	// run.md must exist.
	if _, err := os.Stat(filepath.Join(reportDir, "run.md")); os.IsNotExist(err) {
		t.Error("run.md not created")
	}

	// get-user.md must exist.
	got, err := os.ReadFile(filepath.Join(reportDir, "get-user.md"))
	if err != nil {
		t.Fatalf("read get-user.md: %v", err)
	}

	// Count the 11 section markers (M9-003 adds ### Response metadata).
	content := string(got)
	markers := []string{
		"# ", "## Notes", "<!-- BEGIN curlew:response",
		"## Response (deterministic)", "### Request", "### Response ",
		"### Response metadata", "### Timing", "### Assertions",
		"<!-- END curlew:response", "## Analysis",
	}
	for _, m := range markers {
		if !strings.Contains(content, m) {
			t.Errorf("section marker %q not found in get-user.md", m)
		}
	}

	// Sentinel format check.
	if !strings.Contains(content, "<!-- BEGIN curlew:response id=req-1 slug=get-user run=") {
		t.Errorf("expected sentinel with slug=get-user in get-user.md:\n%s", content)
	}
}

// TestRun_MarkdownFormat_SpliceOnRerun verifies that running twice preserves
// agent-authored content outside the sentinel region.
func TestRun_MarkdownFormat_SpliceOnRerun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	// First run.
	if _, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir); code != 0 {
		t.Fatalf("first run exited %d", code)
	}

	// Append AGENT NOTE after the END sentinel.
	target := filepath.Join(reportDir, "get-user.md")
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	_, _ = f.WriteString("\nAGENT NOTE: hypothesis X\n")
	_ = f.Close()

	// Second run.
	if _, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir); code != 0 {
		t.Fatalf("second run exited %d", code)
	}

	// AGENT NOTE must be preserved.
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), "AGENT NOTE: hypothesis X") {
		t.Errorf("AGENT NOTE not preserved after rerun:\n%s", got)
	}
}

// TestRun_MarkdownFormat_NoSentinelWritesDotNew verifies that a user-handwritten
// file without sentinels causes .md.new to be written alongside, with original untouched.
func TestRun_MarkdownFormat_NoSentinelWritesDotNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Pre-create get-user.md without sentinels.
	handwritten := "# User handwrote this\n\nSome notes.\n"
	target := filepath.Join(reportDir, "get-user.md")
	if err := os.WriteFile(target, []byte(handwritten), 0o644); err != nil {
		t.Fatalf("write handwritten: %v", err)
	}

	_, stderr, _ := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir)

	// Original untouched.
	original, _ := os.ReadFile(target)
	if string(original) != handwritten {
		t.Error("original file should be untouched when it has no sentinels")
	}
	// .new file created.
	if _, err := os.Stat(target + ".new"); os.IsNotExist(err) {
		t.Error("expected get-user.md.new to be created")
	}
	// Warning in stderr.
	if !strings.Contains(stderr, "no sentinel pair") {
		t.Errorf("expected 'no sentinel pair' warning in stderr, got: %q", stderr)
	}
}

// TestConfig_RejectsMarkdownWithoutReport exercises the YAML-resolved code
// path at main.go:1057-1066: when a collection declares `output.format:
// markdown` but no --report flag is supplied on the CLI, the tool must exit
// with code 3 and a message naming the missing requirement.
//
// This is a distinct code path from TestRun_MarkdownFormat_RequiresReport
// (which tests the CLI --format flag path). The YAML-resolved path fires
// after resolveOutputPrecedence has merged collection and project config.
func TestConfig_RejectsMarkdownWithoutReport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	// Collection declares output.format: markdown at YAML level; no --report on CLI.
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
output:
  format: markdown
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
`, srv.URL))

	_, stderr, exitCode := captureRunCmd(t, col)
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "format: markdown requires --report") {
		t.Errorf("expected stderr to mention 'format: markdown requires --report', got: %q", stderr)
	}
}

// TestRun_MarkdownFormat_MalformedSentinelWritesDotNew verifies that a file
// with a malformed sentinel (BEGIN without END) causes .md.new to be written.
func TestRun_MarkdownFormat_MalformedSentinelWritesDotNew(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Pre-create with BEGIN but no END.
	malformed := "# Get user\n<!-- BEGIN curlew:response id=req-1 slug=get-user run=0000000000000000000000000000000a -->\nINCOMPLETE\n"
	target := filepath.Join(reportDir, "get-user.md")
	if err := os.WriteFile(target, []byte(malformed), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}

	_, stderr, _ := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir)

	// Original untouched.
	original, _ := os.ReadFile(target)
	if string(original) != malformed {
		t.Error("original file should be untouched when sentinel is malformed")
	}
	// .new file created.
	if _, err := os.Stat(target + ".new"); os.IsNotExist(err) {
		t.Error("expected get-user.md.new to be created")
	}
	// Warning in stderr.
	if !strings.Contains(stderr, "malformed sentinel") {
		t.Errorf("expected 'malformed sentinel' warning in stderr, got: %q", stderr)
	}
}

// TestRun_MarkdownFormat_DataDriven verifies that --format markdown with a
// data-driven collection produces <slug>/iter-N.md files, an index.md, and
// a run.md aggregate bullet linking to the index.
func TestRun_MarkdownFormat_DataDriven(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "users.csv"), []byte("name\nalice\nbob\ncarol\ndave\neve"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: DD
requests:
  - name: Seed users
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/{{name}}"
    assertions:
      status: 200
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	_, stderr, exitCode := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d\nstderr: %s", exitCode, stderr)
	}

	// 5 iter-*.md files in subdir.
	for i := 0; i < 5; i++ {
		path := filepath.Join(reportDir, "seed-users", fmt.Sprintf("iter-%d.md", i))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s: %v", path, err)
		}
	}
	// index.md exists.
	if _, err := os.Stat(filepath.Join(reportDir, "seed-users", "index.md")); err != nil {
		t.Errorf("expected index.md: %v", err)
	}
	// run.md links to index with aggregate count.
	runMD, _ := os.ReadFile(filepath.Join(reportDir, "run.md"))
	if !strings.Contains(string(runMD), "[Seed users](seed-users/index.md)") {
		t.Errorf("run.md missing aggregate link:\n%s", runMD)
	}
	if !strings.Contains(string(runMD), "5 iterations") {
		t.Errorf("run.md missing iteration count:\n%s", runMD)
	}
}

// TestRun_MarkdownFormat_DataDrivenSplice verifies that re-running a data-driven
// collection preserves agent edits outside sentinels in iter files.
func TestRun_MarkdownFormat_DataDrivenSplice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "data.csv"), []byte("id\n1\n2\n3"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Splice DD
requests:
  - name: Create item
    data_driven:
      source: data.csv
    request:
      method: GET
      url: "%s/{{id}}"
    assertions:
      status: 200
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	// First run.
	if _, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir); code != 0 {
		t.Fatalf("first run exited %d", code)
	}

	// Append AGENT NOTE to iter-1.md.
	iterPath := filepath.Join(reportDir, "create-item", "iter-1.md")
	existing, err := os.ReadFile(iterPath)
	if err != nil {
		t.Fatalf("read iter-1.md: %v", err)
	}
	if err := os.WriteFile(iterPath, append(existing, []byte("\nAGENT NOTE: splice test\n")...), 0o644); err != nil {
		t.Fatalf("write iter-1.md: %v", err)
	}

	// Second run.
	if _, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir); code != 0 {
		t.Fatalf("second run exited %d", code)
	}

	// AGENT NOTE must be preserved.
	after, _ := os.ReadFile(iterPath)
	if !strings.Contains(string(after), "AGENT NOTE: splice test") {
		t.Errorf("AGENT NOTE not preserved after rerun:\n%s", after)
	}
}

// TestRun_MarkdownFormat_ParallelWaves verifies wave grouping on a real
// 3-wave parallel collection (A→B→C chained via extract variables): run.md
// has ## Wave 0, ## Wave 1, ## Wave 2 headers in ascending order, and each
// per-request file carries the matching wave_index value.
func TestRun_MarkdownFormat_ParallelWaves(t *testing.T) {
	// Server returns JSON with extractable fields so the dependency chain
	// forces three distinct waves: A (wave 0) extracts token; B (wave 1)
	// uses token and extracts user_id; C (wave 2) uses user_id.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/a":
			_, _ = w.Write([]byte(`{"token":"tok123"}`))
		case "/b":
			_, _ = w.Write([]byte(`{"user_id":"u42"}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	// A extracts token → wave 0.
	// B uses {{token}}, extracts user_id → wave 1 (depends on A).
	// C uses {{user_id}} → wave 2 (depends on B).
	col := writeCollection(t, tmpDir, "test.yaml", fmt.Sprintf(`name: Par
requests:
  - name: A
    request: {method: GET, url: "%[1]s/a"}
    assertions: {status: 200}
    extract:
      token: "$.token"
  - name: B
    request: {method: GET, url: "%[1]s/b?tok={{token}}"}
    assertions: {status: 200}
    extract:
      user_id: "$.user_id"
  - name: C
    request: {method: GET, url: "%[1]s/c?uid={{user_id}}"}
    assertions: {status: 200}
`, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	_, _, exitCode := captureRunCmd(t, col, "--parallel", "--format", "markdown", "--report", reportDir)
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}

	runMD, _ := os.ReadFile(filepath.Join(reportDir, "run.md"))
	runMDStr := string(runMD)

	// All three wave headers must be present in ascending order.
	for _, header := range []string{"## Wave 0", "## Wave 1", "## Wave 2"} {
		if !strings.Contains(runMDStr, header) {
			t.Errorf("expected %q in run.md:\n%s", header, runMDStr)
		}
	}
	pos0 := strings.Index(runMDStr, "## Wave 0")
	pos1 := strings.Index(runMDStr, "## Wave 1")
	pos2 := strings.Index(runMDStr, "## Wave 2")
	if pos0 >= pos1 || pos1 >= pos2 {
		t.Errorf("wave headers not in ascending order: 0@%d 1@%d 2@%d", pos0, pos1, pos2)
	}

	// Each per-request file must carry the matching wave_index.
	wantWave := map[string]string{
		"a.md": "wave_index: 0",
		"b.md": "wave_index: 1",
		"c.md": "wave_index: 2",
	}
	for file, want := range wantWave {
		content, err := os.ReadFile(filepath.Join(reportDir, file))
		if err != nil {
			t.Errorf("read %s: %v", file, err)
			continue
		}
		if !strings.Contains(string(content), want) {
			t.Errorf("expected %q in %s:\n%s", want, file, content)
		}
	}
}

// TestRun_MarkdownFormat_RedactionInvariant proves that --allow-sensitive does
// NOT flow through to the markdown formatter: even when the flag is set, body
// and headers in the .md output remain redacted because main.go rewrites
// results[i] before dispatch.
func TestRun_MarkdownFormat_RedactionInvariant(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "unless `--allow-sensitive` was passed to a command that accepts it"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}

	const secret = "sk_live_secret123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return the secret in the response body.
		_, _ = fmt.Fprintf(w, `{"token":%q}`, secret)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "secret.yaml", fmt.Sprintf(`name: Secret Test
variables:
  token: !sensitive %q
requests:
  - name: Get user
    request:
      method: GET
      url: "%s"
      headers:
        Authorization: "Bearer {{token}}"
`, secret, srv.URL))
	reportDir := filepath.Join(tmpDir, "resp")

	_, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", reportDir, "--allow-sensitive")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	got, err := os.ReadFile(filepath.Join(reportDir, "get-user.md"))
	if err != nil {
		t.Fatalf("read get-user.md: %v", err)
	}
	// The raw secret must NOT appear in the markdown output.
	if bytes.Contains(got, []byte(secret)) {
		t.Errorf("raw secret %q leaked into markdown despite redaction:\n%s", secret, got)
	}
	// [REDACTED] must appear (proving body/header flowed through the redaction path).
	if !bytes.Contains(got, []byte("[REDACTED]")) {
		t.Errorf("expected [REDACTED] in markdown output:\n%s", got)
	}
}

// ---- M20-001: --locale flag integration tests ----

func TestRun_EnvironmentConfigLocaleFromProjectRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "curlew.yaml"), []byte("project_name: locale-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	collectionsDir := filepath.Join(root, "collections")
	if err := os.MkdirAll(collectionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	col := filepath.Join(collectionsDir, "locale.yaml")
	if err := os.WriteFile(col, []byte(`name: environment-locale
requests:
  - name: faker
    request:
      method: GET
      url: "http://example.invalid/{{$faker.fullName}}"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	envDir := filepath.Join(root, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(`variables: {}
config:
  locale: xx-YY
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--env", "dev"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatal("run with unsupported environment locale returned success")
	}
	if !strings.Contains(stderr.String(), "xx-YY") {
		t.Fatalf("stderr = %q; want environment locale error", stderr.String())
	}
}

func TestRun_UnknownLocale_ErrLocaleUnknown(t *testing.T) {
	// Observable #4: unknown locale -> ERR_LOCALE_UNKNOWN, non-zero exit
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	col := filepath.Join(t.TempDir(), "col.yaml")
	if err := os.WriteFile(col, []byte("name: test\nrequests:\n  items:\n    - name: r\n      request:\n        url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--locale", "xx-YY"}, &bytes.Buffer{}, &stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown locale")
	}
	// The Structured error's Code is "ERR_LOCALE_UNKNOWN" and the message identifies the unknown locale.
	if !strings.Contains(stderr.String(), "xx-YY") {
		t.Errorf("stderr should mention unknown locale 'xx-YY', got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "Supported locales") {
		t.Errorf("stderr should list supported locales, got: %s", stderr.String())
	}
}

func TestRun_LocalePrecedence_FlagBeatsCollection(t *testing.T) {
	// Observable #6 (collection variant): CLI flag beats collection config.locale.
	// The inline collection has config.locale: en-US; passing --locale de-DE with -v
	// should log "resolved locale: de-DE (source: --locale flag)".
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	col := filepath.Join(t.TempDir(), "col.yaml")
	if err := os.WriteFile(col, []byte("name: test\nconfig:\n  locale: en-US\nrequests:\n  items:\n    - name: r\n      request:\n        url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--locale", "de-DE", "--seed", "42", "-v", "--dry-run"}, &stdout, &stderr)
	combined := stdout.String() + stderr.String()
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, combined)
	}
	if !strings.Contains(combined, "resolved locale: de-DE (source: --locale flag)") {
		t.Errorf("expected 'resolved locale: de-DE (source: --locale flag)' in output, got:\n%s", combined)
	}
}

func TestRun_LocalePrecedence_FlagBeatsProjectConfig(t *testing.T) {
	// Verifies Behavior 6: CLI --locale flag beats project config.locale.
	// Creates an curlew.yaml with config.locale: en-US alongside the collection;
	// passing --locale de-DE should win and log "resolved locale: de-DE (source: --locale flag)".
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	dir := t.TempDir()

	// curlew.yaml with project-level locale = en-US
	projectCfg := "config:\n  locale: en-US\nvariables: {}\n"
	if err := os.WriteFile(filepath.Join(dir, "curlew.yaml"), []byte(projectCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Collection with no config.locale (so project config is the only config source).
	col := filepath.Join(dir, "col.yaml")
	if err := os.WriteFile(col, []byte("name: test\nrequests:\n  items:\n    - name: r\n      request:\n        url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--locale", "de-DE", "--seed", "42", "-v", "--dry-run"}, &stdout, &stderr)
	combined := stdout.String() + stderr.String()
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; combined: %s", code, combined)
	}
	if !strings.Contains(combined, "resolved locale: de-DE (source: --locale flag)") {
		t.Errorf("expected 'resolved locale: de-DE (source: --locale flag)' in output, got:\n%s", combined)
	}
}

func TestRun_CollectionLocale_HonoredWhenFlagAbsent(t *testing.T) {
	// Behavior 7: collection config.locale: de-DE is honored when no --locale flag is passed.
	// Creates a collection with config.locale: de-DE and {{$faker.fullName}} in the URL,
	// runs with --seed 42 (no --locale flag), and asserts:
	//   1. Exit code 0.
	//   2. Verbose diagnostic names "collection config" as the locale source.
	//   3. The resolved URL does not contain an en-US surname (proving de-DE pool was used).

	// Capture the URL that the runner resolved and sent.
	var capturedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer ts.Close()

	col := filepath.Join(t.TempDir(), "col.yaml")
	colContent := fmt.Sprintf("name: test\nconfig:\n  locale: de-DE\nrequests:\n  - name: r\n    request:\n      method: GET\n      url: \"%s/{{$faker.fullName}}\"\n", ts.URL)
	if err := os.WriteFile(col, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--seed", "42", "-v"}, &stdout, &stderr)
	combined := stdout.String() + stderr.String()

	if code != 0 {
		t.Fatalf("expected exit 0, got %d; combined: %s", code, combined)
	}

	// Assert 2: verbose diagnostic names "collection config" as the source.
	if !strings.Contains(combined, "resolved locale: de-DE (source: collection config)") {
		t.Errorf("expected 'resolved locale: de-DE (source: collection config)' in output, got:\n%s", combined)
	}

	// Assert 3: the resolved URL used a de-DE name (no common en-US-only surname).
	enUSLastNames := []string{"Smith", "Johnson", "Williams", "Jones", "Brown", "Davis", "Miller", "Wilson"}
	for _, name := range enUSLastNames {
		if strings.Contains(capturedPath, name) {
			t.Errorf("run with collection locale de-DE produced en-US name %q in URL path %q", name, capturedPath)
		}
	}
	if capturedPath == "" || capturedPath == "/" {
		t.Errorf("server received no request or empty path; faker token may not have been interpolated (stdout: %s)", stdout.String())
	}
}

// TestRun_Locale_JaJP_NativePools verifies that ja-JP now resolves to its own
// pool (M20-003) and emits NO fallback warning even with -v.
// Mirrors TestRun_Locale_EnGB_NativePools.
func TestRun_Locale_JaJP_NativePools(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	col := filepath.Join(t.TempDir(), "col.yaml")
	if err := os.WriteFile(col, []byte("name: test\nrequests:\n  items:\n    - name: r\n      request:\n        url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--locale", "ja-JP", "--seed", "42", "-v"}, &bytes.Buffer{}, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "falling back") {
		t.Errorf("ja-JP emitted a fallback warning; its native pool now exists. stderr: %s", stderr.String())
	}
}

// TestRun_Locale_EnGB_NativePools verifies that en-GB now resolves to its own
// pool (M20-002) and emits NO fallback warning even with -v.
func TestRun_Locale_EnGB_NativePools(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	col := filepath.Join(t.TempDir(), "col.yaml")
	if err := os.WriteFile(col, []byte("name: test\nrequests:\n  items:\n    - name: r\n      request:\n        url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	code := runCmdInnerWithErr([]string{col, "--locale", "en-GB", "--seed", "42", "-v"}, &bytes.Buffer{}, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "falling back") {
		t.Errorf("en-GB should not emit fallback warning now that its native pool exists; stderr: %s", stderr.String())
	}
}

// runCmdInnerWithErr is a thin shim to get the exit code from runCmdInner.
func runCmdInnerWithErr(args []string, stdout, stderr *bytes.Buffer) int {
	code, _ := runCmdInner(args, stdout, stderr)
	return code
}
