package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

	// Run a minimal collection. emitTelemetryRunCompleted is called synchronously
	// in the deferred end-of-run block, so the event is on disk by the time
	// runWithTelemetry returns.
	_, _, _ = runWithTelemetry(t, configDir, events, "testdata/minimal.yaml")

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
	_, _, _ = runWithTelemetry(t, configDir, events, "testdata/minimal.yaml")

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

	var out, errOut bytes.Buffer
	exit := runWithWriters([]string{"run", "testdata/minimal.yaml"}, &out, &errOut)
	// Exit must be normal (0 or 1 from collection); no telemetry noise anywhere.
	if exit > 2 {
		t.Errorf("exit = %d; an unwritable telemetry sink must not affect run exit code", exit)
	}
	if bytes.Contains(out.Bytes(), []byte("telemetry")) {
		t.Errorf("stdout contains telemetry noise: %s", out.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("telemetry")) {
		t.Errorf("stderr contains telemetry noise: %s", errOut.String())
	}
}
