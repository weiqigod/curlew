package uiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/uiserver"
)

// TestGitInfo_BranchAndDetached exercises the .git/HEAD reader through the
// run-meta path: a fake .git directory with a symbolic ref, packed-refs
// fallback, and detached HEAD.
func TestGitInfo_RunMetaVariants(t *testing.T) {
	cases := []struct {
		name       string
		setup      func(t *testing.T, root string)
		wantBranch *string
		wantCommit *string
	}{
		{
			name: "symbolic ref with loose ref file",
			setup: func(t *testing.T, root string) {
				gd := filepath.Join(root, ".git")
				mustWrite(t, filepath.Join(gd, "HEAD"), "ref: refs/heads/main\n")
				mustWrite(t, filepath.Join(gd, "refs", "heads", "main"), strings.Repeat("a", 40)+"\n")
			},
			wantBranch: strp("main"),
			wantCommit: strp(strings.Repeat("a", 40)),
		},
		{
			name: "symbolic ref via packed-refs",
			setup: func(t *testing.T, root string) {
				gd := filepath.Join(root, ".git")
				mustWrite(t, filepath.Join(gd, "HEAD"), "ref: refs/heads/feature/x\n")
				mustWrite(t, filepath.Join(gd, "packed-refs"),
					"# pack-refs with: peeled fully-peeled sorted\n"+
						strings.Repeat("b", 40)+" refs/heads/feature/x\n")
			},
			wantBranch: strp("feature/x"),
			wantCommit: strp(strings.Repeat("b", 40)),
		},
		{
			name: "detached HEAD",
			setup: func(t *testing.T, root string) {
				mustWrite(t, filepath.Join(root, ".git", "HEAD"), strings.Repeat("c", 40)+"\n")
			},
			wantBranch: nil,
			wantCommit: strp(strings.Repeat("c", 40)),
		},
		{
			name:       "no git at all",
			setup:      func(t *testing.T, root string) {},
			wantBranch: nil,
			wantCommit: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, root := newTestServer(t, func(o *uiserver.Options) {
				o.Exec = fakeExec
				o.HistoryEnabled = true
			})
			tc.setup(t, root)
			writeFile(t, root, "collections/ok.yaml", "name: OK\nrequests:\n  - name: Fine\n    request: {method: GET, url: \"http://t.test/ok\"}\n")
			runID := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})
			final := waitTerminal(t, ts, runID)
			meta, _ := final["meta"].(map[string]any)
			git, hasGit := meta["git"].(map[string]any)
			if tc.wantBranch == nil && tc.wantCommit == nil {
				if hasGit && git != nil {
					t.Errorf("git = %v, want null", git)
				}
				return
			}
			if !hasGit {
				t.Fatalf("git block missing: %v", meta)
			}
			checkPtr(t, "branch", git["branch"], tc.wantBranch)
			checkPtr(t, "commit", git["commit"], tc.wantCommit)
		})
	}
}

func strp(s string) *string { return &s }

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checkPtr(t *testing.T, field string, got any, want *string) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("%s = %v, want null", field, got)
		}
		return
	}
	if got != *want {
		t.Errorf("%s = %v, want %s", field, got, *want)
	}
}

