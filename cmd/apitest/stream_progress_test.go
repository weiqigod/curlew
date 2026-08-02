package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStreamProgress is the M7-002 regression guard:
// for every affected subcommand, piping stdout must capture ONLY the result
// payload — progress/confirmation lines must appear on stderr.
func TestStreamProgress(t *testing.T) {
	binary := buildBinary(t)

	t.Run("perf", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
		}))
		defer srv.Close()

		f := writeStreamProgressRequestFile(t, srv.URL)
		stdout, stderr, rc := runBinary(t, binary,
			"perf", f, "--vus", "1", "--duration", "100ms")
		if rc != 0 {
			t.Fatalf("perf exit = %d; stderr=%q", rc, stderr)
		}
		// Stdout should contain ONLY the final Results: line.
		if !strings.Contains(stdout, "Results: requests=") {
			t.Errorf("stdout missing 'Results: requests=': %q", stdout)
		}
		for _, leak := range []string{"Load test:", "Running...", "Requests sent:", "VUs:"} {
			if strings.Contains(stdout, leak) {
				t.Errorf("progress leaked to stdout: %q; full stdout: %q", leak, stdout)
			}
		}
		for _, want := range []string{"Load test:", "Running...", "Requests sent:"} {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr missing %q; full stderr: %q", want, stderr)
			}
		}
	})

	t.Run("exec_dry_run", func(t *testing.T) {
		stdout, stderr, rc := runBinary(t, binary, "exec", "https://example.com", "--dry-run")
		if rc != 0 {
			t.Fatalf("exec --dry-run exit = %d; stderr=%q", rc, stderr)
		}
		if !strings.Contains(stdout, "DRY RUN") {
			t.Errorf("stdout missing 'DRY RUN': %q", stdout)
		}
		if !strings.Contains(stdout, "  > GET https://example.com") {
			t.Errorf("stdout missing RequestDetail line (  > GET url): %q", stdout)
		}
		// --dry-run has no stderr progress (no HTTP traffic).
		_ = stderr
	})

	// Worker E2E: The library-level integration test at
	// internal/worker/integration_test.go asserts the stream split at the
	// library boundary (Stderr buffer contains progress; stdout is empty),
	// which provides equivalent coverage for stream-routing regressions.
	// A binary-level worker test is omitted here: it would require setting up
	// a fake coordinator httptest.Server and env vars, adding substantial
	// complexity for no additional regression signal.
}

// writeStreamProgressRequestFile writes a minimal YAML request file to a temp
// dir and returns its path. Separated from perf_test.go's writePerfRequestFile
// to keep this test file self-contained.
func writeStreamProgressRequestFile(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "req.yaml")
	content := "name: stream-progress-smoke\nrequest:\n  method: GET\n  url: \"" + url + "\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write request file: %v", err)
	}
	return path
}
