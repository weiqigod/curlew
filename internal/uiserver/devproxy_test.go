package uiserver_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/peterlindqvist/apitest/internal/uiserver"
)

// TestDevProxy_ForwardsNonAPIPaths covers APITEST_UI_DEV_PROXY mode: non-API
// paths reverse-proxy to the Vite dev server while /api stays local.
func TestDevProxy_ForwardsNonAPIPaths(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("vite:" + r.URL.Path))
	}))
	defer vite.Close()

	ts, _ := newTestServer(t, func(o *uiserver.Options) { o.DevProxy = vite.URL })

	resp, err := http.Get(ts.URL + "/some/spa/route")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) != "vite:/some/spa/route" {
		t.Errorf("proxied body = %q", body)
	}

	// API still served locally (and still token-guarded).
	resp = apiGet(t, ts, "/api/v1/meta")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("meta via dev-proxy server = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

// TestRunsList_Pagination covers limit/offset on the history list.
func TestRunsList_Pagination(t *testing.T) {
	ts, root := newTestServer(t, func(o *uiserver.Options) {
		o.Exec = fakeExec
		o.HistoryEnabled = true
	})
	writeFile(t, root, "collections/ok.yaml", "name: OK\nrequests:\n  - name: Fine\n    request: {method: GET, url: \"http://t.test/ok\"}\n")
	var ids []string
	for range 3 {
		id := startRun(t, ts, map[string]any{"collection": "collections/ok.yaml"})
		waitTerminal(t, ts, id)
		ids = append(ids, id)
	}
	resp := apiGet(t, ts, "/api/v1/runs?limit=1&offset=1")
	out := decodeJSON[struct {
		Runs []struct {
			RunID string `json:"run_id"`
		} `json:"runs"`
		Total int `json:"total"`
	}](t, resp.Body)
	_ = resp.Body.Close()
	if out.Total != 3 || len(out.Runs) != 1 {
		t.Fatalf("paginated = %d/%d, want 1/3", len(out.Runs), out.Total)
	}
	if out.Runs[0].RunID != ids[1] {
		t.Errorf("offset=1 run = %s, want middle run %s", out.Runs[0].RunID, ids[1])
	}
}
