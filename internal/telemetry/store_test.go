package telemetry

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", "") // ensure no env leakage
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s, dir
}

func TestStoreEnable(t *testing.T) {
	s, dir := newTestStore(t)
	id, err := s.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if id == "" {
		t.Error("Enable returned empty install_id")
	}
	// install_id file must exist with mode 0600
	info, err := os.Stat(dir + "/install_id")
	if err != nil {
		t.Fatalf("install_id file missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("install_id mode = %o, want 0600", info.Mode().Perm())
	}
	// telemetry.json must exist with mode 0600
	info2, err := os.Stat(dir + "/telemetry.json")
	if err != nil {
		t.Fatalf("telemetry.json missing: %v", err)
	}
	if runtime.GOOS != "windows" && info2.Mode().Perm() != 0o600 {
		t.Errorf("telemetry.json mode = %o, want 0600", info2.Mode().Perm())
	}
	// Status must report enabled
	enabled, gotID, _, statusErr := s.Status()
	if statusErr != nil {
		t.Fatalf("Status after Enable: %v", statusErr)
	}
	if !enabled {
		t.Error("Status enabled=false after Enable")
	}
	if gotID != id {
		t.Errorf("Status install_id=%q, want %q", gotID, id)
	}
}

func TestStoreEnableIdempotent(t *testing.T) {
	s, _ := newTestStore(t)
	id1, err := s.Enable("")
	if err != nil {
		t.Fatalf("first Enable: %v", err)
	}
	id2, err := s.Enable("")
	if err != nil {
		t.Fatalf("second Enable: %v", err)
	}
	if id1 != id2 {
		t.Errorf("Enable idempotency failed: first=%q, second=%q", id1, id2)
	}
}

func TestStoreStatusNoInstallID(t *testing.T) {
	s, _ := newTestStore(t)
	_, _, _, err := s.Status()
	if !errors.Is(err, ErrNotEnabled) {
		t.Errorf("Status on fresh store: err=%v, want ErrNotEnabled", err)
	}
}

func TestStoreStatusDisabledRetainsID(t *testing.T) {
	s, dir := newTestStore(t)
	_, _ = s.Enable("")
	if err := s.Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	// install_id file must still be on disk
	if _, err := os.Stat(dir + "/install_id"); err != nil {
		t.Fatalf("install_id gone after Disable: %v", err)
	}
	// Status should report disabled but not ErrNotEnabled
	enabled, id, _, err := s.Status()
	if err != nil {
		t.Fatalf("Status after Disable: %v", err)
	}
	if enabled {
		t.Error("Status enabled=true after Disable")
	}
	if id == "" {
		t.Error("Status returned empty install_id after Disable")
	}
}

func TestStoreResetID(t *testing.T) {
	s, _ := newTestStore(t)
	origID, err := s.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	// Add an emission so we can verify they are cleared on reset
	_ = s.RecordEmission(Emission{At: time.Now().UTC().Format(time.RFC3339), EventType: "run.completed", Status: "ok"})
	newID, err := s.ResetID()
	if err != nil {
		t.Fatalf("ResetID: %v", err)
	}
	if newID == origID {
		t.Error("ResetID returned same install_id as original")
	}
	if newID == "" {
		t.Error("ResetID returned empty install_id")
	}
	// Emissions must be cleared
	ems, err := s.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions after ResetID: %v", err)
	}
	if len(ems) != 0 {
		t.Errorf("expected 0 emissions after ResetID, got %d", len(ems))
	}
}

func TestStoreDeleteLocal(t *testing.T) {
	s, dir := newTestStore(t)
	origID, err := s.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	priorID, err := s.DeleteLocal()
	if err != nil {
		t.Fatalf("DeleteLocal: %v", err)
	}
	if priorID != origID {
		t.Errorf("DeleteLocal priorID=%q, want %q", priorID, origID)
	}
	// Both files must be gone
	if _, err := os.Stat(dir + "/install_id"); !os.IsNotExist(err) {
		t.Error("install_id still exists after DeleteLocal")
	}
	if _, err := os.Stat(dir + "/telemetry.json"); !os.IsNotExist(err) {
		t.Error("telemetry.json still exists after DeleteLocal")
	}
}

