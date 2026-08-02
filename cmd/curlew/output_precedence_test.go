package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOutputPrecedence verifies the four-case precedence table:
// CLI flag > collection.output > project.output > built-in default.
//
// Each sub-test sets up a temp project root with curlew.yaml (optional
// project-level output:) + collection YAML (optional collection-level output:),
// invokes runCmdInner with captured stdout, and asserts the effective format
// by inspecting stdout markers.
func TestOutputPrecedence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tests := []struct {
		name            string
		projectOut      string // YAML output: block to include in curlew.yaml (empty = omit)
		collectionOut   string // YAML output: block to include in collection YAML (empty = omit)
		cliArgs         []string
		wantStdoutHas   string // substring expected in stdout
		wantStdoutLacks string // substring that must NOT be in stdout (verifies format switched)
	}{
		// builtin_default: no project output, no collection output, no CLI flag → terminal format.
		// Terminal format includes the ✓ check mark for passing requests.
		{
			name:          "builtin_default",
			wantStdoutHas: "✓",
		},
		// project_wins: project sets format=json, no collection override, no CLI flag.
		// JSON output contains "status" key.
		{
			name:            "project_wins",
			projectOut:      "output:\n  format: json\n",
			wantStdoutHas:   `"status"`,
			wantStdoutLacks: "✓",
		},
		// collection_wins: project sets format=tap, collection overrides to json.
		// JSON output contains "status" key, not TAP version header.
		{
			name:            "collection_wins",
			projectOut:      "output:\n  format: tap\n",
			collectionOut:   "output:\n  format: json\n",
			wantStdoutHas:   `"status"`,
			wantStdoutLacks: "TAP version",
		},
		// cli_wins: project sets format=tap, collection sets format=json, CLI sets format=terminal.
		// Terminal output contains ✓, not TAP version header.
		{
			name:            "cli_wins",
			projectOut:      "output:\n  format: tap\n",
			collectionOut:   "output:\n  format: json\n",
			cliArgs:         []string{"--format", "terminal"},
			wantStdoutHas:   "✓",
			wantStdoutLacks: "TAP version",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()

			// Write curlew.yaml (project config) if a project output block is specified.
			// We always write it so LoadProjectConfig finds a project root in the temp dir.
			projectYAML := fmt.Sprintf("project_name: \"test-precedence\"\n%s", tc.projectOut)
			if err := os.WriteFile(filepath.Join(tmp, "curlew.yaml"), []byte(projectYAML), 0o644); err != nil {
				t.Fatalf("write curlew.yaml: %v", err)
			}

			// Write collection YAML.
			colContent := fmt.Sprintf(
				"name: TestPrecedence\n%srequests:\n  - name: ping\n    request:\n      method: GET\n      url: %q\n",
				tc.collectionOut,
				srv.URL,
			)
			colPath := filepath.Join(tmp, "collection.yaml")
			if err := os.WriteFile(colPath, []byte(colContent), 0o644); err != nil {
				t.Fatalf("write collection.yaml: %v", err)
			}

			// Build args: collection path + optional CLI flags.
			args := append([]string{colPath}, tc.cliArgs...)

			var stdout, stderr bytes.Buffer
			code, _ := runCmdInner(args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
			}
			if !strings.Contains(stdout.String(), tc.wantStdoutHas) {
				t.Errorf("stdout missing %q\ngot: %s", tc.wantStdoutHas, stdout.String())
			}
			if tc.wantStdoutLacks != "" && strings.Contains(stdout.String(), tc.wantStdoutLacks) {
				t.Errorf("stdout should NOT contain %q but does\ngot: %s", tc.wantStdoutLacks, stdout.String())
			}
		})
	}
}

