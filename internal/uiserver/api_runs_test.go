package uiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/uiserver"
)

// fakeExec returns canned responses; /fail fails its status assertion
// upstream, /err returns a transport error.
func fakeExec(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
	switch {
	case strings.HasSuffix(req.URL, "/err"):
		return nil, context.DeadlineExceeded
	case strings.HasSuffix(req.URL, "/fail"):
		return &httpexec.Result{
			StatusCode: 500, Body: []byte(`{"oops":true}`),
			Headers: http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	default:
		return &httpexec.Result{
			StatusCode: 200, Body: []byte(`{"ok":true,"secret":"tok-12345"}`),
			Headers: http.Header{"Content-Type": []string{"application/json"}},
			Timing:  &httpexec.Timing{TTFB: 2 * time.Millisecond, Total: 3 * time.Millisecond},
		}, nil
	}
}

const runnableCollection = `name: Runnable
requests:
  - name: Get ok
    request: {method: GET, url: "http://t.test/ok"}
    assertions:
      status: 200
  - name: Get fail
    request: {method: GET, url: "http://t.test/fail"}
    assertions:
      status: 200
  - name: Get err
    request: {method: GET, url: "http://t.test/err"}
`

// apiPost issues an authorized JSON POST.
func apiPost(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytes.NewReader(data)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+path, rd)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// startRun posts /runs and returns the run id (expects 202).
func startRun(t *testing.T, ts *httptest.Server, params map[string]any) string {
	t.Helper()
	resp := apiPost(t, ts, "/api/v1/runs", params)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted {
		var raw map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&raw)
		t.Fatalf("start run: status = %d, body = %v", resp.StatusCode, raw)
	}
	out := decodeJSON[struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	}](t, resp.Body)
	if out.RunID == "" || out.State != "running" {
		t.Fatalf("start response = %+v", out)
	}
	return out.RunID
}

