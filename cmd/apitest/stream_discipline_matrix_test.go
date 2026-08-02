package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ansiRE matches any CSI-style ANSI escape sequence (ESC [ ...).
var ansiRE = regexp.MustCompile(`\x1b\[`)

// knownProgressStrings must NEVER appear on stdout for any subcommand.
// These are the strings M7-002/003 relocated from stdout to stderr —
// regressions here indicate a new call site that forgot the stderr convention.
var knownProgressStrings = []string{
	"Running...",
	"Load test:",
	"Claimed shard",
	"Resolved ", // "Resolved N secrets from shared template ..."
}

// TestStreamDisciplineMatrix is the M7-004 regression gate. It iterates
// (subcommand × format) cells, spawns the real apitest binary with both
// stdout and stderr wired to pipes, and asserts the three core M7 invariants:
//
//  1. Stdout contains ONLY the declared format's payload.
//  2. Stderr is free of ANSI escape sequences when piped.
//  3. No known-progress string leaks onto stdout.
//
// TTY-wired cells are owned by TestStderrColorFlag (M7-001) so we do not
// duplicate them here; /dev/tty usage would add platform coupling without
// additional regression signal.
//
// Deviation from plan: fixture YAML files use the nested `request:` structure
// (not flat method/url at item level), and parse-error exit code is 3 (not 5)
// since the parser returns CategoryParse errors with exit code 3 in main.go.
func TestStreamDisciplineMatrix(t *testing.T) {
	binary := buildBinary(t)

	// One shared httptest server answers every fixture URL.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Materialize fixtures (their BASE_URL placeholder is resolved here so the
	// tests stay hermetic — no DNS, no external URLs).
	fixtures := materializeStreamDisciplineFixtures(t, srv.URL)

	type stdoutCheck func(t *testing.T, stdout string)

	// htmlReportPath is used by run_html_happy_with_report to thread the
	// generated file path between the args slice and the post-run assertion.
	htmlReportPath := filepath.Join(t.TempDir(), "report.html")

	cases := []struct {
		name                   string
		args                   []string    // passed to ./apitest
		env                    []string    // extra env (on top of os.Environ() minus NO_COLOR)
		wantExitCode           int         // expected exit code; -1 = skip exit-code check (any code accepted)
		stdoutIsEmpty          bool        // when true, stdout must be exactly zero bytes
		checkStdout            stdoutCheck // optional: stronger stdout assertion
		checkHTMLFile          string      // if non-empty, read this file and assert HTML content markers
		extraStderrMustContain []string    // optional: stderr progress-text assertions
	}{
		// --- run × 5 formats × happy path ---
		{
			name:         "run_json_happy",
			args:         []string{"run", fixtures.happy, "--format", "json"},
			wantExitCode: 0,
			checkStdout:  assertValidJSONRunOutput,
		},
		{
			name:         "run_tap_happy",
			args:         []string{"run", fixtures.happy, "--format", "tap"},
			wantExitCode: 0,
			checkStdout:  assertTAPHeader,
		},
		{
			name:         "run_junit_happy",
			args:         []string{"run", fixtures.happy, "--format", "junit"},
			wantExitCode: 0,
			checkStdout:  assertValidJUnitXML,
		},
		{
			name: "run_html_happy_with_report",
			args: []string{
				"run", fixtures.happy, "--format", "html",
				"--report", htmlReportPath,
			},
			wantExitCode:  0,
			stdoutIsEmpty: true,           // html writes to disk; stdout is empty
			checkHTMLFile: htmlReportPath, // verify the generated HTML file
		},
		{
			name:         "run_terminal_happy",
			args:         []string{"run", fixtures.happy},
			wantExitCode: 0,
			checkStdout:  assertTerminalMarkers,
		},

		// --- run × assertion-failure path (exit 1 on all formats) ---
		{
			name:         "run_json_assertion_failure",
			args:         []string{"run", fixtures.assertionFailure, "--format", "json"},
			wantExitCode: 1,
			checkStdout:  assertValidJSONRunOutput, // structure must still parse
		},
		{
			name:         "run_tap_assertion_failure",
			args:         []string{"run", fixtures.assertionFailure, "--format", "tap"},
			wantExitCode: 1,
			checkStdout:  assertTAPHeader,
		},

		// --- run × parse-error path (exit 3, error goes to stderr) ---
		// NOTE: Plan originally specified exit 5 for parse errors, but the
		// actual binary returns exit 3 for CategoryParse errors. Exit 5 is
		// reserved for variable-resolution failures (undefined variable at
		// request time). This fixture uses malformed YAML → exit 3.
		{
			name:          "run_terminal_parse_error",
			args:          []string{"run", fixtures.parseError},
			wantExitCode:  3,
			stdoutIsEmpty: true,
		},

		// --- perf: stdout is ONLY the final "Results:" line ---
		{
			name:                   "perf_progress_on_stderr",
			args:                   []string{"perf", fixtures.perfRequest, "--vus", "1", "--duration", "100ms"},
			wantExitCode:           0,
			checkStdout:            assertPerfResultsLine,
			extraStderrMustContain: []string{"Load test:", "Running..."},
		},

		// --- exec --dry-run: stdout is the "DRY RUN" block; no HTTP traffic ---
		{
			name:         "exec_dry_run",
			args:         []string{"exec", "https://example.com", "--dry-run"},
			wantExitCode: 0,
			checkStdout:  assertExecDryRunStdout,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := os.Environ()
			// Hermetic: strip NO_COLOR so test runs regardless of developer shell.
			env = filterSDMEnv(env, "NO_COLOR")
			env = append(env, tc.env...)

			cmd := exec.Command(binary, tc.args...)
			cmd.Env = env
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			exitCode := 0
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("unexpected error running %s: %v\nstderr=%q", tc.name, err, stderr.String())
			}

			if tc.wantExitCode >= 0 && exitCode != tc.wantExitCode {
				t.Errorf("exit=%d want=%d\nstdout=%q\nstderr=%q",
					exitCode, tc.wantExitCode, stdout.String(), stderr.String())
			}

			// Invariant 1: stdout payload check.
			if tc.stdoutIsEmpty {
				if stdout.Len() != 0 {
					t.Errorf("stdout must be empty; got %q", stdout.String())
				}
			}
			if tc.checkStdout != nil {
				tc.checkStdout(t, stdout.String())
			}
			// HTML file content check (Behavior 4): read the generated file and
			// verify the expected structural markers are present.
			if tc.checkHTMLFile != "" {
				assertHTMLReportContent(t, tc.checkHTMLFile)
			}

			// Invariant 2: no ANSI on piped stderr.
			if ansiRE.MatchString(stderr.String()) {
				t.Errorf("stderr contains ANSI escape sequences (must be stripped when piped): %q",
					stderr.String())
			}

			// Invariant 3: no known-progress leak to stdout.
			for _, leak := range knownProgressStrings {
				if strings.Contains(stdout.String(), leak) {
					t.Errorf("known-progress string %q leaked to stdout: %q", leak, stdout.String())
				}
			}

			// Optional: stronger stderr assertions.
			for _, want := range tc.extraStderrMustContain {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr missing expected %q; stderr=%q", want, stderr.String())
				}
			}
		})
	}
}

