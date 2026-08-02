package backend_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
)

// jsonHandler returns a handler that responds with the given status code and
// JSON body (Content-Type: application/json).
func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// problemHandler returns a handler that responds with the given status code and
// an application/problem+json body with the given code.
func problemHandler(status int, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"type":"about:blank","title":"err","status":`+
			itoa(status)+`,"code":"`+code+`"}`)
	}
}

// plainHandler returns a handler that responds with the given status code and
// a text/plain body.
func plainHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// expectAuthHeader returns a handler that verifies the Authorization header
// equals wantAuth, then responds 200 with a valid vault-config body.
func expectAuthHeader(wantAuth string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if got != wantAuth {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"error":"wrong auth: `+got+`"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"template":"team_secrets: {}\n","version":1}`)
	}
}

// itoa converts an int to a string.
func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestGetTeamVault(t *testing.T) {
	tests := []struct {
		name        string
		orgID       string
		accessToken string
		handler     http.HandlerFunc
		wantErr     error  // sentinel matched with errors.Is; nil = expect no error
		wantTpl     string // expected Template field; "" when wantErr != nil
		wantVer     int64
	}{
		{
			name:    "success returns parsed body",
			orgID:   "org_abc",
			handler: jsonHandler(200, `{"template":"team_secrets:\n  vault_configs: {}\n","version":7}`),
			wantTpl: "team_secrets:\n  vault_configs: {}\n",
			wantVer: 7,
		},
		{
			name:    "404 not found becomes ErrTeamVaultNotFound",
			orgID:   "org_x",
			handler: problemHandler(404, "vault_config_not_found"),
			wantErr: backend.ErrTeamVaultNotFound,
		},
		{
			name:    "402 payment required surfaces problem details",
			orgID:   "org_y",
			handler: problemHandler(402, "tier_gate_team"),
			wantErr: &backend.ProblemDetails{},
		},
		{
			name:    "500 server error surfaces ErrServerError",
			orgID:   "org_q",
			handler: plainHandler(500, "boom"),
			wantErr: backend.ErrServerError,
		},
		{
			name:    "empty orgId returns ErrOrgIDRequired without calling handler",
			orgID:   "",
			handler: nil, // must not be called
			wantErr: backend.ErrOrgIDRequired,
		},
		{
			name:    "invalid orgId format rejects path traversal",
			orgID:   "org/evil",
			handler: nil, // must not be called
			wantErr: errors.New("invalid orgId"),
		},
		{
			name:        "sends Bearer token header",
			orgID:       "org_a",
			accessToken: "at-foo",
			handler:     expectAuthHeader("Bearer at-foo"),
			wantTpl:     "team_secrets: {}\n",
			wantVer:     1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var srv *httptest.Server
			if tc.handler != nil {
				srv = httptest.NewServer(tc.handler)
				defer srv.Close()
			} else {
				// Use a non-routable address so any accidental call fails fast.
				srv = &httptest.Server{URL: "http://127.0.0.1:1"}
			}

			c, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
			if err != nil {
				t.Fatal(err)
			}

			got, gotErr := c.GetTeamVault(context.Background(), tc.orgID, tc.accessToken)

			if tc.wantErr != nil {
				if gotErr == nil {
					t.Fatalf("expected error, got nil (result: %+v)", got)
				}
				// Use errors.Is for sentinels; errors.As for type checks.
				if errors.Is(gotErr, tc.wantErr) {
					return // sentinel matched
				}
				// Type-based check for *ProblemDetails.
				if _, isPD := tc.wantErr.(*backend.ProblemDetails); isPD {
					var pd *backend.ProblemDetails
					if !errors.As(gotErr, &pd) {
						t.Fatalf("expected *ProblemDetails, got %T: %v", gotErr, gotErr)
					}
					return
				}
				// For non-sentinel errors (e.g. invalid orgId format), check the
				// error message contains the expected substring.
				if gotErr.Error() != tc.wantErr.Error() && !strings.Contains(gotErr.Error(), tc.wantErr.Error()) {
					t.Fatalf("error %q does not contain expected %q", gotErr.Error(), tc.wantErr.Error())
				}
				return
			}

			if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if got.Template != tc.wantTpl {
				t.Errorf("Template = %q, want %q", got.Template, tc.wantTpl)
			}
			if got.Version != tc.wantVer {
				t.Errorf("Version = %d, want %d", got.Version, tc.wantVer)
			}
		})
	}
}

func TestGetTeamVault_PathContainsOrgID(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"template":"t","version":1}`)
	}))
	defer srv.Close()

	c, _ := backend.NewClient(backend.Options{BaseURL: srv.URL})
	_, err := c.GetTeamVault(context.Background(), "org-xyz", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPath := "/api/v1/organizations/org-xyz/vault-config"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %q", gotPath, wantPath)
	}
}
