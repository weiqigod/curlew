package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// TestEnvironmentHeaders_SentOnEveryRequest: the same collection, run with an
// environment that has a headers: block, sends those headers on every request,
// including requests from an included collection, while a request's own header
// wins. Run with an environment without headers, it sends none of them.
func TestEnvironmentHeaders_SentOnEveryRequest(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "sends them on every request of the run"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}
	if _, err := docs.Prose("MANUAL.md", "block sends none of them"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}

	var (
		mu       sync.Mutex
		received = map[string]http.Header{}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received[r.URL.Path] = r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCollection(t, filepath.Join(dir, "environments"), "real.yaml", `variables:
  base_url: "`+srv.URL+`"
`)
	writeCollection(t, filepath.Join(dir, "environments"), "mock.yaml", `variables:
  base_url: "`+srv.URL+`"
  mock_run_id: demo-1
headers:
  X-Mock-Run: "{{mock_run_id}}"
  X-Mock-Api: quiddly
`)
	writeCollection(t, dir, "checks.yaml", `name: checks
requests:
  - name: included
    request:
      url: "{{base_url}}/included"
`)
	collPath := writeCollection(t, dir, "c.yaml", `name: environment headers
include:
  - checks.yaml
setup:
  - name: start
    request:
      url: "{{base_url}}/setup"
requests:
  - name: own api
    request:
      url: "{{base_url}}/own"
      headers:
        x-mock-api: credit
teardown:
  - name: end
    request:
      url: "{{base_url}}/teardown"
`)

	_, stderr, code := captureRun(t, "run", collPath, "--env", "mock")
	if code != 0 {
		t.Fatalf("mock run exit %d; stderr=%q", code, stderr)
	}
	mu.Lock()
	for _, path := range []string{"/setup", "/included", "/teardown"} {
		h := received[path]
		if h.Get("X-Mock-Run") != "demo-1" || h.Get("X-Mock-Api") != "quiddly" {
			t.Errorf("%s: got X-Mock-Run=%q X-Mock-Api=%q, want demo-1 and quiddly", path, h.Get("X-Mock-Run"), h.Get("X-Mock-Api"))
		}
	}
	own := received["/own"]
	if got := own.Values("X-Mock-Api"); len(got) != 1 || got[0] != "credit" {
		t.Errorf("/own: X-Mock-Api = %v, want only the request's own value credit", got)
	}
	if own.Get("X-Mock-Run") != "demo-1" {
		t.Errorf("/own: X-Mock-Run = %q, want demo-1", own.Get("X-Mock-Run"))
	}
	received = map[string]http.Header{}
	mu.Unlock()

	_, stderr, code = captureRun(t, "run", collPath, "--env", "real")
	if code != 0 {
		t.Fatalf("real run exit %d; stderr=%q", code, stderr)
	}
	mu.Lock()
	defer mu.Unlock()
	for path, h := range received {
		if _, ok := h["X-Mock-Run"]; ok {
			t.Errorf("%s: X-Mock-Run sent with an environment that has no headers", path)
		}
		if path != "/own" {
			if _, ok := h["X-Mock-Api"]; ok {
				t.Errorf("%s: X-Mock-Api sent with an environment that has no headers", path)
			}
		}
	}
}

// TestEnvironmentHeaders_InvalidBlockIsAConfigError: a headers: block that is
// not a map of names to text stops the run before any request, with exit 3.
func TestEnvironmentHeaders_InvalidBlockIsAConfigError(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "must map each name to one text value and list each name once"); err != nil {
		t.Fatalf("documented claim: %v", err)
	}

	sent := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sent = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeCollection(t, filepath.Join(dir, "environments"), "bad.yaml", "headers:\n  - X-Mock-Run\n")
	collPath := writeCollection(t, dir, "c.yaml", `name: bad headers
requests:
  - name: one
    request:
      url: "`+srv.URL+`/one"
`)

	_, _, code := captureRun(t, "run", collPath, "--env", "bad")
	if code != 3 {
		t.Errorf("exit %d, want 3 (configuration error)", code)
	}
	if sent {
		t.Error("a request was sent despite the invalid environment file")
	}
}
