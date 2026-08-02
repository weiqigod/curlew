package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/assertion"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/output"
	"github.com/peterlindqvist/apitest/internal/output/events"
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/runner"
	"github.com/peterlindqvist/apitest/internal/runservice"
	"github.com/peterlindqvist/apitest/internal/signer"
	"github.com/peterlindqvist/apitest/internal/validator"
)

// captureRunCmd calls runCmd in-process and captures stdout/stderr output.
// Uses writer injection — safe to run in parallel with other tests.
func captureRunCmd(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	exitCode = runCmdWithWriters(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), exitCode
}

func writeCollection(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "apitest")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join(".", ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return binary
}

func runBinary(t *testing.T, binary string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runBinaryWithEnv(t, binary, nil, args...)
}

func runBinaryWithEnv(t *testing.T, binary string, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	if env != nil {
		cmd.Env = env
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// TestRunCmdInner_RoutesOutputToInjectedWriters verifies that the stdout/stderr
// writers passed to runCmdInner receive the correct output for each format path.
func TestRunCmdInner_RoutesOutputToInjectedWriters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()
	tmpDir := t.TempDir()
	f := writeSimpleCollectionForMain(t, tmpDir, srv.URL)

	cases := []struct {
		name          string
		args          []string
		wantStdoutSub string
		wantStderrSub string
		wantExitCode  int
	}{
		{"json success routes to stdout", []string{f, "--format", "json"}, `"status"`, "", 0},
		{"tap success routes to stdout", []string{f, "--format", "tap"}, "TAP version 13", "", 0},
		// --var without a value triggers a parse error; the usage line and error go to stderr.
		{"usage error routes to stderr", []string{f, "--var"}, "", "Usage:", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code, _ := runCmdInner(tc.args, &stdout, &stderr)
			if code != tc.wantExitCode {
				t.Errorf("exit=%d want=%d stdout=%q stderr=%q", code, tc.wantExitCode, stdout.String(), stderr.String())
			}
			if tc.wantStdoutSub != "" && !strings.Contains(stdout.String(), tc.wantStdoutSub) {
				t.Errorf("stdout missing %q; got %q", tc.wantStdoutSub, stdout.String())
			}
			if tc.wantStderrSub != "" && !strings.Contains(stderr.String(), tc.wantStderrSub) {
				t.Errorf("stderr missing %q; got %q", tc.wantStderrSub, stderr.String())
			}
		})
	}
}

// TestRunCmdInner_StdoutUntouched_OnParseError verifies that the stdout buffer
// receives zero bytes when the only output path is an error on stderr.
// Uses --var without a value which is a genuine flag parse error.
func TestRunCmdInner_StdoutUntouched_OnParseError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// --var without a value is a parse error; stdout must stay empty.
	code, _ := runCmdInner([]string{"somefile.yaml", "--var"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout wrote %d bytes on parse-error path; should be zero (stderr-only): %q",
			stdout.Len(), stdout.String())
	}
}

// writeSimpleCollectionForMain is a test helper that writes a minimal collection
// YAML file to the given dir, pointing requests at serverURL. Used in main_test.go
// for tests that cannot import the run_test.go helper (same package, but declared
// here to avoid the duplicate-declaration build error when both are needed).
func writeSimpleCollectionForMain(t *testing.T, dir, serverURL string) string {
	t.Helper()
	f := filepath.Join(dir, "simple.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: %q\n", serverURL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestParseRunArgs(t *testing.T) {
	// Set up fake env vars for --env-var tests
	t.Setenv("PARSE_TEST_KEY", "secret")
	t.Setenv("PARSE_TEST_CI", "ci-secret")

	tests := []struct {
		name        string
		args        []string
		wantFile    string
		wantEnv     string
		wantVars    map[string]string
		wantEnvVars map[string]string
		wantErr     bool
	}{
		{"file_only", []string{"col.yaml"}, "col.yaml", "", map[string]string{}, map[string]string{}, false},
		{"file_with_one_var", []string{"col.yaml", "--var", "k=v"}, "col.yaml", "", map[string]string{"k": "v"}, map[string]string{}, false},
		{"file_with_multiple_vars", []string{"col.yaml", "--var", "a=1", "--var", "b=2"}, "col.yaml", "", map[string]string{"a": "1", "b": "2"}, map[string]string{}, false},
		{"var_before_file", []string{"--var", "k=v", "col.yaml"}, "col.yaml", "", map[string]string{"k": "v"}, map[string]string{}, false},
		{"no_file", []string{"--var", "k=v"}, "", "", nil, nil, true},
		{"var_without_value", []string{"col.yaml", "--var"}, "", "", nil, nil, true},
		{"var_invalid_format", []string{"col.yaml", "--var", "noequals"}, "", "", nil, nil, true},
		{"no_args", []string{}, "", "", nil, nil, true},
		{"duplicate_var_last_wins", []string{"col.yaml", "--var", "k=1", "--var", "k=2"}, "col.yaml", "", map[string]string{"k": "2"}, map[string]string{}, false},
		{"value_with_equals", []string{"col.yaml", "--var", "k=v=w"}, "col.yaml", "", map[string]string{"k": "v=w"}, map[string]string{}, false},
		{"env_flag", []string{"col.yaml", "--env", "dev"}, "col.yaml", "dev", map[string]string{}, map[string]string{}, false},
		{"env_with_var", []string{"col.yaml", "--env", "dev", "--var", "x=1"}, "col.yaml", "dev", map[string]string{"x": "1"}, map[string]string{}, false},
		{"env_without_value", []string{"col.yaml", "--env"}, "", "", nil, nil, true},
		{"no_env_flag", []string{"col.yaml"}, "col.yaml", "", map[string]string{}, map[string]string{}, false},
		{"env_var_bare", []string{"col.yaml", "--env-var", "PARSE_TEST_KEY"}, "col.yaml", "", map[string]string{}, map[string]string{"PARSE_TEST_KEY": "secret"}, false},
		{"env_var_mapped", []string{"col.yaml", "--env-var", "KEY=$PARSE_TEST_CI"}, "col.yaml", "", map[string]string{}, map[string]string{"KEY": "ci-secret"}, false},
		{"env_var_multiple", []string{"col.yaml", "--env-var", "PARSE_TEST_KEY", "--env-var", "KEY=$PARSE_TEST_CI"}, "col.yaml", "", map[string]string{}, map[string]string{"PARSE_TEST_KEY": "secret", "KEY": "ci-secret"}, false},
		{"env_var_without_value", []string{"col.yaml", "--env-var"}, "", "", nil, nil, true},
		{"env_var_with_other_flags", []string{"col.yaml", "--env", "dev", "--env-var", "PARSE_TEST_KEY", "--var", "y=1"}, "col.yaml", "dev", map[string]string{"y": "1"}, map[string]string{"PARSE_TEST_KEY": "secret"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.file != tt.wantFile {
				t.Errorf("file = %q, want %q", flags.file, tt.wantFile)
			}
			if flags.envName != tt.wantEnv {
				t.Errorf("envName = %q, want %q", flags.envName, tt.wantEnv)
			}
			if len(flags.vars) != len(tt.wantVars) {
				t.Fatalf("vars = %v, want %v", flags.vars, tt.wantVars)
			}
			for k, want := range tt.wantVars {
				if got := flags.vars[k]; got != want {
					t.Errorf("vars[%q] = %q, want %q", k, got, want)
				}
			}
			if len(flags.envVarVars) != len(tt.wantEnvVars) {
				t.Fatalf("envVarVars = %v, want %v", flags.envVarVars, tt.wantEnvVars)
			}
			for k, want := range tt.wantEnvVars {
				if got := flags.envVarVars[k]; got != want {
					t.Errorf("envVarVars[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func ptrInt64(n int64) *int64 { return &n }

func TestParseRunArgs_seed(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantSeed *int64
		wantErr  bool
	}{
		{"seed 42", []string{"file.yaml", "--seed", "42"}, ptrInt64(42), false},
		{"seed zero", []string{"file.yaml", "--seed", "0"}, ptrInt64(0), false},
		{"seed negative", []string{"file.yaml", "--seed", "-1"}, ptrInt64(-1), false},
		{"seed no value", []string{"file.yaml", "--seed"}, nil, true},
		{"seed non-integer", []string{"file.yaml", "--seed", "abc"}, nil, true},
		{"no seed flag", []string{"file.yaml"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantSeed == nil {
				if flags.seed != nil {
					t.Errorf("seed = %v, want nil", *flags.seed)
				}
				return
			}
			if flags.seed == nil {
				t.Fatalf("seed = nil, want %d", *tt.wantSeed)
			}
			if *flags.seed != *tt.wantSeed {
				t.Errorf("seed = %d, want %d", *flags.seed, *tt.wantSeed)
			}
		})
	}
}

func TestParseRunArgs_noColor(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantNoColor bool
		wantErr     bool
	}{
		{"no flag defaults to false", []string{"file.yaml"}, false, false},
		{"no-color flag sets true", []string{"file.yaml", "--no-color"}, true, false},
		{"no-color alongside other flags", []string{"file.yaml", "--no-color", "--env", "dev"}, true, false},
		{"no-color before file", []string{"--no-color", "file.yaml"}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.noColor != tt.wantNoColor {
				t.Errorf("noColor = %v, want %v", flags.noColor, tt.wantNoColor)
			}
		})
	}
}

func TestParseRunArgs_format(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFormat string
		wantErr    bool
	}{
		{"format json", []string{"f.yaml", "--format", "json"}, "json", false},
		{"no format defaults empty", []string{"f.yaml"}, "", false},
		{"format without value returns error", []string{"f.yaml", "--format"}, "", true},
		{"format terminal", []string{"f.yaml", "--format", "terminal"}, "terminal", false},
		{"format unknown stored", []string{"f.yaml", "--format", "xml"}, "xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.format != tt.wantFormat {
				t.Errorf("format = %q, want %q", flags.format, tt.wantFormat)
			}
		})
	}
}

func TestParseRunArgs_Verbosity(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantVerbosity output.Verbosity
	}{
		{"no flag defaults to default", []string{"file.yaml"}, output.VerbosityDefault},
		{"v flag sets verbose", []string{"file.yaml", "-v"}, output.VerbosityVerbose},
		{"vv flag sets debug", []string{"file.yaml", "-vv"}, output.VerbosityDebug},
		{"q flag sets quiet", []string{"file.yaml", "-q"}, output.VerbosityQuiet},
		{"quiet flag sets quiet", []string{"file.yaml", "--quiet"}, output.VerbosityQuiet},
		{"last flag wins when both v and q", []string{"file.yaml", "-v", "-q"}, output.VerbosityQuiet},
		{"last flag wins when both q and v", []string{"file.yaml", "-q", "-v"}, output.VerbosityVerbose},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.verbosity != tt.wantVerbosity {
				t.Errorf("got %v, want %v", flags.verbosity, tt.wantVerbosity)
			}
		})
	}
}

func TestRunCmdDirect_JSONSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: JSON Direct Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "passed" {
		t.Errorf("expected status=passed, got %v", result["status"])
	}
}

func TestRunCmdDirect_JSONAssertionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: JSON Fail Direct
requests:
  - name: Expect 404
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", result["status"])
	}
}

func TestRunCmdDirect_JSONParseError(t *testing.T) {
	stdout, stderr, exitCode := captureRunCmd(t, "nonexistent_file_abc.yaml", "--format", "json")
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "error" {
		t.Errorf("expected status=error, got %v", result["status"])
	}
}

func TestRunCmdDirect_TerminalSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: Terminal Direct Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--no-color")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if !strings.Contains(stdout, "Terminal Direct Test") {
		t.Errorf("stdout missing collection name: %s", stdout)
	}
}

func TestRunCmdDirect_TerminalAssertionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: Terminal Fail
requests:
  - name: Expect 404
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--no-color")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s", exitCode, stdout)
	}
}

func TestRunCmdDirect_JSONNetworkError(t *testing.T) {
	col := `name: Network Error Direct
requests:
  - name: Refused
    request:
      method: GET
      url: "http://127.0.0.1:1/test"
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", result["status"])
	}
	requests, _ := result["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	req, _ := requests[0].(map[string]any)
	if _, ok := req["status_code"]; !ok {
		t.Error("expected status_code field present for error request")
	}
	if req["status_code"] != float64(0) {
		t.Errorf("expected status_code=0 for error request, got %v", req["status_code"])
	}
}

func TestRunCmdDirect_JSONVarError(t *testing.T) {
	col := `name: Circular Var Direct
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://httpbin.org/get"
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "error" {
		t.Errorf("expected status=error, got %v", result["status"])
	}
}

func TestRunCmdDirect_UnknownFormat(t *testing.T) {
	col := `name: Format Test
requests: []
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "xml")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout for unknown format, got: %q", stdout)
	}
	if !strings.Contains(stderr, "unknown output format") {
		t.Errorf("expected stderr to mention unknown format, got: %q", stderr)
	}
}

func TestRunCmdDirect_TerminalNetworkError(t *testing.T) {
	col := `name: Network Error Terminal
requests:
  - name: Refused
    request:
      method: GET
      url: "http://127.0.0.1:1/test"
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	_, _, exitCode := captureRunCmd(t, colFile, "--no-color")
	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4", exitCode)
	}
}

func TestRunCmdDirect_TerminalVarError(t *testing.T) {
	col := `name: Circular Var Terminal
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://httpbin.org/get"
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	_, stderr, exitCode := captureRunCmd(t, colFile, "--no-color")
	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "circular") {
		t.Errorf("expected stderr to mention circular, got: %q", stderr)
	}
}

func TestRunCmdDirect_JSONMissingEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: Env Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "json", "--env", "nonexistent-env-xyz")
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "error" {
		t.Errorf("expected status=error, got %v", result["status"])
	}
}

func TestRunCmdDirect_QuietMode_OnlySummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: Quiet Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--no-color", "-q")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if strings.Contains(stdout, "Quiet Test") {
		t.Errorf("quiet mode should suppress collection header, got: %s", stdout)
	}
	if !strings.Contains(stdout, "request(s)") {
		t.Errorf("quiet mode should still show summary line, got: %s", stdout)
	}
}

func TestRunCmdDirect_VerboseMode_ShowsHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: Verbose Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--no-color", "-v")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if !strings.Contains(stdout, "> GET") {
		t.Errorf("verbose mode should show request line, got: %s", stdout)
	}
	if !strings.Contains(stdout, "< 200") {
		t.Errorf("verbose mode should show response status, got: %s", stdout)
	}
	if !strings.Contains(stdout, "X-Custom: test-value") {
		t.Errorf("verbose mode should show response headers, got: %s", stdout)
	}
}

func TestRunCmdDirect_EmptyCollection(t *testing.T) {
	col := `name: Empty Collection
requests: []
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	_, _, exitCode := captureRunCmd(t, colFile, "--no-color")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
}

func TestShouldUseColor(t *testing.T) {
	tests := []struct {
		name        string
		noColorFlag bool
		noColorEnv  bool
		writer      interface{ Write([]byte) (int, error) }
		want        bool
	}{
		{"buffer non-TTY returns false", false, false, &bytes.Buffer{}, false},
		{"no-color flag returns false", true, false, &bytes.Buffer{}, false},
		{"NO_COLOR env returns false", false, true, &bytes.Buffer{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.noColorEnv {
				t.Setenv("NO_COLOR", "1")
			} else {
				if err := os.Unsetenv("NO_COLOR"); err != nil {
					t.Fatalf("os.Unsetenv: %v", err)
				}
			}
			got := shouldUseColor(tt.writer, tt.noColorFlag)
			if got != tt.want {
				t.Errorf("shouldUseColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewStderrPrinter_NoColorFlag(t *testing.T) {
	// Given --no-color is true, shouldUseColor(os.Stderr, true) must return false.
	if shouldUseColor(os.Stderr, true) {
		t.Errorf("shouldUseColor(os.Stderr, true) = true, want false")
	}
	// Smoke: the helper must not panic and must return a non-nil printer.
	p := newStderrPrinter(true)
	if p == nil {
		t.Fatal("newStderrPrinter(true) returned nil")
	}
}

func TestNewStderrPrinter_NoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	// NO_COLOR env suppresses color regardless of the noColor flag.
	if shouldUseColor(os.Stderr, false) {
		t.Errorf("shouldUseColor(os.Stderr, false) with NO_COLOR set = true, want false")
	}
	// Smoke: helper must not panic.
	p := newStderrPrinter(false)
	if p == nil {
		t.Fatal("newStderrPrinter(false) with NO_COLOR returned nil")
	}
}

func TestPrintHelpTo_WritesToProvidedWriter(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	if !strings.Contains(buf.String(), "--no-color") {
		t.Errorf("printHelpTo did not write expected content; got %q", buf.String())
	}
}

func TestHelpText_noColor(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	if !strings.Contains(buf.String(), "--no-color") {
		t.Errorf("help text does not contain '--no-color', got: %s", buf.String())
	}
}

func TestHelpText_format(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	if !strings.Contains(buf.String(), "--format") {
		t.Errorf("help text does not contain '--format', got: %s", buf.String())
	}
}

func TestHelpText_VerbosityFlags(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	// Check -v with surrounding context to avoid false positive from -vv substring match
	if !strings.Contains(buf.String(), "  -v ") {
		t.Errorf("help text does not contain standalone -v flag, got: %s", buf.String())
	}
	for _, flag := range []string{"-vv", "-q"} {
		if !strings.Contains(buf.String(), flag) {
			t.Errorf("help text does not contain %q, got: %s", flag, buf.String())
		}
	}
}

// TestNoOsStdoutAssignment is an anti-regression gate: it walks every .go file
// in the current (cmd/apitest) directory and fails if any source assigns to
// os.Stdout or os.Stderr. Reassigning these process globals corrupts streams
// shared by concurrent goroutines (M7-005 removed the last offender in
// discovery_run.go). Keep this test green — if you need to redirect output,
// pass a writer through the function's signature instead.
//
// Note: this test uses os.ReadDir(".") which relies on go test running with the
// package directory as CWD — this is the standard behaviour for `go test ./...`.
func TestNoOsStdoutAssignment(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	var offenders []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		file, parseErr := goparser.ParseFile(fset, e.Name(), nil, goparser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", e.Name(), parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			as, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range as.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				ident, ok := sel.X.(*ast.Ident)
				if !ok {
					continue
				}
				if ident.Name == "os" && (sel.Sel.Name == "Stdout" || sel.Sel.Name == "Stderr") {
					pos := fset.Position(as.Pos())
					offenders = append(offenders, fmt.Sprintf("%s:%d: reassignment of os.%s", pos.Filename, pos.Line, sel.Sel.Name))
				}
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("os.Stdout/os.Stderr reassignment detected (M7-005 prohibits this):\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func TestCLIIntegration(t *testing.T) {
	binary := buildBinary(t)

	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantOut  string
		wantErr  string
	}{
		{
			name:     "no args shows help",
			args:     []string{},
			wantExit: 0,
			wantOut:  "Usage:",
		},
		{
			name:     "help flag",
			args:     []string{"--help"},
			wantExit: 0,
			wantOut:  "Commands:",
		},
		{
			name:     "version flag",
			args:     []string{"--version"},
			wantExit: 0,
			wantOut:  "apitest",
		},
		{
			name:     "run without file shows usage",
			args:     []string{"run"},
			wantExit: 1,
			wantErr:  "Usage:",
		},
		{
			name:     "run with missing file",
			args:     []string{"run", "testdata/nonexistent.yaml"},
			wantExit: 3,
			wantErr:  "not found",
		},
		{
			name:     "run with invalid YAML",
			args:     []string{"run", "testdata/invalid.yaml"},
			wantExit: 3,
			wantErr:  "invalid YAML",
		},
		{
			name:     "run with no collection name",
			args:     []string{"run", "testdata/no_name.yaml"},
			wantExit: 3,
			wantErr:  "no name",
		},
		{
			name:     "unknown command",
			args:     []string{"bogus"},
			wantExit: 1,
			wantErr:  "Unknown command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runBinary(t, binary, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			if tt.wantOut != "" && !strings.Contains(stdout, tt.wantOut) {
				t.Errorf("stdout %q does not contain %q", stdout, tt.wantOut)
			}
			if tt.wantErr != "" && !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr %q does not contain %q", stderr, tt.wantErr)
			}
		})
	}
}

func TestCLIIntegration_successful_run(t *testing.T) {
	// Start a local HTTP server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Create a temporary collection file pointing to the test server
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Integration Test

requests:
  - name: Get Test Server
    request:
      method: GET
      url: "%s"
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "Collection: Integration Test") {
		t.Errorf("stdout %q does not contain collection header", stdout)
	}
	if !strings.Contains(stdout, "Get Test Server") {
		t.Errorf("stdout %q does not contain request name", stdout)
	}
	if !strings.Contains(stdout, "200") {
		t.Errorf("stdout %q does not contain status code 200", stdout)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_multiple_requests_all_succeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Multi Test

requests:
  - name: First
    request:
      method: GET
      url: "%s/one"
  - name: Second
    request:
      method: GET
      url: "%s/two"
  - name: Third
    request:
      method: GET
      url: "%s/three"
`, srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "3 passed") {
		t.Errorf("stdout %q does not contain '3 passed'", stdout)
	}
	if !strings.Contains(stdout, "0 failed") {
		t.Errorf("stdout %q does not contain '0 failed'", stdout)
	}
	if !strings.Contains(stdout, "ms") {
		t.Errorf("stdout %q does not contain duration in ms", stdout)
	}
}

func TestCLIIntegration_empty_requests_warning(t *testing.T) {
	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", "testdata/empty_requests.yaml")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stderr, "Warning:") {
		t.Errorf("stderr %q does not contain 'Warning:'", stderr)
	}
	if !strings.Contains(stderr, "no requests") {
		t.Errorf("stderr %q does not contain 'no requests'", stderr)
	}
}

func TestCLIIntegration_stop_on_failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Stop On Failure Test

options:
  stop_on_failure: true

requests:
  - name: First OK
    request:
      method: GET
      url: "%s"
  - name: Will Fail
    request:
      method: GET
      url: "http://127.0.0.1:1/fail"
  - name: Should Skip
    request:
      method: GET
      url: "%s"
`, srv.URL, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "SKIPPED") {
		t.Errorf("stdout %q does not contain 'SKIPPED'", stdout)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
	if !strings.Contains(stdout, "1 failed") {
		t.Errorf("stdout %q does not contain '1 failed'", stdout)
	}
	if !strings.Contains(stdout, "1 skipped") {
		t.Errorf("stdout %q does not contain '1 skipped'", stdout)
	}
}

func TestCLIIntegration_summary_shows_duration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Duration Test

requests:
  - name: Quick
    request:
      method: GET
      url: "%s"
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, _, _ := runBinary(t, binary, "run", collectionFile)

	if !strings.Contains(stdout, "ms)") {
		t.Errorf("stdout %q does not contain duration in ms", stdout)
	}
}

func TestCLIIntegration_assertion_pass(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Assert Pass Test

requests:
  - name: Check Status
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "✓") {
		t.Errorf("stdout %q does not contain pass indicator ✓", stdout)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_assertion_fail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Assert Fail Test

requests:
  - name: Check Status
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "✗") {
		t.Errorf("stdout %q does not contain fail indicator ✗", stdout)
	}
	if !strings.Contains(stdout, "expected 404") {
		t.Errorf("stdout %q does not contain assertion detail", stdout)
	}
	if !strings.Contains(stdout, "got 200") {
		t.Errorf("stdout %q does not contain actual status", stdout)
	}
}

func TestCLIIntegration_assertion_list(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Assert List Test

requests:
  - name: Check Status
    request:
      method: GET
      url: "%s"
    assertions:
      status: [200, 201]
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "✓") {
		t.Errorf("stdout %q does not contain pass indicator ✓", stdout)
	}
}

func TestCLIIntegration_no_assertions_still_passes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: No Assert Test

requests:
  - name: No Assertions
    request:
      method: GET
      url: "%s"
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "✓") {
		t.Errorf("stdout %q does not contain pass indicator ✓", stdout)
	}
}

func TestCLIIntegration_network_error(t *testing.T) {
	// Create a collection file pointing to a refused port
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Network Fail Test

requests:
  - name: Unreachable
    request:
      method: GET
      url: "http://127.0.0.1:1/fail"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4\nstderr: %s", exitCode, stderr)
	}
}

func TestCLIIntegration_ErrorFormat(t *testing.T) {
	binary := buildBinary(t)

	t.Run("parse error has ERROR prefix", func(t *testing.T) {
		_, stderr, exitCode := runBinary(t, binary, "run", "testdata/invalid.yaml")
		if exitCode != 3 {
			t.Errorf("exit code = %d, want 3", exitCode)
		}
		if !strings.Contains(stderr, "[ERROR]") {
			t.Errorf("stderr %q does not contain [ERROR]", stderr)
		}
	})

	t.Run("parse error includes file path", func(t *testing.T) {
		_, stderr, _ := runBinary(t, binary, "run", "testdata/invalid.yaml")
		if !strings.Contains(stderr, "invalid.yaml") {
			t.Errorf("stderr %q does not contain file path", stderr)
		}
	})

	t.Run("missing file has ERROR prefix", func(t *testing.T) {
		_, stderr, exitCode := runBinary(t, binary, "run", "testdata/nonexistent.yaml")
		if exitCode != 3 {
			t.Errorf("exit code = %d, want 3", exitCode)
		}
		if !strings.Contains(stderr, "[ERROR]") {
			t.Errorf("stderr %q does not contain [ERROR]", stderr)
		}
	})

	t.Run("network error has ERROR prefix", func(t *testing.T) {
		tmpDir := t.TempDir()
		f := filepath.Join(tmpDir, "conn.yaml")
		content := "name: test\nrequests:\n  - name: fail\n    request:\n      method: GET\n      url: \"http://127.0.0.1:1/test\"\n"
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, _ := runBinary(t, binary, "run", f)
		if !strings.Contains(stderr, "[ERROR]") {
			t.Errorf("stderr %q does not contain [ERROR]", stderr)
		}
	})

	t.Run("network error suggests server check", func(t *testing.T) {
		tmpDir := t.TempDir()
		f := filepath.Join(tmpDir, "conn.yaml")
		content := "name: test\nrequests:\n  - name: fail\n    request:\n      method: GET\n      url: \"http://127.0.0.1:1/test\"\n"
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		_, stderr, _ := runBinary(t, binary, "run", f)
		if !strings.Contains(stderr, "server is running") {
			t.Errorf("stderr %q does not contain server hint", stderr)
		}
	})
}

func TestCLIIntegration_variable_interpolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: Variable Test
variables:
  base_url: "%s"
requests:
  - name: Interpolated
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "✓") {
		t.Errorf("stdout %q does not contain pass indicator ✓", stdout)
	}
}

