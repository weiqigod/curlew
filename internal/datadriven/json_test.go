package datadriven

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSON(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantRows int
		wantErr  error
	}{
		{"valid array of objects", `[{"name":"john"},{"name":"jane"}]`, 2, nil},
		{"empty array", `[]`, 0, ErrEmptyDataFile},
		{"nested values flattened to string", `[{"name":"john","meta":{"age":30}}]`, 1, nil},
		{"not an array", `{"name":"john"}`, 0, ErrMalformedData},
		{"invalid JSON", `[{broken`, 0, ErrMalformedData},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			ds, err := loadJSON(path)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ds.Rows) != tt.wantRows {
				t.Errorf("rows = %d, want %d", len(ds.Rows), tt.wantRows)
			}
		})
	}
}

func TestLoadJSON_RowValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `[{"name":"john","email":"john@x.com"},{"name":"jane","email":"jane@x.com"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds, err := loadJSON(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds.Rows[0]["name"] != "john" {
		t.Errorf("row 0 name = %q, want %q", ds.Rows[0]["name"], "john")
	}
	if ds.Rows[1]["email"] != "jane@x.com" {
		t.Errorf("row 1 email = %q, want %q", ds.Rows[1]["email"], "jane@x.com")
	}
}

func TestLoadJSON_NestedValueToString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := `[{"name":"john","meta":{"age":30,"city":"NYC"}}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds, err := loadJSON(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	meta := ds.Rows[0]["meta"]
	if meta == "" {
		t.Fatal("expected non-empty meta value")
	}
	// Nested values should be JSON-encoded strings
	if meta[0] != '{' {
		t.Errorf("meta = %q, expected JSON object string", meta)
	}
}

func TestLoadJSON_MixedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	// Objects with different key sets
	content := `[{"name":"john","email":"john@x.com"},{"name":"jane","phone":"555-0100"}]`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds, err := loadJSON(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(ds.Rows))
	}
	// Second row should not have email key
	if _, ok := ds.Rows[1]["email"]; ok {
		t.Error("row 1 should not have email key")
	}
	// But should have phone
	if ds.Rows[1]["phone"] != "555-0100" {
		t.Errorf("row 1 phone = %q, want %q", ds.Rows[1]["phone"], "555-0100")
	}
}
