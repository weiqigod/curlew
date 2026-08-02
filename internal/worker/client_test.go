package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_Claim(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantShard  *ShardResponse
		wantErr    error
		wantErrMsg string
	}{
		{
			name: "success_200_returns_shard",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_ = json.NewEncoder(w).Encode(ShardResponse{
					ShardID: "shd_1",
					JobID:   "job_abc",
					State:   "claimed",
				})
			},
			wantShard: &ShardResponse{ShardID: "shd_1", JobID: "job_abc", State: "claimed"},
		},
		{
			name: "no_content_204_returns_nil_nil",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(204)
			},
			wantShard: nil,
		},
		{
			name: "unauthorized_401",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "server_error_500_then_success",
			handler: func() http.HandlerFunc {
				var calls int32
				return func(w http.ResponseWriter, r *http.Request) {
					if atomic.AddInt32(&calls, 1) == 1 {
						w.WriteHeader(500)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					_ = json.NewEncoder(w).Encode(ShardResponse{ShardID: "shd_retry"})
				}
			}(),
			wantShard: &ShardResponse{ShardID: "shd_retry"},
		},
		{
			name: "server_error_500_exhausted",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(500)
			},
			wantErr: ErrNetworkExhausted,
		},
		{
			name: "malformed_json",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				_, _ = w.Write([]byte("{not json"))
			},
			wantErrMsg: "parsing claim response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			c := &Client{
				BaseURL:     srv.URL,
				Token:       "test-token",
				BackoffBase: 1 * time.Millisecond, // fast retries for tests
			}

			shard, err := c.Claim(context.Background(), "acme", "job_abc", &ClaimRequestBody{WorkerID: "wkr_test"})

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Claim() error = %v; want %v", err, tc.wantErr)
				}
			case tc.wantErrMsg != "":
				if err == nil {
					t.Errorf("Claim() error = nil; want error containing %q", tc.wantErrMsg)
				} else if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("Claim() error = %v; want error containing %q", err, tc.wantErrMsg)
				}
			default:
				if err != nil {
					t.Errorf("Claim() error = %v; want nil", err)
				}
				if tc.wantShard == nil {
					if shard != nil {
						t.Errorf("Claim() shard = %v; want nil", shard)
					}
				} else {
					if shard == nil {
						t.Fatal("Claim() shard = nil; want non-nil")
					}
					if shard.ShardID != tc.wantShard.ShardID {
						t.Errorf("Claim() shard.ShardID = %q; want %q", shard.ShardID, tc.wantShard.ShardID)
					}
				}
			}
		})
	}
}

func TestClient_SubmitResult(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "success_202",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(202)
			},
		},
		{
			name: "unauthorized_401",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "connection_drop_then_success",
			handler: func() http.HandlerFunc {
				var calls int32
				return func(w http.ResponseWriter, r *http.Request) {
					if atomic.AddInt32(&calls, 1) == 1 {
						w.WriteHeader(500)
						return
					}
					w.WriteHeader(202)
				}
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			c := &Client{
				BaseURL:     srv.URL,
				Token:       "test-token",
				BackoffBase: 1 * time.Millisecond,
			}

			err := c.SubmitResult(context.Background(), "acme", "job_abc", "shd_1", &SubmitResultBody{
				WorkerID:  "wkr_test",
				PassCount: 3,
			})

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("SubmitResult() error = %v; want %v", err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Errorf("SubmitResult() error = %v; want nil", err)
				}
			}
		})
	}
}

func TestClient_Heartbeat(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    error
		wantErrMsg string
	}{
		{
			name: "success_204",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(204)
			},
		},
		{
			name: "not_found_404",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(404)
				_, _ = w.Write([]byte("not found"))
			},
			wantErrMsg: "404",
		},
		{
			name: "unauthorized_401",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(401)
			},
			wantErr: ErrUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			c := &Client{
				BaseURL:     srv.URL,
				Token:       "test-token",
				BackoffBase: 1 * time.Millisecond,
			}

			err := c.Heartbeat(context.Background(), "acme", "job_abc", "shd_1", &HeartbeatBody{WorkerID: "wkr_test"})

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("Heartbeat() error = %v; want %v", err, tc.wantErr)
				}
			case tc.wantErrMsg != "":
				if err == nil {
					t.Errorf("Heartbeat() error = nil; want error containing %q", tc.wantErrMsg)
				} else if !strings.Contains(err.Error(), tc.wantErrMsg) {
					t.Errorf("Heartbeat() error = %v; want error containing %q", err, tc.wantErrMsg)
				}
			default:
				if err != nil {
					t.Errorf("Heartbeat() error = %v; want nil", err)
				}
			}
		})
	}
}

func TestClient_Retry_BackoffGrows(t *testing.T) {
	var callTimes []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callTimes = append(callTimes, time.Now())
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := &Client{
		BaseURL:     srv.URL,
		Token:       "test-token",
		BackoffBase: 10 * time.Millisecond,
		MaxRetries:  2,
	}

	_, _ = c.Claim(context.Background(), "acme", "job_abc", &ClaimRequestBody{WorkerID: "wkr_test"})

	if len(callTimes) < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", len(callTimes))
	}

	gap1 := callTimes[1].Sub(callTimes[0])
	if gap1 < 5*time.Millisecond {
		t.Errorf("gap between attempt 1 and 2 = %v; expected >= 5ms", gap1)
	}
	if len(callTimes) >= 3 {
		gap2 := callTimes[2].Sub(callTimes[1])
		if gap2 < gap1 {
			t.Errorf("gap2 (%v) < gap1 (%v); expected exponential growth", gap2, gap1)
		}
	}
}
