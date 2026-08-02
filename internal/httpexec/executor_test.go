package httpexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

func TestExecute(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		req        *Request
		wantStatus int
		wantErr    error
	}{
		{
			name:       "successful GET returns 200",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) },
			req:        &Request{Method: "GET"},
			wantStatus: 200,
		},
		{
			name:       "server returns 404",
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) },
			req:        &Request{Method: "GET"},
			wantStatus: 404,
		},
		{
			name: "request headers are sent",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "application/json" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "GET", Headers: map[string]string{"Accept": "application/json"}},
			wantStatus: 200,
		},
		{
			name:    "network error on connection refused",
			req:     &Request{Method: "GET", URL: "http://127.0.0.1:1/unreachable"},
			wantErr: ErrNetwork,
		},
		{
			name: "POST with JSON body sends correct Content-Type",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/json" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "POST", Body: map[string]interface{}{"key": "value"}},
			wantStatus: 200,
		},
		{
			name: "POST with JSON body sends correct JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var m map[string]interface{}
				if json.Unmarshal(body, &m) != nil || m["key"] != "value" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "POST", Body: map[string]interface{}{"key": "value"}},
			wantStatus: 200,
		},
		{
			name: "POST with string body sends raw string",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if string(body) != "raw body content" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "POST", Body: "raw body content"},
			wantStatus: 200,
		},
		{
			name: "POST with string body does not auto-set Content-Type",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "POST", Body: "raw body"},
			wantStatus: 200,
		},
		{
			name: "POST with map body and explicit Content-Type preserves user header",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/vnd.api+json" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req: &Request{
				Method:  "POST",
				Body:    map[string]interface{}{"key": "value"},
				Headers: map[string]string{"Content-Type": "application/vnd.api+json"},
			},
			wantStatus: 200,
		},
		{
			name: "query params appended to URL",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("limit") != "10" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req: &Request{
				Method:      "GET",
				QueryParams: map[string]string{"page": "1", "limit": "10"},
			},
			wantStatus: 200,
		},
		{
			name: "query params with special characters are URL-encoded",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("q") != "hello world" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req: &Request{
				Method:      "GET",
				QueryParams: map[string]string{"q": "hello world"},
			},
			wantStatus: 200,
		},
		{
			name: "empty query params map does not modify URL",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.RawQuery != "" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req:        &Request{Method: "GET", QueryParams: map[string]string{}},
			wantStatus: 200,
		},
		{
			name: "nested JSON body is correctly serialized",
			handler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var m map[string]interface{}
				if json.Unmarshal(body, &m) != nil {
					w.WriteHeader(400)
					return
				}
				user, ok := m["user"].(map[string]interface{})
				if !ok || user["name"] != "Test" {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			},
			req: &Request{
				Method: "POST",
				Body:   map[string]interface{}{"user": map[string]interface{}{"name": "Test"}},
			},
			wantStatus: 200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Start test server for non-error cases
			if tt.handler != nil {
				srv := httptest.NewServer(tt.handler)
				defer srv.Close()
				tt.req.URL = srv.URL
			}

			result, err := Execute(ctx, tt.req)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", result.StatusCode, tt.wantStatus)
			}
			if result.Duration <= 0 {
				t.Error("expected positive duration")
			}
		})
	}
}

func TestExecute_all_methods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method+" request sends correct method", func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					w.WriteHeader(400)
					return
				}
				w.WriteHeader(200)
			}
			srv := httptest.NewServer(http.HandlerFunc(handler))
			defer srv.Close()

			req := &Request{Method: method, URL: srv.URL}
			result, err := Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.StatusCode != 200 {
				t.Errorf("got status %d, want 200", result.StatusCode)
			}
		})
	}
}

func TestExecute_network_error_preserves_inner_error(t *testing.T) {
	ctx := context.Background()
	req := &Request{Method: "GET", URL: "http://127.0.0.1:1/unreachable"}

	_, err := Execute(ctx, req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("error chain missing ErrNetwork: %v", err)
	}

	// Error should be a classified NetworkError
	var ne *apierrors.NetworkError
	if !errors.As(err, &ne) {
		t.Fatal("expected *apierrors.NetworkError")
	}
}

