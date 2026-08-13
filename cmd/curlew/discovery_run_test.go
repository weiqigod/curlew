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

	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/runner"
)

func TestWorseExitCode(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{"both zero", 0, 0, 0},
		{"assertion beats network", 1, 4, 1},
		{"feature gate beats all", 6, 1, 6},
		{"collection error beats guard rail", 3, 2, 3},
		{"guard rail beats network", 2, 4, 2},
		{"config error beats guard rail", 5, 2, 5},
		{"zero and assertion", 0, 1, 1},
		{"a zero b network", 0, 4, 4},
		{"symmetric assertion beats network", 4, 1, 1},
		// Unknown exit codes are treated as severity 0 (success-equivalent).
		// When both have equal severity, a is returned unchanged.
		{"unknown a vs zero same severity returns a", 99, 0, 99},
		{"zero vs unknown b same severity returns a", 0, 99, 0},
		{"unknown vs unknown same severity returns a", 99, 98, 99},
		{"known beats unknown", 1, 99, 1},
		{"unknown loses to known", 99, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worseExitCode(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("worseExitCode(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestAggregateExitCodes(t *testing.T) {
	tests := []struct {
		name  string
		codes []int
		want  int
	}{
		{"empty returns 0", []int{}, 0},
		{"all pass", []int{0, 0, 0}, 0},
		{"one failure wins", []int{0, 1, 0}, 1},
		{"feature gate wins all", []int{0, 1, 6}, 6},
		{"network error", []int{0, 4, 0}, 4},
		{"guard rail beats network", []int{4, 2}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateExitCodes(tt.codes)
			if got != tt.want {
				t.Errorf("aggregateExitCodes(%v) = %d, want %d", tt.codes, got, tt.want)
			}
		})
	}
}

func TestBuildArgsForCollection(t *testing.T) {
	seed42 := int64(42)

	tests := []struct {
		name           string
		path           string
		envName        string
		format         string
		report         string
		cliVars        map[string]string
		envVarVars     map[string]string
		seed           *int64
		color          colorMode
		verbosity      output.Verbosity
		allowSensitive bool
		showDeps       bool
		dryRun         bool
		runParallel    bool
		confirmLargeDS bool
		wantArgs       []string
	}{
		{
			name:     "path only",
			path:     "col.yaml",
			wantArgs: []string{"col.yaml"},
		},
		{
			name:     "with env",
			path:     "col.yaml",
			envName:  "staging",
			wantArgs: []string{"col.yaml", "--env", "staging"},
		},
		{
			name:     "with format",
			path:     "col.yaml",
			format:   "json",
			wantArgs: []string{"col.yaml", "--format", "json"},
		},
		{
			name:     "with report",
			path:     "col.yaml",
			report:   "out.xml",
			wantArgs: []string{"col.yaml", "--report", "out.xml"},
		},
		{
			name:     "with cliVars sorted",
			path:     "col.yaml",
			cliVars:  map[string]string{"z": "last", "a": "first"},
			wantArgs: []string{"col.yaml", "--var", "a=first", "--var", "z=last"},
		},
		{
			name:       "with envVarVars sorted",
			path:       "col.yaml",
			envVarVars: map[string]string{"m": "val", "b": "other"},
			wantArgs:   []string{"col.yaml", "--var", "b=other", "--var", "m=val"},
		},
		{
			name:     "with seed",
			path:     "col.yaml",
			seed:     &seed42,
			wantArgs: []string{"col.yaml", "--seed", fmt.Sprintf("%d", seed42)},
		},
		{
			// Forwarded as --color=never rather than --no-color: the two mean
			// the same thing to the receiving parser, and the explicit form is
			// what lets `always` be forwarded too.
			name:     "colour disabled",
			path:     "col.yaml",
			color:    colorNever,
			wantArgs: []string{"col.yaml", "--color=never"},
		},
		{
			name:     "colour forced",
			path:     "col.yaml",
			color:    colorAlways,
			wantArgs: []string{"col.yaml", "--color=always"},
		},
		{
			// auto is the default and is not forwarded, so a discovered
			// collection decides for itself exactly as a direct run would.
			name:     "colour auto is not forwarded",
			path:     "col.yaml",
			color:    colorAuto,
			wantArgs: []string{"col.yaml"},
		},
		{
			name:      "verbosity verbose",
			path:      "col.yaml",
			verbosity: output.VerbosityVerbose,
			wantArgs:  []string{"col.yaml", "-v"},
		},
		{
			name:      "verbosity debug",
			path:      "col.yaml",
			verbosity: output.VerbosityDebug,
			wantArgs:  []string{"col.yaml", "-vv"},
		},
		{
			name:      "verbosity quiet",
			path:      "col.yaml",
			verbosity: output.VerbosityQuiet,
			wantArgs:  []string{"col.yaml", "-q"},
		},
		{
			name:           "allow sensitive",
			path:           "col.yaml",
			allowSensitive: true,
			wantArgs:       []string{"col.yaml", "--allow-sensitive"},
		},
		{
			name:     "show dependencies",
			path:     "col.yaml",
			showDeps: true,
			wantArgs: []string{"col.yaml", "--show-dependencies"},
		},
		{
			name:     "dry run",
			path:     "col.yaml",
			dryRun:   true,
			wantArgs: []string{"col.yaml", "--dry-run"},
		},
		{
			name:        "parallel",
			path:        "col.yaml",
			runParallel: true,
			wantArgs:    []string{"col.yaml", "--parallel"},
		},
		{
			name:           "confirm large dataset",
			path:           "col.yaml",
			confirmLargeDS: true,
			wantArgs:       []string{"col.yaml", "--confirm-large-dataset"},
		},
		{
			name:           "all flags combined",
			path:           "col.yaml",
			envName:        "prod",
			format:         "tap",
			report:         "report.xml",
			cliVars:        map[string]string{"x": "1"},
			envVarVars:     map[string]string{"y": "2"},
			seed:           &seed42,
			color:          colorNever,
			verbosity:      output.VerbosityVerbose,
			allowSensitive: true,
			showDeps:       true,
			dryRun:         true,
			runParallel:    true,
			confirmLargeDS: true,
			wantArgs: []string{
				"col.yaml",
				"--env", "prod",
				"--format", "tap",
				"--report", "report.xml",
				"--var", "x=1",
				"--var", "y=2",
				"--seed", fmt.Sprintf("%d", seed42),
				"--color=never",
				"-v",
				"--allow-sensitive",
				"--show-dependencies",
				"--dry-run",
				"--parallel",
				"--confirm-large-dataset",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgsForCollection(
				tt.path, tt.envName, tt.format, tt.report,
				tt.cliVars, tt.envVarVars, tt.seed,
				tt.color, tt.verbosity,
				tt.allowSensitive, tt.showDeps, tt.dryRun, tt.runParallel, tt.confirmLargeDS,
			)
			if len(got) != len(tt.wantArgs) {
				t.Fatalf("buildArgsForCollection() = %v (len %d), want %v (len %d)",
					got, len(got), tt.wantArgs, len(tt.wantArgs))
			}
			for i := range got {
				if got[i] != tt.wantArgs[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tt.wantArgs[i])
				}
			}
		})
	}
}

func TestAggregateSummaries(t *testing.T) {
	dur1 := 100 * time.Millisecond
	dur2 := 200 * time.Millisecond

	tests := []struct {
		name string
		in   []*runner.Summary
		want runner.Summary
	}{
		{
			name: "empty input returns zero",
			in:   []*runner.Summary{},
			want: runner.Summary{},
		},
		{
			name: "single passing summary",
			in: []*runner.Summary{
				{Total: 3, Passed: 3, Duration: dur1},
			},
			want: runner.Summary{Total: 3, Passed: 3, Duration: dur1},
		},
		{
			name: "two summaries sum counts",
			in: []*runner.Summary{
				{Total: 3, Passed: 3, Duration: dur1},
				{Total: 2, Passed: 1, Failed: 1, AssertionFailures: 1, Duration: dur2},
			},
			want: runner.Summary{
				Total: 5, Passed: 4, Failed: 1,
				AssertionFailures: 1,
				Duration:          dur1 + dur2,
			},
		},
		{
			name: "any limit_exceeded wins",
			in: []*runner.Summary{
				{Total: 5, Passed: 5},
				{Total: 3, LimitExceeded: true, RequestsExecuted: 3},
			},
			want: runner.Summary{
				Total:            8,
				Passed:           5,
				LimitExceeded:    true,
				RequestsExecuted: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregateSummaries(tt.in)
			if got.Total != tt.want.Total {
				t.Errorf("Total = %d, want %d", got.Total, tt.want.Total)
			}
			if got.Passed != tt.want.Passed {
				t.Errorf("Passed = %d, want %d", got.Passed, tt.want.Passed)
			}
			if got.Failed != tt.want.Failed {
				t.Errorf("Failed = %d, want %d", got.Failed, tt.want.Failed)
			}
			if got.AssertionFailures != tt.want.AssertionFailures {
				t.Errorf("AssertionFailures = %d, want %d", got.AssertionFailures, tt.want.AssertionFailures)
			}
			if got.LimitExceeded != tt.want.LimitExceeded {
				t.Errorf("LimitExceeded = %v, want %v", got.LimitExceeded, tt.want.LimitExceeded)
			}
			if tt.want.Duration > 0 && got.Duration != tt.want.Duration {
				t.Errorf("Duration = %v, want %v", got.Duration, tt.want.Duration)
			}
		})
	}
}

// writeNamedCollection writes a collection YAML with the given name to dir/<filename>.
func writeNamedCollection(t *testing.T, dir, filename, name, serverURL string) string {
	t.Helper()
	f := filepath.Join(dir, filename)
	content := fmt.Sprintf("name: %s\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: %q\n", name, serverURL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// TestCaptureJSONCollection_DoesNotTouchOsStdout proves that captureJSONCollection
// no longer reassigns os.Stdout (M7-005 removed the fd-swap).
func TestCaptureJSONCollection_DoesNotTouchOsStdout(t *testing.T) {
	origStdout := os.Stdout
	origStderr := os.Stderr
	t.Cleanup(func() {
		if os.Stdout != origStdout {
			t.Errorf("os.Stdout was replaced during test run")
		}
		if os.Stderr != origStderr {
			t.Errorf("os.Stderr was replaced during test run")
		}
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	f := writeNamedCollection(t, tmpDir, "col.yaml", "TestCapture", srv.URL)

	args := buildArgsForCollection(f, "", "json", "", nil, nil, nil, colorNever, output.VerbosityDefault,
		false, false, false, false, false)
	var stderr bytes.Buffer
	doc, _, code := captureJSONCollection(args, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if doc == nil || doc.Status != "passed" {
		t.Errorf("expected passed status; got %+v", doc)
	}
}

// TestConcurrentDiscovery is the primary regression gate for M7-005.
// It verifies that two concurrent runCmdInner calls each write to their own
// buffer without interleaving — previously impossible with the os.Stdout fd-swap.
func TestConcurrentDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	colA := writeNamedCollection(t, tmpDir, "a.yaml", "collection_A", srv.URL)
	colB := writeNamedCollection(t, tmpDir, "b.yaml", "collection_B", srv.URL)

	var (
		wg           sync.WaitGroup
		stdoutA      bytes.Buffer
		stdoutB      bytes.Buffer
		stderrA      bytes.Buffer
		stderrB      bytes.Buffer
		codeA, codeB int
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		codeA, _ = runCmdInner([]string{colA, "--format", "json"}, &stdoutA, &stderrA)
	}()
	go func() {
		defer wg.Done()
		codeB, _ = runCmdInner([]string{colB, "--format", "json"}, &stdoutB, &stderrB)
	}()
	wg.Wait()

	if codeA != 0 || codeB != 0 {
		t.Fatalf("exit codes codeA=%d codeB=%d stderrA=%q stderrB=%q",
			codeA, codeB, stderrA.String(), stderrB.String())
	}

	// Each buffer must contain a single well-formed JSON document.
	var docA, docB output.JSONOutput
	if err := json.Unmarshal(bytes.TrimSpace(stdoutA.Bytes()), &docA); err != nil {
		t.Fatalf("buffer A not valid JSON (interleaved?): %v\nraw: %q", err, stdoutA.String())
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdoutB.Bytes()), &docB); err != nil {
		t.Fatalf("buffer B not valid JSON (interleaved?): %v\nraw: %q", err, stdoutB.String())
	}
	if docA.Name != "collection_A" {
		t.Errorf("buffer A got collection %q, want collection_A", docA.Name)
	}
	if docB.Name != "collection_B" {
		t.Errorf("buffer B got collection %q, want collection_B", docB.Name)
	}
	// Each buffer must not contain the other collection's name.
	if strings.Contains(stdoutA.String(), "collection_B") {
		t.Errorf("buffer A contains content from buffer B — interleaving detected")
	}
	if strings.Contains(stdoutB.String(), "collection_A") {
		t.Errorf("buffer B contains content from buffer A — interleaving detected")
	}
}
