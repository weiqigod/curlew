package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmitter_DisabledNoNetwork(t *testing.T) {
	cs, srv := newCaptureServer(http.StatusOK)
	defer srv.Close()
	t.Setenv("APITEST_TELEMETRY_ENDPOINT", srv.URL)

	store, _ := newTestStore(t)
	// Enable then disable — install_id retained, enabled=false
	_, _ = store.Enable("")
	_ = store.Disable()

	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: time.Second})
	emitter := NewEmitter(store, client)
	emitter.Emit(context.Background(), "run.completed", map[string]any{})

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.requests) != 0 {
		t.Errorf("expected 0 HTTP requests when disabled, got %d", len(cs.requests))
	}
	// Emission should be recorded as "disabled" in ring buffer
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 {
		t.Error("expected a disabled emission to be recorded")
	}
	if ems[0].Status != "disabled" {
		t.Errorf("emission status = %q, want disabled", ems[0].Status)
	}
}

func TestEmitter_NoInstallIDNoNetwork(t *testing.T) {
	cs, srv := newCaptureServer(http.StatusOK)
	defer srv.Close()

	store, _ := newTestStore(t)
	// Never enabled
	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: time.Second})
	emitter := NewEmitter(store, client)
	emitter.Emit(context.Background(), "run.completed", nil)

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.requests) != 0 {
		t.Errorf("expected 0 HTTP requests when no install_id, got %d", len(cs.requests))
	}
}

func TestEmitter_EnabledHitsBackend(t *testing.T) {
	cs, srv := newCaptureServer(http.StatusOK)
	defer srv.Close()

	store, _ := newTestStore(t)
	id, err := store.Enable(srv.URL)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: time.Second})
	emitter := NewEmitter(store, client)
	emitter.Emit(context.Background(), "run.completed", map[string]any{"session_id": "s1"})

	cs.mu.Lock()
	defer cs.mu.Unlock()
	if len(cs.requests) != 1 {
		t.Errorf("expected 1 HTTP request, got %d", len(cs.requests))
	}
	if cs.body["install_id"] != id {
		t.Errorf("body install_id = %v, want %s", cs.body["install_id"], id)
	}
	// Emission should be recorded as "ok"
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 || ems[0].Status != "ok" {
		t.Errorf("expected ok emission, got %+v", ems)
	}
}

func TestEmitter_BackendErrorSwallowed(t *testing.T) {
	_, srv := newCaptureServer(http.StatusInternalServerError)
	defer srv.Close()

	store, _ := newTestStore(t)
	_, err := store.Enable(srv.URL)
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	client := NewClient(ClientOptions{Endpoint: srv.URL, Timeout: time.Second})
	emitter := NewEmitter(store, client)
	// Should not panic or return an error to caller
	emitter.Emit(context.Background(), "run.completed", nil)

	// Emission should be recorded as "error"
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 || ems[0].Status != "error" {
		t.Errorf("expected error emission, got %+v", ems)
	}
}

func TestEmitter_ContextCancellationPolite(t *testing.T) {
	testCtx, testCancel := context.WithCancel(context.Background())
	t.Cleanup(testCancel)

	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-testCtx.Done():
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { testCancel(); slowSrv.Close() })

	store, _ := newTestStore(t)
	_, _ = store.Enable(slowSrv.URL)

	client := NewClient(ClientOptions{Endpoint: slowSrv.URL, Timeout: 5 * time.Second})
	emitter := NewEmitter(store, client)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	emitter.Emit(ctx, "run.completed", nil)
	if time.Since(start) > 3*time.Second {
		t.Error("Emit did not return quickly after context cancellation")
	}
}
