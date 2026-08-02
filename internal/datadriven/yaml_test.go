package datadriven

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadYAML(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantRows int
		wantCols []string
		wantErr  error
	}{
		{
			"valid list of maps",
			"- name: john\n  age: 30\n- name: jane\n  age: 25",
			2,
			[]string{"age", "name"},
			nil,
		},
		{
			"single item",
			"- name: alice",
			1,
			[]string{"name"},
			nil,
		},
		{
			"empty list",
			"[]",
			0, nil, ErrEmptyDataFile,
		},
		{
			"empty file",
			"",
			0, nil, ErrEmptyDataFile,
		},
		{
			"not a list",
			"name: john",
			0, nil, ErrMalformedData,
		},
		{
			"list of non-maps",
			"- hello\n- world",
			0, nil, ErrMalformedData,
		},
		{
			"mixed keys across items",
			"- name: john\n  email: j@x.com\n- name: jane\n  phone: '555'",
			2,
			[]string{"email", "name", "phone"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "data.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			ds, err := loadYAML(path)
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
			if ds == nil {
				t.Fatal("expected non-nil DataSet")
			}
			if len(ds.Rows) != tt.wantRows {
				t.Errorf("rows = %d, want %d", len(ds.Rows), tt.wantRows)
			}
			if tt.wantCols != nil {
				if len(ds.Columns) != len(tt.wantCols) {
					t.Fatalf("columns = %v, want %v", ds.Columns, tt.wantCols)
				}
				for i, c := range tt.wantCols {
					if ds.Columns[i] != c {
						t.Errorf("column[%d] = %q, want %q", i, ds.Columns[i], c)
					}
				}
			}
		})
	}
}

func TestLoadYAML_RowValues(t *testing.T) {
	dir := t.TempDir()
	content := "- name: john\n  email: john@example.com\n- name: jane\n  email: jane@example.com"
	path := filepath.Join(dir, "data.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ds, err := loadYAML(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ds.Rows[0]["name"] != "john" {
		t.Errorf("row[0][name] = %q, want %q", ds.Rows[0]["name"], "john")
	}
	if ds.Rows[0]["email"] != "john@example.com" {
		t.Errorf("row[0][email] = %q, want %q", ds.Rows[0]["email"], "john@example.com")
	}
	if ds.Rows[1]["name"] != "jane" {
		t.Errorf("row[1][name] = %q, want %q", ds.Rows[1]["name"], "jane")
	}
}

func TestLoadYAML_TypeConversion(t *testing.T) {
	dir := t.TempDir()
	content := "- active: true\n  count: 42\n  price: 3.14\n  empty: null"
	path := filepath.Join(dir, "data.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ds, err := loadYAML(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(ds.Rows))
	}

	row := ds.Rows[0]
	if row["active"] != "true" {
		t.Errorf("active = %q, want %q", row["active"], "true")
	}
	if row["count"] != "42" {
		t.Errorf("count = %q, want %q", row["count"], "42")
	}
	if row["price"] != "3.14" {
		t.Errorf("price = %q, want %q", row["price"], "3.14")
	}
	if row["empty"] != "" {
		t.Errorf("empty = %q, want %q", row["empty"], "")
	}
}

func TestLoadYAML_NestedValuesStringified(t *testing.T) {
	dir := t.TempDir()
	content := "- name: john\n  meta:\n    age: 30\n    city: NYC"
	path := filepath.Join(dir, "data.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ds, err := loadYAML(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ds.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(ds.Rows))
	}

	// Nested map should be stringified (non-empty)
	meta := ds.Rows[0]["meta"]
	if meta == "" {
		t.Error("expected non-empty string for nested map value")
	}
}
