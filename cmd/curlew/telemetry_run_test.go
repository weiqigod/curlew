package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// runWithTelemetry runs `curlew run <args>` with a configured telemetry
// endpoint and returns stdout, stderr, exit.
func runWithTelemetry(t *testing.T, configDir, endpoint string, runArgs ...string) (string, string, int) {
	t.Helper()
	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	if endpoint != "" {
		t.Setenv("CURLEW_TELEMETRY_ENDPOINT", endpoint)
	} else {
		t.Setenv("CURLEW_TELEMETRY_ENDPOINT", "")
	}
	var out, errOut bytes.Buffer
	args := append([]string{"run"}, runArgs...)
	exit := runWithWriters(args, &out, &errOut)
	return out.String(), errOut.String(), exit
}

func TestRunFiresTelemetryWhenEnabled(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	// Enable telemetry
	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", srv.URL)
	var stdoutBuf, stderrBuf bytes.Buffer
	_ = runWithWriters([]string{"telemetry", "enable"}, &stdoutBuf, &stderrBuf)
	installID := extractInstallID(stdoutBuf.String())
	if installID == "" {
		t.Fatalf("could not enable telemetry, stdout=%q", stdoutBuf.String())
	}

	// Run a minimal collection. emitTelemetryRunCompleted is called synchronously
	// in the deferred end-of-run block, so by the time runWithTelemetry returns
	// the POST has already been attempted.
	_, _, _ = runWithTelemetry(t, configDir, srv.URL, "testdata/minimal.yaml")

	mu.Lock()
	defer mu.Unlock()
	// Find a run.completed event
	var found map[string]any
	for _, b := range bodies {
		if et, ok := b["event_type"].(string); ok && et == "run.completed" {
			found = b
			break
		}
	}
	if found == nil {
		t.Errorf("no run.completed event found in %d bodies: %v", len(bodies), bodies)
		return
	}
	if found["install_id"] != installID {
		t.Errorf("event install_id = %v, want %s", found["install_id"], installID)
	}
	payload, ok := found["event_payload"].(map[string]any)
	if !ok {
		t.Fatalf("event_payload is not a map: %T", found["event_payload"])
	}
	if payload["session_id"] == nil {
		t.Error("event_payload.session_id is nil")
	}
	// session_id must be a UUID and differ from install_id
	sessionID, _ := payload["session_id"].(string)
	if !uuidRE.MatchString(sessionID) {
		t.Errorf("event_payload.session_id %q is not a UUID v4", sessionID)
	}
	if sessionID == installID {
		t.Error("session_id must differ from install_id")
	}
}

func TestRunDoesNotFireWhenDisabled(t *testing.T) {
	var mu sync.Mutex
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configDir := t.TempDir()
	// telemetry never enabled; emitTelemetryRunCompleted is synchronous, so the
	// request count is final when runWithTelemetry returns.
	_, _, _ = runWithTelemetry(t, configDir, srv.URL, "testdata/minimal.yaml")

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 0 {
		t.Errorf("expected 0 telemetry requests when disabled, got %d", requestCount)
	}
}

func TestRunDoesNotBlockOnSlowTelemetryEndpoint(t *testing.T) {
	// Use a slow server; run must exit well within the 2s telemetry deadline.
	testDone, testDoneCancel := func() (chan struct{}, func()) {
		ch := make(chan struct{})
		return ch, sync.OnceFunc(func() { close(ch) })
	}()
	t.Cleanup(testDoneCancel)
	slowSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-testDone:
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
	}))
	t.Cleanup(func() { testDoneCancel(); slowSrv.Close() })

	configDir := t.TempDir()
	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", slowSrv.URL)
	var stdoutBuf, stderrBuf bytes.Buffer
	_ = runWithWriters([]string{"telemetry", "enable"}, &stdoutBuf, &stderrBuf)

	start := time.Now()
	_, _, code := runWithTelemetry(t, configDir, slowSrv.URL, "testdata/minimal.yaml")
	elapsed := time.Since(start)
	// Must exit within 5 seconds (2s telemetry timeout + margin)
	if elapsed > 5*time.Second {
		t.Errorf("run took %v, want < 5s (slow telemetry should not block)", elapsed)
	}
	// Exit code must be the collection's own exit code (0 or 1), not a telemetry error
	if code > 2 {
		t.Errorf("exit code = %d; slow telemetry should not cause high exit codes", code)
	}
}

func TestRunDoesNotFailOnTelemetryError(t *testing.T) {
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	configDir := t.TempDir()
	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", errSrv.URL)
	var stdoutBuf, stderrBuf bytes.Buffer
	_ = runWithWriters([]string{"telemetry", "enable"}, &stdoutBuf, &stderrBuf)

	var out, errOut bytes.Buffer
	exit := runWithWriters([]string{"run", "testdata/minimal.yaml"}, &out, &errOut)
	// Exit must be normal (0 or 1 from collection); no telemetry noise in stdout/stderr
	if exit > 2 {
		t.Errorf("exit = %d; telemetry 500 should not affect run exit code", exit)
	}
	if bytes.Contains(out.Bytes(), []byte("telemetry")) {
		t.Errorf("stdout contains telemetry noise: %s", out.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("telemetry")) {
		t.Errorf("stderr contains telemetry noise: %s", errOut.String())
	}
}
