package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sourceEvent is the subset of assertion.result this file inspects.
type sourceEvent struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	SourceFile string `json:"source_file"`
	SourceLine int    `json:"source_line"`
}

// lineOf returns the 1-based line number of the only line in body containing
// needle. It fails the test if the needle is absent or ambiguous, so a test
// collection edited later cannot silently start asserting the wrong line.
func lineOf(t *testing.T, body, needle string) int {
	t.Helper()
	found := 0
	for i, line := range strings.Split(body, "\n") {
		if strings.Contains(line, needle) {
			found = i + 1
		}
	}
	if found == 0 {
		t.Fatalf("needle %q not found in collection", needle)
	}
	if strings.Count(body, needle) != 1 {
		t.Fatalf("needle %q appears %d times; it must be unique", needle, strings.Count(body, needle))
	}
	return found
}

// TestEvents_AssertionResultCarriesSourceLocation pins the source pointer for
// every assertion kind to the line a developer would edit to change that
// assertion — the operator line for body and header assertions, the `status:`
// line, the `max_duration_ms:` line, the `schema:` line, and the list-item
// line for a cel: expression. Pointing at the enclosing request instead would
// make the pointer useless on a request with a dozen assertions.
func TestEvents_AssertionResultCarriesSourceLocation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"name":"ada"}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "user.schema.json")
	if err := os.WriteFile(schemaPath, []byte(`{"type":"object"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	collection := `name: source-pointers
requests:
  - name: fetch user
    request:
      method: GET
      url: "` + srv.URL + `/"
    assertions:
      status: 999
      body:
        $.user.name:
          equals: "WRONG"
      headers:
        Content-Type:
          equals: "text/plain"
      timing:
        max_duration_ms: 1
      schema: user.schema.json
      cel:
        - "response.status == 418"
`
	collPath := writeCollection(t, dir, "c.yaml", collection)
	evPath := filepath.Join(dir, "ev.ndjson")
	_, stderr, _ := captureRun(t, "run", collPath, "--events", evPath)

	raw, err := os.ReadFile(evPath)
	if err != nil {
		t.Fatalf("no event stream written; stderr=%q", stderr)
	}

	// label -> the unique collection text whose line the pointer must name.
	wantAnchor := map[string]string{
		"status":                     "status: 999",
		"body $.user.name equals":    `equals: "WRONG"`,
		"header Content-Type equals": `equals: "text/plain"`,
		"timing":                     "max_duration_ms: 1",
		"schema":                     "schema: user.schema.json",
		"assertions[0].cel":          `- "response.status == 418"`,
	}

	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var ev sourceEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("malformed NDJSON: %v", err)
		}
		if ev.Kind != "assertion.result" {
			continue
		}
		// The schema label carries the resolved absolute schema path, so match
		// it on its prefix; every other kind has a fixed label.
		key := ev.Label
		if strings.HasPrefix(key, "schema ") {
			key = "schema"
		}
		anchor, ok := wantAnchor[key]
		if !ok {
			t.Errorf("unexpected assertion label %q — update this test's anchor table", ev.Label)
			continue
		}
		seen[key] = true

		if ev.SourceFile == "" {
			t.Errorf("assertion %q carries no source_file", ev.Label)
		} else if ev.SourceFile != collPath {
			t.Errorf("assertion %q: source_file = %q, want %q", ev.Label, ev.SourceFile, collPath)
		}
		if want := lineOf(t, collection, anchor); ev.SourceLine != want {
			t.Errorf("assertion %q: source_line = %d, want %d (the %q line)",
				ev.Label, ev.SourceLine, want, anchor)
		}
	}

	for label := range wantAnchor {
		if !seen[label] {
			t.Errorf("no assertion.result emitted for %q", label)
		}
	}
}