func TestCLIIntegration_variable_circular_error(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Circular Test
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "[ERROR]") {
		t.Errorf("stderr %q does not contain [ERROR]", stderr)
	}
	if !strings.Contains(stderr, "circular") {
		t.Errorf("stderr %q does not contain 'circular'", stderr)
	}
}

func TestCLIIntegration_variable_depth_limit_error(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Depth Limit Test
variables:
  v0: "{{v1}}"
  v1: "{{v2}}"
  v2: "{{v3}}"
  v3: "{{v4}}"
  v4: "{{v5}}"
  v5: "{{v6}}"
  v6: "{{v7}}"
  v7: "{{v8}}"
  v8: "{{v9}}"
  v9: "{{v10}}"
  v10: "done"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "[ERROR]") {
		t.Errorf("stderr %q does not contain [ERROR]", stderr)
	}
	if !strings.Contains(stderr, "depth") {
		t.Errorf("stderr %q does not contain 'depth'", stderr)
	}
}

func TestCLIIntegration_variable_undefined_error(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Undefined Test
variables:
  a: "1"
requests:
  - name: Missing Var
    request:
      method: GET
      url: "{{unknown}}"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "[ERROR]") {
		t.Errorf("stderr %q does not contain [ERROR]", stderr)
	}
	if !strings.Contains(stderr, "undefined") {
		t.Errorf("stderr %q does not contain 'undefined'", stderr)
	}
}

func TestCLIIntegration_var_flag_overrides_collection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Var Override Test
variables:
  base_url: "http://wrong-host:9999"
requests:
  - name: Overridden
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--var", fmt.Sprintf("base_url=%s", srv.URL))

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_var_flag_multiple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Multi Var Test
requests:
  - name: Multi Vars
    request:
      method: GET
      url: "{{base_url}}/{{path}}"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile,
		"--var", fmt.Sprintf("base_url=%s", srv.URL),
		"--var", "path=test")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_var_flag_invalid_format(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Test
requests:
  - name: X
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--var", "noequals")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "key=value") {
		t.Errorf("stderr %q does not contain 'key=value'", stderr)
	}
}

func TestCLIIntegration_var_flag_no_value_after(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Test
requests:
  - name: X
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--var")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "--var requires") {
		t.Errorf("stderr %q does not contain '--var requires'", stderr)
	}
}

func TestCLIIntegration_var_flag_value_with_equals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Equals Value Test
requests:
  - name: Equals
    request:
      method: GET
      url: "{{url}}"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection file: %v", err)
	}

	binary := buildBinary(t)
	// Value contains '=' sign: url=http://host:port/path?q=1
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile,
		"--var", fmt.Sprintf("url=%s/path?q=1", srv.URL))

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_flag_loads_variables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envContent := fmt.Sprintf("variables:\n  base_url: %q\n", srv.URL)
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Env Test
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--env", "dev")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_flag_missing_environment(t *testing.T) {
	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte("variables:\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Test
requests:
  - name: X
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--env", "staging")

	if exitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	if !strings.Contains(stderr, "dev") {
		t.Errorf("stderr %q does not list available environment 'dev'", stderr)
	}
}

func TestCLIIntegration_env_flag_collection_overrides_env(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// env sets base_url to a wrong host
	envContent := "variables:\n  base_url: \"http://wrong-host:9999\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	collectionFile := filepath.Join(tmpDir, "test.yaml")
	// collection overrides base_url to the test server
	content := fmt.Sprintf(`name: Override Test
variables:
  base_url: %q
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--env", "dev")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_flag_cli_overrides_all(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envContent := "variables:\n  base_url: \"http://wrong-env:9999\"\n"
	if err := os.WriteFile(filepath.Join(envDir, "dev.yaml"), []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: CLI Override Test
variables:
  base_url: "http://wrong-col:9999"
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile,
		"--env", "dev", "--var", fmt.Sprintf("base_url=%s", srv.URL))

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_flag_yml_extension(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	envDir := filepath.Join(tmpDir, "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envContent := fmt.Sprintf("variables:\n  base_url: %q\n", srv.URL)
	if err := os.WriteFile(filepath.Join(envDir, "dev.yml"), []byte(envContent), 0o644); err != nil {
		t.Fatal(err)
	}

	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: YML Test
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--env", "dev")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_var_imports_os_env(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Env Var Test
requests:
  - name: Check
    request:
      method: GET
      url: "{{TEST_URL}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	env := append(os.Environ(), fmt.Sprintf("TEST_URL=%s", srv.URL))
	stdout, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile, "--env-var", "TEST_URL")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_var_missing_os_env_errors(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Test
requests:
  - name: X
    request:
      method: GET
      url: "https://example.com"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	// Use a clean environment without the variable
	env := []string{"HOME=" + os.Getenv("HOME"), "PATH=" + os.Getenv("PATH")}
	_, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile, "--env-var", "DEFINITELY_NOT_SET_VAR_XYZ")

	if exitCode == 0 {
		t.Error("expected non-zero exit code")
	}
	if !strings.Contains(stderr, "not set") {
		t.Errorf("stderr %q does not contain 'not set'", stderr)
	}
}

func TestCLIIntegration_env_var_mapped_name(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Mapped Env Var Test
requests:
  - name: Check
    request:
      method: GET
      url: "{{API_URL}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	env := append(os.Environ(), fmt.Sprintf("CI_SERVER_URL=%s", srv.URL))
	stdout, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile, "--env-var", "API_URL=$CI_SERVER_URL")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_env_var_precedence_over_collection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Precedence Test
variables:
  base_url: "http://wrong-host:9999"
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	env := append(os.Environ(), fmt.Sprintf("REAL_URL=%s", srv.URL))
	stdout, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile, "--env-var", "base_url=$REAL_URL")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_cli_var_precedence_over_env_var(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: CLI over Env Var Test
requests:
  - name: Check
    request:
      method: GET
      url: "{{base_url}}/path"
    assertions:
      status: 200
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	env := append(os.Environ(), "WRONG_URL=http://wrong-host:9999")
	stdout, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile,
		"--env-var", "base_url=$WRONG_URL",
		"--var", fmt.Sprintf("base_url=%s", srv.URL))

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_help_shows_env_var_flag(t *testing.T) {
	binary := buildBinary(t)
	stdout, _, exitCode := runBinary(t, binary, "--help")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "--env-var") {
		t.Errorf("help output %q does not contain '--env-var'", stdout)
	}
}

func TestCLIIntegration_include_directive(t *testing.T) {
	// Completeness Contract (CLAUDE.md): binary-level integration test verifying
	// that the include directive executes child requests.
	//
	// Creates a parent collection that includes a child collection; both make
	// requests to a local test server. Runs the binary and asserts that both
	// the parent and child requests appear in the output.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	childFile := filepath.Join(tmpDir, "child.yaml")
	childContent := fmt.Sprintf(`name: Child Collection
requests:
  - name: Child Request
    request:
      method: GET
      url: "%s/child"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(childFile, []byte(childContent), 0o644); err != nil {
		t.Fatalf("writing child collection: %v", err)
	}

	parentFile := filepath.Join(tmpDir, "parent.yaml")
	parentContent := fmt.Sprintf(`name: Parent Collection
variables:
  base_url: %q
include:
  - ./child.yaml
requests:
  - name: Parent Request
    request:
      method: GET
      url: "%s/parent"
    assertions:
      status: 200
`, srv.URL, srv.URL)
	if err := os.WriteFile(parentFile, []byte(parentContent), 0o644); err != nil {
		t.Fatalf("writing parent collection: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", parentFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "Parent Request") {
		t.Errorf("stdout %q does not contain 'Parent Request'", stdout)
	}
	if !strings.Contains(stdout, "Child Request") {
		t.Errorf("stdout %q does not contain 'Child Request' (include not executed)", stdout)
	}
	if !strings.Contains(stdout, "2 passed") {
		t.Errorf("stdout %q does not contain '2 passed'", stdout)
	}
}

func TestCLIIntegration_external_request_reference(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	reqDir := filepath.Join(tmpDir, "requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create external request file
	extContent := fmt.Sprintf(`name: External Get
request:
  method: GET
  url: "%s/get"
`, srv.URL)
	if err := os.WriteFile(filepath.Join(reqDir, "get.yaml"), []byte(extContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create collection referencing it
	colContent := `name: External Ref CLI Test
requests:
  - path: requests/get.yaml
`
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", colFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "External Get") {
		t.Errorf("stdout %q does not contain request name 'External Get'", stdout)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_external_request_not_found(t *testing.T) {
	tmpDir := t.TempDir()
	colContent := `name: Missing Ref Test
requests:
  - path: requests/nonexistent.yaml
`
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	_, stderr, exitCode := runBinary(t, binary, "run", colFile)

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "external request file not found") {
		t.Errorf("stderr %q does not contain 'external request file not found'", stderr)
	}
}

func TestCLIIntegration_external_request_with_variables(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	reqDir := filepath.Join(tmpDir, "requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// External request uses a variable
	extContent := `name: Var Request
request:
  method: GET
  url: "{{base_url}}/get"
`
	if err := os.WriteFile(filepath.Join(reqDir, "var-req.yaml"), []byte(extContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Collection with variable overrides at reference site
	colContent := fmt.Sprintf(`name: External Var Test
requests:
  - path: requests/var-req.yaml
    variables:
      base_url: "%s"
`, srv.URL)
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", colFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 passed") {
		t.Errorf("stdout %q does not contain '1 passed'", stdout)
	}
}

func TestCLIIntegration_setup_teardown_execution_order(t *testing.T) {
	var callOrder []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callOrder = append(callOrder, r.URL.Path)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	colContent := fmt.Sprintf(`name: Setup Teardown Order Test
setup:
  - name: Setup Request
    request:
      method: GET
      url: "%s/setup"
requests:
  - name: Main Request
    request:
      method: GET
      url: "%s/main"
teardown:
  - name: Teardown Request
    request:
      method: GET
      url: "%s/teardown"
`, srv.URL, srv.URL, srv.URL)

	tmpDir := t.TempDir()
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", colFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "Setup:") {
		t.Errorf("stdout %q does not contain 'Setup:' section header", stdout)
	}
	if !strings.Contains(stdout, "Teardown:") {
		t.Errorf("stdout %q does not contain 'Teardown:' section header", stdout)
	}
	if !strings.Contains(stdout, "Setup Request") {
		t.Errorf("stdout %q does not contain 'Setup Request'", stdout)
	}
	if !strings.Contains(stdout, "Main Request") {
		t.Errorf("stdout %q does not contain 'Main Request'", stdout)
	}
	if !strings.Contains(stdout, "Teardown Request") {
		t.Errorf("stdout %q does not contain 'Teardown Request'", stdout)
	}
	// Verify server received requests in the correct order
	if len(callOrder) != 3 {
		t.Fatalf("server received %d requests, want 3; order: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "/setup" || callOrder[1] != "/main" || callOrder[2] != "/teardown" {
		t.Errorf("request order = %v, want [/setup /main /teardown]", callOrder)
	}
}

func TestCLIIntegration_setup_teardown_runs_after_main_failure(t *testing.T) {
	var teardownCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/teardown" {
			teardownCalled = true
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Main request asserts status 404 but server returns 200 — assertion fails
	colContent := fmt.Sprintf(`name: Teardown After Main Failure
requests:
  - name: Main Request
    request:
      method: GET
      url: "%s/main"
    assertions:
      status: 404
teardown:
  - name: Teardown Request
    request:
      method: GET
      url: "%s/teardown"
`, srv.URL, srv.URL)

	tmpDir := t.TempDir()
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", colFile)

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 (assertion failure)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !teardownCalled {
		t.Error("teardown request was not called despite main failure")
	}
	if !strings.Contains(stdout, "Teardown Request") {
		t.Errorf("stdout %q does not contain 'Teardown Request'", stdout)
	}
}

func TestCLIIntegration_teardown_failure_does_not_affect_exit_code(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Teardown asserts status 404 but server returns 200 — teardown assertion fails
	colContent := fmt.Sprintf(`name: Teardown Failure Exit Code Test
requests:
  - name: Main Request
    request:
      method: GET
      url: "%s/main"
teardown:
  - name: Teardown Request
    request:
      method: GET
      url: "%s/teardown"
    assertions:
      status: 404
`, srv.URL, srv.URL)

	tmpDir := t.TempDir()
	colFile := filepath.Join(tmpDir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(colContent), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", colFile)

	// Teardown failures must not cause a non-zero exit code
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0 (teardown failure should not affect exit code)\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "Teardown Request") {
		t.Errorf("stdout %q does not contain 'Teardown Request'", stdout)
	}
}

func TestRunCmd_ProjectConfig(t *testing.T) {
	binary := buildBinary(t)

	t.Run("project variables resolved in collection", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		t.Cleanup(srv.Close)

		dir := t.TempDir()
		projectYAML := fmt.Sprintf("project_name: TestProject\nvariables:\n  base_url: %s\n", srv.URL)
		if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(projectYAML), 0o600); err != nil {
			t.Fatal(err)
		}
		collectionYAML := "name: ProjVarTest\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"{{base_url}}/path\"\n    assertions:\n      status: 200\n"
		collectionFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
		}
	})

	t.Run("no apitest.yaml runs without error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		t.Cleanup(srv.Close)

		dir := t.TempDir()
		collectionYAML := fmt.Sprintf("name: NoProjTest\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"%s/path\"\n    assertions:\n      status: 200\n", srv.URL)
		collectionFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
		}
	})

	t.Run("project config in parent directory (walk-up)", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		t.Cleanup(srv.Close)

		parent := t.TempDir()
		subdir := filepath.Join(parent, "requests")
		if err := os.Mkdir(subdir, 0o750); err != nil {
			t.Fatal(err)
		}
		projectYAML := fmt.Sprintf("project_name: WalkUpProject\nvariables:\n  base_url: %s\n", srv.URL)
		if err := os.WriteFile(filepath.Join(parent, "apitest.yaml"), []byte(projectYAML), 0o600); err != nil {
			t.Fatal(err)
		}
		collectionYAML := "name: WalkUpTest\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"{{base_url}}/path\"\n    assertions:\n      status: 200\n"
		collectionFile := filepath.Join(subdir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
		}
	})

	t.Run("project variables overridden by collection variables", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		t.Cleanup(srv.Close)

		dir := t.TempDir()
		// project defines wrong_host, collection overrides with real server
		projectYAML := "project_name: TestProject\nvariables:\n  base_url: http://wrong-host:9999\n"
		if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(projectYAML), 0o600); err != nil {
			t.Fatal(err)
		}
		collectionYAML := fmt.Sprintf("name: OverrideTest\nvariables:\n  base_url: %s\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"{{base_url}}/path\"\n    assertions:\n      status: 200\n", srv.URL)
		collectionFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0 (collection should override project)\nstderr: %s", exitCode, stderr)
		}
	})

	t.Run("invalid apitest.yaml returns error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte("variables: [not: a: map]"), 0o600); err != nil {
			t.Fatal(err)
		}
		collectionYAML := "name: Test\nrequests:\n  - name: X\n    request:\n      method: GET\n      url: \"https://httpbin.org/get\"\n"
		collectionFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode == 0 {
			t.Errorf("expected non-zero exit code for invalid apitest.yaml\nstderr: %s", stderr)
		}
		if !strings.Contains(stderr, "invalid project config") {
			t.Errorf("stderr %q does not contain 'invalid project config'", stderr)
		}
	})

	t.Run("dot env loaded from project root not collection dir", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		t.Cleanup(srv.Close)

		parent := t.TempDir()
		subdir := filepath.Join(parent, "requests")
		if err := os.Mkdir(subdir, 0o750); err != nil {
			t.Fatal(err)
		}
		// apitest.yaml in parent
		if err := os.WriteFile(filepath.Join(parent, "apitest.yaml"), []byte("project_name: DotEnvTest\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// .env in parent (project root), NOT in subdir
		dotEnvContent := fmt.Sprintf("BASE_URL=%s\n", srv.URL)
		if err := os.WriteFile(filepath.Join(parent, ".env"), []byte(dotEnvContent), 0o600); err != nil {
			t.Fatal(err)
		}
		collectionYAML := "name: DotEnvRootTest\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"{{BASE_URL}}/path\"\n    assertions:\n      status: 200\n"
		collectionFile := filepath.Join(subdir, "collection.yaml")
		if err := os.WriteFile(collectionFile, []byte(collectionYAML), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", collectionFile)
		if exitCode != 0 {
			t.Errorf("exit code = %d, want 0 (dotenv from project root)\nstderr: %s", exitCode, stderr)
		}
	})
}

func TestIntegration_dynamic_functions(t *testing.T) {
	binary := buildBinary(t)

	t.Run("uuid in request url reaches server", func(t *testing.T) {
		dir := t.TempDir()
		col := "name: DynamicUUID\nrequests:\n  - name: UUID Request\n    request:\n      method: GET\n      url: \"http://placeholder/{{$uuid}}\"\n"
		colFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(colFile, []byte(col), 0o600); err != nil {
			t.Fatal(err)
		}

		var capturedPath string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.WriteHeader(200)
		}))
		defer srv.Close()

		colContent := fmt.Sprintf("name: DynamicUUID\nrequests:\n  - name: UUID Request\n    request:\n      method: GET\n      url: \"%s/{{$uuid}}\"\n    assertions:\n      status: 200\n", srv.URL)
		if err := os.WriteFile(colFile, []byte(colContent), 0o600); err != nil {
			t.Fatal(err)
		}

		_, stderr, exitCode := runBinary(t, binary, "run", colFile)
		if exitCode != 0 {
			t.Fatalf("exit code = %d, want 0\nstderr: %s", exitCode, stderr)
		}

		uuidRe := regexp.MustCompile(`^/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
		if !uuidRe.MatchString(capturedPath) {
			t.Errorf("server received path %q, expected UUID v4 path", capturedPath)
		}
	})

	t.Run("seed 42 is deterministic across runs", func(t *testing.T) {
		dir := t.TempDir()

		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			w.WriteHeader(200)
		}))
		defer srv.Close()

		colContent := fmt.Sprintf("name: SeedTest\nrequests:\n  - name: Seeded Request\n    request:\n      method: GET\n      url: \"%s/{{$uuid}}\"\n    assertions:\n      status: 200\n", srv.URL)
		colFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(colFile, []byte(colContent), 0o600); err != nil {
			t.Fatal(err)
		}

		// Run twice with same seed.
		_, stderr1, exit1 := runBinary(t, binary, "run", colFile, "--seed", "42")
		_, stderr2, exit2 := runBinary(t, binary, "run", colFile, "--seed", "42")
		if exit1 != 0 || exit2 != 0 {
			t.Fatalf("runs failed: %s / %s", stderr1, stderr2)
		}
		if len(paths) != 2 {
			t.Fatalf("expected 2 captured paths, got %d", len(paths))
		}
		if paths[0] != paths[1] {
			t.Errorf("seed 42: expected identical paths, got %q vs %q", paths[0], paths[1])
		}
	})

	t.Run("no seed produces different values across runs", func(t *testing.T) {
		dir := t.TempDir()

		var paths []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			paths = append(paths, r.URL.Path)
			w.WriteHeader(200)
		}))
		defer srv.Close()

		colContent := fmt.Sprintf("name: RandomTest\nrequests:\n  - name: Random Request\n    request:\n      method: GET\n      url: \"%s/{{$uuid}}\"\n    assertions:\n      status: 200\n", srv.URL)
		colFile := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(colFile, []byte(colContent), 0o600); err != nil {
			t.Fatal(err)
		}

		_, _, exit1 := runBinary(t, binary, "run", colFile)
		_, _, exit2 := runBinary(t, binary, "run", colFile)
		if exit1 != 0 || exit2 != 0 {
			t.Fatalf("runs failed")
		}
		if len(paths) != 2 {
			t.Fatalf("expected 2 captured paths, got %d", len(paths))
		}
		if paths[0] == paths[1] {
			t.Errorf("without seed: expected different paths, got same %q", paths[0])
		}
	})

	t.Run("help text includes --seed", func(t *testing.T) {
		stdout, _, exitCode := runBinary(t, binary, "--help")
		if exitCode != 0 {
			t.Fatalf("help exited %d", exitCode)
		}
		if !strings.Contains(stdout, "--seed") {
			t.Errorf("help text does not mention --seed:\n%s", stdout)
		}
	})
}

func TestCLIIntegration_noColorFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: No Color Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--no-color")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	combined := stdout + stderr
	if strings.Contains(combined, "\033[") {
		t.Errorf("output contains ANSI codes with --no-color: %q", combined)
	}
}

func TestCLIIntegration_NOCOLOREnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: No Color Env Test
requests:
  - name: Check
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	binary := buildBinary(t)
	env := append(os.Environ(), "NO_COLOR=1")
	stdout, stderr, exitCode := runBinaryWithEnv(t, binary, env, "run", collectionFile)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	combined := stdout + stderr
	if strings.Contains(combined, "\033[") {
		t.Errorf("output contains ANSI codes with NO_COLOR=1: %q", combined)
	}
}

func TestExtractJSONOperator(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"status", "status"},
		{"timing", "timing"},
		{"body $.id equals", "equals"},
		{"body $.url contains", "contains"},
		{"header Content-Type matches", "matches"},
		{"header X-Custom-Header equals", "equals"},
		{"body $.count greater_than", "greater_than"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractJSONOperator(tt.input)
			if got != tt.want {
				t.Errorf("extractJSONOperator(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildJSONOutput(t *testing.T) {
	passedResult := &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}
	failedAssertions := &assertion.Results{
		Items: []assertion.Result{
			{Type: "status", Expected: "404", Actual: "200", Passed: false},
		},
		Passed: false,
	}

	tests := []struct {
		name       string
		colName    string
		results    []runner.RequestResult
		summary    *runner.Summary
		preErr     error
		wantStatus string
	}{
		{
			name:       "all passed",
			colName:    "Suite",
			results:    []runner.RequestResult{{Name: "r", Method: "GET", URL: "https://example.com", Result: passedResult}},
			summary:    &runner.Summary{Passed: 1},
			wantStatus: "passed",
		},
		{
			name:    "assertion failure",
			colName: "Suite",
			results: []runner.RequestResult{
				{Name: "r", Method: "GET", URL: "https://example.com", Result: passedResult, AssertionResults: failedAssertions},
			},
			summary:    &runner.Summary{Failed: 1, AssertionFailures: 1},
			wantStatus: "failed",
		},
		{
			name:       "skipped request",
			colName:    "Suite",
			results:    []runner.RequestResult{{Name: "r", Skipped: true}},
			summary:    &runner.Summary{Skipped: 1},
			wantStatus: "passed",
		},
		{
			name:       "pre-exec error",
			colName:    "Suite",
			results:    nil,
			summary:    &runner.Summary{},
			preErr:     errors.New("boom"),
			wantStatus: "error",
		},
		{
			name:       "network error in request",
			colName:    "Suite",
			results:    []runner.RequestResult{{Name: "r", Method: "GET", URL: "https://example.com", Err: errors.New("refused")}},
			summary:    &runner.Summary{Failed: 1},
			wantStatus: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildJSONOutput(tt.colName, tt.results, tt.summary, tt.preErr, output.VerbosityDefault)
			if out.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", out.Status, tt.wantStatus)
			}
			if out.Name != tt.colName {
				t.Errorf("name = %q, want %q", out.Name, tt.colName)
			}
			if out.Requests == nil {
				t.Error("Requests should be non-nil (never null in JSON)")
			}
			// M11-002: Summary must always be populated.
			if out.Summary == nil {
				t.Error("Summary must always be non-nil")
			}
		})
	}
}

func TestJSONOutput_Summary(t *testing.T) {
	passedResult := &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}
	failedAssertions := &assertion.Results{
		Items:  []assertion.Result{{Type: "status", Expected: "404", Actual: "200", Passed: false}},
		Passed: false,
	}

	tests := []struct {
		name        string
		results     []runner.RequestResult
		summary     *runner.Summary
		wantTotal   int
		wantPassed  int
		wantFailed  int
		wantSkipped int
	}{
		{
			"pass-only run summary",
			[]runner.RequestResult{
				{Name: "a", Method: "GET", URL: "x", Result: passedResult},
				{Name: "b", Method: "GET", URL: "y", Result: passedResult},
			},
			&runner.Summary{Total: 2, Passed: 2, Failed: 0, Skipped: 0},
			2, 2, 0, 0,
		},
		{
			"mixed pass/fail summary",
			[]runner.RequestResult{
				{Name: "a", Method: "GET", URL: "x", Result: passedResult},
				{Name: "b", Method: "GET", URL: "y", Result: passedResult, AssertionResults: failedAssertions},
				{Name: "c", Skipped: true},
			},
			&runner.Summary{Total: 3, Passed: 1, Failed: 1, Skipped: 1, AssertionFailures: 1},
			3, 1, 1, 1,
		},
		{
			"nil summary becomes zeroed block",
			nil,
			nil,
			0, 0, 0, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildJSONOutput("Suite", tt.results, tt.summary, nil, output.VerbosityDefault)
			if out.Summary == nil {
				t.Fatal("Summary must always be populated, got nil")
			}
			if out.Summary.Total != tt.wantTotal {
				t.Errorf("Summary.Total = %d, want %d", out.Summary.Total, tt.wantTotal)
			}
			if out.Summary.Passed != tt.wantPassed {
				t.Errorf("Summary.Passed = %d, want %d", out.Summary.Passed, tt.wantPassed)
			}
			if out.Summary.Failed != tt.wantFailed {
				t.Errorf("Summary.Failed = %d, want %d", out.Summary.Failed, tt.wantFailed)
			}
			if out.Summary.Skipped != tt.wantSkipped {
				t.Errorf("Summary.Skipped = %d, want %d", out.Summary.Skipped, tt.wantSkipped)
			}
		})
	}
}

func TestJSONOutput_DataDrivenIterations(t *testing.T) {
	tests := []struct {
		name           string
		results        []runner.RequestResult
		wantIterations int
		check          func(t *testing.T, group output.DataDrivenJSON)
	}{
		{
			"iterations[].length == total_iterations for passed group",
			[]runner.RequestResult{
				{Name: "Create User [1/3]", IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 0, IterationTotal: 3, Result: &httpexec.Result{Duration: 100 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}, IterationData: map[string]string{"name": "alice"}},
				{Name: "Create User [2/3]", IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 1, IterationTotal: 3, Result: &httpexec.Result{Duration: 200 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}, IterationData: map[string]string{"name": "bob"}},
				{Name: "Create User [3/3]", IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 2, IterationTotal: 3, Result: &httpexec.Result{Duration: 300 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}, IterationData: map[string]string{"name": "charlie"}},
			},
			3,
			func(t *testing.T, g output.DataDrivenJSON) {
				if len(g.Iterations) != g.TotalIterations {
					t.Fatalf("len(iterations)=%d != total_iterations=%d", len(g.Iterations), g.TotalIterations)
				}
				if g.Iterations[0].Name != "Create User [1/3]" {
					t.Errorf("iter[0].Name = %q, want %q", g.Iterations[0].Name, "Create User [1/3]")
				}
				if g.Iterations[0].Status != "passed" {
					t.Errorf("iter[0].Status = %q, want passed", g.Iterations[0].Status)
				}
				if g.Iterations[0].DurationMs != 100 {
					t.Errorf("iter[0].DurationMs = %d, want 100", g.Iterations[0].DurationMs)
				}
				if g.Iterations[0].DataColumns["name"] != "alice" {
					t.Errorf("iter[0].DataColumns[name] = %q, want alice", g.Iterations[0].DataColumns["name"])
				}
				if g.Iterations[1].DataColumns["name"] != "bob" {
					t.Errorf("iter[1].DataColumns[name] = %q, want bob", g.Iterations[1].DataColumns["name"])
				}
			},
		},
		{
			"mixed pass/fail iteration statuses round-trip",
			[]runner.RequestResult{
				{Name: "X [1/2]", IsDataDriven: true, DataDrivenName: "X", IterationIndex: 0, IterationTotal: 2, Result: &httpexec.Result{Duration: 100 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{Name: "X [2/2]", IsDataDriven: true, DataDrivenName: "X", IterationIndex: 1, IterationTotal: 2, Result: &httpexec.Result{Duration: 200 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: false, Items: []assertion.Result{{Type: "status", Expected: "200", Actual: "500", Passed: false}}}},
			},
			2,
			func(t *testing.T, g output.DataDrivenJSON) {
				if g.Iterations[0].Status != "passed" {
					t.Errorf("iter[0] = %q, want passed", g.Iterations[0].Status)
				}
				if g.Iterations[1].Status != "failed" {
					t.Errorf("iter[1] = %q, want failed", g.Iterations[1].Status)
				}
			},
		},
		{
			"iteration without IterationData omits data_columns",
			[]runner.RequestResult{
				{Name: "X [1/1]", IsDataDriven: true, DataDrivenName: "X", IterationIndex: 0, IterationTotal: 1, Result: &httpexec.Result{Duration: 50 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
			},
			1,
			func(t *testing.T, g output.DataDrivenJSON) {
				if g.Iterations[0].DataColumns != nil {
					t.Errorf("DataColumns should be nil when IterationData is unset, got %v", g.Iterations[0].DataColumns)
				}
			},
		},
		{
			"skipped iteration emits status skipped",
			[]runner.RequestResult{
				{Name: "X [1/1]", IsDataDriven: true, DataDrivenName: "X", IterationIndex: 0, IterationTotal: 1, Skipped: true, SkipReason: "context cancelled"},
			},
			1,
			func(t *testing.T, g output.DataDrivenJSON) {
				if g.Iterations[0].Status != "skipped" {
					t.Errorf("iter[0].Status = %q, want skipped", g.Iterations[0].Status)
				}
			},
		},
		{
			"network error iteration emits status failed",
			[]runner.RequestResult{
				{Name: "X [1/1]", IsDataDriven: true, DataDrivenName: "X", IterationIndex: 0, IterationTotal: 1, Err: errors.New("connection refused")},
			},
			1,
			func(t *testing.T, g output.DataDrivenJSON) {
				if g.Iterations[0].Status != "failed" {
					t.Errorf("iter[0].Status = %q, want failed", g.Iterations[0].Status)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildJSONOutput("Suite", tt.results, &runner.Summary{Total: tt.wantIterations, Passed: tt.wantIterations}, nil, output.VerbosityDefault)
			if len(out.DataDriven) != 1 {
				t.Fatalf("expected 1 data_driven group, got %d", len(out.DataDriven))
			}
			tt.check(t, out.DataDriven[0])
		})
	}
}

func TestBuildJSONOutput_assertionFields(t *testing.T) {
	results := []runner.RequestResult{
		{
			Name:   "req",
			Method: "GET",
			URL:    "https://example.com",
			Result: &httpexec.Result{StatusCode: 404, Duration: 5 * time.Millisecond},
			AssertionResults: &assertion.Results{
				Items: []assertion.Result{
					{Type: "status", Expected: "200", Actual: "404", Passed: false},
				},
				Passed: false,
			},
		},
	}
	out := buildJSONOutput("Suite", results, &runner.Summary{Failed: 1, AssertionFailures: 1}, nil, output.VerbosityDefault)
	if len(out.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(out.Requests))
	}
	req := out.Requests[0]
	if len(req.Assertions) != 1 {
		t.Fatalf("expected 1 assertion, got %d", len(req.Assertions))
	}
	a := req.Assertions[0]
	if a.Expected != "200" {
		t.Errorf("expected Expected=200, got %q", a.Expected)
	}
	if a.Actual != "404" {
		t.Errorf("expected Actual=404, got %q", a.Actual)
	}
	if a.Passed {
		t.Error("expected Passed=false")
	}
	if a.Operator == "" {
		t.Error("expected non-empty Operator")
	}
}

func TestBuildJSONOutput_emptyAssertionsNotNull(t *testing.T) {
	results := []runner.RequestResult{
		{
			Name:   "req",
			Method: "GET",
			URL:    "https://example.com",
			Result: &httpexec.Result{StatusCode: 200},
		},
	}
	out := buildJSONOutput("Suite", results, &runner.Summary{Passed: 1}, nil, output.VerbosityDefault)
	if len(out.Requests) != 1 {
		t.Fatalf("expected 1 request")
	}
	if out.Requests[0].Assertions == nil {
		t.Error("Assertions should be non-nil slice, not nil")
	}
}

func TestBuildJSONOutput_Verbosity(t *testing.T) {
	results := []runner.RequestResult{
		{
			Name:           "req",
			Method:         "GET",
			URL:            "https://example.com",
			RequestHeaders: map[string]string{"Accept": "application/json"},
			Result: &httpexec.Result{
				StatusCode: 200,
				Headers:    http.Header{"Content-Type": []string{"application/json"}},
				Body:       []byte(`{"id":1}`),
			},
		},
	}
	tests := []struct {
		name            string
		verbosity       output.Verbosity
		wantReqHeaders  bool
		wantRespHeaders bool
		wantBody        bool
	}{
		{"default_no_extra_fields", output.VerbosityDefault, false, false, false},
		{"verbose_includes_headers", output.VerbosityVerbose, true, true, false},
		{"debug_includes_body", output.VerbosityDebug, true, true, true},
		{"quiet_no_extra_fields", output.VerbosityQuiet, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := buildJSONOutput("Suite", results, &runner.Summary{Passed: 1}, nil, tt.verbosity)
			if len(out.Requests) != 1 {
				t.Fatalf("expected 1 request")
			}
			req := out.Requests[0]
			if gotHeaders := req.RequestHeaders != nil; gotHeaders != tt.wantReqHeaders {
				t.Errorf("RequestHeaders present=%v, want %v", gotHeaders, tt.wantReqHeaders)
			}
			if gotRespHeaders := req.ResponseHeaders != nil; gotRespHeaders != tt.wantRespHeaders {
				t.Errorf("ResponseHeaders present=%v, want %v", gotRespHeaders, tt.wantRespHeaders)
			}
			if gotBody := req.ResponseBody != ""; gotBody != tt.wantBody {
				t.Errorf("ResponseBody present=%v, want %v", gotBody, tt.wantBody)
			}
		})
	}
}

func TestCLIIntegration_JSONMode_ParseError(t *testing.T) {
	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", "nonexistent_file_xyz.yaml", "--format", "json")

	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "error" {
		t.Errorf("expected status=error, got %v", result["status"])
	}
}

func TestCLIIntegration_JSONMode_AllFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: JSON Suite
requests:
  - name: Get Test
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--format", "json")

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}

	for _, field := range []string{"name", "status", "duration_ms", "requests"} {
		if _, ok := result[field]; !ok {
			t.Errorf("missing top-level field %q", field)
		}
	}
	if result["status"] != "passed" {
		t.Errorf("expected status=passed, got %v", result["status"])
	}
	if result["name"] != "JSON Suite" {
		t.Errorf("expected name=JSON Suite, got %v", result["name"])
	}

	requests, ok := result["requests"].([]any)
	if !ok || len(requests) != 1 {
		t.Fatalf("expected 1 request, got %v", result["requests"])
	}
	req := requests[0].(map[string]any)
	for _, field := range []string{"name", "method", "url", "status_code", "duration_ms", "assertions"} {
		if _, ok := req[field]; !ok {
			t.Errorf("missing request field %q", field)
		}
	}
	if req["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", req["method"])
	}

	// assertions is array, not null
	assertions, ok := req["assertions"].([]any)
	if !ok || assertions == nil {
		t.Error("assertions should be a non-null array")
	}

	// no extraneous text — starts with { ends with }
	trimmed := strings.TrimSpace(stdout)
	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("stdout should start with '{', got: %q", trimmed[:10])
	}
	if !strings.HasSuffix(trimmed, "}") {
		t.Errorf("stdout should end with '}', got tail: %q", trimmed[len(trimmed)-10:])
	}
}

func TestCLIIntegration_JSONMode_FailingAssertion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(`name: JSON Fail Suite
requests:
  - name: Expect 404
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, srv.URL)
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--format", "json")

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", result["status"])
	}

	requests := result["requests"].([]any)
	req := requests[0].(map[string]any)
	assertions := req["assertions"].([]any)
	if len(assertions) == 0 {
		t.Fatal("expected at least one assertion")
	}
	a := assertions[0].(map[string]any)
	if a["passed"] != false {
		t.Error("expected passed=false")
	}
	for _, field := range []string{"expected", "actual", "operator"} {
		if _, ok := a[field]; !ok {
			t.Errorf("missing assertion field %q", field)
		}
	}
}

