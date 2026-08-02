package schedule_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/backend"
	"github.com/weiqigod/curlew/internal/worker/schedule"
)

// newTestClient builds a schedule.Client wired to a test httptest.Server.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*schedule.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	bc, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("backend.NewClient: %v", err)
	}
	return &schedule.Client{HTTP: bc, AccessToken: "test-token"}, srv
}

func TestClient_PollNextRun(t *testing.T) {
	const validBody = `{"run_id":"run_a","schedule_id":"sched_b","collection_ref":"file:./api.yaml","env_vars":{},"claim_token":"00000000-0000-0000-0000-000000000001","deadline":"2026-05-11T10:00:00Z"}`

	tests := []struct {
		name         string
		serverStatus int
		serverBody   string
		serverCT     string
		wantErr      error
		wantTyped    bool // *backend.ProblemDetails
		wantRunID    string
	}{
		{
			name:         "200 returns run",
			serverStatus: 200,
			serverBody:   validBody,
			serverCT:     "application/json",
			wantRunID:    "run_a",
		},
		{
			name:         "204 returns ErrNoRunAvailable",
			serverStatus: 204,
			wantErr:      schedule.ErrNoRunAvailable,
		},
		{
			name:         "401 returns ProblemDetails",
			serverStatus: 401,
			serverCT:     "application/problem+json",
			serverBody:   `{"code":"AUTH_EXPIRED","status":401,"title":"Unauthorized","request_id":"r1","type":"https://x"}`,
			wantTyped:    true,
		},
		{
			name:         "500 server error",
			serverStatus: 500,
			serverBody:   "boom",
			wantErr:      backend.ErrServerError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("method = %q; want GET", r.Method)
				}
				if tc.serverCT != "" {
					w.Header().Set("Content-Type", tc.serverCT)
				}
				w.WriteHeader(tc.serverStatus)
				_, _ = io.WriteString(w, tc.serverBody)
			})

			got, err := c.PollNextRun(context.Background())

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v; want %v", err, tc.wantErr)
				}
				return
			}
			if tc.wantTyped {
				var pd *backend.ProblemDetails
				if !errors.As(err, &pd) {
					t.Fatalf("expected *ProblemDetails, got %T: %v", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.RunID != tc.wantRunID {
				t.Errorf("RunID = %q; want %q", got.RunID, tc.wantRunID)
			}
		})
	}
}

func TestClient_PollNextRun_RefreshesExpiredAccessTokenAndRetries(t *testing.T) {
	const validBody = `{"run_id":"run_a","schedule_id":"sched_b","collection_ref":"file:./api.yaml","env_vars":{},"claim_token":"00000000-0000-0000-0000-000000000001","deadline":"2026-05-11T10:00:00Z"}`
	var calls, refreshes int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); calls == 1 && got != "Bearer test-token" {
			t.Errorf("first Authorization = %q", got)
		}
		if calls == 1 {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"https://x","title":"Unauthorized","status":401,"code":"unauthorized"}`)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			t.Errorf("retry Authorization = %q; want refreshed token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, validBody)
	})
	c.RefreshAccessToken = func(context.Context) (string, error) {
		refreshes++
		return "refreshed-token", nil
	}

	run, err := c.PollNextRun(context.Background())
	if err != nil {
		t.Fatalf("PollNextRun: %v", err)
	}
	if run.RunID != "run_a" {
		t.Fatalf("RunID = %q; want run_a", run.RunID)
	}
	if calls != 2 || refreshes != 1 {
		t.Fatalf("calls=%d refreshes=%d; want 2 and 1", calls, refreshes)
	}
}

func TestClient_Heartbeat_ReapedClaim(t *testing.T) {
	tests := []struct {
		name         string
		serverStatus int
		serverBody   string
		serverCT     string
		wantErr      error
	}{
		{
			name:         "200 returns nil",
			serverStatus: 200,
		},
		{
			name:         "409 claim-reaped returns ErrClaimReaped",
			serverStatus: 409,
			serverCT:     "application/problem+json",
			serverBody:   `{"code":"CLAIM_REAPED","status":409,"title":"Claim reaped","request_id":"r1","type":"claim-reaped"}`,
			wantErr:      schedule.ErrClaimReaped,
		},
		{
			name:         "409 stale-claim also returns ErrClaimReaped",
			serverStatus: 409,
			serverCT:     "application/problem+json",
			serverBody:   `{"code":"STALE_CLAIM","status":409,"title":"Stale claim","request_id":"r1","type":"stale-claim"}`,
			wantErr:      schedule.ErrClaimReaped,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if tc.serverCT != "" {
					w.Header().Set("Content-Type", tc.serverCT)
				}
				w.WriteHeader(tc.serverStatus)
				_, _ = io.WriteString(w, tc.serverBody)
			})

			err := c.Heartbeat(context.Background(), "run_x", "tok-1")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v; want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestClient_PostResult_AlreadyCompleted(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(409)
		_, _ = io.WriteString(w, `{"code":"ALREADY_COMPLETED","status":409,"title":"Already completed","request_id":"r1","type":"already-completed"}`)
	})

	payload := &schedule.ResultRequest{
		ClaimToken: "tok-1",
		RunAt:      time.Now(),
		DurationMs: 100,
		PassCount:  1,
	}
	err := c.PostResult(context.Background(), "run_x", payload)
	if !errors.Is(err, schedule.ErrAlreadyCompleted) {
		t.Fatalf("error = %v; want ErrAlreadyCompleted", err)
	}
}

func TestClient_PostResult_NetworkFailure(t *testing.T) {
	bc, err := backend.NewClient(backend.Options{BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	c := &schedule.Client{HTTP: bc, AccessToken: "tok"}

	payload := &schedule.ResultRequest{
		ClaimToken: "tok-1",
		RunAt:      time.Now(),
	}
	err = c.PostResult(context.Background(), "run_x", payload)
	if !errors.Is(err, backend.ErrNetworkFailure) {
		t.Fatalf("error = %v; want ErrNetworkFailure", err)
	}
}

func TestClient_PostResult_Success(t *testing.T) {
	var receivedPath string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(200)
	})

	payload := &schedule.ResultRequest{
		ClaimToken: "tok-1",
		RunAt:      time.Now(),
		PassCount:  3,
	}
	err := c.PostResult(context.Background(), "run_abc", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/api/v1/schedules/runs/run_abc/result"
	if receivedPath != want {
		t.Errorf("path = %q; want %q", receivedPath, want)
	}
}