// --- stdout shape assertions ---

// assertValidJSONRunOutput verifies that stdout is valid JSON containing the
// required top-level fields for a single-collection run output.
//
// Decision 2 (plan): Behavior 1 in the task YAML lists `status/total/passed`
// but those fields only appear in MultiJSONOutput (multi-collection glob mode).
// For a single-file fixture the actual output.JSONOutput struct exposes
// `name`, `status`, `duration_ms`, and `requests` — these are the fields
// asserted here. The smoke-level `jq -e '.status == "passed"'` check in
// the task observable block exercises the `status` field directly.
func assertValidJSONRunOutput(t *testing.T, stdout string) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Errorf("stdout is not valid JSON: %v\nstdout=%q", err, stdout)
		return
	}
	for _, field := range []string{"name", "status", "duration_ms", "requests"} {
		if _, ok := doc[field]; !ok {
			t.Errorf("json output missing %q field: %q", field, stdout)
		}
	}
}

// assertTAPHeader verifies that stdout begins with the TAP version 13 header.
func assertTAPHeader(t *testing.T, stdout string) {
	t.Helper()
	if !strings.HasPrefix(stdout, "TAP version 13") {
		t.Errorf("stdout does not start with TAP version 13 header: %q", stdout)
	}
}

// assertValidJUnitXML verifies that stdout is well-formed JUnit XML with a
// <testsuites> root element.
func assertValidJUnitXML(t *testing.T, stdout string) {
	t.Helper()
	type testsuites struct {
		XMLName xml.Name `xml:"testsuites"`
	}
	var v testsuites
	if err := xml.Unmarshal([]byte(stdout), &v); err != nil {
		t.Errorf("stdout is not valid JUnit XML: %v\nstdout=%q", err, stdout)
	}
}