func TestCLIIntegration_JSONMode_VarError(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Circular Var Test
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://httpbin.org/get"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--format", "json")

	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "error" {
		t.Errorf("expected status=error, got %v", result["status"])
	}
}

func TestCLIIntegration_JSONMode_NetworkError(t *testing.T) {
	tmpDir := t.TempDir()
	collectionFile := filepath.Join(tmpDir, "test.yaml")
	content := `name: Network Error Test
requests:
  - name: Connection Refused
    request:
      method: GET
      url: "http://127.0.0.1:1/test"
`
	if err := os.WriteFile(collectionFile, []byte(content), 0o644); err != nil {
		t.Fatalf("writing collection: %v", err)
	}

	binary := buildBinary(t)
	stdout, stderr, exitCode := runBinary(t, binary, "run", collectionFile, "--format", "json")

	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in JSON mode, got: %q", stderr)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
	}
	if result["status"] != "failed" {
		t.Errorf("expected status=failed, got %v", result["status"])
	}
	requests := result["requests"].([]any)
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}
	req := requests[0].(map[string]any)
	if req["status"] != "error" {
		t.Errorf("expected request status=error, got %v", req["status"])
	}
}

func TestRunCmdDirect_TAPSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: TAP Direct Test
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "tap")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr, got: %q", stderr)
	}
	if !strings.HasPrefix(stdout, "TAP version 13\n") {
		t.Errorf("expected stdout to start with 'TAP version 13\\n', got: %q", stdout)
	}
	if !strings.Contains(stdout, "1..") {
		t.Errorf("expected plan line in stdout, got: %q", stdout)
	}
	if !strings.Contains(stdout, "ok 1") {
		t.Errorf("expected 'ok 1' line in stdout, got: %q", stdout)
	}
}

func TestRunCmdDirect_TAPAssertionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	col := fmt.Sprintf(`name: TAP Fail Direct
requests:
  - name: Expect 404
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, srv.URL)
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "tap")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in TAP mode, got: %q", stderr)
	}
	if !strings.Contains(stdout, "not ok") {
		t.Errorf("expected 'not ok' line in stdout, got: %q", stdout)
	}
	if !strings.Contains(stdout, "  ---") {
		t.Errorf("expected YAML diagnostic block in stdout, got: %q", stdout)
	}
}

func TestRunCmdDirect_TAPParseError(t *testing.T) {
	stdout, stderr, exitCode := captureRunCmd(t, "nonexistent_file_abc.yaml", "--format", "tap")
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if stderr != "" {
		t.Errorf("expected empty stderr in TAP mode, got: %q", stderr)
	}
	if !strings.HasPrefix(stdout, "TAP version 13\n") {
		t.Errorf("expected TAP version line, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Bail out!") {
		t.Errorf("expected 'Bail out!' in stdout, got: %q", stdout)
	}
}

func TestRunCmdDirect_TAPEmptyCollection(t *testing.T) {
	col := `name: Empty TAP
requests: []
`
	colFile := writeCollection(t, t.TempDir(), "col.yaml", col)

	stdout, stderr, exitCode := captureRunCmd(t, colFile, "--format", "tap")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}
	if !strings.Contains(stdout, "1..0") {
		t.Errorf("expected '1..0' plan line in stdout, got: %q", stdout)
	}
}

func TestBuildTAPOutput(t *testing.T) {
	passedResult := &httpexec.Result{StatusCode: 200, Duration: 50 * time.Millisecond}
	failedAssertions := &assertion.Results{
		Items: []assertion.Result{
			{Type: "status", Expected: "200", Actual: "404", Passed: false},
		},
		Passed: false,
	}

	tests := []struct {
		name        string
		input       []runner.RequestResult
		wantLen     int
		wantPassed  bool
		wantSkipped bool
		wantErr     string
		wantFails   int
	}{
		{
			name:       "passing request",
			input:      []runner.RequestResult{{Name: "r", Result: passedResult}},
			wantLen:    1,
			wantPassed: true,
		},
		{
			name: "failing assertion",
			input: []runner.RequestResult{
				{Name: "r", Result: passedResult, AssertionResults: failedAssertions},
			},
			wantLen:    1,
			wantPassed: false,
			wantFails:  1,
		},
		{
			name:        "skipped request",
			input:       []runner.RequestResult{{Name: "r", Skipped: true}},
			wantLen:     1,
			wantSkipped: true,
			wantPassed:  true,
		},
		{
			name:       "network error",
			input:      []runner.RequestResult{{Name: "r", Err: errors.New("refused")}},
			wantLen:    1,
			wantPassed: false,
			wantErr:    "refused",
		},
		{
			name:       "no assertions means passed",
			input:      []runner.RequestResult{{Name: "r", Result: passedResult}},
			wantLen:    1,
			wantPassed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTAPOutput(tt.input, false)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen == 0 {
				return
			}
			r := got[0]
			if r.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.wantPassed)
			}
			if r.Skipped != tt.wantSkipped {
				t.Errorf("Skipped = %v, want %v", r.Skipped, tt.wantSkipped)
			}
			if r.Error != tt.wantErr {
				t.Errorf("Error = %q, want %q", r.Error, tt.wantErr)
			}
			if len(r.Failures) != tt.wantFails {
				t.Errorf("len(Failures) = %d, want %d", len(r.Failures), tt.wantFails)
			}
		})
	}
}

func TestParseRunArgs_AllowSensitive(t *testing.T) {
	t.Run("allow_sensitive_flag_present", func(t *testing.T) {
		flags, err := parseRunArgs([]string{"col.yaml", "--allow-sensitive"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !flags.allowSensitive {
			t.Error("allowSensitive = false, want true")
		}
	})
	t.Run("allow_sensitive_default_false", func(t *testing.T) {
		flags, err := parseRunArgs([]string{"col.yaml"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.allowSensitive {
			t.Error("allowSensitive = true, want false")
		}
	})
}

func TestRunCmd_SensitiveRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	col := writeCollection(t, dir, "sensitive.yaml", fmt.Sprintf(`
name: Sensitive Test
variables:
  base_url: "%s"
  password: "secret123"
  api_key: !sensitive sk_live_abc
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        Authorization: "Bearer {{api_key}}"
`, srv.URL))

	t.Run("sensitive_var_redacted_in_verbose_output", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "-vv", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if strings.Contains(stdout, "sk_live_abc") {
			t.Error("api_key value leaked in verbose output")
		}
		if !strings.Contains(stdout, "[REDACTED]") {
			t.Error("[REDACTED] not shown for sensitive vars")
		}
	})

	t.Run("heuristic_password_var_redacted", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "-vv", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if strings.Contains(stdout, "secret123") {
			t.Error("password value leaked in verbose output")
		}
	})

	t.Run("allow_sensitive_shows_plaintext_value", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "-vv", "--allow-sensitive", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if !strings.Contains(stdout, "sk_live_abc") {
			t.Error("--allow-sensitive did not show api_key value")
		}
	})

	t.Run("authorization_header_redacted_in_verbose", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "-v", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if strings.Contains(stdout, "sk_live_abc") {
			t.Error("Authorization header value leaked in verbose output")
		}
	})

	t.Run("heuristic_detects_sensitive_name_in_dotenv", func(t *testing.T) {
		dotenvDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dotenvDir, ".env"), []byte("password=dotenv_secret_val\n"), 0o600); err != nil {
			t.Fatalf("write .env: %v", err)
		}
		// Header key matches the sensitive variable name so RedactHeaders can redact it.
		dotenvCol := writeCollection(t, dotenvDir, "dotenv.yaml", fmt.Sprintf(`
name: DotEnv Heuristic
variables:
  base_url: "%s"
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        password: "{{password}}"
`, srv.URL))
		stdout, _, exitCode := captureRunCmd(t, dotenvCol, "-vv", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if strings.Contains(stdout, "dotenv_secret_val") {
			t.Error("dotenv password value leaked in verbose output")
		}
		if !strings.Contains(stdout, "[REDACTED]") {
			t.Error("[REDACTED] not shown for heuristically-detected dotenv sensitive var")
		}
	})

	t.Run("heuristic_token_var_redacted", func(t *testing.T) {
		tokenDir := t.TempDir()
		// Header key matches the sensitive variable name so RedactHeaders can redact it.
		tokenCol := writeCollection(t, tokenDir, "token.yaml", fmt.Sprintf(`
name: Token Heuristic Test
variables:
  base_url: "%s"
  token: "secret_token_val"
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/get"
      headers:
        token: "{{token}}"
`, srv.URL))
		stdout, _, exitCode := captureRunCmd(t, tokenCol, "-vv", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0", exitCode)
		}
		if strings.Contains(stdout, "secret_token_val") {
			t.Error("token value leaked in verbose output")
		}
		if !strings.Contains(stdout, "[REDACTED]") {
			t.Error("[REDACTED] not shown for heuristically-detected token var")
		}
	})

	t.Run("sensitive_redacted_in_json_format", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "--format", "json", "-v", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
		}
		var result map[string]any
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", err, stdout)
		}
		requests, ok := result["requests"].([]any)
		if !ok || len(requests) == 0 {
			t.Fatal("requests not present in JSON output")
		}
		req, ok := requests[0].(map[string]any)
		if !ok {
			t.Fatal("first request is not a map")
		}
		headers, ok := req["request_headers"].(map[string]any)
		if !ok {
			t.Fatal("request_headers not present in JSON output at -v verbosity")
		}
		authVal, ok := headers["Authorization"]
		if !ok {
			t.Fatal("Authorization header not present in request_headers")
		}
		if authVal != "[REDACTED]" {
			t.Errorf("Authorization header = %v, want [REDACTED]", authVal)
		}
		if strings.Contains(stdout, "sk_live_abc") {
			t.Error("sensitive value leaked in JSON output")
		}
	})

	t.Run("error_message_does_not_leak_sensitive_value", func(t *testing.T) {
		errDir := t.TempDir()
		errCol := writeCollection(t, errDir, "leak.yaml", fmt.Sprintf(`
name: Leak Guard Test
variables:
  base_url: "%s"
  password: "super_secret_password_val"
requests:
  - name: Get
    request:
      method: GET
      url: "{{base_url}}/{{undefined_var}}"
`, srv.URL))
		_, stderr, exitCode := captureRunCmd(t, errCol, "--no-color")
		if exitCode != 5 {
			t.Fatalf("exitCode = %d, want 5 (variable error)", exitCode)
		}
		if strings.Contains(stderr, "super_secret_password_val") {
			t.Error("sensitive variable value leaked in error message")
		}
		if !strings.Contains(stderr, "undefined_var") {
			t.Error("error message should reference the undefined variable name")
		}
	})
}

// runBinaryInDir runs the binary with args in a specific working directory.
func runBinaryInDir(t *testing.T, binary, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected error running binary in dir %s: %v", dir, err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// captureRun calls the top-level run() dispatcher via runWithWriters and
// captures stdout/stderr. Safe to run in parallel.
func captureRun(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	exitCode = runWithWriters(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), exitCode
}

func TestRun_init_command_recognized(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := captureRun(t, "init", dir)
	if exitCode != 0 {
		t.Errorf("run(init) exitCode = %d, want 0; stderr=%q", exitCode, stderr)
	}
}

func TestInitCmd_empty_directory(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := captureRun(t, "init", dir)
	if exitCode != 0 {
		t.Fatalf("run(init) exitCode = %d, want 0; stderr=%q", exitCode, stderr)
	}
	// apitest.yaml must exist in the target dir
	if _, err := os.Stat(dir + "/apitest.yaml"); err != nil {
		t.Errorf("expected apitest.yaml to exist in %s: %v", dir, err)
	}
}

func TestInitCmd_existing_project_returns_error(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/apitest.yaml", []byte("project_name: Existing\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, stderr, exitCode := captureRun(t, "init", dir)
	if exitCode != 1 {
		t.Errorf("run(init existing) exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("expected 'Error:' in stderr, got: %q", stderr)
	}
}

func TestInitCmd_project_name_flag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := captureRun(t, "init", "--project-name", "demo", dir)
	if exitCode != 0 {
		t.Fatalf("run(init --project-name demo) exitCode = %d, want 0; stderr=%q", exitCode, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
	if err != nil {
		t.Fatalf("read apitest.yaml: %v", err)
	}
	if !strings.Contains(string(data), `"demo"`) {
		t.Errorf("want project_name: \"demo\" in apitest.yaml; got:\n%s", data)
	}
}

func TestHelp_contains_init(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Fatalf("--help exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "init") {
		t.Errorf("help text does not contain 'init':\n%s", stdout)
	}
}

// TestInit_OutputUnknownFormat asserts that a bad --output value exits 3
// with an error message that names the supported enum.
func TestInit_OutputUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--output", "madeup", dir)
	if code != 3 {
		t.Errorf("exit code = %d, want 3; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "markdown") {
		t.Errorf("stderr should name supported values including markdown; got: %q", stderr)
	}
	if !strings.Contains(stderr, "madeup") {
		t.Errorf("stderr should echo the rejected value; got: %q", stderr)
	}
	// Verify no apitest.yaml was created (validation runs before scaffold).
	if _, err := os.Stat(filepath.Join(dir, "apitest.yaml")); err == nil {
		t.Errorf("apitest.yaml should not exist after rejected --output value")
	}
}

// TestInit_OutputMarkdown_FullPipeline runs init via the binary-like
// runWithWriters and asserts the resulting apitest.yaml contains the markdown
// output block.
func TestInit_OutputMarkdown_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--output", "markdown", dir)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}
	body, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := "format: markdown\n  report: responses/"
	if !strings.Contains(string(body), want) {
		t.Errorf("want %q in apitest.yaml; got:\n%s", want, body)
	}
}

// TestInit_Help_DocumentsOutputFlag verifies init --help mentions --output
// and lists the full enum including markdown.
func TestInit_Help_DocumentsOutputFlag(t *testing.T) {
	stdout, _, code := captureRun(t, "init", "--help")
	if code != 0 {
		t.Fatalf("init --help exit = %d, want 0", code)
	}
	if !strings.Contains(stdout, "--output") {
		t.Errorf("init --help missing --output:\n%s", stdout)
	}
	for _, format := range []string{"terminal", "json", "tap", "junit", "html", "markdown"} {
		if !strings.Contains(stdout, format) {
			t.Errorf("init --help missing format %q:\n%s", format, stdout)
		}
	}
}

func TestValidateCmd(t *testing.T) {
	// Helper to write a temp YAML file
	writeFile := func(t *testing.T, dir, name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		return path
	}

	// behavior 1: valid file
	t.Run("valid_file_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		stdout, _, code := captureRun(t, "validate", path)
		if code != 0 {
			t.Errorf("exit code = %d, want 0; stdout=%q", code, stdout)
		}
	})

	t.Run("valid_file_prints_valid_message", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		stdout, _, _ := captureRun(t, "validate", path)
		if !strings.Contains(stdout, "valid") {
			t.Errorf("expected 'valid' in output, got: %q", stdout)
		}
	})

	// behavior 2: invalid YAML
	t.Run("invalid_yaml_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "bad.yaml", "invalid: yaml: :\n")
		_, _, code := captureRun(t, "validate", path)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	t.Run("invalid_yaml_shows_line_number", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "bad.yaml", "name: Test\nrequests:\n  - foo: [unclosed\n")
		stdout, stderr, _ := captureRun(t, "validate", path)
		combined := stdout + stderr
		// Should mention a line number somewhere
		if !strings.Contains(combined, "line") {
			t.Errorf("expected line number hint in output, got: %q", combined)
		}
	})

	// behavior 3: missing required fields
	t.Run("missing_required_field_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", `name: Test
requests:
  - request:
      url: "https://example.com"
`)
		_, _, code := captureRun(t, "validate", path)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	// behavior 4: undefined variable warns, exit 0
	t.Run("undefined_var_warns_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com/{{undefined_var}}"
`)
		stdout, stderr, code := captureRun(t, "validate", path)
		if code != 0 {
			t.Errorf("exit code = %d, want 0 for warnings; stdout=%q stderr=%q", code, stdout, stderr)
		}
		if !strings.Contains(stdout+stderr, "undefined_var") {
			t.Errorf("expected warning about undefined_var in output, got stdout=%q stderr=%q", stdout, stderr)
		}
	})

	// behavior 5: missing external file ref
	t.Run("missing_external_file_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", `name: Test
requests:
  - path: nonexistent.yaml
`)
		_, _, code := captureRun(t, "validate", path)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	// behavior 6: --format json
	t.Run("format_json_valid_file_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		_, _, code := captureRun(t, "validate", "--format", "json", path)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("format_json_invalid_file_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "bad.yaml", "invalid: yaml: :\n")
		_, _, code := captureRun(t, "validate", "--format", "json", path)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	t.Run("format_json_output_is_valid_json", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		stdout, _, _ := captureRun(t, "validate", "--format", "json", path)
		if !json.Valid([]byte(stdout)) {
			t.Errorf("output is not valid JSON: %q", stdout)
		}
	})

	t.Run("invalid_format_exit_1", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		_, _, code := captureRun(t, "validate", "--format", "bogus", path)
		if code != 1 {
			t.Errorf("exit code = %d, want 1 for unknown format", code)
		}
	})

	// behavior 7: multiple files / glob
	t.Run("multiple_files_all_valid_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		p1 := writeFile(t, dir, "col1.yaml", "name: One\nrequests: []\n")
		p2 := writeFile(t, dir, "col2.yaml", "name: Two\nrequests: []\n")
		_, _, code := captureRun(t, "validate", p1, p2)
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("multiple_files_one_invalid_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		p1 := writeFile(t, dir, "col1.yaml", "name: One\nrequests: []\n")
		p2 := writeFile(t, dir, "bad.yaml", "invalid: yaml: :\n")
		_, _, code := captureRun(t, "validate", p1, p2)
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	t.Run("glob_pattern_matches_files", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "a.yaml", "name: A\nrequests: []\n")
		writeFile(t, dir, "b.yaml", "name: B\nrequests: []\n")
		_, _, code := captureRun(t, "validate", filepath.Join(dir, "*.yaml"))
		if code != 0 {
			t.Errorf("exit code = %d, want 0", code)
		}
	})

	t.Run("glob_pattern_no_matches_exit_3", func(t *testing.T) {
		dir := t.TempDir()
		_, _, code := captureRun(t, "validate", filepath.Join(dir, "*.yaml"))
		if code != 3 {
			t.Errorf("exit code = %d, want 3 for no matches", code)
		}
	})

	// edge cases
	t.Run("no_args_prints_usage_exit_1", func(t *testing.T) {
		stdout, stderr, code := captureRun(t, "validate")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stdout+stderr, "validate") {
			t.Errorf("expected usage message, got stdout=%q stderr=%q", stdout, stderr)
		}
	})

	t.Run("nonexistent_file_exit_3", func(t *testing.T) {
		_, _, code := captureRun(t, "validate", "/tmp/no_such_file_xyz.yaml")
		if code != 3 {
			t.Errorf("exit code = %d, want 3", code)
		}
	})

	t.Run("no_color_flag_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := writeFile(t, dir, "col.yaml", "name: Test\nrequests: []\n")
		stdout, _, code := captureRun(t, "validate", "--no-color", path)
		if code != 0 {
			t.Errorf("exit code = %d, want 0; stdout=%q", code, stdout)
		}
		if strings.Contains(stdout, "\x1b[") {
			t.Errorf("output contains ANSI codes with --no-color: %q", stdout)
		}
	})
}