func TestStoreDeleteLocalMissing(t *testing.T) {
	s, _ := newTestStore(t)
	// Files never created; DeleteLocal must not error
	priorID, err := s.DeleteLocal()
	if err != nil {
		t.Errorf("DeleteLocal on missing files: %v", err)
	}
	if priorID != "" {
		t.Errorf("DeleteLocal priorID=%q on missing files, want empty", priorID)
	}
}

func TestStoreRecordEmissionCaps(t *testing.T) {
	s, _ := newTestStore(t)
	_, _ = s.Enable("")
	now := time.Now().UTC()
	for i := range 15 {
		em := Emission{
			At:        now.Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			EventType: "run.completed",
			Status:    "ok",
		}
		if err := s.RecordEmission(em); err != nil {
			t.Fatalf("RecordEmission %d: %v", i, err)
		}
	}
	ems, err := s.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) != maxRecentEmissions {
		t.Errorf("got %d emissions, want %d", len(ems), maxRecentEmissions)
	}
}

func TestStoreRecordEmissionWithoutEnable(t *testing.T) {
	s, _ := newTestStore(t)
	// No-op: telemetry.json does not exist
	err := s.RecordEmission(Emission{At: time.Now().UTC().Format(time.RFC3339), EventType: "run.completed", Status: "ok"})
	if err != nil {
		t.Errorf("RecordEmission without enable: %v", err)
	}
}

func TestStoreResolvedEndpointPrecedence(t *testing.T) {
	s, _ := newTestStore(t)
	// Env > state > default
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", "http://env.example.com")
	state := State{Endpoint: "http://state.example.com"}
	got := s.resolvedEndpoint(state)
	if got != "http://env.example.com" {
		t.Errorf("env precedence: got %q, want http://env.example.com", got)
	}
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", "")
	got = s.resolvedEndpoint(state)
	if got != "http://state.example.com" {
		t.Errorf("state precedence: got %q, want http://state.example.com", got)
	}
	state.Endpoint = ""
	got = s.resolvedEndpoint(state)
	if got != defaultEndpoint {
		t.Errorf("default precedence: got %q, want %q", got, defaultEndpoint)
	}
}

func TestStoreFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission checks not applicable on Windows")
	}
	s, dir := newTestStore(t)
	_, err := s.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	for _, name := range []string{"install_id", "telemetry.json"} {
		info, err := os.Stat(dir + "/" + name)
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 0600", name, info.Mode().Perm())
		}
	}
}

func TestStoreResolvedEndpoint_Public(t *testing.T) {
	s, _ := newTestStore(t)
	_, _ = s.Enable("")
	t.Setenv("CURLEW_TELEMETRY_ENDPOINT", "http://public.example.com")
	ep := s.ResolvedEndpoint()
	if ep != "http://public.example.com" {
		t.Errorf("ResolvedEndpoint = %q, want http://public.example.com", ep)
	}
}

func TestStoreAtomicWriteOnPartialFailure(t *testing.T) {
	// After Enable, no .tmp file should remain
	s, dir := newTestStore(t)
	_, err := s.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if len(e.Name()) > 4 && e.Name()[len(e.Name())-4:] == ".tmp" {
			t.Errorf("found leftover temp file: %s", e.Name())
		}
	}
}

// TestStoreDisableOnFreshDir verifies that Disable is a no-op (returns nil) when
// telemetry.json has never been created, i.e., the user runs `disable` before
// ever running `enable`. The store is already disabled; leaking errStateMissing
// to the caller would be incorrect.
func TestStoreDisableOnFreshDir(t *testing.T) {
	s, _ := newTestStore(t)
	// No Enable has been called — telemetry.json does not exist.
	if err := s.Disable(); err != nil {
		t.Errorf("Disable on fresh dir: got error %v, want nil (no-op)", err)
	}
}
