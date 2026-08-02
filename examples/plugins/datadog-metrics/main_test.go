package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errReader is an io.Reader that returns an error after the initial data.
type errReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos < len(r.data) {
		n := copy(p, r.data[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, r.err
}

func TestPrintMetadata_StandaloneHelp(t *testing.T) {
	var buf bytes.Buffer
	printMetadata(&buf)
	got := buf.String()
	for _, want := range []string{"datadog-metrics", "0.1.0", "on_response", "on_result"} {
		if !strings.Contains(got, want) {
			t.Errorf("metadata missing %q:\n%s", want, got)
		}
	}
}

func TestHandshake_ReturnsHello(t *testing.T) {
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n")
	var out, errw bytes.Buffer
	cfg := config{} // DATADOG_API_KEY unset
	if err := run(context.Background(), cfg, in, &out, &errw); err != nil && err != io.EOF {
		t.Fatalf("run: %v", err)
	}
	var resp response
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	result := resp.Result.(map[string]any)
	if result["name"] != "datadog-metrics" {
		t.Errorf("name: %v", result["name"])
	}
	hooks, _ := result["hooks"].([]any)
	wantHooks := map[string]bool{"on_response": true, "on_result": true}
	for _, h := range hooks {
		delete(wantHooks, h.(string))
	}
	if len(wantHooks) != 0 {
		t.Errorf("missing hooks: %v", wantHooks)
	}
}

func TestOnResponse_SubmitsMetric(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(202)
	}))
	defer srv.Close()

	cfg := config{enabled: true, apiKey: "k", apiURL: srv.URL}
	var errw bytes.Buffer
	params := json.RawMessage(`{"status_code":200,"duration_ms":142}`)
	_, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", params)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if !strings.Contains(errw.String(), "submitted 1 metric") {
		t.Errorf("stderr missing success line: %q", errw.String())
	}
	var payload ddSeries
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Series[0].Points[0].Value != 142 {
		t.Errorf("value: %v", payload.Series[0].Points[0].Value)
	}
}

func TestOnResponse_Disabled_DoesNotSubmit(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hit = true }))
	defer srv.Close()
	cfg := config{enabled: false, apiURL: srv.URL} // no API key
	var errw bytes.Buffer
	_, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", json.RawMessage(`{"status_code":200}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if hit {
		t.Error("expected no HTTP call when disabled")
	}
	if strings.Contains(errw.String(), "submitted") {
		t.Errorf("unexpected submission log: %q", errw.String())
	}
}

func TestOnResponse_NonFatalOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	cfg := config{enabled: true, apiKey: "k", apiURL: srv.URL}
	var errw bytes.Buffer
	result, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", json.RawMessage(`{"status_code":200,"duration_ms":5}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result == nil {
		t.Error("expected identity response, got nil")
	}
	if !strings.Contains(errw.String(), "submit failed") {
		t.Errorf("expected warning log, got %q", errw.String())
	}
}

func TestLoadConfig_MissingKey_Disabled(t *testing.T) {
	cfg := loadConfig([]string{}) // empty env
	if cfg.enabled {
		t.Error("expected disabled when DATADOG_API_KEY missing")
	}
	if cfg.apiURL != "https://api.datadoghq.com" {
		t.Errorf("default url: %q", cfg.apiURL)
	}
}

func TestLoadConfig_WhitespaceKey_Disabled(t *testing.T) {
	cfg := loadConfig([]string{"DATADOG_API_KEY=   "})
	if cfg.enabled {
		t.Error("expected disabled when DATADOG_API_KEY is whitespace-only")
	}
}

func TestLoadConfig_DDSite(t *testing.T) {
	cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_SITE=datadoghq.eu"})
	if cfg.apiURL != "https://api.datadoghq.eu" {
		t.Errorf("site: %q", cfg.apiURL)
	}
	if !cfg.enabled {
		t.Error("expected enabled")
	}
}

func TestLoadConfig_DDAPIURLOverride(t *testing.T) {
	cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_SITE=datadoghq.com", "DD_API_URL=http://localhost:9999/"})
	if cfg.apiURL != "http://localhost:9999" {
		t.Errorf("override: %q", cfg.apiURL)
	}
}

func TestRun_MissingKey_LogsDisabledLine(t *testing.T) {
	in := bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n")
	var out, errw bytes.Buffer
	_ = run(context.Background(), loadConfig([]string{}), in, &out, &errw)
	if !strings.Contains(errw.String(), "DATADOG_API_KEY not set, disabled") {
		t.Errorf("disabled line missing: %q", errw.String())
	}
}

func TestOnResponse_MalformedParams_LogsWarning(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(202)
	}))
	defer srv.Close()
	cfg := config{enabled: true, apiKey: "k", apiURL: srv.URL}
	var errw bytes.Buffer
	// params is not valid JSON
	_, err := handle(context.Background(), cfg, http.DefaultClient, &errw, "apitest/on_response", json.RawMessage(`{not-valid`))
	if err != nil {
		t.Fatalf("handle returned error: %v", err)
	}
	if hit {
		t.Error("expected no HTTP call when params are malformed")
	}
	if !strings.Contains(errw.String(), "on_response: bad params") {
		t.Errorf("expected warning log for bad params, got: %q", errw.String())
	}
}

func TestRun_ReadError_Propagated(t *testing.T) {
	readErr := errors.New("simulated read error")
	r := &errReader{err: readErr}
	var out, errw bytes.Buffer
	err := run(context.Background(), loadConfig([]string{}), r, &out, &errw)
	if !errors.Is(err, readErr) {
		t.Errorf("expected read error to propagate, got: %v", err)
	}
}

func TestRun_EndToEnd_MultipleResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(202) }))
	defer srv.Close()
	cfg := loadConfig([]string{"DATADOG_API_KEY=k", "DD_API_URL=" + srv.URL})
	in := bytes.NewBufferString(
		`{"jsonrpc":"2.0","id":1,"method":"apitest/hello","params":{}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"apitest/on_response","params":{"status_code":200,"duration_ms":42}}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"apitest/on_result","params":{"pass_count":1}}` + "\n",
	)
	var out, errw bytes.Buffer
	if err := run(context.Background(), cfg, in, &out, &errw); err != nil {
		t.Fatalf("run: %v", err)
	}
	// Three responses, one per request
	lines := bytes.Split(bytes.TrimRight(out.Bytes(), "\n"), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("expected 3 response lines, got %d:\n%s", len(lines), out.String())
	}
	if !strings.Contains(errw.String(), "submitted 1 metric") {
		t.Errorf("on_response log missing: %q", errw.String())
	}
}