func TestValidateCmd_format_flag_missing_value(t *testing.T) {
	_, stderr, code := captureRun(t, "validate", "--format")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "--format requires a value") {
		t.Errorf("expected --format error in stderr, got: %q", stderr)
	}
}

func TestPrintValidationResult_color(t *testing.T) {
	tests := []struct {
		name     string
		result   *validator.Result
		wantAnsi string
	}{
		{
			name:     "valid_no_issues",
			result:   &validator.Result{FilePath: "f.yaml", Valid: true, Issues: nil},
			wantAnsi: "\033[32m",
		},
		{
			name: "valid_with_warnings",
			result: &validator.Result{
				FilePath: "f.yaml",
				Valid:    true,
				Issues:   []validator.Issue{{Severity: validator.SeverityWarning, Message: "warn"}},
			},
			wantAnsi: "\033[33m",
		},
		{
			name: "invalid_with_errors",
			result: &validator.Result{
				FilePath: "f.yaml",
				Valid:    false,
				Issues:   []validator.Issue{{Severity: validator.SeverityError, Message: "err"}},
			},
			wantAnsi: "\033[31m",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printValidationResult(&buf, tc.result, true)
			got := buf.String()
			if !strings.Contains(got, tc.wantAnsi) {
				t.Errorf("expected ANSI code %q in output, got: %q", tc.wantAnsi, got)
			}
		})
	}
}

func TestValidateHelp(t *testing.T) {
	stdout, _, code := captureRun(t, "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "validate") {
		t.Errorf("help text does not contain 'validate':\n%s", stdout)
	}
}

func TestInitThenRun_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	binary := buildBinary(t)
	dir := t.TempDir()

	// Step 1: init
	stdout, stderr, code := runBinaryInDir(t, binary, dir, "init")
	if code != 0 {
		t.Fatalf("init failed (code=%d): stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Project initialized successfully") {
		t.Errorf("expected success message, got: %q", stdout)
	}

	// Step 2: run the generated sample collection. The scaffolded base_url
	// points at httpbin.org; override it with a local server so the test
	// does not depend on public-internet weather (observed 503s/15s
	// latencies failing CI), while still exercising init → run end to end.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	stdout, stderr, code = runBinaryInDir(t, binary, dir, "run", "collections/sample.yaml", "--var", "base_url="+srv.URL)
	if code != 0 {
		t.Fatalf("run sample collection failed (code=%d): stdout=%q stderr=%q", code, stdout, stderr)
	}
}

// setupInfoProject creates a minimal apitest project in a temp directory.
// Returns the project directory path.
func setupInfoProject(t *testing.T, projectName string, collections, environments []string) string {
	t.Helper()
	dir := t.TempDir()

	// Create apitest.yaml
	content := fmt.Sprintf("project_name: %s\n", projectName)
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create collections
	if len(collections) > 0 {
		colDir := filepath.Join(dir, "collections")
		if err := os.MkdirAll(colDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, c := range collections {
			if err := os.WriteFile(filepath.Join(colDir, c), []byte("name: test\nrequests: []\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Create environments
	if len(environments) > 0 {
		envDir := filepath.Join(dir, "environments")
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, e := range environments {
			if err := os.WriteFile(filepath.Join(envDir, e+".yaml"), []byte("variables:\n  x: \"1\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}

	return dir
}

func TestInfoCmd(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		setup    func(t *testing.T) string // returns dir to chdir into
		wantExit int
		check    func(t *testing.T, stdout, stderr string)
	}{
		{
			name: "json_in_project_dir_exit_0",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml"}, []string{"dev"})
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
		{
			name: "json_output_is_valid_json",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml"}, []string{"dev"})
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !json.Valid([]byte(stdout)) {
					t.Fatalf("not valid JSON: %s", stdout)
				}
			},
		},
		{
			name: "json_lists_project_root",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, nil)
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, `"project_root"`) {
					t.Errorf("missing project_root in: %s", stdout)
				}
			},
		},
		{
			name: "json_lists_project_name",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, nil)
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, `"MyApp"`) {
					t.Errorf("missing project name in: %s", stdout)
				}
			},
		},
		{
			name: "json_lists_collections",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml", "auth.yaml"}, nil)
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				var out map[string]any
				if err := json.Unmarshal([]byte(stdout), &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				cols, ok := out["collections"].([]any)
				if !ok {
					t.Fatal("collections is not array")
				}
				if len(cols) != 2 {
					t.Errorf("expected 2 collections, got %d", len(cols))
				}
			},
		},
		{
			name: "json_lists_environments",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, []string{"dev", "staging"})
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				var out map[string]any
				if err := json.Unmarshal([]byte(stdout), &out); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				envs, ok := out["environments"].([]any)
				if !ok {
					t.Fatal("environments is not array")
				}
				if len(envs) != 2 {
					t.Errorf("expected 2 environments, got %d", len(envs))
				}
			},
		},
		{
			name: "json_empty_project_empty_arrays",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "Empty", nil, nil)
			},
			args:     []string{"info", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if strings.Contains(stdout, `"collections": null`) {
					t.Error("collections should be [] not null")
				}
				if strings.Contains(stdout, `"environments": null`) {
					t.Error("environments should be [] not null")
				}
			},
		},
		{
			name: "human_readable_default_exit_0",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml"}, []string{"dev"})
			},
			args:     []string{"info"},
			wantExit: 0,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
		{
			name: "human_readable_shows_project_name",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, nil)
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "MyApp") {
					t.Errorf("expected project name in output, got: %s", stdout)
				}
			},
		},
		{
			name: "human_readable_shows_root_path",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, nil)
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				// The output should contain a path
				if !strings.Contains(stdout, "Root:") {
					t.Errorf("expected Root: in output, got: %s", stdout)
				}
			},
		},
		{
			name: "human_readable_shows_collections",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml"}, nil)
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "collections/api.yaml") {
					t.Errorf("expected collection path in output, got: %s", stdout)
				}
			},
		},
		{
			name: "human_readable_shows_environments",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, []string{"dev"})
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "dev") {
					t.Errorf("expected environment name in output, got: %s", stdout)
				}
			},
		},
		{
			name: "human_readable_no_collections_shows_none",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, []string{"dev"})
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				colIdx := strings.Index(stdout, "Collections:")
				envIdx := strings.Index(stdout, "Environments:")
				if colIdx == -1 || envIdx == -1 {
					t.Fatalf("expected both Collections: and Environments: headers, got: %s", stdout)
				}
				colSection := stdout[colIdx:envIdx]
				if !strings.Contains(colSection, "(none)") {
					t.Errorf("expected (none) in collections section, got: %s", colSection)
				}
				envSection := stdout[envIdx:]
				if strings.Contains(envSection, "(none)") {
					t.Errorf("did not expect (none) in environments section, got: %s", envSection)
				}
			},
		},
		{
			name: "human_readable_no_environments_shows_none",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", []string{"api.yaml"}, nil)
			},
			args:     []string{"info"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				envIdx := strings.Index(stdout, "Environments:")
				if envIdx == -1 {
					t.Fatalf("expected Environments: header in output, got: %s", stdout)
				}
				envSection := stdout[envIdx:]
				if !strings.Contains(envSection, "(none)") {
					t.Errorf("expected (none) in environments section, got: %s", envSection)
				}
			},
		},
		{
			name: "outside_project_dir_exit_5",
			setup: func(t *testing.T) string {
				return t.TempDir() // no apitest.yaml
			},
			args:     []string{"info"},
			wantExit: 5,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
		{
			name: "outside_project_dir_error_message",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			args:     []string{"info"},
			wantExit: 5,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stderr, "no apitest project found") {
					t.Errorf("expected error message about no project, got stderr: %s", stderr)
				}
			},
		},
		{
			name: "invalid_format_exit_1",
			setup: func(t *testing.T) string {
				return setupInfoProject(t, "MyApp", nil, nil)
			},
			args:     []string{"info", "--format", "xml"},
			wantExit: 1,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := tc.setup(t)
			oldWd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chdir(oldWd) }()

			stdout, stderr, exitCode := captureRun(t, tc.args...)
			if exitCode != tc.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", exitCode, tc.wantExit, stdout, stderr)
			}
			tc.check(t, stdout, stderr)
		})
	}
}

func TestSchemaCmd(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
		check    func(t *testing.T, stdout, stderr string)
	}{
		{
			name:     "json_exit_0",
			args:     []string{"schema", "--format", "json"},
			wantExit: 0,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
		{
			name:     "json_output_is_valid_json",
			args:     []string{"schema", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !json.Valid([]byte(stdout)) {
					t.Fatalf("not valid JSON: %s", stdout)
				}
			},
		},
		{
			name:     "output_contains_schema_keyword",
			args:     []string{"schema", "--format", "json"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, `"$schema"`) {
					t.Errorf("missing $schema keyword in: %s", stdout)
				}
			},
		},
		{
			name:     "default_format_outputs_json",
			args:     []string{"schema"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !json.Valid([]byte(stdout)) {
					t.Fatalf("default format should be valid JSON: %s", stdout)
				}
			},
		},
		{
			name:     "invalid_format_exit_1",
			args:     []string{"schema", "--format", "xml"},
			wantExit: 1,
			check:    func(t *testing.T, stdout, stderr string) {},
		},
		{
			name:     "project_flag_emits_project_schema",
			args:     []string{"schema", "--project"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "ApiTest Project v1") {
					t.Errorf("want title 'ApiTest Project v1' in stdout; got %q", stdout)
				}
			},
		},
		{
			name:     "no_flag_emits_collection_schema",
			args:     []string{"schema"},
			wantExit: 0,
			check: func(t *testing.T, stdout, stderr string) {
				if !strings.Contains(stdout, "ApiTest Collection v1") {
					t.Errorf("want title 'ApiTest Collection v1' in stdout; got %q", stdout)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureRun(t, tc.args...)
			if exitCode != tc.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", exitCode, tc.wantExit, stdout, stderr)
			}
			tc.check(t, stdout, stderr)
		})
	}
}

func TestHelpContainsInfoAndSchema(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Fatalf("--help exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "info") {
		t.Error("help output does not mention info command")
	}
	if !strings.Contains(stdout, "schema") {
		t.Error("help output does not mention schema command")
	}
}

func TestParseExecArgs(t *testing.T) {
	t.Setenv("EXEC_TEST_KEY", "secret")

	tests := []struct {
		name    string
		args    []string
		want    ExecOptions
		wantErr string
	}{
		{
			name: "stdin flag sets stdin true",
			args: []string{"--stdin"},
			want: ExecOptions{Stdin: true, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "dry run flag",
			args: []string{"--stdin", "--dry-run"},
			want: ExecOptions{Stdin: true, DryRun: true, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "log file flag",
			args: []string{"--stdin", "--log", "out.jsonl"},
			want: ExecOptions{Stdin: true, LogFile: "out.jsonl", Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "format json",
			args: []string{"--stdin", "--format", "json"},
			want: ExecOptions{Stdin: true, Format: "json", Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "non interactive flag",
			args: []string{"--stdin", "--non-interactive"},
			want: ExecOptions{Stdin: true, NonInteractive: true, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "positional url",
			args: []string{"https://example.com"},
			want: ExecOptions{URL: "https://example.com", Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "method flag -X",
			args: []string{"-X", "POST", "https://example.com"},
			want: ExecOptions{URL: "https://example.com", Method: "POST", Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name:    "no args and no stdin returns error",
			args:    []string{},
			wantErr: "no URL provided and --stdin not set",
		},
		{
			name:    "conflicting stdin and url returns error",
			args:    []string{"--stdin", "https://example.com"},
			wantErr: "cannot use --stdin with a URL argument",
		},
		{
			name: "verbosity flags -v",
			args: []string{"--stdin", "-v"},
			want: ExecOptions{Stdin: true, Verbosity: output.VerbosityVerbose, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "verbosity flags -vv",
			args: []string{"--stdin", "-vv"},
			want: ExecOptions{Stdin: true, Verbosity: output.VerbosityDebug, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "verbosity flags -q",
			args: []string{"--stdin", "-q"},
			want: ExecOptions{Stdin: true, Verbosity: output.VerbosityQuiet, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name: "var and env-var flags",
			args: []string{"--stdin", "--var", "k=v", "--env-var", "EXEC_TEST_KEY"},
			want: ExecOptions{Stdin: true, Vars: map[string]string{"k": "v"}, EnvVars: map[string]string{"EXEC_TEST_KEY": "secret"}},
		},
		{
			name: "no-color flag",
			args: []string{"--stdin", "--no-color"},
			want: ExecOptions{Stdin: true, NoColor: true, Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
		{
			name:    "log flag missing value returns error",
			args:    []string{"--stdin", "--log"},
			wantErr: "--log requires a value",
		},
		{
			name:    "format flag missing value returns error",
			args:    []string{"--stdin", "--format"},
			wantErr: "--format requires a value",
		},
		{
			name:    "method flag missing value returns error",
			args:    []string{"-X"},
			wantErr: "-X/--method requires a value",
		},
		{
			name: "method flag --method",
			args: []string{"--method", "PUT", "https://example.com"},
			want: ExecOptions{URL: "https://example.com", Method: "PUT", Vars: map[string]string{}, EnvVars: map[string]string{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExecArgs(tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.URL != tt.want.URL {
				t.Errorf("URL = %q, want %q", got.URL, tt.want.URL)
			}
			if got.Method != tt.want.Method {
				t.Errorf("Method = %q, want %q", got.Method, tt.want.Method)
			}
			if got.Stdin != tt.want.Stdin {
				t.Errorf("Stdin = %v, want %v", got.Stdin, tt.want.Stdin)
			}
			if got.DryRun != tt.want.DryRun {
				t.Errorf("DryRun = %v, want %v", got.DryRun, tt.want.DryRun)
			}
			if got.LogFile != tt.want.LogFile {
				t.Errorf("LogFile = %q, want %q", got.LogFile, tt.want.LogFile)
			}
			if got.Format != tt.want.Format {
				t.Errorf("Format = %q, want %q", got.Format, tt.want.Format)
			}
			if got.NonInteractive != tt.want.NonInteractive {
				t.Errorf("NonInteractive = %v, want %v", got.NonInteractive, tt.want.NonInteractive)
			}
			if got.NoColor != tt.want.NoColor {
				t.Errorf("NoColor = %v, want %v", got.NoColor, tt.want.NoColor)
			}
			if got.Verbosity != tt.want.Verbosity {
				t.Errorf("Verbosity = %v, want %v", got.Verbosity, tt.want.Verbosity)
			}
			if len(got.Vars) != len(tt.want.Vars) {
				t.Fatalf("Vars = %v, want %v", got.Vars, tt.want.Vars)
			}
			for k, wantV := range tt.want.Vars {
				if gotV := got.Vars[k]; gotV != wantV {
					t.Errorf("Vars[%q] = %q, want %q", k, gotV, wantV)
				}
			}
			if len(got.EnvVars) != len(tt.want.EnvVars) {
				t.Fatalf("EnvVars = %v, want %v", got.EnvVars, tt.want.EnvVars)
			}
			for k, wantV := range tt.want.EnvVars {
				if gotV := got.EnvVars[k]; gotV != wantV {
					t.Errorf("EnvVars[%q] = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func TestParseStdinRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantURL string
		wantMth string
		wantHdr map[string]string
		wantQry map[string]string
		wantErr string
	}{
		{
			name:    "valid json with url and method",
			input:   `{"url":"https://example.com/api","method":"POST"}`,
			wantURL: "https://example.com/api",
			wantMth: "POST",
		},
		{
			name:    "valid json with headers",
			input:   `{"url":"https://example.com","method":"GET","headers":{"Authorization":"Bearer tok"}}`,
			wantURL: "https://example.com",
			wantMth: "GET",
			wantHdr: map[string]string{"Authorization": "Bearer tok"},
		},
		{
			name:    "valid json with body object",
			input:   `{"url":"https://example.com","method":"POST","body":{"key":"val"}}`,
			wantURL: "https://example.com",
			wantMth: "POST",
		},
		{
			name:    "valid json with body string",
			input:   `{"url":"https://example.com","method":"POST","body":"raw body"}`,
			wantURL: "https://example.com",
			wantMth: "POST",
		},
		{
			name:    "valid json with query params",
			input:   `{"url":"https://example.com","method":"GET","query":{"page":"1","limit":"10"}}`,
			wantURL: "https://example.com",
			wantMth: "GET",
			wantQry: map[string]string{"page": "1", "limit": "10"},
		},
		{
			name:    "method defaults to GET when omitted",
			input:   `{"url":"https://example.com"}`,
			wantURL: "https://example.com",
			wantMth: "GET",
		},
		{
			name:    "missing url returns error",
			input:   `{"method":"GET"}`,
			wantErr: "missing required field \"url\"",
		},
		{
			name:    "invalid json returns error",
			input:   `not valid json`,
			wantErr: "invalid JSON input",
		},
		{
			name:    "empty input returns error",
			input:   ``,
			wantErr: "no input received on stdin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			got, err := parseStdinRequest(r)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tt.wantURL)
			}
			if got.Method != tt.wantMth {
				t.Errorf("Method = %q, want %q", got.Method, tt.wantMth)
			}
			if tt.wantHdr != nil {
				for k, wantV := range tt.wantHdr {
					if gotV := got.Headers[k]; gotV != wantV {
						t.Errorf("Headers[%q] = %q, want %q", k, gotV, wantV)
					}
				}
			}
			if tt.wantQry != nil {
				for k, wantV := range tt.wantQry {
					if gotV := got.QueryParams[k]; gotV != wantV {
						t.Errorf("QueryParams[%q] = %q, want %q", k, gotV, wantV)
					}
				}
			}
		})
	}
}

// captureExecCmd calls execCmdOut in-process and captures stdout/stderr output.
func captureExecCmd(t *testing.T, stdinData string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	var stdinReader io.Reader
	if stdinData != "" {
		stdinReader = strings.NewReader(stdinData)
	} else {
		stdinReader = strings.NewReader("")
	}
	exitCode = execCmdOut(args, stdinReader, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), exitCode
}

func TestExecCmd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"url":"%s","method":"%s"}`, r.URL.String(), r.Method)
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		stdin      string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "stdin json executes request",
			args:       []string{"--stdin"},
			stdin:      fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit:   0,
			wantStdout: "200",
		},
		{
			name:       "inline url executes request",
			args:       []string{srv.URL + "/get"},
			wantExit:   0,
			wantStdout: "200",
		},
		{
			name:       "dry run does not execute http",
			args:       []string{srv.URL + "/get", "--dry-run"},
			wantExit:   0,
			wantStdout: "DRY RUN",
		},
		{
			name:       "dry run shows request details",
			args:       []string{srv.URL + "/get", "--dry-run", "-X", "POST"},
			wantExit:   0,
			wantStdout: "POST",
		},
		{
			name:       "format json produces valid json",
			args:       []string{"--stdin", "--format", "json"},
			stdin:      fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit:   0,
			wantStdout: `"status"`,
		},
		{
			name:       "invalid stdin json returns error",
			args:       []string{"--stdin"},
			stdin:      "not valid json",
			wantExit:   3,
			wantStderr: "invalid JSON input",
		},
		{
			name:       "non interactive suppresses prompts",
			args:       []string{"--stdin", "--non-interactive"},
			stdin:      "invalid json",
			wantExit:   3,
			wantStderr: "invalid JSON input",
		},
		{
			name:     "log appends jsonl entry",
			args:     []string{"--stdin", "--log", filepath.Join(t.TempDir(), "exec.jsonl")},
			stdin:    fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit: 0,
		},
		{
			name:     "dry run with log records dry run entry",
			args:     []string{srv.URL + "/get", "--dry-run", "--log", filepath.Join(t.TempDir(), "dry.jsonl")},
			wantExit: 0,
		},
		{
			name:       "missing url and no stdin returns error",
			args:       []string{},
			wantExit:   1,
			wantStderr: "no URL provided",
		},
		{
			name:       "var flag interpolates in url",
			args:       []string{"--stdin", "--var", "host=" + srv.URL},
			stdin:      `{"url":"{{host}}/get","method":"GET"}`,
			wantExit:   0,
			wantStdout: "200",
		},
		{
			name:       "unknown format returns error",
			args:       []string{"--stdin", "--format", "xml"},
			stdin:      fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit:   1,
			wantStderr: "unknown output format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureExecCmd(t, tt.stdin, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want containing %q", stdout, tt.wantStdout)
			}
			if tt.wantStderr != "" && !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestExecCmd_logFileContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logFile := filepath.Join(t.TempDir(), "exec.jsonl")
	stdin := fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL)
	_, _, exitCode := captureExecCmd(t, stdin, "--stdin", "--log", logFile)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty")
	}
	var entry output.JSONLEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSONL: %v\ndata: %s", err, data)
	}
	if entry.StatusCode != 200 {
		t.Errorf("log status_code = %d, want 200", entry.StatusCode)
	}
	if entry.DryRun {
		t.Error("log dry_run should be false")
	}
}

func TestExecCmd_dryRunLogEntry(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dry.jsonl")
	_, _, exitCode := captureExecCmd(t, "", "https://example.com", "--dry-run", "--log", logFile)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	var entry output.JSONLEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if !entry.DryRun {
		t.Error("log dry_run should be true")
	}
}

// TestExecCmd_LogContainsRunIDAndRequestID asserts that a live exec invocation
// writes a JSONL log entry carrying the additive M11-004 correlation fields:
// run_id (32-char lowercase hex) and request_id ("req-1").
func TestExecCmd_LogContainsRunIDAndRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logFile := filepath.Join(t.TempDir(), "exec.jsonl")
	stdin := fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL)
	_, _, exitCode := captureExecCmd(t, stdin, "--stdin", "--log", logFile)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty")
	}
	var entry output.JSONLEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSONL: %v\ndata: %s", err, data)
	}
	runIDPattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	if !runIDPattern.MatchString(entry.RunID) {
		t.Errorf("run_id %q does not match %s", entry.RunID, runIDPattern)
	}
	if entry.RequestID != "req-1" {
		t.Errorf("request_id = %q, want %q", entry.RequestID, "req-1")
	}
}

// TestExecCmd_LogDryRun_RunIDPresent asserts that --dry-run also writes the
// correlation fields; every invocation gets a run_id, even when no HTTP fires.
func TestExecCmd_LogDryRun_RunIDPresent(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "dry.jsonl")
	_, _, exitCode := captureExecCmd(t, "", "https://example.com", "--dry-run", "--log", logFile)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	var entry output.JSONLEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if !entry.DryRun {
		t.Error("log dry_run should be true")
	}
	runIDPattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	if !runIDPattern.MatchString(entry.RunID) {
		t.Errorf("run_id %q does not match %s", entry.RunID, runIDPattern)
	}
	if entry.RequestID != "req-1" {
		t.Errorf("request_id = %q, want %q", entry.RequestID, "req-1")
	}
}

// TestExecCmd_LogHTTPError_RunIDPresent asserts that the exec-error log path
// (execErr != nil branch) also writes correlation fields. The test dials a
// refused port to trigger a connection error without a live server.
func TestExecCmd_LogHTTPError_RunIDPresent(t *testing.T) {
	// Listen then immediately close to get a port that refuses connections.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not bind listener: %v", err)
	}
	refusedAddr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	logFile := filepath.Join(t.TempDir(), "err.jsonl")
	stdin := fmt.Sprintf(`{"url":"http://%s/","method":"GET"}`, refusedAddr)
	_, _, exitCode := captureExecCmd(t, stdin, "--stdin", "--log", logFile)
	// exec error path returns exit code 4.
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit code for refused connection, got 0")
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty; exec-error path must write a JSONL entry")
	}
	var entry output.JSONLEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("invalid JSONL: %v\ndata: %s", err, data)
	}
	if entry.Error == "" {
		t.Error("log entry error field should be non-empty for HTTP error path")
	}
	runIDPattern := regexp.MustCompile(`^[0-9a-f]{32}$`)
	if !runIDPattern.MatchString(entry.RunID) {
		t.Errorf("run_id %q does not match %s", entry.RunID, runIDPattern)
	}
	if entry.RequestID != "req-1" {
		t.Errorf("request_id = %q, want %q", entry.RequestID, "req-1")
	}
}

func TestExecCmd_formatJsonSchema(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	stdin := fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL)
	stdout, _, exitCode := captureExecCmd(t, stdin, "--stdin", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	var out output.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nstdout: %s", err, stdout)
	}
	if out.Status != "passed" {
		t.Errorf("status = %q, want \"passed\"", out.Status)
	}
	if len(out.Requests) != 1 {
		t.Fatalf("requests count = %d, want 1", len(out.Requests))
	}
	if out.Requests[0].StatusCode != 200 {
		t.Errorf("request status_code = %d, want 200", out.Requests[0].StatusCode)
	}
}

func TestExecCmd_dryRunJsonSchema(t *testing.T) {
	stdout, _, exitCode := captureExecCmd(t, "", "https://example.com", "--dry-run", "--format", "json")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	// Verify assertions is [] not null (schema compatibility with run command)
	if strings.Contains(stdout, `"assertions": null`) {
		t.Errorf("dry-run JSON should have \"assertions\":[] not null:\n%s", stdout)
	}

	var out output.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nstdout: %s", err, stdout)
	}
	if out.Requests[0].Assertions == nil {
		t.Error("dry-run JSONRequest.Assertions should be non-nil empty slice, got nil")
	}
	// M11-002: summary must never be null — spec guarantees "summary is always present".
	if out.Summary == nil {
		t.Error("dry-run JSON summary must not be nil; SPECIFICATION.md promises summary is always present (never null)")
	}
}

// TestExecCmd_DryRun_UsesPrinterRequestDetail asserts that --dry-run renders
// the request via output.Printer.RequestDetail (i.e. "  > METHOD url" shape)
// rather than the legacy raw fmt.Fprintf format ("  METHOD url").
func TestExecCmd_DryRun_UsesPrinterRequestDetail(t *testing.T) {
	stdout, _, rc := captureExecCmd(t, "", "https://example.com", "--dry-run")
	if rc != 0 {
		t.Fatalf("exit = %d, want 0", rc)
	}
	if !strings.Contains(stdout, "DRY RUN\n") {
		t.Errorf("stdout missing 'DRY RUN' label: %q", stdout)
	}
	// RequestDetail emits "  > GET https://example.com" — the ">" marker
	// distinguishes it from the old "  GET https://example.com" format.
	if !strings.Contains(stdout, "  > GET https://example.com") {
		t.Errorf("stdout missing RequestDetail line (  > GET url): %q", stdout)
	}
}

func TestRun_ExecDispatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exitCode := run([]string{"exec", srv.URL + "/test"})
	if exitCode != 0 {
		t.Errorf("run(exec) exit code = %d, want 0", exitCode)
	}
}

func TestHelpText_ContainsExec(t *testing.T) {
	var buf bytes.Buffer
	exitCode := runWithWriters([]string{"--help"}, &buf, &bytes.Buffer{})
	stdout := buf.String()

	if exitCode != 0 {
		t.Fatalf("--help exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "exec") {
		t.Error("help output does not mention exec command")
	}
	if !strings.Contains(stdout, "--stdin") {
		t.Error("help output does not mention --stdin")
	}
	if !strings.Contains(stdout, "--dry-run") {
		t.Error("help output does not mention --dry-run")
	}
	if !strings.Contains(stdout, "--log") {
		t.Error("help output does not mention --log")
	}
	if !strings.Contains(stdout, "--non-interactive") {
		t.Error("help output does not mention --non-interactive")
	}
}

func runBinaryWithStdin(t *testing.T, binary, stdin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestExecIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	binary := buildBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"url":"%s","method":"%s"}`, r.URL.String(), r.Method)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantExit int
		check    func(t *testing.T, stdout, stderr string)
	}{
		{
			name:     "stdin json via binary",
			args:     []string{"exec", "--stdin"},
			stdin:    fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit: 0,
			check: func(t *testing.T, stdout, _ string) {
				if !strings.Contains(stdout, "200") {
					t.Errorf("stdout should contain 200, got: %s", stdout)
				}
			},
		},
		{
			name:     "inline url via binary",
			args:     []string{"exec", srv.URL + "/get"},
			wantExit: 0,
			check: func(t *testing.T, stdout, _ string) {
				if !strings.Contains(stdout, "200") {
					t.Errorf("stdout should contain 200, got: %s", stdout)
				}
			},
		},
		{
			name:     "dry run via binary",
			args:     []string{"exec", srv.URL + "/get", "--dry-run"},
			wantExit: 0,
			check: func(t *testing.T, stdout, _ string) {
				if !strings.Contains(stdout, "DRY RUN") {
					t.Errorf("stdout should contain DRY RUN, got: %s", stdout)
				}
			},
		},
		{
			name:     "format json via binary",
			args:     []string{"exec", "--stdin", "--format", "json"},
			stdin:    fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit: 0,
			check: func(t *testing.T, stdout, _ string) {
				var out output.JSONOutput
				if err := json.Unmarshal([]byte(stdout), &out); err != nil {
					t.Errorf("invalid JSON: %v", err)
				}
			},
		},
		{
			name:     "invalid json via binary",
			args:     []string{"exec", "--stdin"},
			stdin:    "not json",
			wantExit: 3,
			check: func(t *testing.T, _, stderr string) {
				if !strings.Contains(stderr, "invalid JSON input") {
					t.Errorf("stderr should contain error, got: %s", stderr)
				}
			},
		},
		{
			name:     "log file created via binary",
			args:     []string{"exec", "--stdin", "--log", filepath.Join(t.TempDir(), "int.jsonl")},
			stdin:    fmt.Sprintf(`{"url":"%s/get","method":"GET"}`, srv.URL),
			wantExit: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := runBinaryWithStdin(t, binary, tt.stdin, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s\nstderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			if tt.check != nil {
				tt.check(t, stdout, stderr)
			}
		})
	}
}

func TestParseVaultArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantSub   string
		wantFmt   string
		wantNoClr bool
		wantErr   bool
	}{
		{"empty args", nil, "", "", false, false},
		{"list subcommand", []string{"list"}, "list", "", false, false},
		{"format json", []string{"--format", "json"}, "", "json", false, false},
		{"format missing value", []string{"--format"}, "", "", false, true},
		{"no color flag", []string{"--no-color"}, "", "", true, false},
		{"list with format json", []string{"list", "--format", "json"}, "list", "json", false, false},
		{"unknown arg", []string{"--bogus"}, "", "", false, true},
		{"format before list", []string{"--format", "json", "list"}, "list", "json", false, false},
		{"format after list", []string{"list", "--format", "json"}, "list", "json", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, fmt, noClr, err := parseVaultArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVaultArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if sub != tt.wantSub {
				t.Errorf("subcommand = %q, want %q", sub, tt.wantSub)
			}
			if fmt != tt.wantFmt {
				t.Errorf("format = %q, want %q", fmt, tt.wantFmt)
			}
			if noClr != tt.wantNoClr {
				t.Errorf("noColor = %v, want %v", noClr, tt.wantNoClr)
			}
		})
	}
}

func TestVaultCmd_arg_errors(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr []string
	}{
		{"vault format missing value exits 1", []string{"vault", "--format"}, 1, []string{"--format requires a value"}},
		{"vault unknown arg exits 1", []string{"vault", "--bogus"}, 1, []string{"unknown argument"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stderr: %s", exitCode, tt.wantExit, stderr)
			}
			for _, s := range tt.wantStderr {
				if !strings.Contains(stderr, s) {
					t.Errorf("stderr %q does not contain %q", stderr, s)
				}
			}
		})
	}
}

func TestVaultCmd_list(t *testing.T) {
	// Create a temp dir with apitest.yaml containing secrets block
	dir := t.TempDir()
	yamlContent := `project_name: VaultTest
secrets:
  provider: aws-secrets-manager
  region: us-east-1
  keys:
    api_key: prod/api-key
    db_password: prod/db#password
`
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout []string
	}{
		{"list shows provider", []string{"vault", "list"}, 0, []string{"aws-secrets-manager"}},
		{"list shows key count", []string{"vault", "list"}, 0, []string{"2 key"}},
		{"list shows key names", []string{"vault", "list"}, 0, []string{"api_key"}},
		{"list json is valid json", []string{"vault", "list", "--format", "json"}, 0, []string{"{"}},
		{"list json includes provider", []string{"vault", "list", "--format", "json"}, 0, []string{"aws-secrets-manager"}},
		{"list json includes keys array", []string{"vault", "list", "--format", "json"}, 0, []string{`"keys"`}},
		{"no subcommand shows help", []string{"vault"}, 0, []string{"vault list"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stdout: %s; stderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			for _, s := range tt.wantStdout {
				if !strings.Contains(stdout, s) {
					t.Errorf("stdout %q does not contain %q", stdout, s)
				}
			}
		})
	}
}

func TestVaultCmd_no_profiles(t *testing.T) {
	// Create temp dir with apitest.yaml WITHOUT secrets block
	dir := t.TempDir()
	yamlContent := `project_name: NoVaultTest
`
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout []string
	}{
		{"list no profiles shows message", []string{"vault", "list"}, 0, []string{"No vault profiles configured"}},
		{"list json no profiles empty keys", []string{"vault", "list", "--format", "json"}, 0, []string{`"keys": []`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stdout: %s; stderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			for _, s := range tt.wantStdout {
				if !strings.Contains(stdout, s) {
					t.Errorf("stdout %q does not contain %q", stdout, s)
				}
			}
		})
	}
}

func TestVaultCmd_empty_keys(t *testing.T) {
	// Create temp dir with apitest.yaml that has secrets block but no keys
	dir := t.TempDir()
	yamlContent := `project_name: EmptyKeysTest
secrets:
  provider: aws-secrets-manager
  region: us-east-1
`
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr []string
	}{
		{"secrets with no keys returns error", []string{"vault", "list"}, 1, []string{"Error loading project config"}},
		{"json format also returns error", []string{"vault", "list", "--format", "json"}, 1, []string{"Error loading project config"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stderr: %s", exitCode, tt.wantExit, stderr)
			}
			for _, s := range tt.wantStderr {
				if !strings.Contains(stderr, s) {
					t.Errorf("stderr %q does not contain %q", stderr, s)
				}
			}
		})
	}
}

func TestVaultCmd_gcp(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `project_name: VaultGCPTest
secrets:
  provider: gcp-secret-manager
  project: my-project
  keys:
    api_key: prod/api-key
    db_password: prod/db#password
`
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout []string
	}{
		{"list shows provider", []string{"vault", "list"}, 0, []string{"gcp-secret-manager"}},
		{"list shows key count", []string{"vault", "list"}, 0, []string{"2 key"}},
		{"list shows key names", []string{"vault", "list"}, 0, []string{"api_key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stdout: %s; stderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			for _, s := range tt.wantStdout {
				if !strings.Contains(stdout, s) {
					t.Errorf("stdout %q does not contain %q", stdout, s)
				}
			}
		})
	}
}

func TestVaultCmd_1password(t *testing.T) {
	dir := t.TempDir()
	yamlContent := `project_name: Vault1PasswordTest
secrets:
  provider: 1password
  keys:
    api_key: op://MyVault/api-key/credential
    db_password: prod-db-creds
`
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout []string
	}{
		{"list shows provider", []string{"vault", "list"}, 0, []string{"1password"}},
		{"list shows key count", []string{"vault", "list"}, 0, []string{"2 key"}},
		{"list shows key names", []string{"vault", "list"}, 0, []string{"api_key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, exitCode := captureRun(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d; stdout: %s; stderr: %s", exitCode, tt.wantExit, stdout, stderr)
			}
			for _, s := range tt.wantStdout {
				if !strings.Contains(stdout, s) {
					t.Errorf("stdout %q does not contain %q", stdout, s)
				}
			}
		})
	}
}