// TestRun_BinaryAndOversizeBodies covers the base64 inline path and the
// >256 KiB truncation path plus the /body raw endpoint for both.
func TestRun_BinaryAndOversizeBodies(t *testing.T) {
	bigBody := bytes.Repeat([]byte(`{"k":"v"},`), 40_000) // ~400 KB
	binBody := append([]byte{0x89, 'P', 'N', 'G', 0x00, 0x01}, bytes.Repeat([]byte{0xff, 0x00}, 64)...)
	exec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.HasSuffix(req.URL, "/big") {
			return &httpexec.Result{
				StatusCode: 200, Body: bigBody,
				Headers: http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}
		return &httpexec.Result{
			StatusCode: 200, Body: binBody,
			Headers: http.Header{"Content-Type": []string{"image/png"}},
		}, nil
	}
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = exec })
	writeFile(t, root, "collections/bodies.yaml", `name: Bodies
requests:
  - name: Big
    request: {method: GET, url: "http://t.test/big"}
  - name: Bin
    request: {method: GET, url: "http://t.test/bin"}
`)
	runID := startRun(t, ts, map[string]any{"collection": "collections/bodies.yaml"})
	waitTerminal(t, ts, runID)

	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			RequestID string `json:"request_id"`
			Slug      string `json:"slug"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	ids := map[string]string{}
	for _, e := range list.Requests {
		ids[e.Slug] = e.RequestID
	}

	// Big: inline omitted, truncated true, size real; /body streams it all.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+ids["big"])
	big := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	bb := big["response"].(map[string]any)["body"].(map[string]any)
	if bb["truncated"] != true || bb["size"] != float64(len(bigBody)) {
		t.Errorf("big body meta = %v", bb)
	}
	if c, ok := bb["content"].(string); ok && c != "" {
		t.Error("big body content inlined despite cap")
	}
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+ids["big"]+"/body?which=response")
	raw, _ := readAll(resp)
	if len(raw) != len(bigBody) {
		t.Errorf("raw big body = %d bytes, want %d", len(raw), len(bigBody))
	}

	// Bin: base64 inline with encoding marker; raw endpoint streams original bytes.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+ids["bin"])
	bin := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	nb := bin["response"].(map[string]any)["body"].(map[string]any)
	if nb["encoding"] != "base64" || nb["content_type"] != "image/png" {
		t.Errorf("bin body meta = %v", nb)
	}
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+ids["bin"]+"/body?which=response")
	rawBin, ct := readAll(resp)
	if !bytes.Equal(rawBin, binBody) {
		t.Error("raw binary mismatch")
	}
	if ct != "image/png" {
		t.Errorf("raw content type = %q", ct)
	}

	// Bad which param.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+ids["bin"]+"/body?which=wat")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("which=wat status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func readAll(resp *http.Response) ([]byte, string) {
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(resp.Body)
	return buf.Bytes(), resp.Header.Get("Content-Type")
}

// TestRun_SkipAndRetryDetail covers if:false skip reasons (verbatim), the
// retry attempts block on detail, and request-body capture.
func TestRun_SkipAndRetryDetail(t *testing.T) {
	calls := 0
	exec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.HasSuffix(req.URL, "/flaky") {
			calls++
			if calls < 3 {
				return &httpexec.Result{StatusCode: 503, Body: []byte("busy")}, nil
			}
		}
		return &httpexec.Result{
			StatusCode: 200, Body: []byte(`{"ok":true}`),
			Headers: http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}
	ts, root := newTestServer(t, func(o *uiserver.Options) {
		o.Exec = exec
		o.HistoryEnabled = false
	})
	writeFile(t, root, "collections/sr.yaml", `name: SR
requests:
  - name: Skipped one
    if: "false"
    request: {method: GET, url: "http://t.test/never"}
  - name: Flaky
    retry:
      enabled: true
      max_attempts: 3
      initial_delay_ms: 1
      jitter: false
      retry_on:
        status_codes: [503]
        methods: ["POST"]
    request:
      method: POST
      url: "http://t.test/flaky"
      body: {"hello": "world"}
`)
	runID := startRun(t, ts, map[string]any{"collection": "collections/sr.yaml"})
	final := waitTerminal(t, ts, runID)
	if final["exit_status"] != "passed" {
		t.Fatalf("exit = %v (%v)", final["exit_status"], final["state"])
	}

	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			RequestID  string  `json:"request_id"`
			Slug       string  `json:"slug"`
			Outcome    *string `json:"outcome"`
			SkipReason *string `json:"skip_reason"`
			RetryCount int     `json:"retry_count"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	bySlug := map[string]int{}
	for i, e := range list.Requests {
		bySlug[e.Slug] = i
	}
	skipped := list.Requests[bySlug["skipped-one"]]
	if skipped.Outcome == nil || *skipped.Outcome != "skipped" {
		t.Errorf("skipped outcome = %v", skipped.Outcome)
	}
	if skipped.SkipReason == nil || !strings.Contains(*skipped.SkipReason, "if:") {
		t.Errorf("skip_reason = %v, want verbatim if: reason", skipped.SkipReason)
	}
	flaky := list.Requests[bySlug["flaky"]]
	if flaky.RetryCount != 2 {
		t.Errorf("retry_count = %d, want 2", flaky.RetryCount)
	}

	// Detail: retry attempts block + request body captured.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+flaky.RequestID)
	detail := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	retryBlock, _ := detail["retry"].(map[string]any)
	if retryBlock == nil {
		t.Fatal("retry block missing")
	}
	attempts, _ := retryBlock["attempts"].([]any)
	if len(attempts) != 3 {
		t.Errorf("attempts = %d, want 3", len(attempts))
	}
	reqBlock, _ := detail["request"].(map[string]any)
	if reqBlock == nil || reqBlock["body"] == nil {
		t.Errorf("request body missing from detail: %v", reqBlock)
	}

	// Request-side raw body endpoint.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+flaky.RequestID+"/body?which=request")
	raw, _ := readAll(resp)
	if !strings.Contains(string(raw), "hello") {
		t.Errorf("request raw body = %q", raw)
	}

	// Skip detail panel data: skipped request detail is fetchable.
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+skipped.RequestID)
	sd := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if sd["skipped"] != true {
		t.Errorf("detail.skipped = %v", sd["skipped"])
	}
}

