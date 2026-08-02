// Package main: cmd/curlew/login.go
//
// Implements `curlew login` per the RFC 8628 device-code flow specified
// at docs/SPECIFICATION.md:8205-8222. Polls /api/v1/auth/device/start then
// /api/v1/auth/device/poll via internal/backend, prints the user code and
// verification URI for the user, optionally fires a browser at
// verification_uri_complete on TTY hosts when --no-browser is not set, and
// persists the resulting tokens (refresh via internal/backend.Storage,
// device_id via internal/backend/device).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/weiqigod/curlew/internal/appdir"
	"github.com/weiqigod/curlew/internal/backend"
	"github.com/weiqigod/curlew/internal/backend/device"
)

// Test seams: package-level so tests can substitute deterministic stubs.
var (
	// loginSleep is time.Sleep by default; tests inject a no-op to avoid
	// real delays. Every test that mutates it must pair with t.Cleanup.
	loginSleep = time.Sleep
	// loginOpenBrowser opens a URL in the OS browser. Tests inject a no-op.
	loginOpenBrowser = openBrowserDefault
	// loginIsTTY returns true when stdout is a character device. Tests
	// inject a stub via stubIsTTY or CURLEW_FORCE_NO_TTY=1.
	loginIsTTY = stdoutIsTTY
)

// loginCmdOut handles `curlew login [--no-browser]`.
// All user-facing prompts go to stdout; progress messages go to stderr.
// Exit codes:
//
//	0  — success
//	1  — internal error (URL malformed, FS error, code bug)
//	3  — network failure (ErrNetworkFailure)
//	4  — expired_token or access_denied — re-auth required
//	6  — 5xx without problem+json (ErrServerError)
func loginCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printLoginHelpTo(stdout)
		return 0
	}

	noBrowser := false
	for _, a := range args {
		switch a {
		case "--no-browser":
			noBrowser = true
		default:
			_, _ = fmt.Fprintf(stderr, "Unknown flag: %s\n", a)
			printLoginHelpTo(stderr)
			return 1
		}
	}

	backendURL := os.Getenv("CURLEW_BACKEND_URL")
	if backendURL == "" {
		backendURL = "https://api.apitool.dev"
	}
	client, err := backend.NewClient(backend.Options{BaseURL: backendURL})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start, err := client.StartDevice(ctx)
	if err != nil {
		return loginExitForError(err, stderr)
	}

	presentCode(stdout, start)
	if !noBrowser && loginIsTTY() {
		_ = loginOpenBrowser(start.VerificationURIComplete) // best-effort
	}
	_, _ = fmt.Fprintln(stderr, "Polling for confirmation...")

	poll, err := pollUntilDecision(ctx, client, start)
	if err != nil {
		return loginExitForError(err, stderr)
	}

	cfgDir, err := loginConfigDir()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := persistLogin(cfgDir, poll); err != nil {
		_, _ = fmt.Fprintf(stderr, "persist login: %v\n", err)
		return 1
	}

	email := emailFromLicenseJWT(poll.LicenseJWT)
	_, _ = fmt.Fprintf(stdout, "Authentication complete. Welcome, %s.\n", email)
	return 0
}

// presentCode prints the bare verification_uri and user_code to stdout per
// Open Decision #3. verification_uri_complete is reserved for browser auto-open.
func presentCode(stdout io.Writer, s backend.DeviceStart) {
	_, _ = fmt.Fprintf(stdout, "First, copy your one-time code: %s\n", s.UserCode)
	_, _ = fmt.Fprintf(stdout, "Then visit: %s\n", s.VerificationURI)
}

