package prcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// makeResultsFile writes a results JSON to a temp file and returns the path.
func makeResultsFile(t *testing.T, passCount, failCount int) string {
	t.Helper()
	payload := ResultsPayload{
		CollectionName: "test-suite",
		RunAt:          "2026-04-16T10:00:00Z",
		DurationMs:     500,
		PassCount:      passCount,
		FailCount:      failCount,
		TriggeredBy:    "pr-check",
		Items:          []ResultItem{},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal results: %v", err)
	}
	f, err := os.CreateTemp(t.TempDir(), "results*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return f.Name()
}

// makeMockBackend creates a test server that handles results and pr-checks endpoints.
func makeMockBackend(t *testing.T, resultStatus, prCheckStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/results") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resultStatus)
			if resultStatus >= 200 && resultStatus < 300 {
				_, _ = w.Write([]byte(`{"result_id":"res_test001","status":"accepted"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"error"}`))
			}
			return
		}
		if strings.Contains(r.URL.Path, "/pr-checks") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(prCheckStatus)
			if prCheckStatus >= 200 && prCheckStatus < 300 {
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			} else {
				_, _ = w.Write([]byte(`{"error":"error"}`))
			}
			return
		}
		w.WriteHeader(404)
	}))
}

func TestRun_Success(t *testing.T) {
	srv := makeMockBackend(t, 202, 200)
	defer srv.Close()

	cfg := Config{
		BackendURL:   srv.URL,
		BackendToken: "tok",
		Org:          "acme",
		PR:           42,
		Repo:         "acme/api",
		ResultsFile:  makeResultsFile(t, 3, 0),
	}

	var buf bytes.Buffer
	result, err := Run(context.Background(), cfg, &buf)
	if err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if result.ResultID != "res_test001" {
		t.Errorf("ResultID = %q; want res_test001", result.ResultID)
	}
	if result.State != "success" {
		t.Errorf("State = %q; want success", result.State)
	}
	if result.Pass != 3 || result.Fail != 0 {
		t.Errorf("Pass=%d Fail=%d; want Pass=3 Fail=0", result.Pass, result.Fail)
	}
}

func TestRun_DryRun_NoHTTP(t *testing.T) {
	// Point at an unroutable address — if any HTTP is attempted, it will fail
	cfg := Config{
		BackendURL:   "http://127.0.0.1:0", // port 0 = no server
		BackendToken: "tok",
		Org:          "acme",
		PR:           42,
		Repo:         "acme/api",
		ResultsFile:  makeResultsFile(t, 3, 0),
		DryRun:       true,
	}

	var buf bytes.Buffer
	result, err := Run(context.Background(), cfg, &buf)
	if err != nil {
		t.Fatalf("Run() dry-run = %v; want nil", err)
	}
	if result.State != "success" {
		t.Errorf("State = %q; want success", result.State)
	}
	// Should print JSON payloads
	out := buf.String()
	if !strings.Contains(out, "collection_name") {
		t.Errorf("dry-run output missing 'collection_name': %q", out)
	}
}

func TestRun_FailingTests_StateFailure(t *testing.T) {
	var capturedPrCheck PrCheckPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/results") {
			w.WriteHeader(202)
			_, _ = w.Write([]byte(`{"result_id":"res_fail","status":"accepted"}`))
			return
		}
		if strings.Contains(r.URL.Path, "/pr-checks") {
			_ = json.NewDecoder(r.Body).Decode(&capturedPrCheck)
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	cfg := Config{
		BackendURL:   srv.URL,
		BackendToken: "tok",
		Org:          "acme",
		PR:           99,
		Repo:         "acme/api",
		ResultsFile:  makeResultsFile(t, 2, 1),
	}

	var buf bytes.Buffer
	result, err := Run(context.Background(), cfg, &buf)
	if err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if result.State != "failure" {
		t.Errorf("State = %q; want failure", result.State)
	}
	if capturedPrCheck.State != "failure" {
		t.Errorf("pr-check state = %q; want failure", capturedPrCheck.State)
	}
	if capturedPrCheck.PR != 99 {
		t.Errorf("pr-check PR = %d; want 99", capturedPrCheck.PR)
	}
}

func TestRun_MissingConfig_Error(t *testing.T) {
	cfg := Config{} // empty
	var buf bytes.Buffer
	_, err := Run(context.Background(), cfg, &buf)
	if err == nil {
		t.Fatal("Run() = nil; want error")
	}
	if !errors.Is(err, ErrBackendURLMissing) {
		t.Errorf("error = %v; want ErrBackendURLMissing", err)
	}
}

func TestRun_Unauthorized_Error(t *testing.T) {
	srv := makeMockBackend(t, 401, 200)
	defer srv.Close()

	cfg := Config{
		BackendURL:   srv.URL,
		BackendToken: "bad-token",
		Org:          "acme",
		PR:           42,
		Repo:         "acme/api",
		ResultsFile:  makeResultsFile(t, 1, 0),
	}
	var buf bytes.Buffer
	_, err := Run(context.Background(), cfg, &buf)
	if err == nil {
		t.Fatal("Run() = nil; want error")
	}
	if !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error = %v; want ErrUnauthorized", err)
	}
}

func TestRun_ConnectionRefused_Error(t *testing.T) {
	// Create a server then close it immediately
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	cfg := Config{
		BackendURL:   addr,
		BackendToken: "tok",
		Org:          "acme",
		PR:           42,
		Repo:         "acme/api",
		ResultsFile:  makeResultsFile(t, 1, 0),
	}
	var buf bytes.Buffer
	_, err := Run(context.Background(), cfg, &buf)
	if err == nil {
		t.Fatal("Run() = nil; want error")
	}
	if !errors.Is(err, ErrNetworkFailure) {
		t.Errorf("error = %v; want ErrNetworkFailure", err)
	}
}

func TestRun_BadResultsFile_Error(t *testing.T) {
	cfg := Config{
		BackendURL:   "http://localhost",
		BackendToken: "tok",
		Org:          "acme",
		PR:           42,
		Repo:         "acme/api",
		ResultsFile:  "/nonexistent/file.json",
	}
	var buf bytes.Buffer
	_, err := Run(context.Background(), cfg, &buf)
	if err == nil {
		t.Fatal("Run() = nil; want error")
	}
	if !strings.Contains(err.Error(), "reading results file") {
		t.Errorf("error = %q; want containing 'reading results file'", err.Error())
	}
}