// waitTerminal polls GET /runs/{id} until the run leaves running/cancelling.
func waitTerminal(t *testing.T, ts *httptest.Server, runID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp := apiGet(t, ts, "/api/v1/runs/"+runID)
		m := decodeJSON[map[string]any](t, resp.Body)
		_ = resp.Body.Close()
		state, _ := m["state"].(string)
		if state != "running" && state != "cancelling" && state != "" {
			return m
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not reach a terminal state")
	return nil
}

func TestRun_FullLifecycle(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)

	runID := startRun(t, ts, map[string]any{"collection": "collections/run.yaml", "mode": "all"})
	final := waitTerminal(t, ts, runID)

	if final["state"] != "completed" || final["exit_status"] != "failed" {
		t.Errorf("state=%v exit_status=%v, want completed/failed", final["state"], final["exit_status"])
	}
	summary, _ := final["summary"].(map[string]any)
	if summary["total"] != float64(3) || summary["passed"] != float64(1) ||
		summary["failed"] != float64(1) || summary["error"] != float64(1) {
		t.Errorf("summary = %v", summary)
	}
	if final["source"] != "memory" {
		t.Errorf("source = %v, want memory (ring)", final["source"])
	}

	// Light list: outcomes, fail_message, compact error.
	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			RequestID  string  `json:"request_id"`
			Slug       string  `json:"slug"`
			Outcome    *string `json:"outcome"`
			FailMsg    *string `json:"fail_message"`
			Error      *struct{ Category, Message string }
			SourceFile string `json:"source_file"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if len(list.Requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(list.Requests))
	}
	byName := map[string]int{}
	for i, e := range list.Requests {
		byName[e.Slug] = i
	}
	failEntry := list.Requests[byName["get-fail"]]
	if failEntry.Outcome == nil || *failEntry.Outcome != "failed" {
		t.Errorf("get-fail outcome = %v", failEntry.Outcome)
	}
	if failEntry.FailMsg == nil || !strings.Contains(*failEntry.FailMsg, "expected") {
		t.Errorf("fail_message = %v", failEntry.FailMsg)
	}
	errEntry := list.Requests[byName["get-err"]]
	if errEntry.Outcome == nil || *errEntry.Outcome != "error" || errEntry.Error == nil {
		t.Errorf("get-err entry = %+v", errEntry)
	}
	if failEntry.SourceFile != "collections/run.yaml" {
		t.Errorf("source_file = %q", failEntry.SourceFile)
	}

	// Full detail for the passed request: bodies, headers, timing, snippet.
	okEntry := list.Requests[byName["get-ok"]]
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+okEntry.RequestID)
	detail := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if detail["outcome"] != "passed" || detail["status_code"] != float64(200) {
		t.Errorf("detail outcome/status = %v/%v", detail["outcome"], detail["status_code"])
	}
	respBlock, _ := detail["response"].(map[string]any)
	if respBlock == nil {
		t.Fatal("response block missing")
	}
	body, _ := respBlock["body"].(map[string]any)
	if c, _ := body["content"].(string); !strings.Contains(c, `"ok":true`) {
		t.Errorf("response body content = %v", body)
	}
	timing, _ := detail["timing"].(map[string]any)
	if timing == nil || timing["total_us"] != float64(3000) {
		t.Errorf("timing = %v", timing)
	}
	source, _ := detail["source"].(map[string]any)
	if source["file"] != "collections/run.yaml" || source["line"] == float64(0) {
		t.Errorf("source = %v", source)
	}
	if snippet, _ := source["snippet"].([]any); len(snippet) == 0 {
		t.Errorf("snippet empty: %v", source)
	}

	// Raw body endpoint.
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/runs/"+runID+"/requests/"+okEntry.RequestID+"/body?which=response", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	bresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bresp.Body.Close() }()
	if bresp.StatusCode != 200 {
		t.Fatalf("body endpoint status = %d", bresp.StatusCode)
	}
	if cd := bresp.Header.Get("Content-Disposition"); !strings.Contains(cd, "get-ok-response") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	// NDJSON events endpoint (free for ring runs).
	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/events")
	evBody := new(bytes.Buffer)
	_, _ = evBody.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(evBody.String(), `"kind":"run.start"`) || !strings.Contains(evBody.String(), `"kind":"run.end"`) {
		t.Errorf("events stream incomplete:\n%s", evBody.String())
	}
}

func TestRun_MidRunSeedingAndSingleFlight(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	blockingExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if calls.Add(1) == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte("{}")}, nil
	}
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = blockingExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)

	runID := startRun(t, ts, map[string]any{"collection": "collections/run.yaml"})

	// Planned requests appear immediately with outcome null.
	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			Outcome *string `json:"outcome"`
			Phase   string  `json:"phase"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if len(list.Requests) != 3 {
		t.Fatalf("planned requests = %d, want 3", len(list.Requests))
	}
	pendingSeen := false
	for _, e := range list.Requests {
		if e.Outcome == nil {
			pendingSeen = true
		}
	}
	if !pendingSeen {
		t.Error("no pending (outcome null) rows mid-run")
	}

	// /runs/current reflects the active run.
	resp = apiGet(t, ts, "/api/v1/runs/current")
	current := decodeJSON[struct {
		Run *struct {
			RunID string `json:"run_id"`
			State string `json:"state"`
		} `json:"run"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if current.Run == nil || current.Run.RunID != runID || current.Run.State != "running" {
		t.Errorf("current = %+v", current.Run)
	}

	// Second start → 409 run_active with the active id in details.
	resp409 := apiPost(t, ts, "/api/v1/runs", map[string]any{"collection": "collections/run.yaml"})
	if resp409.StatusCode != http.StatusConflict {
		t.Errorf("second start status = %d, want 409", resp409.StatusCode)
	}
	envlp := decodeJSON[map[string]map[string]any](t, resp409.Body)
	_ = resp409.Body.Close()
	if details, _ := envlp["error"]["details"].(map[string]any); details["run_id"] != runID {
		t.Errorf("409 details = %v", envlp["error"])
	}

	close(release)
	waitTerminal(t, ts, runID)

	// After completion, /runs/current is null.
	resp = apiGet(t, ts, "/api/v1/runs/current")
	cur2 := decodeJSON[map[string]any](t, resp.Body)
	_ = resp.Body.Close()
	if cur2["run"] != nil {
		t.Errorf("current after completion = %v", cur2["run"])
	}
}

func TestRun_Cancel(t *testing.T) {
	release := make(chan struct{})
	blockingExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		select {
		case <-release:
			return &httpexec.Result{StatusCode: 200, Body: []byte("{}")}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = blockingExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)
	defer close(release)

	runID := startRun(t, ts, map[string]any{"collection": "collections/run.yaml"})
	resp := apiPost(t, ts, "/api/v1/runs/"+runID+"/cancel", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("cancel status = %d", resp.StatusCode)
	}
	out := decodeJSON[map[string]string](t, resp.Body)
	_ = resp.Body.Close()
	if out["state"] != "cancelling" {
		t.Errorf("cancel state = %v", out)
	}
	final := waitTerminal(t, ts, runID)
	if final["state"] != "cancelled" || final["exit_status"] != "cancelled" {
		t.Errorf("final = state %v exit %v", final["state"], final["exit_status"])
	}

	// Cancel of a non-active run → 404.
	resp404 := apiPost(t, ts, "/api/v1/runs/"+runID+"/cancel", nil)
	if resp404.StatusCode != http.StatusNotFound {
		t.Errorf("cancel completed run = %d, want 404", resp404.StatusCode)
	}
	_ = resp404.Body.Close()
}

func TestRun_StartValidationErrors(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)
	writeFile(t, root, "collections/broken.yaml", "name: [oops\n")

	cases := []struct {
		name       string
		params     map[string]any
		wantStatus int
		wantCode   string
	}{
		{"invalid collection", map[string]any{"collection": "collections/broken.yaml"}, 422, "collection_invalid"},
		{"unknown env", map[string]any{"collection": "collections/run.yaml", "env": "nope"}, 422, "env_not_found"},
		{"escape root", map[string]any{"collection": "../../etc/passwd"}, 400, "bad_request"},
		{"selection without collection", map[string]any{"mode": "selection", "selection": []string{"Get ok"}}, 400, "bad_request"},
		{"selection empty", map[string]any{"collection": "collections/run.yaml", "mode": "selection"}, 400, "bad_request"},
		{"rerun_of missing", map[string]any{"mode": "rerun_failed"}, 400, "bad_request"},
		{"unknown mode", map[string]any{"collection": "collections/run.yaml", "mode": "wat"}, 400, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := apiPost(t, ts, "/api/v1/runs", tc.params)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.wantStatus)
			}
			envlp := decodeJSON[map[string]map[string]any](t, resp.Body)
			if envlp["error"]["code"] != tc.wantCode {
				t.Errorf("code = %v, want %s", envlp["error"]["code"], tc.wantCode)
			}
		})
	}
}

func TestRun_EnvNotFoundMapping(t *testing.T) {
	// env_not_found must map config.ErrEnvironmentNotFound to 422 even though
	// it surfaces from inside the run goroutine's pre-flight... it is checked
	// synchronously at start: the orchestrator validates by running Execute,
	// which fails fast. Covered by TestRun_StartValidationErrors; this test
	// asserts an existing env passes.
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)
	writeFile(t, root, "environments/dev.yaml", "variables:\n  base_url: http://t.test\n")
	runID := startRun(t, ts, map[string]any{"collection": "collections/run.yaml", "env": "dev"})
	final := waitTerminal(t, ts, runID)
	if final["state"] != "completed" {
		t.Errorf("state = %v", final["state"])
	}
}

func TestRun_SelectionMode(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)

	runID := startRun(t, ts, map[string]any{
		"collection": "collections/run.yaml", "mode": "selection", "selection": []string{"Get ok"},
	})
	final := waitTerminal(t, ts, runID)
	summary, _ := final["summary"].(map[string]any)
	if summary["total"] != float64(1) || summary["passed"] != float64(1) {
		t.Errorf("selection summary = %v", summary)
	}
}

func TestRun_RerunFailed(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/run.yaml", runnableCollection)

	first := startRun(t, ts, map[string]any{"collection": "collections/run.yaml"})
	waitTerminal(t, ts, first)

	second := startRun(t, ts, map[string]any{"mode": "rerun_failed", "rerun_of": first})
	final := waitTerminal(t, ts, second)
	summary, _ := final["summary"].(map[string]any)
	// Reruns the failed + errored requests only.
	if summary["total"] != float64(2) {
		t.Errorf("rerun total = %v, want 2", summary["total"])
	}

	// rerun_of with no failures → 400.
	okOnly := `name: OK
requests:
  - name: Fine
    request: {method: GET, url: "http://t.test/ok"}
`
	writeFile(t, root, "collections/okonly.yaml", okOnly)
	third := startRun(t, ts, map[string]any{"collection": "collections/okonly.yaml"})
	waitTerminal(t, ts, third)
	resp := apiPost(t, ts, "/api/v1/runs", map[string]any{"mode": "rerun_failed", "rerun_of": third})
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("rerun of clean run = %d, want 400", resp.StatusCode)
	}
}

func TestRun_BatchSingleCollection(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/a.yaml", runnableCollection)
	runID := startRun(t, ts, map[string]any{"collection": nil})
	final := waitTerminal(t, ts, runID)
	if final["state"] != "completed" {
		t.Errorf("state = %v", final["state"])
	}
}

func TestRun_BatchMultipleCollections(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/a.yaml", runnableCollection)
	writeFile(t, root, "collections/b.yaml", strings.Replace(runnableCollection, "Runnable", "Other", 1))
	writeFile(t, root, "collections/broken.yaml", "name: [oops\n") // excluded from batch

	runID := startRun(t, ts, map[string]any{"collection": nil})
	final := waitTerminal(t, ts, runID)
	summary, _ := final["summary"].(map[string]any)
	if summary["total"] != float64(6) {
		t.Errorf("batch total = %v, want 6", summary["total"])
	}
	// Wave fields omitted for batch runs.
	if _, present := summary["wave_count"]; present {
		t.Error("wave_count present on batch summary")
	}
	// meta has null collection_file.
	meta, _ := final["meta"].(map[string]any)
	if meta["collection_file"] != nil {
		t.Errorf("batch collection_file = %v, want null", meta["collection_file"])
	}

	// Request ids carry per-collection prefixes; entries group by source_file.
	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			RequestID  string `json:"request_id"`
			SourceFile string `json:"source_file"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if len(list.Requests) != 6 {
		t.Fatalf("batch requests = %d, want 6", len(list.Requests))
	}
	files := map[string]bool{}
	prefixes := map[string]bool{}
	for _, e := range list.Requests {
		files[e.SourceFile] = true
		if i := strings.Index(e.RequestID, "req-"); i > 0 {
			prefixes[e.RequestID[:i]] = true
		}
	}
	if len(files) != 2 {
		t.Errorf("source files = %v, want 2", files)
	}
	if !prefixes["c1-"] || !prefixes["c2-"] {
		t.Errorf("request id prefixes = %v, want c1-/c2-", prefixes)
	}
}

func TestRun_RedactionInDetailAndEvents(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/sec.yaml", `name: Sec
variables:
  api_secret:
    value: "tok-12345"
    sensitive: true
requests:
  - name: Leaky
    request:
      method: GET
      url: "http://t.test/ok"
      headers:
        Authorization: "Bearer {{api_secret}}"
`)
	runID := startRun(t, ts, map[string]any{"collection": "collections/sec.yaml"})
	waitTerminal(t, ts, runID)

	resp := apiGet(t, ts, "/api/v1/runs/"+runID+"/requests")
	list := decodeJSON[struct {
		Requests []struct {
			RequestID string `json:"request_id"`
		} `json:"requests"`
	}](t, resp.Body)
	_ = resp.Body.Close()

	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/requests/"+list.Requests[0].RequestID)
	raw := new(bytes.Buffer)
	_, _ = raw.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if strings.Contains(raw.String(), "tok-12345") {
		t.Errorf("sensitive value leaked in detail:\n%s", raw.String())
	}
	if !strings.Contains(raw.String(), "[REDACTED]") {
		t.Errorf("redaction marker missing in detail:\n%s", raw.String())
	}

	resp = apiGet(t, ts, "/api/v1/runs/"+runID+"/events")
	evRaw := new(bytes.Buffer)
	_, _ = evRaw.ReadFrom(resp.Body)
	_ = resp.Body.Close()
	if strings.Contains(evRaw.String(), "tok-12345") {
		t.Errorf("sensitive value leaked in events:\n%s", evRaw.String())
	}
}

func TestRing_EvictsBeyondFive(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/ok.yaml", `name: OK
requests:
  - name: Fine
    request: {method: GET, url: "http://t.test/ok"}
`)
	var ids []string
	for range 6 {
		id := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})
		waitTerminal(t, ts, id)
		ids = append(ids, id)
	}
	// Oldest run fell out of the ring (history disabled: no store).
	resp := apiGet(t, ts, "/api/v1/runs/"+ids[0])
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("evicted run status = %d, want 404", resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp = apiGet(t, ts, "/api/v1/runs/"+ids[5])
	if resp.StatusCode != http.StatusOK {
		t.Errorf("recent run status = %d, want 200", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestHistory_DisabledNothingWritten(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) { o.Exec = fakeExec })
	writeFile(t, root, "collections/ok.yaml", `name: OK
requests:
  - name: Fine
    request: {method: GET, url: "http://t.test/ok"}
`)
	runID := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})
	waitTerminal(t, ts, runID)

	// List is empty when history is disabled.
	resp := apiGet(t, ts, "/api/v1/runs")
	listOut := decodeJSON[struct {
		Runs  []map[string]any `json:"runs"`
		Total int              `json:"total"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if listOut.Total != 0 || len(listOut.Runs) != 0 {
		t.Errorf("history list = %d/%d, want empty", len(listOut.Runs), listOut.Total)
	}

	if _, err := os.Stat(filepath.Join(root, ".curlew", "ui")); !os.IsNotExist(err) {
		t.Error(".curlew/ui exists with history disabled — nothing must ever be written")
	}
}

func TestHistory_PersistListDeleteCompare(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) {
		o.Exec = fakeExec
		o.HistoryEnabled = true
	})
	writeFile(t, root, "collections/run.yaml", runnableCollection)

	first := startRun(t, ts, map[string]any{"collection": "collections/run.yaml"})
	waitTerminal(t, ts, first)
	second := startRun(t, ts, map[string]any{"collection": "collections/run.yaml"})
	waitTerminal(t, ts, second)

	// Store layout + self-ignoring gitignore.
	if data, err := os.ReadFile(filepath.Join(root, ".curlew", "ui", ".gitignore")); err != nil || string(data) != "*\n" {
		t.Errorf("store .gitignore = %q, %v", data, err)
	}
	for _, id := range []string{first, second} {
		for _, f := range []string{"meta.json", "events.ndjson", "detail.json"} {
			if _, err := os.Stat(filepath.Join(root, ".curlew", "ui", "runs", id, f)); err != nil {
				t.Errorf("missing %s for %s: %v", f, id, err)
			}
		}
	}

	// List newest first.
	resp := apiGet(t, ts, "/api/v1/runs")
	listOut := decodeJSON[struct {
		Runs  []map[string]any `json:"runs"`
		Total int              `json:"total"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if listOut.Total != 2 || len(listOut.Runs) != 2 {
		t.Fatalf("history list = %d/%d, want 2/2", len(listOut.Runs), listOut.Total)
	}
	if listOut.Runs[0]["run_id"] != second {
		t.Errorf("newest first violated: %v", listOut.Runs[0]["run_id"])
	}

	// Compare aligns by slug; identical runs change nothing.
	resp = apiGet(t, ts, "/api/v1/compare?base="+first+"&target="+second)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("compare status = %d", resp.StatusCode)
	}
	cmp := decodeJSON[struct {
		Pairs []struct {
			Slug  string `json:"slug"`
			Delta struct {
				OutcomeChanged bool `json:"outcome_changed"`
				StatusChanged  bool `json:"status_changed"`
			} `json:"delta"`
		} `json:"pairs"`
		OnlyInBase   []any `json:"only_in_base"`
		OnlyInTarget []any `json:"only_in_target"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if len(cmp.Pairs) != 3 || len(cmp.OnlyInBase) != 0 || len(cmp.OnlyInTarget) != 0 {
		t.Fatalf("compare pairs = %d only_base %d only_target %d", len(cmp.Pairs), len(cmp.OnlyInBase), len(cmp.OnlyInTarget))
	}
	for _, p := range cmp.Pairs {
		if p.Delta.OutcomeChanged || p.Delta.StatusChanged {
			t.Errorf("pair %s reports changes between identical runs", p.Slug)
		}
	}

	// Delete a persisted run.
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/runs/"+first, nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	dresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = dresp.Body.Close()
	if dresp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", dresp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(root, ".curlew", "ui", "runs", first)); !os.IsNotExist(err) {
		t.Error("deleted run directory still exists")
	}
}

func TestOpen_NoEditor409AndLaunch(t *testing.T) {
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	if err := listener.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CURLEW_TEST_EDITOR_ADDRESS", listener.Addr().String())
	ts, root := newTestServer(t, func(o *uiserver.Options) {
		o.EditorCommand = `"` + os.Args[0] + `" -test.run=^TestOpenEditorHelper$ -- "{file}:{line}"`
	})
	writeFile(t, root, "collections/c.yaml", validCollection)

	resp := apiPost(t, ts, "/api/v1/open", map[string]any{"file": "collections/c.yaml", "line": 3})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("open = %d, want 204", resp.StatusCode)
	}
	connection, err := listener.AcceptTCP()
	if err != nil {
		t.Fatalf("editor helper did not release its working directory: %v", err)
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var location string
	if err := json.NewDecoder(connection).Decode(&location); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "collections", "c.yaml") + ":3"; location != want {
		t.Fatalf("editor location = %q, want %q", location, want)
	}

	// Outside root → 400.
	resp = apiPost(t, ts, "/api/v1/open", map[string]any{"file": "../etc/passwd", "line": 1})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("open escape = %d, want 400", resp.StatusCode)
	}
}

func TestOpenEditorHelper(t *testing.T) {
	address := os.Getenv("CURLEW_TEST_EDITOR_ADDRESS")
	if address == "" {
		return
	}
	if err := os.Chdir(os.TempDir()); err != nil {
		t.Fatal(err)
	}
	connection, err := net.DialTimeout("tcp4", address, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	if err := json.NewEncoder(connection).Encode(os.Args[len(os.Args)-1]); err != nil {
		t.Fatal(err)
	}
}