func TestExecute_NetworkErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		req      *Request
		ctx      context.Context
		wantKind apierrors.NetworkErrorKind
		wantHint string
	}{
		{
			"connection refused classifies correctly",
			&Request{Method: "GET", URL: "http://127.0.0.1:1/unreachable"},
			context.Background(),
			apierrors.NetworkConnectionRefused,
			"server is running",
		},
		{
			"dns failure classifies correctly",
			&Request{Method: "GET", URL: "http://nonexistent.invalid/test"},
			context.Background(),
			apierrors.NetworkDNS,
			"hostname",
		},
		{
			"classified errors preserve ErrNetwork",
			&Request{Method: "GET", URL: "http://127.0.0.1:1/unreachable"},
			context.Background(),
			apierrors.NetworkConnectionRefused,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Execute(tt.ctx, tt.req)
			if err == nil {
				t.Fatal("expected error")
			}

			var ne *apierrors.NetworkError
			if !errors.As(err, &ne) {
				t.Fatalf("expected *apierrors.NetworkError, got %T: %v", err, err)
			}
			if ne.Kind != tt.wantKind {
				t.Errorf("Kind = %v, want %v", ne.Kind, tt.wantKind)
			}
			if tt.wantHint != "" && !strings.Contains(ne.Hint, tt.wantHint) {
				t.Errorf("Hint = %q, want to contain %q", ne.Hint, tt.wantHint)
			}

			// Verify ErrNetwork is still in the chain
			if !errors.Is(err, ErrNetwork) {
				t.Error("error chain should still contain ErrNetwork")
			}
		})
	}
}

func TestExecute_body_is_captured(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"name":"Alice"}`))
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	req := &Request{Method: "GET", URL: srv.URL}
	result, err := Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{"id":1,"name":"Alice"}`
	if string(result.Body) != want {
		t.Errorf("Body = %q, want %q", string(result.Body), want)
	}
}

func TestExecute_empty_body_is_empty_slice(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	req := &Request{Method: "GET", URL: srv.URL}
	result, err := Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Body == nil {
		t.Fatal("Body should not be nil")
	}
	if len(result.Body) != 0 {
		t.Errorf("Body length = %d, want 0", len(result.Body))
	}
}

func TestExecute_response_headers_captured(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom", "test-value")
		w.WriteHeader(200)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	req := &Request{Method: "GET", URL: srv.URL}
	result, err := Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Headers == nil {
		t.Fatal("Headers should not be nil")
	}
	if result.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want %q", result.Headers.Get("Content-Type"), "application/json")
	}
	if result.Headers.Get("X-Custom") != "test-value" {
		t.Errorf("X-Custom = %q, want %q", result.Headers.Get("X-Custom"), "test-value")
	}
}

func TestExecute_timeout_includes_duration(t *testing.T) {
	// Server that hangs until request context is done
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	req := &Request{Method: "GET", URL: srv.URL}
	_, err := Execute(ctx, req)
	if err == nil {
		t.Fatal("expected error")
	}

	var ne *apierrors.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("expected *apierrors.NetworkError, got %T: %v", err, err)
	}
	if ne.Kind != apierrors.NetworkTimeout {
		t.Errorf("Kind = %v, want NetworkTimeout", ne.Kind)
	}
	if ne.Duration <= 0 {
		t.Error("expected positive Duration")
	}
	if !strings.Contains(ne.Message, "timed out after") {
		t.Errorf("Message = %q, want to contain 'timed out after'", ne.Message)
	}
	if !strings.Contains(ne.Hint, "timeout") {
		t.Errorf("Hint = %q, want to contain 'timeout'", ne.Hint)
	}
}

func TestExecute_cancelled_context(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &Request{Method: "GET", URL: "http://localhost:9999"}
	_, err := Execute(ctx, req)
	if !errors.Is(err, ErrNetwork) {
		t.Fatalf("got error %v, want %v", err, ErrNetwork)
	}
}

func TestExecute_byte_slice_body_sent_verbatim(t *testing.T) {
	payload := []byte{0x00, 0x01, 0x02, 0xDE, 0xAD, 0xBE, 0xEF, 0xFF}
	var received []byte
	var receivedCT string
	handler := func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		received = b
		receivedCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	req := &Request{
		Method:  "POST",
		URL:     srv.URL,
		Body:    payload,
		Headers: map[string]string{"Content-Type": "application/octet-stream"},
	}
	if _, err := Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !sliceEqual(received, payload) {
		t.Errorf("received = %x, want %x (bytes must be sent verbatim)", received, payload)
	}
	if receivedCT != "application/octet-stream" {
		t.Errorf("Content-Type at server = %q, want application/octet-stream", receivedCT)
	}
}

func TestExecute_byte_slice_body_no_auto_content_type(t *testing.T) {
	// Without a user-provided Content-Type header, the []byte branch must not
	// invent one (unlike the map branch which auto-applies application/json).
	var receivedCT string
	handler := func(w http.ResponseWriter, r *http.Request) {
		receivedCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	req := &Request{Method: "POST", URL: srv.URL, Body: []byte("raw")}
	if _, err := Execute(context.Background(), req); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if receivedCT != "" {
		t.Errorf("Content-Type = %q, want empty (no auto-detect for []byte)", receivedCT)
	}
}

func sliceEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
