package uiserver_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/uiserver"
)

const testToken = "0123456789abcdef0123456789abcdef"

// newTestServer builds a Server over a scaffolded project and returns the
// httptest server plus the project root.
func newTestServer(t *testing.T, opts ...func(*uiserver.Options)) (*httptest.Server, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "curlew.yaml"), []byte("project_name: Test Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "collections"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := uiserver.Options{
		Root:        root,
		ProjectName: "Test Project",
		Version:     "0.1.0-test",
		Token:       testToken,
	}
	for _, f := range opts {
		f(&o)
	}
	srv, err := uiserver.NewServer(o)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, root
}

// apiGet performs a GET with the session token and a loopback Host header.
func apiGet(t *testing.T, ts *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeJSON[T any](t *testing.T, r io.Reader) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func TestAPI_MissingToken_401(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/meta")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	body := decodeJSON[map[string]any](t, resp.Body)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "unauthorized" {
		t.Errorf("error.code = %v, want unauthorized", errObj["code"])
	}
}

func TestAPI_WrongToken_401(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAPI_XCurlewUITokenHeader_Accepted(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/meta", nil)
	req.Header.Set("X-Curlew-UI-Token", testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAPI_BadHostHeader_403(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Host = "evil.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	body := decodeJSON[map[string]any](t, resp.Body)
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "forbidden_origin" {
		t.Errorf("error.code = %v, want forbidden_origin", errObj["code"])
	}
}

func TestAPI_BadOrigin_403(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/meta", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAPI_UnknownPath_404JSON(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := apiGet(t, ts, "/api/v1/nope")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestMeta_Shape(t *testing.T) {
	ts, _ := newTestServer(t, func(o *uiserver.Options) {
		o.DefaultEnv = "dev"
	})
	resp := apiGet(t, ts, "/api/v1/meta")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	m := decodeJSON[map[string]any](t, resp.Body)

	server, _ := m["server"].(map[string]any)
	if server["api_version"] != float64(1) {
		t.Errorf("server.api_version = %v, want 1", server["api_version"])
	}
	if server["events_schema_version"] != "1.3" {
		t.Errorf("events_schema_version = %v, want 1.3", server["events_schema_version"])
	}
	project, _ := m["project"].(map[string]any)
	if project["name"] != "Test Project" {
		t.Errorf("project.name = %v", project["name"])
	}
	if project["default_env"] != "dev" {
		t.Errorf("project.default_env = %v, want dev", project["default_env"])
	}
	history, _ := m["history"].(map[string]any)
	if history["enabled"] != false {
		t.Errorf("history.enabled = %v, want false when history is off", history["enabled"])
	}
	limits, _ := m["limits"].(map[string]any)
	if limits["inline_body_bytes"] != float64(262144) {
		t.Errorf("limits.inline_body_bytes = %v", limits["inline_body_bytes"])
	}
	if limits["event_body_bytes"] != float64(2048) {
		t.Errorf("limits.event_body_bytes = %v", limits["event_body_bytes"])
	}
	if limits["memory_runs"] != float64(5) {
		t.Errorf("limits.memory_runs = %v", limits["memory_runs"])
	}
}

func TestMeta_HistoryEnabled(t *testing.T) {
	ts, _ := newTestServer(t, func(o *uiserver.Options) {
		o.HistoryEnabled = true
		o.MaxRuns = 50
	})
	resp := apiGet(t, ts, "/api/v1/meta")
	defer func() { _ = resp.Body.Close() }()
	m := decodeJSON[map[string]any](t, resp.Body)
	history, _ := m["history"].(map[string]any)
	if history["enabled"] != true {
		t.Errorf("history.enabled = %v, want true", history["enabled"])
	}
	if history["max_runs"] != float64(50) {
		t.Errorf("history.max_runs = %v, want 50", history["max_runs"])
	}
}

func TestAssets_ServedWithoutToken(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	// The committed placeholder serves until a real UI build overwrites it.
	if !strings.Contains(string(body), "curlew") {
		t.Errorf("placeholder index.html not served: %q", string(body)[:min(len(body), 120)])
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control = %q, want no-cache", cc)
	}
}

func TestAssets_SPAFallback(t *testing.T) {
	ts, _ := newTestServer(t)
	resp, err := http.Get(ts.URL + "/some/deep/path")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (SPA fallback)", resp.StatusCode)
	}
}

func TestPOST_NonJSONContentType_400(t *testing.T) {
	ts, _ := newTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/runs", strings.NewReader("a=b"))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