// TestHistory_PersistedRunServesAfterRestart covers the store read path:
// a persisted run is served through findRun (source=store), the events
// download, and compare after the ring is gone (new Server instance).
func TestHistory_PersistedRunServesAfterRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "apitest.yaml"), []byte("project_name: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkServer := func() *httptest.Server {
		srv, err := uiserver.NewServer(uiserver.Options{
			Root: root, ProjectName: "t", Version: "test", Token: testToken,
			HistoryEnabled: true, Exec: fakeExec,
		})
		if err != nil {
			t.Fatal(err)
		}
		ts := httptest.NewServer(srv.Handler())
		t.Cleanup(ts.Close)
		return ts
	}
	ts1 := mkServer()
	writeFile(t, root, "collections/ok.yaml", "name: OK\nrequests:\n  - name: Fine\n    request: {method: GET, url: \"http://t.test/ok\"}\n")
	runA := startRun(t, ts1, map[string]any{"collection": "collections/ok.yaml"})
	waitTerminal(t, ts1, runA)
	runB := startRun(t, ts1, map[string]any{"collection": "collections/ok.yaml"})
	waitTerminal(t, ts1, runB)

	// Fresh server: ring is empty; everything must come from the store.
	ts2 := mkServer()
	resp := apiGet(t, ts2, "/api/v1/runs/"+runA)
	got := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if got["source"] != "store" {
		t.Errorf("source = %v, want store", got["source"])
	}
	resp = apiGet(t, ts2, "/api/v1/runs/"+runA+"/requests")
	if resp.StatusCode != 200 {
		t.Errorf("persisted request list = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp = apiGet(t, ts2, "/api/v1/runs/"+runA+"/events")
	raw, ct := readAll(resp)
	if ct != "application/x-ndjson" || !bytes.Contains(raw, []byte("run.end")) {
		t.Errorf("persisted events: ct=%q len=%d", ct, len(raw))
	}
	resp = apiGet(t, ts2, "/api/v1/compare?base="+runA+"&target="+runB)
	if resp.StatusCode != 200 {
		t.Errorf("persisted compare = %d", resp.StatusCode)
	}
	var cmp struct {
		Pairs []json.RawMessage `json:"pairs"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&cmp)
	_ = resp.Body.Close()
	if len(cmp.Pairs) != 1 {
		t.Errorf("persisted compare pairs = %d, want 1", len(cmp.Pairs))
	}
}

// TestCollectionFilterRestrictsTreeAndRuns covers the --collection filter.
func TestCollectionFilterRestrictsTreeAndRuns(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) {
		o.Exec = fakeExec
		o.CollectionFilter = "collections/a.yaml"
	})
	writeFile(t, root, "collections/a.yaml", "name: A\nrequests:\n  - name: Fine\n    request: {method: GET, url: \"http://t.test/ok\"}\n")
	writeFile(t, root, "collections/b.yaml", "name: B\nrequests:\n  - name: Other\n    request: {method: GET, url: \"http://t.test/ok\"}\n")

	resp := apiGet(t, ts, "/api/v1/tree")
	tree := decodeJSON[struct {
		Collections []struct {
			Path string `json:"path"`
		} `json:"collections"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if len(tree.Collections) != 1 || tree.Collections[0].Path != "collections/a.yaml" {
		t.Errorf("filtered tree = %+v", tree.Collections)
	}

	// Batch run respects the filter (single valid collection → no gate).
	runID := startRun(t, ts, map[string]any{"collection": nil})
	final := waitTerminal(t, ts, runID)
	summary, _ := final["summary"].(map[string]any)
	if summary["total"] != float64(1) {
		t.Errorf("filtered batch total = %v, want 1", summary["total"])
	}
}