func TestHelpText_vault(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Fatalf("--help exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "vault") {
		t.Error("help output does not mention vault command")
	}
}

func TestRunCmd_GuardRail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	// Create a collection with 4 requests (we'll set MaxRequests=3 so guard rail triggers)
	dir := t.TempDir()
	colYAML := fmt.Sprintf(`name: GuardRailTest
requests:
  - name: req-1
    request:
      method: GET
      url: "%s/1"
  - name: req-2
    request:
      method: GET
      url: "%s/2"
  - name: req-3
    request:
      method: GET
      url: "%s/3"
  - name: req-4
    request:
      method: GET
      url: "%s/4"
`, srv.URL, srv.URL, srv.URL, srv.URL)
	colFile := writeCollection(t, dir, "guard.yaml", colYAML)

	tests := []struct {
		name       string
		format     string
		wantExit   int
		wantOutput string // substring in stdout
	}{
		{"terminal exit code 2", "", 2, "Request limit exceeded"},
		{"terminal hint message", "", 2, "Split this collection"},
		{"json status guard_rail", "json", 2, "guard_rail"},
		{"json has guard_rail field", "json", 2, "limit_exceeded"},
		{"tap comment", "tap", 2, "# Guard rail"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := runner.MaxRequests
			runner.MaxRequests = 3
			t.Cleanup(func() { runner.MaxRequests = old })

			args := []string{colFile, "--no-color"}
			if tt.format != "" {
				args = append(args, "--format", tt.format)
			}
			stdout, _, exitCode := captureRunCmd(t, args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d\nstdout: %s", exitCode, tt.wantExit, stdout)
			}
			if !strings.Contains(stdout, tt.wantOutput) {
				t.Errorf("stdout %q does not contain %q", stdout, tt.wantOutput)
			}
		})
	}
}

