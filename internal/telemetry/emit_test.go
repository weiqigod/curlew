package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sinkAt returns a FileSink writing to a fresh file under dir, plus its path.
func sinkAt(t *testing.T, name string) (*FileSink, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	return NewFileSink(path), path
}

// eventLines returns the non-empty lines written to the events file. A missing
// file counts as zero events — nothing was ever emitted.
func eventLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test-controlled path
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func TestEmitter_DisabledWritesNothing(t *testing.T) {
	store, _ := newTestStore(t)
	// Enable then disable — install_id retained, enabled=false
	_, _ = store.Enable("")
	_ = store.Disable()

	sink, path := sinkAt(t, "telemetry.ndjson")
	NewEmitter(store, sink).Emit("run.completed", map[string]any{})

	if got := eventLines(t, path); len(got) != 0 {
		t.Errorf("expected 0 events when disabled, got %d: %v", len(got), got)
	}
	// Emission should be recorded as "disabled" in ring buffer
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 {
		t.Fatal("expected a disabled emission to be recorded")
	}
	if ems[0].Status != "disabled" {
		t.Errorf("emission status = %q, want disabled", ems[0].Status)
	}
}

func TestEmitter_NoInstallIDWritesNothing(t *testing.T) {
	store, _ := newTestStore(t) // never enabled

	sink, path := sinkAt(t, "telemetry.ndjson")
	NewEmitter(store, sink).Emit("run.completed", nil)

	if got := eventLines(t, path); len(got) != 0 {
		t.Errorf("expected 0 events when no install_id, got %d: %v", len(got), got)
	}
}

func TestEmitter_EnabledAppendsToFile(t *testing.T) {
	store, _ := newTestStore(t)
	id, err := store.Enable("")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}

	sink, path := sinkAt(t, "telemetry.ndjson")
	NewEmitter(store, sink).Emit("run.completed", map[string]any{"session_id": "s1"})

	lines := eventLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(lines), lines)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("event is not valid JSON: %v", err)
	}
	if rec["install_id"] != id {
		t.Errorf("install_id = %v, want %s", rec["install_id"], id)
	}
	if rec["event_type"] != "run.completed" {
		t.Errorf("event_type = %v, want run.completed", rec["event_type"])
	}
	// Emission should be recorded as "ok"
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 || ems[0].Status != "ok" {
		t.Errorf("expected ok emission, got %+v", ems)
	}
}

func TestEmitter_SinkErrorSwallowed(t *testing.T) {
	store, _ := newTestStore(t)
	if _, err := store.Enable(""); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	// Point the sink at a path whose parent is a regular file, so the write
	// cannot succeed. The caller must not see the failure.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	sink := NewFileSink(filepath.Join(blocker, "telemetry.ndjson"))
	NewEmitter(store, sink).Emit("run.completed", nil)

	// Emission should be recorded as "error"
	ems, err := store.RecentEmissions()
	if err != nil {
		t.Fatalf("RecentEmissions: %v", err)
	}
	if len(ems) == 0 || ems[0].Status != "error" {
		t.Errorf("expected error emission, got %+v", ems)
	}
}
