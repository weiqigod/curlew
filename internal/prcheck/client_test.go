package prcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_UploadResults(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		respBody   string
		wantErr    string
		wantID     string
	}{
		{"success_202", 202, `{"result_id":"res_abc","status":"accepted"}`, "", "res_abc"},
		{"unauthorized_401", 401, `{"error":"unauthorized"}`, "unauthorized", ""},
		{"server_error_500", 500, `{"error":"internal"}`, "HTTP 500", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("Authorization") == "" {
					t.Error("missing Authorization header")
				}
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.respBody))
			}))
			defer srv.Close()

			client := &Client{
				BaseURL:    srv.URL,
				Token:      "test-token",
				HTTPClient: srv.Client(),
			}
			payload := &ResultsPayload{
				CollectionName: "test",
				PassCount:      3,
				FailCount:      0,
				Items:          []ResultItem{},
			}
			resp, err := client.UploadResults(context.Background(), "acme", payload)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("UploadResults() = nil error; want error containing %q", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Errorf("UploadResults() error = %q; want containing %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("UploadResults() = %v; want nil", err)
			}
			if resp.ResultID != tc.wantID {
				t.Errorf("ResultID = %q; want %q", resp.ResultID, tc.wantID)
			}
		})
	}
}

func TestClient_PostPrCheck(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    string
	}{
		{"success_200", 200, ""},
		{"unauthorized_401", 401, "unauthorized"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var received PrCheckPayload
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&received)
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
			defer srv.Close()

			client := &Client{
				BaseURL:    srv.URL,
				Token:      "test-token",
				HTTPClient: srv.Client(),
			}
			payload := &PrCheckPayload{
				Repo:     "acme/api",
				PR:       42,
				State:    "success",
				ResultID: "res_123",
			}
			err := client.PostPrCheck(context.Background(), "acme", payload)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("PostPrCheck() = nil error; want error containing %q", tc.wantErr)
				}
				if !contains(err.Error(), tc.wantErr) {
					t.Errorf("PostPrCheck() error = %q; want containing %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("PostPrCheck() = %v; want nil", err)
			}
			// Verify payload was sent correctly
			if tc.statusCode == 200 {
				if received.Repo != "acme/api" || received.PR != 42 || received.State != "success" {
					t.Errorf("received payload = %+v; want {Repo:acme/api PR:42 State:success}", received)
				}
			}
		})
	}
}

func TestClient_RetryOnConnectionRefused(t *testing.T) {
	// Create a server and immediately close it to get a real closed port
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	var attempts int32
	// Wrap the default transport to count attempts
	originalTransport := http.DefaultTransport
	countingTransport := &countingRoundTripper{
		inner:    originalTransport,
		attempts: &attempts,
	}

	client := &Client{
		BaseURL: addr,
		Token:   "test-token",
		HTTPClient: &http.Client{
			Transport: countingTransport,
			Timeout:   2 * time.Second,
		},
	}
	payload := &ResultsPayload{
		CollectionName: "test",
		Items:          []ResultItem{},
	}
	_, err := client.UploadResults(context.Background(), "acme", payload)
	if err == nil {
		t.Fatal("UploadResults() = nil; want error")
	}
	// Should be ErrNetworkFailure after retries
	if !contains(err.Error(), "network error") {
		t.Errorf("error = %q; want containing 'network error'", err.Error())
	}
	// Should have made maxRetries+1 = 3 attempts
	got := int(atomic.LoadInt32(&attempts))
	if got != maxRetries+1 {
		t.Errorf("attempts = %d; want %d", got, maxRetries+1)
	}
}

func TestClient_RetryBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping backoff timing test in short mode")
	}
	// Start a server and immediately close it to get a real closed port.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()

	var attempts int32
	countingTransport := &countingRoundTripper{
		inner:    http.DefaultTransport,
		attempts: &attempts,
	}
	client := &Client{
		BaseURL: addr,
		Token:   "test-token",
		HTTPClient: &http.Client{
			Transport: countingTransport,
			Timeout:   5 * time.Second,
		},
	}
	payload := &ResultsPayload{
		CollectionName: "test",
		Items:          []ResultItem{},
	}

	start := time.Now()
	_, err := client.UploadResults(context.Background(), "acme", payload)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("UploadResults() = nil; want error")
	}
	// All retries should have fired: maxRetries+1 attempts total.
	got := int(atomic.LoadInt32(&attempts))
	if got != maxRetries+1 {
		t.Errorf("attempts = %d; want %d", got, maxRetries+1)
	}
	// Total elapsed should be at least maxRetries * retryInterval due to backoff sleeps.
	minExpected := time.Duration(maxRetries) * retryInterval
	if elapsed < minExpected {
		t.Errorf("elapsed = %v; want >= %v (backoff not applied)", elapsed, minExpected)
	}
}

// countingRoundTripper wraps a transport and counts request attempts.
type countingRoundTripper struct {
	inner    http.RoundTripper
	attempts *int32
}

func (c *countingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(c.attempts, 1)
	return c.inner.RoundTrip(req)
}
