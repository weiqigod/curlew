package datadriven

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCSV(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantRows int
		wantCols []string
		wantErr  error
	}{
		{
			"valid 3 rows",
			"name,email\njohn,john@x.com\njane,jane@x.com\nbob,bob@x.com",
			3,
			[]string{"name", "email"},
			nil,
		},
		{
			"single row",
			"name\nalice",
			1,
			[]string{"name"},
			nil,
		},
		{
			"empty file no header",
			"",
			0,
			nil,
			ErrEmptyDataFile,
		},
		{
			"header only no data",
			"name,email\n",
			0,
			nil,
			ErrEmptyDataFile,
		},
		{
			"inconsistent columns",
			"name,email\njohn",
			0,
			nil,
			ErrMalformedData,
		},
		{
			"quoted fields with commas",
			"name,address\njohn,\"123 Main St, Apt 4\"\njane,\"456 Oak Ave\"",
			2,
			[]string{"name", "address"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "test.csv")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			ds, err := loadCSV(path)
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
			if tt.wantCols != nil {
				if len(ds.Columns) != len(tt.wantCols) {
					t.Errorf("columns = %v, want %v", ds.Columns, tt.wantCols)
				}
				for i, c := range tt.wantCols {
					if i < len(ds.Columns) && ds.Columns[i] != c {
						t.Errorf("column[%d] = %q, want %q", i, ds.Columns[i], c)
					}
				}
			}
		})
	}
}

func TestLoadCSV_RowValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	content := "name,email\njohn,john@x.com\njane,jane@x.com"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds, err := loadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ds.Rows[0]["name"] != "john" {
		t.Errorf("row 0 name = %q, want %q", ds.Rows[0]["name"], "john")
	}
	if ds.Rows[0]["email"] != "john@x.com" {
		t.Errorf("row 0 email = %q, want %q", ds.Rows[0]["email"], "john@x.com")
	}
	if ds.Rows[1]["name"] != "jane" {
		t.Errorf("row 1 name = %q, want %q", ds.Rows[1]["name"], "jane")
	}
}
