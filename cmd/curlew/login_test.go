package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/backend"
)

// base64RawURLEncodeString returns the raw base64url encoding of s (no padding).
// Used by TestEmailFromLicenseJWT to craft test JWT payloads without importing base64.
func base64RawURLEncodeString(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// captureLoginRun calls loginCmdOut in-process and captures stdout/stderr.
// Uses writer injection for stdout/stderr; package-level stubs are reset via
// t.Cleanup so tests must run sequentially (do not call t.Parallel).
func captureLoginRun(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	exitCode = loginCmdOut(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), exitCode
}

// stubLoginSleep replaces loginSleep during a test, recording durations slept.
// Returns a cleanup function that restores the original.
func stubLoginSleep(t *testing.T) *[]time.Duration {
	t.Helper()
	var slept []time.Duration
	orig := loginSleep
	loginSleep = func(d time.Duration) { slept = append(slept, d) }
	t.Cleanup(func() { loginSleep = orig })
	return &slept
}

// stubNoBrowser replaces loginOpenBrowser with a no-op and tracks call count.
func stubNoBrowser(t *testing.T) *atomic.Int32 {
	t.Helper()
	var opened atomic.Int32
	orig := loginOpenBrowser
	loginOpenBrowser = func(_ string) error { opened.Add(1); return nil }
	t.Cleanup(func() { loginOpenBrowser = orig })
	return &opened
}

// stubIsTTY replaces loginIsTTY to return the given value.
func stubIsTTY(t *testing.T, val bool) {
	t.Helper()
	orig := loginIsTTY
	loginIsTTY = func() bool { return val }
	t.Cleanup(func() { loginIsTTY = orig })
}

// successPollResponse is the JSON returned by a successful poll.
const successPollBody = `{"license_jwt":"` + testLoginJWT + `","access_token":"a","refresh_token":"rt-aaaaaaaaaaaaa","device_id":"dev_abc"}`

// testLoginJWT is a stub JWT with email=smoke@example.com, tier=enterprise.
// The header+payload are real base64url JSON; the signature is fake (not
// verified in login path — we only parse the payload for the welcome message).
// Generated via base64url-encode of the JSON bytes.
const testLoginJWT = "eyJhbGciOiJSUzI1NiIsImtpZCI6ImFwaXRlc3QtMjAyNS0wMSIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJhcGl0ZXN0LWxpY2Vuc2Utc2VydmVyIiwiYXVkIjoiYXBpdGVzdC1jbGkiLCJzdWIiOiJ0ZXN0LXVzZXIiLCJlbWFpbCI6InNtb2tlQGV4YW1wbGUuY29tIiwidGllciI6ImVudGVycHJpc2UiLCJleHAiOjk5OTk5OTk5OTl9.fakesig"

// newDeviceCodeServer returns a test HTTP server implementing the device-code
// flow. pollBehaviors is a slice where each element controls one poll response:
//   - "pending" → 400 AUTH_DEVICE_AUTHORIZATION_PENDING
//   - "slow_down" → 400 AUTH_DEVICE_SLOW_DOWN
//   - "expired" → 400 AUTH_DEVICE_EXPIRED_TOKEN
//   - "access_denied" → 400 AUTH_DEVICE_ACCESS_DENIED
//   - "server_error" → 500 plain text (ErrServerError)
//   - "success" → 200 with successPollBody
func newDeviceCodeServer(t *testing.T, pollBehaviors []string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var pollCount atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"device_code":"d","user_code":"ABCD-EFGH","verification_uri":"https://app.apitool.dev/device","verification_uri_complete":"https://app.apitool.dev/device?user_code=ABCD-EFGH","expires_in":900,"interval":1}`)
	})
	mux.HandleFunc("/api/v1/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		n := int(pollCount.Add(1)) - 1 // 0-indexed
		behavior := "success"
		if n < len(pollBehaviors) {
			behavior = pollBehaviors[n]
		}
		switch behavior {
		case "pending":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"code":"AUTH_DEVICE_AUTHORIZATION_PENDING","status":400,"title":"pending"}`)
		case "slow_down":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"code":"AUTH_DEVICE_SLOW_DOWN","status":400,"title":"slow_down"}`)
		case "expired":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"code":"AUTH_DEVICE_EXPIRED_TOKEN","status":400,"title":"expired"}`)
		case "access_denied":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"code":"AUTH_DEVICE_ACCESS_DENIED","status":400,"title":"access_denied"}`)
		case "server_error":
			w.WriteHeader(500)
			_, _ = io.WriteString(w, "internal server error")
		default: // "success"
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, successPollBody)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &pollCount
}

// setupLoginEnv configures env vars so loginCmdOut uses a test server and
// temp config dir, not the real network or real ~/.config.
func setupLoginEnv(t *testing.T, srvURL string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CURLEW_BACKEND_URL", srvURL)
	t.Setenv("CURLEW_CONFIG_DIR", dir)
	t.Setenv("CURLEW_FORCE_NO_TTY", "1")
	return dir
}

// --- Test functions ---

func TestLogin_HappyPath_PrintsCodePersistsTokens(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"success"})
	dir := setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	stdout, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}

	if !strings.Contains(stdout, "First, copy your one-time code: ABCD-EFGH") {
		t.Errorf("stdout missing code line: %q", stdout)
	}
	if !strings.Contains(stdout, "Then visit: https://app.apitool.dev/device") {
		t.Errorf("stdout missing visit line: %q", stdout)
	}
	if !strings.Contains(stdout, "Authentication complete. Welcome, smoke@example.com.") {
		t.Errorf("stdout missing completion line with email: %q", stdout)
	}

	// Verify device.json was written.
	devicePath := filepath.Join(dir, "device.json")
	data, err := os.ReadFile(devicePath)
	if err != nil {
		t.Fatalf("device.json not written: %v", err)
	}
	var rec map[string]interface{}
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("device.json not valid JSON: %v", err)
	}
	if rec["device_id"] != "dev_abc" {
		t.Errorf("device_id = %q, want %q", rec["device_id"], "dev_abc")
	}
}

func TestLogin_NoBrowserFlag_SuppressesAutoOpen(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"success"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubIsTTY(t, true) // TTY so auto-open would fire without --no-browser
	opened := stubNoBrowser(t)

	_, _, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("unexpected exit code %d", rc)
	}
	if opened.Load() != 0 {
		t.Errorf("browser opened %d times, want 0 (--no-browser suppresses it)", opened.Load())
	}
}

func TestLogin_TTY_OpensBrowserAtVerificationComplete(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"success"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubIsTTY(t, true)
	opened := stubNoBrowser(t)
	// No --no-browser flag.
	t.Setenv("CURLEW_FORCE_NO_TTY", "") // clear so our stubIsTTY controls it

	_, _, rc := captureLoginRun(t /* no --no-browser */)
	if rc != 0 {
		t.Fatalf("unexpected exit code %d", rc)
	}
	if opened.Load() != 1 {
		t.Errorf("browser opened %d times, want 1", opened.Load())
	}
}

func TestLogin_NoTTY_SkipsBrowserEvenWithoutFlag(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"success"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubIsTTY(t, false)
	opened := stubNoBrowser(t)
	t.Setenv("CURLEW_FORCE_NO_TTY", "") // ensure our stub controls TTY

	_, _, rc := captureLoginRun(t /* no --no-browser */)
	if rc != 0 {
		t.Fatalf("unexpected exit code %d", rc)
	}
	if opened.Load() != 0 {
		t.Errorf("browser opened %d times, want 0 (no TTY)", opened.Load())
	}
}

func TestLogin_Polling_PendingThenSuccess(t *testing.T) {
	srv, pollCount := newDeviceCodeServer(t, []string{"pending", "pending", "success"})
	setupLoginEnv(t, srv.URL)
	slept := stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("rc=%d stderr=%q", rc, stderr)
	}
	if got := pollCount.Load(); got != 3 {
		t.Errorf("poll count = %d, want 3", got)
	}
	// Three polls: two pending (no sleep before first poll) + one success.
	// The loop sleeps BEFORE each poll, so 3 polls = 3 sleeps.
	if len(*slept) != 3 {
		t.Errorf("sleeps = %d (%v), want 3", len(*slept), *slept)
	}
}

func TestLogin_Polling_SlowDownAddsFiveSeconds(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"slow_down", "success"})
	setupLoginEnv(t, srv.URL)
	slept := stubLoginSleep(t)
	stubNoBrowser(t)

	_, _, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("unexpected exit code %d", rc)
	}
	// First sleep is base interval (1s from server). After slow_down, interval += 5s.
	// 2 polls = 2 sleeps.
	if len(*slept) != 2 {
		t.Fatalf("sleeps = %d (%v), want 2", len(*slept), *slept)
	}
	if (*slept)[0] != time.Second {
		t.Errorf("first sleep = %v, want 1s", (*slept)[0])
	}
	if (*slept)[1] != 6*time.Second {
		t.Errorf("second sleep after slow_down = %v, want 6s (1s+5s)", (*slept)[1])
	}
}

func TestLogin_Polling_ExpiredToken_ExitFour(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"expired"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 4 {
		t.Errorf("rc=%d, want 4 (expired_token); stderr=%q", rc, stderr)
	}
	if !strings.Contains(stderr, "Code expired") {
		t.Errorf("stderr missing 'Code expired': %q", stderr)
	}
}

func TestLogin_PersistsRefreshAndDevice(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"success"})
	dir := setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, _, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("unexpected exit code %d", rc)
	}

	// 1. device.json must exist with correct device_id.
	devicePath := filepath.Join(dir, "device.json")
	deviceData, err := os.ReadFile(devicePath)
	if err != nil {
		t.Fatalf("device.json not written: %v", err)
	}
	var deviceRec map[string]interface{}
	if err := json.Unmarshal(deviceData, &deviceRec); err != nil {
		t.Fatalf("device.json not valid JSON: %v", err)
	}
	if deviceRec["device_id"] != "dev_abc" {
		t.Errorf("device_id = %v, want dev_abc", deviceRec["device_id"])
	}

	// 2. Refresh token must be persisted via internal/backend.Storage.
	// Use the same options as persistLogin (no ForceFile) so we read from
	// whichever backend (keychain or encrypted file) was actually written.
	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: dir,
		DeviceID:  "dev_abc",
	})
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	gotToken, err := storage.GetRefreshToken()
	if err != nil {
		t.Fatalf("GetRefreshToken: %v", err)
	}
	if gotToken != "rt-aaaaaaaaaaaaa" {
		t.Errorf("refresh token = %q, want %q", gotToken, "rt-aaaaaaaaaaaaa")
	}
	gotAccessToken, err := storage.GetAccessToken()
	if err != nil {
		t.Fatalf("GetAccessToken: %v", err)
	}
	if gotAccessToken != "a" {
		t.Errorf("access token = %q, want %q", gotAccessToken, "a")
	}
}

func TestLogin_Help_DocumentsNoBrowser(t *testing.T) {
	stdout, _, rc := captureLoginRun(t, "--help")
	if rc != 0 {
		t.Errorf("--help exit code = %d, want 0", rc)
	}
	if !strings.Contains(stdout, "--no-browser") {
		t.Errorf("help text missing --no-browser: %q", stdout)
	}
	if !strings.Contains(strings.ToLower(stdout), "device") {
		t.Errorf("help text missing device-code description: %q", stdout)
	}
}

func TestLogin_NetworkFailure_ExitThree(t *testing.T) {
	setupLoginEnv(t, "http://127.0.0.1:0") // port 0 = nothing listening
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 3 {
		t.Errorf("rc=%d, want 3 (network failure); stderr=%q", rc, stderr)
	}
}

func TestLogin_Polling_AccessDenied_ExitFour(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"access_denied"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 4 {
		t.Errorf("rc=%d, want 4 (access_denied); stderr=%q", rc, stderr)
	}
	if !strings.Contains(stderr, "denied") {
		t.Errorf("stderr missing denial message: %q", stderr)
	}
}

func TestLogin_ServerError_ExitSix(t *testing.T) {
	srv, _ := newDeviceCodeServer(t, []string{"server_error"})
	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 6 {
		t.Errorf("rc=%d, want 6 (server error); stderr=%q", rc, stderr)
	}
	if !strings.Contains(stderr, "backend error") {
		t.Errorf("stderr missing 'backend error': %q", stderr)
	}
}

func TestLogin_ExpiresInZero_ExitFour(t *testing.T) {
	// Server returns expires_in: 0 — should be treated as immediate expiry (exit 4).
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"device_code":"d","user_code":"ABCD-EFGH","verification_uri":"https://app.apitool.dev/device","verification_uri_complete":"https://app.apitool.dev/device?user_code=ABCD-EFGH","expires_in":0,"interval":1}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	_, stderr, rc := captureLoginRun(t, "--no-browser")
	if rc != 4 {
		t.Errorf("rc=%d, want 4 (expires_in=0 immediate expiry); stderr=%q", rc, stderr)
	}
	if !strings.Contains(stderr, "Code expired") {
		t.Errorf("stderr missing 'Code expired': %q", stderr)
	}
}

func TestEmailFromLicenseJWT(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid JWT with email claim",
			input: testLoginJWT,
			want:  "smoke@example.com",
		},
		{
			name:  "not a JWT (no dots)",
			input: "notajwt",
			want:  "unknown",
		},
		{
			name:  "invalid base64 in payload",
			input: "header.!!INVALID!!.sig",
			want:  "unknown",
		},
		{
			name:  "valid base64 but not JSON",
			input: "header." + base64RawURLEncodeString("{not-json}") + ".sig",
			want:  "unknown",
		},
		{
			name:  "valid JSON but missing email claim",
			input: "header." + base64RawURLEncodeString(`{"sub":"user","tier":"pro"}`) + ".sig",
			want:  "unknown",
		},
		{
			name:  "empty string",
			input: "",
			want:  "unknown",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := emailFromLicenseJWT(tc.input)
			if got != tc.want {
				t.Errorf("emailFromLicenseJWT(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPersistLogin_DeviceWriteError(t *testing.T) {
	if os.Getenv("CI") != "" || os.Getuid() == 0 {
		t.Skip("skipping: root or CI may bypass permission checks")
	}
	// Make the config dir read-only so device.Write fails.
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	poll := backend.DevicePollResult{
		LicenseJWT:   testLoginJWT,
		AccessToken:  "a",
		RefreshToken: "rt-test",
		DeviceID:     "dev_test",
	}
	err := persistLogin(dir, poll)
	if err == nil {
		t.Fatal("persistLogin on read-only dir: want error, got nil")
	}
	if !strings.Contains(err.Error(), "persist device.json") {
		t.Errorf("error should mention persist device.json: %v", err)
	}
}

func TestLogin_TwoConsecutiveLogins_LeavesOldFamily(t *testing.T) {
	var revokeCalled atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/device/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"device_code":"d","user_code":"ABCD-EFGH","verification_uri":"https://app.apitool.dev/device","verification_uri_complete":"https://app.apitool.dev/device?user_code=ABCD-EFGH","expires_in":900,"interval":1}`)
	})
	mux.HandleFunc("/api/v1/auth/device/poll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, successPollBody)
	})
	mux.HandleFunc("/api/v1/auth/revoke", func(w http.ResponseWriter, _ *http.Request) {
		revokeCalled.Add(1)
		w.WriteHeader(204)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	setupLoginEnv(t, srv.URL)
	stubLoginSleep(t)
	stubNoBrowser(t)

	// First login.
	_, _, rc := captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("first login rc=%d", rc)
	}
	// Second login.
	_, _, rc = captureLoginRun(t, "--no-browser")
	if rc != 0 {
		t.Fatalf("second login rc=%d", rc)
	}
	// The CLI must NOT call /revoke implicitly.
	if n := revokeCalled.Load(); n != 0 {
		t.Errorf("/revoke called %d times, want 0 (implicit revocation is forbidden)", n)
	}
}
