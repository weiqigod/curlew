package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureServer is a test helper that records the last request's
// Idempotency-Key, Content-Type, and parsed JSON body.
type captureServer struct {
	mu          sync.Mutex
	idemKey     string
	contentType string
	body        map[string]any
	requests    []string // Idempotency-Key per request
	status      int      // response status to return
}

func newCaptureServer(status int) (*captureServer, *httptest.Server) {
	cs := &captureServer{status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		defer cs.mu.Unlock()
		cs.idemKey = r.Header.Get("Idempotency-Key")
		cs.contentType = r.Header.Get("Content-Type")
		cs.requests = append(cs.requests, cs.idemKey)
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		cs.body = b
		w.WriteHeader(cs.status)
	}))
	return cs, srv
}

func TestClientEmit_PostsCorrectBody(t *testing.T) {
	cs, srv := newCaptureServer(http.StatusOK)
	defer srv.Close()

	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: 2 * time.Second})
	payload := map[string]any{"session_id": "test-session", "collection_size": 3}
	err := client.Emit(context.Background(), "install-123", "run.completed", payload)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", cs.contentType)
	}
	if cs.idemKey == "" {
		t.Error("Idempotency-Key header is empty")
	}
	if !uuidRE.MatchString(cs.idemKey) {
		t.Errorf("Idempotency-Key %q is not a valid UUID v4", cs.idemKey)
	}
	if cs.body["install_id"] != "install-123" {
		t.Errorf("body install_id = %v, want install-123", cs.body["install_id"])
	}
	if cs.body["event_type"] != "run.completed" {
		t.Errorf("body event_type = %v, want run.completed", cs.body["event_type"])
	}
	if cs.body["event_payload"] == nil {
		t.Error("body event_payload is nil")
	}
}

func TestClientEmit_GeneratesUniqueIdempotencyKeyPerCall(t *testing.T) {
	cs, srv := newCaptureServer(http.StatusOK)
	defer srv.Close()
	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: 2 * time.Second})
	for range 5 {
		_ = client.Emit(context.Background(), "id", "run.completed", nil)
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	seen := make(map[string]bool)
	for _, k := range cs.requests {
		if seen[k] {
			t.Errorf("duplicate Idempotency-Key: %q", k)
		}
		seen[k] = true
	}
	if len(cs.requests) != 5 {
		t.Errorf("expected 5 requests, got %d", len(cs.requests))
	}
}

func TestClientEmit_202IsSuccess(t *testing.T) {
	_, srv := newCaptureServer(http.StatusAccepted)
	defer srv.Close()
	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: 2 * time.Second})
	if err := client.Emit(context.Background(), "id", "run.completed", nil); err != nil {
		t.Errorf("202 should be success, got: %v", err)
	}
}

func TestClientEmit_429IsNonFatal(t *testing.T) {
	_, srv := newCaptureServer(http.StatusTooManyRequests)
	defer srv.Close()
	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: 2 * time.Second})
	if err := client.Emit(context.Background(), "id", "run.completed", nil); err != nil {
		t.Errorf("429 should be non-fatal, got: %v", err)
	}
}

func TestClientEmit_500IsErrorButRecoverable(t *testing.T) {
	_, srv := newCaptureServer(http.StatusInternalServerError)
	defer srv.Close()
	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: 2 * time.Second})
	err := client.Emit(context.Background(), "id", "run.completed", nil)
	if err == nil {
		t.Error("500 should return an error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}

func TestClientEmit_NetworkFailureReturnsError(t *testing.T) {
	// Point at a closed server
	_, srv := newCaptureServer(http.StatusOK)
	url := srv.URL
	srv.Close() // close immediately

	client := NewClient(ClientOptions{Endpoint: url, Timeout: 2 * time.Second})
	err := client.Emit(context.Background(), "id", "run.completed", nil)
	if err == nil {
		t.Error("network failure should return error")
	}
}

func TestClientEmit_TimeoutIsBounded(t *testing.T) {
	// Handler blocks until client disconnects; we use a wrapping context
	// to signal when the test finishes so the goroutine exits cleanly.
	testCtx, testCancel := context.WithCancel(context.Background())
	t.Cleanup(testCancel)

	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-testCtx.Done():
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		testCancel() // unblocks handler so Close() can drain
		slowSrv.Close()
	})

	timeout := 200 * time.Millisecond
	client := NewClient(ClientOptions{Endpoint: slowSrv.URL, Timeout: timeout})
	start := time.Now()
	err := client.Emit(context.Background(), "id", "run.completed", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("slow server should return error (timeout)")
	}
	margin := 2 * time.Second
	if elapsed > timeout+margin {
		t.Errorf("Emit took %v, want <= %v", elapsed, timeout+margin)
	}
}

func TestClientEmit_RespectsContext(t *testing.T) {
	testCtx, testCancel := context.WithCancel(context.Background())
	t.Cleanup(testCancel)

	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-testCtx.Done():
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() {
		testCancel()
		slowSrv.Close()
	})

	client := NewClient(ClientOptions{Endpoint: slowSrv.URL, Timeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := client.Emit(ctx, "id", "run.completed", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Error("cancelled context should return error")
	}
	if elapsed > 3*time.Second {
		t.Errorf("Emit took %v after context cancel, want < 3s", elapsed)
	}
}
