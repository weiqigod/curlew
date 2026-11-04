package runner

import (
	"context"
	"sync"
	"testing"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
)

// headerRecorder captures the headers of every request sent, keyed by URL.
type headerRecorder struct {
	mu   sync.Mutex
	sent map[string]map[string]string
}

func newHeaderRecorder() *headerRecorder {
	return &headerRecorder{sent: make(map[string]map[string]string)}
}

func (h *headerRecorder) exec(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	headers := make(map[string]string, len(req.Headers))
	for k, v := range req.Headers {
		headers[k] = v
	}
	h.sent[req.URL] = headers
	return &httpexec.Result{StatusCode: 200}, nil
}

func (h *headerRecorder) headers(url string) map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sent[url]
}

func TestRun_EnvironmentHeadersReachEveryPhase(t *testing.T) {
	col := makeCollectionWithPhases([]string{"setup"}, []string{"main"}, []string{"teardown"})
	rec := newHeaderRecorder()

	_, _, err := Run(context.Background(), col, rec.exec, VarSources{
		EnvFile:            map[string]string{"run_id": "demo-1"},
		EnvironmentHeaders: map[string]string{"X-Mock-Run": "{{run_id}}", "X-Mock-Api": "quiddly"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{"setup", "main", "teardown"} {
		got := rec.headers("https://example.com/" + name)
		if got["X-Mock-Run"] != "demo-1" {
			t.Errorf("%s: X-Mock-Run = %q, want demo-1 (templated from the variable)", name, got["X-Mock-Run"])
		}
		if got["X-Mock-Api"] != "quiddly" {
			t.Errorf("%s: X-Mock-Api = %q, want quiddly", name, got["X-Mock-Api"])
		}
	}
}

func TestRun_RequestHeaderWinsOverEnvironmentHeader(t *testing.T) {
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{{
			Name: "own",
			Request: parser.Request{
				Method:  "GET",
				URL:     "https://example.com/own",
				Headers: map[string]string{"x-mock-api": "credit"},
			},
		}}},
	}
	rec := newHeaderRecorder()

	_, _, err := Run(context.Background(), col, rec.exec, VarSources{
		EnvironmentHeaders: map[string]string{"X-Mock-Api": "quiddly", "X-Mock-Run": "demo-1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := rec.headers("https://example.com/own")
	if got["x-mock-api"] != "credit" {
		t.Errorf("x-mock-api = %q, want the request's own value credit", got["x-mock-api"])
	}
	if v, ok := got["X-Mock-Api"]; ok {
		t.Errorf("environment header sent alongside the request's own header (case-insensitive name): %q", v)
	}
	if got["X-Mock-Run"] != "demo-1" {
		t.Errorf("X-Mock-Run = %q, want demo-1", got["X-Mock-Run"])
	}
}

func TestRun_EnvironmentHeadersLeaveTheCollectionUnchanged(t *testing.T) {
	col := makeCollectionWithPhases([]string{"setup"}, []string{"main"}, nil)
	col.Requests.Items[0].Request.Headers = map[string]string{"Accept": "application/json"}

	_, _, err := Run(context.Background(), col, successExecutor, VarSources{
		EnvironmentHeaders: map[string]string{"X-Mock-Run": "demo-1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if col.Setup.Items[0].Request.Headers != nil {
		t.Errorf("setup headers = %v, want the collection left as loaded", col.Setup.Items[0].Request.Headers)
	}
	if got := col.Requests.Items[0].Request.Headers; len(got) != 1 || got["Accept"] != "application/json" {
		t.Errorf("main headers = %v, want only the collection's own Accept header", got)
	}
}

func TestRun_EnvironmentHeadersInParallelRuns(t *testing.T) {
	col := makeCollectionWithPhases(nil, []string{"a", "b", "c"}, nil)
	rec := newHeaderRecorder()

	_, _, err := Run(context.Background(), col, rec.exec, VarSources{
		Parallel:           true,
		EnvironmentHeaders: map[string]string{"X-Mock-Run": "demo-1"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, name := range []string{"a", "b", "c"} {
		if got := rec.headers("https://example.com/" + name)["X-Mock-Run"]; got != "demo-1" {
			t.Errorf("%s: X-Mock-Run = %q, want demo-1", name, got)
		}
	}
}

func TestRun_NoEnvironmentHeadersSendsNone(t *testing.T) {
	col := makeCollectionWithPhases(nil, []string{"main"}, nil)
	rec := newHeaderRecorder()

	_, _, err := Run(context.Background(), col, rec.exec, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := rec.headers("https://example.com/main"); len(got) != 0 {
		t.Errorf("headers = %v, want none", got)
	}
}
