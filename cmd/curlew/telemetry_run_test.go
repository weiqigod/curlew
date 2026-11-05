package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// runWithTelemetry runs `curlew run <args>` with a configured telemetry events
// file and returns stdout, stderr, exit.
func runWithTelemetry(t *testing.T, configDir, eventsFile string, runArgs ...string) (string, string, int) {
	t.Helper()
	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_FILE", eventsFile)
	var out, errOut bytes.Buffer
	args := append([]string{"run"}, runArgs...)
	exit := runWithWriters(args, &out, &errOut)
	return out.String(), errOut.String(), exit
}

// collectedEvents parses the NDJSON events file. A missing file means no
// events were ever recorded.
func collectedEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	var out []map[string]any
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("event line %q is not valid JSON: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

func telemetryCollection(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	collection := fmt.Sprintf(`name: Telemetry Test
requests:
  - name: loopback
    request:
      method: GET
      url: %q
    assertions:
      status: 200
`, server.URL)
	return writeCollection(t, t.TempDir(), "telemetry.yaml", collection), &requests
}

func requireSuccessfulTelemetryRun(t *testing.T, configDir, eventsFile string) {
	t.Helper()
	collection, requests := telemetryCollection(t)
	_, stderr, exit := runWithTelemetry(t, configDir, eventsFile, collection)
	if exit != 0 {
		t.Fatalf("run exit = %d, want 0; stderr=%q", exit, stderr)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("loopback requests = %d, want 1", got)
	}
}

func TestRunRecordsTelemetryWhenEnabled(t *testing.T) {
	configDir := t.TempDir()
	events := filepath.Join(t.TempDir(), "telemetry.ndjson")

	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_FILE", events)
	var stdoutBuf, stderrBuf bytes.Buffer
	_ = runWithWriters([]string{"telemetry", "enable"}, &stdoutBuf, &stderrBuf)
	installID := extractInstallID(stdoutBuf.String())
	if installID == "" {
		t.Fatalf("could not enable telemetry, stdout=%q", stdoutBuf.String())
	}

	// Run a loopback collection. emitTelemetryRunCompleted is called synchronously
	// in the deferred end-of-run block, so the event is on disk by the time
	// runWithTelemetry returns.
	requireSuccessfulTelemetryRun(t, configDir, events)

	var found map[string]any
	recs := collectedEvents(t, events)
	for _, rec := range recs {
		if et, ok := rec["event_type"].(string); ok && et == "run.completed" {
			found = rec
			break
		}
	}
	if found == nil {
		t.Fatalf("no run.completed event found in %d records: %v", len(recs), recs)
	}
	if found["install_id"] != installID {
		t.Errorf("event install_id = %v, want %s", found["install_id"], installID)
	}
	payload, ok := found["event_payload"].(map[string]any)
	if !ok {
		t.Fatalf("event_payload is not a map: %T", found["event_payload"])
	}
	if payload["session_id"] == nil {
		t.Fatal("event_payload.session_id is nil")
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

func TestRunWritesNothingWhenDisabled(t *testing.T) {
	configDir := t.TempDir()
	events := filepath.Join(t.TempDir(), "telemetry.ndjson")

	// telemetry never enabled
	requireSuccessfulTelemetryRun(t, configDir, events)

	if recs := collectedEvents(t, events); len(recs) != 0 {
		t.Errorf("expected 0 telemetry events when disabled, got %d: %v", len(recs), recs)
	}
}

// A telemetry sink that cannot be written must never affect the run's exit
// code or leak noise into the user-facing output.
func TestRunDoesNotFailOnTelemetryError(t *testing.T) {
	configDir := t.TempDir()
	// Parent is a regular file, so every append fails.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	events := filepath.Join(blocker, "telemetry.ndjson")

	t.Setenv("CURLEW_CONFIG_DIR", configDir)
	t.Setenv("CURLEW_TELEMETRY_FILE", events)
	var stdoutBuf, stderrBuf bytes.Buffer
	_ = runWithWriters([]string{"telemetry", "enable"}, &stdoutBuf, &stderrBuf)

	collection, requests := telemetryCollection(t)
	var out, errOut bytes.Buffer
	exit := runWithWriters([]string{"run", collection}, &out, &errOut)
	if exit != 0 {
		t.Errorf("exit = %d, want 0; an unwritable telemetry sink must not affect run exit code", exit)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("loopback requests = %d, want 1", got)
	}
	if bytes.Contains(out.Bytes(), []byte("telemetry")) {
		t.Errorf("stdout contains telemetry noise: %s", out.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("telemetry")) {
		t.Errorf("stderr contains telemetry noise: %s", errOut.String())
	}
}