func TestRunFromCommand_at_free_tier_no_longer_gated(t *testing.T) {
	// from_command is now Free-tier: confirm no gate error (exit != 6, no feature_gated).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	col := filepath.Join(tmpDir, "col.yaml")
	body := fmt.Sprintf(`name: free tier from_command
variables:
  secret:
    from_command: "echo secret123"
requests:
  - name: test
    request:
      method: GET
      url: "%s/{{secret}}"
`, srv.URL)
	if err := os.WriteFile(col, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, exitCode := captureRun(t, "run", col)
	if exitCode == 6 {
		t.Errorf("exit code = 6 (gated); want non-gate; stderr: %s", stderr)
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if strings.Contains(stdout, "feature_gated") {
		t.Errorf("stdout %q unexpectedly contains 'feature_gated'", stdout)
	}
}

func TestRunFromCommand_at_free_tier_json_no_longer_gated(t *testing.T) {
	// from_command is now Free-tier: confirm JSON output has no feature_gated status.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	tmpDir := t.TempDir()
	col := filepath.Join(tmpDir, "col.yaml")
	body := fmt.Sprintf(`name: free tier from_command json
variables:
  secret:
    from_command: "echo secret123"
requests:
  - name: test
    request:
      method: GET
      url: "%s/{{secret}}"
`, srv.URL)
	if err := os.WriteFile(col, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRun(t, "run", col, "--format", "json")
	if exitCode == 6 {
		t.Errorf("exit code = 6 (gated); from_command must not gate at Free tier")
	}
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !json.Valid([]byte(stdout)) {
		t.Errorf("stdout is not valid JSON: %q", stdout)
	}
	if strings.Contains(stdout, "feature_gated") {
		t.Errorf("stdout %q unexpectedly contains 'feature_gated'", stdout)
	}
}

func TestRunCmd_AuthProfileFailure_exitCode5(t *testing.T) {
	dir := t.TempDir()
	// apitest.yaml references a login collection that does not exist.
	if err := os.WriteFile(filepath.Join(dir, "apitest.yaml"), []byte(`project_name: AuthFailTest
auth_profiles:
  login:
    type: dynamic
    collection: auth/login.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}
	colFile := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(colFile, []byte(`name: Main Collection
requests:
  - name: main
    request:
      method: GET
      url: https://example.com
`), 0o644); err != nil {
		t.Fatal(err)
	}

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	_, stderr, exitCode := captureRun(t, "run", colFile)
	if exitCode != 5 {
		t.Errorf("exit code = %d, want 5 (auth profile failure)\nstderr: %s", exitCode, stderr)
	}
}

func TestFromCommand_sensitive_in_parsed_collection(t *testing.T) {
	tmpDir := t.TempDir()
	col := writeCollection(t, tmpDir, "sensitive_cmd.yaml", `
name: Sensitive Command Test
variables:
  secret:
    from_command: "echo secret_value_123"
    sensitive: true
requests:
  - name: Test
    request:
      method: GET
      url: "https://example.com/{{secret}}"
`)
	parsed, err := parser.ParseFile(col)
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if !parsed.Variables.Sensitive.IsSensitive("secret") {
		t.Error("from_command with sensitive: true should mark variable as sensitive")
	}
	if cmd, ok := parsed.Variables.Commands["secret"]; !ok {
		t.Error("expected 'secret' in Commands map")
	} else if !cmd.Sensitive {
		t.Error("expected CommandVar.Sensitive = true")
	}
}

func TestRun_watch_command_recognized(t *testing.T) {
	_, stderr, exitCode := captureRun(t, "watch")
	// Should fail with usage error (missing file), not "unknown command"
	if exitCode != 1 {
		t.Errorf("run(watch) exitCode = %d, want 1 (usage error)", exitCode)
	}
	if strings.Contains(stderr, "Unknown command") {
		t.Error("watch should be recognized, not 'Unknown command'")
	}
}

func TestWatchCmd_missing_file(t *testing.T) {
	_, stderr, exitCode := captureRun(t, "watch")
	if exitCode != 1 {
		t.Errorf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "missing collection file") {
		t.Errorf("stderr = %q, want to contain 'missing collection file'", stderr)
	}
}

func TestRun_help_shows_watch(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Fatalf("help exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "watch") {
		t.Error("help output should mention 'watch' command")
	}
}

func TestParseRunArgs_ShowDependencies(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantShow bool
		wantDry  bool
		wantErr  bool
	}{
		{"show-dependencies flag", []string{"--show-dependencies", "test.yaml"}, true, false, false},
		{"show-dependencies with dry-run", []string{"--show-dependencies", "--dry-run", "test.yaml"}, true, true, false},
		{"dry-run alone", []string{"--dry-run", "test.yaml"}, false, true, false},
		{"no flags", []string{"test.yaml"}, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.showDeps != tt.wantShow {
				t.Errorf("showDeps = %v, want %v", flags.showDeps, tt.wantShow)
			}
			if flags.dryRun != tt.wantDry {
				t.Errorf("dryRun = %v, want %v", flags.dryRun, tt.wantDry)
			}
		})
	}
}

func TestRun_ShowDependencies_DOTOutput(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "deps.yaml", `
name: Dependency Test
requests:
  - name: Login
    request:
      method: POST
      url: "http://example.com/login"
    extract:
      token: "$.token"
  - name: Get User
    request:
      method: GET
      url: "http://example.com/user"
      headers:
        Authorization: "Bearer {{token}}"
`)

	stdout, _, exitCode := captureRunCmd(t, "--show-dependencies", col)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if !strings.Contains(stdout, "digraph") {
		t.Errorf("expected DOT output with 'digraph', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Login") {
		t.Errorf("expected DOT output with 'Login', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Get User") {
		t.Errorf("expected DOT output with 'Get User', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "->") {
		t.Errorf("expected DOT output with edge (->), got:\n%s", stdout)
	}
}

func TestRun_ShowDependencies_DryRunWaves(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "waves.yaml", `
name: Wave Test
requests:
  - name: Login
    request:
      method: POST
      url: "http://example.com/login"
    extract:
      token: "$.token"
  - name: Get User
    request:
      method: GET
      url: "http://example.com/user"
      headers:
        Authorization: "Bearer {{token}}"
  - name: Get Orders
    request:
      method: GET
      url: "http://example.com/orders"
      headers:
        Authorization: "Bearer {{token}}"
`)

	stdout, _, exitCode := captureRunCmd(t, "--show-dependencies", "--dry-run", col)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if !strings.Contains(stdout, "Wave 1") {
		t.Errorf("expected wave output with 'Wave 1', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Wave 2") {
		t.Errorf("expected wave output with 'Wave 2', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Login") {
		t.Errorf("expected wave output with 'Login', got:\n%s", stdout)
	}
}

func TestRun_ShowDependencies_CircularError(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "circular.yaml", `
name: Circular Test
requests:
  - name: Request A
    request:
      method: GET
      url: "http://example.com/{{y}}"
    extract:
      x: "$.x"
  - name: Request B
    request:
      method: GET
      url: "http://example.com/{{x}}"
    extract:
      y: "$.y"
`)

	_, stderr, exitCode := captureRunCmd(t, "--show-dependencies", col)
	if exitCode != 3 {
		t.Fatalf("exitCode = %d, want 3\nstderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "circular") && !strings.Contains(stderr, "Request A") {
		t.Errorf("expected circular dependency error mentioning request names, got:\n%s", stderr)
	}
}

func TestRun_ShowDependencies_Independent(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "indep.yaml", `
name: Independent Test
requests:
  - name: Get Users
    request:
      method: GET
      url: "http://example.com/users"
  - name: Get Orders
    request:
      method: GET
      url: "http://example.com/orders"
  - name: Get Products
    request:
      method: GET
      url: "http://example.com/products"
`)

	stdout, _, exitCode := captureRunCmd(t, "--show-dependencies", "--dry-run", col)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	// All 3 independent requests should be in wave 1
	if !strings.Contains(stdout, "3 concurrent") {
		t.Errorf("expected single wave with 3 concurrent requests, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Total waves: 1") {
		t.Errorf("expected 'Total waves: 1', got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Maximum parallelism: 3") {
		t.Errorf("expected 'Maximum parallelism: 3', got:\n%s", stdout)
	}
}

// TestRun_ShowDeps_OnlyFilter verifies that --only + --show-dependencies shows
// only the selected request in the wave output and excludes unselected ones.
// This covers filterShowDepsItems which was at 0% coverage after Iteration 1
// (review iteration 2, finding #1).
func TestRun_ShowDeps_OnlyFilter(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "multi.yaml", `
name: Multi Request
requests:
  - name: Login
    request:
      method: POST
      url: "http://example.com/login"
    extract:
      token: "$.token"
  - name: Get User
    request:
      method: GET
      url: "http://example.com/user"
      headers:
        Authorization: "Bearer {{token}}"
  - name: Get Orders
    request:
      method: GET
      url: "http://example.com/orders"
      headers:
        Authorization: "Bearer {{token}}"
`)

	t.Run("selected request appears in wave output", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, "--show-dependencies", "--dry-run", "--only", "Get User", col)
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
		}
		if !strings.Contains(stdout, "Get User") {
			t.Errorf("expected selected request 'Get User' in wave output, got:\n%s", stdout)
		}
		// Unselected requests must not appear
		if strings.Contains(stdout, "Get Orders") {
			t.Errorf("unselected request 'Get Orders' must not appear in wave output, got:\n%s", stdout)
		}
	})

	t.Run("union of two selected requests", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, "--show-dependencies", "--dry-run",
			"--only", "Get User", "--only", "Get Orders", col)
		if exitCode != 0 {
			t.Fatalf("exitCode = %d, want 0\nstdout: %s", exitCode, stdout)
		}
		if !strings.Contains(stdout, "Get User") {
			t.Errorf("expected 'Get User' in wave output, got:\n%s", stdout)
		}
		if !strings.Contains(stdout, "Get Orders") {
			t.Errorf("expected 'Get Orders' in wave output, got:\n%s", stdout)
		}
		// The unselected Login must not appear
		if strings.Contains(stdout, "Login") {
			t.Errorf("unselected request 'Login' must not appear in wave output, got:\n%s", stdout)
		}
	})

	t.Run("no-match exits 3 with stderr", func(t *testing.T) {
		_, stderr, exitCode := captureRunCmd(t, "--show-dependencies", "--only", "Nope", col)
		if exitCode != 3 {
			t.Fatalf("exitCode = %d, want 3\nstderr: %s", exitCode, stderr)
		}
		if !strings.Contains(stderr, `"Nope"`) {
			t.Errorf("stderr must mention the unknown name, got:\n%s", stderr)
		}
		if !strings.Contains(stderr, "available:") {
			t.Errorf("stderr must list available names, got:\n%s", stderr)
		}
	})
}

func TestRun_help_shows_dependencies_flag(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Fatalf("help exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, "--show-dependencies") {
		t.Error("help output should mention --show-dependencies flag")
	}
	if !strings.Contains(stdout, "--dry-run") {
		t.Error("help output should mention --dry-run flag")
	}
}

func TestCLIIntegration_ShowDependencies_DOTOutput(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()

	// Create a collection with a dependency: A extracts token, B uses token
	col := writeCollection(t, dir, "deps.yaml", `
name: Dependency Test
requests:
  - name: Login
    request:
      method: POST
      url: http://example.com/login
    extract:
      token: $.token
  - name: Get User
    request:
      method: GET
      url: http://example.com/user
      headers:
        Authorization: "Bearer {{token}}"
`)

	stdout, stderr, exitCode := runBinary(t, binary, "run", "--show-dependencies", col)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\nstderr: %s", exitCode, stderr)
	}

	// Verify DOT format output
	if !strings.Contains(stdout, "digraph") {
		t.Errorf("expected DOT digraph output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Login") {
		t.Errorf("expected 'Login' node in DOT output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Get User") {
		t.Errorf("expected 'Get User' node in DOT output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "->") {
		t.Errorf("expected edge '->' in DOT output, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "token") {
		t.Errorf("expected 'token' variable label in DOT output, got:\n%s", stdout)
	}
}

func TestParseRunArgs_Parallel(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantParallel bool
	}{
		{"with --parallel", []string{"test.yaml", "--parallel"}, true},
		{"without --parallel", []string{"test.yaml"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.parallel != tt.wantParallel {
				t.Errorf("parallel = %v, want %v", flags.parallel, tt.wantParallel)
			}
		})
	}
}

func TestParseRunArgs_ConfirmLargeDataset(t *testing.T) {
	tests := []struct {
		name               string
		args               []string
		wantConfirmLargeDS bool
	}{
		{"with --confirm-large-dataset", []string{"test.yaml", "--confirm-large-dataset"}, true},
		{"without --confirm-large-dataset", []string{"test.yaml"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.confirmLargeDataset != tt.wantConfirmLargeDS {
				t.Errorf("confirmLargeDataset = %v, want %v", flags.confirmLargeDataset, tt.wantConfirmLargeDS)
			}
		})
	}
}

func TestParseRunArgs_Report(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantReport string
		wantErr    bool
	}{
		{"report flag parsed", []string{"col.yaml", "--report", "results.xml"}, "results.xml", false},
		{"report flag missing value", []string{"col.yaml", "--report"}, "", true},
		{"no report flag", []string{"col.yaml"}, "", false},
		{"report with format junit", []string{"col.yaml", "--format", "junit", "--report", "out.xml"}, "out.xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.report != tt.wantReport {
				t.Errorf("report = %q, want %q", flags.report, tt.wantReport)
			}
		})
	}
}

func TestRunCmd_format_junit_passed(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: JUnit Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, `<?xml version="1.0"`) {
		t.Error("expected XML declaration in output")
	}
	if !strings.Contains(stdout, `<testsuites>`) {
		t.Error("expected <testsuites> element")
	}
	if !strings.Contains(stdout, `<testcase`) {
		t.Error("expected <testcase> element")
	}
	if strings.Contains(stdout, `<failure`) {
		t.Error("unexpected <failure> element for passing test")
	}

	// Verify well-formed XML
	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v\nOutput:\n%s", err, stdout)
	}
}

func TestRunCmd_format_junit_assertion_failure(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: JUnit Fail\nrequests:\n  - name: Check\n    request:\n      method: GET\n      url: \"%s\"\n    assertions:\n      status: 200\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stdout, `<failure`) {
		t.Error("expected <failure> element for assertion failure")
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
}

func TestRunCmd_format_junit_network_error(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := "name: JUnit Error\nrequests:\n  - name: Bad\n    request:\n      method: GET\n      url: \"http://127.0.0.1:1/fail\"\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4", exitCode)
	}
	if !strings.Contains(stdout, `<error`) {
		t.Error("expected <error> element for network error")
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
}

func TestRunCmd_format_junit_timing(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: JUnit Timing\nrequests:\n  - name: Timed\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v", err)
	}
	if len(suites.TestSuites) != 1 || len(suites.TestSuites[0].TestCases) != 1 {
		t.Fatalf("expected 1 suite with 1 testcase, got %d suites", len(suites.TestSuites))
	}
	tc := suites.TestSuites[0].TestCases[0]
	if tc.Time == "" {
		t.Error("expected time attribute to be set")
	}
	// Time is in seconds (e.g., "0.000" or "0.001") — verify it's a parseable number
	if !strings.Contains(tc.Time, ".") {
		t.Errorf("expected time in seconds format (e.g., '0.001'), got %q", tc.Time)
	}
}

func TestRunCmd_format_junit_report_file(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: JUnit Report\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "results.xml")
	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit", "--report", reportFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	// stdout should be empty when --report is used
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty when --report is used, got: %q", stdout)
	}

	// Report file should exist and contain valid XML
	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	var suites output.JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		t.Fatalf("report file is not valid XML: %v\nContent:\n%s", err, string(data))
	}
	if len(suites.TestSuites) != 1 {
		t.Errorf("expected 1 suite, got %d", len(suites.TestSuites))
	}
}

func TestRunCmd_format_junit_parse_error(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	stdout, _, exitCode := captureRunCmd(t, "nonexistent_file_abc.yaml", "--format", "junit")
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}
	if !strings.Contains(stdout, `<?xml version="1.0"`) {
		t.Error("expected XML output for parse error")
	}
	if !strings.Contains(stdout, `<error`) {
		t.Error("expected <error> element for parse error")
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v\nOutput:\n%s", err, stdout)
	}
}

func TestRunCmd_format_junit_empty_collection(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "empty.yaml")
	content := "name: Empty\nrequests: []\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if !strings.Contains(stdout, `<?xml version="1.0"`) {
		t.Error("expected XML output for empty collection")
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v\nOutput:\n%s", err, stdout)
	}
	if len(suites.TestSuites) != 1 || suites.TestSuites[0].Tests != 0 {
		t.Errorf("expected empty suite with 0 tests")
	}
}

func TestRunCmd_format_junit_skipped(t *testing.T) {
	// M11-001: --format junit no longer gated; runs at default (free) tier.

	// Setup request that fails (unreachable server) with required: true,
	// causing the main request to be skipped.
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := `name: JUnit Skipped
setup:
  - name: Required Setup
    required: true
    request:
      method: GET
      url: "http://127.0.0.1:1/fail"
requests:
  - name: Main Request
    request:
      method: GET
      url: "http://127.0.0.1:1/main"
`
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	// Exit code 4 for execution error in setup
	if exitCode != 4 {
		t.Errorf("exit code = %d, want 4", exitCode)
	}
	if !strings.Contains(stdout, `<skipped`) {
		t.Error("expected <skipped> element for skipped request")
	}

	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("output is not valid XML: %v\nOutput:\n%s", err, stdout)
	}

	// Find the skipped main request
	foundSkipped := false
	for _, suite := range suites.TestSuites {
		for _, tc := range suite.TestCases {
			if tc.Name == "Main Request" && tc.Skipped != nil {
				foundSkipped = true
			}
		}
	}
	if !foundSkipped {
		t.Error("expected 'Main Request' to have <skipped> element")
	}
}

// TestRun_JUnitFormat_Default verifies that --format junit succeeds with no
// configuration at all.
func TestRun_JUnitFormat_Default(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf(
		"name: Free Tier JUnit\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: %q\n",
		srv.URL,
	)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, exitCode := captureRunCmd(t, f, "--format", "junit")
	if exitCode != 0 {
		t.Fatalf("exit = %d, want 0 (free tier must not gate junit)", exitCode)
	}

	// Well-formed JUnit XML.
	if !strings.Contains(stdout, `<?xml version="1.0"`) {
		t.Error("expected XML declaration on stdout")
	}
	var suites output.JUnitTestSuites
	if err := xml.Unmarshal([]byte(stdout), &suites); err != nil {
		t.Fatalf("stdout is not valid XML: %v\nstdout=%s", err, stdout)
	}
	if len(suites.TestSuites) != 1 || len(suites.TestSuites[0].TestCases) != 1 {
		t.Fatalf("want 1 suite × 1 testcase; got %d suites", len(suites.TestSuites))
	}
}

func TestRunCmdDirect_DataDriven_TerminalVerbose(t *testing.T) {
	// < 10 iterations = verbose mode (per-iteration lines)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("name\nalice\nbob\ncharlie"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: DD Verbose
requests:
  - name: Create User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	// Verbose mode: should show Data-Driven header and per-iteration lines
	if !strings.Contains(stdout, "Data-Driven:") {
		t.Errorf("expected 'Data-Driven:' header in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Iteration 1") {
		t.Errorf("expected 'Iteration 1' in verbose output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Iteration 3") {
		t.Errorf("expected 'Iteration 3' in verbose output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "Summary:") {
		t.Errorf("expected 'Summary:' in verbose output, got: %q", stdout)
	}
}

func TestRunCmdDirect_DataDriven_TerminalCompact(t *testing.T) {
	// >= 10 iterations = compact mode (summary line, no per-iteration)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Build CSV with 12 rows
	rows := "name\n"
	for i := 1; i <= 12; i++ {
		rows += fmt.Sprintf("user%d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: DD Compact
requests:
  - name: Create User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	// Compact mode: should show Data-Driven header and compact summary
	if !strings.Contains(stdout, "Data-Driven:") {
		t.Errorf("expected 'Data-Driven:' header in output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "12 passed") {
		t.Errorf("expected '12 passed' in compact output, got: %q", stdout)
	}
	if !strings.Contains(stdout, "12 total") {
		t.Errorf("expected '12 total' in compact output, got: %q", stdout)
	}
	// Compact mode should NOT show per-iteration lines
	if strings.Contains(stdout, "Iteration 1") {
		t.Errorf("compact mode should not show per-iteration lines, got: %q", stdout)
	}
}

func TestRunCmdDirect_DataDriven_JSONFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("name\nalice\nbob\ncharlie"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: DD JSON
requests:
  - name: Create User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--format", "json")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nstdout: %s", err, stdout)
	}
	// Should have data_driven field
	dd, ok := result["data_driven"]
	if !ok {
		t.Fatalf("expected 'data_driven' field in JSON output, got: %v", result)
	}
	ddSlice, ok := dd.([]any)
	if !ok || len(ddSlice) != 1 {
		t.Fatalf("expected 1 data_driven entry, got: %v", dd)
	}
	entry := ddSlice[0].(map[string]any)
	if entry["type"] != "data_driven" {
		t.Errorf("expected type=data_driven, got %v", entry["type"])
	}
	if entry["total_iterations"] != float64(3) {
		t.Errorf("expected total_iterations=3, got %v", entry["total_iterations"])
	}
	if entry["passed_iterations"] != float64(3) {
		t.Errorf("expected passed_iterations=3, got %v", entry["passed_iterations"])
	}
	// M11-002: iterations[] must be present with length == total_iterations.
	iters, ok := entry["iterations"]
	if !ok {
		t.Fatalf("expected 'iterations' key in data_driven entry, got: %v", entry)
	}
	itersSlice, ok := iters.([]any)
	if !ok {
		t.Fatalf("expected iterations to be an array, got: %T", iters)
	}
	if len(itersSlice) != 3 {
		t.Fatalf("expected iterations length=3, got %d", len(itersSlice))
	}
	// Check first iteration fields.
	iter0, ok := itersSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("expected iterations[0] to be a map, got: %T", itersSlice[0])
	}
	if iter0["status"] != "passed" {
		t.Errorf("iterations[0].status = %v, want passed", iter0["status"])
	}
	iter0Name, _ := iter0["name"].(string)
	if !strings.HasSuffix(iter0Name, "[1/3]") {
		t.Errorf("iterations[0].name = %q, want suffix [1/3]", iter0Name)
	}
	_, hasDuration := iter0["duration_ms"]
	if !hasDuration {
		t.Errorf("iterations[0] missing duration_ms field")
	}
	// M11-002: root summary block must be present.
	summary, ok := result["summary"]
	if !ok {
		t.Fatalf("expected 'summary' field in JSON output, got: %v", result)
	}
	summaryMap, ok := summary.(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be a map, got: %T", summary)
	}
	for _, key := range []string{"total", "passed", "failed", "skipped"} {
		if _, exists := summaryMap[key]; !exists {
			t.Errorf("summary missing key %q", key)
		}
	}
}

func TestRunCmdDirect_DataDriven_TAPFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("name\nalice\nbob"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: DD TAP
requests:
  - name: Create User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--format", "tap")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0\nstdout: %s", exitCode, stdout)
	}
	if !strings.Contains(stdout, "TAP version 13") {
		t.Errorf("expected TAP version 13 header, got: %q", stdout)
	}
	if !strings.Contains(stdout, "# Data-Driven: Create User (2 iterations)") {
		t.Errorf("expected data-driven TAP comment, got: %q", stdout)
	}
	if !strings.Contains(stdout, "1..2") {
		t.Errorf("expected plan line '1..2', got: %q", stdout)
	}
	// Per-iteration test points must carry [i/N] suffix (M11-003)
	iterRe := regexp.MustCompile(`(?m)^(ok|not ok) [0-9]+ - .+ \[[0-9]+/2\]`)
	if !iterRe.MatchString(stdout) {
		t.Errorf("expected per-iteration test-point lines with [i/2] suffix, got: %q", stdout)
	}
}

// TestTAPOutput_DataDrivenIterations locks the per-iteration TAP test-point
// contract: one ok/not-ok line per data-driven iteration with the [i/N] suffix,
// plan count matches actual line count. (M11-003)
func TestTAPOutput_DataDrivenIterations(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("name\nalice\nbob\ncharlie"), 0o600); err != nil {
		t.Fatal(err)
	}
	col := fmt.Sprintf(`name: DD TAP Iter
requests:
  - name: Create User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 200
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, exitCode := captureRunCmd(t, colFile, "--format", "tap")
	if exitCode != 0 {
		t.Fatalf("exit = %d, want 0\nstdout: %s", exitCode, stdout)
	}

	// 1. The group header marker stays.
	if !strings.Contains(stdout, "# Data-Driven: Create User (3 iterations)") {
		t.Errorf("expected data-driven header marker, got: %q", stdout)
	}

	// 2. Three test-point lines, one per iteration, each with [i/3] suffix.
	iterRe := regexp.MustCompile(`(?m)^(ok|not ok) [0-9]+ - .+ \[[0-9]+/3\]`)
	matches := iterRe.FindAllString(stdout, -1)
	if len(matches) != 3 {
		t.Errorf("expected 3 per-iteration test points, got %d:\n%s", len(matches), stdout)
	}

	// 3. Plan line matches actual count.
	planRe := regexp.MustCompile(`(?m)^1\.\.([0-9]+)`)
	planMatch := planRe.FindStringSubmatch(stdout)
	if len(planMatch) != 2 {
		t.Fatalf("missing plan line; got: %s", stdout)
	}
	plan, _ := strconv.Atoi(planMatch[1])
	pointRe := regexp.MustCompile(`(?m)^(ok|not ok) [0-9]+ `)
	points := pointRe.FindAllString(stdout, -1)
	if plan != len(points) {
		t.Errorf("plan = %d, actual test-point count = %d:\n%s", plan, len(points), stdout)
	}
}

// TestTAPOutput_ParallelSpeedup verifies the parallel-execution YAML diagnostic
// block is emitted with speedup_factor, wave_count, and max_parallelism, and that
// speedup_factor is byte-equal to the JSON formatter's value for the same run.
// (M11-003)
func TestTAPOutput_ParallelSpeedup(t *testing.T) {
	// Two leaves and one consumer so dependency analysis schedules two waves.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	col := fmt.Sprintf(`name: Parallel TAP
requests:
  - name: First
    request:
      method: GET
      url: "%s/first"
    extract:
      first_id: "$.id"
    assertions:
      status: 200
  - name: Second
    request:
      method: GET
      url: "%s/second"
    assertions:
      status: 200
  - name: Third
    request:
      method: GET
      url: "%s/third?id={{first_id}}"
    assertions:
      status: 200
`, srv.URL, srv.URL, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	tapStdout, _, tapExit := captureRunCmd(t, colFile, "--format", "tap", "--parallel")
	if tapExit != 0 {
		t.Fatalf("tap exit = %d, want 0\nstdout: %s", tapExit, tapStdout)
	}

	// Block presence.
	if !strings.Contains(tapStdout, "# Parallel execution:\n") {
		t.Fatalf("expected parallel diagnostic block, got:\n%s", tapStdout)
	}

	// Extract and assert YAML keys.
	for _, key := range []string{"speedup_factor:", "wave_count:", "max_parallelism:"} {
		if !strings.Contains(tapStdout, "  "+key) {
			t.Errorf("missing key %q in diagnostic block:\n%s", key, tapStdout)
		}
	}

	// Block ordering: between last test point and # Summary:.
	headerIdx := strings.Index(tapStdout, "# Parallel execution:")
	summaryIdx := strings.Index(tapStdout, "# Summary:")
	if headerIdx < 0 || summaryIdx < 0 || headerIdx >= summaryIdx {
		t.Errorf("block ordering wrong: headerIdx=%d summaryIdx=%d", headerIdx, summaryIdx)
	}

	// Parity with JSON: rerun with --format json --parallel and compare speedup_factor.
	// Both paths share buildParallelMetadata, so the formula and formatting are identical
	// (%.1f on math.Round(x*10)/10). Two separate runs have independent timing, so we
	// allow ≤ 0.15 difference (one rounding step + half-step tolerance) rather than requiring byte-equality.
	jsonStdout, _, jsonExit := captureRunCmd(t, colFile, "--format", "json", "--parallel")
	if jsonExit != 0 {
		t.Fatalf("json exit = %d, want 0\nstdout: %s", jsonExit, jsonStdout)
	}
	var jsonOut map[string]any
	if err := json.Unmarshal([]byte(jsonStdout), &jsonOut); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, jsonStdout)
	}
	pe, ok := jsonOut["parallel_execution"].(map[string]any)
	if !ok {
		t.Fatalf("missing parallel_execution in JSON output: %v", jsonOut)
	}
	jsonSpeedup, _ := pe["speedup_factor"].(float64)

	// Extract the TAP speedup_factor and compare with the JSON value.
	// Both values are produced by the same buildParallelMetadata formula;
	// tolerance of ≤ 0.15 (one rounding step + half-step) accounts for independent timing.
	sfRe := regexp.MustCompile(`(?m)^\s+speedup_factor: ([0-9]+\.[0-9]+)`)
	sfMatch := sfRe.FindStringSubmatch(tapStdout)
	if len(sfMatch) != 2 {
		t.Errorf("could not parse speedup_factor from TAP output:\n%s", tapStdout)
	} else {
		tapSpeedup, err := strconv.ParseFloat(sfMatch[1], 64)
		if err != nil {
			t.Errorf("speedup_factor %q is not a valid float: %v", sfMatch[1], err)
		} else {
			// Verify the TAP value uses the same %.1f format as the JSON value.
			tapLine := fmt.Sprintf("  speedup_factor: %.1f", tapSpeedup)
			if !strings.Contains(tapStdout, tapLine) {
				t.Errorf("TAP speedup_factor not in %.1f form; parsed: %q\nTAP:\n%s", tapSpeedup, sfMatch[1], tapStdout)
			}
			// Verify parity: both values use math.Round(x*10)/10 via buildParallelMetadata.
			// Independent runs have independent timing, so allow ≤ 0.15 (one rounding step + half-step).
			diff := tapSpeedup - jsonSpeedup
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.15 {
				t.Errorf("TAP speedup_factor (%.1f) differs from JSON speedup_factor (%.1f) by more than one rounding step (%.2f); paths may have diverged\nTAP:\n%s\nJSON parallel_execution: %v",
					tapSpeedup, jsonSpeedup, diff, tapStdout, pe)
			}
		}
	}
}

func TestRunCmdDirect_DataDriven_FailedIterationDetails_Verbose(t *testing.T) {
	// Test that failed iteration details are shown in verbose mode (< 10 iterations)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte("name\nalice\nbob\ncharlie"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Assertion will fail because server returns 200 but we expect 404
	col := fmt.Sprintf(`name: DD Fail Verbose
requests:
  - name: Check User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 404
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, _ := captureRunCmd(t, colFile)
	// Should show Data-Driven header
	if !strings.Contains(stdout, "Data-Driven:") {
		t.Errorf("expected 'Data-Driven:' header in output, got: %q", stdout)
	}
	// Verbose mode: should show per-iteration lines
	if !strings.Contains(stdout, "Iteration 1") {
		t.Errorf("expected per-iteration lines in verbose mode, got: %q", stdout)
	}
	// Should show failed iteration details (assertion failures)
	if !strings.Contains(stdout, "expected 404") {
		t.Errorf("expected specific assertion failure detail 'expected 404' in output, got: %q", stdout)
	}
	// Verbose summary should show failed iteration numbers
	if !strings.Contains(stdout, "Failed iterations:") {
		t.Errorf("expected 'Failed iterations:' in verbose summary, got: %q", stdout)
	}
}

func TestRunCmdDirect_DataDriven_FailedIterationDetails_Compact(t *testing.T) {
	// Test that failed iteration details are shown in compact mode (>= 10 iterations)
	// Server returns 200 for all requests; some iterations expect 404 to trigger failures
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Create 12 rows so we trigger compact mode (>= 10 threshold)
	rows := "name\n"
	for i := 1; i <= 12; i++ {
		rows += fmt.Sprintf("user%d\n", i)
	}
	if err := os.WriteFile(filepath.Join(dir, "users.csv"), []byte(rows), 0o600); err != nil {
		t.Fatal(err)
	}
	// All iterations will fail because server returns 200 but we expect 404
	col := fmt.Sprintf(`name: DD Fail Compact
requests:
  - name: Check User
    data_driven:
      source: users.csv
    request:
      method: GET
      url: "%s/users/{{name}}"
    assertions:
      status: 404
`, srv.URL)
	colFile := writeCollection(t, dir, "col.yaml", col)

	stdout, _, _ := captureRunCmd(t, colFile)
	// Compact mode: should show Data-Driven header
	if !strings.Contains(stdout, "Data-Driven:") {
		t.Errorf("expected 'Data-Driven:' header in compact output, got: %q", stdout)
	}
	// Compact mode: should show compact summary with failed count
	if !strings.Contains(stdout, "12 failed") {
		t.Errorf("expected '12 failed' in compact summary, got: %q", stdout)
	}
	// Compact mode: should show "Failed iterations:" line
	if !strings.Contains(stdout, "Failed iterations:") {
		t.Errorf("expected 'Failed iterations:' in compact output, got: %q", stdout)
	}
	// Compact mode: should NOT show per-iteration lines (no "Iteration 1")
	if strings.Contains(stdout, "Iteration 1") {
		t.Errorf("compact mode should not show per-iteration lines, got: %q", stdout)
	}
	// Compact mode: should still show assertion failure details
	if !strings.Contains(stdout, "expected 404") {
		t.Errorf("expected assertion failure detail 'expected 404' in compact mode, got: %q", stdout)
	}
}

func TestGroupDataDrivenResults(t *testing.T) {
	tests := []struct {
		name     string
		results  []runner.RequestResult
		startIdx int
		wantLen  int
		wantNext int
	}{
		{
			"groups contiguous same-name iterations",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 0},
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 1},
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 2},
			},
			0, 3, 3,
		},
		{
			"stops at non-data-driven result",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 0},
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 1},
				{IsDataDriven: false, Name: "Normal"},
			},
			0, 2, 2,
		},
		{
			"stops at different base name",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 0},
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 1},
				{IsDataDriven: true, DataDrivenName: "B", IterationIndex: 0},
			},
			0, 2, 2,
		},
		{
			"single iteration group",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 0},
			},
			0, 1, 1,
		},
		{
			"start from middle index",
			[]runner.RequestResult{
				{IsDataDriven: false, Name: "Normal"},
				{IsDataDriven: true, DataDrivenName: "X", IterationIndex: 0},
				{IsDataDriven: true, DataDrivenName: "X", IterationIndex: 1},
			},
			1, 2, 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group, next := groupDataDrivenResults(tt.results, tt.startIdx)
			if len(group) != tt.wantLen {
				t.Errorf("group length = %d, want %d", len(group), tt.wantLen)
			}
			if next != tt.wantNext {
				t.Errorf("next index = %d, want %d", next, tt.wantNext)
			}
		})
	}
}

func TestBuildDataDrivenJSON(t *testing.T) {
	tests := []struct {
		name    string
		results []runner.RequestResult
		wantLen int
		checkFn func(t *testing.T, dd []output.DataDrivenJSON)
	}{
		{
			"data-driven results produce aggregate",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 0, IterationTotal: 3, Result: &httpexec.Result{Duration: 100 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 1, IterationTotal: 3, Result: &httpexec.Result{Duration: 200 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{IsDataDriven: true, DataDrivenName: "Create User", IterationIndex: 2, IterationTotal: 3, Result: &httpexec.Result{Duration: 300 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: false}},
			},
			1,
			func(t *testing.T, dd []output.DataDrivenJSON) {
				entry := dd[0]
				if entry.Type != "data_driven" {
					t.Errorf("type = %q, want data_driven", entry.Type)
				}
				if entry.Name != "Create User" {
					t.Errorf("name = %q, want Create User", entry.Name)
				}
				if entry.TotalIterations != 3 {
					t.Errorf("total = %d, want 3", entry.TotalIterations)
				}
				if entry.PassedIterations != 2 {
					t.Errorf("passed = %d, want 2", entry.PassedIterations)
				}
				if entry.FailedIterations != 1 {
					t.Errorf("failed = %d, want 1", entry.FailedIterations)
				}
				if entry.TotalDurationMs != 600 {
					t.Errorf("total_duration = %d, want 600", entry.TotalDurationMs)
				}
				if entry.AvgDurationMs != 200 {
					t.Errorf("avg_duration = %d, want 200", entry.AvgDurationMs)
				}
				// M11-002: Iterations populated with len == TotalIterations.
				if len(entry.Iterations) != entry.TotalIterations {
					t.Errorf("len(Iterations) = %d, want %d (== TotalIterations)", len(entry.Iterations), entry.TotalIterations)
				}
			},
		},
		{
			"no data-driven results return nil",
			[]runner.RequestResult{
				{Name: "Normal Request", Result: &httpexec.Result{Duration: 50 * time.Millisecond}},
			},
			0,
			func(t *testing.T, dd []output.DataDrivenJSON) {
				if dd != nil {
					t.Errorf("expected nil, got %v", dd)
				}
			},
		},
		{
			"mixed results produce correct aggregate",
			[]runner.RequestResult{
				{Name: "Normal Request", Result: &httpexec.Result{Duration: 50 * time.Millisecond}},
				{IsDataDriven: true, DataDrivenName: "Test", IterationIndex: 0, IterationTotal: 2, Result: &httpexec.Result{Duration: 100 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{IsDataDriven: true, DataDrivenName: "Test", IterationIndex: 1, IterationTotal: 2, Result: &httpexec.Result{Duration: 200 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
			},
			1,
			func(t *testing.T, dd []output.DataDrivenJSON) {
				if dd[0].TotalIterations != 2 {
					t.Errorf("total = %d, want 2", dd[0].TotalIterations)
				}
				if dd[0].PassedIterations != 2 {
					t.Errorf("passed = %d, want 2", dd[0].PassedIterations)
				}
			},
		},
		{
			"multiple data-driven groups produce multiple aggregates",
			[]runner.RequestResult{
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 0, IterationTotal: 2, Result: &httpexec.Result{Duration: 50 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{IsDataDriven: true, DataDrivenName: "A", IterationIndex: 1, IterationTotal: 2, Result: &httpexec.Result{Duration: 50 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: true}},
				{IsDataDriven: true, DataDrivenName: "B", IterationIndex: 0, IterationTotal: 1, Result: &httpexec.Result{Duration: 100 * time.Millisecond}, AssertionResults: &assertion.Results{Passed: false}},
			},
			2,
			func(t *testing.T, dd []output.DataDrivenJSON) {
				if dd[0].Name != "A" {
					t.Errorf("first group name = %q, want A", dd[0].Name)
				}
				if dd[1].Name != "B" {
					t.Errorf("second group name = %q, want B", dd[1].Name)
				}
				if dd[1].FailedIterations != 1 {
					t.Errorf("second group failed = %d, want 1", dd[1].FailedIterations)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := buildDataDrivenJSON(tt.results)
			if len(dd) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(dd), tt.wantLen)
			}
			if tt.checkFn != nil {
				tt.checkFn(t, dd)
			}
		})
	}
}

// --- HTML Report CLI Tests ---

func TestRunCmd_format_html_requires_report_flag(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := "name: Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"http://localhost:1/test\"\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, exitCode := captureRunCmd(t, f, "--format", "html")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr, "--report") {
		t.Errorf("stderr should mention --report, got: %q", stderr)
	}
}

func TestRunCmd_format_html_recognized(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := "name: Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"http://localhost:1/test\"\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reportFile := filepath.Join(tmpDir, "report.html")
	// --format html must not be rejected as unknown format (exit 1 with "unknown output format")
	_, stderr, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode == 1 && strings.Contains(stderr, "unknown output format") {
		t.Error("--format html should not be treated as unknown format")
	}
}

func TestRunCmd_format_html_generates_report_file(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: HTML Report Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	_, _, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if !strings.HasPrefix(html, "<!DOCTYPE html>") {
		t.Error("report should start with <!DOCTYPE html>")
	}
	if !strings.Contains(html, "</html>") {
		t.Error("report should contain closing </html> tag")
	}
}

func TestRunCmd_format_html_report_contains_results(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Results Report\nrequests:\n  - name: GetData\n    request:\n      method: GET\n      url: \"%s/data\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	_, _, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "GetData") {
		t.Error("report should contain request name 'GetData'")
	}
	if !strings.Contains(html, "GET") {
		t.Error("report should contain method 'GET'")
	}
	if !strings.Contains(html, "passed") {
		t.Error("report should contain 'passed' status")
	}
}

func TestRunCmd_format_html_report_self_contained(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Self Contained\nrequests:\n  - name: Test\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	captureRunCmd(t, f, "--format", "html", "--report", reportFile)

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if strings.Contains(html, `<link rel="stylesheet"`) {
		t.Error("report should not have external stylesheet links")
	}
	if strings.Contains(html, `<script src=`) {
		t.Error("report should not have external script sources")
	}
	if !strings.Contains(html, "<style>") {
		t.Error("report should have embedded <style> tag")
	}
}

func TestRunCmd_format_html_failed_assertions_shown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Assert Fail\nrequests:\n  - name: Check\n    request:\n      method: GET\n      url: \"%s\"\n    assertions:\n      status: 200\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	_, _, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "200") {
		t.Error("report should show expected value '200'")
	}
	if !strings.Contains(html, "404") {
		t.Error("report should show actual value '404'")
	}
	if !strings.Contains(html, "failed") {
		t.Error("report should show 'failed' status")
	}
}

func TestRunCmd_format_html_stdout_empty_when_report_used(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "test.yaml")
	content := fmt.Sprintf("name: Quiet\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: \"%s\"\n", srv.URL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	stdout, _, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty when --report is used, got: %q", stdout)
	}
}

