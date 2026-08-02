package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// telemetryRoundtrip runs `apitest telemetry <args>` with APITEST_CONFIG_DIR
// set to the provided tmpDir and optional endpoint, returning stdout, stderr, exit.
func telemetryRoundtrip(t *testing.T, tmpDir, endpoint string, args ...string) (stdout, stderr string, exit int) {
	t.Helper()
	if endpoint != "" {
		t.Setenv("APITEST_TELEMETRY_ENDPOINT", endpoint)
	} else {
		t.Setenv("APITEST_TELEMETRY_ENDPOINT", "")
	}
	t.Setenv("APITEST_CONFIG_DIR", tmpDir)
	var out, errOut bytes.Buffer
	exit = runWithWriters(append([]string{"telemetry"}, args...), &out, &errOut)
	return out.String(), errOut.String(), exit
}

func TestTelemetryCmd_StatusOnFreshDir(t *testing.T) {
	dir := t.TempDir()
	stdout, _, exit := telemetryRoundtrip(t, dir, "", "status")
	if exit != 1 {
		t.Errorf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stdout, "no install_id") {
		t.Errorf("stdout %q does not mention no install_id", stdout)
	}
}

func TestTelemetryCmd_EnableCreatesFiles(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	stdout, _, exit := telemetryRoundtrip(t, dir, srv.URL, "enable")
	if exit != 0 {
		t.Errorf("exit = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "install_id=") {
		t.Errorf("stdout %q missing install_id=", stdout)
	}
	if !strings.Contains(stdout, "events will post to") {
		t.Errorf("stdout %q missing events will post to", stdout)
	}
	// install_id file must exist with mode 0600
	idPath := filepath.Join(dir, "install_id")
	info, err := os.Stat(idPath)
	if err != nil {
		t.Fatalf("install_id file missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("install_id mode = %o, want 0600", info.Mode().Perm())
	}
}

func TestTelemetryCmd_EnableIdempotent(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	// Extract install_id from first enable
	stdout1, _, _ := telemetryRoundtrip(t, dir, srv.URL, "enable")
	id1 := extractInstallID(stdout1)
	stdout2, _, _ := telemetryRoundtrip(t, dir, srv.URL, "enable")
	id2 := extractInstallID(stdout2)
	if id1 == "" || id2 == "" {
		t.Fatalf("could not extract install_id: %q %q", stdout1, stdout2)
	}
	if id1 != id2 {
		t.Errorf("install_id changed: first=%q second=%q", id1, id2)
	}
}

func TestTelemetryCmd_EnableThenStatus(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	stdout1, _, _ := telemetryRoundtrip(t, dir, srv.URL, "enable")
	id1 := extractInstallID(stdout1)
	stdout2, _, exit := telemetryRoundtrip(t, dir, srv.URL, "status")
	if exit != 0 {
		t.Errorf("status exit = %d, want 0", exit)
	}
	if !strings.Contains(stdout2, id1) {
		t.Errorf("status output %q does not contain install_id %q", stdout2, id1)
	}
	if !strings.Contains(stdout2, "enabled") {
		t.Errorf("status output %q does not say enabled", stdout2)
	}
}

func TestTelemetryCmd_EnableDisableStatus(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	_, _, _ = telemetryRoundtrip(t, dir, srv.URL, "enable")
	_, _, _ = telemetryRoundtrip(t, dir, srv.URL, "disable")
	stdout, _, exit := telemetryRoundtrip(t, dir, srv.URL, "status")
	if exit != 0 {
		t.Errorf("status after disable exit = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "disabled") {
		t.Errorf("status output %q does not say disabled", stdout)
	}
	if !strings.Contains(stdout, "retained") {
		t.Errorf("status output %q does not say retained", stdout)
	}
}

func TestTelemetryCmd_ResetID(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	stdout1, _, _ := telemetryRoundtrip(t, dir, srv.URL, "enable")
	id1 := extractInstallID(stdout1)
	stdout2, _, exit := telemetryRoundtrip(t, dir, srv.URL, "reset-id")
	if exit != 0 {
		t.Errorf("reset-id exit = %d, want 0", exit)
	}
	if !strings.Contains(stdout2, "regenerated") {
		t.Errorf("reset-id output %q does not say regenerated", stdout2)
	}
	// install_id should differ now
	stdout3, _, _ := telemetryRoundtrip(t, dir, srv.URL, "status")
	id2 := extractInstallIDFromStatus(stdout3)
	if id1 != "" && id2 != "" && id1 == id2 {
		t.Errorf("install_id unchanged after reset-id: %q", id1)
	}
}

func TestTelemetryCmd_ResetIDWithoutEnable(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exit := telemetryRoundtrip(t, dir, "", "reset-id")
	if exit == 0 {
		t.Error("reset-id without enable should fail")
	}
	if stderr == "" {
		t.Error("reset-id without enable should print error to stderr")
	}
}

func TestTelemetryCmd_Export(t *testing.T) {
	_, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	stdout1, _, _ := telemetryRoundtrip(t, dir, srv.URL, "enable")
	id := extractInstallID(stdout1)
	// Trigger a run.completed emission via Emitter (simulate via enable)
	stdout2, _, exit := telemetryRoundtrip(t, dir, srv.URL, "export")
	if exit != 0 {
		t.Errorf("export exit = %d, want 0", exit)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout2), &out); err != nil {
		t.Fatalf("export output is not valid JSON: %v\nOutput: %s", err, stdout2)
	}
	if out["install_id"] != id {
		t.Errorf("export install_id = %v, want %s", out["install_id"], id)
	}
}

func TestTelemetryCmd_DeleteRequest_Online(t *testing.T) {
	cs, srv := newTelemetryCaptureServer(t, http.StatusOK)
	dir := t.TempDir()
	_, _, _ = telemetryRoundtrip(t, dir, srv.URL, "enable")
	stdout, _, exit := telemetryRoundtrip(t, dir, srv.URL, "delete-request")
	if exit != 0 {
		t.Errorf("delete-request exit = %d, want 0", exit)
	}
	if !strings.Contains(stdout, "deleted") {
		t.Errorf("delete-request output %q does not say deleted", stdout)
	}
	// install_id file must be gone
	if _, err := os.Stat(filepath.Join(dir, "install_id")); !os.IsNotExist(err) {
		t.Error("install_id still exists after delete-request")
	}
	// backend should have received the delete event
	cs.mu.Lock()
	defer cs.mu.Unlock()
	found := false
	for _, body := range cs.bodies {
		if et, ok := body["event_type"].(string); ok && et == "telemetry.delete_request" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected telemetry.delete_request event to be posted to backend")
	}
}

func TestTelemetryCmd_DeleteRequest_Offline(t *testing.T) {
	// Point at a dead endpoint
	dir := t.TempDir()
	_ = os.MkdirAll(dir, 0o700)
	t.Setenv("APITEST_CONFIG_DIR", dir)
	t.Setenv("APITEST_TELEMETRY_ENDPOINT", "http://127.0.0.1:1") // unreachable
	var out, errOut bytes.Buffer
	// First enable so there are files to delete
	_ = runWithWriters([]string{"telemetry", "enable"}, &out, &errOut)
	out.Reset()
	errOut.Reset()

	exit := runWithWriters([]string{"telemetry", "delete-request"}, &out, &errOut)
	if exit != 0 {
		t.Errorf("delete-request offline exit = %d, want 0", exit)
	}
	// Files must be removed locally despite offline failure
	if _, err := os.Stat(filepath.Join(dir, "install_id")); !os.IsNotExist(err) {
		t.Error("install_id still exists after offline delete-request")
	}
}

// TestTelemetryCmd_DeleteRequestOnFreshDir verifies that `delete-request` run
// before `enable` produces a clear "nothing to delete" message (not a broken
// "install_id  deleted locally" message with an empty install_id) and exits 0.
func TestTelemetryCmd_DeleteRequestOnFreshDir(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exit := telemetryRoundtrip(t, dir, "", "delete-request")
	if exit != 0 {
		t.Errorf("delete-request on fresh dir exit = %d, want 0", exit)
	}
	if stderr != "" {
		t.Errorf("delete-request on fresh dir wrote to stderr: %q", stderr)
	}
	if !strings.Contains(stdout, "nothing to delete") {
		t.Errorf("delete-request on fresh dir output %q does not contain 'nothing to delete'", stdout)
	}
	// Must not contain the malformed double-space message from an empty install_id
	if strings.Contains(stdout, "install_id  deleted") {
		t.Errorf("delete-request produced broken message with empty install_id: %q", stdout)
	}
	// Must not claim "event posted" when nothing was sent
	if strings.Contains(stdout, "event posted") {
		t.Errorf("delete-request on fresh dir falsely claims 'event posted': %q", stdout)
	}
}

// TestTelemetryCmd_DisableOnFreshDir verifies that `disable` before `enable`
// exits 0 and does not leak the internal errStateMissing sentinel to the user.
func TestTelemetryCmd_DisableOnFreshDir(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exit := telemetryRoundtrip(t, dir, "", "disable")
	if exit != 0 {
		t.Errorf("disable on fresh dir exit = %d, want 0", exit)
	}
	if strings.Contains(stderr, "errStateMissing") || strings.Contains(stderr, "state file absent") {
		t.Errorf("disable on fresh dir leaked internal sentinel to stderr: %q", stderr)
	}
	// Should either print the normal disabled message or a quiet no-op message
	_ = stdout // any non-error output is acceptable
}

func TestTelemetryCmd_UnknownSubcommand(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exit := telemetryRoundtrip(t, dir, "", "bogus")
	if exit != 1 {
		t.Errorf("exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr, "Unknown") {
		t.Errorf("stderr %q does not mention Unknown", stderr)
	}
}

func TestTelemetryCmd_Help(t *testing.T) {
	dir := t.TempDir()
	stdout, _, exit := telemetryRoundtrip(t, dir, "", "--help")
	if exit != 0 {
		t.Errorf("--help exit = %d, want 0", exit)
	}
	for _, verb := range []string{"enable", "disable", "status", "reset-id", "export", "delete-request"} {
		if !strings.Contains(stdout, verb) {
			t.Errorf("--help output missing verb %q", verb)
		}
	}
}

func TestPrintHelpListsTelemetry(t *testing.T) {
	var buf bytes.Buffer
	printHelpTo(&buf)
	help := buf.String()
	if !strings.Contains(help, "telemetry") {
		t.Error("printHelpTo output does not mention 'telemetry'")
	}
}

func TestUsageSynopsisHasTelemetry(t *testing.T) {
	got := usageSynopses["telemetry"]
	if got == "" {
		t.Error("usageSynopses[\"telemetry\"] is empty")
	}
}

func newTelemetryCaptureServer(t *testing.T, status int) (*telemetryCaptureServerState, *httptest.Server) {
	t.Helper()
	state := &telemetryCaptureServerState{status: status}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		state.bodies = append(state.bodies, b)
		state.requests = append(state.requests, r.Header.Get("Idempotency-Key"))
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return state, srv
}

type telemetryCaptureServerState struct {
	mu       sync.Mutex
	bodies   []map[string]any
	requests []string
	status   int
}

// extractInstallID finds "install_id=<uuid>" in a line and returns the uuid.
func extractInstallID(output string) string {
	const prefix = "install_id="
	idx := strings.Index(output, prefix)
	if idx < 0 {
		return ""
	}
	rest := output[idx+len(prefix):]
	// UUID is 36 chars
	if len(rest) >= 36 {
		candidate := rest[:36]
		if uuidRE.MatchString(candidate) {
			return candidate
		}
	}
	return ""
}

// extractInstallIDFromStatus extracts the install_id from a status output line.
func extractInstallIDFromStatus(output string) string {
	return extractInstallID(output)
}
