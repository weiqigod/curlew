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

// streamEvent is the subset of the NDJSON envelope this file inspects.
type streamEvent struct {
	Kind        string `json:"kind"`
	RequestID   string `json:"request_id"`
	RequestSlug string `json:"request_slug"`
	Label       string `json:"label"`
}

func readEventStream(t *testing.T, path string) []streamEvent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events stream: %v", err)
	}
	var out []streamEvent
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		var ev streamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("malformed NDJSON line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	if len(out) == 0 {
		t.Fatalf("event stream %s is empty", path)
	}
	return out
}

// runCollectionForEvents executes body against a stub server and returns the
// parsed event stream. The collection text may reference {{base_url}}.
func runCollectionForEvents(t *testing.T, body string) []streamEvent {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user":{"name":"ada"},"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeCollection(t, dir, "c.yaml", strings.ReplaceAll(body, "{{base_url}}", srv.URL))
	evPath := filepath.Join(dir, "ev.ndjson")
	// Exit code is deliberately ignored: these collections contain failing
	// assertions on purpose, and a failing run still emits a full stream.
	_, stderr, _ := captureRun(t, "run", filepath.Join(dir, "c.yaml"), "--events", evPath)
	if _, err := os.Stat(evPath); err != nil {
		t.Fatalf("no event stream written; stderr=%q", stderr)
	}
	return readEventStream(t, evPath)
}

// assertEveryAssertionNamesItsRequest is the invariant: an assertion.result
// must identify its request on its own. Reading request_id alone forces a
// consumer to buffer the whole stream to build a req-N → slug table, and that
// table is run-local — request ids are positional, so inserting a request
// shifts every id after it. A tailing consumer (curlew watch) and a consumer
// comparing two runs both need the stable slug on the event itself.
func assertEveryAssertionNamesItsRequest(t *testing.T, evs []streamEvent) {
	t.Helper()
	slugByID := map[string]string{}
	for _, ev := range evs {
		if ev.Kind == "request.start" {
			slugByID[ev.RequestID] = ev.RequestSlug
		}
	}
	seen := 0
	for _, ev := range evs {
		if ev.Kind != "assertion.result" {
			continue
		}
		seen++
		if ev.RequestSlug == "" {
			t.Errorf("assertion.result %q (request_id %q) carries no request_slug", ev.Label, ev.RequestID)
			continue
		}
		if want := slugByID[ev.RequestID]; want != "" && ev.RequestSlug != want {
			t.Errorf("assertion.result %q has request_slug %q; the request.start sharing request_id %q has %q",
				ev.Label, ev.RequestSlug, ev.RequestID, want)
		}
	}
	if seen == 0 {
		t.Fatal("stream contained no assertion.result events")
	}
}

// TestEvents_AssertionResultNamesItsRequest_Serial covers the plain sequential
// path, including a failing assertion — the event an agent reads first.
func TestEvents_AssertionResultNamesItsRequest_Serial(t *testing.T) {
	evs := runCollectionForEvents(t, `
name: provenance
requests:
  - name: fetch user
    request:
      method: GET
      url: "{{base_url}}/"
    assertions:
      status: 200
      body:
        $.user.name:
          equals: "WRONG"
  - name: check flag
    request:
      method: GET
      url: "{{base_url}}/"
    assertions:
      status: 200
`)
	assertEveryAssertionNamesItsRequest(t, evs)
}

// TestEvents_AssertionResultNamesItsRequest_Parallel covers the wave executor,
// which builds its own assertion events rather than sharing the serial path.
func TestEvents_AssertionResultNamesItsRequest_Parallel(t *testing.T) {
	evs := runCollectionForEvents(t, `
name: provenance-parallel
parallel:
  max_concurrent: 2
requests:
  - name: alpha
    request:
      method: GET
      url: "{{base_url}}/"
    assertions:
      status: 200
  - name: beta
    request:
      method: GET
      url: "{{base_url}}/"
    assertions:
      status: 201
`)
	assertEveryAssertionNamesItsRequest(t, evs)
}

// TestEvents_AssertionResultNamesItsRequest_DataDriven covers per-iteration
// requests, where the slug is derived per row rather than per collection item.
func TestEvents_AssertionResultNamesItsRequest_DataDriven(t *testing.T) {
	evs := runCollectionForEvents(t, `
name: provenance-data
requests:
  - name: lookup
    request:
      method: GET
      url: "{{base_url}}/"
    data:
      - case: "first"
      - case: "second"
    assertions:
      status: 200
`)
	assertEveryAssertionNamesItsRequest(t, evs)
}
