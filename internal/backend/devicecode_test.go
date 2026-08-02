package backend_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
)

func TestStartDevice(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		ctype   string
		body    string
		want    backend.DeviceStart
		wantErr error
	}{
		{
			name:   "happy path",
			status: 200,
			ctype:  "application/json",
			body:   `{"device_code":"d","user_code":"ABCD-EFGH","verification_uri":"https://x/device","verification_uri_complete":"https://x/device?user_code=ABCD-EFGH","expires_in":900,"interval":5}`,
			want: backend.DeviceStart{
				DeviceCode:              "d",
				UserCode:                "ABCD-EFGH",
				VerificationURI:         "https://x/device",
				VerificationURIComplete: "https://x/device?user_code=ABCD-EFGH",
				ExpiresIn:               900,
				Interval:                5,
			},
		},
		{
			name:    "500 plain returns ErrServerError",
			status:  500,
			ctype:   "text/plain",
			body:    "boom",
			wantErr: backend.ErrServerError,
		},
		{
			name:    "400 problem+json returns ProblemDetails",
			status:  400,
			ctype:   "application/problem+json",
			body:    `{"code":"SOME_CODE","status":400,"title":"bad request"}`,
			wantErr: nil, // want *ProblemDetails via errors.As
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/auth/device/start" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method %q", r.Method)
				}
				w.Header().Set("Content-Type", tc.ctype)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			got, err := c.StartDevice(context.Background())
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("StartDevice error = %v, want errors.Is(%v)", err, tc.wantErr)
				}
				return
			}
			// For the 400 problem+json case, we want a *ProblemDetails
			if tc.status == 400 {
				var pd *backend.ProblemDetails
				if !errors.As(err, &pd) {
					t.Errorf("StartDevice error = %v, want *ProblemDetails", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("StartDevice: unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("StartDevice = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPollDevice_SentinelMapping(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		wantIs error
		wantOK bool // wantOK=true means expect a success (DevicePollResult)
	}{
		{
			name:   "pending",
			status: 400,
			body:   `{"code":"AUTH_DEVICE_AUTHORIZATION_PENDING","status":400,"title":"pending"}`,
			wantIs: backend.ErrAuthorizationPending,
		},
		{
			name:   "slow_down",
			status: 400,
			body:   `{"code":"AUTH_DEVICE_SLOW_DOWN","status":400,"title":"slow_down"}`,
			wantIs: backend.ErrSlowDown,
		},
		{
			name:   "expired",
			status: 400,
			body:   `{"code":"AUTH_DEVICE_EXPIRED_TOKEN","status":400,"title":"expired"}`,
			wantIs: backend.ErrExpiredToken,
		},
		{
			name:   "denied",
			status: 400,
			body:   `{"code":"AUTH_DEVICE_ACCESS_DENIED","status":400,"title":"denied"}`,
			wantIs: backend.ErrAccessDenied,
		},
		{
			name:   "success",
			status: 200,
			body:   `{"license_jwt":"L","access_token":"A","refresh_token":"R","device_id":"D"}`,
			wantOK: true,
		},
		{
			name:   "unrelated problem code passes through as ProblemDetails",
			status: 401,
			body:   `{"code":"AUTH_REFRESH_REUSED","status":401,"title":"reused"}`,
			wantIs: nil, // wants *ProblemDetails via errors.As
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/auth/device/poll" {
					t.Errorf("unexpected path %q", r.URL.Path)
				}
				if tc.status != 200 {
					w.Header().Set("Content-Type", "application/problem+json")
				} else {
					w.Header().Set("Content-Type", "application/json")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}

			result, err := c.PollDevice(context.Background(), "device-code-123")

			if tc.wantOK {
				if err != nil {
					t.Fatalf("PollDevice: unexpected error: %v", err)
				}
				if result.LicenseJWT != "L" || result.RefreshToken != "R" || result.DeviceID != "D" {
					t.Errorf("PollDevice result = %+v, want {L,A,R,D}", result)
				}
				return
			}

			if err == nil {
				t.Fatalf("PollDevice: expected error, got nil")
			}

			if tc.wantIs != nil {
				if !errors.Is(err, tc.wantIs) {
					t.Errorf("PollDevice error = %v, want errors.Is(%v)", err, tc.wantIs)
				}
				return
			}

			// unrelated problem code: should be *ProblemDetails
			var pd *backend.ProblemDetails
			if !errors.As(err, &pd) {
				t.Errorf("PollDevice error = %v, want *ProblemDetails", err)
			}
		})
	}
}
