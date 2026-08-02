package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/peterlindqvist/apitest/internal/output"
)

// chdirTemp changes into dir for the duration of the test, restoring the
// original working directory at cleanup. Tests that use os.Chdir must
// call this to avoid polluting other tests that run after.
func chdirTemp(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// collectionYAML creates a minimal collection YAML that GETs the given URL
// and asserts status 200.
func collectionYAML(name, url string) string {
	return fmt.Sprintf(`name: %s
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 200
`, name, url)
}

// failCollectionYAML creates a collection that GETs the URL but asserts status 404 (always fails).
func failCollectionYAML(name, url string) string {
	return fmt.Sprintf(`name: %s
requests:
  - name: Get
    request:
      method: GET
      url: "%s"
    assertions:
      status: 404
`, name, url)
}

func TestRunCmd_GlobDiscovery_ThreeCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a_test.yaml"), []byte(collectionYAML("A", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b_test.yaml"), []byte(collectionYAML("B", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "c_test.yaml"), []byte(collectionYAML("C", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	// Non-matching file — should not be executed.
	if err := os.WriteFile(filepath.Join(dir, "ignore.yaml"), []byte(collectionYAML("Ignored", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}

	chdirTemp(t, dir)

	stdout, _, exitCode := captureRunCmd(t, "**/*_test.yaml")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stdout: %s", exitCode, stdout)
	}
	// Verify all 3 collection names appear in output.
	for _, name := range []string{"A", "B", "C"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("expected collection %q in stdout, got: %s", name, stdout)
		}
	}
	// Ignored collection should not appear.
	if strings.Contains(stdout, "Ignored") {
		t.Errorf("unexpected 'Ignored' collection in stdout: %s", stdout)
	}
}

func TestRunCmd_GlobDiscovery_ZeroMatches(t *testing.T) {
	dir := t.TempDir()
	chdirTemp(t, dir)

	_, stderr, exitCode := captureRunCmd(t, "**/*.nomatch")
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "no collections matched") {
		t.Errorf("expected 'no collections matched' in stderr, got: %s", stderr)
	}
}

func TestRunCmd_GlobDiscovery_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	chdirTemp(t, dir)

	_, stderr, exitCode := captureRunCmd(t, "../**/*.yaml")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stderr, "escapes working directory") {
		t.Errorf("expected traversal error in stderr, got: %s", stderr)
	}
}

func TestRunCmd_GlobDiscovery_MiddleFailureDoesNotAbort(t *testing.T) {
	var callCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	// Three collections: A passes, B fails assertion, C passes.
	if err := os.WriteFile(filepath.Join(dir, "a_test.yaml"), []byte(collectionYAML("A", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	// B always fails (asserts 404 but server returns 200).
	if err := os.WriteFile(filepath.Join(dir, "b_test.yaml"), []byte(failCollectionYAML("B", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c_test.yaml"), []byte(collectionYAML("C", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}

	chdirTemp(t, dir)

	_, _, exitCode := captureRunCmd(t, "**/*_test.yaml")
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1 (assertion failure)", exitCode)
	}
	// All three collections must have been executed (3 HTTP calls).
	if got := atomic.LoadInt64(&callCount); got != 3 {
		t.Errorf("HTTP call count = %d, want 3 (all collections ran)", got)
	}
}

func TestRunCmd_GlobDiscovery_JSONFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	for _, name := range []string{"a_test", "b_test"} {
		content := collectionYAML(strings.ToUpper(name[:1]), srv.URL)
		if err := os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	chdirTemp(t, dir)

	stdout, _, exitCode := captureRunCmd(t, "**/*_test.yaml", "--format", "json")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0; stdout: %s", exitCode, stdout)
	}

	var parsed output.MultiJSONOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v\nstdout: %s", err, stdout)
	}
	if parsed.Total != 2 {
		t.Errorf("total_collections = %d, want 2", parsed.Total)
	}
	if len(parsed.Collections) != 2 {
		t.Errorf("len(collections) = %d, want 2", len(parsed.Collections))
	}
}

func TestRunCmd_GlobDiscovery_LiteralPathUnchanged(t *testing.T) {
	// A literal path (no glob metachars) must bypass discovery entirely.
	// We verify this by using a tmpdir where no file exists — a glob would
	// return ErrNoMatches but a literal path returns the parse error (exit 3).
	dir := t.TempDir()
	chdirTemp(t, dir)

	_, _, exitCode := captureRunCmd(t, "nonexistent.yaml")
	// Exit code 3 = collection parse error (file not found), NOT exit code 2
	// (ErrNoMatches from discovery). This proves discovery was NOT invoked.
	if exitCode != 3 {
		t.Errorf("exit code = %d, want 3 (file-not-found, not discovery)", exitCode)
	}
}

func TestRunCmd_GlobDiscovery_Ignored(t *testing.T) {
	var callCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	draftsDir := filepath.Join(dir, "drafts")
	if err := os.MkdirAll(draftsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// One real collection.
	if err := os.WriteFile(filepath.Join(dir, "a_test.yaml"), []byte(collectionYAML("A", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	// One draft — should be ignored.
	if err := os.WriteFile(filepath.Join(draftsDir, "draft_test.yaml"), []byte(collectionYAML("Draft", srv.URL)), 0o600); err != nil {
		t.Fatal(err)
	}
	// .apitestignore excludes drafts.
	if err := os.WriteFile(filepath.Join(dir, ".apitestignore"), []byte("**/drafts/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	chdirTemp(t, dir)

	_, _, exitCode := captureRunCmd(t, "**/*_test.yaml")
	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}
	// Only 1 HTTP call (draft was ignored).
	if got := atomic.LoadInt64(&callCount); got != 1 {
		t.Errorf("HTTP call count = %d, want 1 (draft ignored)", got)
	}
}