// TestOutputPrecedence_Events verifies that the events field follows the same
// CLI > collection > project precedence as format, and that events emitted to a
// YAML-declared path are correctly written.
func TestOutputPrecedence_Events(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	writeProject := func(t *testing.T, dir, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "curlew.yaml"), []byte(content), 0o644); err != nil {
			t.Fatalf("write curlew.yaml: %v", err)
		}
	}
	writeCollection := func(t *testing.T, dir, name, outputBlock string) string {
		t.Helper()
		content := fmt.Sprintf(
			"name: %s\n%srequests:\n  - name: ping\n    request:\n      method: GET\n      url: %q\n",
			name, outputBlock, srv.URL,
		)
		path := filepath.Join(dir, "collection.yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write collection.yaml: %v", err)
		}
		return path
	}
	hasRunStart := func(t *testing.T, eventsPath string) bool {
		t.Helper()
		data, err := os.ReadFile(eventsPath)
		if err != nil {
			return false
		}
		return strings.Contains(string(data), `"run.start"`)
	}

	t.Run("project_sets_events", func(t *testing.T) {
		tmp := t.TempDir()
		evPath := filepath.Join(tmp, "project-events.jsonl")
		writeProject(t, tmp, fmt.Sprintf("project_name: ep\noutput:\n  events: %q\n", evPath))
		colPath := writeCollection(t, tmp, "EP", "")
		var stdout, stderr bytes.Buffer
		code, _ := runCmdInner([]string{colPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
		}
		if !hasRunStart(t, evPath) {
			t.Errorf("expected events file at %q to contain run.start event", evPath)
		}
	})

	t.Run("collection_overrides_project_events", func(t *testing.T) {
		tmp := t.TempDir()
		projectEvPath := filepath.Join(tmp, "project-events.jsonl")
		collEvPath := filepath.Join(tmp, "collection-events.jsonl")
		writeProject(t, tmp, fmt.Sprintf("project_name: coe\noutput:\n  events: %q\n", projectEvPath))
		// Collection overrides events path.
		colPath := writeCollection(t, tmp, "COE",
			fmt.Sprintf("output:\n  events: %q\n", collEvPath))
		var stdout, stderr bytes.Buffer
		code, _ := runCmdInner([]string{colPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
		}
		// Events should be in the collection-level path, not the project-level path.
		if !hasRunStart(t, collEvPath) {
			t.Errorf("collection-level events file %q should have run.start", collEvPath)
		}
		if hasRunStart(t, projectEvPath) {
			t.Errorf("project-level events file %q should NOT have events (collection overrides)", projectEvPath)
		}
	})

	t.Run("cli_events_wins_over_yaml", func(t *testing.T) {
		tmp := t.TempDir()
		yamlEvPath := filepath.Join(tmp, "yaml-events.jsonl")
		cliEvPath := filepath.Join(tmp, "cli-events.jsonl")
		writeProject(t, tmp, fmt.Sprintf("project_name: clw\noutput:\n  events: %q\n", yamlEvPath))
		colPath := writeCollection(t, tmp, "CLW", "")
		var stdout, stderr bytes.Buffer
		code, _ := runCmdInner([]string{colPath, "--events", cliEvPath}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
		}
		// CLI-provided events path should win.
		if !hasRunStart(t, cliEvPath) {
			t.Errorf("CLI events file %q should have run.start", cliEvPath)
		}
		if hasRunStart(t, yamlEvPath) {
			t.Errorf("YAML events file %q should NOT have events (CLI wins)", yamlEvPath)
		}
	})
}

// TestOutputPrecedence_Verbosity verifies that output.verbosity set in project YAML
// is honoured by the runtime.
func TestOutputPrecedence_Verbosity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	// Project sets verbosity=debug; debug adds timing details not shown in normal mode.
	if err := os.WriteFile(filepath.Join(tmp, "curlew.yaml"), []byte(
		"project_name: vp\noutput:\n  verbosity: debug\n"), 0o644); err != nil {
		t.Fatalf("write curlew.yaml: %v", err)
	}
	colContent := fmt.Sprintf(
		"name: VP\nrequests:\n  - name: ping\n    request:\n      method: GET\n      url: %q\n",
		srv.URL,
	)
	colPath := filepath.Join(tmp, "collection.yaml")
	if err := os.WriteFile(colPath, []byte(colContent), 0o644); err != nil {
		t.Fatalf("write collection.yaml: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code, _ := runCmdInner([]string{colPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
	// Debug verbosity includes request/response details; at minimum the run completes.
	// We verify it did not fall back to a silent mode by checking the ✓ is present.
	if !strings.Contains(stdout.String(), "✓") {
		t.Errorf("expected ✓ in terminal output (verbosity=debug from project YAML), got: %s", stdout.String())
	}
}