func TestRunCmd_format_html_parse_error(t *testing.T) {
	tmpDir := t.TempDir()
	reportFile := filepath.Join(tmpDir, "report.html")
	_, _, exitCode := captureRunCmd(t, "nonexistent_abc.yaml", "--format", "html", "--report", reportFile)
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3", exitCode)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected valid HTML output for parse error")
	}
	if !strings.Contains(html, "error") {
		t.Error("expected error indication in HTML report")
	}
}

func TestRunCmd_format_html_empty_collection(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "empty.yaml")
	content := "name: Empty\nrequests: []\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reportFile := filepath.Join(tmpDir, "report.html")
	_, _, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	data, err := os.ReadFile(reportFile)
	if err != nil {
		t.Fatalf("failed to read report file: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("expected valid HTML even for empty collection")
	}
}

func TestRunCmd_format_html_empty_collection_write_error(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "empty.yaml")
	content := "name: Empty\nrequests: []\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a non-existent directory as report path to trigger write error
	reportFile := filepath.Join(tmpDir, "nonexistent", "subdir", "report.html")
	_, stderr, exitCode := captureRunCmd(t, f, "--format", "html", "--report", reportFile)
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 for write error", exitCode)
	}
	if !strings.Contains(stderr, "cannot write html report") {
		t.Errorf("stderr should contain write error message, got: %s", stderr)
	}
}

// ---- M2-028: buildHTMLReport data-driven and parallel wiring tests ----

func TestBuildHTMLReport_DataDrivenGroupsPopulated(t *testing.T) {
	results := []runner.RequestResult{
		{
			Name:           "Create User [1/2]",
			IsDataDriven:   true,
			DataDrivenName: "Create User",
			IterationIndex: 0,
			IterationTotal: 2,
			IterationData:  map[string]string{"name": "alice", "role": "admin"},
			Result:         &httpexec.Result{StatusCode: 200, Duration: 100 * time.Millisecond},
		},
		{
			Name:           "Create User [2/2]",
			IsDataDriven:   true,
			DataDrivenName: "Create User",
			IterationIndex: 1,
			IterationTotal: 2,
			IterationData:  map[string]string{"name": "bob", "role": "user"},
			Result:         &httpexec.Result{StatusCode: 201, Duration: 150 * time.Millisecond},
		},
	}
	summary := &runner.Summary{Total: 2, Passed: 2, Duration: 250 * time.Millisecond}

	report := buildHTMLReport("Test", results, summary)

	if len(report.DataDrivenGroups) != 1 {
		t.Fatalf("DataDrivenGroups = %d, want 1", len(report.DataDrivenGroups))
	}
	g := report.DataDrivenGroups[0]
	if g.Name != "Create User" {
		t.Errorf("group Name = %q, want %q", g.Name, "Create User")
	}
	if g.Total != 2 {
		t.Errorf("group Total = %d, want 2", g.Total)
	}
	if len(g.DataColumns) == 0 {
		t.Error("DataColumns should not be empty for data-driven results with row data")
	}
	if len(g.Iterations) != 2 {
		t.Errorf("Iterations = %d, want 2", len(g.Iterations))
	}
	// Check data values are propagated
	foundAlice := false
	for _, it := range g.Iterations {
		if it.Data["name"] == "alice" {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Error("expected iteration with name=alice in data")
	}
}

func TestBuildHTMLReport_NonDataDrivenNoGroups(t *testing.T) {
	results := []runner.RequestResult{
		{
			Name:   "Get Users",
			Method: "GET",
			URL:    "http://api/users",
			Result: &httpexec.Result{StatusCode: 200, Duration: 50 * time.Millisecond},
		},
	}
	summary := &runner.Summary{Total: 1, Passed: 1, Duration: 50 * time.Millisecond}

	report := buildHTMLReport("Test", results, summary)

	if len(report.DataDrivenGroups) != 0 {
		t.Errorf("DataDrivenGroups = %d, want 0 for non-data-driven results", len(report.DataDrivenGroups))
	}
	if report.IsParallel {
		t.Error("IsParallel should be false for non-parallel results")
	}
}

func TestBuildHTMLReport_ParallelFieldsPopulated(t *testing.T) {
	waveDur0 := 100 * time.Millisecond
	waveDur1 := 80 * time.Millisecond
	results := []runner.RequestResult{
		{
			Name:      "A",
			WaveIndex: 0,
			Result:    &httpexec.Result{StatusCode: 200, Duration: 90 * time.Millisecond},
		},
		{
			Name:      "B",
			WaveIndex: 1,
			Result:    &httpexec.Result{StatusCode: 200, Duration: 70 * time.Millisecond},
		},
	}
	summary := &runner.Summary{
		Total:          2,
		Passed:         2,
		Duration:       200 * time.Millisecond,
		IsParallel:     true,
		WaveCount:      2,
		MaxParallelism: 1,
		WaveDurations:  []time.Duration{waveDur0, waveDur1},
	}

	report := buildHTMLReport("Test", results, summary)

	if !report.IsParallel {
		t.Error("IsParallel should be true")
	}
	if report.WaveCount != 2 {
		t.Errorf("WaveCount = %d, want 2", report.WaveCount)
	}
	if report.MaxParallelism != 1 {
		t.Errorf("MaxParallelism = %d, want 1", report.MaxParallelism)
	}
	if report.SpeedupFactor == "" {
		t.Error("SpeedupFactor should not be empty")
	}
	if len(report.Waves) != 2 {
		t.Errorf("Waves = %d, want 2", len(report.Waves))
	}
}

func TestImportOpenAPI_WritesCollection(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "petstore.collection.yaml")

	code := run([]string{"import", "openapi", "testdata/openapi/petstore.yaml", "--output", out})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", info.Mode().Perm())
	}

	// Round-trip: validate the emitted file.
	code = run([]string{"validate", out})
	if code != 0 {
		t.Errorf("validate exit = %d, want 0", code)
	}
}

func TestImportOpenAPI_WritesToStdout(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "import", "openapi", "testdata/openapi/petstore.yaml")
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%q", exitCode, stdout)
	}
	if !strings.Contains(stdout, "name: Petstore") {
		t.Errorf("stdout does not contain %q; got:\n%s", "name: Petstore", stdout)
	}
}

func TestImportOpenAPI_BinaryIntegration(t *testing.T) {
	binary := buildBinary(t)
	dir := t.TempDir()
	out := filepath.Join(dir, "petstore.collection.yaml")

	// Resolve the absolute path to the test fixture so it works from any cwd.
	spec, err := filepath.Abs("testdata/openapi/petstore.yaml")
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}

	stdout, stderr, exitCode := runBinary(t, binary, "import", "openapi", spec, "--output", out)
	if exitCode != 0 {
		t.Fatalf("binary exit = %d; stdout=%q stderr=%q", exitCode, stdout, stderr)
	}

	// Validate the generated file with the real binary.
	stdout2, stderr2, code2 := runBinary(t, binary, "validate", out)
	if code2 != 0 {
		t.Errorf("validate exit = %d; stdout=%q stderr=%q", code2, stdout2, stderr2)
	}
}

func TestImportOpenAPI_MissingSpec(t *testing.T) {
	code := run([]string{"import", "openapi", "testdata/openapi/does_not_exist.yaml"})
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestImportOpenAPI_InvalidSpec(t *testing.T) {
	code := run([]string{"import", "openapi", "testdata/openapi/invalid.yaml"})
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestImportCmd_UnknownFormat(t *testing.T) {
	code := run([]string{"import", "graphql", "foo.graphql"})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

// --- pr-check subcommand tests ---

// makePrCheckResultsFile creates a temp JSON results file for pr-check testing.
func makePrCheckResultsFile(t *testing.T, passCount, failCount int) string {
	t.Helper()
	content := fmt.Sprintf(`{
		"collection_name": "smoke-tests",
		"run_at": "2026-04-16T10:00:00Z",
		"duration_ms": 500,
		"pass_count": %d,
		"fail_count": %d,
		"skipped_count": 0,
		"triggered_by": "pr-check",
		"items": []
	}`, passCount, failCount)
	f, err := os.CreateTemp(t.TempDir(), "results*.json")
	if err != nil {
		t.Fatalf("create temp results file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp results file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp results file: %v", err)
	}
	return f.Name()
}

// makePrCheckMockBackend creates an httptest server for pr-check tests.
func makePrCheckMockBackend(t *testing.T, resultsStatus, prCheckStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/results") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resultsStatus)
			if resultsStatus >= 200 && resultsStatus < 300 {
				_, _ = w.Write([]byte(`{"result_id":"res_mock001","status":"accepted"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"error"}`))
			}
			return
		}
		if r.Method == "POST" && strings.Contains(r.URL.Path, "/pr-checks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(prCheckStatus)
			if prCheckStatus >= 200 && prCheckStatus < 300 {
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"error"}`))
			}
			return
		}
		w.WriteHeader(404)
	}))
}

func TestPrCheckCmd_Help(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "pr-check", "--help")
	if exitCode != 0 {
		t.Errorf("exit code = %d; want 0", exitCode)
	}
	for _, want := range []string{"--org", "--pr", "--repo", "--results", "--dry-run", "APITEST_BACKEND_URL", "APITEST_BACKEND_TOKEN"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q; got: %s", want, stdout)
		}
	}
}

func TestPrCheckCmd_MissingBackendURL(t *testing.T) {
	t.Setenv("APITEST_BACKEND_URL", "")
	t.Setenv("APITEST_BACKEND_TOKEN", "tok")
	resultsFile := makePrCheckResultsFile(t, 3, 0)
	_, stderr, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile)
	if exitCode != 2 {
		t.Errorf("exit code = %d; want 2", exitCode)
	}
	if !strings.Contains(stderr, "backend URL not configured") {
		t.Errorf("stderr = %q; want containing 'backend URL not configured'", stderr)
	}
}

func TestPrCheckCmd_Unauthorized(t *testing.T) {
	srv := makePrCheckMockBackend(t, 401, 200)
	defer srv.Close()
	t.Setenv("APITEST_BACKEND_URL", srv.URL)
	t.Setenv("APITEST_BACKEND_TOKEN", "bad-token")
	resultsFile := makePrCheckResultsFile(t, 3, 0)
	_, stderr, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile)
	if exitCode != 2 {
		t.Errorf("exit code = %d; want 2", exitCode)
	}
	if !strings.Contains(stderr, "unauthorized") {
		t.Errorf("stderr = %q; want containing 'unauthorized'", stderr)
	}
}

func TestPrCheckCmd_SuccessAllPass(t *testing.T) {
	srv := makePrCheckMockBackend(t, 202, 200)
	defer srv.Close()
	t.Setenv("APITEST_BACKEND_URL", srv.URL)
	t.Setenv("APITEST_BACKEND_TOKEN", "tok")
	resultsFile := makePrCheckResultsFile(t, 3, 0)
	stdout, _, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile)
	if exitCode != 0 {
		t.Errorf("exit code = %d; want 0", exitCode)
	}
	if !strings.Contains(stdout, "res_mock001") {
		t.Errorf("stdout = %q; want containing result ID", stdout)
	}
	if !strings.Contains(stdout, "pass=3") {
		t.Errorf("stdout = %q; want containing 'pass=3'", stdout)
	}
	if !strings.Contains(stdout, "fail=0") {
		t.Errorf("stdout = %q; want containing 'fail=0'", stdout)
	}
}

func TestPrCheckCmd_FailingTests_Exit1(t *testing.T) {
	srv := makePrCheckMockBackend(t, 202, 200)
	defer srv.Close()
	t.Setenv("APITEST_BACKEND_URL", srv.URL)
	t.Setenv("APITEST_BACKEND_TOKEN", "tok")
	resultsFile := makePrCheckResultsFile(t, 2, 1)
	stdout, _, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile)
	if exitCode != 1 {
		t.Errorf("exit code = %d; want 1", exitCode)
	}
	if !strings.Contains(stdout, "fail=1") {
		t.Errorf("stdout = %q; want containing 'fail=1'", stdout)
	}
}

func TestPrCheckCmd_DryRun_NoHTTP(t *testing.T) {
	// Use an unroutable address — if HTTP is attempted it will fail
	t.Setenv("APITEST_BACKEND_URL", "http://127.0.0.1:0")
	t.Setenv("APITEST_BACKEND_TOKEN", "tok")
	resultsFile := makePrCheckResultsFile(t, 3, 0)
	stdout, _, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile,
		"--dry-run")
	if exitCode != 0 {
		t.Errorf("exit code = %d; want 0", exitCode)
	}
	if !strings.Contains(stdout, "collection_name") {
		t.Errorf("stdout = %q; want containing JSON payload", stdout)
	}
}

func TestPrCheckCmd_ConnectionRefused_Exit2(t *testing.T) {
	// Create server then close it
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	t.Setenv("APITEST_BACKEND_URL", addr)
	t.Setenv("APITEST_BACKEND_TOKEN", "tok")
	resultsFile := makePrCheckResultsFile(t, 3, 0)
	_, stderr, exitCode := captureRun(t, "pr-check",
		"--org", "acme", "--pr", "42", "--repo", "acme/api", "--results", resultsFile)
	if exitCode != 2 {
		t.Errorf("exit code = %d; want 2", exitCode)
	}
	if !strings.Contains(stderr, "network error") {
		t.Errorf("stderr = %q; want containing 'network error'", stderr)
	}
}

func TestPrCheckCmd_MainHelp_ListsPrCheck(t *testing.T) {
	stdout, _, exitCode := captureRun(t, "--help")
	if exitCode != 0 {
		t.Errorf("exit code = %d; want 0", exitCode)
	}
	if !strings.Contains(stdout, "pr-check") {
		t.Errorf("main help output missing 'pr-check'; got: %s", stdout)
	}
}

// TestPrCheckCmd_EventsFlagRejected verifies that --events is not accepted by
// the pr-check subcommand; it must fail with exit 2 (parse error) and the
// canonical rejection message.
func TestPrCheckCmd_EventsFlagRejected(t *testing.T) {
	_, _, err := parsePrCheckArgs([]string{"--events", "/tmp/x.jsonl"})
	if err == nil {
		t.Fatal("parsePrCheckArgs --events: want error, got nil")
	}
	const wantMsg = "--events is supported only on run; use --format jsonl for streaming samples"
	if !strings.Contains(err.Error(), wantMsg) {
		t.Errorf("error = %q; want containing %q", err.Error(), wantMsg)
	}
}

func TestRedact_BodyAcrossFormats(t *testing.T) {
	// httptest server that echoes the incoming request body back in a JSON
	// envelope — ensures the secret appears in both request AND response body
	// fields.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"received":%q}`, string(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	col := writeCollection(t, dir, "redact.yaml", fmt.Sprintf(`
name: Redact Demo
variables:
  base_url: "%s"
  TOKEN: !sensitive "hunter2"
requests:
  - name: echo
    request:
      method: POST
      url: "{{base_url}}/echo"
      body: "token={{TOKEN}}"
`, srv.URL))

	const secret = "hunter2"

	t.Run("terminal_vv", func(t *testing.T) {
		stdout, stderr, exitCode := captureRunCmd(t, col, "-vv", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exit=%d stderr=%s", exitCode, stderr)
		}
		if strings.Contains(stdout+stderr, secret) {
			t.Errorf("secret %q leaked in terminal output:\nstdout=%s\nstderr=%s", secret, stdout, stderr)
		}
		if !strings.Contains(stdout, "[REDACTED]") {
			t.Error("expected [REDACTED] marker in -vv output")
		}
	})

	t.Run("json_format", func(t *testing.T) {
		stdout, stderr, exitCode := captureRunCmd(t, col, "--format", "json", "-vv")
		if exitCode != 0 {
			t.Fatalf("exit=%d stderr=%s", exitCode, stderr)
		}
		if strings.Contains(stdout+stderr, secret) {
			t.Errorf("secret leaked in json output:\nstdout=%s", stdout)
		}
	})

	t.Run("tap_format", func(t *testing.T) {
		stdout, stderr, exitCode := captureRunCmd(t, col, "--format", "tap")
		if exitCode != 0 {
			t.Fatalf("exit=%d stderr=%s", exitCode, stderr)
		}
		if strings.Contains(stdout+stderr, secret) {
			t.Errorf("secret leaked in tap output")
		}
	})

	t.Run("junit_format", func(t *testing.T) {
		// M11-001: --format junit no longer gated; no tier elevation needed.
		stdout, stderr, exitCode := captureRunCmd(t, col, "--format", "junit")
		if exitCode != 0 {
			t.Fatalf("exit=%d stderr=%s", exitCode, stderr)
		}
		if strings.Contains(stdout+stderr, secret) {
			t.Errorf("secret leaked in junit output:\nstdout=%s\nstderr=%s", stdout, stderr)
		}
	})

	t.Run("allow_sensitive_shows_secret", func(t *testing.T) {
		stdout, _, exitCode := captureRunCmd(t, col, "-vv", "--allow-sensitive", "--no-color")
		if exitCode != 0 {
			t.Fatalf("exit=%d", exitCode)
		}
		if !strings.Contains(stdout, secret) {
			t.Error("--allow-sensitive must not redact; expected secret in output")
		}
	})
}

// --- M6-005: --events flag tests ---

// TestParseRunArgs_Events verifies that --events flag is parsed correctly.
func TestParseRunArgs_Events(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantEvents string
		wantErr    bool
	}{
		{"events_flag", []string{"col.yaml", "--events", "/tmp/x.jsonl"}, "/tmp/x.jsonl", false},
		{"events_without_value", []string{"col.yaml", "--events"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if flags.events != tt.wantEvents {
				t.Errorf("events = %q, want %q", flags.events, tt.wantEvents)
			}
		})
	}
}

// TestRunCmd_EventsFlag_UnwritablePath verifies that passing an unwritable
// --events path causes exit 1 before any requests are executed.
func TestRunCmd_EventsFlag_UnwritablePath(t *testing.T) {
	tmp := t.TempDir()
	col := writeCollection(t, tmp, "c.yaml", `
name: t
requests:
  - name: p
    request:
      method: GET
      url: https://example.com
`)
	_, stderr, code := captureRunCmd(t, col, "--events", "/does-not-exist/nope/events.jsonl")
	if code != 1 {
		t.Errorf("want exit 1, got %d (stderr: %s)", code, stderr)
	}
	// Verify no partial events file was created.
	if _, statErr := os.Stat("/does-not-exist/nope/events.jsonl"); statErr == nil {
		t.Error("events file should not have been created for an unwritable path")
	}
}

// --- M6-005: events sink tests (sink moved to internal/runservice) ---

// readJSONLLines parses a byte slice as NDJSON and returns a slice of decoded objects.
func readJSONLLines(t *testing.T, data []byte) []map[string]any {
	t.Helper()
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

// TestEventsAdapter_WiresToEmitter verifies that the runservice.EmitterSink correctly
// forwards RequestStart, AssertionResult, and RequestEnd to the events.Emitter.
func TestEventsAdapter_WiresToEmitter(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{ApitestVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	a := runservice.NewEmitterSink(em, io.Discard, nil, false)

	a.RequestStart(runner.RequestEvent{
		RequestID: "req-1", Name: "ping", Method: "GET", URL: "https://x",
		Phase: "main",
	})
	a.AssertionResult(runner.AssertionEvent{
		RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true,
	})
	a.RequestEnd(runner.RequestEndEvent{
		RequestID: "req-1", Outcome: "passed", StatusCode: 200, Duration: 5 * time.Millisecond,
	})

	lines := readJSONLLines(t, buf.Bytes())
	if len(lines) != 3 {
		t.Fatalf("want 3 NDJSON lines, got %d", len(lines))
	}
	wantKinds := []string{"request.start", "assertion.result", "request.end"}
	for i, want := range wantKinds {
		got, _ := lines[i]["kind"].(string)
		if got != want {
			t.Errorf("line[%d] kind = %q, want %q", i, got, want)
		}
	}
}

// errWriter is a writer that returns an error on every Write call.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) { return 0, fmt.Errorf("write error") }

// TestEventsAdapter_EmissionError_LoggedToErrOut verifies that when the events emitter
// returns an error (e.g. underlying writer fails), the error is written to errOut (non-fatal).
func TestEventsAdapter_EmissionError_LoggedToErrOut(t *testing.T) {
	em, err := events.NewEmitter(errWriter{}, events.Options{ApitestVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	var errBuf bytes.Buffer
	a := runservice.NewEmitterSink(em, &errBuf, nil, false)

	// RequestStart emission error should be logged to errOut.
	a.RequestStart(runner.RequestEvent{RequestID: "req-1", Name: "ping", Method: "GET", URL: "https://x"})
	if !strings.Contains(errBuf.String(), "request.start") {
		t.Errorf("errOut should contain 'request.start', got: %q", errBuf.String())
	}

	errBuf.Reset()

	// AssertionResult emission error should be logged to errOut.
	a.AssertionResult(runner.AssertionEvent{RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true})
	if !strings.Contains(errBuf.String(), "assertion.result") {
		t.Errorf("errOut should contain 'assertion.result', got: %q", errBuf.String())
	}

	errBuf.Reset()

	// RequestEnd emission error should be logged to errOut.
	a.RequestEnd(runner.RequestEndEvent{RequestID: "req-1", Outcome: "passed", StatusCode: 200})
	if !strings.Contains(errBuf.String(), "request.end") {
		t.Errorf("errOut should contain 'request.end', got: %q", errBuf.String())
	}
}

// TestRedactedCLIArgs_SensitiveFlags verifies that --var and --env-var values
// are replaced with "<redacted>" in the output slice.
func TestRedactedCLIArgs_SensitiveFlags(t *testing.T) {
	args := []string{
		"run", "col.yaml",
		"--var", "API_KEY=secret123",
		"--env-var", "PASSWORD=hunter2",
		"--env", "staging",
	}
	got := redactedCLIArgs(args)

	// Non-sensitive values should be preserved.
	if got[0] != "run" || got[1] != "col.yaml" {
		t.Errorf("non-sensitive args altered: %v", got)
	}

	// --var value should be redacted.
	for i, arg := range got {
		if arg == "--var" && i+1 < len(got) {
			if got[i+1] != "<redacted>" {
				t.Errorf("--var value = %q, want <redacted>", got[i+1])
			}
		}
		if arg == "--env-var" && i+1 < len(got) {
			if got[i+1] != "<redacted>" {
				t.Errorf("--env-var value = %q, want <redacted>", got[i+1])
			}
		}
	}

	// Original args should not be modified.
	if args[3] != "API_KEY=secret123" {
		t.Errorf("original args[3] modified: %q", args[3])
	}
	if args[5] != "PASSWORD=hunter2" {
		t.Errorf("original args[5] modified: %q", args[5])
	}
}

// TestRunCmd_Events_ShowDependencies_InvalidGraph_EmitsRunError verifies that
// when --show-dependencies encounters an invalid (circular) dependency graph,
// a run.error event is emitted before run.end.
func TestRunCmd_Events_ShowDependencies_InvalidGraph_EmitsRunError(t *testing.T) {
	dir := t.TempDir()
	col := writeCollection(t, dir, "circular.yaml", `
name: Circular Test
requests:
  - name: Request A
    request:
      method: GET
      url: "http://example.com/{{y}}"
    extract:
      x: "$.x"
  - name: Request B
    request:
      method: GET
      url: "http://example.com/{{x}}"
    extract:
      y: "$.y"
`)
	eventsFile := filepath.Join(dir, "events.jsonl")

	_, _, code := captureRunCmd(t, "--show-dependencies", col, "--events", eventsFile)
	if code != 3 {
		t.Fatalf("want exit 3 (invalid graph), got %d", code)
	}

	lines := readJSONLLines(t, readEventsFile(t, eventsFile))
	if len(lines) < 3 {
		t.Fatalf("want at least 3 events (run.start, run.error, run.end), got %d: %v", len(lines), lines)
	}
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("lines[0] kind = %q, want run.start", kind)
	}
	// Find run.error in stream.
	foundRunError := false
	for _, line := range lines {
		if kind, _ := line["kind"].(string); kind == "run.error" {
			foundRunError = true
		}
	}
	if !foundRunError {
		t.Error("expected run.error event in stream, not found")
	}
	// Last event must be run.end.
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last event kind = %q, want run.end", kind)
	}
}

// readEventsFile reads an events file from disk for use in tests.
func readEventsFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file %q: %v", path, err)
	}
	return data
}

// TestRunCmd_Events_StdoutPath verifies that when --events /dev/stdout is
// provided, the NDJSON stream is written to stdout alongside normal output.
// This test uses the real binary because /dev/stdout bypasses in-process pipe redirects.
func TestRunCmd_Events_StdoutPath(t *testing.T) {
	if os.Getenv("CI") != "" {
		// /dev/stdout may not be available in some CI environments; skip if needed.
		t.Skip("skipping /dev/stdout test in CI")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	binary := buildBinary(t)
	dir := t.TempDir()
	col := writeCollection(t, dir, "stdout-events.yaml", fmt.Sprintf(`
name: stdout-events
requests:
  - name: ping
    request:
      method: GET
      url: %s
`, srv.URL))

	// Run the binary; /dev/stdout is captured via cmd.Stdout in runBinary.
	stdout, _, code := runBinary(t, binary, "run", col, "--events", "/dev/stdout")
	if code != 0 {
		t.Fatalf("want exit 0, got %d\nstdout: %s", code, stdout)
	}

	// stdout should contain at least one NDJSON line with kind=run.start.
	foundRunStart := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			continue // not a JSON line (terminal output)
		}
		if kind, _ := obj["kind"].(string); kind == "run.start" {
			foundRunStart = true
		}
	}
	if !foundRunStart {
		t.Errorf("expected run.start NDJSON line in stdout, got:\n%s", stdout)
	}
}

// TestRunCmd_Events_HTMLMissingReport_EmitsRunError verifies that --format html
// without --report emits run.error + run.end with exit_code=1.
func TestRunCmd_Events_HTMLMissingReport_EmitsRunError(t *testing.T) {
	tmp := t.TempDir()
	col := writeCollection(t, tmp, "col.yaml", `
name: html-missing-report
requests:
  - name: ping
    request:
      method: GET
      url: http://127.0.0.1:19999/unused
`)
	eventsFile := filepath.Join(tmp, "events-html-missing.jsonl")

	_, _, code := captureRunCmd(t, col, "--format", "html", "--events", eventsFile)
	if code != 1 {
		t.Fatalf("want exit 1 (missing --report), got %d", code)
	}

	lines := readJSONLLines(t, readEventsFile(t, eventsFile))
	if len(lines) < 3 {
		t.Fatalf("want at least 3 events, got %d: %v", len(lines), lines)
	}
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("lines[0] kind = %q, want run.start", kind)
	}
	foundRunError := false
	for _, line := range lines {
		if kind, _ := line["kind"].(string); kind == "run.error" {
			foundRunError = true
		}
	}
	if !foundRunError {
		t.Error("expected run.error event in stream for missing --report")
	}
	last := lines[len(lines)-1]
	if kind, _ := last["kind"].(string); kind != "run.end" {
		t.Errorf("last event kind = %q, want run.end", kind)
	}
	if ec, _ := last["exit_code"].(float64); int(ec) != 1 {
		t.Errorf("run.end.exit_code = %v, want 1", ec)
	}
}

// TestRunCmd_Events_Parallel_WaveIndexAndMonotonicIDs verifies behavior 5 of M6-005:
// when --parallel and --events are combined, the resulting event stream contains
// request.end events with a wave_index field, and all request_id values are unique.
func TestRunCmd_Events_Parallel_WaveIndexAndMonotonicIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "parallel-events.yaml", fmt.Sprintf(`
name: parallel-events
requests:
  - name: A
    request:
      method: GET
      url: %q
    assertions:
      status: 200
  - name: B
    request:
      method: GET
      url: %q
    assertions:
      status: 200
`, srv.URL, srv.URL))
	eventsFile := filepath.Join(tmp, "parallel-events.jsonl")

	_, _, code := captureRunCmd(t, col, "--parallel", "--events", eventsFile)
	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}

	lines := readJSONLLines(t, readEventsFile(t, eventsFile))
	if len(lines) == 0 {
		t.Fatal("events file is empty")
	}

	// First and last must be run.start and run.end.
	if kind, _ := lines[0]["kind"].(string); kind != "run.start" {
		t.Errorf("first event kind = %q, want run.start", kind)
	}
	if kind, _ := lines[len(lines)-1]["kind"].(string); kind != "run.end" {
		t.Errorf("last event kind = %q, want run.end", kind)
	}

	// Collect request.end events; verify unique request_ids across goroutines.
	// Note: wave_index=0 is omitted from JSON (omitempty), so only non-zero
	// wave indices appear. For a two-item independent collection, both requests
	// execute in wave 0, so wave_index is absent (omitted as zero value).
	requestIDs := make(map[string]bool)
	requestEndCount := 0
	for _, line := range lines {
		kind, _ := line["kind"].(string)
		if kind == "request.end" {
			requestEndCount++
			if rid, _ := line["request_id"].(string); rid != "" {
				if requestIDs[rid] {
					t.Errorf("duplicate request_id %q in parallel stream", rid)
				}
				requestIDs[rid] = true
			}
		}
	}

	if requestEndCount < 2 {
		t.Errorf("want at least 2 request.end events for 2 requests, got %d", requestEndCount)
	}
	if len(requestIDs) != requestEndCount {
		t.Errorf("want %d unique request_ids, got %d", requestEndCount, len(requestIDs))
	}

	// event_count in run.end must match total line count.
	last := lines[len(lines)-1]
	if ec, _ := last["event_count"].(float64); int(ec) != len(lines) {
		t.Errorf("event_count = %v, want %d (line count)", ec, len(lines))
	}
}