// pollUntilDecision implements the RFC 8628 §3.5 polling loop with
// authorization_pending and slow_down handling. Caps at start.ExpiresIn
// seconds to prevent a runaway loop on a broken backend response.
func pollUntilDecision(
	ctx context.Context,
	c *backend.Client,
	start backend.DeviceStart,
) (backend.DevicePollResult, error) {
	interval := time.Duration(start.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresIn := start.ExpiresIn
	if expiresIn <= 0 {
		// Defensive: treat missing/zero expires_in as immediate expiry.
		return backend.DevicePollResult{}, backend.ErrExpiredToken
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	for {
		if time.Now().After(deadline) {
			return backend.DevicePollResult{}, backend.ErrExpiredToken
		}
		loginSleep(interval)
		res, err := c.PollDevice(ctx, start.DeviceCode)
		switch {
		case err == nil:
			return res, nil
		case errors.Is(err, backend.ErrAuthorizationPending):
			continue
		case errors.Is(err, backend.ErrSlowDown):
			interval += 5 * time.Second // RFC 8628 §3.5
			continue
		default:
			return backend.DevicePollResult{}, err
		}
	}
}

// persistLogin saves the tokens via the existing storage layers.
// device_id → internal/backend/device (device.json, plaintext)
// refresh_token + access_token → internal/backend.Storage (keychain or encrypted file)
func persistLogin(cfgDir string, p backend.DevicePollResult) error {
	if err := device.Write(cfgDir, device.Record{
		DeviceID: p.DeviceID,
		IssuedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("persist device.json: %w", err)
	}

	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: cfgDir,
		DeviceID:  p.DeviceID,
	})
	if err != nil {
		return fmt.Errorf("init storage: %w", err)
	}
	if err := storage.SetRefreshToken(p.RefreshToken); err != nil {
		return fmt.Errorf("persist refresh token: %w", err)
	}
	if err := storage.SetAccessToken(p.AccessToken); err != nil {
		return fmt.Errorf("persist access token: %w", err)
	}
	return nil
}

// loginExitForError maps well-known error sentinels to exit codes.
func loginExitForError(err error, stderr io.Writer) int {
	switch {
	case errors.Is(err, backend.ErrExpiredToken):
		_, _ = fmt.Fprintln(stderr, "Code expired; run curlew login again")
		return 4
	case errors.Is(err, backend.ErrAccessDenied):
		_, _ = fmt.Fprintln(stderr, "Login was denied in the browser")
		return 4
	case errors.Is(err, backend.ErrNetworkFailure):
		_, _ = fmt.Fprintf(stderr, "network failure: %v\n", err)
		return 3
	case errors.Is(err, backend.ErrServerError):
		_, _ = fmt.Fprintf(stderr, "backend error: %v\n", err)
		return 6
	default:
		_, _ = fmt.Fprintf(stderr, "login failed: %v\n", err)
		return 1
	}
}

// loginConfigDir returns the directory for login state files.
// Honors CURLEW_CONFIG_DIR, falling back to ~/.config/curlew.
func loginConfigDir() (string, error) {
	return appdir.ResolveConfigDir()
}

// stdoutIsTTY returns true when stdout is a character device.
// CURLEW_FORCE_NO_TTY=1 forces false for testing.
func stdoutIsTTY() bool {
	if os.Getenv("CURLEW_FORCE_NO_TTY") == "1" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// openBrowserDefault opens a URL in the OS default browser via os/exec.
// Uses open (macOS), cmd /c start (Windows), or xdg-open (Linux/other).
// Fire-and-forget: the child process is not waited on.
func openBrowserDefault(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// emailFromLicenseJWT decodes the JWT payload without verification and
// returns the email claim. Used only for the human-friendly welcome message.
func emailFromLicenseJWT(jwt string) string {
	parts := strings.SplitN(jwt, ".", 3)
	if len(parts) != 3 {
		return "unknown"
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "unknown"
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Email == "" {
		return "unknown"
	}
	return claims.Email
}

// printLoginHelpTo writes usage for `curlew login` to w.
func printLoginHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: curlew login [--no-browser]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Authenticate via the device-code flow (RFC 8628).")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "When you run `curlew login`, the CLI:")
	_, _ = fmt.Fprintln(w, "  1. Requests a device code from the curlew backend.")
	_, _ = fmt.Fprintln(w, "  2. Prints a one-time user code and verification URL.")
	_, _ = fmt.Fprintln(w, "  3. Opens the URL in your browser (unless --no-browser is set or stdin is not a TTY).")
	_, _ = fmt.Fprintln(w, "  4. Polls the backend until you approve or the code expires.")
	_, _ = fmt.Fprintln(w, "  5. Securely persists the API/refresh tokens and device ID locally.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Options:")
	_, _ = fmt.Fprintln(w, "  --no-browser    Skip automatic browser launch; print the URL and code only.")
	_, _ = fmt.Fprintln(w, "                  Required for headless/CI environments.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Exit codes:")
	_, _ = fmt.Fprintln(w, "  0  Authentication successful.")
	_, _ = fmt.Fprintln(w, "  3  Network error — backend unreachable.")
	_, _ = fmt.Fprintln(w, "  4  Device code expired or access denied — run `curlew login` again.")
	_, _ = fmt.Fprintln(w, "  6  Backend returned 5xx — retry later.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Env vars:")
	_, _ = fmt.Fprintln(w, "  CURLEW_BACKEND_URL=url    Backend base URL (default: https://api.apitool.dev)")
	_, _ = fmt.Fprintln(w, "  CURLEW_CONFIG_DIR=path    Override ~/.config/curlew")
}
