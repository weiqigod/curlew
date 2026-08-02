package httpexec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExecute_TimingFreshConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	// A dedicated transport guarantees the first request sees a fresh connection.
	prev := http.DefaultClient.Transport
	http.DefaultClient.Transport = &http.Transport{}
	defer func() { http.DefaultClient.Transport = prev }()

	res, err := Execute(context.Background(), &Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Timing == nil {
		t.Fatal("Timing is nil for a successful HTTP exchange")
	}
	if res.Timing.Reused {
		t.Error("Reused = true on first request over a fresh transport")
	}
	if res.Timing.Connect <= 0 {
		t.Errorf("Connect = %v, want > 0 on a fresh connection", res.Timing.Connect)
	}
	if res.Timing.TLS != 0 {
		t.Errorf("TLS = %v, want 0 for plain http", res.Timing.TLS)
	}
	if res.Timing.TTFB <= 0 {
		t.Errorf("TTFB = %v, want > 0", res.Timing.TTFB)
	}
	if res.Timing.Total <= 0 {
		t.Errorf("Total = %v, want > 0", res.Timing.Total)
	}
	if res.Timing.Total < res.Timing.TTFB {
		t.Errorf("Total %v < TTFB %v", res.Timing.Total, res.Timing.TTFB)
	}
}

func TestExecute_TimingReusedConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	prev := http.DefaultClient.Transport
	http.DefaultClient.Transport = &http.Transport{}
	defer func() { http.DefaultClient.Transport = prev }()

	ctx := context.Background()
	if _, err := Execute(ctx, &Request{Method: "GET", URL: srv.URL}); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	res, err := Execute(ctx, &Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if res.Timing == nil {
		t.Fatal("Timing is nil")
	}
	if !res.Timing.Reused {
		t.Error("Reused = false on second request to the same host")
	}
	// Pooled connection: connection phases did not occur.
	if res.Timing.DNS != 0 || res.Timing.Connect != 0 || res.Timing.TLS != 0 {
		t.Errorf("phase durations on reused connection: dns=%v connect=%v tls=%v, want all 0",
			res.Timing.DNS, res.Timing.Connect, res.Timing.TLS)
	}
}

func TestExecute_TimingTLS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	prev := http.DefaultClient.Transport
	http.DefaultClient.Transport = srv.Client().Transport
	defer func() { http.DefaultClient.Transport = prev }()

	res, err := Execute(context.Background(), &Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Timing == nil {
		t.Fatal("Timing is nil")
	}
	if res.Timing.TLS <= 0 {
		t.Errorf("TLS = %v, want > 0 for https", res.Timing.TLS)
	}
}

func TestExecute_DurationSemanticsUnchanged(t *testing.T) {
	// Result.Duration is measured around Do() only and excludes body read;
	// Timing.Total spans Do() entry to body fully read, so Total >= Duration
	// minus scheduling noise. Assert the invariant loosely.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	res, err := Execute(context.Background(), &Request{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", res.Duration)
	}
	if res.Timing == nil {
		t.Fatal("Timing is nil")
	}
	if res.Timing.Total+5*time.Millisecond < res.Duration {
		t.Errorf("Total %v unexpectedly far below Duration %v", res.Timing.Total, res.Duration)
	}
}