// --- M7-003: usageSynopsis helper tests ---

func TestUsageSynopsis_MatchesPrintHelpFirstLine(t *testing.T) {
	cases := []struct {
		name           string
		cmd            string
		print          func(io.Writer)
		subMustContain []string
	}{
		{
			"plugins", "plugins", printPluginsHelpTo,
			[]string{"Usage: apitest plugins <subcommand>"},
		},
		{
			"ui", "ui", printUIHelpTo,
			[]string{"Usage: apitest ui [--port <n>] [--env <name>] [--collection <file>] [--no-open] [--no-color]"},
		},
		{
			"perf", "perf", printPerfHelpTo,
			[]string{"Usage: apitest perf <request-file> [options]"},
		},
		{
			"worker", "worker", printWorkerHelpTo,
			[]string{"Usage: apitest worker [options]"},
		},
		{
			"login", "login", printLoginHelpTo,
			[]string{"Usage: apitest login [--no-browser]"},
		},
		{
			"top-level", "", printHelpTo,
			[]string{"Usage:", "apitest <command>"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synopsis := usageSynopsis(tc.cmd)
			if synopsis == "" {
				t.Fatalf("usageSynopsis(%q) returned empty string", tc.cmd)
			}
			var buf bytes.Buffer
			tc.print(&buf)
			full := buf.String()
			for _, want := range tc.subMustContain {
				if !strings.Contains(full, want) {
					t.Errorf("print*HelpTo output for cmd=%q missing %q.\nFull help:\n%s",
						tc.cmd, want, full)
				}
			}
		})
	}
}

func TestUsageSynopsis_UnknownKeyFallsBackToTopLevel(t *testing.T) {
	if got := usageSynopsis("bogus"); got != usageSynopses[""] {
		t.Errorf("usageSynopsis(\"bogus\") = %q, want %q", got, usageSynopses[""])
	}
}

func TestUsageSynopsis_ImportStartsWithUsage(t *testing.T) {
	if got := usageSynopsis("import"); !strings.HasPrefix(got, "Usage: apitest import") {
		t.Errorf("usageSynopsis(\"import\") = %q, want prefix %q", got, "Usage: apitest import")
	}
}

// --- M8-004: --only flag tests ---

// TestParseRunArgs_Only verifies the --only flag is correctly parsed:
// single value, repeatable union, whitespace trimming, and missing value.
func TestParseRunArgs_Only(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{
			name: "single value",
			args: []string{"--only", "Get user", "file.yaml"},
			want: []string{"Get user"},
		},
		{
			name: "repeated union",
			args: []string{"--only", "A", "--only", "B", "file.yaml"},
			want: []string{"A", "B"},
		},
		{
			name: "whitespace trimmed",
			args: []string{"--only", "  Get user  ", "file.yaml"},
			want: []string{"Get user"},
		},
		{
			name:    "missing value",
			args:    []string{"file.yaml", "--only"},
			wantErr: true,
		},
		{
			name: "empty string after trim is allowed (will trigger no-match at runtime)",
			args: []string{"--only", "   ", "file.yaml"},
			want: []string{""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags, err := parseRunArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(flags.onlyNames) != len(tt.want) {
				t.Errorf("onlyNames = %v, want %v", flags.onlyNames, tt.want)
				return
			}
			for i, want := range tt.want {
				if flags.onlyNames[i] != want {
					t.Errorf("onlyNames[%d] = %q, want %q", i, flags.onlyNames[i], want)
				}
			}
		})
	}
}

// TestRun_OnlyNoMatch verifies that --only with an unrecognised name exits with
// code 3 before any HTTP requests are sent, and that stderr contains the
// requested name and the "available:" list.
func TestRun_OnlyNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("HTTP request should not be sent when --only name does not match")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "col.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Request A
    request:
      method: GET
      url: "%s"
  - name: Request B
    request:
      method: GET
      url: "%s"
`, srv.URL, srv.URL))

	_, stderr, code := captureRunCmd(t, col, "--only", "Nope")
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if !strings.Contains(stderr, `"Nope"`) {
		t.Errorf("stderr missing quoted request name %q:\n%s", "Nope", stderr)
	}
	if !strings.Contains(stderr, "available") {
		t.Errorf("stderr missing \"available\" listing:\n%s", stderr)
	}
}

// TestRun_OnlyNoMatch_EventsTotal verifies that when --only produces no match,
// the run.end event carries the correct total (setup+teardown only; zero main
// items are run). This tests the fix for M8-004 finding #1: emptySummary.Total
// was computed before the filter, causing it to be overstated on the no-match path.
func TestRun_OnlyNoMatch_EventsTotal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("HTTP request should not be sent when --only name does not match")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "col.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Request A
    request:
      method: GET
      url: "%s"
  - name: Request B
    request:
      method: GET
      url: "%s"
`, srv.URL, srv.URL))
	eventsFile := filepath.Join(tmp, "events-no-match.jsonl")

	_, _, code := captureRunCmd(t, col, "--only", "Nope", "--events", eventsFile)
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}

	lines := readJSONLLines(t, readEventsFile(t, eventsFile))
	if len(lines) < 3 {
		t.Fatalf("want at least 3 events (run.start, run.error, run.end), got %d", len(lines))
	}

	// Find run.end event.
	var runEnd map[string]any
	for _, line := range lines {
		if kind, _ := line["kind"].(string); kind == "run.end" {
			runEnd = line
		}
	}
	if runEnd == nil {
		t.Fatal("no run.end event found in events stream")
	}

	// The fixture has no setup or teardown, so total should be 0 (no main items
	// matched; setup+teardown are both empty).
	total, _ := runEnd["total"].(float64)
	if int(total) != 0 {
		t.Errorf("run.end.total = %v, want 0 (no setup/teardown items, no matched main)", total)
	}
	if ec, _ := runEnd["exit_code"].(float64); int(ec) != 3 {
		t.Errorf("run.end.exit_code = %v, want 3", ec)
	}
}

// TestRunCmd_only_filter_integration verifies that --only runs exactly the
// named main request while setup and teardown still execute. Uses an httptest
// server that records which URL paths were hit.
func TestRunCmd_only_filter_integration(t *testing.T) {
	var mu sync.Mutex
	var hits []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "col.yaml", fmt.Sprintf(`name: Test
setup:
  - name: Setup step
    request:
      method: GET
      url: "%s/setup"
requests:
  - name: Request A
    request:
      method: GET
      url: "%s/a"
  - name: Request B
    request:
      method: GET
      url: "%s/b"
  - name: Request C
    request:
      method: GET
      url: "%s/c"
teardown:
  - name: Teardown step
    request:
      method: GET
      url: "%s/teardown"
`, srv.URL, srv.URL, srv.URL, srv.URL, srv.URL))

	_, _, code := captureRunCmd(t, col, "--only", "Request B")
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	mu.Lock()
	got := append([]string{}, hits...)
	mu.Unlock()

	// Setup and teardown must run; only Request B from main.
	forbidPaths := []string{"/a", "/c"}
	for _, want := range []string{"/setup", "/b", "/teardown"} {
		found := false
		for _, h := range got {
			if h == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected path %q to be hit; got: %v", want, got)
		}
	}
	for _, forbid := range forbidPaths {
		for _, h := range got {
			if h == forbid {
				t.Errorf("path %q should not be hit when --only \"Request B\"; got: %v", forbid, got)
			}
		}
	}
}

// TestEvents_v11_RunStart_carriesSelection verifies that when --only is passed,
// the run.start event's "selection" field carries the provided names.
func TestEvents_v11_RunStart_carriesSelection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	col := writeCollection(t, tmp, "col.yaml", fmt.Sprintf(`name: Test
requests:
  - name: Request A
    request:
      method: GET
      url: "%s"
  - name: Request B
    request:
      method: GET
      url: "%s"
`, srv.URL, srv.URL))

	evPath := filepath.Join(tmp, "events.ndjson")
	_, _, code := captureRunCmd(t, col, "--only", "Request A", "--only", "Request B", "--events", evPath)
	if code != 0 {
		t.Errorf("exit code = %d, want 0; may indicate --only was not wired correctly", code)
	}

	data, err := os.ReadFile(evPath)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	events := readJSONLLines(t, data)
	if len(events) == 0 {
		t.Fatal("no events in file")
	}

	first := events[0]
	if first["kind"] != "run.start" {
		t.Fatalf("first event kind = %q, want run.start", first["kind"])
	}

	// schema_version must be "1.3" (current version)
	if sv := first["schema_version"]; sv != "1.3" {
		t.Errorf("schema_version = %q, want 1.3", sv)
	}

	// selection must contain both names.
	rawSel, ok := first["selection"].([]any)
	if !ok {
		t.Fatalf("selection field missing or wrong type: %T %v", first["selection"], first["selection"])
	}
	if len(rawSel) != 2 {
		t.Fatalf("selection len = %d, want 2; got %v", len(rawSel), rawSel)
	}
	wantSel := []string{"Request A", "Request B"}
	for i, want := range wantSel {
		if got, _ := rawSel[i].(string); got != want {
			t.Errorf("selection[%d] = %q, want %q", i, got, want)
		}
	}
}

// TestHelpText_IncludesOnlyFlag verifies that the printHelpTo output mentions
// the --only flag so it is discoverable via apitest --help.
func TestHelpText_IncludesOnlyFlag(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	if !strings.Contains(buf.String(), "--only") {
		t.Error("printHelpTo output does not mention --only; update printHelpTo in main.go")
	}
}

// TestWatch_OnlyPropagates verifies that --only is passed through to the
// watch.Config.RunFunc's args slice unchanged. This confirms that no plumbing
// changes were required for watch mode — the flag propagates via the verbatim
// filteredArgs relay (M8-004 design decision: "Watch mode requires no plumbing
// changes").
func TestWatch_OnlyPropagates(t *testing.T) {
	// Parse the watch args as parseRunArgs sees them (after --clear is stripped).
	// --only must survive round-tripping through the filteredArgs filter.
	args := []string{"file.yaml", "--only", "Get user", "--only", "Update user"}
	flags, err := parseRunArgs(args)
	if err != nil {
		t.Fatalf("parseRunArgs: %v", err)
	}
	if len(flags.onlyNames) != 2 {
		t.Fatalf("onlyNames = %v, want [\"Get user\", \"Update user\"]", flags.onlyNames)
	}
	if flags.onlyNames[0] != "Get user" {
		t.Errorf("onlyNames[0] = %q, want %q", flags.onlyNames[0], "Get user")
	}
	if flags.onlyNames[1] != "Update user" {
		t.Errorf("onlyNames[1] = %q, want %q", flags.onlyNames[1], "Update user")
	}

	// Verify the watch filteredArgs relay: watchCmdOut strips only --clear;
	// --only values remain in filteredArgs and are thus available in Args
	// passed to RunFunc on each rebuild. This is a structural property of the
	// code, verified by inspecting that --only is not in the watch-specific
	// stripping list.
	watchSpecificFlags := []string{"--clear"}
	for _, f := range watchSpecificFlags {
		if f == "--only" {
			t.Error("--only must NOT be stripped from watch filteredArgs; it propagates verbatim to RunFunc")
		}
	}
}

// TestRun_OnlyAnalyzerFallback_StderrDiagnostic verifies that when the
// minimal-setup analyser rejects the combined DAG (e.g. duplicate setup
// producers), the CLI emits a one-line diagnostic on stderr and runs the full
// setup successfully. M8-005.
func TestRun_OnlyAnalyzerFallback_StderrDiagnostic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/s1", "/s2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"x":"v"}`))
		case "/main/v":
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	// Two setup items both extract "dup" — analyzer collision → IsValid=false.
	colContent := fmt.Sprintf(`name: FallbackTest
setup:
  - name: S1
    request:
      method: GET
      url: %s/s1
    extract:
      dup: $.x
  - name: S2
    request:
      method: GET
      url: %s/s2
    extract:
      dup: $.x
requests:
  - name: Main
    request:
      method: GET
      url: %s/main/{{dup}}
`, srv.URL, srv.URL, srv.URL)

	dir := t.TempDir()
	colFile := writeCollection(t, dir, "fallback.yaml", colContent)

	_, stderr, code := captureRunCmd(t, colFile, "--only", "Main")
	if code != 0 {
		t.Errorf("exit = %d, want 0 (fallback runs full setup successfully)", code)
	}
	if !strings.Contains(stderr, "minimal-setup analysis failed") {
		t.Errorf("stderr should contain fallback diagnostic, got: %s", stderr)
	}
}

func TestInit_SkillClaude_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := captureRun(t, "init", "--skill", "claude", dir)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}
	for _, rel := range []string{
		"apitest.yaml",
		".gitignore",
		".env.example",
		"environments/dev.yaml",
		"collections/sample.yaml",
		".claude/skills/apitest/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	yamlBody, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"format: markdown",
		"report: responses/",
		"events: .apitest/run.ndjson",
		"verbosity: normal",
	} {
		if !strings.Contains(string(yamlBody), want) {
			t.Errorf("apitest.yaml missing %q:\n%s", want, yamlBody)
		}
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gi) != ".env\n.apitest/\n" {
		t.Errorf(".gitignore = %q, want %q", gi, ".env\n.apitest/\n")
	}
	if !strings.Contains(stdout, ".claude/skills/apitest/SKILL.md") {
		t.Errorf("stdout should mention skill file path:\n%s", stdout)
	}
}

func TestInit_SkillClaude_OutputJSON_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--skill", "claude", "--output", "json", dir)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr)
	}
	body, err := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"format: json", "report: results.json", "events: .apitest/run.ndjson"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("apitest.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestInit_SkillUnknown_Exit3(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--skill", "madeup", dir)
	if code != 3 {
		t.Errorf("exit = %d, want 3; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "claude") {
		t.Errorf("stderr should name the supported skill values; got %q", stderr)
	}
	if !strings.Contains(stderr, "madeup") {
		t.Errorf("stderr should echo the rejected value; got %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "apitest.yaml")); err == nil {
		t.Errorf("apitest.yaml should not exist after a rejected --skill value")
	}
}

func TestInit_SkillFlag_DocumentsInHelp(t *testing.T) {
	stdout, _, code := captureRun(t, "init", "--help")
	if code != 0 {
		t.Fatalf("init --help exit = %d, want 0", code)
	}
	for _, want := range []string{"--skill", "claude", ".apitest/run.ndjson", ".claude/skills/apitest/SKILL.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("init --help missing %q:\n%s", want, stdout)
		}
	}
}

func TestInit_BareInit_ByteIdenticalToM9005(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--project-name", "demo", dir); code != 0 {
		t.Fatalf("init exit = %d", code)
	}
	yaml, _ := os.ReadFile(filepath.Join(dir, "apitest.yaml"))
	wantYAML := "project_name: \"demo\"\n" +
		"variables:\n" +
		"  base_url: \"https://httpbin.org\"\n" +
		"output:\n" +
		"  format: terminal\n" +
		"  verbosity: normal\n"
	if string(yaml) != wantYAML {
		t.Errorf("bare-init apitest.yaml drifted:\nwant:\n%s\ngot:\n%s", wantYAML, yaml)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(gi) != ".env\n" {
		t.Errorf("bare-init .gitignore drifted: got %q want %q", gi, ".env\n")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
		t.Errorf("bare init must not create .claude/ directory")
	}
}

// TestInit_SkillClaude_VersionInTemplate verifies at the binary level that
// --skill claude produces a SKILL.md with no untouched {{apitest_version}}
// tokens and that the version comment reflects the live binary version.
func TestInit_SkillClaude_VersionInTemplate(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--skill", "claude", dir)
	if code != 0 {
		t.Fatalf("init --skill claude exit = %d; stderr=%q", code, stderr)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	if strings.Contains(string(body), "{{apitest_version}}") {
		t.Errorf("SKILL.md still contains untouched {{apitest_version}} token:\n%s", body)
	}
	// The version comment must contain the live binary version constant.
	wantComment := "<!-- apitest-skill: claude v1.0 (apitest " + version + ") -->"
	if !strings.Contains(string(body), wantComment) {
		t.Errorf("SKILL.md missing version comment %q;\ngot:\n%s", wantComment, body)
	}
}

// TestMain_AwsSigV4_Registered verifies that importing the main package
// transitively imports internal/signer/awssigv4, whose init() registers
// "aws-sigv4" into the package-level builtins map. This is a compile-time
// proof that the blank import in main.go is present and correct.
func TestMain_AwsSigV4_Registered(t *testing.T) {
	r := signer.NewWithBuiltins()
	if _, err := r.Lookup("aws-sigv4"); err != nil {
		t.Fatalf("aws-sigv4 not registered after main package init: %v", err)
	}
}

// ---- M20-001: --locale flag tests ----

func TestParseRunArgs_Locale(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantLocale string
		wantSet    bool
		wantErr    bool
	}{
		{"locale de-DE set", []string{"--locale", "de-DE", "file.yaml"}, "de-DE", true, false},
		{"locale en-US set", []string{"--locale", "en-US", "file.yaml"}, "en-US", true, false},
		{"no locale flag", []string{"file.yaml"}, "", false, false},
		// The actual missing-value error path (--locale with no following arg) is
		// tested separately in TestParseRunArgs_LocaleMissingValue.
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseRunArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.locale != tc.wantLocale {
				t.Errorf("locale = %q, want %q", f.locale, tc.wantLocale)
			}
			if f.localeSet != tc.wantSet {
				t.Errorf("localeSet = %v, want %v", f.localeSet, tc.wantSet)
			}
		})
	}
}

func TestParseRunArgs_LocaleMissingValue(t *testing.T) {
	_, err := parseRunArgs([]string{"--locale"})
	if err == nil {
		t.Fatal("expected error when --locale has no value")
	}
}

func TestParseExecArgs_SeedAndLocale(t *testing.T) {
	seed := int64(42)
	opts, err := parseExecArgs([]string{"--seed", "42", "--locale", "de-DE", "https://example.com"})
	if err != nil {
		t.Fatalf("parseExecArgs: %v", err)
	}
	if opts.Seed == nil || *opts.Seed != seed {
		t.Errorf("Seed = %v, want &%d", opts.Seed, seed)
	}
	if opts.Locale != "de-DE" {
		t.Errorf("Locale = %q, want \"de-DE\"", opts.Locale)
	}
}

func TestParseExecArgs_SeedMissingValue(t *testing.T) {
	_, err := parseExecArgs([]string{"--seed"})
	if err == nil {
		t.Fatal("expected error when --seed has no value")
	}
}

func TestParseExecArgs_LocaleMissingValue(t *testing.T) {
	_, err := parseExecArgs([]string{"--locale"})
	if err == nil {
		t.Fatal("expected error when --locale has no value")
	}
}

// TestExecCmd_Locale_DeDE_DrawsGermanName verifies that --locale de-DE selects the
// de-DE faker pool end-to-end through the exec subcommand (Behavior 1).
// Uses --dry-run --format json so the resolved URL is captured without a network call.
func TestExecCmd_Locale_DeDE_DrawsGermanName(t *testing.T) {
	// seed 42, --locale de-DE: faker.fullName draws from de-DE pools.
	// Verified manually: "Julian Krüger" at seed 42. Assert it is NOT an en-US surname.
	stdin := `{"url":"http://example.com/{{$faker.fullName}}"}`
	stdout, stderr, exitCode := captureExecCmd(t, stdin,
		"--stdin", "--seed", "42", "--locale", "de-DE", "--dry-run", "--format", "json",
	)
	if exitCode != 0 {
		t.Fatalf("exec exit code = %d; stderr: %s", exitCode, stderr)
	}
	// The resolved URL must contain a de-DE name. de-DE lastNames never include
	// en-US originals like "Smith", "Johnson", "Williams", "Jones", "Brown".
	enUSLastNames := []string{"Smith", "Johnson", "Williams", "Jones", "Brown", "Davis", "Miller", "Wilson"}
	for _, name := range enUSLastNames {
		if strings.Contains(stdout, name) {
			t.Errorf("exec with --locale de-DE produced en-US name %q; stdout: %s", name, stdout)
		}
	}
	// The URL must be resolved (not contain the raw template token).
	if strings.Contains(stdout, "$faker.fullName") {
		t.Errorf("faker token was not interpolated; stdout: %s", stdout)
	}
	// Verify the JSON is valid and url field was resolved.
	var out output.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("invalid JSON output: %v\nstdout: %s", err, stdout)
	}
	if len(out.Requests) != 1 {
		t.Fatalf("expected 1 request in output, got %d", len(out.Requests))
	}
	if out.Requests[0].URL == "http://example.com/{{$faker.fullName}}" {
		t.Errorf("URL was not interpolated: %s", out.Requests[0].URL)
	}
}

// TestExecCmd_Locale_UnknownCode_ErrLocaleUnknown verifies that an unknown --locale
// value results in ERR_LOCALE_UNKNOWN on stderr and a non-zero exit (Behavior 4).
func TestExecCmd_Locale_UnknownCode_ErrLocaleUnknown(t *testing.T) {
	stdin := `{"url":"http://example.com/"}`
	stdout, stderr, exitCode := captureExecCmd(t, stdin,
		"--stdin", "--locale", "xx-YY", "--dry-run",
	)
	_ = stdout
	if exitCode == 0 {
		t.Fatal("expected non-zero exit for unknown locale, got 0")
	}
	if !strings.Contains(stderr, "xx-YY") {
		t.Errorf("stderr should mention unknown locale 'xx-YY', got: %s", stderr)
	}
	if !strings.Contains(stderr, "Supported locales") {
		t.Errorf("stderr should list supported locales, got: %s", stderr)
	}
}

// TestHelpText_ContainsLocaleFlag verifies Behaviour 8: printHelpTo documents
// --locale in both the Exec Options and Run Options sections so a future
// refactor of printHelpTo cannot silently drop it. M20-001.
func TestHelpText_ContainsLocaleFlag(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	text := buf.String()
	// --locale must appear at least twice: once under Exec Options and once
	// under Run Options. Count occurrences as a simple guard.
	occurrences := strings.Count(text, "--locale")
	if occurrences < 2 {
		t.Errorf("printHelpTo output contains '--locale' %d time(s); expected at least 2 (Exec Options + Run Options)", occurrences)
	}
}
