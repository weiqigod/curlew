package backend_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/backend"
)

// problemResponse writes an application/problem+json response with the given
// status and problem code.
func problemResponse(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"type":   "https://api.apitool.dev/errors/" + code,
		"title":  title,
		"status": status,
		"code":   code,
	})
}

// newRefreshStub returns a test server that responds with a problem+json whose
// code is the given problem code. Used to test sentinel mapping.
func newRefreshStub(t *testing.T, status int, code, title string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/refresh" {
			http.NotFound(w, r)
			return
		}
		problemResponse(w, status, code, title)
	}))
}

func TestRefreshTokens_AuthRefreshExpired_ReturnsSentinel(t *testing.T) {
	srv := newRefreshStub(t, 401, "AUTH_REFRESH_EXPIRED", "Refresh token expired")
	defer srv.Close()

	cfgDir := t.TempDir()
	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1", ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("rt-old"); err != nil {
		t.Fatal(err)
	}

	_, err = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	if !errors.Is(err, backend.ErrRefreshExpired) {
		t.Errorf("got %v; want errors.Is(ErrRefreshExpired)", err)
	}
	// The original ProblemDetails must still be accessible via errors.As.
	var pd *backend.ProblemDetails
	if !errors.As(err, &pd) {
		t.Errorf("errors.As(*ProblemDetails) = false; want true")
	} else if pd.Code != "AUTH_REFRESH_EXPIRED" {
		t.Errorf("ProblemDetails.Code = %q; want AUTH_REFRESH_EXPIRED", pd.Code)
	}
	// Error() must return a non-empty string that includes the sentinel text.
	if msg := err.Error(); msg == "" {
		t.Error("err.Error() returned empty string")
	} else if !strings.Contains(msg, "refresh token expired") {
		t.Errorf("err.Error() = %q; want it to contain sentinel text", msg)
	}
}

func TestRefreshTokens_AuthRefreshReused_ReturnsSentinel(t *testing.T) {
	srv := newRefreshStub(t, 401, "AUTH_REFRESH_REUSED", "Refresh token reused — family revoked")
	defer srv.Close()

	cfgDir := t.TempDir()
	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1", ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("rt-old"); err != nil {
		t.Fatal(err)
	}

	_, err = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	if !errors.Is(err, backend.ErrRefreshReused) {
		t.Errorf("got %v; want errors.Is(ErrRefreshReused)", err)
	}
}

func TestRefreshTokens_AuthDeviceMismatch_ReturnsSentinel(t *testing.T) {
	srv := newRefreshStub(t, 403, "AUTH_DEVICE_MISMATCH", "Device mismatch")
	defer srv.Close()

	cfgDir := t.TempDir()
	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1", ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("rt-old"); err != nil {
		t.Fatal(err)
	}

	_, err = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	if !errors.Is(err, backend.ErrDeviceMismatch) {
		t.Errorf("got %v; want errors.Is(ErrDeviceMismatch)", err)
	}
}

func TestRefreshTokens_UnknownAuthCode_BubblesProblemDetails(t *testing.T) {
	srv := newRefreshStub(t, 401, "AUTH_UNKNOWN_CODE", "Unknown error")
	defer srv.Close()

	cfgDir := t.TempDir()
	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1", ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("rt-old"); err != nil {
		t.Fatal(err)
	}

	_, err = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	// No sentinel — must NOT match any known sentinel.
	if errors.Is(err, backend.ErrRefreshExpired) {
		t.Error("got ErrRefreshExpired; want no sentinel")
	}
	if errors.Is(err, backend.ErrRefreshReused) {
		t.Error("got ErrRefreshReused; want no sentinel")
	}
	if errors.Is(err, backend.ErrDeviceMismatch) {
		t.Error("got ErrDeviceMismatch; want no sentinel")
	}
	// ProblemDetails must still be accessible.
	var pd *backend.ProblemDetails
	if !errors.As(err, &pd) {
		t.Errorf("errors.As(*ProblemDetails) = false; want true (got %T: %v)", err, err)
	} else if pd.Code != "AUTH_UNKNOWN_CODE" {
		t.Errorf("ProblemDetails.Code = %q; want AUTH_UNKNOWN_CODE", pd.Code)
	}
}
