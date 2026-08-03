package telemetry_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/telemetry"
)

// readEvents parses an NDJSON telemetry file into decoded records.
func readEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %q is not valid JSON: %v", line, err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan events file: %v", err)
	}
	return out
}

func TestFileSink_Emit(t *testing.T) {
	t.Run("writes_one_ndjson_record_per_event", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.ndjson")
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "run.completed", map[string]any{"exit_code": 0}); err != nil {
			t.Fatalf("emit: %v", err)
		}
		if err := sink.Emit("install-1", "run.completed", map[string]any{"exit_code": 3}); err != nil {
			t.Fatalf("emit: %v", err)
		}

		events := readEvents(t, path)
		if len(events) != 2 {
			t.Fatalf("expected 2 records, got %d: %v", len(events), events)
		}
		if events[0]["event_type"] != "run.completed" {
			t.Errorf("event_type = %v, want run.completed", events[0]["event_type"])
		}
		if events[0]["install_id"] != "install-1" {
			t.Errorf("install_id = %v, want install-1", events[0]["install_id"])
		}
		payload, ok := events[1]["event_payload"].(map[string]any)
		if !ok {
			t.Fatalf("event_payload is not an object: %v", events[1]["event_payload"])
		}
		if payload["exit_code"] != float64(3) {
			t.Errorf("exit_code = %v, want 3", payload["exit_code"])
		}
	})

	t.Run("records_carry_a_timestamp", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.ndjson")
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "run.completed", nil); err != nil {
			t.Fatalf("emit: %v", err)
		}

		events := readEvents(t, path)
		if len(events) != 1 {
			t.Fatalf("expected 1 record, got %d", len(events))
		}
		at, _ := events[0]["at"].(string)
		if at == "" {
			t.Errorf("expected an 'at' timestamp, got: %v", events[0])
		}
	})

	t.Run("nil_payload_is_written_as_empty_object", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.ndjson")
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "cmd.invoked", nil); err != nil {
			t.Fatalf("emit: %v", err)
		}

		events := readEvents(t, path)
		payload, ok := events[0]["event_payload"].(map[string]any)
		if !ok {
			t.Fatalf("event_payload is not an object: %v", events[0]["event_payload"])
		}
		if len(payload) != 0 {
			t.Errorf("expected empty payload object, got: %v", payload)
		}
	})

	t.Run("creates_parent_directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "nested", "dir", "telemetry.ndjson")
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "cmd.invoked", nil); err != nil {
			t.Fatalf("emit into a missing directory should create it: %v", err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected the events file to exist: %v", err)
		}
	})

	t.Run("file_is_owner_only", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.ndjson")
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "cmd.invoked", nil); err != nil {
			t.Fatalf("emit: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissions = %o, want 600", perm)
		}
	})

	t.Run("appends_to_an_existing_file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "telemetry.ndjson")
		if err := os.WriteFile(path, []byte(`{"event_type":"pre-existing"}`+"\n"), 0o600); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		sink := telemetry.NewFileSink(path)

		if err := sink.Emit("install-1", "cmd.invoked", nil); err != nil {
			t.Fatalf("emit: %v", err)
		}

		events := readEvents(t, path)
		if len(events) != 2 {
			t.Fatalf("expected the pre-existing record to survive, got %d records", len(events))
		}
		if events[0]["event_type"] != "pre-existing" {
			t.Errorf("first record = %v, want the pre-existing one", events[0])
		}
	})
}