// assertTerminalMarkers verifies that terminal-format stdout contains the
// universal collection header and a result indicator glyph.
func assertTerminalMarkers(t *testing.T, stdout string) {
	t.Helper()
	if !strings.Contains(stdout, "Collection: ") {
		t.Errorf("terminal stdout missing 'Collection: ' header: %q", stdout)
	}
	if !strings.ContainsAny(stdout, "✓✗") {
		t.Errorf("terminal stdout missing result indicator (✓ or ✗): %q", stdout)
	}
}

// assertPerfResultsLine verifies that perf stdout contains the results summary.
func assertPerfResultsLine(t *testing.T, stdout string) {
	t.Helper()
	if !strings.Contains(stdout, "Results: requests=") {
		t.Errorf("perf stdout missing 'Results: requests=': %q", stdout)
	}
}

// assertExecDryRunStdout verifies that exec --dry-run stdout contains the
// DRY RUN marker.
func assertExecDryRunStdout(t *testing.T, stdout string) {
	t.Helper()
	if !strings.Contains(stdout, "DRY RUN") {
		t.Errorf("exec --dry-run stdout missing 'DRY RUN': %q", stdout)
	}
}

// assertHTMLReportContent reads the HTML report file at path and verifies
// the structural markers that the html.go template always emits:
// the DOCTYPE prologue, the page title, and the CSS class names for the
// status-bar and summary-card sections (plan Decision 3).
func assertHTMLReportContent(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("cannot read HTML report file %q: %v", path, err)
		return
	}
	content := string(data)
	for _, marker := range []string{
		"<!DOCTYPE html>",
		"apitest report",
		"status-bar",
		"summary-card",
	} {
		if !strings.Contains(content, marker) {
			t.Errorf("HTML report missing marker %q; file=%q", marker, path)
		}
	}
}

// --- fixtures helper ---

// streamDisciplineFixtures holds materialized fixture file paths and env helpers.
type streamDisciplineFixtures struct {
	happy            string
	assertionFailure string
	parseError       string
	perfRequest      string
}

// materializeStreamDisciplineFixtures copies the source fixtures from
// testdata/stream-discipline/ to a temp dir, substituting {{BASE_URL}} with
// the provided URL so tests are hermetic.
func materializeStreamDisciplineFixtures(t *testing.T, baseURL string) streamDisciplineFixtures {
	t.Helper()
	srcDir := filepath.Join("testdata", "stream-discipline")
	dstDir := t.TempDir()
	copyAndSubstitute := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(srcDir, name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		content := strings.ReplaceAll(string(raw), "{{BASE_URL}}", baseURL)
		out := filepath.Join(dstDir, name)
		if err := os.WriteFile(out, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		return out
	}

	// Perf uses writeStreamProgressRequestFile (already in stream_progress_test.go).
	perfReq := writeStreamProgressRequestFile(t, baseURL)

	return streamDisciplineFixtures{
		happy:            copyAndSubstitute("happy.yaml"),
		assertionFailure: copyAndSubstitute("assertion_failure.yaml"),
		parseError:       copyAndSubstitute("parse_error.yaml"),
		perfRequest:      perfReq,
	}
}

// filterSDMEnv returns env with entries matching key= prefix removed.
// Named to avoid collision with any future filterEnv helper in the same package.
func filterSDMEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}
