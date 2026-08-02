package backend_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
)

func TestClient_GetJSON(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		contentType string
		body        string
		wantTyped   bool   // expect *ProblemDetails via errors.As
		wantCode    string // expected ProblemDetails.Code (when wantTyped)
		wantErrIs   error  // expected sentinel via errors.Is
	}{
		{
			name:        "200 application/json parses into typed result",
			statusCode:  200,
			contentType: "application/json",
			body:        `{"ok":true}`,
		},
		{
			name:        "401 problem+json AUTH_REFRESH_REUSED yields *ProblemDetails",
			statusCode:  401,
			contentType: "application/problem+json",
			body:        `{"code":"AUTH_REFRESH_REUSED","status":401,"title":"x","request_id":"req_1","type":"https://x"}`,
			wantTyped:   true,
			wantCode:    "AUTH_REFRESH_REUSED",
		},
		{
			name:        "500 without problem+json yields ErrServerError",
			statusCode:  500,
			contentType: "text/plain",
			body:        "boom",
			wantErrIs:   backend.ErrServerError,
		},
		{
			name:        "403 without problem+json yields generic HTTP error",
			statusCode:  403,
			contentType: "text/plain",
			body:        "no",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
			if err != nil {
				t.Fatal(err)
			}

			var out map[string]any
			gotErr := c.GetJSON(context.Background(), "/test", "", &out)

			if tc.wantErrIs != nil {
				if !errors.Is(gotErr, tc.wantErrIs) {
					t.Fatalf("got %v; want %v", gotErr, tc.wantErrIs)
				}
				return
			}
			if tc.wantTyped {
				var pd *backend.ProblemDetails
				if !errors.As(gotErr, &pd) {
					t.Fatalf("expected *ProblemDetails, got %T: %v", gotErr, gotErr)
				}
				if pd.Code != tc.wantCode {
					t.Errorf("Code = %q; want %q", pd.Code, tc.wantCode)
				}
				return
			}
			if tc.statusCode >= 200 && tc.statusCode < 300 && gotErr != nil {
				t.Fatalf("unexpected error for 2xx: %v", gotErr)
			}
			if tc.statusCode >= 400 && tc.statusCode < 500 && tc.contentType != "application/problem+json" {
				// generic HTTP error — some error expected
				if gotErr == nil {
					t.Fatal("expected error for 4xx, got nil")
				}
			}
		})
	}
}

func TestClient_PostJSON_setsBearerHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PostJSON(context.Background(), "/test", "abc123", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bearer abc123" {
		t.Fatalf("Authorization = %q; want %q", got, "Bearer abc123")
	}
}

func TestClient_GetJSON_setsBearerHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/test", "abc123", &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bearer abc123" {
		t.Fatalf("Authorization = %q; want %q", got, "Bearer abc123")
	}
}

func TestClient_BuildRefreshRequest_neverSetsAuthorizationHeader(t *testing.T) {
	c, err := backend.NewClient(backend.Options{BaseURL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	refreshToken := "rt-secret-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	req, err := c.BuildRefreshRequest(context.Background(), refreshToken, "dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization header set on refresh request: %q", got)
	}
	body, _ := io.ReadAll(req.Body)
	if !bytes.Contains(body, []byte("rt-secret-")) {
		t.Fatalf("refresh_token missing from body: %s", body)
	}
}

func TestClient_NetworkFailure(t *testing.T) {
	c, err := backend.NewClient(backend.Options{BaseURL: "http://127.0.0.1:1"}) // closed port
	if err != nil {
		t.Fatal(err)
	}
	err = c.GetJSON(context.Background(), "/x", "", nil)
	if !errors.Is(err, backend.ErrNetworkFailure) {
		t.Fatalf("got %v; want ErrNetworkFailure", err)
	}
}

func TestNewClient_EmptyBaseURLReturnsError(t *testing.T) {
	_, err := backend.NewClient(backend.Options{})
	if err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
}

func TestClient_GetJSONOptional(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		contentType string
		body        string
		wantFound   bool
		wantTyped   bool  // expect *ProblemDetails via errors.As
		wantErrIs   error // expected sentinel via errors.Is
		wantErr     bool  // any error
	}{
		{
			name:        "200 returns found=true and decodes body",
			statusCode:  200,
			contentType: "application/json",
			body:        `{"ok":true}`,
			wantFound:   true,
		},
		{
			name:       "204 returns found=false and nil error",
			statusCode: 204,
			wantFound:  false,
		},
		{
			name:        "401 problem+json returns found=false and ProblemDetails error",
			statusCode:  401,
			contentType: "application/problem+json",
			body:        `{"code":"AUTH_EXPIRED","status":401,"title":"x","request_id":"r1","type":"https://x"}`,
			wantFound:   false,
			wantTyped:   true,
		},
		{
			name:       "500 without problem+json returns ErrServerError",
			statusCode: 500,
			body:       "boom",
			wantFound:  false,
			wantErrIs:  backend.ErrServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.contentType != "" {
					w.Header().Set("Content-Type", tc.contentType)
				}
				w.WriteHeader(tc.statusCode)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
			if err != nil {
				t.Fatal(err)
			}

			var out map[string]any
			found, gotErr := c.GetJSONOptional(context.Background(), "/test", "", &out)

			if found != tc.wantFound {
				t.Errorf("found = %v; want %v", found, tc.wantFound)
			}

			if tc.wantErrIs != nil {
				if !errors.Is(gotErr, tc.wantErrIs) {
					t.Fatalf("got %v; want %v", gotErr, tc.wantErrIs)
				}
				return
			}
			if tc.wantTyped {
				var pd *backend.ProblemDetails
				if !errors.As(gotErr, &pd) {
					t.Fatalf("expected *ProblemDetails, got %T: %v", gotErr, gotErr)
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}
		})
	}
}
