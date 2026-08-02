package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/peterlindqvist/apitest/internal/backend"
	"github.com/peterlindqvist/apitest/internal/backend/device"
)

// internalCmdOut dispatches `apitest internal <subcommand>`. Hidden behind
// APITEST_INTERNAL=1; not listed in the top-level help or usageSynopses.
func internalCmdOut(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "Usage: apitest internal <subcommand>")
		_, _ = fmt.Fprintln(stderr, "Subcommands:")
		_, _ = fmt.Fprintln(stderr, "  backend-probe    Self-test the internal/backend client")
		_, _ = fmt.Fprintln(stderr, "  seed-login       Seed isolated E2E login state from environment")
		return 1
	}
	switch args[0] {
	case "backend-probe":
		return backendProbeCmdOut(args[1:], stdout, stderr)
	case "seed-login":
		return seedLoginCmdOut(stdout, stderr)
	default:
		_, _ = fmt.Fprintf(stderr, "Unknown internal subcommand: %s\n", args[0])
		return 1
	}
}

// seedLoginCmdOut writes isolated login state for local E2E scripts. It is
// reachable only through the APITEST_INTERNAL=1 command gate and deliberately
// reads secrets from environment variables instead of command-line arguments.
func seedLoginCmdOut(stdout, stderr io.Writer) int {
	cfgDir := os.Getenv("APITEST_CONFIG_DIR")
	deviceID := os.Getenv("APITEST_INTERNAL_DEVICE_ID")
	refreshToken := os.Getenv("APITEST_INTERNAL_REFRESH_TOKEN")
	accessToken := os.Getenv("APITEST_INTERNAL_ACCESS_TOKEN")
	if cfgDir == "" || deviceID == "" || refreshToken == "" || accessToken == "" {
		_, _ = fmt.Fprintln(stderr,
			"seed-login requires APITEST_CONFIG_DIR, APITEST_INTERNAL_DEVICE_ID, "+
				"APITEST_INTERNAL_REFRESH_TOKEN, and APITEST_INTERNAL_ACCESS_TOKEN")
		return 1
	}

	if err := device.Write(cfgDir, device.Record{
		DeviceID: deviceID,
		IssuedAt: time.Now().UTC(),
	}); err != nil {
		_, _ = fmt.Fprintf(stderr, "write device state: %v\n", err)
		return 1
	}

	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: cfgDir,
		DeviceID:  deviceID,
		ForceFile: true,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "init token storage: %v\n", err)
		return 1
	}
	if err := storage.SetRefreshToken(refreshToken); err != nil {
		_, _ = fmt.Fprintf(stderr, "write refresh token: %v\n", err)
		return 1
	}
	if err := storage.SetAccessToken(accessToken); err != nil {
		_, _ = fmt.Fprintf(stderr, "write access token: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintln(stdout, "OK: isolated login state seeded")
	return 0
}

// backendProbeCmdOut runs the self-test described in M14-004's observable
// section. Stands up an httptest.Server in-process, exercises the client
// against it, asserts ProblemDetails decoding, and reports the storage
// backend selected.
func backendProbeCmdOut(args []string, stdout, stderr io.Writer) int {
	var (
		baseURL  string
		selfTest bool
	)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--base":
			i++
			if i >= len(args) {
				_, _ = fmt.Fprintln(stderr, "--base requires a value")
				return 1
			}
			baseURL = args[i]
		case "--self-test":
			selfTest = true
		default:
			_, _ = fmt.Fprintf(stderr, "Unknown flag: %s\n", args[i])
			return 1
		}
	}
	if !selfTest {
		_, _ = fmt.Fprintln(stderr, "backend-probe currently requires --self-test")
		return 1
	}
	_ = baseURL // accepted for forward-compat per Decision #5 in the plan

	// Stand up the fake backend.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/v1/auth/refresh-reused":
			w.Header().Set("Content-Type", "application/problem+json")
			w.Header().Set("X-Request-Id", "req_probe_1")
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"type":"https://x/refresh-token-reused","title":"Refresh token reuse detected","status":401,"detail":"x","code":"AUTH_REFRESH_REUSED","request_id":"req_probe_1"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	// Step 1: client built — GET /healthz
	var hc map[string]bool
	if err := client.GetJSON(context.Background(), "/healthz", "", &hc); err != nil {
		_, _ = fmt.Fprintf(stderr, "client probe failed: %v\n", err)
		return 1
	}

	// Step 2: lock OK — acquire and release the flock under a temp dir
	cfgDir, err := os.MkdirTemp("", "apitest-probe-")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() { _ = os.RemoveAll(cfgDir) }()

	storage, err := backend.NewStorage(backend.StorageOptions{
		ConfigDir: cfgDir, DeviceID: "probe-dev", ForceFile: true,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if err := storage.SetRefreshToken("rt-probe-aaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	// Exercise the lock path by running a single RefreshTokens call against a dead endpoint;
	// the token is already in storage, so this verifies flock create+acquire+release.
	// We use a minimal in-process client pointing at our fake server for the refresh path.
	refreshSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"license_jwt":"L","access_token":"A","refresh_token":"rt-probe-refreshed"}`))
	}))
	defer refreshSrv.Close()

	refreshClient, err := backend.NewClient(backend.Options{BaseURL: refreshSrv.URL})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if _, err := backend.RefreshTokens(context.Background(), refreshClient, storage, cfgDir, "probe-dev"); err != nil {
		_, _ = fmt.Fprintf(stderr, "lock probe failed: %v\n", err)
		return 1
	}

	// Step 3: keychain probe — without ForceFile, what does the live system look like?
	sysStore, _ := backend.NewStorage(backend.StorageOptions{
		ConfigDir: filepath.Join(cfgDir, "sys"), DeviceID: "probe-dev",
	})
	keyAvail := sysStore != nil && sysStore.KeychainAvailable()

	// Step 4: rfc7807 mapping — POST against the reused-token fixture
	var ignored map[string]any
	err = client.PostJSON(context.Background(), "/api/v1/auth/refresh-reused", "",
		map[string]string{"refresh_token": "x", "device_id": "y"}, &ignored)
	var pd *backend.ProblemDetails
	if !errors.As(err, &pd) || pd.Code != "AUTH_REFRESH_REUSED" {
		_, _ = fmt.Fprintf(stderr, "rfc7807 mapping failed: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(stdout, "OK: client built; lock OK; keychain available=%t; rfc7807 mapping OK\n", keyAvail)
	return 0
}
