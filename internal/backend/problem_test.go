package backend_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
)

func makeResp(statusCode int, contentType, body string, extraHeaders map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if contentType != "" {
		resp.Header.Set("Content-Type", contentType)
	}
	for k, v := range extraHeaders {
		resp.Header.Set(k, v)
	}
	return resp
}

func TestDecodeProblem(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		statusCode  int
		extraHeader map[string]string
		wantErr     error
		wantCode    string
		wantStatus  int
		wantReqID   string
	}{
		{
			name:        "AUTH_REFRESH_REUSED canonical body",
			contentType: "application/problem+json",
			body:        `{"type":"https://api.apitool.dev/errors/refresh-token-reused","title":"Refresh token reuse detected","status":401,"detail":"Token has been rotated...","code":"AUTH_REFRESH_REUSED","request_id":"req_a3f4d2c1"}`,
			statusCode:  401,
			wantCode:    "AUTH_REFRESH_REUSED",
			wantStatus:  401,
			wantReqID:   "req_a3f4d2c1",
		},
		{
			name:        "content-type with charset suffix",
			contentType: "application/problem+json; charset=utf-8",
			body:        `{"code":"AUTH_REFRESH_EXPIRED","status":401,"title":"x"}`,
			statusCode:  401,
			wantCode:    "AUTH_REFRESH_EXPIRED",
			wantStatus:  401,
		},
		{
			name:        "fallback - request_id from header when missing in body",
			contentType: "application/problem+json",
			body:        `{"code":"AUTH_INVALID_REFRESH","title":"x"}`,
			statusCode:  401,
			extraHeader: map[string]string{"X-Request-Id": "req-from-header"},
			wantCode:    "AUTH_INVALID_REFRESH",
			wantStatus:  401,
			wantReqID:   "req-from-header",
		},
		{
			name:        "fallback - status from response when missing in body",
			contentType: "application/problem+json",
			body:        `{"code":"X","title":"x"}`,
			statusCode:  500,
			wantCode:    "X",
			wantStatus:  500,
		},
		{
			name:        "wrong content-type returns ErrNotProblem",
			contentType: "application/json",
			body:        `{"code":"X"}`,
			statusCode:  400,
			wantErr:     backend.ErrNotProblem,
		},
		{
			name:        "missing content-type returns ErrNotProblem",
			contentType: "",
			body:        `{"code":"X"}`,
			statusCode:  400,
			wantErr:     backend.ErrNotProblem,
		},
		{
			name:        "malformed JSON returns parse error",
			contentType: "application/problem+json",
			body:        `{not-json}`,
			statusCode:  401,
			wantErr:     errors.New("decode problem+json"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := makeResp(tc.statusCode, tc.contentType, tc.body, tc.extraHeader)
			pd, err := backend.DecodeProblem(resp)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				// For the ErrNotProblem sentinel, check the returned error wraps it.
				// For other expected errors, check the message string.
				if tc.wantErr == backend.ErrNotProblem {
					if !errors.Is(err, backend.ErrNotProblem) {
						t.Fatalf("got %v; want ErrNotProblem", err)
					}
				} else {
					if !strings.Contains(err.Error(), tc.wantErr.Error()) {
						t.Fatalf("got %v; want error containing %q", err, tc.wantErr.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pd.Code != tc.wantCode {
				t.Errorf("Code = %q; want %q", pd.Code, tc.wantCode)
			}
			if pd.Status != tc.wantStatus {
				t.Errorf("Status = %d; want %d", pd.Status, tc.wantStatus)
			}
			if tc.wantReqID != "" && pd.RequestID != tc.wantReqID {
				t.Errorf("RequestID = %q; want %q", pd.RequestID, tc.wantReqID)
			}
		})
	}
}

func TestProblemDetails_Error(t *testing.T) {
	tests := []struct {
		name   string
		pd     *backend.ProblemDetails
		wantIn []string
	}{
		{
			name:   "with RequestID",
			pd:     &backend.ProblemDetails{Title: "Refresh token reuse", Status: 401, Code: "AUTH_REFRESH_REUSED", RequestID: "req_abc"},
			wantIn: []string{"status=401", "code=AUTH_REFRESH_REUSED", "request_id=req_abc"},
		},
		{
			name:   "without RequestID",
			pd:     &backend.ProblemDetails{Title: "Token expired", Status: 401, Code: "AUTH_REFRESH_EXPIRED"},
			wantIn: []string{"status=401", "code=AUTH_REFRESH_EXPIRED"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.pd.Error()
			for _, want := range tc.wantIn {
				if !strings.Contains(msg, want) {
					t.Errorf("Error() = %q; missing %q", msg, want)
				}
			}
		})
	}
}
